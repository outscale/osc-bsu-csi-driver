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
	"reflect"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	dm "github.com/outscale/osc-bsu-csi-driver/pkg/cloud/devicemanager"
	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud/mocks"
	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	osc "github.com/outscale/osc-sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	defaultRegion = "test-region"
	defaultZone   = "test-regiona"
	expZone       = "us-west-2b"
)

func TestCreateDisk(t *testing.T) {
	testCases := []struct {
		name               string
		volumeName         string
		volState           string
		diskOptions        *DiskOptions
		expDisk            *Disk
		expErr             error
		expCreateVolumeErr error
		expDescVolumeErr   error
	}{
		{
			name:       "fail: no provided zone",
			volumeName: "vol-test-name",
			diskOptions: &DiskOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: "",
			},
			expDisk: &Disk{
				VolumeID:         "vol-test",
				CapacityGiB:      1,
				AvailabilityZone: defaultZone,
			},
			expErr: nil,
		},
		{
			name:       "success: normal with provided zone",
			volumeName: "vol-test-name",
			diskOptions: &DiskOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: expZone,
			},
			expDisk: &Disk{
				VolumeID:         "vol-test",
				CapacityGiB:      1,
				AvailabilityZone: expZone,
			},
			expErr: nil,
		},
		{
			name:       "success: normal with encrypted volume",
			volumeName: "vol-test-name",
			diskOptions: &DiskOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: expZone,
				Encrypted:        true,
				KmsKeyID:         "arn:aws:kms:us-east-1:012345678910:key/abcd1234-a123-456a-a12b-a123b4cd56ef",
			},
			expDisk: &Disk{
				VolumeID:         "vol-test",
				CapacityGiB:      1,
				AvailabilityZone: expZone,
			},
			expErr: errors.New("Encryption is not supported yet by Outscale API"),
		},
		{
			name:       "fail: CreateVolume returned CreateVolume error",
			volumeName: "vol-test-name-error",
			diskOptions: &DiskOptions{
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
			volState:   "creating",
			diskOptions: &DiskOptions{
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
			diskOptions: &DiskOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: expZone,
				SnapshotID:       "snapshot-test",
			},
			expDisk: &Disk{
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
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)
			volState := tc.volState
			if volState == "" {
				volState = "available"
			}

			vol := osc.CreateVolumeResponse{
				Volume: &osc.Volume{},
			}
			vol.Volume.SetVolumeId(tc.diskOptions.Tags[VolumeNameTagKey])
			vol.Volume.SetSize(util.BytesToGiB(tc.diskOptions.CapacityBytes))
			vol.Volume.SetState(volState)
			vol.Volume.SetSubregionName(tc.diskOptions.AvailabilityZone)

			readSnapshot := osc.Snapshot{
				SnapshotId: &tc.diskOptions.SnapshotID,
			}
			readSnapshot.SetVolumeId("snap-test-volume")
			readSnapshot.SetState("completed")

			tag := osc.CreateTagsResponse{}
			ctx := context.Background()
			if !tc.diskOptions.Encrypted {
				mockOscInterface.EXPECT().CreateVolume(gomock.Eq(ctx), gomock.Any()).Return(vol, nil, tc.expCreateVolumeErr)
				mockOscInterface.EXPECT().CreateTags(gomock.Eq(ctx), gomock.Any()).Return(tag, nil, nil).AnyTimes()
				mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{vol.GetVolume()}}, nil, tc.expDescVolumeErr).AnyTimes()
				if len(tc.diskOptions.SnapshotID) > 0 {
					mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{readSnapshot}}, nil, nil).AnyTimes()
				}
			}

			disk, err := c.CreateDisk(ctx, tc.volumeName, tc.diskOptions)
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
}

func TestDeleteDisk(t *testing.T) {
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
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)

			ctx := context.Background()
			mockOscInterface.EXPECT().DeleteVolume(gomock.Eq(ctx), gomock.Any()).Return(osc.DeleteVolumeResponse{}, nil, tc.expErr)

			ok, err := c.DeleteDisk(ctx, tc.volumeID)
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

func TestAttachDisk(t *testing.T) {
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
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)

			vol := osc.Volume{
				VolumeId: &tc.volumeID,
				LinkedVolumes: &[]osc.LinkedVolume{
					{},
				},
			}
			vol.GetLinkedVolumes()[0].SetState("attached")

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{vol}}, nil, nil).AnyTimes()
			mockOscInterface.EXPECT().ReadVms(gomock.Eq(ctx), gomock.Any()).Return(newDescribeInstancesOutput(tc.nodeID), nil, nil)
			mockOscInterface.EXPECT().LinkVolume(gomock.Eq(ctx), gomock.Any()).Return(osc.LinkVolumeResponse{}, nil, tc.expErr)

			devicePath, err := c.AttachDisk(ctx, tc.volumeID, tc.nodeID)
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

