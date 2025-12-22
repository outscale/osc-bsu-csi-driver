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
	"time"

	"github.com/outscale/goutils/k8s/batch"
	"github.com/outscale/goutils/sdk/mocks_osc"
	"github.com/outscale/goutils/sdk/ptr"
	dm "github.com/outscale/osc-bsu-csi-driver/pkg/cloud/devicemanager"
	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
		volOptions            *VolumeOptions
		expVolume             *Volume
		expErr                error
		expCreateVolumeErr    error
	}{
		{
			name:       "success: a default zone is used",
			volumeName: "vol-test-name",
			volOptions: &VolumeOptions{
				CapacityBytes: util.GiBToBytes(1),
			},
			expVolume: &Volume{
				VolumeID:         "vol-test",
				CapacityGiB:      1,
				AvailabilityZone: defaultZone,
			},
			firstState: osc.VolumeStateCreating,
			nextState:  osc.VolumeStateAvailable,
		},
		{
			name:       "success: normal with provided zone",
			volumeName: "vol-test-name",
			volOptions: &VolumeOptions{
				CapacityBytes: util.GiBToBytes(1),
				SubRegion:     expZone,
			},
			expVolume: &Volume{
				VolumeID:         "vol-test",
				CapacityGiB:      1,
				AvailabilityZone: expZone,
			},
			firstState: osc.VolumeStateCreating,
			nextState:  osc.VolumeStateAvailable,
		},
		{
			name:       "success: normal with tags",
			volumeName: "vol-test-name",
			volOptions: &VolumeOptions{
				CapacityBytes: util.GiBToBytes(1),
				SubRegion:     expZone,
				Tags:          map[string]string{"foo": "bar"},
			},
			expVolume: &Volume{
				VolumeID:         "vol-test",
				CapacityGiB:      1,
				AvailabilityZone: expZone,
			},
			firstState: osc.VolumeStateCreating,
			nextState:  osc.VolumeStateAvailable,
		},
		{
			name:       "fail: normal with encrypted volume",
			volumeName: "vol-test-name",
			volOptions: &VolumeOptions{
				CapacityBytes: util.GiBToBytes(1),
				SubRegion:     expZone,
				Encrypted:     true,
				KmsKeyID:      "arn:aws:kms:us-east-1:012345678910:key/abcd1234-a123-456a-a12b-a123b4cd56ef",
			},
			expErr: errors.New("volume-level encryption is not supported"),
		},
		{
			name:       "fail: CreateVolume returned CreateVolume error",
			volumeName: "vol-test-name-error",
			volOptions: &VolumeOptions{
				CapacityBytes: util.GiBToBytes(1),
				SubRegion:     expZone,
			},
			expErr:             errors.New("create volume: CreateVolume generic error"),
			expCreateVolumeErr: errors.New("CreateVolume generic error"),
		},
		{
			name:       "fail: CreateVolume returned an error",
			volumeName: "vol-test-name-error",
			firstState: osc.VolumeStateCreating,
			volOptions: &VolumeOptions{
				CapacityBytes: util.GiBToBytes(1),
				SubRegion:     expZone,
			},
			expErr:             errors.New("create volume: error"),
			expCreateVolumeErr: errors.New("error"),
		},
		{
			name:       "success: normal from snapshot",
			volumeName: "vol-test-name",
			volOptions: &VolumeOptions{
				CapacityBytes: util.GiBToBytes(1),
				SubRegion:     expZone,
				SnapshotID:    ptr.To("snapshot-test"),
			},
			expVolume: &Volume{
				VolumeID:         "vol-test",
				CapacityGiB:      1,
				AvailabilityZone: expZone,
			},
			firstState: osc.VolumeStateCreating,
			nextState:  osc.VolumeStateAvailable,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c.Start(ctx)
			if tc.firstState == "" {
				tc.firstState = osc.VolumeStateAvailable
			}

			az := tc.volOptions.SubRegion
			if az == "" {
				az = defaultZone
			}
			typ := tc.volOptions.VolumeType
			if typ == "" {
				typ = osc.VolumeTypeGp2
			}
			switch {
			case tc.expErr != nil && tc.expCreateVolumeErr == nil:
			case tc.expCreateVolumeErr != nil:
				mockOscInterface.EXPECT().CreateVolume(gomock.Any(), gomock.Eq(osc.CreateVolumeRequest{
					ClientToken:   ptr.To(tc.volumeName),
					SubregionName: az,
					Size:          ptr.To(util.BytesToGiB(tc.volOptions.CapacityBytes)),
					SnapshotId:    tc.volOptions.SnapshotID,
					VolumeType:    &typ,
				})).Return(nil, tc.expCreateVolumeErr)
			default:
				firstVol := osc.Volume{
					VolumeId:      tc.expVolume.VolumeID,
					Size:          util.BytesToGiB(tc.volOptions.CapacityBytes),
					State:         tc.firstState,
					SubregionName: tc.volOptions.SubRegion,
				}
				nextVol := firstVol
				nextVol.State = tc.nextState
				mockOscInterface.EXPECT().CreateVolume(gomock.Any(), gomock.Eq(osc.CreateVolumeRequest{
					ClientToken:   ptr.To(tc.volumeName),
					SubregionName: az,
					Size:          ptr.To(util.BytesToGiB(tc.volOptions.CapacityBytes)),
					SnapshotId:    tc.volOptions.SnapshotID,
					VolumeType:    &typ,
				})).Return(&osc.CreateVolumeResponse{
					Volume: &firstVol,
				}, nil)
				if tc.nextState != "" {
					var tags []osc.ResourceTag
					if tc.volOptions.Tags != nil {
						for k, v := range tc.volOptions.Tags {
							tags = append(tags, osc.ResourceTag{Key: k, Value: v})
						}
					}
					tags = append(tags, osc.ResourceTag{Key: VolumeNameTagKey, Value: tc.volumeName})
					mockOscInterface.EXPECT().CreateTags(gomock.Any(), gomock.Eq(osc.CreateTagsRequest{
						ResourceIds: []string{firstVol.VolumeId},
						Tags:        tags,
					})).Return(&osc.CreateTagsResponse{}, nil)

					mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Eq(osc.ReadVolumesRequest{
						Filters:        &osc.FiltersVolume{VolumeIds: &[]string{firstVol.VolumeId}},
						ResultsPerPage: ptr.To(1),
					})).Return(&osc.ReadVolumesResponse{Volumes: &[]osc.Volume{nextVol}}, nil).AnyTimes()
				}
				if tc.volOptions.SnapshotID != nil {
					readSnapshot := osc.Snapshot{
						SnapshotId: *tc.volOptions.SnapshotID,
						VolumeId:   "snap-test-volume",
						State:      osc.SnapshotStateCompleted,
					}
					mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Any()).Return(&osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{readSnapshot}}, nil).AnyTimes()
				}
			}

			vol, err := c.CreateVolume(ctx, tc.volumeName, tc.volOptions)
			if tc.expErr != nil {
				require.EqualError(t, err, tc.expErr.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expVolume.CapacityGiB, vol.CapacityGiB)
				assert.Equal(t, tc.expVolume.VolumeID, vol.VolumeID)
				assert.Equal(t, tc.expVolume.AvailabilityZone, vol.AvailabilityZone)
			}

			mockCtrl.Finish()
		})
	}

	t.Run("Volume is created, even when ReadVolumes returns a temporary error", func(t *testing.T) {
		volumeID := "vol-foo"
		volName := "kvol-foo"

		mockCtrl := gomock.NewController(t)
		mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
		c := newCloud(mockOscInterface)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		c.Start(ctx)

		firstVolume := osc.Volume{}
		firstVolume.VolumeId = volumeID
		firstVolume.State = osc.VolumeStateCreating
		firstVolume.Size = 4
		nextVolume := firstVolume
		nextVolume.State = osc.VolumeStateAvailable
		mockOscInterface.EXPECT().CreateVolume(gomock.Any(), gomock.Eq(osc.CreateVolumeRequest{
			ClientToken:   &volName,
			SubregionName: "az",
			Size:          ptr.To(4),
			VolumeType:    ptr.To(osc.VolumeTypeGp2),
		})).Return(&osc.CreateVolumeResponse{
			Volume: &firstVolume,
		}, nil)
		mockOscInterface.EXPECT().CreateTags(gomock.Any(), gomock.Any()).Return(&osc.CreateTagsResponse{}, nil)
		mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Any()).Return(&osc.ReadVolumesResponse{Volumes: &[]osc.Volume{nextVolume}}, nil).After(
			mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Any()).Return(nil, errors.New("an error")),
		)

		vol, err := c.CreateVolume(ctx, volName, &VolumeOptions{
			SubRegion:     "az",
			VolumeType:    "gp2",
			CapacityBytes: util.GiBToBytes(4),
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

			mockOscInterface.EXPECT().DeleteVolume(gomock.Any(), gomock.Eq(osc.DeleteVolumeRequest{VolumeId: tc.volumeID})).
				Return(&osc.DeleteVolumeResponse{}, tc.expErr)

			ok, err := c.DeleteVolume(t.Context(), tc.volumeID)
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
		name             string
		volumeID         string
		nodeID           string
		expLinkVolumeErr error
		expErr           error
	}{
		{
			name:     "success: normal",
			volumeID: "vol-test-1234",
			nodeID:   "node-1234",
		},
		{
			name:     "fail: AttachVolume returned an error",
			volumeID: "vol-test-1234",
			nodeID:   "node-1234",
			expErr:   errors.New("error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c.Start(ctx)
			vol := osc.Volume{
				VolumeId: tc.volumeID,
				LinkedVolumes: []osc.LinkedVolume{
					{State: osc.LinkedVolumeStateAttached},
				},
			}

			mockOscInterface.EXPECT().ReadVms(gomock.Any(), gomock.Eq(osc.ReadVmsRequest{
				Filters: &osc.FiltersVm{VmIds: &[]string{tc.nodeID}},
			})).Return(newDescribeInstancesOutput(tc.nodeID), nil)
			mockOscInterface.EXPECT().LinkVolume(gomock.Any(), gomock.Eq(osc.LinkVolumeRequest{
				DeviceName: "/dev/xvdb",
				VolumeId:   tc.volumeID,
				VmId:       tc.nodeID,
			})).Return(&osc.LinkVolumeResponse{}, tc.expErr)
			if tc.expErr == nil {
				mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Eq(osc.ReadVolumesRequest{
					Filters:        &osc.FiltersVolume{VolumeIds: &[]string{tc.volumeID}},
					ResultsPerPage: ptr.To(1),
				})).Return(&osc.ReadVolumesResponse{Volumes: &[]osc.Volume{vol}}, nil)
			}
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
		initial  osc.VolumeState
		expErr   error
	}{
		{
			name:     "success: volume is already available",
			volumeID: "vol-test-1234",
			nodeID:   "node-1234",
			initial:  osc.VolumeStateAvailable,
		},
		{
			name:     "success: volume is detached",
			volumeID: "vol-test-1234",
			nodeID:   "node-1234",
			initial:  osc.VolumeStateInUse,
		},
		{
			name:     "fail: UnlinkVolume returned an error",
			volumeID: "vol-test-1234",
			nodeID:   "node-1234",
			initial:  osc.VolumeStateInUse,
			expErr:   errors.New("error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c.Start(ctx)

			vol := osc.Volume{
				VolumeId:      tc.volumeID,
				LinkedVolumes: nil,
				State:         tc.initial,
			}

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
			mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Eq(osc.ReadVolumesRequest{
				Filters: &osc.FiltersVolume{VolumeIds: &[]string{tc.volumeID}},
			})).Return(&osc.ReadVolumesResponse{Volumes: &[]osc.Volume{vol}}, nil)
			if tc.initial == osc.VolumeStateInUse {
				mockOscInterface.EXPECT().ReadVms(gomock.Any(), gomock.Eq(osc.ReadVmsRequest{
					Filters: &osc.FiltersVm{VmIds: &[]string{tc.nodeID}},
				})).Return(vm, nil)
				mockOscInterface.EXPECT().UnlinkVolume(gomock.Any(), gomock.Eq(osc.UnlinkVolumeRequest{
					VolumeId: tc.volumeID,
				})).Return(&osc.UnlinkVolumeResponse{}, tc.expErr)
				if tc.expErr == nil {
					detached := vol
					detached.State = osc.VolumeStateAvailable
					mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Eq(osc.ReadVolumesRequest{
						Filters:        &osc.FiltersVolume{VolumeIds: &[]string{tc.volumeID}},
						ResultsPerPage: ptr.To(1),
					})).Return(&osc.ReadVolumesResponse{Volumes: &[]osc.Volume{detached}}, nil)
				}
			}
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
			firstState:       osc.VolumeStateCreating,
			nextState:        osc.VolumeStateAvailable,
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
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c.Start(ctx)
			if tc.firstState == "" {
				tc.firstState = osc.VolumeStateAvailable
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

			mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Any()).Return(&osc.ReadVolumesResponse{Volumes: &[]osc.Volume{firstVol}}, tc.expErr)
			if tc.nextState != "" {
				mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Any()).Return(&osc.ReadVolumesResponse{Volumes: &[]osc.Volume{nextVol}}, nil).After(
					mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Any()).Return(nil, errors.New("error")),
				)
			}

			vol, err := c.CheckCreatedVolume(ctx, tc.volumeName)
			if tc.expErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.volumeName, vol.VolumeID)
				assert.Equal(t, util.BytesToGiB(tc.volumeCapacity), vol.CapacityGiB)
				if tc.snapshotId != nil {
					require.NotNil(t, vol.SnapshotID)
					assert.Equal(t, *tc.snapshotId, *vol.SnapshotID)
				} else {
					assert.Empty(t, vol.SnapshotID)
				}
			}

			mockCtrl.Finish()
		})
	}
}

