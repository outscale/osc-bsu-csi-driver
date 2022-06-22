/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package osc

import (
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/outscale/osc-sdk-go/v2"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

// ********************* CCM ServiceResolver functions *********************

// SetupMetadataResolver resolver for osc metadata service
func SetupMetadataResolver() endpoints.ResolverFunc {
	return func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		return endpoints.ResolvedEndpoint{
			URL:           "http://169.254.169.254/latest",
			SigningRegion: "custom-signing-region",
		}, nil
	}
}

// Endpoint builder for outscale
func Endpoint(region string, service string) string {
	return "https://" + service + "." + region + ".outscale.com"
}

// SetupServiceResolver resolver for osc service
func SetupServiceResolver(region string) endpoints.ResolverFunc {

	return func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {

		supportedService := map[string]string{
			endpoints.Ec2ServiceID:                  "fcu",
			endpoints.ElasticloadbalancingServiceID: "lbu",
			endpoints.IamServiceID:                  "eim",
			endpoints.DirectconnectServiceID:        "directlink",
			endpoints.KmsServiceID:                  "kms",
		}
		var oscService string
		var ok bool
		if oscService, ok = supportedService[service]; ok {
			return endpoints.ResolvedEndpoint{
				URL:           Endpoint(region, oscService),
				SigningRegion: region,
				SigningName:   service,
			}, nil
		}
		return endpoints.DefaultResolver().EndpointFor(
			service, region, optFns...)
	}
}

// ********************* CCM Utils functions *********************

// Following functions are used to set outscale endpoints
func newEC2MetadataSvc() *ec2metadata.EC2Metadata {
	awsConfig := &aws.Config{
		EndpointResolver: endpoints.ResolverFunc(SetupMetadataResolver()),
	}
	awsConfig.WithLogLevel(aws.LogDebugWithSigning | aws.LogDebugWithHTTPBody | aws.LogDebugWithRequestRetries | aws.LogDebugWithRequestErrors)

	sess := session.Must(session.NewSession(awsConfig))

	addOscUserAgent(&sess.Handlers)

	return ec2metadata.New(sess)
}

// NewMetadata create a new metadata service
func NewMetadata() (MetadataService, error) {
	klog.V(10).Infof("NewMetadata")
	svc := newEC2MetadataSvc()

	metadata, err := NewMetadataService(svc)
	if err != nil {
		return nil, fmt.Errorf("could not get metadata from AWS: %v", err)
	}
	return metadata, err
}

// NewSession create a new session
func NewSession(meta EC2Metadata) (*session.Session, error) {
	initMetadata := func(meta EC2Metadata) (MetadataService, error) {
		if meta == nil {
			return NewMetadata()
		}
		value, ok := meta.(MetadataService)
		if ok {
			return value, nil
		}
		return nil, fmt.Errorf("Unable to retrieve Metadata")
	}
	provider := []credentials.Provider{
		&credentials.EnvProvider{},
		&credentials.SharedCredentialsProvider{},
	}
	metadata, err := initMetadata(meta)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize OSC Metadata session: %v", err)
	}

	awsConfig := &aws.Config{
		Region:                        aws.String(metadata.GetRegion()),
		Credentials:                   credentials.NewChainCredentials(provider),
		CredentialsChainVerboseErrors: aws.Bool(true),
		EndpointResolver:              endpoints.ResolverFunc(SetupServiceResolver(metadata.GetRegion())),
	}
	awsConfig.WithLogLevel(aws.LogDebugWithSigning | aws.LogDebugWithHTTPBody | aws.LogDebugWithRequestRetries | aws.LogDebugWithRequestErrors)
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize NewSession session: %v", err)
	}
	return sess, nil
}

func newEc2Filter(name string, values ...string) *ec2.Filter {
	filter := &ec2.Filter{
		Name: aws.String(name),
	}
	for _, value := range values {
		filter.Values = append(filter.Values, aws.String(value))
	}
	return filter
}

// mapNodeNameToPrivateDNSName maps a k8s NodeName to an AWS Instance PrivateDNSName
// This is a simple string cast
func mapNodeNameToPrivateDNSName(nodeName types.NodeName) string {
	return string(nodeName)
}

// mapInstanceToNodeName maps an OSC instance to a k8s NodeName, by extracting the PrivateDNSName
func mapInstanceToNodeName(i *osc.Vm) types.NodeName {
	return types.NodeName(aws.StringValue(i.PrivateDnsName))
}

