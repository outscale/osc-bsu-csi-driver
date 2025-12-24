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
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/outscale/goutils/sdk/batch"
	"github.com/outscale/goutils/sdk/mocks_osc"
	dm "github.com/outscale/osc-bsu-csi-driver/pkg/cloud/devicemanager"
	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/utils/ptr"
)

const (
	defaultRegion = "test-region"
	defaultZone   = "test-regiona"
	expZone       = "us-west-2b"
)

func TestCreateVolume(t *testing.T) {
	testCases := []struct {
		name                  string
		volumeName            string
		firstState, nextState osc.VolumeState
		diskOptions           *VolumeOptions
		expDisk               *Volume
		expErr                error
		expCreateVolumeErr    error
		expDescVolumeErr      error
	}{
		{
			name:       "fail: no provided zone",
			volumeName: "vol-test-name",
			diskOptions: &VolumeOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: "",
			},
			expDisk: &Volume{
				VolumeID:         "vol-test",
				CapacityGiB:      1,
				AvailabilityZone: defaultZone,
			},
			expErr: nil,
		},
		{
			name:       "success: normal with provided zone",
			volumeName: "vol-test-name",
			diskOptions: &VolumeOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: expZone,
			},
			expDisk: &Volume{
				VolumeID:         "vol-test",
				CapacityGiB:      1,
				AvailabilityZone: expZone,
			},
			firstState: "creating",
			nextState:  "available",
			expErr:     nil,
		},
		{
			name:       "success: normal with encrypted volume",
			volumeName: "vol-test-name",
			diskOptions: &VolumeOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: expZone,
				Encrypted:        true,
				KmsKeyID:         "arn:aws:kms:us-east-1:012345678910:key/abcd1234-a123-456a-a12b-a123b4cd56ef",
			},
			expDisk: &Volume{
				VolumeID:         "vol-test",
				CapacityGiB:      1,
				AvailabilityZone: expZone,
			},
			expErr: errors.New("Encryption is not supported yet by Outscale API"),
		},
		{
			name:       "fail: CreateVolume returned CreateVolume error",
			volumeName: "vol-test-name-error",
			diskOptions: &VolumeOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: expZone,
			},
			expErr:             errors.New("could not create volume: CreateVolume generic error"),
			expCreateVolumeErr: errors.New("CreateVolume generic error"),
		},
		{
			name:       "fail: CreateVolume returned a DescribeVolumes error",
			volumeName: "vol-test-name-error",
			firstState: "creating",
			diskOptions: &VolumeOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: expZone,
			},
			expErr:             errors.New("could not create volume: DescribeVolumes generic error"),
			expCreateVolumeErr: errors.New("DescribeVolumes generic error"),
		},
		{
			name:       "success: normal from snapshot",
			volumeName: "vol-test-name",
			diskOptions: &VolumeOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: expZone,
				SnapshotID:       "snapshot-test",
			},
			expDisk: &Volume{
				VolumeID:         "vol-test",
				CapacityGiB:      1,
				AvailabilityZone: expZone,
			},
			expErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)
			ctx := t.Context()
			c.Start(ctx)
			if tc.firstState == "" {
				tc.firstState = "available"
			}

			firstVol := osc.Volume{
				VolumeId:      tc.diskOptions.Tags[VolumeNameTagKey],
				Size:          util.BytesToGiB(tc.diskOptions.CapacityBytes),
				State:         tc.firstState,
				SubregionName: tc.diskOptions.AvailabilityZone,
			}
			nextVol := firstVol
			nextVol.State = tc.nextState

			readSnapshot := osc.Snapshot{
				SnapshotId: tc.diskOptions.SnapshotID,
			}
			readSnapshot.VolumeId = "snap-test-volume"
			readSnapshot.State = "completed"

			tag := osc.CreateTagsResponse{}
			if !tc.diskOptions.Encrypted {
				mockOscInterface.EXPECT().CreateVolume(gomock.Eq(ctx), gomock.Any()).Return(osc.CreateVolumeResponse{
					Volume: &firstVol,
				}, nil, tc.expCreateVolumeErr)
				mockOscInterface.EXPECT().CreateTags(gomock.Eq(ctx), gomock.Any()).Return(tag, nil, nil).AnyTimes()
				if tc.nextState != "" {
					mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{nextVol}}, nil, tc.expDescVolumeErr).AnyTimes()
				}
				if len(tc.diskOptions.SnapshotID) > 0 {
					mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{readSnapshot}}, nil, nil).AnyTimes()
				}
			}

			disk, err := c.CreateVolume(ctx, tc.volumeName, tc.diskOptions)
			if tc.expErr != nil {
				require.EqualError(t, err, tc.expErr.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expDisk.CapacityGiB, disk.CapacityGiB)
				assert.Equal(t, tc.expDisk.VolumeID, disk.VolumeID)
				assert.Equal(t, tc.expDisk.AvailabilityZone, disk.AvailabilityZone)
			}

			mockCtrl.Finish()
		})
	}

	t.Run("Volume is created, even when ReadVolumes is throttled", func(t *testing.T) {
		volumeID := "vol-foo"
		volName := "kvol-foo"

		mockCtrl := gomock.NewController(t)
		mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
		c := newCloud(mockOscInterface)
		ctx := t.Context()
		c.Start(ctx)

		firstVolume := osc.Volume{}
		firstVolume.VolumeId = volumeID
		firstVolume.State = osc.VolumeStateCreating
		firstVolume.Size = 4
		nextVolume := firstVolume
		nextVolume.State = osc.VolumeStateAvailable
		tag := osc.CreateTagsResponse{}
		mockOscInterface.EXPECT().CreateVolume(gomock.Eq(ctx), gomock.Eq(osc.CreateVolumeRequest{
			ClientToken:   &volName,
			SubregionName: "az",
			Size:          ptr.To[int](4),
			VolumeType:    ptr.To("gp2"),
		})).Return(osc.CreateVolumeResponse{
			Volume: &firstVolume,
		}, nil, nil)
		mockOscInterface.EXPECT().CreateTags(gomock.Eq(ctx), gomock.Any()).Return(tag, nil, nil)
		mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{nextVolume}}, nil, nil).After(
			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{}, &http.Response{StatusCode: 429}, errors.New("throttled")),
		)

		vol, err := c.CreateVolume(ctx, volName, &VolumeOptions{
			AvailabilityZone: "az",
			VolumeType:       "gp2",
			CapacityBytes:    util.GiBToBytes(4),
		})
		require.NoError(t, err)
		assert.Equal(t, volumeID, vol.VolumeID)
		mockCtrl.Finish()
	})
}

