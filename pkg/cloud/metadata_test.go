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
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var (
	stdInstanceID       = "instance-1"
	stdInstanceType     = "t2.medium"
	stdRegion           = "az-1"
	stdAvailabilityZone = "az-1a"
)

func TestNewMetadataService(t *testing.T) {
	testCases := []struct {
		name             string
		isAvailable      bool
		identityDocument ec2metadata.EC2InstanceIdentityDocument
		err              error
	}{
		{
			name:        "success: normal",
			isAvailable: true,
			identityDocument: ec2metadata.EC2InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				InstanceType:     stdInstanceType,
				Region:           stdRegion,
				AvailabilityZone: stdAvailabilityZone,
			},
			err: nil,
		},
		{
			name:        "fail: metadata not available",
			isAvailable: false,
			identityDocument: ec2metadata.EC2InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				InstanceType:     stdInstanceType,
				Region:           stdRegion,
				AvailabilityZone: stdAvailabilityZone,
			},
			err: nil,
		},
		{
			name:        "fail: GetInstanceIdentityDocument returned error",
			isAvailable: true,
			identityDocument: ec2metadata.EC2InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				InstanceType:     stdInstanceType,
				Region:           stdRegion,
				AvailabilityZone: stdAvailabilityZone,
			},
			err: fmt.Errorf(""),
		},
		{
			name:        "fail: GetInstanceIdentityDocument returned empty instance",
			isAvailable: true,
			identityDocument: ec2metadata.EC2InstanceIdentityDocument{
				InstanceID:       "",
				InstanceType:     stdInstanceType,
				Region:           stdRegion,
				AvailabilityZone: stdAvailabilityZone,
			},
			err: fmt.Errorf("Empty content"),
		},
		{
			name:        "fail: GetInstanceIdentityDocument returned empty region",
			isAvailable: true,
			identityDocument: ec2metadata.EC2InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				InstanceType:     stdInstanceType,
				Region:           "",
				AvailabilityZone: stdAvailabilityZone,
			},
			err: fmt.Errorf("Empty content"),
		},
		{
			name:        "fail: GetInstanceIdentityDocument returned empty az",
			isAvailable: true,
			identityDocument: ec2metadata.EC2InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				InstanceType:     stdInstanceType,
				Region:           stdRegion,
				AvailabilityZone: "",
			},
			err: fmt.Errorf("Empty content"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockEC2Metadata := mocks.NewMockEC2Metadata(mockCtrl)

			mockEC2Metadata.EXPECT().Available().Return(tc.isAvailable)
			if tc.isAvailable {
				mockEC2Metadata.EXPECT().GetMetadata(gomock.Eq("instance-id")).Return(tc.identityDocument.InstanceID, tc.err)
				if tc.err == nil {
					mockEC2Metadata.EXPECT().GetMetadata(gomock.Eq("instance-type")).Return(tc.identityDocument.InstanceType, tc.err)
					mockEC2Metadata.EXPECT().GetMetadata(gomock.Eq("placement/availability-zone")).Return(tc.identityDocument.AvailabilityZone, tc.err)
				}
			}

			m, err := NewMetadataService(mockEC2Metadata)
			if tc.isAvailable && tc.err == nil {
				require.NoError(t, err)
				assert.Equal(t, tc.identityDocument.InstanceID, m.GetInstanceID())
				assert.Equal(t, tc.identityDocument.InstanceType, m.GetInstanceType())
				assert.Equal(t, tc.identityDocument.Region, m.GetRegion())
				assert.Equal(t, tc.identityDocument.AvailabilityZone, m.GetAvailabilityZone())
			} else {
				require.Error(t, err)
			}

			mockCtrl.Finish()
		})
	}
}
