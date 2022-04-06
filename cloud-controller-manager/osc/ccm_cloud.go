/*
Copyright 2014 The Kubernetes Authors.

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
	"context"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	informercorev1 "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"

	cloudprovider "k8s.io/cloud-provider"
	servicehelpers "k8s.io/cloud-provider/service/helpers"
)

// ********************* CCM Cloud Object Def *********************

// Cloud is an implementation of Interface, LoadBalancer and Instances for Amazon Web Services.
type Cloud struct {
	compute  Compute
	elb      ELB
	metadata EC2Metadata
	cfg      *CloudConfig
	region   string
	vpcID    string

	instances cloudprovider.InstancesV2

	tagging awsTagging

	// The AWS instance that we are running on
	// Note that we cache some state in awsInstance (mountpoints), so we must preserve the instance
	selfAWSInstance *awsInstance

	instanceCache instanceCache

	clientBuilder cloudprovider.ControllerClientBuilder
	kubeClient    clientset.Interface

	nodeInformer informercorev1.NodeInformer
	// Extract the function out to make it easier to test
	nodeInformerHasSynced cache.InformerSynced
	eventBroadcaster      record.EventBroadcaster
	eventRecorder         record.EventRecorder
}

// ********************* CCM Cloud Object functions *********************

// ********************* CCM Cloud Context functions *********************
// Builds the awsInstance for the EC2 instance on which we are running.
// This is called when the AWSCloud is initialized, and should not be called otherwise (because the awsInstance for the local instance is a singleton with drive mapping state)
func (c *Cloud) buildSelfAWSInstance() (*awsInstance, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("buildSelfAWSInstance()")
	if c.selfAWSInstance != nil {
		panic("do not call buildSelfAWSInstance directly")
	}
	instanceID, err := c.metadata.GetMetadata("instance-id")
	if err != nil {
		return nil, fmt.Errorf("error fetching instance-id from ec2 metadata service: %q", err)
	}

	// We want to fetch the hostname via the EC2 metadata service
	// (`GetMetadata("local-hostname")`): But see #11543 - we need to use
	// the EC2 API to get the privateDnsName in case of a private DNS zone
	// e.g. mydomain.io, because the metadata service returns the wrong
	// hostname.  Once we're doing that, we might as well get all our
	// information from the instance returned by the EC2 API - it is a
	// single API call to get all the information, and it means we don't
	// have two code paths.
	instance, err := c.getInstanceByID(instanceID)
	if err != nil {
		return nil, fmt.Errorf("error finding instance %s: %q", instanceID, err)
	}
	return newAWSInstance(c.compute, instance), nil
}

// SetInformers implements InformerUser interface by setting up informer-fed caches for aws lib to
// leverage Kubernetes API for caching
func (c *Cloud) SetInformers(informerFactory informers.SharedInformerFactory) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("SetInformers(%v)", informerFactory)
	klog.Infof("Setting up informers for Cloud")
	c.nodeInformer = informerFactory.Core().V1().Nodes()
	c.nodeInformerHasSynced = c.nodeInformer.Informer().HasSynced
}

// AddSSHKeyToAllInstances is currently not implemented.
func (c *Cloud) AddSSHKeyToAllInstances(ctx context.Context, user string, keyData []byte) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("AddSSHKeyToAllInstances(%v,%v)", user, keyData)
	return cloudprovider.NotImplemented
}

// CurrentNodeName returns the name of the current node
func (c *Cloud) CurrentNodeName(ctx context.Context, hostname string) (types.NodeName, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("CurrentNodeName(%v)", hostname)
	return c.selfAWSInstance.nodeName, nil
}

// Initialize passes a Kubernetes clientBuilder interface to the cloud provider
func (c *Cloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder,
	stop <-chan struct{}) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("Initialize(%v,%v)", clientBuilder, stop)
	c.clientBuilder = clientBuilder
	c.kubeClient = clientBuilder.ClientOrDie("aws-cloud-provider")
	c.eventBroadcaster = record.NewBroadcaster()
	c.eventBroadcaster.StartLogging(klog.Infof)
	c.eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: c.kubeClient.CoreV1().Events("")})
	c.eventRecorder = c.eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "aws-cloud-provider"})
}

// Clusters returns the list of clusters.
func (c *Cloud) Clusters() (cloudprovider.Clusters, bool) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("Clusters()")
	return nil, false
}

// ProviderName returns the cloud provider ID.
func (c *Cloud) ProviderName() string {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("ProviderName")
	return ProviderName
}

// LoadBalancer returns an implementation of LoadBalancer for Amazon Web Services.
func (c *Cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("LoadBalancer()")
	return c, true
}

// Instances returns an implementation of Instances for Amazon Web Services.
func (c *Cloud) Instances() (cloudprovider.Instances, bool) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("Instances()")
	return c, true
}

// InstancesV2 is an implementation for instances and should only be implemented by external cloud providers.
// Implementing InstancesV2 is behaviorally identical to Instances but is optimized to significantly reduce
// API calls to the cloud provider when registering and syncing nodes.
// Also returns true if the interface is supported, false otherwise.
func (c *Cloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return c.instances, true
}

// Zones returns an implementation of Zones for Amazon Web Services.
func (c *Cloud) Zones() (cloudprovider.Zones, bool) {
	debugPrintCallerFunctionName()
	return c, true
}

// Routes returns an implementation of Routes for Amazon Web Services.
func (c *Cloud) Routes() (cloudprovider.Routes, bool) {
	debugPrintCallerFunctionName()
	return c, false
}

// HasClusterID returns true if the cluster has a clusterID
func (c *Cloud) HasClusterID() bool {
	debugPrintCallerFunctionName()
	return len(c.tagging.clusterID()) > 0
}

// NodeAddresses is an implementation of Instances.NodeAddresses.
func (c *Cloud) NodeAddresses(ctx context.Context, name types.NodeName) ([]v1.NodeAddress, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("NodeAddresses(%v)", name)
	if c.selfAWSInstance.nodeName == name || len(name) == 0 {
		addresses := []v1.NodeAddress{}

		macs, err := c.metadata.GetMetadata("network/interfaces/macs/")
		if err != nil {
			return nil, fmt.Errorf("error querying AWS metadata for %q: %q", "network/interfaces/macs", err)
		}

		for _, macID := range strings.Split(macs, "\n") {
			if macID == "" {
				continue
			}
			macPath := path.Join("network/interfaces/macs/", macID, "local-ipv4s")
			internalIPs, err := c.metadata.GetMetadata(macPath)
			if err != nil {
				return nil, fmt.Errorf("error querying AWS metadata for %q: %q", macPath, err)
			}
			for _, internalIP := range strings.Split(internalIPs, "\n") {
				if internalIP == "" {
					continue
				}
				addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalIP, Address: internalIP})
			}
		}

		externalIP, err := c.metadata.GetMetadata("public-ipv4")
		if err != nil {
			//TODO: It would be nice to be able to determine the reason for the failure,
			// but the AWS client masks all failures with the same error description.
			klog.V(4).Info("Could not determine public IP from AWS metadata.")
		} else {
			addresses = append(addresses, v1.NodeAddress{Type: v1.NodeExternalIP, Address: externalIP})
		}

		localHostname, err := c.metadata.GetMetadata("local-hostname")
		if err != nil || len(localHostname) == 0 {
			//TODO: It would be nice to be able to determine the reason for the failure,
			// but the AWS client masks all failures with the same error description.
			klog.V(4).Info("Could not determine private DNS from AWS metadata.")
		} else {
			hostname, internalDNS := parseMetadataLocalHostname(localHostname)
			addresses = append(addresses, v1.NodeAddress{Type: v1.NodeHostName, Address: hostname})
			for _, d := range internalDNS {
				addresses = append(addresses, v1.NodeAddress{Type: v1.NodeInternalDNS, Address: d})
			}
		}

		externalDNS, err := c.metadata.GetMetadata("public-hostname")
		if err != nil || len(externalDNS) == 0 {
			//TODO: It would be nice to be able to determine the reason for the failure,
			// but the AWS client masks all failures with the same error description.
			klog.V(4).Info("Could not determine public DNS from AWS metadata.")
		} else {
			addresses = append(addresses, v1.NodeAddress{Type: v1.NodeExternalDNS, Address: externalDNS})
		}

		return addresses, nil
	}

	instance, err := c.getInstanceByNodeName(name)
	if err != nil {
		return nil, fmt.Errorf("getInstanceByNodeName failed for %q with %q", name, err)
	}
	return extractNodeAddresses(instance)
}

// NodeAddressesByProviderID returns the node addresses of an instances with the specified unique providerID
// This method will not be called from the node that is requesting this ID. i.e. metadata service
// and other local methods cannot be used here
func (c *Cloud) NodeAddressesByProviderID(ctx context.Context, providerID string) ([]v1.NodeAddress, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("NodeAddressesByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToAWSInstanceID()
	if err != nil {
		return nil, err
	}

	instance, err := describeInstance(c.compute, instanceID)
	if err != nil {
		return nil, err
	}

	return extractNodeAddresses(instance)
}

// InstanceExistsByProviderID returns true if the instance with the given provider id still exists.
// If false is returned with no error, the instance will be immediately deleted by the cloud controller manager.
func (c *Cloud) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("InstanceExistsByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToAWSInstanceID()
	if err != nil {
		return false, err
	}

	request := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			newEc2Filter("instance-id", string(instanceID)),
		},
	}

	instances, err := c.compute.ReadVms(request)
	if err != nil {
		return false, err
	}
	if len(instances) == 0 {
		return false, nil
	}
	if len(instances) > 1 {
		return false, fmt.Errorf("multiple instances found for instance: %s", instanceID)
	}

	state := instances[0].State.Name
	if *state == ec2.InstanceStateNameTerminated {
		klog.Warningf("the instance %s is terminated", instanceID)
		return false, nil
	}

	return true, nil
}

// InstanceShutdownByProviderID returns true if the instance is in safe state to detach volumes
func (c *Cloud) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("InstanceShutdownByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToAWSInstanceID()
	if err != nil {
		return false, err
	}

	request := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{instanceID.awsString()},
	}

	instances, err := c.compute.ReadVms(request)
	if err != nil {
		return false, err
	}
	if len(instances) == 0 {
		klog.Warningf("the instance %s does not exist anymore", providerID)
		// returns false, because otherwise node is not deleted from cluster
		// false means that it will continue to check InstanceExistsByProviderID
		return false, nil
	}
	if len(instances) > 1 {
		return false, fmt.Errorf("multiple instances found for instance: %s", instanceID)
	}

	instance := instances[0]
	if instance.State != nil {
		state := aws.StringValue(instance.State.Name)
		// valid state for detaching volumes
		if state == ec2.InstanceStateNameStopped {
			return true, nil
		}
	}
	return false, nil
}

// InstanceID returns the cloud provider ID of the node with the specified nodeName.
func (c *Cloud) InstanceID(ctx context.Context, nodeName types.NodeName) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("InstanceID(%v)", nodeName)
	// In the future it is possible to also return an endpoint as:
	// <endpoint>/<zone>/<instanceid>
	if c.selfAWSInstance.nodeName == nodeName {
		return "/" + c.selfAWSInstance.availabilityZone + "/" + c.selfAWSInstance.awsID, nil
	}
	inst, err := c.getInstanceByNodeName(nodeName)
	if err != nil {
		if err == cloudprovider.InstanceNotFound {
			// The Instances interface requires that we return InstanceNotFound (without wrapping)
			return "", err
		}
		return "", fmt.Errorf("getInstanceByNodeName failed for %q with %q", nodeName, err)
	}
	return "/" + aws.StringValue(inst.Placement.AvailabilityZone) + "/" + aws.StringValue(inst.InstanceId), nil
}

// InstanceTypeByProviderID returns the cloudprovider instance type of the node with the specified unique providerID
// This method will not be called from the node that is requesting this ID. i.e. metadata service
// and other local methods cannot be used here
func (c *Cloud) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("InstanceTypeByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToAWSInstanceID()
	if err != nil {
		return "", err
	}

	instance, err := describeInstance(c.compute, instanceID)
	if err != nil {
		return "", err
	}

	return aws.StringValue(instance.InstanceType), nil
}

// InstanceType returns the type of the node with the specified nodeName.
func (c *Cloud) InstanceType(ctx context.Context, nodeName types.NodeName) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("InstanceType(%v)", nodeName)
	if c.selfAWSInstance.nodeName == nodeName {
		return c.selfAWSInstance.instanceType, nil
	}
	inst, err := c.getInstanceByNodeName(nodeName)
	if err != nil {
		return "", fmt.Errorf("getInstanceByNodeName failed for %q with %q", nodeName, err)
	}
	return aws.StringValue(inst.InstanceType), nil
}

// GetZone implements Zones.GetZone
func (c *Cloud) GetZone(ctx context.Context) (cloudprovider.Zone, error) {
	debugPrintCallerFunctionName()
	return cloudprovider.Zone{
		FailureDomain: c.selfAWSInstance.availabilityZone,
		Region:        c.region,
	}, nil
}

// GetZoneByProviderID implements Zones.GetZoneByProviderID
// This is particularly useful in external cloud providers where the kubelet
// does not initialize node data.
func (c *Cloud) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("GetZoneByProviderID(%v)", providerID)
	instanceID, err := KubernetesInstanceID(providerID).MapToAWSInstanceID()
	if err != nil {
		return cloudprovider.Zone{}, err
	}
	instance, err := c.getInstanceByID(string(instanceID))
	if err != nil {
		return cloudprovider.Zone{}, err
	}

	zone := cloudprovider.Zone{
		FailureDomain: *(instance.Placement.AvailabilityZone),
		Region:        c.region,
	}

	return zone, nil
}

// GetZoneByNodeName implements Zones.GetZoneByNodeName
// This is particularly useful in external cloud providers where the kubelet
// does not initialize node data.
func (c *Cloud) GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("GetZoneByNodeName(%v)", nodeName)
	instance, err := c.getInstanceByNodeName(nodeName)
	if err != nil {
		return cloudprovider.Zone{}, err
	}
	zone := cloudprovider.Zone{
		FailureDomain: *(instance.Placement.AvailabilityZone),
		Region:        c.region,
	}

	return zone, nil

}

// Retrieves instance's vpc id from metadata
func (c *Cloud) findVPCID() (string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findVPCID()")
	macs, err := c.metadata.GetMetadata("network/interfaces/macs/")
	if err != nil {
		return "", fmt.Errorf("could not list interfaces of the instance: %q", err)
	}

	// loop over interfaces, first vpc id returned wins
	for _, macPath := range strings.Split(macs, "\n") {
		if len(macPath) == 0 {
			continue
		}
		url := fmt.Sprintf("network/interfaces/macs/%svpc-id", macPath)
		vpcID, err := c.metadata.GetMetadata(url)
		if err != nil {
			continue
		}
		return vpcID, nil
	}
	return "", fmt.Errorf("could not find VPC ID in instance metadata")
}

// ********************* CCM Cloud Resource LBU Functions  *********************

func (c *Cloud) addLoadBalancerTags(loadBalancerName string, requested map[string]string) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("addLoadBalancerTags(%v,%v)", loadBalancerName, requested)
	var tags []*elb.Tag
	for k, v := range requested {
		tag := &elb.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		tags = append(tags, tag)
	}

	request := &elb.AddTagsInput{}
	request.LoadBalancerNames = []*string{&loadBalancerName}
	request.Tags = tags

	_, err := c.elb.AddTags(request)
	if err != nil {
		return fmt.Errorf("error adding tags to load balancer: %v", err)
	}
	return nil
}

// Gets the current load balancer state
func (c *Cloud) describeLoadBalancer(name string) (*elb.LoadBalancerDescription, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("describeLoadBalancer(%v)", name)
	request := &elb.DescribeLoadBalancersInput{}
	request.LoadBalancerNames = []*string{&name}

	response, err := c.elb.DescribeLoadBalancers(request)
	if err != nil {
		if awsError, ok := err.(awserr.Error); ok {
			if awsError.Code() == "LoadBalancerNotFound" {
				return nil, nil
			}
		}
		return nil, err
	}

	var ret *elb.LoadBalancerDescription
	for _, loadBalancer := range response.LoadBalancerDescriptions {
		if ret != nil {
			klog.Errorf("Found multiple load balancers with name: %s", name)
		}
		ret = loadBalancer
	}
	return ret, nil
}

// Retrieves the specified security group from the AWS API, or returns nil if not found
func (c *Cloud) findSecurityGroup(securityGroupID string) (*ec2.SecurityGroup, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findSecurityGroup(%v)", securityGroupID)
	describeSecurityGroupsRequest := &ec2.DescribeSecurityGroupsInput{
		GroupIds: []*string{&securityGroupID},
	}
	// We don't apply our tag filters because we are retrieving by ID

	groups, err := c.compute.ReadSecurityGroups(describeSecurityGroupsRequest)
	if err != nil {
		klog.Warningf("Error retrieving security group: %q", err)
		return nil, err
	}

	if len(groups) == 0 {
		return nil, nil
	}
	if len(groups) != 1 {
		// This should not be possible - ids should be unique
		return nil, fmt.Errorf("multiple security groups found with same id %q", securityGroupID)
	}
	group := groups[0]
	return group, nil
}

// Makes sure the security group ingress is exactly the specified permissions
// Returns true if and only if changes were made
// The security group must already exist
func (c *Cloud) setSecurityGroupIngress(securityGroupID string, permissions IPPermissionSet) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("setSecurityGroupIngress(%v,%v)", securityGroupID, permissions)
	// We do not want to make changes to the Global defined SG
	if securityGroupID == c.cfg.Global.ElbSecurityGroup {
		return false, nil
	}

	group, err := c.findSecurityGroup(securityGroupID)
	if err != nil {
		klog.Warningf("Error retrieving security group %q", err)
		return false, err
	}

	if group == nil {
		return false, fmt.Errorf("security group not found: %s", securityGroupID)
	}

	klog.V(2).Infof("Existing security group ingress: %s %v", securityGroupID, group.IpPermissions)

	actual := NewIPPermissionSet(group.IpPermissions...)

	// EC2 groups rules together, for example combining:
	//
	// { Port=80, Range=[A] } and { Port=80, Range=[B] }
	//
	// into { Port=80, Range=[A,B] }
	//
	// We have to ungroup them, because otherwise the logic becomes really
	// complicated, and also because if we have Range=[A,B] and we try to
	// add Range=[A] then EC2 complains about a duplicate rule.
	permissions = permissions.Ungroup()
	actual = actual.Ungroup()

	remove := actual.Difference(permissions)
	add := permissions.Difference(actual)

	if add.Len() == 0 && remove.Len() == 0 {
		return false, nil
	}

	// TODO: There is a limit in VPC of 100 rules per security group, so we
	// probably should try grouping or combining to fit under this limit.
	// But this is only used on the ELB security group currently, so it
	// would require (ports * CIDRS) > 100.  Also, it isn't obvious exactly
	// how removing single permissions from compound rules works, and we
	// don't want to accidentally open more than intended while we're
	// applying changes.
	if add.Len() != 0 {
		klog.V(2).Infof("Adding security group ingress: %s %v", securityGroupID, add.List())

		request := &ec2.AuthorizeSecurityGroupIngressInput{}
		request.GroupId = &securityGroupID
		request.IpPermissions = add.List()
		_, err = c.compute.CreateSecurityGroupRule(request)
		if err != nil {
			return false, fmt.Errorf("error authorizing security group ingress: %q", err)
		}
	}
	if remove.Len() != 0 {
		klog.V(2).Infof("Remove security group ingress: %s %v", securityGroupID, remove.List())

		request := &ec2.RevokeSecurityGroupIngressInput{}
		request.GroupId = &securityGroupID
		request.IpPermissions = remove.List()
		_, err = c.compute.RevokeSecurityGroupIngress(request)
		if err != nil {
			return false, fmt.Errorf("error revoking security group ingress: %q", err)
		}
	}

	return true, nil
}

// Makes sure the security group includes the specified permissions
// Returns true if and only if changes were made
// The security group must already exist
func (c *Cloud) addSecurityGroupIngress(securityGroupID string, addPermissions []*ec2.IpPermission, isPublicCloud bool) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("addSecurityGroupIngress(%v,%v,%v)", securityGroupID, addPermissions, isPublicCloud)
	// We do not want to make changes to the Global defined SG
	if securityGroupID == c.cfg.Global.ElbSecurityGroup {
		return false, nil
	}

	group, err := c.findSecurityGroup(securityGroupID)
	if err != nil {
		klog.Warningf("Error retrieving security group: %q", err)
		return false, err
	}

	if group == nil {
		return false, fmt.Errorf("security group not found: %s", securityGroupID)
	}

	klog.Infof("Existing security group ingress: %s %v", securityGroupID, group.IpPermissions)

	changes := []*ec2.IpPermission{}
	for _, addPermission := range addPermissions {
		hasUserID := false
		for i := range addPermission.UserIdGroupPairs {
			if addPermission.UserIdGroupPairs[i].UserId != nil {
				hasUserID = true
			}
		}

		found := false
		for _, groupPermission := range group.IpPermissions {
			if ipPermissionExists(addPermission, groupPermission, hasUserID) {
				found = true
				break
			}
		}

		if !found {
			changes = append(changes, addPermission)
		}
	}

	if len(changes) == 0 && !isPublicCloud {
		return false, nil
	}

	klog.Infof("Adding security group ingress: %s %v isPublic %v)", securityGroupID, changes, isPublicCloud)

	request := &ec2.AuthorizeSecurityGroupIngressInput{}
	request.GroupId = &securityGroupID
	if !isPublicCloud {
		request.IpPermissions = changes
	} else {
		request.SourceSecurityGroupName = aws.String(DefaultSrcSgName)
		request.SourceSecurityGroupOwnerId = aws.String(DefaultSgOwnerID)
	}
	_, err = c.compute.CreateSecurityGroupRule(request)
	if err != nil {
		ignore := false
		if isPublicCloud {
			if awsError, ok := err.(awserr.Error); ok {
				if awsError.Code() == "InvalidPermission.Duplicate" {
					klog.V(2).Infof("Ignoring InvalidPermission.Duplicate for security group (%s), assuming is used by other public LB", securityGroupID)
					ignore = true
				}
			}
		}
		if !ignore {
			klog.Warningf("Error authorizing security group ingress %q", err)
			return false, fmt.Errorf("error authorizing security group ingress: %q", err)
		}
	}

	return true, nil
}

// Makes sure the security group no longer includes the specified permissions
// Returns true if and only if changes were made
// If the security group no longer exists, will return (false, nil)
func (c *Cloud) removeSecurityGroupIngress(securityGroupID string, removePermissions []*ec2.IpPermission, isPublicCloud bool) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("removeSecurityGroupIngress(%v,%v)", securityGroupID, removePermissions)
	// We do not want to make changes to the Global defined SG
	if securityGroupID == c.cfg.Global.ElbSecurityGroup {
		return false, nil
	}

	group, err := c.findSecurityGroup(securityGroupID)
	if err != nil {
		klog.Warningf("Error retrieving security group: %q", err)
		return false, err
	}

	if group == nil {
		klog.Warning("Security group not found: ", securityGroupID)
		return false, nil
	}

	changes := []*ec2.IpPermission{}
	for _, removePermission := range removePermissions {
		hasUserID := false
		for i := range removePermission.UserIdGroupPairs {
			if removePermission.UserIdGroupPairs[i].UserId != nil {
				hasUserID = true
			}
		}

		var found *ec2.IpPermission
		for _, groupPermission := range group.IpPermissions {
			if ipPermissionExists(removePermission, groupPermission, hasUserID) {
				found = removePermission
				break
			}
		}

		if found != nil {
			changes = append(changes, found)
		}
	}

	if len(changes) == 0 && !isPublicCloud {
		return false, nil
	}

	klog.Infof("Removing security group ingress: %s %v", securityGroupID, changes)

	request := &ec2.RevokeSecurityGroupIngressInput{}
	request.GroupId = &securityGroupID
	if !isPublicCloud {
		request.IpPermissions = changes
	} else {
		request.SourceSecurityGroupName = aws.String(DefaultSrcSgName)
		request.SourceSecurityGroupOwnerId = aws.String(DefaultSgOwnerID)
	}

	_, err = c.compute.RevokeSecurityGroupIngress(request)
	if err != nil {
		klog.Warningf("Error revoking security group ingress: %q", err)
		return false, err
	}

	return true, nil
}

// Makes sure the security group exists.
// For multi-cluster isolation, name must be globally unique, for example derived from the service UUID.
// Additional tags can be specified
// Returns the security group id or error
func (c *Cloud) ensureSecurityGroup(name string, description string, additionalTags map[string]string) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("ensureSecurityGroup (%v,%v,%v)", name, description, additionalTags)

	groupID := ""
	attempt := 0
	for {
		attempt++

		// Note that we do _not_ add our tag filters; group-name + vpc-id is the EC2 primary key.
		// However, we do check that it matches our tags.
		// If it doesn't have any tags, we tag it; this is how we recover if we failed to tag before.
		// If it has a different cluster's tags, that is an error.
		// This shouldn't happen because name is expected to be globally unique (UUID derived)
		request := &ec2.DescribeSecurityGroupsInput{}
		request.Filters = []*ec2.Filter{
			newEc2Filter("group-name", name),
		}

		if c.vpcID != "" {
			request.Filters = append(request.Filters, newEc2Filter("vpc-id", c.vpcID))
		}

		securityGroups, err := c.compute.ReadSecurityGroups(request)
		if err != nil {
			return "", err
		}

		if len(securityGroups) >= 1 {
			if len(securityGroups) > 1 {
				klog.Warningf("Found multiple security groups with name: %q", name)
			}
			err := c.tagging.readRepairClusterTags(
				c.compute, aws.StringValue(securityGroups[0].GroupId),
				ResourceLifecycleOwned, nil, securityGroups[0].Tags)
			if err != nil {
				return "", err
			}

			return aws.StringValue(securityGroups[0].GroupId), nil
		}

		createRequest := &ec2.CreateSecurityGroupInput{}
		if c.vpcID != "" {
			createRequest.VpcId = &c.vpcID
		}
		createRequest.GroupName = &name
		createRequest.Description = &description

		createResponse, err := c.compute.CreateSecurityGroup(createRequest)
		if err != nil {
			ignore := false
			switch err := err.(type) {
			case awserr.Error:
				if err.Code() == "InvalidGroup.Duplicate" && attempt < MaxReadThenCreateRetries {
					klog.V(2).Infof("Got InvalidGroup.Duplicate while creating security group (race?); will retry")
					ignore = true
				}
			}
			if !ignore {
				klog.Errorf("Error creating security group: %q", err)
				return "", err
			}
			time.Sleep(1 * time.Second)
		} else {
			groupID = aws.StringValue(createResponse.GroupId)
			break
		}
	}
	if groupID == "" {
		return "", fmt.Errorf("created security group, but id was not returned: %s", name)
	}

	err := c.tagging.createTags(c.compute, groupID, ResourceLifecycleOwned, additionalTags)
	if err != nil {
		// If we retry, ensureClusterTags will recover from this - it
		// will add the missing tags.  We could delete the security
		// group here, but that doesn't feel like the right thing, as
		// the caller is likely to retry the create
		return "", fmt.Errorf("error tagging security group: %q", err)
	}
	return groupID, nil
}

// Finds the subnets associated with the cluster, by matching tags.
// For maximal backwards compatibility, if no subnets are tagged, it will fall-back to the current subnet.
// However, in future this will likely be treated as an error.
func (c *Cloud) findSubnets() ([]*ec2.Subnet, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findSubnets()")
	request := &ec2.DescribeSubnetsInput{}
	var err error
	var subnets []*ec2.Subnet
	err = nil
	subnets = []*ec2.Subnet{}
	if c.vpcID != "" {
		request.Filters = []*ec2.Filter{newEc2Filter("vpc-id", c.vpcID)}

		subnets, err := c.compute.DescribeSubnets(request)
		if err != nil {
			return nil, fmt.Errorf("error describing subnets: %q", err)
		}

		var matches []*ec2.Subnet
		for _, subnet := range subnets {
			if c.tagging.hasClusterTag(subnet.Tags) {
				matches = append(matches, subnet)
			}
		}

		if len(matches) != 0 {
			return matches, nil
		}
	}

	if c.selfAWSInstance.subnetID != "" {
		// Fall back to the current instance subnets, if nothing is tagged
		klog.Warningf("No tagged subnets found; will fall-back to the current subnet only.  This is likely to be an error in a future version of k8s.")
		request = &ec2.DescribeSubnetsInput{}
		request.Filters = []*ec2.Filter{newEc2Filter("subnet-id", c.selfAWSInstance.subnetID)}

		subnets, err = c.compute.DescribeSubnets(request)
		if err != nil {
			return nil, fmt.Errorf("error describing subnets: %q", err)
		}
		return subnets, nil

	}

	return subnets, nil

}

// Finds the subnets to use for an ELB we are creating.
// Normal (Internet-facing) ELBs must use public subnets, so we skip private subnets.
// Internal ELBs can use public or private subnets, but if we have a private subnet we should prefer that.
func (c *Cloud) findELBSubnets(internalELB bool) ([]string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findELBSubnets(%v)", internalELB)

	subnets, err := c.findSubnets()
	if err != nil {
		return nil, err
	}
	var rt []*ec2.RouteTable
	if c.vpcID != "" {
		vpcIDFilter := newEc2Filter("vpc-id", c.vpcID)
		rRequest := &ec2.DescribeRouteTablesInput{}
		rRequest.Filters = []*ec2.Filter{vpcIDFilter}
		rt, err = c.compute.DescribeRouteTables(rRequest)
		if err != nil {
			return nil, fmt.Errorf("error describe route table: %q", err)
		}
	}

	// Try to break the tie using a tag
	var tagName string
	if internalELB {
		tagName = TagNameSubnetInternalELB
	} else {
		tagName = TagNameSubnetPublicELB
	}

	subnetsByAZ := make(map[string]*ec2.Subnet)
	for _, subnet := range subnets {
		az := aws.StringValue(subnet.AvailabilityZone)
		id := aws.StringValue(subnet.SubnetId)
		if az == "" || id == "" {
			klog.Warningf("Ignoring subnet with empty az/id: %v", subnet)
			continue
		}

		isPublic, err := isSubnetPublic(rt, id)
		if err != nil {
			return nil, err
		}
		if !internalELB && !isPublic {
			klog.V(2).Infof("Ignoring private subnet for public ELB %q", id)
			continue
		}

		existing := subnetsByAZ[az]
		_, subnetHasTag := findTag(subnet.Tags, tagName)
		if existing == nil {
			if subnetHasTag {
				subnetsByAZ[az] = subnet
			} else if isPublic && !internalELB {
				subnetsByAZ[az] = subnet
			}
			continue
		}

		_, existingHasTag := findTag(existing.Tags, tagName)

		if existingHasTag != subnetHasTag {
			if subnetHasTag {
				subnetsByAZ[az] = subnet
			}
			continue
		}

		// If we have two subnets for the same AZ we arbitrarily choose the one that is first lexicographically.
		// TODO: Should this be an error.
		if strings.Compare(*existing.SubnetId, *subnet.SubnetId) > 0 {
			klog.Warningf("Found multiple subnets in AZ %q; choosing %q between subnets %q and %q", az, *subnet.SubnetId, *existing.SubnetId, *subnet.SubnetId)
			subnetsByAZ[az] = subnet
			continue
		}

		klog.Warningf("Found multiple subnets in AZ %q; choosing %q between subnets %q and %q", az, *existing.SubnetId, *existing.SubnetId, *subnet.SubnetId)
		continue
	}

	var azNames []string
	for key := range subnetsByAZ {
		azNames = append(azNames, key)
	}

	sort.Strings(azNames)

	var subnetIDs []string
	for _, key := range azNames {
		subnetIDs = append(subnetIDs, aws.StringValue(subnetsByAZ[key].SubnetId))
	}

	return subnetIDs, nil
}

// buildELBSecurityGroupList returns list of SecurityGroups which should be
// attached to ELB created by a service. List always consist of at least
// 1 member which is an SG created for this service or a SG from the Global config.
// Extra groups can be specified via annotation, as can extra tags for any
// new groups. The annotation "ServiceAnnotationLoadBalancerSecurityGroups" allows for
// setting the security groups specified.
func (c *Cloud) buildELBSecurityGroupList(serviceName types.NamespacedName, loadBalancerName string, annotations map[string]string) ([]string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("buildELBSecurityGroupList(%v,%v,%v)", serviceName, loadBalancerName, annotations)
	var err error
	var securityGroupID string

	if c.cfg.Global.ElbSecurityGroup != "" {
		securityGroupID = c.cfg.Global.ElbSecurityGroup
	} else {
		// Create a security group for the load balancer
		sgName := "k8s-elb-" + loadBalancerName
		sgDescription := fmt.Sprintf("Security group for Kubernetes ELB %s (%v)", loadBalancerName, serviceName)
		securityGroupID, err = c.ensureSecurityGroup(sgName, sgDescription, getLoadBalancerAdditionalTags(annotations))
		if err != nil {
			klog.Errorf("Error creating load balancer security group: %q", err)
			return nil, err
		}
	}

	sgList := []string{}

	for _, extraSG := range strings.Split(annotations[ServiceAnnotationLoadBalancerSecurityGroups], ",") {
		extraSG = strings.TrimSpace(extraSG)
		if len(extraSG) > 0 {
			sgList = append(sgList, extraSG)
		}
	}

	// If no Security Groups have been specified with the ServiceAnnotationLoadBalancerSecurityGroups annotation, we add the default one.
	if len(sgList) == 0 {
		sgList = append(sgList, securityGroupID)
	}

	for _, extraSG := range strings.Split(annotations[ServiceAnnotationLoadBalancerExtraSecurityGroups], ",") {
		extraSG = strings.TrimSpace(extraSG)
		if len(extraSG) > 0 {
			sgList = append(sgList, extraSG)
		}
	}

	return sgList, nil
}

// EnsureLoadBalancer implements LoadBalancer.EnsureLoadBalancer
func (c *Cloud) EnsureLoadBalancer(ctx context.Context, clusterName string, apiService *v1.Service,
	nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("EnsureLoadBalancer(%v, %v, %v)", clusterName, apiService, nodes)
	klog.V(10).Infof("EnsureLoadBalancer.annotations(%v)", apiService.Annotations)
	annotations := apiService.Annotations
	if apiService.Spec.SessionAffinity != v1.ServiceAffinityNone {
		// ELB supports sticky sessions, but only when configured for HTTP/HTTPS
		return nil, fmt.Errorf("unsupported load balancer affinity: %v", apiService.Spec.SessionAffinity)
	}

	if len(apiService.Spec.Ports) == 0 {
		return nil, fmt.Errorf("requested load balancer with no ports")
	}

	// Figure out what mappings we want on the load balancer
	listeners := []*elb.Listener{}

	sslPorts := getPortSets(annotations[ServiceAnnotationLoadBalancerSSLPorts])

	for _, port := range apiService.Spec.Ports {
		if port.Protocol != v1.ProtocolTCP {
			return nil, fmt.Errorf("Only TCP LoadBalancer is supported for AWS ELB")
		}
		if port.NodePort == 0 {
			klog.Errorf("Ignoring port without NodePort defined: %v", port)
			continue
		}

		listener, err := buildListener(port, annotations, sslPorts)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, listener)
	}

	if apiService.Spec.LoadBalancerIP != "" {
		return nil, fmt.Errorf("LoadBalancerIP cannot be specified for AWS ELB")
	}

	instances, err := c.findInstancesForELB(nodes)
	klog.V(10).Infof("Debug OSC: c.findInstancesForELB(nodes) : %v", instances)
	if err != nil {
		return nil, err
	}

	sourceRanges, err := servicehelpers.GetLoadBalancerSourceRanges(apiService)
	klog.V(10).Infof("Debug OSC:  servicehelpers.GetLoadBalancerSourceRanges : %v", sourceRanges)
	if err != nil {
		return nil, err
	}

	// Determine if this is tagged as an Internal ELB
	internalELB := false
	internalAnnotation := apiService.Annotations[ServiceAnnotationLoadBalancerInternal]
	if internalAnnotation == "false" {
		internalELB = false
	} else if internalAnnotation != "" {
		internalELB = true
	}
	klog.V(10).Infof("Debug OSC:  internalELB : %v", internalELB)

	// Determine if we need to set the Proxy protocol policy
	proxyProtocol := false
	proxyProtocolAnnotation := apiService.Annotations[ServiceAnnotationLoadBalancerProxyProtocol]
	if proxyProtocolAnnotation != "" {
		if proxyProtocolAnnotation != "*" {
			return nil, fmt.Errorf("annotation %q=%q detected, but the only value supported currently is '*'", ServiceAnnotationLoadBalancerProxyProtocol, proxyProtocolAnnotation)
		}
		proxyProtocol = true
	}

	// Some load balancer attributes are required, so defaults are set. These can be overridden by annotations.
	loadBalancerAttributes := &elb.LoadBalancerAttributes{
		ConnectionDraining: &elb.ConnectionDraining{Enabled: aws.Bool(false)},
		ConnectionSettings: &elb.ConnectionSettings{IdleTimeout: aws.Int64(60)},
	}

	if annotations[ServiceAnnotationLoadBalancerAccessLogS3BucketName] != "" &&
		annotations[ServiceAnnotationLoadBalancerAccessLogS3BucketPrefix] != "" {

		loadBalancerAttributes.AccessLog = &elb.AccessLog{Enabled: aws.Bool(false)}

		// Determine if access log enabled/disabled has been specified
		accessLogEnabledAnnotation := annotations[ServiceAnnotationLoadBalancerAccessLogEnabled]
		if accessLogEnabledAnnotation != "" {
			accessLogEnabled, err := strconv.ParseBool(accessLogEnabledAnnotation)
			if err != nil {
				return nil, fmt.Errorf("error parsing service annotation: %s=%s",
					ServiceAnnotationLoadBalancerAccessLogEnabled,
					accessLogEnabledAnnotation,
				)
			}
			loadBalancerAttributes.AccessLog.Enabled = &accessLogEnabled
		}
		// Determine if an access log emit interval has been specified
		accessLogEmitIntervalAnnotation := annotations[ServiceAnnotationLoadBalancerAccessLogEmitInterval]
		if accessLogEmitIntervalAnnotation != "" {
			accessLogEmitInterval, err := strconv.ParseInt(accessLogEmitIntervalAnnotation, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("error parsing service annotation: %s=%s",
					ServiceAnnotationLoadBalancerAccessLogEmitInterval,
					accessLogEmitIntervalAnnotation,
				)
			}
			loadBalancerAttributes.AccessLog.EmitInterval = &accessLogEmitInterval
		}

		// Determine if access log s3 bucket name has been specified
		accessLogS3BucketNameAnnotation := annotations[ServiceAnnotationLoadBalancerAccessLogS3BucketName]
		if accessLogS3BucketNameAnnotation != "" {
			loadBalancerAttributes.AccessLog.S3BucketName = &accessLogS3BucketNameAnnotation
		}

		// Determine if access log s3 bucket prefix has been specified
		accessLogS3BucketPrefixAnnotation := annotations[ServiceAnnotationLoadBalancerAccessLogS3BucketPrefix]
		if accessLogS3BucketPrefixAnnotation != "" {
			loadBalancerAttributes.AccessLog.S3BucketPrefix = &accessLogS3BucketPrefixAnnotation
		}
		klog.V(10).Infof("Debug OSC:  loadBalancerAttributes.AccessLog : %v", loadBalancerAttributes.AccessLog)
	}

	// Determine if connection draining enabled/disabled has been specified
	connectionDrainingEnabledAnnotation := annotations[ServiceAnnotationLoadBalancerConnectionDrainingEnabled]
	if connectionDrainingEnabledAnnotation != "" {
		connectionDrainingEnabled, err := strconv.ParseBool(connectionDrainingEnabledAnnotation)
		if err != nil {
			return nil, fmt.Errorf("error parsing service annotation: %s=%s",
				ServiceAnnotationLoadBalancerConnectionDrainingEnabled,
				connectionDrainingEnabledAnnotation,
			)
		}
		loadBalancerAttributes.ConnectionDraining.Enabled = &connectionDrainingEnabled
	}

	// Determine if connection draining timeout has been specified
	connectionDrainingTimeoutAnnotation := annotations[ServiceAnnotationLoadBalancerConnectionDrainingTimeout]
	if connectionDrainingTimeoutAnnotation != "" {
		connectionDrainingTimeout, err := strconv.ParseInt(connectionDrainingTimeoutAnnotation, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing service annotation: %s=%s",
				ServiceAnnotationLoadBalancerConnectionDrainingTimeout,
				connectionDrainingTimeoutAnnotation,
			)
		}
		loadBalancerAttributes.ConnectionDraining.Timeout = &connectionDrainingTimeout
	}

	// Determine if connection idle timeout has been specified
	connectionIdleTimeoutAnnotation := annotations[ServiceAnnotationLoadBalancerConnectionIdleTimeout]
	if connectionIdleTimeoutAnnotation != "" {
		connectionIdleTimeout, err := strconv.ParseInt(connectionIdleTimeoutAnnotation, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing service annotation: %s=%s",
				ServiceAnnotationLoadBalancerConnectionIdleTimeout,
				connectionIdleTimeoutAnnotation,
			)
		}
		loadBalancerAttributes.ConnectionSettings.IdleTimeout = &connectionIdleTimeout
	}

	// Find the subnets that the ELB will live in
	subnetIDs, err := c.findELBSubnets(internalELB)
	klog.V(2).Infof("Debug OSC:  c.findELBSubnets(internalELB) : %v", subnetIDs)

	if err != nil {
		klog.Errorf("Error listing subnets in VPC: %q", err)
		return nil, err
	}

	// Bail out early if there are no subnets
	if len(subnetIDs) == 0 {
		klog.Warningf("could not find any suitable subnets for creating the ELB")
	}

	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, apiService)
	serviceName := types.NamespacedName{Namespace: apiService.Namespace, Name: apiService.Name}

	klog.V(10).Infof("Debug OSC:  loadBalancerName : %v", loadBalancerName)
	klog.V(10).Infof("Debug OSC:  serviceName : %v", serviceName)
	klog.V(10).Infof("Debug OSC:  serviceName : %v", annotations)

	var securityGroupIDs []string

	if len(subnetIDs) == 0 || c.vpcID == "" {
		securityGroupIDs = []string{DefaultSrcSgName}
	} else {
		securityGroupIDs, err = c.buildELBSecurityGroupList(serviceName, loadBalancerName, annotations)
	}

	klog.V(10).Infof("Debug OSC:  ensured securityGroupIDs : %v", securityGroupIDs)

	if err != nil {
		return nil, err
	}
	if len(securityGroupIDs) == 0 {
		return nil, fmt.Errorf("[BUG] ELB can't have empty list of Security Groups to be assigned, this is a Kubernetes bug, please report")
	}

	if len(subnetIDs) > 0 && c.vpcID != "" {
		ec2SourceRanges := []*ec2.IpRange{}
		for _, sourceRange := range sourceRanges.StringSlice() {
			ec2SourceRanges = append(ec2SourceRanges, &ec2.IpRange{CidrIp: aws.String(sourceRange)})
		}

		permissions := NewIPPermissionSet()
		for _, port := range apiService.Spec.Ports {

			portInt64 := int64(port.Port)
			protocol := strings.ToLower(string(port.Protocol))

			permission := &ec2.IpPermission{}
			permission.FromPort = &portInt64
			permission.ToPort = &portInt64
			permission.IpRanges = ec2SourceRanges
			permission.IpProtocol = &protocol

			permissions.Insert(permission)
		}

		// Allow ICMP fragmentation packets, important for MTU discovery
		{
			permission := &ec2.IpPermission{
				IpProtocol: aws.String("icmp"),
				FromPort:   aws.Int64(3),
				ToPort:     aws.Int64(4),
				IpRanges:   ec2SourceRanges,
			}

			permissions.Insert(permission)
		}
		_, err = c.setSecurityGroupIngress(securityGroupIDs[0], permissions)
		if err != nil {
			return nil, err
		}
	}

	// Build the load balancer itself
	loadBalancer, err := c.ensureLoadBalancer(
		serviceName,
		loadBalancerName,
		listeners,
		subnetIDs,
		securityGroupIDs,
		internalELB,
		proxyProtocol,
		loadBalancerAttributes,
		annotations,
	)
	if err != nil {
		return nil, err
	}

	if sslPolicyName, ok := annotations[ServiceAnnotationLoadBalancerSSLNegotiationPolicy]; ok {
		err := c.ensureSSLNegotiationPolicy(loadBalancer, sslPolicyName)
		if err != nil {
			return nil, err
		}

		for _, port := range c.getLoadBalancerTLSPorts(loadBalancer) {
			err := c.setSSLNegotiationPolicy(loadBalancerName, sslPolicyName, port)
			if err != nil {
				return nil, err
			}
		}
	}

	if path, healthCheckNodePort := servicehelpers.GetServiceHealthCheckPathPort(apiService); path != "" {
		klog.V(4).Infof("service %v (%v) needs health checks on :%d%s)", apiService.Name, loadBalancerName, healthCheckNodePort, path)
		err = c.ensureLoadBalancerHealthCheck(loadBalancer, "HTTP", healthCheckNodePort, path, annotations)
		if err != nil {
			return nil, fmt.Errorf("Failed to ensure health check for localized service %v on node port %v: %q", loadBalancerName, healthCheckNodePort, err)
		}
	} else {
		klog.V(4).Infof("service %v does not need custom health checks", apiService.Name)
		// We only configure a TCP health-check on the first port
		var tcpHealthCheckPort int32
		for _, listener := range listeners {
			if listener.InstancePort == nil {
				continue
			}
			tcpHealthCheckPort = int32(*listener.InstancePort)
			break
		}
		annotationProtocol := strings.ToLower(annotations[ServiceAnnotationLoadBalancerBEProtocol])
		var hcProtocol string
		if annotationProtocol == "https" || annotationProtocol == "ssl" {
			hcProtocol = "SSL"
		} else {
			hcProtocol = "TCP"
		}
		// there must be no path on TCP health check
		err = c.ensureLoadBalancerHealthCheck(loadBalancer, hcProtocol, tcpHealthCheckPort, "", annotations)
		if err != nil {
			return nil, err
		}
	}

	err = c.updateInstanceSecurityGroupsForLoadBalancer(loadBalancer, instances, securityGroupIDs)
	if err != nil {
		klog.Warningf("Error opening ingress rules for the load balancer to the instances: %q", err)
		return nil, err
	}

	err = c.ensureLoadBalancerInstances(aws.StringValue(loadBalancer.LoadBalancerName), loadBalancer.Instances, instances)
	if err != nil {
		klog.Warningf("Error registering instances with the load balancer: %q", err)
		return nil, err
	}

	klog.V(1).Infof("Loadbalancer %s (%v) has DNS name %s", loadBalancerName, serviceName, aws.StringValue(loadBalancer.DNSName))

	// TODO: Wait for creation?

	status := toStatus(loadBalancer)
	return status, nil
}

// GetLoadBalancer is an implementation of LoadBalancer.GetLoadBalancer
func (c *Cloud) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*v1.LoadBalancerStatus, bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("GetLoadBalancer(%v,%v)", clusterName, service)
	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, service)

	lb, err := c.describeLoadBalancer(loadBalancerName)
	if err != nil {
		return nil, false, err
	}

	if lb == nil {
		return nil, false, nil
	}

	status := toStatus(lb)
	return status, true, nil
}

// GetLoadBalancerName is an implementation of LoadBalancer.GetLoadBalancerName
func (c *Cloud) GetLoadBalancerName(ctx context.Context, clusterName string, service *v1.Service) string {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("GetLoadBalancerName(%v,%v)", clusterName, service)

	//The unique name of the load balancer (32 alphanumeric or hyphen characters maximum, but cannot start or end with a hyphen).
	ret := strings.Replace(string(service.UID), "-", "", -1)

	if s, ok := service.Annotations[ServiceAnnotationLoadBalancerName]; ok {
		re := regexp.MustCompile("^[a-zA-Z0-9-]+$")
		fmt.Println("e.MatchString(s): ", s, re.MatchString(s))
		if len(s) <= 0 || !re.MatchString(s) {
			klog.Warningf("Ignoring %v annotation, empty string or does not respect lb name constraints: %v", ServiceAnnotationLoadBalancerName, s)
		} else {
			ret = s
		}
	}

	nameLength := LbNameMaxLength
	if s, ok := service.Annotations[ServiceAnnotationLoadBalancerNameLength]; ok {
		var err error
		nameLength, err = strconv.ParseInt(s, 10, 0)
		if err != nil || nameLength > LbNameMaxLength {
			klog.Warningf("Ignoring %v annotation, failed parsing %v value %v or value greater than %v ", ServiceAnnotationLoadBalancerNameLength, s, err, LbNameMaxLength)
			nameLength = LbNameMaxLength
		}
	}
	if int64(len(ret)) > nameLength {
		ret = ret[:nameLength]
	}
	return strings.Trim(ret, "-")
}

// Return all the security groups that are tagged as being part of our cluster
func (c *Cloud) getTaggedSecurityGroups() (map[string]*ec2.SecurityGroup, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getTaggedSecurityGroups()")
	request := &ec2.DescribeSecurityGroupsInput{}
	request.Filters = []*ec2.Filter{
		newEc2Filter("tag:"+c.tagging.clusterTagKey(),
			[]string{ResourceLifecycleOwned, ResourceLifecycleShared}...),
		newEc2Filter("tag:"+TagNameMainSG+c.tagging.clusterID(), "True"),
	}

	groups, err := c.compute.ReadSecurityGroups(request)
	if err != nil {
		return nil, fmt.Errorf("error querying security groups: %q", err)
	}

	m := make(map[string]*ec2.SecurityGroup)
	for _, group := range groups {
		if !c.tagging.hasClusterTag(group.Tags) {
			continue
		}

		id := aws.StringValue(group.GroupId)
		if id == "" {
			klog.Warningf("Ignoring group without id: %v", group)
			continue
		}
		m[id] = group
	}
	return m, nil
}

// Open security group ingress rules on the instances so that the load balancer can talk to them
// Will also remove any security groups ingress rules for the load balancer that are _not_ needed for allInstances
func (c *Cloud) updateInstanceSecurityGroupsForLoadBalancer(lb *elb.LoadBalancerDescription,
	instances map[InstanceID]*ec2.Instance,
	securityGroupIDs []string) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("updateInstanceSecurityGroupsForLoadBalancer(%v, %v, %v)", lb, instances, securityGroupIDs)

	if c.cfg.Global.DisableSecurityGroupIngress {
		return nil
	}

	// Determine the load balancer security group id
	loadBalancerSecurityGroupID := ""
	securityGroupsItem := []string{}
	if len(lb.SecurityGroups) > 0 {
		for _, securityGroup := range lb.SecurityGroups {
			securityGroupsItem = append(securityGroupsItem, *securityGroup)
		}
	} else if len(securityGroupIDs) > 0 {
		securityGroupsItem = securityGroupIDs
	}

	for _, securityGroup := range securityGroupsItem {
		if securityGroup == "" {
			continue
		}
		if loadBalancerSecurityGroupID != "" {
			// We create LBs with one SG
			klog.Warningf("Multiple security groups for load balancer: %q", aws.StringValue(lb.LoadBalancerName))
		}
		loadBalancerSecurityGroupID = securityGroup
	}

	if loadBalancerSecurityGroupID == "" {
		return fmt.Errorf("could not determine security group for load balancer: %s", aws.StringValue(lb.LoadBalancerName))
	}

	klog.V(10).Infof("loadBalancerSecurityGroupID(%v)", loadBalancerSecurityGroupID)

	// Get the actual list of groups that allow ingress from the load-balancer
	var actualGroups []*ec2.SecurityGroup
	{
		describeRequest := &ec2.DescribeSecurityGroupsInput{}
		if loadBalancerSecurityGroupID != DefaultSrcSgName {
			describeRequest.Filters = []*ec2.Filter{
				newEc2Filter("ip-permission.group-id", loadBalancerSecurityGroupID),
			}
		} else {
			describeRequest.Filters = []*ec2.Filter{
				newEc2Filter("ip-permission.group-name", loadBalancerSecurityGroupID),
			}
		}
		response, err := c.compute.ReadSecurityGroups(describeRequest)
		if err != nil {
			return fmt.Errorf("error querying security groups for ELB: %q", err)
		}
		for _, sg := range response {
			if !c.tagging.hasClusterTag(sg.Tags) {
				continue
			}
			actualGroups = append(actualGroups, sg)
		}
	}

	klog.V(10).Infof("actualGroups(%v)", actualGroups)

	taggedSecurityGroups, err := c.getTaggedSecurityGroups()
	if err != nil {
		return fmt.Errorf("error querying for tagged security groups: %q", err)
	}
	klog.V(10).Infof("taggedSecurityGroups(%v)", taggedSecurityGroups)

	// Open the firewall from the load balancer to the instance
	// We don't actually have a trivial way to know in advance which security group the instance is in
	// (it is probably the node security group, but we don't easily have that).
	// However, we _do_ have the list of security groups on the instance records.

	// Map containing the changes we want to make; true to add, false to remove
	instanceSecurityGroupIds := map[string]bool{}

	// Scan instances for groups we want open
	for _, instance := range instances {
		securityGroup, err := findSecurityGroupForInstance(instance, taggedSecurityGroups)
		if err != nil {
			return err
		}

		if securityGroup == nil {
			klog.Warning("Ignoring instance without security group: ", aws.StringValue(instance.InstanceId))
			continue
		}
		id := aws.StringValue(securityGroup.GroupId)
		if id == "" {
			klog.Warningf("found security group without id: %v", securityGroup)
			continue
		}

		instanceSecurityGroupIds[id] = true
	}

	klog.V(10).Infof("instanceSecurityGroupIds(%v)", instanceSecurityGroupIds)

	// Compare to actual groups
	for _, actualGroup := range actualGroups {
		actualGroupID := aws.StringValue(actualGroup.GroupId)
		if actualGroupID == "" {
			klog.Warning("Ignoring group without ID: ", actualGroup)
			continue
		}

		adding, found := instanceSecurityGroupIds[actualGroupID]
		if found && adding {
			// We don't need to make a change; the permission is already in place
			delete(instanceSecurityGroupIds, actualGroupID)
		} else {
			// This group is not needed by allInstances; delete it
			instanceSecurityGroupIds[actualGroupID] = false
		}
	}

	klog.V(10).Infof("instanceSecurityGroupIds(%v)", instanceSecurityGroupIds)
	for instanceSecurityGroupID, add := range instanceSecurityGroupIds {
		if add {
			klog.V(2).Infof("Adding rule for traffic from the load balancer (%s) to instances (%s)", loadBalancerSecurityGroupID, instanceSecurityGroupID)
		} else {
			klog.V(2).Infof("Removing rule for traffic from the load balancer (%s) to instance (%s)", loadBalancerSecurityGroupID, instanceSecurityGroupID)
		}
		isPublicCloud := (loadBalancerSecurityGroupID == DefaultSrcSgName)
		permissions := []*ec2.IpPermission{}
		if !isPublicCloud {
			// This setting is applied when we are in a vpc
			sourceGroupID := &ec2.UserIdGroupPair{}
			sourceGroupID.GroupId = &loadBalancerSecurityGroupID

			allProtocols := "-1"

			permission := &ec2.IpPermission{}
			permission.IpProtocol = &allProtocols
			permission.UserIdGroupPairs = []*ec2.UserIdGroupPair{sourceGroupID}
			permissions = []*ec2.IpPermission{permission}
		}

		if add {
			changed, err := c.addSecurityGroupIngress(instanceSecurityGroupID, permissions, isPublicCloud)
			if err != nil {
				return err
			}
			if !changed {
				klog.Warning("Allowing ingress was not needed; concurrent change? groupId=", instanceSecurityGroupID)
			}
		} else {
			changed, err := c.removeSecurityGroupIngress(instanceSecurityGroupID, permissions, isPublicCloud)
			if err != nil {
				return err
			}
			if !changed {
				klog.Warning("Revoking ingress was not needed; concurrent change? groupId=", instanceSecurityGroupID)
			}
		}
	}

	return nil
}

// EnsureLoadBalancerDeleted implements LoadBalancer.EnsureLoadBalancerDeleted.
func (c *Cloud) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("EnsureLoadBalancerDeleted(%v, %v)", clusterName, service)
	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, service)

	lb, err := c.describeLoadBalancer(loadBalancerName)
	if err != nil {
		return err
	}

	if lb == nil {
		klog.Info("Load balancer already deleted: ", loadBalancerName)
		return nil
	}

	loadBalancerSGs := []string{}
	if len(lb.SecurityGroups) == 0 && c.vpcID == "" {
		loadBalancerSGs = append(loadBalancerSGs, DefaultSrcSgName)
	} else {
		loadBalancerSGs = aws.StringValueSlice(lb.SecurityGroups)
	}

	{
		// De-register the load balancer security group from the instances security group
		err = c.ensureLoadBalancerInstances(aws.StringValue(lb.LoadBalancerName),
			lb.Instances,
			map[InstanceID]*ec2.Instance{})
		if err != nil {
			klog.Errorf("ensureLoadBalancerInstances deregistering load balancer %v,%v,%v : %q",
				aws.StringValue(lb.LoadBalancerName),
				lb.Instances,
				nil, err)
		}

		// De-authorize the load balancer security group from the instances security group
		err = c.updateInstanceSecurityGroupsForLoadBalancer(lb, nil, loadBalancerSGs)
		if err != nil {
			klog.Errorf("Error deregistering load balancer from instance security groups: %q", err)
			return err
		}
	}

	{
		// Delete the load balancer itself
		request := &elb.DeleteLoadBalancerInput{}
		request.LoadBalancerName = lb.LoadBalancerName

		_, err = c.elb.DeleteLoadBalancer(request)
		if err != nil {
			// TODO: Check if error was because load balancer was concurrently deleted
			klog.Errorf("Error deleting load balancer: %q", err)
			return err
		}
	}

	{
		// Delete the security group(s) for the load balancer
		// Note that this is annoying: the load balancer disappears from the API immediately, but it is still
		// deleting in the background.  We get a DependencyViolation until the load balancer has deleted itself

		describeRequest := &ec2.DescribeSecurityGroupsInput{}
		describeRequest.Filters = []*ec2.Filter{
			newEc2Filter("group-id", loadBalancerSGs...),
		}
		response, err := c.compute.ReadSecurityGroups(describeRequest)
		if err != nil {
			return fmt.Errorf("error querying security groups for ELB: %q", err)
		}

		// Collect the security groups to delete
		securityGroupIDs := map[string]struct{}{}

		for _, sg := range response {
			sgID := aws.StringValue(sg.GroupId)

			if sgID == c.cfg.Global.ElbSecurityGroup {
				//We don't want to delete a security group that was defined in the Cloud Configuration.
				continue
			}
			if sgID == "" {
				klog.Warningf("Ignoring empty security group in %s", service.Name)
				continue
			}

			if !c.tagging.hasClusterTag(sg.Tags) {
				klog.Warningf("Ignoring security group with no cluster tag in %s", service.Name)
				continue
			}

			securityGroupIDs[sgID] = struct{}{}
		}

		// Loop through and try to delete them
		timeoutAt := time.Now().Add(time.Second * 600)
		for {
			for securityGroupID := range securityGroupIDs {
				request := &ec2.DeleteSecurityGroupInput{}
				request.GroupId = &securityGroupID
				_, err := c.compute.DeleteSecurityGroup(request)
				if err == nil {
					delete(securityGroupIDs, securityGroupID)
				} else {
					ignore := false
					if awsError, ok := err.(awserr.Error); ok {
						if awsError.Code() == "DependencyViolation" || awsError.Code() == "InvalidGroup.InUse" {
							klog.V(2).Infof("Ignoring DependencyViolation or  InvalidGroup.InUse while deleting load-balancer security group (%s), assuming because LB is in process of deleting", securityGroupID)
							ignore = true
						}
					}
					if !ignore {
						return fmt.Errorf("error while deleting load balancer security group (%s): %q", securityGroupID, err)
					}
				}
			}

			if len(securityGroupIDs) == 0 {
				klog.V(2).Info("Deleted all security groups for load balancer: ", service.Name)
				break
			}

			if time.Now().After(timeoutAt) {
				ids := []string{}
				for id := range securityGroupIDs {
					ids = append(ids, id)
				}

				return fmt.Errorf("timed out deleting ELB: %s. Could not delete security groups %v", service.Name, strings.Join(ids, ","))
			}

			klog.V(2).Info("Waiting for load-balancer to delete so we can delete security groups: ", service.Name)

			time.Sleep(10 * time.Second)
		}
	}

	return nil
}

// UpdateLoadBalancer implements LoadBalancer.UpdateLoadBalancer
func (c *Cloud) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("UpdateLoadBalancer(%v, %v, %s)", clusterName, service, nodes)
	instances, err := c.findInstancesForELB(nodes)
	if err != nil {
		return err
	}

	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, service)
	lb, err := c.describeLoadBalancer(loadBalancerName)
	if err != nil {
		return err
	}

	if lb == nil {
		return fmt.Errorf("Load balancer not found")
	}

	if sslPolicyName, ok := service.Annotations[ServiceAnnotationLoadBalancerSSLNegotiationPolicy]; ok {
		err := c.ensureSSLNegotiationPolicy(lb, sslPolicyName)
		if err != nil {
			return err
		}
		for _, port := range c.getLoadBalancerTLSPorts(lb) {
			err := c.setSSLNegotiationPolicy(loadBalancerName, sslPolicyName, port)
			if err != nil {
				return err
			}
		}
	}

	err = c.ensureLoadBalancerInstances(aws.StringValue(lb.LoadBalancerName), lb.Instances, instances)
	if err != nil {
		return nil
	}

	securityGroupsItem := []string{}
	if len(lb.SecurityGroups) == 0 && c.vpcID == "" {
		securityGroupsItem = append(securityGroupsItem, DefaultSrcSgName)
	}

	err = c.updateInstanceSecurityGroupsForLoadBalancer(lb, instances, securityGroupsItem)
	if err != nil {
		return err
	}

	return nil
}

// ********************* CCM Node Resource Functions  *********************

// Returns the instance with the specified ID
func (c *Cloud) getInstanceByID(instanceID string) (*ec2.Instance, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getInstanceByID(%v)", instanceID)
	instances, err := c.getInstancesByIDs([]*string{&instanceID})
	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, cloudprovider.InstanceNotFound
	}
	if len(instances) > 1 {
		return nil, fmt.Errorf("multiple instances found for instance: %s", instanceID)
	}

	return instances[instanceID], nil
}

func (c *Cloud) getInstancesByIDs(instanceIDs []*string) (map[string]*ec2.Instance, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getInstancesByIDs(%v)", instanceIDs)

	instancesByID := make(map[string]*ec2.Instance)
	if len(instanceIDs) == 0 {
		return instancesByID, nil
	}

	request := &ec2.DescribeInstancesInput{
		InstanceIds: instanceIDs,
	}

	instances, err := c.compute.ReadVms(request)
	if err != nil {
		return nil, err
	}

	for _, instance := range instances {
		instanceID := aws.StringValue(instance.InstanceId)
		if instanceID == "" {
			continue
		}

		instancesByID[instanceID] = instance
	}

	return instancesByID, nil
}

func (c *Cloud) getInstancesByNodeNames(nodeNames []string, states ...string) ([]*ec2.Instance, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getInstancesByNodeNames(%v, %v)", nodeNames, states)

	names := aws.StringSlice(nodeNames)
	ec2Instances := []*ec2.Instance{}

	for i := 0; i < len(names); i += filterNodeLimit {
		end := i + filterNodeLimit
		if end > len(names) {
			end = len(names)
		}

		nameSlice := names[i:end]

		nodeNameFilter := &ec2.Filter{
			Name:   aws.String("private-dns-name"),
			Values: nameSlice,
		}

		filters := []*ec2.Filter{nodeNameFilter}
		if len(states) > 0 {
			filters = append(filters, newEc2Filter("instance-state-name", states...))
		}

		instances, err := c.describeInstances(filters)
		if err != nil {
			klog.V(2).Infof("Failed to describe instances %v", nodeNames)
			return nil, err
		}
		ec2Instances = append(ec2Instances, instances...)
	}

	if len(ec2Instances) == 0 {
		klog.V(3).Infof("Failed to find any instances %v", nodeNames)
		return nil, nil
	}
	return ec2Instances, nil
}

// TODO: Move to instanceCache
func (c *Cloud) describeInstances(filters []*ec2.Filter) ([]*ec2.Instance, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("describeInstances(%v)", filters)

	request := &ec2.DescribeInstancesInput{
		Filters: filters,
	}

	response, err := c.compute.ReadVms(request)
	if err != nil {
		return nil, err
	}

	var matches []*ec2.Instance
	for _, instance := range response {
		if c.tagging.hasClusterTag(instance.Tags) {
			matches = append(matches, instance)
		}
	}
	return matches, nil
}

// Returns the instance with the specified node name
// Returns nil if it does not exist
func (c *Cloud) findInstanceByNodeName(nodeName types.NodeName) (*ec2.Instance, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findInstanceByNodeName(%v)", nodeName)

	privateDNSName := mapNodeNameToPrivateDNSName(nodeName)
	filters := []*ec2.Filter{
		newEc2Filter("tag:"+TagNameClusterNode, privateDNSName),
		// exclude instances in "terminated" state
		newEc2Filter("instance-state-name", aliveFilter...),
		newEc2Filter("tag:"+c.tagging.clusterTagKey(),
			[]string{ResourceLifecycleOwned, ResourceLifecycleShared}...),
	}

	instances, err := c.describeInstances(filters)

	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, nil
	}
	if len(instances) > 1 {
		return nil, fmt.Errorf("multiple instances found for name: %s", nodeName)
	}

	return instances[0], nil
}

// Returns the instance with the specified node name
// Like findInstanceByNodeName, but returns error if node not found
func (c *Cloud) getInstanceByNodeName(nodeName types.NodeName) (*ec2.Instance, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getInstanceByNodeName(%v)", nodeName)

	var instance *ec2.Instance

	// we leverage node cache to try to retrieve node's provider id first, as
	// get instance by provider id is way more efficient than by filters in
	// aws context
	awsID, err := c.nodeNameToProviderID(nodeName)
	if err != nil {
		klog.V(3).Infof("Unable to convert node name %q to aws instanceID, fall back to findInstanceByNodeName: %v", nodeName, err)
		instance, err = c.findInstanceByNodeName(nodeName)
		// we need to set provider id for next calls

	} else {
		instance, err = c.getInstanceByID(string(awsID))
	}
	if err == nil && instance == nil {
		return nil, cloudprovider.InstanceNotFound
	}
	return instance, err
}

func (c *Cloud) getFullInstance(nodeName types.NodeName) (*awsInstance, *ec2.Instance, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getFullInstance(%v)", nodeName)
	if nodeName == "" {
		instance, err := c.getInstanceByID(c.selfAWSInstance.awsID)
		return c.selfAWSInstance, instance, err
	}
	instance, err := c.getInstanceByNodeName(nodeName)
	if err != nil {
		return nil, nil, err
	}
	awsInstance := newAWSInstance(c.compute, instance)
	return awsInstance, instance, err
}

func (c *Cloud) nodeNameToProviderID(nodeName types.NodeName) (InstanceID, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("nodeNameToProviderID(%v)", nodeName)
	if len(nodeName) == 0 {
		return "", fmt.Errorf("no nodeName provided")
	}

	if c.nodeInformerHasSynced == nil || !c.nodeInformerHasSynced() {
		return "", fmt.Errorf("node informer has not synced yet")
	}

	node, err := c.nodeInformer.Lister().Get(string(nodeName))
	if err != nil {
		return "", err
	}
	if len(node.Spec.ProviderID) == 0 {
		return "", fmt.Errorf("node has no providerID")
	}

	return KubernetesInstanceID(node.Spec.ProviderID).MapToAWSInstanceID()
}