func TestDeleteVolume(t *testing.T) {
	testCases := []struct {
		name     string
		volumeID string
		expResp  bool
		expErr   error
	}{
		{
			name:     "success: normal",
			volumeID: "vol-test-1234",
			expResp:  true,
			expErr:   nil,
		},
		{
			name:     "fail: DeleteVolume returned generic error",
			volumeID: "vol-test-1234",
			expResp:  false,
			expErr:   errors.New("DeleteVolume generic error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)

			ctx := context.Background()
			mockOscInterface.EXPECT().DeleteVolume(gomock.Eq(ctx), gomock.Any()).Return(osc.DeleteVolumeResponse{}, nil, tc.expErr)

			ok, err := c.DeleteVolume(ctx, tc.volumeID)
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.expResp, ok)

			mockCtrl.Finish()
		})
	}
}

func TestAttachVolume(t *testing.T) {
	testCases := []struct {
		name     string
		volumeID string
		nodeID   string
		expErr   error
	}{
		{
			name:     "success: normal",
			volumeID: "vol-test-1234",
			nodeID:   "node-1234",
			expErr:   nil,
		},
		{
			name:     "fail: AttachVolume returned generic error",
			volumeID: "vol-test-1234",
			nodeID:   "node-1234",
			expErr:   errors.New(""),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)
			ctx := t.Context()
			c.Start(ctx)

			vol := osc.Volume{
				VolumeId: tc.volumeID,
				LinkedVolumes: &[]osc.LinkedVolume{
					{},
				},
			}
			(*vol.LinkedVolumes)[0].State = ptr.To(osc.LinkedVolumeStateAttached)

			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{vol}}, nil, nil).AnyTimes()
			mockOscInterface.EXPECT().ReadVms(gomock.Eq(ctx), gomock.Any()).Return(newDescribeInstancesOutput(tc.nodeID), nil, nil)
			mockOscInterface.EXPECT().LinkVolume(gomock.Eq(ctx), gomock.Any()).Return(osc.LinkVolumeResponse{}, nil, tc.expErr)

			devicePath, err := c.AttachVolume(ctx, tc.volumeID, tc.nodeID)
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.True(t, strings.HasPrefix(devicePath, "/dev/"))
			}

			mockCtrl.Finish()
		})
	}
}