// Returns the first security group for an instance, or nil
// We only create instances with one security group, so we don't expect multiple security groups.
// However, if there are multiple security groups, we will choose the one tagged with our cluster filter.
// Otherwise we will return an error.
func findSecurityGroupForInstance(instance *osc.Vm, taggedSecurityGroups map[string]*ec2.SecurityGroup) (*ec2.GroupIdentifier, error) {
	instanceID := instance.GetVmId()

	klog.Infof("findSecurityGroupForInstance instance.InstanceId : %v", instance.GetVmId())
	klog.Infof("findSecurityGroupForInstance instance.SecurityGroups : %v", instance.SecurityGroups)
	klog.Infof("findSecurityGroupForInstance taggedSecurityGroups : %v", taggedSecurityGroups)

	var tagged []*ec2.GroupIdentifier
	var untagged []*ec2.GroupIdentifier
	for _, group := range instance.GetSecurityGroups() {
		groupID := group.GetSecurityGroupId()
		if groupID == "" {
			klog.Warningf("Ignoring security group without id for instance %q: %v", instanceID, group)
			continue
		}
		_, isTagged := taggedSecurityGroups[groupID]
		ec2Group := ec2.GroupIdentifier{
			GroupId:   &groupID,
			GroupName: group.SecurityGroupName,
		}
		if isTagged {
			tagged = append(tagged, &ec2Group)
		} else {
			untagged = append(untagged, &ec2Group)
		}
	}

	klog.V(4).Infof("looking sg tagged: %v", tagged)
	klog.V(4).Infof("looking sg untagged: %v", untagged)

	if len(tagged) > 0 {
		// We create instances with one SG
		// If users create multiple SGs, they must tag one of them as being k8s owned
		if len(tagged) != 1 {
			taggedGroups := ""
			for _, v := range tagged {
				taggedGroups += fmt.Sprintf("%s(%s) ", *v.GroupId, *v.GroupName)
			}
			return nil, fmt.Errorf("Multiple tagged security groups found for instance %s; ensure only the k8s security group is tagged; the tagged groups were %v", instanceID, taggedGroups)
		}
		return tagged[0], nil
	}

	if len(untagged) > 0 {
		// For back-compat, we will allow a single untagged SG
		if len(untagged) != 1 {
			return nil, fmt.Errorf("Multiple untagged security groups found for instance %s; ensure the k8s security group is tagged", instanceID)
		}
		return untagged[0], nil
	}

	klog.Warningf("No security group found for instance %q", instanceID)
	return nil, nil
}

// buildListener creates a new listener from the given port, adding an SSL certificate
// if indicated by the appropriate annotations.
func buildListener(port v1.ServicePort, annotations map[string]string, sslPorts *portSets) (*elb.Listener, error) {
	loadBalancerPort := int64(port.Port)
	portName := strings.ToLower(port.Name)
	instancePort := int64(port.NodePort)
	protocol := strings.ToLower(string(port.Protocol))
	instanceProtocol := protocol

	listener := &elb.Listener{}
	listener.InstancePort = &instancePort
	listener.LoadBalancerPort = &loadBalancerPort
	certID := annotations[ServiceAnnotationLoadBalancerCertificate]
	if certID != "" && (sslPorts == nil || sslPorts.numbers.Has(loadBalancerPort) || sslPorts.names.Has(portName)) {
		instanceProtocol = annotations[ServiceAnnotationLoadBalancerBEProtocol]
		if instanceProtocol == "" {
			protocol = "ssl"
			instanceProtocol = "tcp"
		} else {
			protocol = backendProtocolMapping[instanceProtocol]
			if protocol == "" {
				return nil, fmt.Errorf("Invalid backend protocol %s for %s in %s", instanceProtocol, certID, ServiceAnnotationLoadBalancerBEProtocol)
			}
		}
		listener.SSLCertificateId = &certID
	} else if annotationProtocol := annotations[ServiceAnnotationLoadBalancerBEProtocol]; annotationProtocol == "http" {
		instanceProtocol = annotationProtocol
		protocol = "http"
	}
	protocol = strings.ToUpper(protocol)
	instanceProtocol = strings.ToUpper(instanceProtocol)
	listener.Protocol = &protocol
	listener.InstanceProtocol = &instanceProtocol

	return listener, nil
}

