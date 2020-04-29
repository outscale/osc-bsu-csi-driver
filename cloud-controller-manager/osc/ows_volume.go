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

// ********************* CCM Object Definitions *********************

// Used to represent a mount device for attaching an EBS volume
// This should be stored as a single letter (i.e. c, not sdc or /dev/sdc)
type mountDevice string

// VolumeOptions specifies capacity and tags for a volume.
type VolumeOptions struct {
	CapacityGB       int
	Tags             map[string]string
	VolumeType       string
	AvailabilityZone string
	// IOPSPerGB x CapacityGB will give total IOPS of the volume to create.
	// Calculated total IOPS will be capped at MaxTotalIOPS.
	IOPSPerGB int
	Encrypted bool
	// fully qualified resource name to the key to use for encryption.
	// example: arn:aws:kms:us-east-1:012345678910:key/abcd1234-a123-456a-a12b-a123b4cd56ef
	KmsKeyID string
}
