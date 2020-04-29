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
	"log"
)

// MetadataService represents AWS metadata service.
type MetadataService interface {
	GetInstanceID() string
	GetInstanceType() string
	GetRegion() string
	GetAvailabilityZone() string
}

// Metadata represents OSC metadata data.
type Metadata struct {
	InstanceID       string
	InstanceType     string
	Region           string
	AvailabilityZone string
}

var _ MetadataService = &Metadata{}

// GetInstanceID returns the instance identification.
func (m *Metadata) GetInstanceID() string {
	return m.InstanceID
}

// GetInstanceType returns the instance type.
func (m *Metadata) GetInstanceType() string {
	return m.InstanceType
}

// GetRegion returns the region which the instance is in.
func (m *Metadata) GetRegion() string {
	return m.Region
}

// GetAvailabilityZone returns the Availability Zone which the instance is in.
func (m *Metadata) GetAvailabilityZone() string {
	return m.AvailabilityZone
}

// NewMetadataService returns a new MetadataServiceImplementation.
func NewMetadataService(svc EC2Metadata) (MetadataService, error) {

	if !svc.Available() {
		return nil, fmt.Errorf("EC2 instance metadata is not available")
	}

	instanceID, err := svc.GetMetadata("instance-id")
	if err != nil || len(instanceID) == 0 {
		return nil, fmt.Errorf("could not get valid EC2 instance ID")
	}
	instanceType, err := svc.GetMetadata("instance-type")
	if err != nil || len(instanceType) == 0 {
		return nil, fmt.Errorf("could not get valid EC2 instance type")
	}

	availabilityZone, err := svc.GetMetadata("placement/availability-zone")
	if err != nil || len(availabilityZone) == 0 {
		return nil, fmt.Errorf("could not get valid EC2 availavility zone")
	}
	region := availabilityZone[0 : len(availabilityZone)-1]
	log.Println("region : ", region)
	if len(region) == 0 {
		return nil, fmt.Errorf("could not get valid EC2 region")
	}

	return &Metadata{
		InstanceID:       instanceID,
		InstanceType:     instanceType,
		Region:           region,
		AvailabilityZone: availabilityZone,
	}, nil
}