func isSubnetPublic(rt []*ec2.RouteTable, subnetID string) (bool, error) {
	var subnetTable *ec2.RouteTable
	for _, table := range rt {
		for _, assoc := range table.Associations {
			if aws.StringValue(assoc.SubnetId) == subnetID {
				subnetTable = table
				break
			}
		}
	}

	if subnetTable == nil {
		// If there is no explicit association, the subnet will be implicitly
		// associated with the VPC's main routing table.
		for _, table := range rt {
			for _, assoc := range table.Associations {
				if aws.BoolValue(assoc.Main) == true {
					klog.V(4).Infof("Assuming implicit use of main routing table %s for %s",
						aws.StringValue(table.RouteTableId), subnetID)
					subnetTable = table
					break
				}
			}
		}
	}

	if subnetTable == nil {
		return false, fmt.Errorf("could not locate routing table for subnet %s", subnetID)
	}

	for _, route := range subnetTable.Routes {
		// There is no direct way in the AWS API to determine if a subnet is public or private.
		// A public subnet is one which has an internet gateway route
		// we look for the gatewayId and make sure it has the prefix of igw to differentiate
		// from the default in-subnet route which is called "local"
		// or other virtual gateway (starting with vgv)
		// or vpc peering connections (starting with pcx).
		if strings.HasPrefix(aws.StringValue(route.GatewayId), "igw") {
			return true, nil
		}
	}

	return false, nil
}

type portSets struct {
	names   sets.String
	numbers sets.Int64
}

// getPortSets returns a portSets structure representing port names and numbers
// that the comma-separated string describes. If the input is empty or equal to
// "*", a nil pointer is returned.
func getPortSets(annotation string) (ports *portSets) {
	if annotation != "" && annotation != "*" {
		ports = &portSets{
			sets.NewString(),
			sets.NewInt64(),
		}
		portStringSlice := strings.Split(annotation, ",")
		for _, item := range portStringSlice {
			port, err := strconv.Atoi(item)
			if err != nil {
				ports.names.Insert(item)
			} else {
				ports.numbers.Insert(int64(port))
			}
		}
	}
	return
}

func toStatus(lb *elb.LoadBalancerDescription) *v1.LoadBalancerStatus {
	status := &v1.LoadBalancerStatus{}

	if aws.StringValue(lb.DNSName) != "" {
		var ingress v1.LoadBalancerIngress
		ingress.Hostname = aws.StringValue(lb.DNSName)
		status.Ingress = []v1.LoadBalancerIngress{ingress}
	}

	return status
}

// Finds the value for a given tag.
func findTag(tags []*ec2.Tag, key string) (string, bool) {
	for _, tag := range tags {
		if aws.StringValue(tag.Key) == key {
			return aws.StringValue(tag.Value), true
		}
	}
	return "", false
}

func isEqualIntPointer(l, r *int64) bool {
	if l == nil {
		return r == nil
	}
	if r == nil {
		return l == nil
	}
	return *l == *r
}

func isEqualStringPointer(l, r *string) bool {
	if l == nil {
		return r == nil
	}
	if r == nil {
		return l == nil
	}
	return *l == *r
}

