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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
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

				if _, err := oscDriver.CreateVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}
			},
		},
		{
			name: "restore snapshot",
			testFunc: func(t *testing.T) {
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
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				snapshotID := ""
				if rsp.Volume != nil && rsp.Volume.ContentSource != nil && rsp.Volume.ContentSource.GetSnapshot() != nil {
					snapshotID = rsp.Volume.ContentSource.GetSnapshot().SnapshotId
				}
				if rsp.Volume.ContentSource.GetSnapshot().SnapshotId != "snapshot-id" {
					t.Errorf("Unexpected snapshot ID: %q", snapshotID)
				}
			},
		},
		{
			name: "restore snapshot, volume already exists",
			testFunc: func(t *testing.T) {
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
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				snapshotID := ""
				if rsp.Volume != nil && rsp.Volume.ContentSource != nil && rsp.Volume.ContentSource.GetSnapshot() != nil {
					snapshotID = rsp.Volume.ContentSource.GetSnapshot().SnapshotId
				}
				if rsp.Volume.ContentSource.GetSnapshot().SnapshotId != "snapshot-id" {
					t.Errorf("Unexpected snapshot ID: %q", snapshotID)
				}
			},
		},
		{
			name: "restore snapshot, volume already exists with different snapshot ID",
			testFunc: func(t *testing.T) {
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

				if _, err := oscDriver.CreateVolume(ctx, req); err == nil {
					t.Error("CreateVolume with invalid SnapshotID unexpectedly succeeded")
				}
			},
		},
		{
			name: "fail no name",
			testFunc: func(t *testing.T) {
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

				if _, err := oscDriver.CreateVolume(ctx, req); err != nil {
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
			name: "success same name and same capacity",
			testFunc: func(t *testing.T) {
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

				if _, err := oscDriver.CreateVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				// Subsequent call returns the created disk
				mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(mockDisk, nil)
				resp, err := oscDriver.CreateVolume(ctx, extraReq)
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				vol := resp.GetVolume()
				if vol == nil {
					t.Fatalf("Expected volume %v, got nil", expVol)
				}

				if vol.GetCapacityBytes() != expVol.GetCapacityBytes() {
					t.Fatalf("Expected volume capacity bytes: %v, got: %v", expVol.GetCapacityBytes(), vol.GetCapacityBytes())
				}

				if vol.GetVolumeId() != expVol.GetVolumeId() {
					t.Fatalf("Expected volume id: %v, got: %v", expVol.GetVolumeId(), vol.GetVolumeId())
				}

				if expVol.GetAccessibleTopology() != nil {
					if !reflect.DeepEqual(expVol.GetAccessibleTopology(), vol.GetAccessibleTopology()) {
						t.Fatalf("Expected AccessibleTopology to be %+v, got: %+v", expVol.GetAccessibleTopology(), vol.GetAccessibleTopology())
					}
				}

				for expKey, expVal := range expVol.GetVolumeContext() {
					ctx := vol.GetVolumeContext()
					if gotVal, ok := ctx[expKey]; !ok || gotVal != expVal {
						t.Fatalf("Expected volume context for key %v: %v, got: %v", expKey, expVal, gotVal)
					}
				}
			},
		},
		{
			name: "fail same name and different capacity",
			testFunc: func(t *testing.T) {
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
				if err != nil {
					t.Fatalf("Unable to get volume size bytes for req: %s", err)
				}
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
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				extraVolSizeBytes, err := getVolSizeBytes(extraReq)
				if err != nil {
					t.Fatalf("Unable to get volume size bytes for req: %s", err)
				}

				// Subsequent failure
				mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(extraReq.Name), gomock.Eq(extraVolSizeBytes)).Return(cloud.Disk{}, cloud.ErrDiskExistsDiffSize)
				if _, err := oscDriver.CreateVolume(ctx, extraReq); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.AlreadyExists {
						t.Fatalf("Expected error code %d, got %d", codes.AlreadyExists, srvErr.Code())
					}
				} else {
					t.Fatalf("Expected error %v, got no error", codes.AlreadyExists)
				}
			},
		},
		{
			name: "success no capacity range",
			testFunc: func(t *testing.T) {
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
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				vol := resp.GetVolume()
				if vol == nil {
					t.Fatalf("Expected volume %v, got nil", expVol)
				}

				if vol.GetCapacityBytes() != expVol.GetCapacityBytes() {
					t.Fatalf("Expected volume capacity bytes: %v, got: %v", expVol.GetCapacityBytes(), vol.GetCapacityBytes())
				}

				for expKey, expVal := range expVol.GetVolumeContext() {
					ctx := vol.GetVolumeContext()
					if gotVal, ok := ctx[expKey]; !ok || gotVal != expVal {
						t.Fatalf("Expected volume context for key %v: %v, got: %v", expKey, expVal, gotVal)
					}
				}
			},
		},
		{
			name: "success with correct round up",
			testFunc: func(t *testing.T) {
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
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				vol := resp.GetVolume()
				if vol == nil {
					t.Fatalf("Expected volume %v, got nil", expVol)
				}

				if vol.GetCapacityBytes() != expVol.GetCapacityBytes() {
					t.Fatalf("Expected volume capacity bytes: %v, got: %v", expVol.GetCapacityBytes(), vol.GetCapacityBytes())
				}
			},
		},
		{
			name: "success with volume type io1",
			testFunc: func(t *testing.T) {
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

				if _, err := oscDriver.CreateVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}
			},
		},
		{
			name: "success with volume type sc1",
			testFunc: func(t *testing.T) {
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

				if _, err := oscDriver.CreateVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}
			},
		},
		{
			name: "success with volume type standard",
			testFunc: func(t *testing.T) {
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

				if _, err := oscDriver.CreateVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}
			},
		},
		{
			name: "success with volume encryption",
			testFunc: func(t *testing.T) {
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
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				assert.Equal(t, "true", volumeResponse.GetVolume().VolumeContext[EncryptedKey])
			},
		},
		{
			name: "success with volume encryption",
			testFunc: func(t *testing.T) {
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
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				assert.Equal(t, "true", volumeResponse.GetVolume().VolumeContext[EncryptedKey])
				assert.Equal(t, "", volumeResponse.GetVolume().VolumeContext[LuksCipherKey])
				assert.Equal(t, "", volumeResponse.GetVolume().VolumeContext[LuksHashKey])
				assert.Equal(t, "", volumeResponse.GetVolume().VolumeContext[LuksKeySizeKey])
			},
		},
		{
			name: "success with volume encryption with parameters",
			testFunc: func(t *testing.T) {
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
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				assert.Equal(t, "true", volumeResponse.GetVolume().VolumeContext[EncryptedKey])
				assert.Equal(t, "cipher", volumeResponse.GetVolume().VolumeContext[LuksCipherKey])
				assert.Equal(t, "hash", volumeResponse.GetVolume().VolumeContext[LuksHashKey])
				assert.Equal(t, "keysize", volumeResponse.GetVolume().VolumeContext[LuksKeySizeKey])
			},
		},
		{
			name: "fail with invalid volume parameter",
			testFunc: func(t *testing.T) {
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
				if err == nil {
					t.Fatalf("Expected CreateVolume to fail but got no error")
				}

				srvErr, ok := status.FromError(err)
				if !ok {
					t.Fatalf("Could not get error status code from error: %v", srvErr)
				}
				if srvErr.Code() != codes.InvalidArgument {
					t.Fatalf("Expect InvalidArgument but got: %s", srvErr.Code())
				}
			},
		},
		{
			name: "success when volume exists and contains VolumeContext and AccessibleTopology",
			testFunc: func(t *testing.T) {
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

				if _, err := oscDriver.CreateVolume(ctx, req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				mockCloud.EXPECT().GetDiskByName(gomock.Eq(ctx), gomock.Eq(req.Name), gomock.Eq(stdVolSize)).Return(mockDisk, nil)
				resp, err := oscDriver.CreateVolume(ctx, extraReq)
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}

				vol := resp.GetVolume()
				if vol == nil {
					t.Fatalf("Expected volume %v, got nil", expVol)
				}

				for expKey, expVal := range expVol.GetVolumeContext() {
					ctx := vol.GetVolumeContext()
					if gotVal, ok := ctx[expKey]; !ok || gotVal != expVal {
						t.Fatalf("Expected volume context for key %v: %v, got: %v", expKey, expVal, gotVal)
					}
				}

				if expVol.GetAccessibleTopology() != nil {
					if !reflect.DeepEqual(expVol.GetAccessibleTopology(), vol.GetAccessibleTopology()) {
						t.Fatalf("Expected AccessibleTopology to be %+v, got: %+v", expVol.GetAccessibleTopology(), vol.GetAccessibleTopology())
					}
				}
			},
		},
		{
			name: "success with extra tags",
			testFunc: func(t *testing.T) {
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
				if err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					t.Fatalf("Unexpected error: %v", srvErr.Code())
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
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
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
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
			},
		},
		{
			name: "fail no name",
			testFunc: func(t *testing.T) {
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
			},
		},
		{
			name: "fail same name different volume ID",
			testFunc: func(t *testing.T) {
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
			},
		},
		{
			name: "success same name same volume ID",
			testFunc: func(t *testing.T) {
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
			},
		},
		{
			name: "success with extra tags",
			testFunc: func(t *testing.T) {
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
			},
		},
		{
			name: "snapshot in error are recreated",
			testFunc: func(t *testing.T) {
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
					State:          "completed",
				}
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().GetSnapshotByName(gomock.Eq(ctx), gomock.Eq(req.GetName())).
					Return(cloud.Snapshot{SnapshotID: "snap_foo", State: "error"}, nil)
				mockCloud.EXPECT().DeleteSnapshot(gomock.Eq(ctx), gomock.Eq("snap_foo")).Return(true, nil)
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
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
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
				if _, err := oscDriver.DeleteSnapshot(ctx, req); err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			},
		},
		{
			name: "success not found",
			testFunc: func(t *testing.T) {
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
				if _, err := oscDriver.DeleteSnapshot(ctx, req); err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestListSnapshots(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
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

				if len(resp.GetEntries()) != len(mockCloudSnapshotsResponse.Snapshots) {
					t.Fatalf("Expected %d entries, got %d", len(mockCloudSnapshotsResponse.Snapshots), len(resp.GetEntries()))
				}
			},
		},
		{
			name: "success no snapshots",
			testFunc: func(t *testing.T) {
				req := &csi.ListSnapshotsRequest{}
				ctx := context.Background()
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().ListSnapshots(gomock.Eq(ctx), gomock.Eq(""), gomock.Eq(int32(0)), gomock.Eq("")).Return(cloud.ListSnapshotsResponse{}, cloud.ErrNotFound)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				resp, err := oscDriver.ListSnapshots(context.Background(), req)
				require.NoError(t, err)

				if !reflect.DeepEqual(resp, &csi.ListSnapshotsResponse{}) {
					t.Fatalf("Expected empty response, got %+v", resp)
				}
			},
		},
		{
			name: "success snapshot ID",
			testFunc: func(t *testing.T) {
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

				if len(resp.GetEntries()) != 1 {
					t.Fatalf("Expected %d entry, got %d", 1, len(resp.GetEntries()))
				}
			},
		},
		{
			name: "success snapshot ID not found",
			testFunc: func(t *testing.T) {
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

				if !reflect.DeepEqual(resp, &csi.ListSnapshotsResponse{}) {
					t.Fatalf("Expected empty response, got %+v", resp)
				}
			},
		},
		{
			name: "fail snapshot ID multiple found",
			testFunc: func(t *testing.T) {
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

				if _, err := oscDriver.ListSnapshots(context.Background(), req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.Internal {
						t.Fatalf("Expected error code %d, got %d message %s", codes.Internal, srvErr.Code(), srvErr.Message())
					}
				} else {
					t.Fatalf("Expected error code %d, got no error", codes.Internal)
				}
			},
		},
		{
			name: "fail 0 < MaxEntries < 5",
			testFunc: func(t *testing.T) {
				req := &csi.ListSnapshotsRequest{
					MaxEntries: 4,
				}

				ctx := context.Background()
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()
				mockCloud := mocks.NewMockCloud(mockCtl)
				mockCloud.EXPECT().ListSnapshots(gomock.Eq(ctx), gomock.Eq(""), gomock.Eq(int32(4)), gomock.Eq("")).Return(cloud.ListSnapshotsResponse{}, cloud.ErrInvalidMaxResults)

				oscDriver := controllerService{
					cloud:         mockCloud,
					driverOptions: &DriverOptions{},
				}

				if _, err := oscDriver.ListSnapshots(context.Background(), req); err != nil {
					srvErr, ok := status.FromError(err)
					if !ok {
						t.Fatalf("Could not get error status code from error: %v", srvErr)
					}
					if srvErr.Code() != codes.InvalidArgument {
						t.Fatalf("Expected error code %d, got %d message %s", codes.InvalidArgument, srvErr.Code(), srvErr.Message())
					}
				} else {
					t.Fatalf("Expected error code %d, got no error", codes.InvalidArgument)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
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