func TestDetachVolume(t *testing.T) {
	testCases := []struct {
		name     string
		volumeID string
		nodeID   string
		expErr   error
	}{
		{
			name:     "success: normal",
			volumeID: "vol-test-1234",
			nodeID:   "node-1234",
			expErr:   nil,
		},
		{
			name:     "fail: DetachVolume returned generic error",
			volumeID: "vol-test-1234",
			nodeID:   "node-1234",
			expErr:   errors.New("DetachVolume generic error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)
			ctx := t.Context()
			c.Start(ctx)

			vol := osc.Volume{
				VolumeId:      tc.volumeID,
				LinkedVolumes: nil,
			}

			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{vol}}, nil, nil).AnyTimes()
			// Create a Vm and add device in BSU
			vm := newDescribeInstancesOutput(tc.nodeID)
			devicePath := "/dev/sdb"
			(*vm.Vms)[0].BlockDeviceMappings = []osc.BlockDeviceMappingCreated{
				{
					DeviceName: devicePath,
					Bsu: osc.BsuCreated{
						VolumeId: tc.volumeID,
					},
				},
			}
			mockOscInterface.EXPECT().ReadVms(gomock.Eq(ctx), gomock.Any()).Return(vm, nil, nil)
			mockOscInterface.EXPECT().UnlinkVolume(gomock.Eq(ctx), gomock.Any()).Return(osc.UnlinkVolumeResponse{}, nil, tc.expErr)

			err := c.DetachVolume(ctx, tc.volumeID, tc.nodeID)
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			mockCtrl.Finish()
		})
	}
}

func TestCheckCreatedVolume(t *testing.T) {
	testCases := []struct {
		name                  string
		volumeName            string
		volumeCapacity        int64
		availabilityZone      string
		snapshotId            *string
		firstState, nextState osc.VolumeState
		expErr                error
	}{
		{
			name:             "success: normal",
			volumeName:       "vol-test-1234",
			volumeCapacity:   util.GiBToBytes(1),
			snapshotId:       nil,
			availabilityZone: expZone,
			expErr:           nil,
		},
		{
			name:             "success: normal with retry",
			volumeName:       "vol-test-1234",
			volumeCapacity:   util.GiBToBytes(1),
			snapshotId:       nil,
			availabilityZone: expZone,
			firstState:       "creating",
			nextState:        "available",
			expErr:           nil,
		},
		{
			name:             "success: normal with snapshotId",
			volumeName:       "vol-test-1234",
			volumeCapacity:   util.GiBToBytes(1),
			snapshotId:       ptr.To("snapshot-id123"),
			availabilityZone: expZone,
			expErr:           nil,
		},
		{
			name:           "fail: DescribeVolumes returned generic error",
			volumeName:     "vol-test-1234",
			volumeCapacity: util.GiBToBytes(1),
			snapshotId:     nil,
			expErr:         errors.New("DescribeVolumes generic error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)
			ctx := t.Context()
			c.Start(ctx)
			if tc.firstState == "" {
				tc.firstState = "available"
			}
			firstVol := osc.Volume{
				VolumeId:      tc.volumeName,
				SubregionName: tc.availabilityZone,
				SnapshotId:    tc.snapshotId,
				State:         tc.firstState,
				Size:          util.BytesToGiB(tc.volumeCapacity),
			}
			nextVol := firstVol
			nextVol.State = tc.nextState

			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{firstVol}}, nil, tc.expErr)
			if tc.nextState != "" {
				mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{nextVol}}, nil, nil).After(
					mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{}, &http.Response{StatusCode: 429}, errors.New("throttled")),
				)
			}

			disk, err := c.CheckCreatedVolume(ctx, tc.volumeName, tc.volumeCapacity)
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.volumeName, disk.VolumeID)
				assert.Equal(t, util.BytesToGiB(tc.volumeCapacity), disk.CapacityGiB)
				if tc.snapshotId != nil {
					assert.Equal(t, *tc.snapshotId, disk.SnapshotID)
				} else {
					assert.Empty(t, disk.SnapshotID)
				}
			}

			mockCtrl.Finish()
		})
	}
}