func ipPermissionExists(newPermission, existing *ec2.IpPermission, compareGroupUserIDs bool) bool {
	if !isEqualIntPointer(newPermission.FromPort, existing.FromPort) {
		return false
	}
	if !isEqualIntPointer(newPermission.ToPort, existing.ToPort) {
		return false
	}
	if !isEqualStringPointer(newPermission.IpProtocol, existing.IpProtocol) {
		return false
	}
	// Check only if newPermission is a subset of existing. Usually it has zero or one elements.
	// Not doing actual CIDR math yet; not clear it's needed, either.
	klog.V(4).Infof("Comparing %v to %v", newPermission, existing)
	if len(newPermission.IpRanges) > len(existing.IpRanges) {
		return false
	}

	for j := range newPermission.IpRanges {
		found := false
		for k := range existing.IpRanges {
			if isEqualStringPointer(newPermission.IpRanges[j].CidrIp, existing.IpRanges[k].CidrIp) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	for _, leftPair := range newPermission.UserIdGroupPairs {
		found := false
		for _, rightPair := range existing.UserIdGroupPairs {
			if isEqualUserGroupPair(leftPair, rightPair, compareGroupUserIDs) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func isEqualUserGroupPair(l, r *ec2.UserIdGroupPair, compareGroupUserIDs bool) bool {
	klog.V(2).Infof("Comparing %v to %v", *l.GroupId, *r.GroupId)
	if isEqualStringPointer(l.GroupId, r.GroupId) {
		if compareGroupUserIDs {
			if isEqualStringPointer(l.UserId, r.UserId) {
				return true
			}
		} else {
			return true
		}
	}

	return false
}

// Derives the region from a valid az name.
// Returns an error if the az is known invalid (empty)
func azToRegion(az string) (string, error) {
	if len(az) < 1 {
		return "", fmt.Errorf("invalid (empty) AZ")
	}
	region := az[:len(az)-1]
	return region, nil
}

// isRegionValid accepts an AWS region name and returns if the region is a
// valid region known to the AWS SDK. Considers the region returned from the
// EC2 metadata service to be a valid region as it's only available on a host
// running in a valid AWS region.
func isRegionValid(region string, metadata EC2Metadata) bool {
	// Does the AWS SDK know about the region?
	for _, p := range endpoints.DefaultPartitions() {
		for r := range p.Regions() {
			if r == region {
				return true
			}
		}
	}

	// ap-northeast-3 is purposely excluded from the SDK because it
	// requires an access request (for more details see):
	// https://github.com/aws/aws-sdk-go/issues/1863
	if region == "ap-northeast-3" {
		return true
	}

	// Fallback to checking if the region matches the instance metadata region
	// (ignoring any user overrides). This just accounts for running an old
	// build of Kubernetes in a new region that wasn't compiled into the SDK
	// when Kubernetes was built.
	if az, err := getAvailabilityZone(metadata); err == nil {
		if r, err := azToRegion(az); err == nil && region == r {
			return true
		}
	}

	return false
}

// newAWSInstance creates a new awsInstance object
func newAWSInstance(ec2Service Compute, instance *osc.Vm) *VM {
	az := ""
	if instance.Placement != nil {
		az = instance.Placement.GetSubregionName()
	}
	self := &VM{
		compute:          ec2Service,
		awsID:            aws.StringValue(instance.VmId),
		nodeName:         mapInstanceToNodeName(instance),
		availabilityZone: az,
		instanceType:     aws.StringValue(instance.VmType),
		vpcID:            aws.StringValue(instance.NetId),
		subnetID:         aws.StringValue(instance.SubnetId),
	}

	return self
}

// extractNodeAddresses maps the instance information from OSC to an array of NodeAddresses
func extractNodeAddresses(instance *osc.Vm) ([]v1.NodeAddress, error) {
	// Not clear if the order matters here, but we might as well indicate a sensible preference order

	if instance == nil {
		return nil, fmt.Errorf("nil instance passed to extractNodeAddresses")
	}

	addresses := []v1.NodeAddress{}

	// handle internal network interfaces
	if len(instance.GetNics()) > 0 {
		for _, networkInterface := range instance.GetNics() {
			// skip network interfaces that are not currently in use
			if *networkInterface.State != "in-use" {
				continue
			}

			for _, internalIP := range networkInterface.GetPrivateIps() {
				if ipAddress := internalIP.GetPrivateIp(); ipAddress != "" {
					ip := net.ParseIP(ipAddress)
					if ip == nil {
						return nil, fmt.Errorf("OSC instance had invalid private address: %s (%q)", instance.GetVmId(), ipAddress)
					}
					addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalIP, Address: ip.String()})
				}
			}
		}
	} else {
		privateIPAddress := instance.GetPrivateIp()
		if privateIPAddress != "" {
			ip := net.ParseIP(privateIPAddress)
			if ip == nil {
				return nil, fmt.Errorf("OSC instance had invalid private address: %s (%s)", instance.GetVmId(), privateIPAddress)
			}
			addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalIP, Address: ip.String()})
		}
	}
	// TODO: Other IP addresses (multiple ips)?
	publicIPAddress := instance.GetPublicIp()
	if publicIPAddress != "" {
		ip := net.ParseIP(publicIPAddress)
		if ip == nil {
			return nil, fmt.Errorf("OSC instance had invalid public address: %s (%s)", instance.GetVmId(), publicIPAddress)
		}
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeExternalIP, Address: ip.String()})
	}
	privateDNSName := aws.StringValue(instance.PrivateDnsName)
	if privateDNSName != "" {
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalDNS, Address: privateDNSName})
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeHostName, Address: privateDNSName})
	}
	publicDNSName := aws.StringValue(instance.PublicDnsName)
	if publicDNSName != "" {
		addresses = append(addresses, v1.NodeAddress{Type: v1.NodeExternalDNS, Address: publicDNSName})
	}

	return addresses, nil
}

