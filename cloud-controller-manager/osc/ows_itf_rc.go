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
	"github.com/aws/aws-sdk-go/aws/endpoints"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
)

// ********************* CCM Object Interface *********************

// Volumes is an interface for managing cloud-provisioned volumes
// TODO: Allow other clouds to implement this
type Volumes interface {
	// Attach the disk to the node with the specified NodeName
	// nodeName can be empty to mean "the instance on which we are running"
	// Returns the device (e.g. /dev/xvdf) where we attached the volume
	AttachDisk(diskName KubernetesVolumeID, nodeName types.NodeName) (string, error)
	// Detach the disk from the node with the specified NodeName
	// nodeName can be empty to mean "the instance on which we are running"
	// Returns the device where the volume was attached
	DetachDisk(diskName KubernetesVolumeID, nodeName types.NodeName) (string, error)

	// Create a volume with the specified options
	CreateDisk(volumeOptions *VolumeOptions) (volumeName KubernetesVolumeID, err error)
	// Delete the specified volume
	// Returns true iff the volume was deleted
	// If the was not found, returns (false, nil)
	DeleteDisk(volumeName KubernetesVolumeID) (bool, error)

	// Get labels to apply to volume on creation
	GetVolumeLabels(volumeName KubernetesVolumeID) (map[string]string, error)

	// Get volume's disk path from volume name
	// return the device path where the volume is attached
	GetDiskPath(volumeName KubernetesVolumeID) (string, error)

	// Check if the volume is already attached to the node with the specified NodeName
	DiskIsAttached(diskName KubernetesVolumeID, nodeName types.NodeName) (bool, error)

	// Check if disks specified in argument map are still attached to their respective nodes.
	DisksAreAttached(map[types.NodeName][]KubernetesVolumeID) (map[types.NodeName]map[KubernetesVolumeID]bool, error)

	// Expand the disk to new size
	ResizeDisk(diskName KubernetesVolumeID, oldSize resource.Quantity, newSize resource.Quantity) (resource.Quantity, error)
}

// Interface to make the CloudConfig immutable for awsSDKProvider
type awsCloudConfigProvider interface {
	getResolver() endpoints.ResolverFunc
}