func TestGetDiskByID(t *testing.T) {
	testCases := []struct {
		name             string
		volumeID         string
		availabilityZone string
		snapshotId       *string
		expErr           error
	}{

		{
			name:             "success: normal",
			volumeID:         "vol-test-1234",
			snapshotId:       nil,
			availabilityZone: expZone,
			expErr:           nil,
		},
		{
			name:             "success: normal with snapshotId",
			volumeID:         "vol-test-1234",
			snapshotId:       ptr.To("snapshot-id123"),
			availabilityZone: expZone,
			expErr:           nil,
		},
		{
			name:       "fail: DescribeVolumes returned generic error",
			volumeID:   "vol-test-1234",
			snapshotId: nil,
			expErr:     errors.New("DescribeVolumes generic error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(
				osc.ReadVolumesResponse{
					Volumes: &[]osc.Volume{
						{
							VolumeId:      tc.volumeID,
							SubregionName: tc.availabilityZone,
							SnapshotId:    tc.snapshotId,
							Size:          1,
							Iops:          100,
						},
					},
				},
				nil,
				tc.expErr,
			)

			disk, err := c.GetVolumeByID(ctx, tc.volumeID)
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.volumeID, disk.VolumeID)
				assert.Equal(t, tc.availabilityZone, disk.AvailabilityZone)
				if tc.snapshotId != nil {
					assert.Equal(t, *tc.snapshotId, disk.SnapshotID)
				} else {
					assert.Empty(t, disk.SnapshotID)
				}
			}

			mockCtrl.Finish()
		})
	}
}

