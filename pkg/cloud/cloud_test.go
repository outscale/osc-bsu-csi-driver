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
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/golang/mock/gomock"

	"github.com/outscale/osc-sdk-go/osc"

	dm "github.com/outscale-dev/osc-bsu-csi-driver/pkg/cloud/devicemanager"
	"github.com/outscale-dev/osc-bsu-csi-driver/pkg/cloud/mocks"
	"github.com/outscale-dev/osc-bsu-csi-driver/pkg/util"
)

const (
	defaultZone = "test-az"
	expZone     = "us-west-2b"
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
			name:       "success: normal",
			volumeName: "vol-test-name",
			diskOptions: &DiskOptions{
				CapacityBytes: util.GiBToBytes(1),
				Tags:          map[string]string{VolumeNameTagKey: "vol-test"},
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
			expErr: fmt.Errorf("Encryption is not supported yet by OSC API"),
		},
		{
			name:       "fail: CreateVolume returned CreateVolume error",
			volumeName: "vol-test-name-error",
			diskOptions: &DiskOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: expZone,
			},
			expErr:             fmt.Errorf("could not create volume in OSC: CreateVolume generic error"),
			expCreateVolumeErr: fmt.Errorf("CreateVolume generic error"),
		},
		{
			name:       "fail: CreateVolume returned a DescribeVolumes error",
			volumeName: "vol-test-name-error",
			volState:   "creating",
			diskOptions: &DiskOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: "",
			},
			expErr:             fmt.Errorf("could not create volume in OSC: DescribeVolumes generic error"),
			expCreateVolumeErr: fmt.Errorf("DescribeVolumes generic error"),
		},
		{
			name:       "fail: CreateVolume returned a volume with wrong state",
			volumeName: "vol-test-name-error",
			volState:   "creating",
			diskOptions: &DiskOptions{
				CapacityBytes:    util.GiBToBytes(1),
				Tags:             map[string]string{VolumeNameTagKey: "vol-test"},
				AvailabilityZone: "",
			},
			expErr: fmt.Errorf("failed to get an available volume in OSC: timed out waiting for the condition"),
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
				Volume: osc.Volume{
					VolumeId:      tc.diskOptions.Tags[VolumeNameTagKey],
					Size:          int32(util.BytesToGiB(tc.diskOptions.CapacityBytes)),
					State:         volState,
					SubregionName: tc.diskOptions.AvailabilityZone,
				},
			}

			readSnapshot := osc.Snapshot{
				SnapshotId: tc.diskOptions.SnapshotID,
				VolumeId:   "snap-test-volume",
				State:      "completed",
			}

			tag := osc.CreateTagsResponse{}
			ctx := context.Background()
			if tc.diskOptions.Encrypted == false {
				mockOscInterface.EXPECT().CreateVolume(gomock.Eq(ctx), gomock.Any()).Return(vol, nil, tc.expCreateVolumeErr)
				mockOscInterface.EXPECT().CreateTags(gomock.Eq(ctx), gomock.Any()).Return(tag, nil, nil).AnyTimes()
				mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: []osc.Volume{vol.Volume}}, nil, tc.expDescVolumeErr).AnyTimes()
				if len(tc.diskOptions.SnapshotID) > 0 {
					mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: []osc.Snapshot{readSnapshot}}, nil, nil).AnyTimes()
				}
			}

			disk, err := c.CreateDisk(ctx, tc.volumeName, tc.diskOptions)
			if err != nil {
				if tc.expErr == nil {
					t.Fatalf("CreateDisk() failed: expected no error, got: %v", err)
				} else if tc.expErr.Error() != err.Error() {
					t.Fatalf("CreateDisk() failed: expected error %q, got: %q", tc.expErr, err)
				}
			} else {
				if tc.expErr != nil {
					t.Fatal("CreateDisk() failed: expected error, got nothing")
				} else {
					if tc.expDisk.CapacityGiB != disk.CapacityGiB {
						t.Fatalf("CreateDisk() failed: expected capacity %d, got %d", tc.expDisk.CapacityGiB, disk.CapacityGiB)
					}
					if tc.expDisk.VolumeID != disk.VolumeID {
						t.Fatalf("CreateDisk() failed: expected capacity %q, got %q", tc.expDisk.VolumeID, disk.VolumeID)
					}
					if tc.expDisk.AvailabilityZone != disk.AvailabilityZone {
						t.Fatalf("CreateDisk() failed: expected availabilityZone %q, got %q", tc.expDisk.AvailabilityZone, disk.AvailabilityZone)
					}
				}
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
			expErr:   fmt.Errorf("DeleteVolume generic error"),
		},
		{
			name:     "fail: DeleteVolume returned not found error",
			volumeID: "vol-test-1234",
			expResp:  false,
			expErr:   awserr.New("InvalidVolume.NotFound", "", nil),
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
			if err != nil && tc.expErr == nil {
				t.Fatalf("DeleteDisk() failed: expected no error, got: %v", err)
			}

			if err == nil && tc.expErr != nil {
				t.Fatal("DeleteDisk() failed: expected error, got nothing")
			}

			if tc.expResp != ok {
				t.Fatalf("DeleteDisk() failed: expected return %v, got %v", tc.expResp, ok)
			}

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
			expErr:   fmt.Errorf(""),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)

			vol := osc.Volume{
				VolumeId:      tc.volumeID,
				LinkedVolumes: []osc.LinkedVolume{{State: "attached"}},
			}

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: []osc.Volume{vol}}, nil, nil).AnyTimes()
			mockOscInterface.EXPECT().ReadVms(gomock.Eq(ctx), gomock.Any()).Return(newDescribeInstancesOutput(tc.nodeID), nil, nil)
			mockOscInterface.EXPECT().LinkVolume(gomock.Eq(ctx), gomock.Any()).Return(osc.LinkVolumeResponse{}, nil, tc.expErr)

			devicePath, err := c.AttachDisk(ctx, tc.volumeID, tc.nodeID)
			if err != nil {
				if tc.expErr == nil {
					t.Fatalf("AttachDisk() failed: expected no error, got: %v", err)
				}
			} else {
				if tc.expErr != nil {
					t.Fatal("AttachDisk() failed: expected error, got nothing")
				}
				if !strings.HasPrefix(devicePath, "/dev/") {
					t.Fatal("AttachDisk() failed: expected valid device path, got empty string")
				}
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
			expErr:   fmt.Errorf("DetachVolume generic error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)

			vol := osc.Volume{
				VolumeId:      tc.volumeID,
				LinkedVolumes: nil,
			}

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: []osc.Volume{vol}}, nil, nil).AnyTimes()
			mockOscInterface.EXPECT().ReadVms(gomock.Eq(ctx), gomock.Any()).Return(newDescribeInstancesOutput(tc.nodeID), nil, nil)
			mockOscInterface.EXPECT().UnlinkVolume(gomock.Eq(ctx), gomock.Any()).Return(osc.UnlinkVolumeResponse{}, nil, tc.expErr)

			err := c.DetachDisk(ctx, tc.volumeID, tc.nodeID)
			if err != nil {
				if tc.expErr == nil {
					t.Fatalf("DetachDisk() failed: expected no error, got: %v", err)
				}
			} else {
				if tc.expErr != nil {
					t.Fatal("DetachDisk() failed: expected error, got nothing")
				}
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
		expErr           error
	}{
		{
			name:             "success: normal",
			volumeName:       "vol-test-1234",
			volumeCapacity:   util.GiBToBytes(1),
			availabilityZone: expZone,
			expErr:           nil,
		},
		{
			name:           "fail: DescribeVolumes returned generic error",
			volumeName:     "vol-test-1234",
			volumeCapacity: util.GiBToBytes(1),
			expErr:         fmt.Errorf("DescribeVolumes generic error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
			c := newCloud(mockOscInterface)

			vol := osc.Volume{
				VolumeId:      tc.volumeName,
				Size:          int32(util.BytesToGiB(tc.volumeCapacity)),
				SubregionName: tc.availabilityZone,
			}

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadVolumesResponse{Volumes: []osc.Volume{vol}}, nil, tc.expErr)

			disk, err := c.GetDiskByName(ctx, tc.volumeName, tc.volumeCapacity)
			if err != nil {
				if tc.expErr == nil {
					t.Fatalf("GetDiskByName() failed: expected no error, got: %v", err)
				}
			} else {
				if tc.expErr != nil {
					t.Fatal("GetDiskByName() failed: expected error, got nothing")
				}
				if disk.CapacityGiB != util.BytesToGiB(tc.volumeCapacity) {
					t.Fatalf("GetDiskByName() failed: expected capacity %d, got %d", util.BytesToGiB(tc.volumeCapacity), disk.CapacityGiB)
				}
				if tc.availabilityZone != disk.AvailabilityZone {
					t.Fatalf("GetDiskByName() failed: expected availabilityZone %q, got %q", tc.availabilityZone, disk.AvailabilityZone)
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
		expErr           error
	}{
		{
			name:             "success: normal",
			volumeID:         "vol-test-1234",
			availabilityZone: expZone,
			expErr:           nil,
		},
		{
			name:     "fail: DescribeVolumes returned generic error",
			volumeID: "vol-test-1234",
			expErr:   fmt.Errorf("DescribeVolumes generic error"),
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
					Volumes: []osc.Volume{
						{
							VolumeId:      tc.volumeID,
							SubregionName: tc.availabilityZone,
						},
					},
				},
				nil,
				tc.expErr,
			)

			disk, err := c.GetDiskByID(ctx, tc.volumeID)
			if err != nil {
				if tc.expErr == nil {
					t.Fatalf("GetDisk() failed: expected no error, got: %v", err)
				}
			} else {
				if tc.expErr != nil {
					t.Fatal("GetDisk() failed: expected error, got nothing")
				}
				if disk.VolumeID != tc.volumeID {
					t.Fatalf("GetDisk() failed: expected ID %q, got %q", tc.volumeID, disk.VolumeID)
				}
				if tc.availabilityZone != disk.AvailabilityZone {
					t.Fatalf("GetDiskByName() failed: expected availabilityZone %q, got %q", tc.availabilityZone, disk.AvailabilityZone)
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
				Snapshot: osc.Snapshot{
					SnapshotId: tc.snapshotOptions.Tags[SnapshotNameTagKey],
					VolumeId:   "snap-test-volume",
					State:      "completed",
				},
			}

			tag := osc.CreateTagsResponse{}
			ctx := context.Background()
			mockOscInterface.EXPECT().CreateSnapshot(gomock.Eq(ctx), gomock.Any()).Return(oscsnapshot, nil, tc.expErr)
			mockOscInterface.EXPECT().CreateTags(gomock.Eq(ctx), gomock.Any()).Return(tag, nil, nil).AnyTimes()
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: []osc.Snapshot{oscsnapshot.Snapshot}}, nil, nil).AnyTimes()

			snapshot, err := c.CreateSnapshot(ctx, tc.expSnapshot.SourceVolumeID, tc.snapshotOptions)
			if err != nil {
				if tc.expErr == nil {
					t.Fatalf("CreateSnapshot() failed: expected no error, got: %v", err)
				}
			} else {
				if tc.expErr != nil {
					t.Fatal("CreateSnapshot() failed: expected error, got nothing")
				} else {
					if snapshot.SourceVolumeID != tc.expSnapshot.SourceVolumeID {
						t.Fatalf("CreateSnapshot() failed: expected source volume ID %s, got %v", tc.expSnapshot.SourceVolumeID, snapshot.SourceVolumeID)
					}
				}
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
			expErr:       fmt.Errorf("DeleteSnapshot generic error"),
		},
		{
			name:         "fail: delete snapshot return not found error",
			snapshotName: "snap-test-name",
			expErr:       awserr.New("InvalidSnapshot.NotFound", "", nil),
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
			if err != nil {
				if tc.expErr == nil {
					t.Fatalf("DeleteSnapshot() failed: expected no error, got: %v", err)
				}
			} else {
				if tc.expErr != nil {
					t.Fatal("DeleteSnapshot() failed: expected error, got nothing")
				}
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

			oscsnapshot := osc.Snapshot{
				SnapshotId: tc.snapshotOptions.Tags[SnapshotNameTagKey],
				VolumeId:   "snap-test-volume",
				State:      "completed",
			}

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: []osc.Snapshot{oscsnapshot}}, nil, nil)

			_, err := c.GetSnapshotByName(ctx, tc.snapshotOptions.Tags[SnapshotNameTagKey])
			if err != nil {
				if tc.expErr == nil {
					t.Fatalf("GetSnapshotByName() failed: expected no error, got: %v", err)
				}
			} else {
				if tc.expErr != nil {
					t.Fatal("GetSnapshotByName() failed: expected error, got nothing")
				}
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

			oscsnapshot := osc.Snapshot{
				SnapshotId: tc.snapshotOptions.Tags[SnapshotNameTagKey],
				VolumeId:   "snap-test-volume",
				State:      "completed",
			}

			ctx := context.Background()
			mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: []osc.Snapshot{oscsnapshot}}, nil, nil)

			_, err := c.GetSnapshotByID(ctx, tc.snapshotOptions.Tags[SnapshotNameTagKey])
			if err != nil {
				if tc.expErr == nil {
					t.Fatalf("GetSnapshotByName() failed: expected no error, got: %v", err)
				}
			} else {
				if tc.expErr != nil {
					t.Fatal("GetSnapshotByName() failed: expected error, got nothing")
				}
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
				oscsnapshot := []osc.Snapshot{
					{
						SnapshotId: expSnapshots[0].SnapshotID,
						VolumeId:   "snap-test-volume1",
						State:      "completed",
					},
					{
						SnapshotId: expSnapshots[1].SnapshotID,
						VolumeId:   "snap-test-volume2",
						State:      "completed",
					},
				}

				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()
				mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
				c := newCloud(mockOscInterface)

				ctx := context.Background()
				fmt.Printf("Read snapshot :\n")
				mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: oscsnapshot}, nil, nil)
				fmt.Printf("End Read snapshot :\n")
				_, err := c.ListSnapshots(ctx, "", 0, "")
				if err != nil {
					t.Fatalf("ListSnapshots() failed: expected no error, got: %v", err)
				}
			},
		},
		{
			name: "success: with volume ID",
			testFunc: func(t *testing.T) {
				sourceVolumeID := "snap-test-volume"
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
						State:      "completed",
					},
					{
						SnapshotId: expSnapshots[1].SnapshotID,
						VolumeId:   sourceVolumeID,
						State:      "completed",
					},
				}

				mockCtrl := gomock.NewController(t)
				defer mockCtrl.Finish()
				mockOscInterface := mocks.NewMockOscInterface(mockCtrl)
				c := newCloud(mockOscInterface)

				ctx := context.Background()

				mockOscInterface.EXPECT().ReadSnapshots(gomock.Eq(ctx), gomock.Any()).Return(osc.ReadSnapshotsResponse{Snapshots: oscsnapshot}, nil, nil)

				resp, err := c.ListSnapshots(ctx, sourceVolumeID, 0, "")
				if err != nil {
					t.Fatalf("ListSnapshots() failed: expected no error, got: %v", err)
				}

				if len(resp.Snapshots) != len(expSnapshots) {
					t.Fatalf("Expected %d snapshots, got %d", len(expSnapshots), len(resp.Snapshots))
				}

				for _, snap := range resp.Snapshots {
					if snap.SourceVolumeID != sourceVolumeID {
						t.Fatalf("Unexpected source volume.  Expected %s, got %s", sourceVolumeID, snap.SourceVolumeID)
					}
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

				if _, err := c.ListSnapshots(ctx, "", 0, ""); err == nil {
					t.Fatalf("ListSnapshots() failed: expected an error, got none")
				}
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

				if _, err := c.ListSnapshots(ctx, "", 0, ""); err != nil {
					if err != ErrNotFound {
						t.Fatalf("Expected error %v, got %v", ErrNotFound, err)
					}
				} else {
					t.Fatalf("Expected error, got none")
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func newCloud(mockOscInterface OscInterface) *cloud {
	return &cloud{
		region: "test-region",
		dm:     dm.NewDeviceManager(),
		client: mockOscInterface,
	}
}

func newDescribeInstancesOutput(nodeID string) osc.ReadVmsResponse {
	return osc.ReadVmsResponse{
		Vms: []osc.Vm{
			{VmId: nodeID},
		},
	}
}

func TestResizeDisk(t *testing.T) {
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
				VolumeId:      "vol-test",
				Size:          1,
				SubregionName: defaultZone,
				State:         "available",
			},
			modifiedVolume: osc.UpdateVolumeResponse{
				Volume: osc.Volume{
					VolumeId:      "vol-test",
					Size:          2,
					SubregionName: defaultZone,
				},
			},
			reqSizeGiB: 2,
			expErr:     nil,
		},
		{
			name:     "success: normal modifying state",
			volumeID: "vol-test",
			existingVolume: osc.Volume{
				VolumeId:      "vol-test",
				Size:          1,
				SubregionName: defaultZone,
				State:         "available",
			},
			modifiedVolume: osc.UpdateVolumeResponse{
				Volume: osc.Volume{
					VolumeId:      "vol-test",
					Size:          2,
					SubregionName: defaultZone,
				},
			},
			reqSizeGiB: 2,
			expErr:     nil,
		},
		{
			name:     "success: with previous expansion",
			volumeID: "vol-test",
			existingVolume: osc.Volume{
				VolumeId:      "vol-test",
				Size:          2,
				SubregionName: defaultZone,
				State:         "available",
			},
			reqSizeGiB: 2,
			expErr:     nil,
		},
		{
			name:                "fail: volume doesn't exist",
			volumeID:            "vol-test",
			existingVolumeError: fmt.Errorf("InvalidVolume.NotFound"),
			reqSizeGiB:          2,
			expErr:              fmt.Errorf("ResizeDisk generic error"),
		},
		{
			name:     "failure: volume in modifying state",
			volumeID: "vol-test",
			existingVolume: osc.Volume{
				VolumeId:      "vol-test",
				Size:          1,
				SubregionName: defaultZone,
			},
			reqSizeGiB: 2,
			expErr:     fmt.Errorf("ResizeDisk generic error"),
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
						Volumes: []osc.Volume{
							tc.existingVolume,
						},
					},
					nil,
					tc.existingVolumeError,
				)

				if tc.expErr == nil && int32(tc.existingVolume.Size) != int32(tc.reqSizeGiB) {
					resizedVolume := osc.Volume{
						VolumeId:      "vol-test",
						Size:          tc.reqSizeGiB,
						SubregionName: defaultZone,
						State:         "available",
					}
					mockEC2.EXPECT().ReadVolumes(gomock.Eq(ctx), gomock.Any()).Return(
						osc.ReadVolumesResponse{
							Volumes: []osc.Volume{
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

			newSize, err := c.ResizeDisk(ctx, tc.volumeID, util.GiBToBytes(int64(tc.reqSizeGiB)))
			if err != nil {
				if tc.expErr == nil {
					t.Fatalf("ResizeDisk() failed: expected no error, got: %v", err)
				}
			} else {
				if tc.expErr != nil {
					t.Fatal("ResizeDisk() failed: expected error, got nothing")
				} else {
					if int32(tc.reqSizeGiB) != int32(newSize) {
						t.Fatalf("ResizeDisk() failed: expected capacity %d, got %d", tc.reqSizeGiB, newSize)
					}
				}
			}

			mockCtrl.Finish()
		})
	}
}
