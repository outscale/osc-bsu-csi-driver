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

package driver

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud"
	"github.com/outscale/osc-bsu-csi-driver/pkg/driver/mocks"
	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	"github.com/outscale/osc-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/ptr"
)

const (
	expZone       = "us-west-2b"
	expInstanceID = "i-123456789abcdef01"
)

func TestNewControllerService(t *testing.T) {
	var (
		cloudObj   cloud.Cloud
		testErr    = errors.New("test error")
		testRegion = "test-region"

		getNewCloudFunc = func(expectedRegion string) func(region string, opts ...cloud.CloudOption) (cloud.Cloud, error) {
			return func(region string, opts ...cloud.CloudOption) (cloud.Cloud, error) {
				if region != expectedRegion {
					t.Fatalf("expected region %q but got %q", expectedRegion, region)
				}
				return cloudObj, nil
			}
		}
	)

	testCases := []struct {
		name                  string
		awsRegion, oscRegion  string
		newCloudFunc          func(string, ...cloud.CloudOption) (cloud.Cloud, error)
		newMetadataFuncErrors bool
		expectPanic           bool
	}{
		{
			name:         "AWS_REGION variable set, newCloud does not error",
			awsRegion:    "foo",
			newCloudFunc: getNewCloudFunc("foo"),
		},
		{
			name:         "OSC_REGION variable set, newCloud does not error",
			oscRegion:    "foo",
			newCloudFunc: getNewCloudFunc("foo"),
		},
		{
			name:      "OSC_REGION variable set, newCloud errors",
			oscRegion: "foo",
			newCloudFunc: func(region string, opts ...cloud.CloudOption) (cloud.Cloud, error) {
				return nil, testErr
			},
			expectPanic: true,
		},
		{
			name:         "AWS_REGION/OSC_REGION variable not set, newMetadata does not error",
			newCloudFunc: getNewCloudFunc(testRegion),
		},
		{
			name:                  "AWS_REGION/OSC_REGION variable not set, newMetadata errors",
			newCloudFunc:          getNewCloudFunc(testRegion),
			newMetadataFuncErrors: true,
			expectPanic:           true,
		},
	}

	driverOptions := &DriverOptions{
		endpoint: "test",
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oldNewCloudFunc := NewCloudFunc
			defer func() { NewCloudFunc = oldNewCloudFunc }()
			NewCloudFunc = tc.newCloudFunc

			t.Setenv("AWS_REGION", tc.awsRegion)
			t.Setenv("OSC_REGION", tc.oscRegion)
			if tc.awsRegion == "" && tc.oscRegion == "" {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()
				mockMetadataService := mocks.NewMockMetadataService(mockCtl)

				oldNewMetadataFunc := NewMetadataFunc
				defer func() { NewMetadataFunc = oldNewMetadataFunc }()
				NewMetadataFunc = func() (cloud.MetadataService, error) {
					if tc.newMetadataFuncErrors {
						return nil, testErr
					}
					return mockMetadataService, nil
				}

				if !tc.newMetadataFuncErrors {
					mockMetadataService.EXPECT().GetRegion().Return(testRegion)
				}
			}

			if tc.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("The code did not panic")
					}
				}()
			}

			controllerService := newControllerService(driverOptions)

			if controllerService.cloud != cloudObj {
				t.Fatalf("expected cloud attribute to be equal to instantiated cloud object")
			}
			if !reflect.DeepEqual(controllerService.driverOptions, driverOptions) {
				t.Fatalf("expected driverOptions attribute to be equal to input")
			}
		})
	}
}