func TestCreateSnapshot(t *testing.T) {
	t.Run("Snapshot is created", func(t *testing.T) {
		snapID := "snap-foo"
		volumeID := "vol-foo"
		snapName := "ksnap-foo"

		mockCtrl := gomock.NewController(t)
		mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
		c := newCloud(mockOscInterface)
		ctx := t.Context()
		c.Start(ctx)

		created := osc.Snapshot{}
		created.SnapshotId = snapID
		created.VolumeId = volumeID
		created.State = "in-queue"

		pending := created
		pending.State = "pending"

		completed := created
		completed.State = "completed"

		tag := osc.CreateTagsResponse{}
		mockOscInterface.EXPECT().CreateSnapshot(gomock.Eq(ctx), gomock.Eq(osc.CreateSnapshotRequest{
			ClientToken: &snapName,
			Description: ptr.To("Created by Outscale BSU CSI driver for volume " + volumeID),
			VolumeId:    &volumeID,
		})).Return(osc.CreateSnapshotResponse{
			Snapshot: &created,
		}, nil, nil)
		mockOscInterface.EXPECT().CreateTags(gomock.Eq(ctx), gomock.Any()).Return(tag, nil, nil)
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{completed}}, nil, nil).After(
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{pending}}, nil, nil).After(
				mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{created}}, nil, nil),
			),
		)

		snapshot, err := c.CreateSnapshot(ctx, volumeID, &SnapshotOptions{
			Tags: map[string]string{
				SnapshotNameTagKey: snapName,
			},
		})
		require.NoError(t, err)
		assert.Equal(t, volumeID, snapshot.SourceVolumeID)
		assert.True(t, snapshot.IsReadyToUse())
		mockCtrl.Finish()
	})

	t.Run("Snapshot is created, even when ReadSnapshots is throttled", func(t *testing.T) {
		snapID := "snap-foo"
		volumeID := "vol-foo"
		snapName := "ksnap-foo"

		mockCtrl := gomock.NewController(t)
		mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
		c := newCloud(mockOscInterface)
		ctx := t.Context()
		c.Start(ctx)

		firstSnap := osc.Snapshot{
			SnapshotId: snapID,
			VolumeId:   volumeID,
			State:      osc.SnapshotStateInQueue,
		}
		nextSnap := firstSnap
		nextSnap.State = osc.SnapshotStateCompleted

		tag := osc.CreateTagsResponse{}
		mockOscInterface.EXPECT().CreateSnapshot(gomock.Eq(ctx), gomock.Eq(osc.CreateSnapshotRequest{
			ClientToken: &snapName,
			Description: ptr.To("Created by Outscale BSU CSI driver for volume " + volumeID),
			VolumeId:    &volumeID,
		})).Return(osc.CreateSnapshotResponse{
			Snapshot: &firstSnap,
		}, nil, nil)
		mockOscInterface.EXPECT().CreateTags(gomock.Eq(ctx), gomock.Any()).Return(tag, nil, nil)
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{nextSnap}}, nil, nil).After(
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{}, &http.Response{StatusCode: 429}, errors.New("throttled")),
		)

		snapshot, err := c.CreateSnapshot(ctx, volumeID, &SnapshotOptions{
			Tags: map[string]string{
				SnapshotNameTagKey: snapName,
			},
		})
		require.NoError(t, err)
		assert.Equal(t, volumeID, snapshot.SourceVolumeID)
		assert.True(t, snapshot.IsReadyToUse())
		mockCtrl.Finish()
	})
}

func TestDeleteSnapshot(t *testing.T) {
	testCases := []struct {
		name         string
		snapshotName string
		expErr       error
	}{
		{
			name:         "success: normal",
			snapshotName: "snap-test-name",
			expErr:       nil,
		},
		{
			name:         "fail: delete snapshot return generic error",
			snapshotName: "snap-test-name",
			expErr:       errors.New("DeleteSnapshot generic error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)

			ctx := context.Background()
			mockOscInterface.EXPECT().DeleteSnapshot(gomock.Eq(ctx), gomock.Any()).Return(osc.DeleteSnapshotResponse{}, nil, tc.expErr)

			_, err := c.DeleteSnapshot(ctx, tc.snapshotName)
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			mockCtrl.Finish()
		})
	}
}

func TestCheckCreatedSnapshot(t *testing.T) {
	testCases := []struct {
		name                  string
		snapshotName          string
		snapshotOptions       *SnapshotOptions
		expSnapshot           *Snapshot
		firstState, nextState osc.SnapshotState
		expErr                error
	}{
		{
			name:         "success: normal",
			snapshotName: "snap-test-name",
			snapshotOptions: &SnapshotOptions{
				Tags: map[string]string{
					SnapshotNameTagKey: "snap-test-name",
				},
			},
			expSnapshot: &Snapshot{
				SourceVolumeID: "snap-test-volume",
			},
			firstState: osc.SnapshotStateCompleted,
			expErr:     nil,
		},
		{
			name:         "success: normal after retry",
			snapshotName: "snap-test-name",
			snapshotOptions: &SnapshotOptions{
				Tags: map[string]string{
					SnapshotNameTagKey: "snap-test-name",
				},
			},
			expSnapshot: &Snapshot{
				SourceVolumeID: "snap-test-volume",
			},
			firstState: osc.SnapshotStateInQueue,
			nextState:  osc.SnapshotStateCompleted,
			expErr:     nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)
			ctx := t.Context()
			c.Start(ctx)

			firstSnap := osc.Snapshot{
				SnapshotId: tc.snapshotOptions.Tags[SnapshotNameTagKey],
				VolumeId:   "snap-test-volume",
				State:      tc.firstState,
			}
			nextSnap := firstSnap
			nextSnap.State = tc.nextState
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{firstSnap}}, nil, nil)
			if tc.nextState != "" {
				mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{nextSnap}}, nil, nil).After(
					mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{}, &http.Response{StatusCode: 429}, errors.New("throttled")),
				)
			}

			_, err := c.CheckCreatedSnapshot(ctx, tc.snapshotOptions.Tags[SnapshotNameTagKey])
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			mockCtrl.Finish()
		})
	}
}