func TestDetachDisk(t *testing.T) {
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
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)

			vol := osc.Volume{
				VolumeId:      &tc.volumeID,
				LinkedVolumes: nil,
			}

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{vol}}, nil, nil).AnyTimes()
			// Create a Vm and add device in BSU
			vm := newDescribeInstancesOutput(tc.nodeID)
			devicePath := "/dev/sdb"
			vm.GetVms()[0].BlockDeviceMappings = &[]osc.BlockDeviceMappingCreated{
				{
					DeviceName: &devicePath,
					Bsu: &osc.BsuCreated{
						VolumeId: &tc.volumeID,
					},
				},
			}
			mockOscInterface.EXPECT().ReadVms(gomock.Eq(ctx), gomock.Any()).Return(vm, nil, nil)
			mockOscInterface.EXPECT().UnlinkVolume(gomock.Eq(ctx), gomock.Any()).Return(osc.UnlinkVolumeResponse{}, nil, tc.expErr)

			err := c.DetachDisk(ctx, tc.volumeID, tc.nodeID)
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			mockCtrl.Finish()
		})
	}
}

func TestGetDiskByName(t *testing.T) {
	testCases := []struct {
		name             string
		volumeName       string
		volumeCapacity   int64
		availabilityZone string
		snapshotId       *string
		expErr           error
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
			name:             "success: normal with snapshotId",
			volumeName:       "vol-test-1234",
			volumeCapacity:   util.GiBToBytes(1),
			snapshotId:       osc.PtrString("snapshot-id123"),
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
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)

			vol := osc.Volume{
				VolumeId:      &tc.volumeName,
				SubregionName: &tc.availabilityZone,
				SnapshotId:    tc.snapshotId,
			}
			vol.SetSize(int32(util.BytesToGiB(tc.volumeCapacity)))

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: &[]osc.Volume{vol}}, nil, tc.expErr)

			disk, err := c.GetDiskByName(ctx, tc.volumeName, tc.volumeCapacity)
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.volumeName, disk.VolumeID)
				assert.Equal(t, util.BytesToGiB(tc.volumeCapacity), disk.CapacityGiB)
				if tc.snapshotId != nil {
					assert.Equal(t, *tc.snapshotId, disk.SnapshotID)
				} else {
					assert.Equal(t, "", disk.SnapshotID)
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
			snapshotId:       osc.PtrString("snapshot-id123"),
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
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(
				osc.ReadVolumesResponse{
					Volumes: &[]osc.Volume{
						{
							VolumeId:      &tc.volumeID,
							SubregionName: &tc.availabilityZone,
							SnapshotId:    tc.snapshotId,
						},
					},
				},
				nil,
				tc.expErr,
			)

			disk, err := c.GetDiskByID(ctx, tc.volumeID)
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.volumeID, disk.VolumeID)
				assert.Equal(t, tc.availabilityZone, disk.AvailabilityZone)
				if tc.snapshotId != nil {
					assert.Equal(t, *tc.snapshotId, disk.SnapshotID)
				} else {
					assert.Equal(t, "", disk.SnapshotID)
				}
			}

			mockCtrl.Finish()
		})
	}
}

func TestCreateSnapshot(t *testing.T) {
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
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)

			oscsnapshot := osc.CreateSnapshotResponse{
				Snapshot: &osc.Snapshot{},
			}
			oscsnapshot.Snapshot.SetSnapshotId(tc.snapshotOptions.Tags[SnapshotNameTagKey])
			oscsnapshot.Snapshot.SetVolumeId("snap-test-volume")
			oscsnapshot.Snapshot.SetState("completed")

			tag := osc.CreateTagsResponse{}
			ctx := context.Background()
			mockOscInterface.EXPECT().CreateSnapshot(gomock.Eq(ctx), gomock.Any()).Return(oscsnapshot, nil, tc.expErr)
			mockOscInterface.EXPECT().CreateTags(gomock.Eq(ctx), gomock.Any()).Return(tag, nil, nil).AnyTimes()
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{oscsnapshot.GetSnapshot()}}, nil, nil).AnyTimes()

			snapshot, err := c.CreateSnapshot(ctx, tc.expSnapshot.SourceVolumeID, tc.snapshotOptions)
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expSnapshot.SourceVolumeID, snapshot.SourceVolumeID)
			}

			mockCtrl.Finish()
		})
	}
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
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
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

