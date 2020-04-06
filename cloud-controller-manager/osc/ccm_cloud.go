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
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"k8s.io/api/core/v1"
	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	informercorev1 "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/cloud-provider"
	nodehelpers "k8s.io/cloud-provider/node/helpers"
	servicehelpers "k8s.io/cloud-provider/service/helpers"
	cloudvolume "k8s.io/cloud-provider/volume"
	volerr "k8s.io/cloud-provider/volume/errors"
	volumehelpers "k8s.io/cloud-provider/volume/helpers"
)



// ********************* CCM Cloud Object Def *********************

// Cloud is an implementation of Interface, LoadBalancer and Instances for Amazon Web Services.
type Cloud struct {
	ec2      EC2
	elb      ELB
	elbv2    ELBV2
	asg      ASG
	kms      KMS
	metadata EC2Metadata
	cfg      *CloudConfig
	region   string
	vpcID    string

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

	eventBroadcaster record.EventBroadcaster
	eventRecorder    record.EventRecorder

	// We keep an active list of devices we have assigned but not yet
	// attached, to avoid a race condition where we assign a device mapping
	// and then get a second request before we attach the volume
	attachingMutex sync.Mutex
	attaching      map[types.NodeName]map[mountDevice]EBSVolumeID

	// state of our device allocator for each node
	deviceAllocators map[types.NodeName]DeviceAllocator
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
	return newAWSInstance(c.ec2, instance), nil
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

// Zones returns an implementation of Zones for Amazon Web Services.
func (c *Cloud) Zones() (cloudprovider.Zones, bool) {
	debugPrintCallerFunctionName()
	return c, true
}

// Routes returns an implementation of Routes for Amazon Web Services.
func (c *Cloud) Routes() (cloudprovider.Routes, bool) {
	debugPrintCallerFunctionName()
	return c, true
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

	instance, err := describeInstance(c.ec2, instanceID)
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
		InstanceIds: []*string{instanceID.awsString()},
	}

	instances, err := c.ec2.DescribeInstances(request)
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

	instances, err := c.ec2.DescribeInstances(request)
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

	instance, err := describeInstance(c.ec2, instanceID)
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

// GetCandidateZonesForDynamicVolume retrieves  a list of all the zones in which nodes are running
// It currently involves querying all instances
func (c *Cloud) GetCandidateZonesForDynamicVolume() (sets.String, error) {
	// We don't currently cache this; it is currently used only in volume
	// creation which is expected to be a comparatively rare occurrence.

	// TODO: Caching / expose v1.Nodes to the cloud provider?
	// TODO: We could also query for subnets, I think

	// Note: It is more efficient to call the EC2 API twice with different tag
	// filters than to call it once with a tag filter that results in a logical
	// OR. For really large clusters the logical OR will result in EC2 API rate
	// limiting.
	instances := []*ec2.Instance{}

	baseFilters := []*ec2.Filter{newEc2Filter("instance-state-name", "running")}

	filters := c.tagging.addFilters(baseFilters)
	di, err := c.describeInstances(filters)
	if err != nil {
		return nil, err
	}

	instances = append(instances, di...)

	if c.tagging.usesLegacyTags {
		filters = c.tagging.addLegacyFilters(baseFilters)
		di, err = c.describeInstances(filters)
		if err != nil {
			return nil, err
		}

		instances = append(instances, di...)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no instances returned")
	}

	zones := sets.NewString()

	for _, instance := range instances {
		// We skip over master nodes, if the installation tool labels them with one of the well-known master labels
		// This avoids creating a volume in a zone where only the master is running - e.g. #34583
		// This is a short-term workaround until the scheduler takes care of zone selection
		master := false
		for _, tag := range instance.Tags {
			tagKey := aws.StringValue(tag.Key)
			if awsTagNameMasterRoles.Has(tagKey) {
				master = true
			}
		}

		if master {
			klog.V(4).Infof("Ignoring master instance %q in zone discovery", aws.StringValue(instance.InstanceId))
			continue
		}

		if instance.Placement != nil {
			zone := aws.StringValue(instance.Placement.AvailabilityZone)
			zones.Insert(zone)
		}
	}

	klog.V(2).Infof("Found instances in zones %s", zones)
	return zones, nil
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

// Gets the mountDevice already assigned to the volume, or assigns an unused mountDevice.
// If the volume is already assigned, this will return the existing mountDevice with alreadyAttached=true.
// Otherwise the mountDevice is assigned by finding the first available mountDevice, and it is returned with alreadyAttached=false.
func (c *Cloud) getMountDevice(i *awsInstance, info *ec2.Instance,
 								volumeID EBSVolumeID,
 								assign bool) (assigned mountDevice, alreadyAttached bool, err error) {

	debugPrintCallerFunctionName()
	deviceMappings := map[mountDevice]EBSVolumeID{}
	for _, blockDevice := range info.BlockDeviceMappings {
		name := aws.StringValue(blockDevice.DeviceName)
		if strings.HasPrefix(name, "/dev/sd") {
			name = name[7:]
		}
		if strings.HasPrefix(name, "/dev/xvd") {
			name = name[8:]
		}
		if len(name) < 1 || len(name) > 2 {
			klog.Warningf("Unexpected EBS DeviceName: %q", aws.StringValue(blockDevice.DeviceName))
		}
		deviceMappings[mountDevice(name)] = EBSVolumeID(aws.StringValue(blockDevice.Ebs.VolumeId))
	}

	// We lock to prevent concurrent mounts from conflicting
	// We may still conflict if someone calls the API concurrently,
	// but the AWS API will then fail one of the two attach operations
	c.attachingMutex.Lock()
	defer c.attachingMutex.Unlock()

	for mountDevice, volume := range c.attaching[i.nodeName] {
		deviceMappings[mountDevice] = volume
	}

	// Check to see if this volume is already assigned a device on this machine
	for mountDevice, mappingVolumeID := range deviceMappings {
		if volumeID == mappingVolumeID {
			if assign {
				klog.Warningf("Got assignment call for already-assigned volume: %s@%s", mountDevice, mappingVolumeID)
			}
			return mountDevice, true, nil
		}
	}

	if !assign {
		return mountDevice(""), false, nil
	}

	// Find the next unused device name
	deviceAllocator := c.deviceAllocators[i.nodeName]
	if deviceAllocator == nil {
		// we want device names with two significant characters, starting with /dev/xvdbb
		// the allowed range is /dev/xvd[b-c][a-z]
		// http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
		deviceAllocator = NewDeviceAllocator()
		c.deviceAllocators[i.nodeName] = deviceAllocator
	}
	// We need to lock deviceAllocator to prevent possible race with Deprioritize function
	deviceAllocator.Lock()
	defer deviceAllocator.Unlock()

	chosen, err := deviceAllocator.GetNext(deviceMappings)
	if err != nil {
		klog.Warningf("Could not assign a mount device.  mappings=%v, error: %v", deviceMappings, err)
		return "", false, fmt.Errorf("too many EBS volumes attached to node %s", i.nodeName)
	}

	attaching := c.attaching[i.nodeName]
	if attaching == nil {
		attaching = make(map[mountDevice]EBSVolumeID)
		c.attaching[i.nodeName] = attaching
	}
	attaching[chosen] = volumeID
	klog.V(2).Infof("Assigned mount device %s -> volume %s", chosen, volumeID)

	return chosen, false, nil
}

// endAttaching removes the entry from the "attachments in progress" map
// It returns true if it was found (and removed), false otherwise
func (c *Cloud) endAttaching(i *awsInstance, volumeID EBSVolumeID, mountDevice mountDevice) bool {
	c.attachingMutex.Lock()
	defer c.attachingMutex.Unlock()

	existingVolumeID, found := c.attaching[i.nodeName][mountDevice]
	if !found {
		return false
	}
	if volumeID != existingVolumeID {
		// This actually can happen, because getMountDevice combines the attaching map with the volumes
		// attached to the instance (as reported by the EC2 API).  So if endAttaching comes after
		// a 10 second poll delay, we might well have had a concurrent request to allocate a mountpoint,
		// which because we allocate sequentially is _very_ likely to get the immediately freed volume
		klog.Infof("endAttaching on device %q assigned to different volume: %q vs %q", mountDevice, volumeID, existingVolumeID)
		return false
	}
	klog.V(2).Infof("Releasing in-process attachment entry: %s -> volume %s", mountDevice, volumeID)
	delete(c.attaching[i.nodeName], mountDevice)
	return true
}

// applyUnSchedulableTaint applies a unschedulable taint to a node after verifying
// if node has become unusable because of volumes getting stuck in attaching state.
func (c *Cloud) applyUnSchedulableTaint(nodeName types.NodeName, reason string) {
	node, fetchErr := c.kubeClient.CoreV1().Nodes().Get(string(nodeName), metav1.GetOptions{})
	if fetchErr != nil {
		klog.Errorf("Error fetching node %s with %v", nodeName, fetchErr)
		return
	}

	taint := &v1.Taint{
		Key:    nodeWithImpairedVolumes,
		Value:  "true",
		Effect: v1.TaintEffectNoSchedule,
	}
	err := nodehelpers.AddOrUpdateTaintOnNode(c.kubeClient, string(nodeName), taint)
	if err != nil {
		klog.Errorf("Error applying taint to node %s with error %v", nodeName, err)
		return
	}
	c.eventRecorder.Eventf(node, v1.EventTypeWarning, volumeAttachmentStuck, reason)
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


// ********************* CCM Cloud Resource Volumes Functions  *********************

// AttachDisk implements Volumes.AttachDisk
func (c *Cloud) AttachDisk(diskName KubernetesVolumeID, nodeName types.NodeName) (string, error) {
	disk, err := newAWSDisk(c, diskName)
	if err != nil {
		return "", err
	}

	awsInstance, info, err := c.getFullInstance(nodeName)
	if err != nil {
		return "", fmt.Errorf("error finding instance %s: %q", nodeName, err)
	}

	// mountDevice will hold the device where we should try to attach the disk
	var mountDevice mountDevice
	// alreadyAttached is true if we have already called AttachVolume on this disk
	var alreadyAttached bool

	// attachEnded is set to true if the attach operation completed
	// (successfully or not), and is thus no longer in progress
	attachEnded := false
	defer func() {
		if attachEnded {
			if !c.endAttaching(awsInstance, disk.awsID, mountDevice) {
				klog.Errorf("endAttaching called for disk %q when attach not in progress", disk.awsID)
			}
		}
	}()

	mountDevice, alreadyAttached, err = c.getMountDevice(awsInstance, info, disk.awsID, true)
	if err != nil {
		return "", err
	}

	// Inside the instance, the mountpoint always looks like /dev/xvdX (?)
	hostDevice := "/dev/xvd" + string(mountDevice)
	// We are using xvd names (so we are HVM only)
	// See http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
	ec2Device := "/dev/xvd" + string(mountDevice)

	if !alreadyAttached {
		available, err := c.checkIfAvailable(disk, "attaching", awsInstance.awsID)
		if err != nil {
			klog.Error(err)
		}

		if !available {
			attachEnded = true
			return "", err
		}
		request := &ec2.AttachVolumeInput{
			Device:     aws.String(ec2Device),
			InstanceId: aws.String(awsInstance.awsID),
			VolumeId:   disk.awsID.awsString(),
		}

		attachResponse, err := c.ec2.AttachVolume(request)
		if err != nil {
			attachEnded = true
			// TODO: Check if the volume was concurrently attached?
			return "", wrapAttachError(err, disk, awsInstance.awsID)
		}
		if da, ok := c.deviceAllocators[awsInstance.nodeName]; ok {
			da.Deprioritize(mountDevice)
		}
		klog.V(2).Infof("AttachVolume volume=%q instance=%q request returned %v", disk.awsID, awsInstance.awsID, attachResponse)
	}

	attachment, err := disk.waitForAttachmentStatus("attached")

	if err != nil {
		if err == wait.ErrWaitTimeout {
			c.applyUnSchedulableTaint(nodeName, "Volume stuck in attaching state - node needs reboot to fix impaired state.")
		}
		return "", err
	}

	// The attach operation has finished
	attachEnded = true

	// Double check the attachment to be 100% sure we attached the correct volume at the correct mountpoint
	// It could happen otherwise that we see the volume attached from a previous/separate AttachVolume call,
	// which could theoretically be against a different device (or even instance).
	if attachment == nil {
		// Impossible?
		return "", fmt.Errorf("unexpected state: attachment nil after attached %q to %q", diskName, nodeName)
	}
	if ec2Device != aws.StringValue(attachment.Device) {
		return "", fmt.Errorf("disk attachment of %q to %q failed: requested device %q but found %q", diskName, nodeName, ec2Device, aws.StringValue(attachment.Device))
	}
	if awsInstance.awsID != aws.StringValue(attachment.InstanceId) {
		return "", fmt.Errorf("disk attachment of %q to %q failed: requested instance %q but found %q", diskName, nodeName, awsInstance.awsID, aws.StringValue(attachment.InstanceId))
	}

	return hostDevice, nil
}

// DetachDisk implements Volumes.DetachDisk
func (c *Cloud) DetachDisk(diskName KubernetesVolumeID, nodeName types.NodeName) (string, error) {
	diskInfo, attached, err := c.checkIfAttachedToNode(diskName, nodeName)
	if err != nil {
		if isAWSErrorVolumeNotFound(err) {
			// Someone deleted the volume being detached; complain, but do nothing else and return success
			klog.Warningf("DetachDisk %s called for node %s but volume does not exist; assuming the volume is detached", diskName, nodeName)
			return "", nil
		}

		return "", err
	}

	if !attached && diskInfo.ec2Instance != nil {
		klog.Warningf("DetachDisk %s called for node %s but volume is attached to node %s", diskName, nodeName, diskInfo.nodeName)
		return "", nil
	}

	if !attached {
		return "", nil
	}

	awsInstance := newAWSInstance(c.ec2, diskInfo.ec2Instance)

	mountDevice, alreadyAttached, err := c.getMountDevice(awsInstance, diskInfo.ec2Instance, diskInfo.disk.awsID, false)
	if err != nil {
		return "", err
	}

	if !alreadyAttached {
		klog.Warningf("DetachDisk called on non-attached disk: %s", diskName)
		// TODO: Continue?  Tolerate non-attached error from the AWS DetachVolume call?
	}

	request := ec2.DetachVolumeInput{
		InstanceId: &awsInstance.awsID,
		VolumeId:   diskInfo.disk.awsID.awsString(),
	}

	response, err := c.ec2.DetachVolume(&request)
	if err != nil {
		return "", fmt.Errorf("error detaching EBS volume %q from %q: %q", diskInfo.disk.awsID, awsInstance.awsID, err)
	}

	if response == nil {
		return "", errors.New("no response from DetachVolume")
	}

	attachment, err := diskInfo.disk.waitForAttachmentStatus("detached")
	if err != nil {
		return "", err
	}
	if da, ok := c.deviceAllocators[awsInstance.nodeName]; ok {
		da.Deprioritize(mountDevice)
	}
	if attachment != nil {
		// We expect it to be nil, it is (maybe) interesting if it is not
		klog.V(2).Infof("waitForAttachmentStatus returned non-nil attachment with state=detached: %v", attachment)
	}

	if mountDevice != "" {
		c.endAttaching(awsInstance, diskInfo.disk.awsID, mountDevice)
		// We don't check the return value - we don't really expect the attachment to have been
		// in progress, though it might have been
	}

	hostDevicePath := "/dev/xvd" + string(mountDevice)
	return hostDevicePath, err
}

// CreateDisk implements Volumes.CreateDisk
func (c *Cloud) CreateDisk(volumeOptions *VolumeOptions) (KubernetesVolumeID, error) {
	var createType string
	var iops int64
	switch volumeOptions.VolumeType {
	case VolumeTypeGP2, VolumeTypeSC1, VolumeTypeST1:
		createType = volumeOptions.VolumeType

	case VolumeTypeIO1:
		// See http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CreateVolume.html
		// for IOPS constraints. AWS will throw an error if IOPS per GB gets out
		// of supported bounds, no need to check it here.
		createType = volumeOptions.VolumeType
		iops = int64(volumeOptions.CapacityGB * volumeOptions.IOPSPerGB)

		// Cap at min/max total IOPS, AWS would throw an error if it gets too
		// low/high.
		if iops < MinTotalIOPS {
			iops = MinTotalIOPS
		}
		if iops > MaxTotalIOPS {
			iops = MaxTotalIOPS
		}

	case "":
		createType = DefaultVolumeType

	default:
		return "", fmt.Errorf("invalid AWS VolumeType %q", volumeOptions.VolumeType)
	}

	request := &ec2.CreateVolumeInput{}
	request.AvailabilityZone = aws.String(volumeOptions.AvailabilityZone)
	request.Size = aws.Int64(int64(volumeOptions.CapacityGB))
	request.VolumeType = aws.String(createType)
	request.Encrypted = aws.Bool(volumeOptions.Encrypted)
	if len(volumeOptions.KmsKeyID) > 0 {
		request.KmsKeyId = aws.String(volumeOptions.KmsKeyID)
		request.Encrypted = aws.Bool(true)
	}
	if iops > 0 {
		request.Iops = aws.Int64(iops)
	}

	tags := volumeOptions.Tags
	tags = c.tagging.buildTags(ResourceLifecycleOwned, tags)

	var tagList []*ec2.Tag
	for k, v := range tags {
		tagList = append(tagList, &ec2.Tag{
			Key: aws.String(k), Value: aws.String(v),
		})
	}
	request.TagSpecifications = append(request.TagSpecifications, &ec2.TagSpecification{
		Tags:         tagList,
		ResourceType: aws.String(ec2.ResourceTypeVolume),
	})

	response, err := c.ec2.CreateVolume(request)
	if err != nil {
		return "", err
	}

	awsID := EBSVolumeID(aws.StringValue(response.VolumeId))
	if awsID == "" {
		return "", fmt.Errorf("VolumeID was not returned by CreateVolume")
	}
	volumeName := KubernetesVolumeID("aws://" + aws.StringValue(response.AvailabilityZone) + "/" + string(awsID))

	err = c.waitUntilVolumeAvailable(volumeName)
	if err != nil {
		// AWS has a bad habbit of reporting success when creating a volume with
		// encryption keys that either don't exists or have wrong permissions.
		// Such volume lives for couple of seconds and then it's silently deleted
		// by AWS. There is no other check to ensure that given KMS key is correct,
		// because Kubernetes may have limited permissions to the key.
		if isAWSErrorVolumeNotFound(err) {
			err = fmt.Errorf("failed to create encrypted volume: the volume disappeared after creation, most likely due to inaccessible KMS encryption key")
		}
		return "", err
	}

	return volumeName, nil
}

func (c *Cloud) waitUntilVolumeAvailable(volumeName KubernetesVolumeID) error {
	disk, err := newAWSDisk(c, volumeName)
	if err != nil {
		// Unreachable code
		return err
	}
	time.Sleep(5 * time.Second)
	backoff := wait.Backoff{
		Duration: volumeCreateInitialDelay,
		Factor:   volumeCreateBackoffFactor,
		Steps:    volumeCreateBackoffSteps,
	}
	err = wait.ExponentialBackoff(backoff, func() (done bool, err error) {
		vol, err := disk.describeVolume()
		if err != nil {
			return true, err
		}
		if vol.State != nil {
			switch *vol.State {
			case "available":
				// The volume is Available, it won't be deleted now.
				return true, nil
			case "creating":
				return false, nil
			default:
				return true, fmt.Errorf("unexpected State of newly created AWS EBS volume %s: %q", volumeName, *vol.State)
			}
		}
		return false, nil
	})
	return err
}

// DeleteDisk implements Volumes.DeleteDisk
func (c *Cloud) DeleteDisk(volumeName KubernetesVolumeID) (bool, error) {
	awsDisk, err := newAWSDisk(c, volumeName)
	if err != nil {
		return false, err
	}
	available, err := c.checkIfAvailable(awsDisk, "deleting", "")
	if err != nil {
		if isAWSErrorVolumeNotFound(err) {
			klog.V(2).Infof("Volume %s not found when deleting it, assuming it's deleted", awsDisk.awsID)
			return false, nil
		}
		klog.Error(err)
	}

	if !available {
		return false, err
	}

	return awsDisk.deleteVolume()
}

func (c *Cloud) checkIfAvailable(disk *awsDisk, opName string, instance string) (bool, error) {
	info, err := disk.describeVolume()

	if err != nil {
		klog.Errorf("Error describing volume %q: %q", disk.awsID, err)
		// if for some reason we can not describe volume we will return error
		return false, err
	}

	volumeState := aws.StringValue(info.State)
	opError := fmt.Sprintf("Error %s EBS volume %q", opName, disk.awsID)
	if len(instance) != 0 {
		opError = fmt.Sprintf("%q to instance %q", opError, instance)
	}

	// Only available volumes can be attached or deleted
	if volumeState != "available" {
		// Volume is attached somewhere else and we can not attach it here
		if len(info.Attachments) > 0 {
			attachment := info.Attachments[0]
			instanceID := aws.StringValue(attachment.InstanceId)
			attachedInstance, ierr := c.getInstanceByID(instanceID)
			attachErr := fmt.Sprintf("%s since volume is currently attached to %q", opError, instanceID)
			if ierr != nil {
				klog.Error(attachErr)
				return false, errors.New(attachErr)
			}
			devicePath := aws.StringValue(attachment.Device)
			nodeName := mapInstanceToNodeName(attachedInstance)

			danglingErr := volerr.NewDanglingError(attachErr, nodeName, devicePath)
			return false, danglingErr
		}

		attachErr := fmt.Errorf("%s since volume is in %q state", opError, volumeState)
		return false, attachErr
	}

	return true, nil
}

// GetLabelsForVolume gets the volume labels for a volume
func (c *Cloud) GetLabelsForVolume(ctx context.Context, pv *v1.PersistentVolume) (map[string]string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("GetLabelsForVolume(%v)", pv)
	// Ignore if not AWSElasticBlockStore.
	if pv.Spec.AWSElasticBlockStore == nil {
		return nil, nil
	}

	// Ignore any volumes that are being provisioned
	if pv.Spec.AWSElasticBlockStore.VolumeID == cloudvolume.ProvisionedVolumeName {
		return nil, nil
	}

	spec := KubernetesVolumeID(pv.Spec.AWSElasticBlockStore.VolumeID)
	labels, err := c.GetVolumeLabels(spec)
	if err != nil {
		return nil, err
	}

	return labels, nil
}

// GetVolumeLabels implements Volumes.GetVolumeLabels
func (c *Cloud) GetVolumeLabels(volumeName KubernetesVolumeID) (map[string]string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("GetVolumeLabels (%v)", volumeName)
	awsDisk, err := newAWSDisk(c, volumeName)
	if err != nil {
		return nil, err
	}
	info, err := awsDisk.describeVolume()
	if err != nil {
		return nil, err
	}
	labels := make(map[string]string)
	az := aws.StringValue(info.AvailabilityZone)
	if az == "" {
		return nil, fmt.Errorf("volume did not have AZ information: %q", aws.StringValue(info.VolumeId))
	}

	labels[v1.LabelZoneFailureDomain] = az
	region, err := azToRegion(az)
	if err != nil {
		return nil, err
	}
	labels[v1.LabelZoneRegion] = region

	return labels, nil
}

// GetDiskPath implements Volumes.GetDiskPath
func (c *Cloud) GetDiskPath(volumeName KubernetesVolumeID) (string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("GetDiskPath(%v)", volumeName)
	awsDisk, err := newAWSDisk(c, volumeName)
	if err != nil {
		return "", err
	}
	info, err := awsDisk.describeVolume()
	if err != nil {
		return "", err
	}
	if len(info.Attachments) == 0 {
		return "", fmt.Errorf("No attachment to volume %s", volumeName)
	}
	return aws.StringValue(info.Attachments[0].Device), nil
}

// DiskIsAttached implements Volumes.DiskIsAttached
func (c *Cloud) DiskIsAttached(diskName KubernetesVolumeID, nodeName types.NodeName) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("DiskIsAttached(%v, %v)", diskName, nodeName)
	_, attached, err := c.checkIfAttachedToNode(diskName, nodeName)
	if err != nil {
		if isAWSErrorVolumeNotFound(err) {
			// The disk doesn't exist, can't be attached
			klog.Warningf("DiskIsAttached called for volume %s on node %s but the volume does not exist", diskName, nodeName)
			return false, nil
		}

		return true, err
	}

	return attached, nil
}

// DisksAreAttached returns a map of nodes and Kubernetes volume IDs indicating
// if the volumes are attached to the node
func (c *Cloud) DisksAreAttached(nodeDisks map[types.NodeName][]KubernetesVolumeID) (map[types.NodeName]map[KubernetesVolumeID]bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("DisksAreAttached(%v)", nodeDisks)
	attached := make(map[types.NodeName]map[KubernetesVolumeID]bool)

	if len(nodeDisks) == 0 {
		return attached, nil
	}

	nodeNames := []string{}
	for nodeName, diskNames := range nodeDisks {
		for _, diskName := range diskNames {
			setNodeDisk(attached, diskName, nodeName, false)
		}
		nodeNames = append(nodeNames, mapNodeNameToPrivateDNSName(nodeName))
	}

	// Note that we get instances regardless of state.
	// This means there might be multiple nodes with the same node names.
	awsInstances, err := c.getInstancesByNodeNames(nodeNames)
	if err != nil {
		// When there is an error fetching instance information
		// it is safer to return nil and let volume information not be touched.
		return nil, err
	}

	if len(awsInstances) == 0 {
		klog.V(2).Infof("DisksAreAttached found no instances matching node names; will assume disks not attached")
		return attached, nil
	}

	// Note that we check that the volume is attached to the correct node, not that it is attached to _a_ node
	for _, awsInstance := range awsInstances {
		nodeName := mapInstanceToNodeName(awsInstance)

		diskNames := nodeDisks[nodeName]
		if len(diskNames) == 0 {
			continue
		}

		awsInstanceState := "<nil>"
		if awsInstance != nil && awsInstance.State != nil {
			awsInstanceState = aws.StringValue(awsInstance.State.Name)
		}
		if awsInstanceState == "terminated" {
			// Instance is terminated, safe to assume volumes not attached
			// Note that we keep volumes attached to instances in other states (most notably, stopped)
			continue
		}

		idToDiskName := make(map[EBSVolumeID]KubernetesVolumeID)
		for _, diskName := range diskNames {
			volumeID, err := diskName.MapToAWSVolumeID()
			if err != nil {
				return nil, fmt.Errorf("error mapping volume spec %q to aws id: %v", diskName, err)
			}
			idToDiskName[volumeID] = diskName
		}

		for _, blockDevice := range awsInstance.BlockDeviceMappings {
			volumeID := EBSVolumeID(aws.StringValue(blockDevice.Ebs.VolumeId))
			diskName, found := idToDiskName[volumeID]
			if found {
				// Disk is still attached to node
				setNodeDisk(attached, diskName, nodeName, true)
			}
		}
	}

	return attached, nil
}

// ResizeDisk resizes an EBS volume in GiB increments, it will round up to the
// next GiB if arguments are not provided in even GiB increments
func (c *Cloud) ResizeDisk(diskName KubernetesVolumeID,
						   oldSize resource.Quantity,
						   newSize resource.Quantity) (resource.Quantity, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("ResizeDisk(%v,%v,%v)", diskName, oldSize, newSize)
	awsDisk, err := newAWSDisk(c, diskName)
	if err != nil {
		return oldSize, err
	}

	volumeInfo, err := awsDisk.describeVolume()
	if err != nil {
		descErr := fmt.Errorf("AWS.ResizeDisk Error describing volume %s with %v", diskName, err)
		return oldSize, descErr
	}
	// AWS resizes in chunks of GiB (not GB)
	requestGiB := volumehelpers.RoundUpToGiB(newSize)
	newSizeQuant := resource.MustParse(fmt.Sprintf("%dGi", requestGiB))

	// If disk already if of greater or equal size than requested we return
	if aws.Int64Value(volumeInfo.Size) >= requestGiB {
		return newSizeQuant, nil
	}
	_, err = awsDisk.modifyVolume(requestGiB)

	if err != nil {
		return oldSize, err
	}
	return newSizeQuant, nil
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

// Gets the current load balancer state
func (c *Cloud) describeLoadBalancerv2(name string) (*elbv2.LoadBalancer, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("describeLoadBalancerv2(%v)", name)
	request := &elbv2.DescribeLoadBalancersInput{
		Names: []*string{aws.String(name)},
	}

	response, err := c.elbv2.DescribeLoadBalancers(request)
	if err != nil {
		if awsError, ok := err.(awserr.Error); ok {
			if awsError.Code() == elbv2.ErrCodeLoadBalancerNotFoundException {
				return nil, nil
			}
		}
		return nil, fmt.Errorf("error describing load balancer: %q", err)
	}

	// AWS will not return 2 load balancers with the same name _and_ type.
	for i := range response.LoadBalancers {
		if aws.StringValue(response.LoadBalancers[i].Type) == elbv2.LoadBalancerTypeEnumNetwork {
			return response.LoadBalancers[i], nil
		}
	}

	return nil, fmt.Errorf("NLB '%s' could not be found", name)
}



// Retrieves the specified security group from the AWS API, or returns nil if not found
func (c *Cloud) findSecurityGroup(securityGroupID string) (*ec2.SecurityGroup, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findSecurityGroup(%v)", securityGroupID)
	describeSecurityGroupsRequest := &ec2.DescribeSecurityGroupsInput{
		GroupIds: []*string{&securityGroupID},
	}
	// We don't apply our tag filters because we are retrieving by ID

	groups, err := c.ec2.DescribeSecurityGroups(describeSecurityGroupsRequest)
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
		_, err = c.ec2.AuthorizeSecurityGroupIngress(request)
		if err != nil {
			return false, fmt.Errorf("error authorizing security group ingress: %q", err)
		}
	}
	if remove.Len() != 0 {
		klog.V(2).Infof("Remove security group ingress: %s %v", securityGroupID, remove.List())

		request := &ec2.RevokeSecurityGroupIngressInput{}
		request.GroupId = &securityGroupID
		request.IpPermissions = remove.List()
		_, err = c.ec2.RevokeSecurityGroupIngress(request)
		if err != nil {
			return false, fmt.Errorf("error revoking security group ingress: %q", err)
		}
	}

	return true, nil
}

// Makes sure the security group includes the specified permissions
// Returns true if and only if changes were made
// The security group must already exist
func (c *Cloud) addSecurityGroupIngress(securityGroupID string, addPermissions []*ec2.IpPermission) (bool, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("addSecurityGroupIngress(%v,%v)", securityGroupID, addPermissions)
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

	if len(changes) == 0 {
		return false, nil
	}

	klog.Infof("Adding security group ingress: %s %v", securityGroupID, changes)

	request := &ec2.AuthorizeSecurityGroupIngressInput{}
	request.GroupId = &securityGroupID
	request.IpPermissions = changes
	_, err = c.ec2.AuthorizeSecurityGroupIngress(request)
	if err != nil {
		klog.Warningf("Error authorizing security group ingress %q", err)
		return false, fmt.Errorf("error authorizing security group ingress: %q", err)
	}

	return true, nil
}

// Makes sure the security group no longer includes the specified permissions
// Returns true if and only if changes were made
// If the security group no longer exists, will return (false, nil)
func (c *Cloud) removeSecurityGroupIngress(securityGroupID string, removePermissions []*ec2.IpPermission) (bool, error) {
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

	if len(changes) == 0 {
		return false, nil
	}

	klog.Infof("Removing security group ingress: %s %v", securityGroupID, changes)

	request := &ec2.RevokeSecurityGroupIngressInput{}
	request.GroupId = &securityGroupID
	request.IpPermissions = changes
	_, err = c.ec2.RevokeSecurityGroupIngress(request)
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
			newEc2Filter("vpc-id", c.vpcID),
		}

		securityGroups, err := c.ec2.DescribeSecurityGroups(request)
		if err != nil {
			return "", err
		}

		if len(securityGroups) >= 1 {
			if len(securityGroups) > 1 {
				klog.Warningf("Found multiple security groups with name: %q", name)
			}
			err := c.tagging.readRepairClusterTags(
				c.ec2, aws.StringValue(securityGroups[0].GroupId),
				ResourceLifecycleOwned, nil, securityGroups[0].Tags)
			if err != nil {
				return "", err
			}

			return aws.StringValue(securityGroups[0].GroupId), nil
		}

		createRequest := &ec2.CreateSecurityGroupInput{}
		createRequest.VpcId = &c.vpcID
		createRequest.GroupName = &name
		createRequest.Description = &description

		createResponse, err := c.ec2.CreateSecurityGroup(createRequest)
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

	err := c.tagging.createTags(c.ec2, groupID, ResourceLifecycleOwned, additionalTags)
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
	request.Filters = []*ec2.Filter{newEc2Filter("vpc-id", c.vpcID)}

	subnets, err := c.ec2.DescribeSubnets(request)
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

	// Fall back to the current instance subnets, if nothing is tagged
	klog.Warningf("No tagged subnets found; will fall-back to the current subnet only.  This is likely to be an error in a future version of k8s.")

	request = &ec2.DescribeSubnetsInput{}
	request.Filters = []*ec2.Filter{newEc2Filter("subnet-id", c.selfAWSInstance.subnetID)}

	subnets, err = c.ec2.DescribeSubnets(request)
	if err != nil {
		return nil, fmt.Errorf("error describing subnets: %q", err)
	}

	return subnets, nil
}

// Finds the subnets to use for an ELB we are creating.
// Normal (Internet-facing) ELBs must use public subnets, so we skip private subnets.
// Internal ELBs can use public or private subnets, but if we have a private subnet we should prefer that.
func (c *Cloud) findELBSubnets(internalELB bool) ([]string, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("findELBSubnets(%v)", internalELB)
	vpcIDFilter := newEc2Filter("vpc-id", c.vpcID)

	subnets, err := c.findSubnets()
	if err != nil {
		return nil, err
	}

	rRequest := &ec2.DescribeRouteTablesInput{}
	rRequest.Filters = []*ec2.Filter{vpcIDFilter}
	rt, err := c.ec2.DescribeRouteTables(rRequest)
	if err != nil {
		return nil, fmt.Errorf("error describe route table: %q", err)
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
 									nodes []*v1.Node)(*v1.LoadBalancerStatus, error) {
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
	v2Mappings := []nlbPortMapping{}

	sslPorts := getPortSets(annotations[ServiceAnnotationLoadBalancerSSLPorts])

	for _, port := range apiService.Spec.Ports {
		if port.Protocol != v1.ProtocolTCP {
			return nil, fmt.Errorf("Only TCP LoadBalancer is supported for AWS ELB")
		}
		if port.NodePort == 0 {
			klog.Errorf("Ignoring port without NodePort defined: %v", port)
			continue
		}

		if isNLB(annotations) {
			portMapping := nlbPortMapping{
				FrontendPort:     int64(port.Port),
				FrontendProtocol: string(port.Protocol),
				TrafficPort:      int64(port.NodePort),
				TrafficProtocol:  string(port.Protocol),

				// if externalTrafficPolicy == "Local", we'll override the
				// health check later
				HealthCheckPort:     int64(port.NodePort),
				HealthCheckProtocol: elbv2.ProtocolEnumTcp,
			}

			certificateARN := annotations[ServiceAnnotationLoadBalancerCertificate]
			if certificateARN != "" && (sslPorts == nil || sslPorts.numbers.Has(int64(port.Port)) || sslPorts.names.Has(port.Name)) {
				portMapping.FrontendProtocol = elbv2.ProtocolEnumTls
				portMapping.SSLCertificateARN = certificateARN
				portMapping.SSLPolicy = annotations[ServiceAnnotationLoadBalancerSSLNegotiationPolicy]

				if backendProtocol := annotations[ServiceAnnotationLoadBalancerBEProtocol]; backendProtocol == "ssl" {
					portMapping.TrafficProtocol = elbv2.ProtocolEnumTls
				}
			}

			v2Mappings = append(v2Mappings, portMapping)
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

	if isNLB(annotations) {

		if path, healthCheckNodePort := servicehelpers.GetServiceHealthCheckPathPort(apiService); path != "" {
			for i := range v2Mappings {
				v2Mappings[i].HealthCheckPort = int64(healthCheckNodePort)
				v2Mappings[i].HealthCheckPath = path
				v2Mappings[i].HealthCheckProtocol = elbv2.ProtocolEnumHttp
			}
		}

		// Find the subnets that the ELB will live in
		subnetIDs, err := c.findELBSubnets(internalELB)
		klog.V(10).Infof("Debug OSC:  c.findELBSubnets(internalELB) : %v", subnetIDs)

		if err != nil {
			klog.Errorf("Error listing subnets in VPC: %q", err)
			return nil, err
		}
		// Bail out early if there are no subnets
		if len(subnetIDs) == 0 {
			return nil, fmt.Errorf("could not find any suitable subnets for creating the ELB")
		}

		loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, apiService)
		serviceName := types.NamespacedName{Namespace: apiService.Namespace, Name: apiService.Name}

		instanceIDs := []string{}
		for id := range instances {
			instanceIDs = append(instanceIDs, string(id))
		}

		v2LoadBalancer, err := c.ensureLoadBalancerv2(
			serviceName,
			loadBalancerName,
			v2Mappings,
			instanceIDs,
			subnetIDs,
			internalELB,
			annotations,
		)
		if err != nil {
			return nil, err
		}

		sourceRangeCidrs := []string{}
		for cidr := range sourceRanges {
			sourceRangeCidrs = append(sourceRangeCidrs, cidr)
		}
		if len(sourceRangeCidrs) == 0 {
			sourceRangeCidrs = append(sourceRangeCidrs, "0.0.0.0/0")
		}

		err = c.updateInstanceSecurityGroupsForNLB(loadBalancerName, instances, sourceRangeCidrs, v2Mappings)
		if err != nil {
			klog.Warningf("Error opening ingress rules for the load balancer to the instances: %q", err)
			return nil, err
		}

		// We don't have an `ensureLoadBalancerInstances()` function for elbv2
		// because `ensureLoadBalancerv2()` requires instance Ids

		// TODO: Wait for creation?
		return v2toStatus(v2LoadBalancer), nil
	}

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
		ConnectionDraining:     &elb.ConnectionDraining{Enabled: aws.Bool(false)},
		ConnectionSettings:     &elb.ConnectionSettings{IdleTimeout: aws.Int64(60)},
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
		return nil, fmt.Errorf("could not find any suitable subnets for creating the ELB")
	}

	loadBalancerName := c.GetLoadBalancerName(ctx, clusterName, apiService)
	serviceName := types.NamespacedName{Namespace: apiService.Namespace, Name: apiService.Name}

	klog.V(10).Infof("Debug OSC:  loadBalancerName : %v", loadBalancerName)
	klog.V(10).Infof("Debug OSC:  serviceName : %v", serviceName)
	klog.V(10).Infof("Debug OSC:  serviceName : %v", annotations)

	securityGroupIDs, err := c.buildELBSecurityGroupList(serviceName, loadBalancerName, annotations)
	klog.V(10).Infof("Debug OSC:  ensured securityGroupIDs : %v", securityGroupIDs)


	if err != nil {
		return nil, err
	}
	if len(securityGroupIDs) == 0 {
		return nil, fmt.Errorf("[BUG] ELB can't have empty list of Security Groups to be assigned, this is a Kubernetes bug, please report")
	}

	{
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

	err = c.updateInstanceSecurityGroupsForLoadBalancer(loadBalancer, instances)
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

	if isNLB(service.Annotations) {
		lb, err := c.describeLoadBalancerv2(loadBalancerName)
		if err != nil {
			return nil, false, err
		}
		if lb == nil {
			return nil, false, nil
		}
		return v2toStatus(lb), true, nil
	}

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
	// TODO: replace DefaultLoadBalancerName to generate more meaningful loadbalancer names.
	return cloudprovider.DefaultLoadBalancerName(service)
}

// Return all the security groups that are tagged as being part of our cluster
func (c *Cloud) getTaggedSecurityGroups() (map[string]*ec2.SecurityGroup, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("getTaggedSecurityGroups()")
	request := &ec2.DescribeSecurityGroupsInput{}
	request.Filters = []*ec2.Filter{
		newEc2Filter("tag:" + c.tagging.clusterTagKey(),
					 []string{ResourceLifecycleOwned, ResourceLifecycleShared,}...),
		newEc2Filter("tag:" + TagNameMainSG + c.tagging.clusterID() , "True"),
	}

	groups, err := c.ec2.DescribeSecurityGroups(request)
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
func (c *Cloud) updateInstanceSecurityGroupsForLoadBalancer(lb *elb.LoadBalancerDescription, instances map[InstanceID]*ec2.Instance) error {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("updateInstanceSecurityGroupsForLoadBalancer(%v, %v)", lb, instances)

	if c.cfg.Global.DisableSecurityGroupIngress {
		return nil
	}

	// Determine the load balancer security group id
	loadBalancerSecurityGroupID := ""
	for _, securityGroup := range lb.SecurityGroups {
		if aws.StringValue(securityGroup) == "" {
			continue
		}
		if loadBalancerSecurityGroupID != "" {
			// We create LBs with one SG
			klog.Warningf("Multiple security groups for load balancer: %q", aws.StringValue(lb.LoadBalancerName))
		}
		loadBalancerSecurityGroupID = *securityGroup
	}
	if loadBalancerSecurityGroupID == "" {
		return fmt.Errorf("could not determine security group for load balancer: %s", aws.StringValue(lb.LoadBalancerName))
	}

	// Get the actual list of groups that allow ingress from the load-balancer
	var actualGroups []*ec2.SecurityGroup
	{
		describeRequest := &ec2.DescribeSecurityGroupsInput{}
		describeRequest.Filters = []*ec2.Filter{
			newEc2Filter("ip-permission.group-id", loadBalancerSecurityGroupID),
		}
		response, err := c.ec2.DescribeSecurityGroups(describeRequest)
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

	taggedSecurityGroups, err := c.getTaggedSecurityGroups()
	if err != nil {
		return fmt.Errorf("error querying for tagged security groups: %q", err)
	}

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
	klog.V(2).Infof("instanceSecurityGroupID, add := range instanceSecurityGroupIds %v", instanceSecurityGroupIds)
	for instanceSecurityGroupID, add := range instanceSecurityGroupIds {
		if add {
			klog.V(2).Infof("Adding rule for traffic from the load balancer (%s) to instances (%s)", loadBalancerSecurityGroupID, instanceSecurityGroupID)
		} else {
			klog.V(2).Infof("Removing rule for traffic from the load balancer (%s) to instance (%s)", loadBalancerSecurityGroupID, instanceSecurityGroupID)
		}
		sourceGroupID := &ec2.UserIdGroupPair{}
		sourceGroupID.GroupId = &loadBalancerSecurityGroupID

		allProtocols := "-1"

		permission := &ec2.IpPermission{}
		permission.IpProtocol = &allProtocols
		permission.UserIdGroupPairs = []*ec2.UserIdGroupPair{sourceGroupID}

		permissions := []*ec2.IpPermission{permission}

		if add {
			changed, err := c.addSecurityGroupIngress(instanceSecurityGroupID, permissions)
			if err != nil {
				return err
			}
			if !changed {
				klog.Warning("Allowing ingress was not needed; concurrent change? groupId=", instanceSecurityGroupID)
			}
		} else {
			changed, err := c.removeSecurityGroupIngress(instanceSecurityGroupID, permissions)
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

	if isNLB(service.Annotations) {
		lb, err := c.describeLoadBalancerv2(loadBalancerName)
		if err != nil {
			return err
		}
		if lb == nil {
			klog.Info("Load balancer already deleted: ", loadBalancerName)
			return nil
		}

		// Delete the LoadBalancer and target groups
		//
		// Deleting a target group while associated with a load balancer will
		// fail. We delete the loadbalancer first. This does leave the
		// possibility of zombie target groups if DeleteLoadBalancer() fails
		//
		// * Get target groups for NLB
		// * Delete Load Balancer
		// * Delete target groups
		// * Clean up SecurityGroupRules
		{

			targetGroups, err := c.elbv2.DescribeTargetGroups(
				&elbv2.DescribeTargetGroupsInput{LoadBalancerArn: lb.LoadBalancerArn},
			)
			if err != nil {
				return fmt.Errorf("error listing target groups before deleting load balancer: %q", err)
			}

			_, err = c.elbv2.DeleteLoadBalancer(
				&elbv2.DeleteLoadBalancerInput{LoadBalancerArn: lb.LoadBalancerArn},
			)
			if err != nil {
				return fmt.Errorf("error deleting load balancer %q: %v", loadBalancerName, err)
			}

			for _, group := range targetGroups.TargetGroups {
				_, err := c.elbv2.DeleteTargetGroup(
					&elbv2.DeleteTargetGroupInput{TargetGroupArn: group.TargetGroupArn},
				)
				if err != nil {
					return fmt.Errorf("error deleting target groups after deleting load balancer: %q", err)
				}
			}
		}

		return c.updateInstanceSecurityGroupsForNLB(loadBalancerName, nil, nil, nil)
	}

	lb, err := c.describeLoadBalancer(loadBalancerName)
	if err != nil {
		return err
	}

	if lb == nil {
		klog.Info("Load balancer already deleted: ", loadBalancerName)
		return nil
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
		err = c.updateInstanceSecurityGroupsForLoadBalancer(lb, nil)
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

		var loadBalancerSGs = aws.StringValueSlice(lb.SecurityGroups)

		describeRequest := &ec2.DescribeSecurityGroupsInput{}
		describeRequest.Filters = []*ec2.Filter{
			newEc2Filter("group-id", loadBalancerSGs...),
		}
		response, err := c.ec2.DescribeSecurityGroups(describeRequest)
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
				_, err := c.ec2.DeleteSecurityGroup(request)
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
	if isNLB(service.Annotations) {
		lb, err := c.describeLoadBalancerv2(loadBalancerName)
		if err != nil {
			return err
		}
		if lb == nil {
			return fmt.Errorf("Load balancer not found")
		}
		_, err = c.EnsureLoadBalancer(ctx, clusterName, service, nodes)
		return err
	}
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

	err = c.updateInstanceSecurityGroupsForLoadBalancer(lb, instances)
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

	instances, err := c.ec2.DescribeInstances(request)
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

	response, err := c.ec2.DescribeInstances(request)
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
		newEc2Filter("tag:" + TagNameClusterNode, privateDNSName),
		// exclude instances in "terminated" state
		newEc2Filter("instance-state-name", aliveFilter...),
		newEc2Filter("tag:" + c.tagging.clusterTagKey(),
				     []string{ResourceLifecycleOwned, ResourceLifecycleShared,}...),
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
	awsInstance := newAWSInstance(c.ec2, instance)
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