func TestGetSnapshotByID(t *testing.T) {
	testCases := []struct {
		name            string
		snapshotName    string
		snapshotOptions *SnapshotOptions
		expSnapshot     *Snapshot
		expErr          error
	}{
		{
			name:         "success: normal",
			snapshotName: "snap-test-name",
			snapshotOptions: &SnapshotOptions{
				Tags: map[string]string{
					SnapshotNameTagKey: "snap-test-name",
				},
			},
			expSnapshot: &Snapshot{
				SourceVolumeID: "snap-test-volume",
			},
			expErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)

			oscsnapshot := osc.Snapshot{}
			oscsnapshot.SnapshotId = tc.snapshotOptions.Tags[SnapshotNameTagKey]
			oscsnapshot.VolumeId = "snap-test-volume"
			oscsnapshot.State = "completed"

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{oscsnapshot}}, nil, nil)

			_, err := c.GetSnapshotByID(ctx, tc.snapshotOptions.Tags[SnapshotNameTagKey])
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			mockCtrl.Finish()
		})
	}
}

func TestListSnapshots(t *testing.T) {
	t.Run("success without token and maxitems", func(t *testing.T) {
		expSnapshots := []Snapshot{
			{
				SourceVolumeID: "snap-test-volume1",
				SnapshotID:     "snap-test-name1",
			},
			{
				SourceVolumeID: "snap-test-volume2",
				SnapshotID:     "snap-test-name2",
			},
		}
		state := osc.SnapshotStateCompleted
		volumeIds := []string{
			"snap-test-volume1",
			"snap-test-volume2",
		}
		oscsnapshot := []osc.Snapshot{
			{
				SnapshotId: expSnapshots[0].SnapshotID,
				VolumeId:   volumeIds[0],
				State:      state,
			},
			{
				SnapshotId: expSnapshots[1].SnapshotID,
				VolumeId:   volumeIds[1],
				State:      state,
			},
		}

		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
		c := newCloud(mockOscInterface)

		ctx := context.Background()
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Eq(osc.ReadSnapshotsRequest{})).Return(osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil, nil)
		_, err := c.ListSnapshots(ctx, "", 0, "")
		require.NoError(t, err)
	})
	t.Run("success with max entries", func(t *testing.T) {
		expSnapshots := []Snapshot{
			{
				SourceVolumeID: "snap-test-volume1",
				SnapshotID:     "snap-test-name1",
			},
			{
				SourceVolumeID: "snap-test-volume2",
				SnapshotID:     "snap-test-name2",
			},
		}
		state := osc.SnapshotStateCompleted
		volumeIds := []string{
			"snap-test-volume1",
			"snap-test-volume2",
		}
		oscsnapshot := []osc.Snapshot{
			{
				SnapshotId: expSnapshots[0].SnapshotID,
				VolumeId:   volumeIds[0],
				State:      state,
			},
			{
				SnapshotId: expSnapshots[1].SnapshotID,
				VolumeId:   volumeIds[1],
				State:      state,
			},
		}

		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
		c := newCloud(mockOscInterface)

		ctx := context.Background()
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Eq(osc.ReadSnapshotsRequest{
			ResultsPerPage: ptr.To(10),
		})).Return(osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil, nil)
		_, err := c.ListSnapshots(ctx, "", 10, "")
		require.NoError(t, err)
	})
	t.Run("success with max entries over OAPI max (1000)", func(t *testing.T) {
		expSnapshots := []Snapshot{
			{
				SourceVolumeID: "snap-test-volume1",
				SnapshotID:     "snap-test-name1",
			},
			{
				SourceVolumeID: "snap-test-volume2",
				SnapshotID:     "snap-test-name2",
			},
		}
		state := osc.SnapshotStateCompleted
		volumeIds := []string{
			"snap-test-volume1",
			"snap-test-volume2",
		}
		oscsnapshot := []osc.Snapshot{
			{
				SnapshotId: expSnapshots[0].SnapshotID,
				VolumeId:   volumeIds[0],
				State:      state,
			},
			{
				SnapshotId: expSnapshots[1].SnapshotID,
				VolumeId:   volumeIds[1],
				State:      state,
			},
		}

		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
		c := newCloud(mockOscInterface)

		ctx := context.Background()
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Eq(osc.ReadSnapshotsRequest{
			ResultsPerPage: ptr.To(1000),
		})).Return(osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil, nil)
		_, err := c.ListSnapshots(ctx, "", 2000, "")
		require.NoError(t, err)
	})
	t.Run("success with next token", func(t *testing.T) {
		expSnapshots := []Snapshot{
			{
				SourceVolumeID: "snap-test-volume1",
				SnapshotID:     "snap-test-name1",
			},
			{
				SourceVolumeID: "snap-test-volume2",
				SnapshotID:     "snap-test-name2",
			},
		}
		state := osc.SnapshotStateCompleted
		volumeIds := []string{
			"snap-test-volume1",
			"snap-test-volume2",
		}
		oscsnapshot := []osc.Snapshot{
			{
				SnapshotId: expSnapshots[0].SnapshotID,
				VolumeId:   volumeIds[0],
				State:      state,
			},
			{
				SnapshotId: expSnapshots[1].SnapshotID,
				VolumeId:   volumeIds[1],
				State:      state,
			},
		}

		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
		c := newCloud(mockOscInterface)

		ctx := context.Background()
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Eq(osc.ReadSnapshotsRequest{
			NextPageToken: ptr.To([]byte("foo")),
		})).Return(osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil, nil)
		_, err := c.ListSnapshots(ctx, "", 0, "foo")
		require.NoError(t, err)
	})
	t.Run("success: with volume ID", func(t *testing.T) {
		sourceVolumeID := "snap-test-volume"
		state := osc.SnapshotStateCompleted
		expSnapshots := []*Snapshot{
			{
				SourceVolumeID: sourceVolumeID,
				SnapshotID:     "snap-test-name1",
			},
			{
				SourceVolumeID: sourceVolumeID,
				SnapshotID:     "snap-test-name2",
			},
		}
		oscsnapshot := []osc.Snapshot{
			{
				SnapshotId: expSnapshots[0].SnapshotID,
				VolumeId:   sourceVolumeID,
				State:      state,
			},
			{
				SnapshotId: expSnapshots[1].SnapshotID,
				VolumeId:   sourceVolumeID,
				State:      state,
			},
		}

		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
		c := newCloud(mockOscInterface)

		ctx := context.Background()

		mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil, nil)

		resp, err := c.ListSnapshots(ctx, sourceVolumeID, 0, "")
		require.NoError(t, err)
		require.Len(t, resp.Snapshots, len(expSnapshots))
		for _, snap := range resp.Snapshots {
			assert.Equal(t, sourceVolumeID, snap.SourceVolumeID)
		}
	})
	t.Run("fail: Osc ReadSnasphot error", func(t *testing.T) {
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
		c := newCloud(mockOscInterface)

		ctx := context.Background()

		mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{}, nil, errors.New("test error"))

		_, err := c.ListSnapshots(ctx, "", 0, "")
		require.Error(t, err)
	})
}

