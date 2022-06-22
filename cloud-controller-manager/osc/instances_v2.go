/*
Copyright 2020 The Kubernetes Authors.
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
	"os"
	"strings"

	"k8s.io/klog/v2"

	"github.com/outscale/osc-sdk-go/v2"

	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
)

// newInstances returns an implementation of cloudprovider.InstancesV2
func newInstancesV2(az string) (cloudprovider.InstancesV2, error) {

	region, err := azToRegion(az)
	if err != nil {
		return nil, err
	}
	config := osc.NewConfiguration()
	config.Debug = false
	client := osc.NewAPIClient(config)
	ctx := context.WithValue(context.Background(), osc.ContextAWSv4, osc.AWSv4{
		AccessKey: os.Getenv("OSC_ACCESS_KEY"),
		SecretKey: os.Getenv("OSC_SECRET_KEY"),
	})
	ctx = context.WithValue(ctx, osc.ContextServerIndex, 0)
	ctx = context.WithValue(ctx, osc.ContextServerVariables, map[string]string{"region": region})

	return &instancesV2{
		availabilityZone: az,
		region:           region,
		client:           client,
		ctx:              ctx,
	}, nil
}

// instances is an implementation of cloudprovider.InstancesV2
type instancesV2 struct {
	availabilityZone string
	client           *osc.APIClient
	ctx              context.Context
	region           string
}

// InstanceExists indicates whether a given node exists according to the cloud provider
func (i *instancesV2) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	_, err := i.getInstance(ctx, node)

	if err == cloudprovider.InstanceNotFound {
		klog.V(6).Infof("instance not found for node: %s", node.Name)
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// InstanceShutdown returns true if the instance is shutdown according to the cloud provider.
func (i *instancesV2) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	ec2Instance, err := i.getInstance(ctx, node)
	if err != nil {
		return false, err
	}

	if ec2Instance.State != nil {
		state := ec2Instance.GetState()
		// valid state for detaching volumes
		if state == "stopped" {
			return true, nil
		}
	}

	return false, nil
}

// InstanceMetadata returns the instance's metadata.
func (i *instancesV2) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	var err error
	var oscInstance *osc.Vm

	//  TODO: support node name policy other than private DNS names
	oscInstance, err = i.getInstance(ctx, node)
	if err != nil {
		return nil, err
	}

	nodeAddresses, err := extractNodeAddresses(oscInstance)
	if err != nil {
		return nil, err
	}

	providerID, err := getInstanceProviderIDV2(oscInstance)
	if err != nil {
		return nil, err
	}

	metadata := &cloudprovider.InstanceMetadata{
		ProviderID:    providerID,
		InstanceType:  oscInstance.GetVmType(),
		NodeAddresses: nodeAddresses,
	}

	return metadata, nil
}

// getInstance returns the instance if the instance with the given node info still exists.
// If false an error will be returned, the instance will be immediately deleted by the cloud controller manager.
func (i *instancesV2) getInstance(ctx context.Context, node *v1.Node) (*osc.Vm, error) {
	var request *osc.ReadVmsRequest
	if node.Spec.ProviderID == "" {
		// get Instance by private DNS name
		request = &osc.ReadVmsRequest{}
		/*
			Filters: []*ec2.Filter{
				newEc2Filter("private-dns-name", node.Name),
			},
		}*/
		klog.V(4).Infof("looking for node by private DNS name %v", node.Name)
	} else {
		// get Instance by provider ID
		instanceID, err := parseInstanceIDFromProviderIDV2(node.Spec.ProviderID)
		if err != nil {
			return nil, err
		}

		request = &osc.ReadVmsRequest{
			Filters: &osc.FiltersVm{
				VmIds: &[]string{instanceID},
			},
		}
		klog.V(4).Infof("looking for node by provider ID %v", node.Spec.ProviderID)
	}

	response, httpRes, err := i.client.VmApi.ReadVms(ctx).ReadVmsRequest(*request).Execute()
	klog.V(4).Infof("Get Response from Describe Instances  %v", response)

	if err != nil {
		if httpRes != nil {
			return nil, fmt.Errorf("error describing ec2 instances: %v (Status:%v)", err, httpRes.Status)
		}
		return nil, fmt.Errorf("error describing ec2 instances: %v", err)
	}

	if !response.HasVms() {
		return nil, fmt.Errorf("error describing ec2 instances: %v", err)
	}

	instances := []osc.Vm{}

	if node.Spec.ProviderID == "" {
		for _, instance := range response.GetVms() {
			if instance.GetPrivateDnsName() == node.Name {
				instances = append(instances, instance)
			}
		}
	} else {
		instances = response.GetVms()
	}

	if len(instances) == 0 {
		return nil, cloudprovider.InstanceNotFound
	}

	if len(instances) > 1 {
		return nil, errors.New("getInstance: multiple instances found")
	}

	state := instances[0].State
	if *state == "terminated" {
		return nil, cloudprovider.InstanceNotFound
	}

	return &instances[0], nil
}

// getInstanceProviderID returns the provider ID of an instance which is ultimately set in the node.Spec.ProviderID field.
// The well-known format for a node's providerID is:
//    * aws:///<availability-zone>/<instance-id>
func getInstanceProviderIDV2(instance *osc.Vm) (string, error) {
	if instance.Placement.GetSubregionName() == "" {
		return "", errors.New("instance availability zone was not set")
	}

	if instance.GetVmId() == "" {
		return "", errors.New("instance ID was not set")
	}

	// TODO: Check the impact of changing this ?
	return "aws:///" + instance.Placement.GetSubregionName() + "/" + instance.GetVmId(), nil
}

// parseInstanceIDFromProviderID parses the node's instance ID based on the following formats:
//   * aws://<availability-zone>/<instance-id>
//   * aws:///<instance-id>
//   * <instance-id>
// This function always assumes a valid providerID format was provided.
func parseInstanceIDFromProviderIDV2(providerID string) (string, error) {
	// trim the provider name prefix 'aws://', renaming providerID should contain metadata in one of the following formats:
	// * <availability-zone>/<instance-id>
	// * /<availability-zone>/<instance-id>
	// * <instance-id>
	instanceID := ""
	metadata := strings.Split(strings.TrimPrefix(providerID, "aws://"), "/")
	if len(metadata) == 1 {
		// instance-id
		instanceID = metadata[0]
	} else if len(metadata) == 2 {
		// az/instance-id
		instanceID = metadata[1]
	} else if len(metadata) == 3 {
		// /az/instance-id
		instanceID = metadata[2]
	}

	return instanceID, nil
}