func TestGetVolumeByID(t *testing.T) {
	testCases := []struct {
		name             string
		volumeID         string
		availabilityZone string
		snapshotId       *string
		found            bool
		expErr           error
	}{
		{
			name:             "success: normal",
			volumeID:         "vol-test-1234",
			snapshotId:       nil,
			availabilityZone: expZone,
			found:            true,
		},
		{
			name:             "success: normal with snapshotId",
			volumeID:         "vol-test-1234",
			snapshotId:       ptr.To("snapshot-id123"),
			availabilityZone: expZone,
			found:            true,
		},
		{
			name:     "fail: not found",
			volumeID: "vol-test-1234",
			expErr:   ErrNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c.Start(ctx)

			res := osc.ReadVolumesResponse{
				Volumes: &[]osc.Volume{},
			}
			if tc.found {
				res.Volumes = &[]osc.Volume{
					{
						VolumeId:      tc.volumeID,
						SubregionName: tc.availabilityZone,
						SnapshotId:    tc.snapshotId,
						Size:          1,
						Iops:          100,
					},
				}
			}
			mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Eq(osc.ReadVolumesRequest{
				Filters:        &osc.FiltersVolume{VolumeIds: &[]string{tc.volumeID}},
				ResultsPerPage: ptr.To(1),
			})).Return(&res, nil)

			vol, err := c.GetVolumeByID(ctx, tc.volumeID)
			if tc.expErr != nil {
				require.ErrorIs(t, err, tc.expErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.volumeID, vol.VolumeID)
				assert.Equal(t, tc.availabilityZone, vol.AvailabilityZone)
				if tc.snapshotId != nil {
					require.NotNil(t, vol.SnapshotID)
					assert.Equal(t, *tc.snapshotId, *vol.SnapshotID)
				} else {
					assert.Empty(t, vol.SnapshotID)
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
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		c.Start(ctx)

		inQueue := osc.Snapshot{}
		inQueue.SnapshotId = snapID
		inQueue.VolumeId = volumeID
		inQueue.State = osc.SnapshotStateInQueue

		pending := inQueue
		pending.State = osc.SnapshotStatePending

		completed := inQueue
		completed.State = osc.SnapshotStateCompleted

		mockOscInterface.EXPECT().CreateSnapshot(gomock.Any(), gomock.Eq(osc.CreateSnapshotRequest{
			ClientToken: &snapName,
			Description: ptr.To("Created by Outscale BSU CSI driver for volume " + volumeID),
			VolumeId:    &volumeID,
		})).Return(&osc.CreateSnapshotResponse{
			Snapshot: &inQueue,
		}, nil)
		mockOscInterface.EXPECT().CreateTags(gomock.Any(), gomock.Eq(osc.CreateTagsRequest{
			ResourceIds: []string{snapID},
			Tags:        []osc.ResourceTag{{Key: "foo", Value: "bar"}, {Key: SnapshotNameTagKey, Value: snapName}},
		})).Return(&osc.CreateTagsResponse{}, nil)
		q := osc.ReadSnapshotsRequest{
			Filters:        &osc.FiltersSnapshot{SnapshotIds: &[]string{snapID}},
			ResultsPerPage: ptr.To(1),
		}
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Eq(q)).Return(&osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{completed}}, nil).After(
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Eq(q)).Return(&osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{pending}}, nil).After(
				mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Eq(q)).Return(&osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{inQueue}}, nil),
			),
		)

		snapshot, err := c.CreateSnapshot(ctx, snapName, volumeID, &SnapshotOptions{
			Tags: map[string]string{
				"foo": "bar",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, volumeID, snapshot.SourceVolumeID)
		assert.True(t, snapshot.IsReadyToUse())
		mockCtrl.Finish()
	})

	t.Run("Snapshot is created, even when ReadSnapshots returns an error", func(t *testing.T) {
		snapID := "snap-foo"
		volumeID := "vol-foo"
		snapName := "ksnap-foo"

		mockCtrl := gomock.NewController(t)
		mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
		c := newCloud(mockOscInterface)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		c.Start(ctx)

		firstSnap := osc.Snapshot{
			SnapshotId: snapID,
			VolumeId:   volumeID,
			State:      osc.SnapshotStateInQueue,
		}
		nextSnap := firstSnap
		nextSnap.State = osc.SnapshotStateCompleted

		mockOscInterface.EXPECT().CreateSnapshot(gomock.Any(), gomock.Eq(osc.CreateSnapshotRequest{
			ClientToken: &snapName,
			Description: ptr.To("Created by Outscale BSU CSI driver for volume " + volumeID),
			VolumeId:    &volumeID,
		})).Return(&osc.CreateSnapshotResponse{
			Snapshot: &firstSnap,
		}, nil)
		mockOscInterface.EXPECT().CreateTags(gomock.Any(), gomock.Any()).Return(&osc.CreateTagsResponse{}, nil)
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Any()).Return(&osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{nextSnap}}, nil).After(
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Any()).Return(nil, errors.New("error")),
		)

		snapshot, err := c.CreateSnapshot(ctx, snapName, volumeID, &SnapshotOptions{
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
			name:         "fail: delete snapshot return an error",
			snapshotName: "snap-test-name",
			expErr:       errors.New("error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)

			ctx := t.Context()
			mockOscInterface.EXPECT().DeleteSnapshot(gomock.Any(), gomock.Any()).Return(&osc.DeleteSnapshotResponse{}, tc.expErr)

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
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c.Start(ctx)

			firstSnap := osc.Snapshot{
				SnapshotId: tc.snapshotOptions.Tags[SnapshotNameTagKey],
				VolumeId:   "snap-test-volume",
				State:      tc.firstState,
			}
			nextSnap := firstSnap
			nextSnap.State = tc.nextState
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Any()).Return(&osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{firstSnap}}, nil)
			if tc.nextState != "" {
				mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Any()).Return(&osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{nextSnap}}, nil).After(
					mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Any()).Return(&osc.ReadSnapshotsResponse{}, errors.New("throttled")),
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
		name         string
		snapshotID   string
		snapshotName string
		found        bool
		expErr       error
	}{
		{
			name:         "success: normal",
			snapshotID:   "snap-foo",
			snapshotName: "snap-test-name",
			found:        true,
		},
		{
			name:         "failure: not found",
			snapshotID:   "snap-foo",
			snapshotName: "snap-test-name",
			expErr:       ErrNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			c.Start(ctx)

			oscsnapshot := osc.Snapshot{}
			oscsnapshot.SnapshotId = tc.snapshotID
			oscsnapshot.VolumeId = "snap-test-volume"
			oscsnapshot.State = "completed"

			res := osc.ReadSnapshotsResponse{Snapshots: &[]osc.Snapshot{}}
			if tc.found {
				res.Snapshots = &[]osc.Snapshot{oscsnapshot}
			}
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Eq(osc.ReadSnapshotsRequest{
				Filters:        &osc.FiltersSnapshot{SnapshotIds: &[]string{tc.snapshotID}},
				ResultsPerPage: ptr.To(1),
			})).Return(&res, nil)

			_, err := c.GetSnapshotByID(ctx, tc.snapshotID)
			if tc.expErr != nil {
				require.ErrorIs(t, err, tc.expErr)
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

		ctx := t.Context()
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Eq(osc.ReadSnapshotsRequest{
			Filters: &osc.FiltersSnapshot{
				TagKeys: &[]string{SnapshotNameTagKey},
			},
		})).Return(&osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil)
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

		ctx := t.Context()
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Eq(osc.ReadSnapshotsRequest{
			Filters: &osc.FiltersSnapshot{
				TagKeys: &[]string{SnapshotNameTagKey},
			},
			ResultsPerPage: ptr.To(10),
		})).Return(&osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil)
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

		ctx := t.Context()
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Eq(osc.ReadSnapshotsRequest{
			Filters: &osc.FiltersSnapshot{
				TagKeys: &[]string{SnapshotNameTagKey},
			},
			ResultsPerPage: ptr.To(1000),
		})).Return(&osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil)
		_, err := c.ListSnapshots(ctx, "", 2000, "")
		require.NoError(t, err)
	})
	t.Run("success with next token & default results per page", func(t *testing.T) {
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

		ctx := t.Context()
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Eq(osc.ReadSnapshotsRequest{
			Filters: &osc.FiltersSnapshot{
				TagKeys: &[]string{SnapshotNameTagKey},
			},
			NextPageToken:  ptr.To("foo"),
			ResultsPerPage: ptr.To(1000),
		})).Return(&osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil)
		_, err := c.ListSnapshots(ctx, "", 0, "foo")
		require.NoError(t, err)
	})
	t.Run("success with next token & results per page", func(t *testing.T) {
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

		ctx := t.Context()
		mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Eq(osc.ReadSnapshotsRequest{
			Filters: &osc.FiltersSnapshot{
				TagKeys: &[]string{SnapshotNameTagKey},
			},
			NextPageToken:  ptr.To("foo"),
			ResultsPerPage: ptr.To(10),
		})).Return(&osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil)
		_, err := c.ListSnapshots(ctx, "", 10, "foo")
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

		ctx := t.Context()

		mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Any()).Return(&osc.ReadSnapshotsResponse{Snapshots: &oscsnapshot}, nil)

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

		ctx := t.Context()

		mockOscInterface.EXPECT().ReadSnapshots(gomock.Any(), gomock.Any()).Return(&osc.ReadSnapshotsResponse{}, errors.New("test error"))

		_, err := c.ListSnapshots(ctx, "", 0, "")
		require.Error(t, err)
	})
}