func newCloud(mock *mocks_osc.MockClient) *cloud {
	return &cloud{
		dm:              dm.NewDeviceManager(),
		client:          mock,
		snapshotWatcher: batch.NewSnapshotBatcherByID(time.Second, mock),
		volumeWatcher:   batch.NewVolumeBatcherByID(time.Second, mock),
	}
}

func newDescribeInstancesOutput(nodeID string) osc.ReadVmsResponse {
	return osc.ReadVmsResponse{
		Vms: &[]osc.Vm{
			{VmId: nodeID},
		},
	}
}

func TestResizeVolume(t *testing.T) {
	volumeId := "vol-test"
	var existingVolumeSize = 1
	var modifiedVolumeSize = 2
	defaultZoneVar := defaultZone
	state := osc.VolumeStateAvailable
	testCases := []struct {
		name                string
		volumeID            string
		existingVolume      osc.Volume
		existingVolumeError error
		modifiedVolume      osc.UpdateVolumeResponse
		modifiedVolumeError error
		reqSizeGiB          int
		expErr              error
	}{
		{
			name:     "success: normal",
			volumeID: "vol-test",
			existingVolume: osc.Volume{
				VolumeId:      volumeId,
				Size:          existingVolumeSize,
				SubregionName: defaultZoneVar,
				State:         state,
			},
			modifiedVolume: osc.UpdateVolumeResponse{
				Volume: &osc.Volume{
					VolumeId:      volumeId,
					Size:          modifiedVolumeSize,
					SubregionName: defaultZoneVar,
				},
			},
			reqSizeGiB: 2,
		},
		{
			name:     "success: with previous expansion",
			volumeID: "vol-test",
			existingVolume: osc.Volume{
				VolumeId:      volumeId,
				Size:          modifiedVolumeSize,
				SubregionName: defaultZoneVar,
				State:         state,
			},
			reqSizeGiB: 2,
		},
		{
			name:                "fail: volume doesn't exist",
			volumeID:            "vol-test",
			existingVolumeError: errors.New("InvalidVolume.NotFound"),
			reqSizeGiB:          2,
			expErr:              errors.New("ResizeDisk generic error"),
		},
		{
			name:     "success: volume in modifying state",
			volumeID: "vol-test",
			existingVolume: osc.Volume{
				VolumeId:      volumeId,
				Size:          existingVolumeSize,
				SubregionName: defaultZoneVar,
			},
			modifiedVolume: osc.UpdateVolumeResponse{
				Volume: &osc.Volume{
					VolumeId:      volumeId,
					Size:          modifiedVolumeSize,
					SubregionName: defaultZoneVar,
				},
			},
			reqSizeGiB: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockEC2 := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockEC2)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c.Start(ctx)

			if !reflect.DeepEqual(tc.existingVolume, osc.Volume{}) || tc.existingVolumeError != nil {
				mockEC2.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(
					osc.ReadVolumesResponse{
						Volumes: &[]osc.Volume{
							tc.existingVolume,
						},
					},
					nil,
					tc.existingVolumeError,
				)

				if tc.expErr == nil && tc.existingVolume.Size != tc.reqSizeGiB {
					resizedVolume := osc.Volume{
						VolumeId:      volumeId,
						Size:          tc.reqSizeGiB,
						SubregionName: defaultZoneVar,
						State:         state,
					}
					mockEC2.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(
						osc.ReadVolumesResponse{
							Volumes: &[]osc.Volume{
								resizedVolume,
							},
						},
						nil,
						tc.existingVolumeError,
					)
				}
			}
			if !reflect.DeepEqual(tc.modifiedVolume, osc.UpdateVolumeResponse{}) || tc.modifiedVolumeError != nil {
				mockEC2.EXPECT().UpdateVolume(gomock.Eq(ctx), gomock.Any()).Return(
					tc.modifiedVolume,
					nil,
					tc.modifiedVolumeError).AnyTimes()
			}

			newSize, err := c.ResizeVolume(ctx, tc.volumeID, util.GiBToBytes(tc.reqSizeGiB))
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, util.GiBToBytes(tc.reqSizeGiB), newSize)
			}

			mockCtrl.Finish()
		})
	}
}
