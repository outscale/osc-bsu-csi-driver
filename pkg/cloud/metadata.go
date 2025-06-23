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

package cloud

import (
	"bufio"
	"errors"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
)

type EC2Metadata interface {
	Available() bool
	GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error)
	GetMetadata(p string) (string, error)
}

// MetadataService represents AWS metadata service.
type MetadataService interface {
	GetInstanceID() string
	GetInstanceType() string
	GetRegion() string
	GetAvailabilityZone() string
	GetMountedDevices() []string
}

type Metadata struct {
	InstanceID       string
	InstanceType     string
	Region           string
	AvailabilityZone string
	MountedDevices   []string
}

var _ MetadataService = &Metadata{}

// GetInstanceID returns the instance identification.
func (m *Metadata) GetInstanceID() string {
	return m.InstanceID
}

// GetInstanceID returns the instance type.
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

// GetMountedDevices returns the list of volume devices (/dev/xxx).
func (m *Metadata) GetMountedDevices() []string {
	return m.MountedDevices
}

func NewMetadata() (MetadataService, error) {
	sess := session.Must(session.NewSession(&aws.Config{
		EndpointResolver: util.OscSetupMetadataResolver(),
	}))
	svc := ec2metadata.New(sess)
	return NewMetadataService(svc)
}

// NewMetadataService returns a new MetadataServiceImplementation.
func NewMetadataService(svc EC2Metadata) (MetadataService, error) {
	if !svc.Available() {
		return nil, errors.New("EC2 instance metadata is not available")
	}

	instanceID, err := svc.GetMetadata("instance-id")
	if err != nil || instanceID == "" {
		return nil, errors.New("could not get valid EC2 instance ID")
	}
	instanceType, err := svc.GetMetadata("instance-type")
	if err != nil || instanceType == "" {
		return nil, errors.New("could not get valid EC2 instance type")
	}

	availabilityZone, err := svc.GetMetadata("placement/availability-zone")
	if err != nil || availabilityZone == "" {
		return nil, errors.New("could not get valid EC2 availavility zone")
	}
	region := availabilityZone[0 : len(availabilityZone)-1]
	if len(region) == 0 {
		return nil, errors.New("could not get valid EC2 region")
	}

	var devices []string
	volumes, err := svc.GetMetadata("block-device-mapping/")
	if err != nil || volumes == "" {
		return nil, errors.New("could not get valid EC2 availavility zone")
	}
	scanner := bufio.NewScanner(strings.NewReader(volumes))
	for scanner.Scan() {
		dev, err := svc.GetMetadata("block-device-mapping/" + scanner.Text())
		if err != nil || volumes == "" {
			return nil, errors.New("could not get valid EC2 availavility zone")
		}
		// the root volume appears twice
		if !slices.Contains(devices, dev) {
			devices = append(devices, dev)
		}
	}
	return &Metadata{
		InstanceID:       instanceID,
		InstanceType:     instanceType,
		Region:           region,
		AvailabilityZone: availabilityZone,
		MountedDevices:   devices,
	}, nil
}