func newCloud(mock *mocks_osc.MockClient) *cloud {
	return &cloud{
		region:          defaultRegion,
		dm:              dm.NewDeviceManager(),
		client:          mock,
		snapshotWatcher: batch.NewSnapshotBatcherByID(time.Second, mock),
		volumeWatcher:   batch.NewVolumeBatcherByID(time.Second, mock),
	}
}

func newDescribeInstancesOutput(nodeID string) *osc.ReadVmsResponse {
	return &osc.ReadVmsResponse{
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
			mockOscInterface := mocks_osc.NewMockClient(mockCtrl)
			c := newCloud(mockOscInterface)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			c.Start(ctx)

			if !reflect.DeepEqual(tc.existingVolume, osc.Volume{}) || tc.existingVolumeError != nil {
				mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Any()).Return(
					&osc.ReadVolumesResponse{
						Volumes: &[]osc.Volume{
							tc.existingVolume,
						},
					},
					tc.existingVolumeError,
				)

				if tc.expErr == nil && tc.existingVolume.Size != tc.reqSizeGiB {
					resizedVolume := osc.Volume{
						VolumeId:      volumeId,
						Size:          tc.reqSizeGiB,
						SubregionName: defaultZoneVar,
						State:         state,
					}
					mockOscInterface.EXPECT().ReadVolumes(gomock.Any(), gomock.Any()).Return(
						&osc.ReadVolumesResponse{
							Volumes: &[]osc.Volume{
								resizedVolume,
							},
						},
						tc.existingVolumeError,
					)
				}
			}
			if !reflect.DeepEqual(tc.modifiedVolume, osc.UpdateVolumeResponse{}) || tc.modifiedVolumeError != nil {
				mockOscInterface.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Return(
					&tc.modifiedVolume,
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