func TestGetSnapshotByName(t *testing.T) {
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
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)

			oscsnapshot := osc.Snapshot{}
			oscsnapshot.SetSnapshotId(tc.snapshotOptions.Tags[SnapshotNameTagKey])
			oscsnapshot.SetVolumeId("snap-test-volume")
			oscsnapshot.SetState("completed")

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{oscsnapshot}}, nil, nil)

			_, err := c.GetSnapshotByName(ctx, tc.snapshotOptions.Tags[SnapshotNameTagKey])
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
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)

			oscsnapshot := osc.Snapshot{}
			oscsnapshot.SetSnapshotId(tc.snapshotOptions.Tags[SnapshotNameTagKey])
			oscsnapshot.SetVolumeId("snap-test-volume")
			oscsnapshot.SetState("completed")

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
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success: normal",
			testFunc: func(t *testing.T) {
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
				state := "completed"
				volumeIds := []string{
					"snap-test-volume1",
					"snap-test-volume2",
				}
				oscsnapshot := []osc.Snapshot{
					{
						SnapshotId: &expSnapshots[0].SnapshotID,
						VolumeId:   &volumeIds[0],
						State:      &state,
					},
					{
						SnapshotId: &expSnapshots[1].SnapshotID,
						VolumeId:   &volumeIds[1],
						State:      &state,
					},
				}

				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()
				mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
				c := newCloud(mockOscInterface)

				ctx := context.Background()
				mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil, nil)
				_, err := c.ListSnapshots(ctx, "", 0, "")
				require.NoError(t, err)
			},
		},
		{
			name: "success: with volume ID",
			testFunc: func(t *testing.T) {
				sourceVolumeID := "snap-test-volume"
				state := "completed"
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
						SnapshotId: &expSnapshots[0].SnapshotID,
						VolumeId:   &sourceVolumeID,
						State:      &state,
					},
					{
						SnapshotId: &expSnapshots[1].SnapshotID,
						VolumeId:   &sourceVolumeID,
						State:      &state,
					},
				}

				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()
				mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
				c := newCloud(mockOscInterface)

				ctx := context.Background()

				mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil, nil)

				resp, err := c.ListSnapshots(ctx, sourceVolumeID, 0, "")
				require.NoError(t, err)
				require.Len(t, resp.Snapshots, len(expSnapshots))
				for _, snap := range resp.Snapshots {
					assert.Equal(t, sourceVolumeID, snap.SourceVolumeID)
				}
			},
		},
		{
			name: "fail: Osc ReadSnasphot error",
			testFunc: func(t *testing.T) {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()
				mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
				c := newCloud(mockOscInterface)

				ctx := context.Background()

				mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{}, nil, errors.New("test error"))

				_, err := c.ListSnapshots(ctx, "", 0, "")
				require.Error(t, err)
			},
		},
		{
			name: "fail: no snapshots ErrNotFound",
			testFunc: func(t *testing.T) {
				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()
				mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
				c := newCloud(mockOscInterface)

				ctx := context.Background()

				mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{}, nil, nil)

				_, err := c.ListSnapshots(ctx, "", 0, "")
				require.ErrorIs(t, err, ErrNotFound)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func newCloud(mockOscInterface OscInterface) *cloud {
	return &cloud{
		region:  defaultRegion,
		dm:      dm.NewDeviceManager(),
		client:  mockOscInterface,
		backoff: DefaultBackoffPolicy,
	}
}

func newDescribeInstancesOutput(nodeID string) osc.ReadVmsResponse {
	return osc.ReadVmsResponse{
		Vms: &[]osc.Vm{
			{VmId: &nodeID},
		},
	}
}

func TestResizeDisk(t *testing.T) {
	volumeId := "vol-test"
	var existingVolumeSize int32 = 1
	var modifiedVolumeSize int32 = 2
	defaultZoneVar := defaultZone
	state := "available"
	testCases := []struct {
		name                string
		volumeID            string
		existingVolume      osc.Volume
		existingVolumeError error
		modifiedVolume      osc.UpdateVolumeResponse
		modifiedVolumeError error
		reqSizeGiB          int32
		expErr              error
	}{
		{
			name:     "success: normal",
			volumeID: "vol-test",
			existingVolume: osc.Volume{
				VolumeId:      &volumeId,
				Size:          &existingVolumeSize,
				SubregionName: &defaultZoneVar,
				State:         &state,
			},
			modifiedVolume: osc.UpdateVolumeResponse{
				Volume: &osc.Volume{
					VolumeId:      &volumeId,
					Size:          &modifiedVolumeSize,
					SubregionName: &defaultZoneVar,
				},
			},
			reqSizeGiB: 2,
			expErr:     nil,
		},
		{
			name:     "success: with previous expansion",
			volumeID: "vol-test",
			existingVolume: osc.Volume{
				VolumeId:      &volumeId,
				Size:          &modifiedVolumeSize,
				SubregionName: &defaultZoneVar,
				State:         &state,
			},
			reqSizeGiB: 2,
			expErr:     nil,
		},
		{
			name:                "fail: volume doesn't exist",
			volumeID:            "vol-test",
			existingVolumeError: errors.New("InvalidVolume.NotFound"),
			reqSizeGiB:          2,
			expErr:              errors.New("ResizeDisk generic error"),
		},
		{
			name:     "failure: volume in modifying state",
			volumeID: "vol-test",
			existingVolume: osc.Volume{
				VolumeId:      &volumeId,
				Size:          &existingVolumeSize,
				SubregionName: &defaultZoneVar,
			},
			reqSizeGiB: 2,
			expErr:     errors.New("ResizeDisk generic error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockEC2 := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockEC2)
			ctx := context.Background()

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

				if tc.expErr == nil && tc.existingVolume.GetSize() != tc.reqSizeGiB {
					resizedVolume := osc.Volume{
						VolumeId:      &volumeId,
						Size:          &tc.reqSizeGiB,
						SubregionName: &defaultZoneVar,
						State:         &state,
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

			newSize, err := c.ResizeDisk(ctx, tc.volumeID, util.GiBToBytes(tc.reqSizeGiB))
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