func TestCreateVolume(t *testing.T) {
	stdVolCap := []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}
	stdVolSize := int64(5 * 1024 * 1024 * 1024)
	stdCapRange := &csi.CapacityRange{RequiredBytes: stdVolSize}
	stdParams := map[string]string{}

	t.Run("success normal", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "random-vol-name",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         nil,
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
	})
	t.Run("restore snapshot", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "random-vol-name",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         nil,
			VolumeContentSource: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: "snapshot-id",
					},
				},
			},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
			SnapshotID:       "snapshot-id",
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		rsp, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
		snapshotID := ""
		if rsp.Volume != nil && rsp.Volume.ContentSource != nil && rsp.Volume.ContentSource.GetSnapshot() != nil {
			snapshotID = rsp.Volume.ContentSource.GetSnapshot().SnapshotId
		}
		if rsp.Volume.ContentSource.GetSnapshot().SnapshotId != "snapshot-id" {
			t.Errorf("Unexpected snapshot ID: %q", snapshotID)
		}
	})
	t.Run("restore snapshot, volume already exists", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "random-vol-name",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         nil,
			VolumeContentSource: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: "snapshot-id",
					},
				},
			},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
			SnapshotID:       "snapshot-id",
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		rsp, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
		snapshotID := ""
		if rsp.Volume != nil && rsp.Volume.ContentSource != nil && rsp.Volume.ContentSource.GetSnapshot() != nil {
			snapshotID = rsp.Volume.ContentSource.GetSnapshot().SnapshotId
		}
		if rsp.Volume.ContentSource.GetSnapshot().SnapshotId != "snapshot-id" {
			t.Errorf("Unexpected snapshot ID: %q", snapshotID)
		}
	})
	t.Run("restore snapshot, volume already exists with different snapshot ID", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "random-vol-name",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         nil,
			VolumeContentSource: &csi.VolumeContentSource{
				Type: &csi.VolumeContentSource_Snapshot{
					Snapshot: &csi.VolumeContentSource_SnapshotSource{
						SnapshotId: "snapshot-id",
					},
				},
			},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
			SnapshotID:       "another-snapshot-id",
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.CreateVolume(ctx, req)
		require.Error(t, err)
		status, _ := status.FromError(err)
		require.NotNil(t, status)
		assert.Equal(t, codes.AlreadyExists, status.Code())
	})
	t.Run("fail no name", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         stdParams,
		}

		ctx := context.Background()

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.CreateVolume(ctx, req)
		require.Error(t, err)
		status, _ := status.FromError(err)
		assert.Equal(t, codes.InvalidArgument, status.Code())
	})
	t.Run("success same name and same capacity", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "test-vol",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         stdParams,
		}
		extraReq := &csi.CreateVolumeRequest{
			Name:               "test-vol",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         stdParams,
		}
		expVol := &csi.Volume{
			CapacityBytes: stdVolSize,
			VolumeId:      "test-vol",
			VolumeContext: map[string]string{},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)

		// Subsequent call returns the created disk
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(mockDisk, nil)
		resp, err := oscDriver.CreateVolume(ctx, extraReq)
		require.NoError(t, err)
		vol := resp.GetVolume()
		require.NotNil(t, vol)

		assert.Equal(t, expVol.GetCapacityBytes(), vol.GetCapacityBytes())
		assert.Equal(t, expVol.GetVolumeId(), vol.GetVolumeId())
		assert.Equal(t, expVol.GetVolumeContext(), vol.GetVolumeContext())
	})
	t.Run("fail same name and different capacity", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "test-vol",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         stdParams,
		}
		extraReq := &csi.CreateVolumeRequest{
			Name:               "test-vol",
			CapacityRange:      &csi.CapacityRange{RequiredBytes: 10000},
			VolumeCapabilities: stdVolCap,
			Parameters:         stdParams,
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
		}
		volSizeBytes, err := getVolSizeBytes(req)
		require.NoError(t, err)
		mockDisk.CapacityGiB = util.BytesToGiB(volSizeBytes)

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(volSizeBytes)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err = oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
		extraVolSizeBytes, err := getVolSizeBytes(extraReq)
		require.NoError(t, err)

		// Subsequent failure
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(extraReq.Name), gomock.Eq(extraVolSizeBytes)).Return(cloud.Disk{}, cloud.ErrDiskExistsDiffSize)
		_, err = oscDriver.CreateVolume(ctx, extraReq)
		require.Error(t, err)
		status, _ := status.FromError(err)
		require.NotNil(t, status)
		assert.Equal(t, codes.AlreadyExists, status.Code())
	})
	t.Run("success no capacity range", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "test-vol",
			VolumeCapabilities: stdVolCap,
			Parameters:         stdParams,
		}
		expVol := &csi.Volume{
			CapacityBytes: cloud.DefaultVolumeSize,
			VolumeId:      "vol-test",
			VolumeContext: map[string]string{},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(cloud.DefaultVolumeSize),
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(cloud.DefaultVolumeSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		resp, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
		vol := resp.GetVolume()
		require.NotNil(t, vol)
		assert.Equal(t, expVol.GetCapacityBytes(), vol.GetCapacityBytes())
		assert.Equal(t, expVol.GetVolumeContext(), vol.GetVolumeContext())
	})
	t.Run("success with correct round up", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "vol-test",
			CapacityRange:      &csi.CapacityRange{RequiredBytes: 1073741825},
			VolumeCapabilities: stdVolCap,
			Parameters:         nil,
		}
		expVol := &csi.Volume{
			CapacityBytes: 2147483648, // 1 GiB + 1 byte = 2 GiB
			VolumeId:      "vol-test",
			VolumeContext: map[string]string{},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(expVol.CapacityBytes),
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(expVol.CapacityBytes)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		resp, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
		vol := resp.GetVolume()
		require.NotNil(t, vol)

		if vol.GetCapacityBytes() != expVol.GetCapacityBytes() {
			t.Fatalf("Expected volume capacity bytes: %v, got: %v", expVol.GetCapacityBytes(), vol.GetCapacityBytes())
		}
	})
	t.Run("success with volume type io1", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "vol-test",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters: map[string]string{
				VolumeTypeKey: cloud.VolumeTypeIO1,
				IopsPerGBKey:  "5",
			},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
	})
	t.Run("success with volume type sc1", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "vol-test",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters: map[string]string{
				VolumeTypeKey: cloud.VolumeTypeIO1,
			},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
	})
	t.Run("success with volume type standard", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "vol-test",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters: map[string]string{
				VolumeTypeKey: cloud.VolumeTypeSTANDARD,
			},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
	})
	t.Run("success with volume encryption", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "vol-test",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters: map[string]string{
				EncryptedKey: "true",
			},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		volumeResponse, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, "true", volumeResponse.GetVolume().VolumeContext[EncryptedKey])
	})
	t.Run("success with volume encryption", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "vol-test",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters: map[string]string{
				EncryptedKey: "true",
			},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		volumeResponse, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, "true", volumeResponse.GetVolume().VolumeContext[EncryptedKey])
		assert.Equal(t, "", volumeResponse.GetVolume().VolumeContext[LuksCipherKey])
		assert.Equal(t, "", volumeResponse.GetVolume().VolumeContext[LuksHashKey])
		assert.Equal(t, "", volumeResponse.GetVolume().VolumeContext[LuksKeySizeKey])
	})
	t.Run("success with volume encryption with parameters", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "vol-test",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters: map[string]string{
				EncryptedKey:   "true",
				LuksCipherKey:  "cipher",
				LuksHashKey:    "hash",
				LuksKeySizeKey: "keysize",
			},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		volumeResponse, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, "true", volumeResponse.GetVolume().VolumeContext[EncryptedKey])
		assert.Equal(t, "cipher", volumeResponse.GetVolume().VolumeContext[LuksCipherKey])
		assert.Equal(t, "hash", volumeResponse.GetVolume().VolumeContext[LuksHashKey])
		assert.Equal(t, "keysize", volumeResponse.GetVolume().VolumeContext[LuksKeySizeKey])
	})
	t.Run("fail with invalid volume parameter", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "vol-test",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters: map[string]string{
				VolumeTypeKey: cloud.VolumeTypeIO1,
				"unknownKey":  "unknownValue",
			},
		}

		ctx := context.Background()

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.CreateVolume(ctx, req)
		require.Error(t, err)
		status, _ := status.FromError(err)
		require.NotNil(t, status)
		assert.Equal(t, codes.InvalidArgument, status.Code())
	})
	t.Run("success when volume exists and contains VolumeContext and AccessibleTopology", func(t *testing.T) {
		req := &csi.CreateVolumeRequest{
			Name:               "test-vol",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         map[string]string{},
			AccessibilityRequirements: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{TopologyKey: expZone},
					},
				},
			},
		}
		extraReq := &csi.CreateVolumeRequest{
			Name:               "test-vol",
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         map[string]string{},
			AccessibilityRequirements: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{TopologyKey: expZone},
					},
				},
			},
		}
		expVol := &csi.Volume{
			CapacityBytes: stdVolSize,
			VolumeId:      "vol-test",
			VolumeContext: map[string]string{},
			AccessibleTopology: []*csi.Topology{
				{
					Segments: map[string]string{TopologyKey: expZone},
				},
			},
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Any()).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)

		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(mockDisk, nil)
		resp, err := oscDriver.CreateVolume(ctx, extraReq)
		require.NoError(t, err)
		vol := resp.GetVolume()
		require.NotNil(t, vol)

		assert.Equal(t, expVol.GetVolumeContext(), vol.GetVolumeContext())

		if expVol.GetAccessibleTopology() != nil {
			assert.Equal(t, expVol.GetAccessibleTopology(), vol.GetAccessibleTopology())
		}
	})
	t.Run("success with extra tags", func(t *testing.T) {
		const (
			volumeName          = "random-vol-name"
			extraVolumeTagKey   = "extra-tag-key"
			extraVolumeTagValue = "extra-tag-value"
		)
		req := &csi.CreateVolumeRequest{
			Name:               volumeName,
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         nil,
		}

		ctx := context.Background()

		mockDisk := cloud.Disk{
			VolumeID:         req.Name,
			AvailabilityZone: expZone,
			CapacityGiB:      util.BytesToGiB(stdVolSize),
		}

		diskOptions := &cloud.DiskOptions{
			CapacityBytes: stdVolSize,
			Tags: map[string]string{
				cloud.VolumeNameTagKey: volumeName,
				extraVolumeTagKey:      extraVolumeTagValue,
			},
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(diskOptions)).Return(mockDisk, nil)

		oscDriver := controllerService{
			cloud: mockCloud,
			driverOptions: &DriverOptions{
				extraVolumeTags: map[string]string{
					extraVolumeTagKey: extraVolumeTagValue,
				},
			},
		}

		_, err := oscDriver.CreateVolume(ctx, req)
		require.NoError(t, err)
	})
	t.Run("out of quota failure", func(t *testing.T) {
		const (
			volumeName          = "random-vol-name"
			extraVolumeTagKey   = "extra-tag-key"
			extraVolumeTagValue = "extra-tag-value"
		)
		req := &csi.CreateVolumeRequest{
			Name:               volumeName,
			CapacityRange:      stdCapRange,
			VolumeCapabilities: stdVolCap,
			Parameters:         nil,
		}

		ctx := context.Background()

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetDiskByName(gomock.Any(), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(cloud.Disk{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateDisk(gomock.Any(), gomock.Eq(req.Name), gomock.Any()).
			Return(cloud.Disk{}, cloud.NewOAPIError(osc.Errors{Code: ptr.To("10018"), Type: ptr.To("TooManyResources (QuotaExceeded)")}))

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.CreateVolume(ctx, req)
		require.Error(t, err)
		status, ok := status.FromError(err)
		require.True(t, ok)
		require.NotNil(t, status)
		assert.Equal(t, codes.ResourceExhausted, status.Code())
	})
}

func TestDeleteVolume(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
				req := &csi.DeleteVolumeRequest{
					VolumeId: "vol-test",
				}
				expResp := &csi.DeleteVolumeResponse{}

				ctx := context.Background()
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().DeleteDisk(gomock.Eq(ctx), gomock.Eq(req.VolumeId)).Return(true, nil)
				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}
				resp, err := oscDriver.DeleteVolume(ctx, req)
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}
				if !reflect.DeepEqual(resp, expResp) {
					t.Fatalf("Expected resp to be %+v, got: %+v", expResp, resp)
				}
			},
		},
		{
			name: "success invalid volume id",
			testFunc: func(t *testing.T) {
				req := &csi.DeleteVolumeRequest{
					VolumeId: "invalid-volume-name",
				}
				expResp := &csi.DeleteVolumeResponse{}

				ctx := context.Background()
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().DeleteDisk(gomock.Eq(ctx), gomock.Eq(req.VolumeId)).Return(false, cloud.ErrNotFound)
				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}
				resp, err := oscDriver.DeleteVolume(ctx, req)
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}
				if !reflect.DeepEqual(resp, expResp) {
					t.Fatalf("Expected resp to be %+v, got: %+v", expResp, resp)
				}
			},
		},
		{
			name: "fail delete disk",
			testFunc: func(t *testing.T) {
				req := &csi.DeleteVolumeRequest{
					VolumeId: "test-vol",
				}

				ctx := context.Background()
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().DeleteDisk(gomock.Eq(ctx), gomock.Eq(req.VolumeId)).Return(false, fmt.Errorf("DeleteDisk could not delete volume"))
				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}
				resp, err := oscDriver.DeleteVolume(ctx, req)
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.Internal {
						t.Fatalf("Unexpected error: %v", srvErr.Code())
					}
				} else {
					t.Fatalf("Expected error, got nil")
				}

				if resp != nil {
					t.Fatalf("Expected resp to be nil, got: %+v", resp)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestPickAvailabilityZone(t *testing.T) {
	testCases := []struct {
		name        string
		requirement *csi.TopologyRequirement
		expZone     string
	}{
		{
			name: "Pick from preferred",
			requirement: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{TopologyKey: expZone},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{TopologyKey: expZone},
					},
				},
			},
			expZone: expZone,
		},
		{
			name: "Pick from requisite",
			requirement: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{TopologyKey: expZone},
					},
				},
			},
			expZone: expZone,
		},
		{
			name: "Pick from requisite topologyK8sKey",
			requirement: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{TopologyK8sKey: expZone},
					},
				},
			},
			expZone: expZone,
		},
		{
			name: "Pick from multi requisites",
			requirement: &csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{TopologyKey: expZone, TopologyK8sKey: expZone},
					},
				},
			},
			expZone: expZone,
		},
		{
			name: "Pick from empty topology",
			requirement: &csi.TopologyRequirement{
				Preferred: []*csi.Topology{{}},
				Requisite: []*csi.Topology{{}},
			},
			expZone: "",
		},
		{
			name:        "Topology Requirement is nil",
			requirement: nil,
			expZone:     "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := pickAvailabilityZone(tc.requirement)
			assert.Equal(t, tc.expZone, actual)
		})
	}
}