// parseMetadataLocalHostname parses the output of "local-hostname" metadata.
// If a DHCP option set is configured for a VPC and it has multiple domain names, GetMetadata
// returns a string containing first the hostname followed by additional domain names,
// space-separated. For example, if the DHCP option set has:
// domain-name = us-west-2.compute.internal a.a b.b c.c d.d;
// $ curl http://169.254.169.254/latest/meta-data/local-hostname
// ip-192-168-111-51.us-west-2.compute.internal a.a b.b c.c d.d
func parseMetadataLocalHostname(metadata string) (string, []string) {
	localHostnames := strings.Fields(metadata)
	hostname := localHostnames[0]
	internalDNS := []string{hostname}

	privateAddress := strings.Split(hostname, ".")[0]
	for _, h := range localHostnames[1:] {
		internalDNSAddress := privateAddress + "." + h
		internalDNS = append(internalDNS, internalDNSAddress)
	}
	return hostname, internalDNS
}

func getAvailabilityZone(metadata EC2Metadata) (string, error) {
	return metadata.GetMetadata("placement/availability-zone")
}

func updateConfigZone(cfg *CloudConfig, metadata EC2Metadata) error {
	if cfg.Global.Zone == "" {
		if metadata != nil {
			klog.Info("Zone not specified in configuration file; querying AWS metadata service")
			var err error
			cfg.Global.Zone, err = getAvailabilityZone(metadata)
			if err != nil {
				return err
			}
		}
		if cfg.Global.Zone == "" {
			return fmt.Errorf("no zone specified in configuration file")
		}
	}

	return nil
}

func newAWSSDKProvider(creds *credentials.Credentials, cfg *CloudConfig) *awsSDKProvider {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("newAWSSDKProvider(%v,%v)", creds, cfg)
	return &awsSDKProvider{
		creds:          creds,
		cfg:            cfg,
		regionDelayers: make(map[string]*CrossRequestRetryDelay),
	}
}

// *** DEBUG FUNCTIONS

func debugGetFrame(skipFrames int) runtime.Frame {
	// We need the frame at index skipFrames+2, since we never want runtime.Callers and getFrame
	targetFrameIndex := skipFrames + 2

	// Set size to targetFrameIndex+2 to ensure we have room for one more caller than we need
	programCounters := make([]uintptr, targetFrameIndex+2)
	n := runtime.Callers(0, programCounters)

	frame := runtime.Frame{Function: "unknown"}
	if n > 0 {
		frames := runtime.CallersFrames(programCounters[:n])
		for more, frameIndex := true, 0; more && frameIndex <= targetFrameIndex; frameIndex++ {
			var frameCandidate runtime.Frame
			frameCandidate, more = frames.Next()
			if frameIndex == targetFrameIndex {
				frame = frameCandidate
			}
		}
	}

	return frame
}

func debugPrintCallerFunctionName() {
	called := debugGetFrame(1)
	caller := debugGetFrame(2)
	klog.V(10).Infof(
		"DebugStack %s{"+
			"\n\t call	 {\n\t\tFunc:%s, \n\t\tFile:(%s:%d)\n\t}"+
			"\n\t called {\n\t\tFunc:%s, \n\t\tFile:(%s:%d)\n\t}"+
			"\n}",
		called.Function,
		called.Function, called.File, called.Line,
		caller.Function, caller.File, caller.Line)
}

func debugGetCurrentFunctionName() string {
	// Skip debugGetCurrentFunctionName
	return debugGetFrame(1).Function
}

func debugGetCallerFunctionName() string {
	// Skip debugGetCallerFunctionName and the function to get the caller of
	return debugGetFrame(2).Function
}

func Contains(list []string, element string) bool {
	for _, el := range list {
		if el == element {
			return true
		}
	}
	return false
}