func TestCreateSnapshot(t *testing.T) {
	t.Run("success normal", func(t *testing.T) {
		req := &csi.CreateSnapshotRequest{
			Name:           "test-snapshot",
			Parameters:     nil,
			SourceVolumeId: "vol-test",
		}

		ctx := context.Background()
		mockSnapshot := cloud.Snapshot{
			SnapshotID:     fmt.Sprintf("snapshot-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Uint64()),
			SourceVolumeID: req.SourceVolumeId,
			Size:           1,
			CreationTime:   time.Now(),
			State:          "completed",
		}
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetSnapshotByName(gomock.Eq(ctx), gomock.Eq(req.GetName())).Return(cloud.Snapshot{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateSnapshot(gomock.Eq(ctx), gomock.Eq(req.SourceVolumeId), gomock.Any()).Return(mockSnapshot, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}
		resp, err := oscDriver.CreateSnapshot(context.Background(), req)
		require.NoError(t, err)

		snap := resp.GetSnapshot()
		require.NotNil(t, snap)
		assert.True(t, snap.ReadyToUse)
	})
	t.Run("fail no name", func(t *testing.T) {
		req := &csi.CreateSnapshotRequest{
			Parameters:     nil,
			SourceVolumeId: "vol-test",
		}

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}
		_, err := oscDriver.CreateSnapshot(context.Background(), req)
		require.Error(t, err)
		srvErr, ok := status.FromError(err)
		assert.True(t, ok, "Should get an error status code")
		assert.Equal(t, codes.InvalidArgument, srvErr.Code())
	})
	t.Run("fail same name different volume ID", func(t *testing.T) {
		req := &csi.CreateSnapshotRequest{
			Name:           "test-snapshot",
			Parameters:     nil,
			SourceVolumeId: "vol-test",
		}
		extraReq := &csi.CreateSnapshotRequest{
			Name:           "test-snapshot",
			Parameters:     nil,
			SourceVolumeId: "vol-xxx",
		}

		ctx := context.Background()
		mockSnapshot := cloud.Snapshot{
			SnapshotID:     fmt.Sprintf("snapshot-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Uint64()),
			SourceVolumeID: req.SourceVolumeId,
			Size:           1,
			CreationTime:   time.Now(),
			State:          "completed",
		}
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetSnapshotByName(gomock.Eq(ctx), gomock.Eq(req.GetName())).Return(cloud.Snapshot{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateSnapshot(gomock.Eq(ctx), gomock.Eq(req.SourceVolumeId), gomock.Any()).Return(mockSnapshot, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}
		resp, err := oscDriver.CreateSnapshot(context.Background(), req)
		if err != nil {
			srvErr, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Could not get error status code from error: %v", srvErr)
			}
			if srvErr.Code() != codes.OK {
				t.Fatalf("Expected error code %d, got %d message %s", codes.OK, srvErr.Code(), srvErr.Message())
			}
			t.Fatalf("Unexpected error: %v", err)
		}
		snap := resp.GetSnapshot()
		require.NotNil(t, snap)
		assert.True(t, snap.ReadyToUse)

		mockCloud.EXPECT().GetSnapshotByName(gomock.Eq(ctx), gomock.Eq(extraReq.GetName())).Return(mockSnapshot, nil)
		_, err = oscDriver.CreateSnapshot(ctx, extraReq)
		if err != nil {
			srvErr, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Could not get error status code from error: %v", srvErr)
			}
			if srvErr.Code() != codes.AlreadyExists {
				t.Fatalf("Expected error code %d, got %d message %s", codes.AlreadyExists, srvErr.Code(), srvErr.Message())
			}
		} else {
			t.Fatalf("Expected error %v, got no error", codes.AlreadyExists)
		}
	})
	t.Run("success same name same volume ID", func(t *testing.T) {
		req := &csi.CreateSnapshotRequest{
			Name:           "test-snapshot",
			Parameters:     nil,
			SourceVolumeId: "vol-test",
		}
		extraReq := &csi.CreateSnapshotRequest{
			Name:           "test-snapshot",
			Parameters:     nil,
			SourceVolumeId: "vol-test",
		}

		ctx := context.Background()
		mockSnapshot := cloud.Snapshot{
			SnapshotID:     fmt.Sprintf("snapshot-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Uint64()),
			SourceVolumeID: req.SourceVolumeId,
			Size:           1,
			CreationTime:   time.Now(),
			State:          "completed",
		}
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetSnapshotByName(gomock.Eq(ctx), gomock.Eq(req.GetName())).Return(cloud.Snapshot{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateSnapshot(gomock.Eq(ctx), gomock.Eq(req.SourceVolumeId), gomock.Any()).Return(mockSnapshot, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}
		resp, err := oscDriver.CreateSnapshot(context.Background(), req)
		require.NoError(t, err)
		snap := resp.GetSnapshot()
		require.NotNil(t, snap)
		assert.True(t, snap.ReadyToUse)

		mockCloud.EXPECT().GetSnapshotByName(gomock.Eq(ctx), gomock.Eq(extraReq.GetName())).Return(mockSnapshot, nil)
		_, err = oscDriver.CreateSnapshot(ctx, extraReq)
		require.NoError(t, err)
	})
	t.Run("success with extra tags", func(t *testing.T) {
		req := &csi.CreateSnapshotRequest{
			Name:           "test-snapshot",
			Parameters:     nil,
			SourceVolumeId: "vol-test",
		}
		extraReq := &csi.CreateSnapshotRequest{
			Name:           "test-snapshot",
			Parameters:     nil,
			SourceVolumeId: "vol-test",
		}
		extraSnapshotTagKey := "foo"
		extraSnapshotTagValue := "bar"
		snapshotOptions := &cloud.SnapshotOptions{
			Tags: map[string]string{
				cloud.SnapshotNameTagKey: req.Name,
				extraSnapshotTagKey:      extraSnapshotTagValue,
			},
		}

		ctx := context.Background()
		mockSnapshot := cloud.Snapshot{
			SnapshotID:     fmt.Sprintf("snapshot-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Uint64()),
			SourceVolumeID: req.SourceVolumeId,
			Size:           1,
			CreationTime:   time.Now(),
			State:          "completed",
		}
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetSnapshotByName(gomock.Eq(ctx), gomock.Eq(req.GetName())).Return(cloud.Snapshot{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateSnapshot(gomock.Eq(ctx), gomock.Eq(req.SourceVolumeId), gomock.Eq(snapshotOptions)).Return(mockSnapshot, nil)

		oscDriver := controllerService{
			cloud: mockCloud,
			driverOptions: &DriverOptions{
				extraSnapshotTags: map[string]string{
					extraSnapshotTagKey: extraSnapshotTagValue,
				},
			},
		}
		resp, err := oscDriver.CreateSnapshot(context.Background(), req)
		require.NoError(t, err)
		snap := resp.GetSnapshot()
		require.NotNil(t, snap)
		assert.True(t, snap.ReadyToUse)

		mockCloud.EXPECT().GetSnapshotByName(gomock.Eq(ctx), gomock.Eq(extraReq.GetName())).Return(mockSnapshot, nil)
		_, err = oscDriver.CreateSnapshot(ctx, extraReq)
		require.NoError(t, err)
	})
	t.Run("snapshot in error return Resource exhausted", func(t *testing.T) {
		req := &csi.CreateSnapshotRequest{
			Name:           "test-snapshot",
			Parameters:     nil,
			SourceVolumeId: "vol-test",
		}
		extraSnapshotTagKey := "foo"
		extraSnapshotTagValue := "bar"
		snapshotOptions := &cloud.SnapshotOptions{
			Tags: map[string]string{
				cloud.SnapshotNameTagKey: req.Name,
				extraSnapshotTagKey:      extraSnapshotTagValue,
			},
		}

		ctx := context.Background()
		mockSnapshot := cloud.Snapshot{
			SnapshotID:     fmt.Sprintf("snapshot-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Uint64()),
			SourceVolumeID: req.SourceVolumeId,
			Size:           1,
			CreationTime:   time.Now(),
			State:          "error",
		}
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetSnapshotByName(gomock.Eq(ctx), gomock.Eq(req.GetName())).
			Return(cloud.Snapshot{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateSnapshot(gomock.Eq(ctx), gomock.Eq(req.SourceVolumeId), gomock.Eq(snapshotOptions)).Return(mockSnapshot, nil)

		oscDriver := controllerService{
			cloud: mockCloud,
			driverOptions: &DriverOptions{
				extraSnapshotTags: map[string]string{
					extraSnapshotTagKey: extraSnapshotTagValue,
				},
			},
		}
		_, err := oscDriver.CreateSnapshot(context.Background(), req)
		require.Error(t, err)
		status, _ := status.FromError(err)
		require.NotNil(t, status)
		assert.Equal(t, codes.ResourceExhausted, status.Code())
	})
	t.Run("quota errors return Resource exhausted", func(t *testing.T) {
		req := &csi.CreateSnapshotRequest{
			Name:           "test-snapshot",
			Parameters:     nil,
			SourceVolumeId: "vol-test",
		}
		extraSnapshotTagKey := "foo"
		extraSnapshotTagValue := "bar"
		snapshotOptions := &cloud.SnapshotOptions{
			Tags: map[string]string{
				cloud.SnapshotNameTagKey: req.Name,
				extraSnapshotTagKey:      extraSnapshotTagValue,
			},
		}

		ctx := context.Background()
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetSnapshotByName(gomock.Eq(ctx), gomock.Eq(req.GetName())).
			Return(cloud.Snapshot{}, cloud.ErrNotFound)
		mockCloud.EXPECT().CreateSnapshot(gomock.Eq(ctx), gomock.Eq(req.SourceVolumeId), gomock.Eq(snapshotOptions)).
			Return(cloud.Snapshot{}, cloud.NewOAPIError(osc.Errors{Code: ptr.To("10026"), Type: ptr.To("TooManyResources (QuotaExceeded)")}))

		oscDriver := controllerService{
			cloud: mockCloud,
			driverOptions: &DriverOptions{
				extraSnapshotTags: map[string]string{
					extraSnapshotTagKey: extraSnapshotTagValue,
				},
			},
		}
		_, err := oscDriver.CreateSnapshot(context.Background(), req)
		require.Error(t, err)
		status, _ := status.FromError(err)
		require.NotNil(t, status)
		assert.Equal(t, codes.ResourceExhausted, status.Code())
	})
	t.Run("snapshot in error return Resource exhausted (retry)", func(t *testing.T) {
		req := &csi.CreateSnapshotRequest{
			Name:           "test-snapshot",
			Parameters:     nil,
			SourceVolumeId: "vol-test",
		}
		extraSnapshotTagKey := "foo"
		extraSnapshotTagValue := "bar"

		ctx := context.Background()
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetSnapshotByName(gomock.Eq(ctx), gomock.Eq(req.GetName())).
			Return(cloud.Snapshot{SnapshotID: "snap_foo", State: "error"}, nil)

		oscDriver := controllerService{
			cloud: mockCloud,
			driverOptions: &DriverOptions{
				extraSnapshotTags: map[string]string{
					extraSnapshotTagKey: extraSnapshotTagValue,
				},
			},
		}
		_, err := oscDriver.CreateSnapshot(context.Background(), req)
		require.Error(t, err)
		status, _ := status.FromError(err)
		require.NotNil(t, status)
		assert.Equal(t, codes.ResourceExhausted, status.Code())
	})
}

func TestDeleteSnapshot(t *testing.T) {
	t.Run("success normal", func(t *testing.T) {
		ctx := context.Background()

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()
		mockCloud := mocks.NewMockCloud(mockCtl)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		req := &csi.DeleteSnapshotRequest{
			SnapshotId: "xxx",
		}

		mockCloud.EXPECT().DeleteSnapshot(gomock.Eq(ctx), gomock.Eq("xxx")).Return(true, nil)
		_, err := oscDriver.DeleteSnapshot(ctx, req)
		require.NoError(t, err)
	})
	t.Run("success not found", func(t *testing.T) {
		ctx := context.Background()

		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()
		mockCloud := mocks.NewMockCloud(mockCtl)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		req := &csi.DeleteSnapshotRequest{
			SnapshotId: "xxx",
		}

		mockCloud.EXPECT().DeleteSnapshot(gomock.Eq(ctx), gomock.Eq("xxx")).Return(false, cloud.ErrNotFound)
		_, err := oscDriver.DeleteSnapshot(ctx, req)
		require.NoError(t, err)
	})
}

func TestListSnapshots(t *testing.T) {
	t.Run("success normal", func(t *testing.T) {
		req := &csi.ListSnapshotsRequest{}
		mockCloudSnapshotsResponse := cloud.ListSnapshotsResponse{
			Snapshots: []cloud.Snapshot{
				{
					SnapshotID:     "snapshot-1",
					SourceVolumeID: "test-vol",
					Size:           1,
					CreationTime:   time.Now(),
				},
				{
					SnapshotID:     "snapshot-2",
					SourceVolumeID: "test-vol",
					Size:           1,
					CreationTime:   time.Now(),
				},
			},
			NextToken: "",
		}

		ctx := context.Background()
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().ListSnapshots(gomock.Eq(ctx), gomock.Eq(""), gomock.Eq(int32(0)), gomock.Eq("")).Return(mockCloudSnapshotsResponse, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		resp, err := oscDriver.ListSnapshots(context.Background(), req)
		require.NoError(t, err)
		assert.Len(t, resp.GetEntries(), len(mockCloudSnapshotsResponse.Snapshots))
	})
	t.Run("success no snapshots", func(t *testing.T) {
		req := &csi.ListSnapshotsRequest{}
		ctx := context.Background()
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().ListSnapshots(gomock.Eq(ctx), gomock.Eq(""), gomock.Eq(int32(0)), gomock.Eq("")).Return(cloud.ListSnapshotsResponse{}, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		resp, err := oscDriver.ListSnapshots(context.Background(), req)
		require.NoError(t, err)
		assert.Empty(t, resp.Entries)
	})
	t.Run("success with nextToken", func(t *testing.T) {
		req := &csi.ListSnapshotsRequest{
			StartingToken: "foo",
		}
		ctx := context.Background()
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().ListSnapshots(gomock.Eq(ctx), gomock.Eq(""), gomock.Eq(int32(0)), gomock.Eq("foo")).Return(cloud.ListSnapshotsResponse{
			Snapshots: []cloud.Snapshot{{}},
			NextToken: "bar",
		}, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		resp, err := oscDriver.ListSnapshots(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, "bar", resp.NextToken)
	})
	t.Run("invalid nextToken", func(t *testing.T) {
		req := &csi.ListSnapshotsRequest{
			StartingToken: "foo",
		}
		ctx := context.Background()
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().ListSnapshots(gomock.Eq(ctx), gomock.Eq(""), gomock.Eq(int32(0)), gomock.Eq("foo")).
			Return(cloud.ListSnapshotsResponse{}, cloud.NewOAPIError(osc.Errors{Code: ptr.To("4116")}))

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.ListSnapshots(context.Background(), req)
		require.Error(t, err)
		status, _ := status.FromError(err)
		require.NotNil(t, status)
		assert.Equal(t, codes.Aborted, status.Code())
	})
	t.Run("success snapshot ID", func(t *testing.T) {
		req := &csi.ListSnapshotsRequest{
			SnapshotId: "snapshot-1",
		}
		mockCloudSnapshotsResponse := cloud.Snapshot{
			SnapshotID:     "snapshot-1",
			SourceVolumeID: "test-vol",
			Size:           1,
			CreationTime:   time.Now(),
		}

		ctx := context.Background()
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetSnapshotByID(gomock.Eq(ctx), gomock.Eq("snapshot-1")).Return(mockCloudSnapshotsResponse, nil)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		resp, err := oscDriver.ListSnapshots(context.Background(), req)
		require.NoError(t, err)
		assert.Len(t, resp.GetEntries(), 1)
	})
	t.Run("success snapshot ID not found", func(t *testing.T) {
		req := &csi.ListSnapshotsRequest{
			SnapshotId: "snapshot-1",
		}

		ctx := context.Background()
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetSnapshotByID(gomock.Eq(ctx), gomock.Eq("snapshot-1")).Return(cloud.Snapshot{}, cloud.ErrNotFound)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		resp, err := oscDriver.ListSnapshots(context.Background(), req)
		require.NoError(t, err)
		assert.Empty(t, resp.GetEntries())
	})
	t.Run("fail snapshot ID multiple found", func(t *testing.T) {
		req := &csi.ListSnapshotsRequest{
			SnapshotId: "snapshot-1",
		}

		ctx := context.Background()
		mockCtl := gomock.NewController(t)
		defer mockCtl.Finish()

		mockCloud := mocks.NewMockCloud(mockCtl)
		mockCloud.EXPECT().GetSnapshotByID(gomock.Eq(ctx), gomock.Eq("snapshot-1")).Return(cloud.Snapshot{}, cloud.ErrMultiSnapshots)

		oscDriver := controllerService{
			cloud:         mockCloud,
			driverOptions: &DriverOptions{},
		}

		_, err := oscDriver.ListSnapshots(context.Background(), req)
		require.Error(t, err)
		st, _ := status.FromError(err)
		require.NotNil(t, st)
		assert.Equal(t, codes.Internal, st.Code())
	})
}

func TestControllerPublishVolume(t *testing.T) {
	stdVolCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	expDevicePath := "/dev/xvda"

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerPublishVolumeRequest{
					NodeId:           expInstanceID,
					VolumeCapability: stdVolCap,
					VolumeId:         "vol-test",
				}
				expResp := &csi.ControllerPublishVolumeResponse{
					PublishContext: map[string]string{DevicePathKey: expDevicePath},
				}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().IsExistInstance(gomock.Eq(ctx), gomock.Eq(req.NodeId)).Return(true)
				mockCloud.EXPECT().GetDiskByID(gomock.Eq(ctx), gomock.Any()).Return(cloud.Disk{}, nil)
				mockCloud.EXPECT().AttachDisk(gomock.Eq(ctx), gomock.Any(), gomock.Eq(req.NodeId)).Return(expDevicePath, nil)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				resp, err := oscDriver.ControllerPublishVolume(ctx, req)
				require.NoError(t, err)

				if !reflect.DeepEqual(resp, expResp) {
					t.Fatalf("Expected resp to be %+v, got: %+v", expResp, resp)
				}
			},
		},
		{
			name: "success when resource is not found",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerUnpublishVolumeRequest{
					NodeId:   expInstanceID,
					VolumeId: "vol-test",
				}
				expResp := &csi.ControllerUnpublishVolumeResponse{}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().DetachDisk(gomock.Eq(ctx), req.VolumeId, req.NodeId).Return(cloud.ErrNotFound)

				oscDriver := controllerService{cloud: mockCloud}
				resp, err := oscDriver.ControllerUnpublishVolume(ctx, req)
				require.NoError(t, err)

				if !reflect.DeepEqual(resp, expResp) {
					t.Fatalf("Expected resp to be %+v, got: %+v", expResp, resp)
				}
			},
		},
		{
			name: "fail no VolumeId",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerPublishVolumeRequest{}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				if _, err := oscDriver.ControllerPublishVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.InvalidArgument {
						t.Fatalf("Expected error code %d, got %d message %s", codes.InvalidArgument, srvErr.Code(), srvErr.Message())
					}
				} else {
					t.Fatalf("Expected error %v, got no error", codes.InvalidArgument)
				}
			},
		},
		{
			name: "fail no NodeId",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerPublishVolumeRequest{
					VolumeId: "vol-test",
				}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				if _, err := oscDriver.ControllerPublishVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.InvalidArgument {
						t.Fatalf("Expected error code %d, got %d message %s", codes.InvalidArgument, srvErr.Code(), srvErr.Message())
					}
				} else {
					t.Fatalf("Expected error %v, got no error", codes.InvalidArgument)
				}
			},
		},
		{
			name: "fail no VolumeCapability",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerPublishVolumeRequest{
					NodeId:   expInstanceID,
					VolumeId: "vol-test",
				}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				if _, err := oscDriver.ControllerPublishVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.InvalidArgument {
						t.Fatalf("Expected error code %d, got %d message %s", codes.InvalidArgument, srvErr.Code(), srvErr.Message())
					}
				} else {
					t.Fatalf("Expected error %v, got no error", codes.InvalidArgument)
				}
			},
		},
		{
			name: "fail invalid VolumeCapability",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerPublishVolumeRequest{
					NodeId: expInstanceID,
					VolumeCapability: &csi.VolumeCapability{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_UNKNOWN,
						},
					},
					VolumeId: "vol-test",
				}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				if _, err := oscDriver.ControllerPublishVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.InvalidArgument {
						t.Fatalf("Expected error code %d, got %d message %s", codes.InvalidArgument, srvErr.Code(), srvErr.Message())
					}
				} else {
					t.Fatalf("Expected error %v, got no error", codes.InvalidArgument)
				}
			},
		},
		{
			name: "fail instance not found",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerPublishVolumeRequest{
					NodeId:           "does-not-exist",
					VolumeId:         "vol-test",
					VolumeCapability: stdVolCap,
				}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().IsExistInstance(gomock.Eq(ctx), gomock.Eq(req.NodeId)).Return(false)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				if _, err := oscDriver.ControllerPublishVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.NotFound {
						t.Fatalf("Expected error code %d, got %d message %s", codes.NotFound, srvErr.Code(), srvErr.Message())
					}
				} else {
					t.Fatalf("Expected error %v, got no error", codes.NotFound)
				}
			},
		},
		{
			name: "fail volume not found",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerPublishVolumeRequest{
					VolumeId:         "does-not-exist",
					NodeId:           expInstanceID,
					VolumeCapability: stdVolCap,
				}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().IsExistInstance(gomock.Eq(ctx), gomock.Eq(req.NodeId)).Return(true)
				mockCloud.EXPECT().GetDiskByID(gomock.Eq(ctx), gomock.Any()).Return(cloud.Disk{}, cloud.ErrNotFound)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				if _, err := oscDriver.ControllerPublishVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.NotFound {
						t.Fatalf("Expected error code %d, got %d message %s", codes.NotFound, srvErr.Code(), srvErr.Message())
					}
				} else {
					t.Fatalf("Expected error %v, got no error", codes.NotFound)
				}
			},
		},
		{
			name: "fail attach disk with already exists error",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerPublishVolumeRequest{
					VolumeId:         "does-not-exist",
					NodeId:           expInstanceID,
					VolumeCapability: stdVolCap,
				}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().IsExistInstance(gomock.Eq(ctx), gomock.Eq(req.NodeId)).Return(true)
				mockCloud.EXPECT().GetDiskByID(gomock.Eq(ctx), gomock.Any()).Return(cloud.Disk{}, nil)
				mockCloud.EXPECT().AttachDisk(gomock.Eq(ctx), gomock.Any(), gomock.Eq(req.NodeId)).Return("", cloud.ErrAlreadyExists)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				if _, err := oscDriver.ControllerPublishVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.AlreadyExists {
						t.Fatalf("Expected error code %d, got %d message %s", codes.AlreadyExists, srvErr.Code(), srvErr.Message())
					}
				} else {
					t.Fatalf("Expected error %v, got no error", codes.AlreadyExists)
				}
			},
		},
		{
			name: "pass encryption variables to NodePublish",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerPublishVolumeRequest{
					NodeId:           expInstanceID,
					VolumeCapability: stdVolCap,
					VolumeId:         "vol-test",
					VolumeContext: map[string]string{
						EncryptedKey:   "true",
						LuksCipherKey:  "cipher",
						LuksHashKey:    "hash",
						LuksKeySizeKey: "keySize",
					},
				}
				expResp := &csi.ControllerPublishVolumeResponse{
					PublishContext: map[string]string{
						DevicePathKey:  expDevicePath,
						EncryptedKey:   "true",
						LuksCipherKey:  "cipher",
						LuksHashKey:    "hash",
						LuksKeySizeKey: "keySize",
					},
				}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().IsExistInstance(gomock.Eq(ctx), gomock.Eq(req.NodeId)).Return(true)
				mockCloud.EXPECT().GetDiskByID(gomock.Eq(ctx), gomock.Any()).Return(cloud.Disk{}, nil)
				mockCloud.EXPECT().AttachDisk(gomock.Eq(ctx), gomock.Any(), gomock.Eq(req.NodeId)).Return(expDevicePath, nil)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				resp, err := oscDriver.ControllerPublishVolume(ctx, req)
				require.NoError(t, err)

				if !reflect.DeepEqual(resp, expResp) {
					t.Fatalf("Expected resp to be %+v, got: %+v", expResp, resp)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestControllerUnpublishVolume(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerUnpublishVolumeRequest{
					NodeId:   expInstanceID,
					VolumeId: "vol-test",
				}
				expResp := &csi.ControllerUnpublishVolumeResponse{}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().DetachDisk(gomock.Eq(ctx), req.VolumeId, req.NodeId).Return(nil)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				resp, err := oscDriver.ControllerUnpublishVolume(ctx, req)
				require.NoError(t, err)

				if !reflect.DeepEqual(resp, expResp) {
					t.Fatalf("Expected resp to be %+v, got: %+v", expResp, resp)
				}
			},
		},
		{
			name: "fail no VolumeId",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerUnpublishVolumeRequest{}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				if _, err := oscDriver.ControllerUnpublishVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.InvalidArgument {
						t.Fatalf("Expected error code %d, got %d message %s", codes.InvalidArgument, srvErr.Code(), srvErr.Message())
					}
				} else {
					t.Fatalf("Expected error %v, got no error", codes.InvalidArgument)
				}
			},
		},
		{
			name: "fail no NodeId",
			testFunc: func(t *testing.T) {
				req := &csi.ControllerUnpublishVolumeRequest{
					VolumeId: "vol-test",
				}

				ctx := context.Background()

				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				if _, err := oscDriver.ControllerUnpublishVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.InvalidArgument {
						t.Fatalf("Expected error code %d, got %d message %s", codes.InvalidArgument, srvErr.Code(), srvErr.Message())
					}
				} else {
					t.Fatalf("Expected error %v, got no error", codes.InvalidArgument)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}
