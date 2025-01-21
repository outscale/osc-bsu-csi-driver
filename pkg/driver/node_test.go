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
	"os"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/outscale/osc-bsu-csi-driver/pkg/driver/internal"
	"github.com/outscale/osc-bsu-csi-driver/pkg/driver/luks"
	"github.com/outscale/osc-bsu-csi-driver/pkg/driver/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	exec "k8s.io/utils/exec"
)

func TestNodeStageVolume(t *testing.T) {

	var (
		targetPath          = "/test/path"
		devicePath          = "/dev/fake"
		encryptedDeviceName = "fake_crypt"
		encryptedDevicePath = "/dev/mapper/fake_crypt"
		passphrase          = "ThisIsASecretKey"
		stdVolCap           = &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					FsType: FSTypeExt4,
				},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		}
	)
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: targetPath,
					VolumeCapability:  stdVolCap,
					VolumeId:          "vol-test",
				}

				gomock.InOrder(
					mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(true, nil),
					mockMounter.EXPECT().ExistsPath(gomock.Eq(targetPath)).Return(false, nil),
				)

				mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
				mockMounter.EXPECT().GetDeviceName(targetPath).Return("", 1, nil)
				mockMounter.EXPECT().GetDiskFormat(devicePath).Return("", nil)
				mockMounter.EXPECT().FormatAndMount(gomock.Eq(devicePath), gomock.Eq(targetPath), gomock.Eq(FSTypeExt4), gomock.Any())
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success normal [raw block]",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: "/dev/fake"},
					StagingTargetPath: "/test/path",
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
				}

				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success with mount options",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{
								MountFlags: []string{"dirsync", "noexec"},
							},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
				}

				gomock.InOrder(
					mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(true, nil),
					mockMounter.EXPECT().ExistsPath(gomock.Eq(targetPath)).Return(false, nil),
				)

				mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
				mockMounter.EXPECT().GetDeviceName(targetPath).Return("", 1, nil)
				mockMounter.EXPECT().GetDiskFormat(gomock.Eq(devicePath)).Return("", nil)
				mockMounter.EXPECT().Command(gomock.Eq("mkfs.xfs"), gomock.Eq(devicePath)).Return(exec.New().Command("mkfs"))
				mockMounter.EXPECT().FormatAndMount(gomock.Eq(devicePath), gomock.Eq(targetPath), gomock.Eq(FSTypeXfs), gomock.Eq([]string{"dirsync", "noexec"}))
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success fsType ext3",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{
								FsType: FSTypeExt3,
							},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
				}

				gomock.InOrder(
					mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(true, nil),
					mockMounter.EXPECT().ExistsPath(gomock.Eq(targetPath)).Return(false, nil),
				)

				mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
				mockMounter.EXPECT().GetDeviceName(targetPath).Return("", 1, nil)
				mockMounter.EXPECT().GetDiskFormat(devicePath).Return("", nil)
				mockMounter.EXPECT().FormatAndMount(gomock.Eq(devicePath), gomock.Eq(targetPath), gomock.Eq(FSTypeExt3), gomock.Any())
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success mount with default fsType xfs",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{
								FsType: "",
							},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
				}

				gomock.InOrder(
					mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(true, nil),
					mockMounter.EXPECT().ExistsPath(gomock.Eq(targetPath)).Return(false, nil),
				)

				mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
				mockMounter.EXPECT().GetDeviceName(targetPath).Return("", 1, nil)
				mockMounter.EXPECT().GetDiskFormat(gomock.Eq(devicePath)).Return("", nil)
				mockMounter.EXPECT().Command(gomock.Eq("mkfs.xfs"), gomock.Eq(devicePath)).Return(exec.New().Command("mkfs"))
				mockMounter.EXPECT().FormatAndMount(gomock.Eq(devicePath), gomock.Eq(targetPath), gomock.Eq(FSTypeXfs), gomock.Any())
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "fail no VolumeId",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: targetPath,
					VolumeCapability:  stdVolCap,
				}

				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail no mount",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
				}

				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail no StagingTargetPath",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				devicePath := "/dev/fake"
				req := &csi.NodeStageVolumeRequest{
					PublishContext:   map[string]string{DevicePathKey: devicePath},
					VolumeCapability: stdVolCap,
					VolumeId:         "vol-test",
				}

				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail no VolumeCapability",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: "/test/path",
					VolumeId:          "vol-test",
				}
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail invalid VolumeCapability",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: "/dev/fake"},
					StagingTargetPath: "/test/path",
					VolumeCapability: &csi.VolumeCapability{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_UNKNOWN,
						},
					},
					VolumeId: "vol-test",
				}
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail no devicePath",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					StagingTargetPath: targetPath,
					VolumeCapability:  stdVolCap,
					VolumeId:          "vol-test",
				}
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "success device already mounted at target",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: targetPath,
					VolumeCapability:  stdVolCap,
					VolumeId:          "vol-test",
				}

				gomock.InOrder(
					mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(true, nil),
					mockMounter.EXPECT().ExistsPath(gomock.Eq(targetPath)).Return(false, nil),
				)

				mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
				mockMounter.EXPECT().GetDeviceName(targetPath).Return(devicePath, 1, nil)
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success mount of a disk from an old CSI plugin version (<= 0.0.14beta) with default FSType",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{
								FsType: "",
							},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
				}

				gomock.InOrder(
					mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(true, nil),
					mockMounter.EXPECT().ExistsPath(gomock.Eq(targetPath)).Return(false, nil),
				)

				mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
				mockMounter.EXPECT().GetDeviceName(targetPath).Return("", 1, nil)
				mockMounter.EXPECT().GetDiskFormat(gomock.Eq(devicePath)).Return("ext4", nil)
				mockMounter.EXPECT().FormatAndMount(gomock.Eq(devicePath), gomock.Eq(targetPath), gomock.Eq(FSTypeExt4), gomock.Any())
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success encryption with no parameters",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext: map[string]string{
						DevicePathKey: devicePath,
						EncryptedKey:  "true",
					},
					StagingTargetPath: targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{
								FsType: "",
							},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
					Secrets: map[string]string{
						LuksPassphraseKey: passphrase,
					},
				}

				gomock.InOrder(
					mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(true, nil),
					mockMounter.EXPECT().ExistsPath(gomock.Eq(targetPath)).Return(false, nil),
				)

				mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
				mockMounter.EXPECT().GetDeviceName(targetPath).Return("", 1, nil)
				// Check Luks
				mockMounter.EXPECT().IsLuks(gomock.Eq(devicePath)).Return(false)
				mockMounter.EXPECT().LuksFormat(gomock.Eq(devicePath), gomock.Eq(passphrase), gomock.Eq(luks.LuksContext{Cipher: "", Hash: "", KeySize: ""})).Return(nil)
				mockMounter.EXPECT().CheckLuksPassphrase(gomock.Eq(devicePath), gomock.Eq(passphrase)).Return(nil)
				mockMounter.EXPECT().LuksOpen(gomock.Eq(devicePath), gomock.Eq(encryptedDeviceName), gomock.Eq(passphrase))

				// Format opened luks device
				mockMounter.EXPECT().GetDiskFormat(gomock.Eq(encryptedDevicePath)).Return(defaultFsType, nil)
				mockMounter.EXPECT().FormatAndMount(gomock.Eq(encryptedDevicePath), gomock.Eq(targetPath), gomock.Eq(defaultFsType), gomock.Any())
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success encryption with parameters",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext: map[string]string{
						DevicePathKey:  devicePath,
						EncryptedKey:   "true",
						LuksCipherKey:  "anCipher",
						LuksHashKey:    "AnHashAlgo",
						LuksKeySizeKey: "AnKeySize",
					},
					StagingTargetPath: targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{
								FsType: "",
							},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
					Secrets: map[string]string{
						LuksPassphraseKey: passphrase,
					},
				}

				gomock.InOrder(
					mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(true, nil),
					mockMounter.EXPECT().ExistsPath(gomock.Eq(targetPath)).Return(false, nil),
				)

				mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
				mockMounter.EXPECT().GetDeviceName(targetPath).Return("", 1, nil)
				// Check Luks
				mockMounter.EXPECT().IsLuks(gomock.Eq(devicePath)).Return(false)
				mockMounter.EXPECT().LuksFormat(gomock.Eq(devicePath), gomock.Eq(passphrase), gomock.Eq(luks.LuksContext{Cipher: req.PublishContext[LuksCipherKey], Hash: req.PublishContext[LuksHashKey], KeySize: req.PublishContext[LuksKeySizeKey]})).Return(nil)
				mockMounter.EXPECT().CheckLuksPassphrase(gomock.Eq(devicePath), gomock.Eq(passphrase)).Return(nil)
				mockMounter.EXPECT().LuksOpen(gomock.Eq(devicePath), gomock.Eq(encryptedDeviceName), gomock.Eq(passphrase))

				// Format opened luks device
				mockMounter.EXPECT().GetDiskFormat(gomock.Eq(encryptedDevicePath)).Return(defaultFsType, nil)
				mockMounter.EXPECT().FormatAndMount(gomock.Eq(encryptedDevicePath), gomock.Eq(targetPath), gomock.Eq(defaultFsType), gomock.Any())
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "failure encryption with no passphrase",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext: map[string]string{
						DevicePathKey: devicePath,
						EncryptedKey:  "true",
					},
					StagingTargetPath: targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{
								FsType: "",
							},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
					Secrets:  map[string]string{},
				}

				gomock.InOrder(
					mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(true, nil),
					mockMounter.EXPECT().ExistsPath(gomock.Eq(targetPath)).Return(false, nil),
				)

				mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
				mockMounter.EXPECT().GetDeviceName(targetPath).Return("", 1, nil)
				// Check Luks
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				require.Error(t, err)
			},
		},
		{
			name: "success encryption already format",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeStageVolumeRequest{
					PublishContext: map[string]string{
						DevicePathKey: devicePath,
						EncryptedKey:  "true",
					},
					StagingTargetPath: targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{
								FsType: "",
							},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
					Secrets: map[string]string{
						LuksPassphraseKey: passphrase,
					},
				}

				gomock.InOrder(
					mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(true, nil),
					mockMounter.EXPECT().ExistsPath(gomock.Eq(targetPath)).Return(false, nil),
				)

				mockMounter.EXPECT().MakeDir(targetPath).Return(nil)
				mockMounter.EXPECT().GetDeviceName(targetPath).Return("", 1, nil)
				// Check Luks (it is already format)
				mockMounter.EXPECT().IsLuks(gomock.Eq(devicePath)).Return(true)
				mockMounter.EXPECT().CheckLuksPassphrase(gomock.Eq(devicePath), gomock.Eq(passphrase)).Return(nil)
				mockMounter.EXPECT().LuksOpen(gomock.Eq(devicePath), gomock.Eq(encryptedDeviceName), gomock.Eq(passphrase))

				// Format opened luks device
				mockMounter.EXPECT().GetDiskFormat(gomock.Eq(encryptedDevicePath)).Return(defaultFsType, nil)
				mockMounter.EXPECT().FormatAndMount(gomock.Eq(encryptedDevicePath), gomock.Eq(targetPath), gomock.Eq(defaultFsType), gomock.Any())
				_, err := oscDriver.NodeStageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestNodeUnstageVolume(t *testing.T) {
	targetPath := "/test/path"
	devicePath := "/dev/fake"
	encryptedDeviceName := "fake_crypt"

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				mockMounter.EXPECT().GetDeviceName(gomock.Eq(targetPath)).Return(devicePath, 1, nil)
				mockMounter.EXPECT().Unmount(gomock.Eq(targetPath)).Return(nil)
				mockMounter.EXPECT().IsLuksMapping(gomock.Eq(devicePath)).Return(false, "", nil)

				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
					VolumeId:          "vol-test",
				}

				_, err := oscDriver.NodeUnstageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success no device mounted at target",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				mockMounter.EXPECT().GetDeviceName(gomock.Eq(targetPath)).Return(devicePath, 0, nil)

				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
					VolumeId:          "vol-test",
				}
				_, err := oscDriver.NodeUnstageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success device mounted at multiple targets",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				mockMounter.EXPECT().GetDeviceName(gomock.Eq(targetPath)).Return(devicePath, 2, nil)
				mockMounter.EXPECT().Unmount(gomock.Eq(targetPath)).Return(nil)
				mockMounter.EXPECT().IsLuksMapping(gomock.Eq(devicePath)).Return(false, "", nil)

				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
					VolumeId:          "vol-test",
				}

				_, err := oscDriver.NodeUnstageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "fail no VolumeId",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
				}

				_, err := oscDriver.NodeUnstageVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail no StagingTargetPath",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeUnstageVolumeRequest{
					VolumeId: "vol-test",
				}
				_, err := oscDriver.NodeUnstageVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail GetDeviceName returns error",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				mockMounter.EXPECT().GetDeviceName(gomock.Eq(targetPath)).Return("", 0, errors.New("GetDeviceName faield"))

				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
					VolumeId:          "vol-test",
				}

				_, err := oscDriver.NodeUnstageVolume(context.TODO(), req)
				expectErr(t, err, codes.Internal)
			},
		},
		{
			name: "success encryption",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				mockMounter.EXPECT().GetDeviceName(gomock.Eq(targetPath)).Return(devicePath, 1, nil)
				mockMounter.EXPECT().Unmount(gomock.Eq(targetPath)).Return(nil)
				mockMounter.EXPECT().IsLuksMapping(gomock.Eq(devicePath)).Return(true, encryptedDeviceName, nil)
				mockMounter.EXPECT().LuksClose(gomock.Eq(encryptedDeviceName)).Return(nil)
				req := &csi.NodeUnstageVolumeRequest{
					StagingTargetPath: targetPath,
					VolumeId:          "vol-test",
				}

				_, err := oscDriver.NodeUnstageVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestNodePublishVolume(t *testing.T) {
	targetPath := "/test/path"
	stagingTargetPath := "/test/staging/path"
	devicePath := "/dev/fake"
	stdVolCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				mockMounter.EXPECT().MakeDir(gomock.Eq(targetPath)).Return(nil)
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Eq(targetPath)).Return(true, nil)
				mockMounter.EXPECT().Mount(gomock.Eq(stagingTargetPath), gomock.Eq(targetPath), gomock.Eq(defaultFsType), gomock.Eq([]string{"bind"})).Return(nil)

				req := &csi.NodePublishVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: stagingTargetPath,
					TargetPath:        targetPath,
					VolumeCapability:  stdVolCap,
					VolumeId:          "vol-test",
				}

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success normal idempotency",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				mockMounter.EXPECT().MakeDir(gomock.Eq(targetPath)).Return(nil)
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Eq(targetPath)).Return(false, nil)

				req := &csi.NodePublishVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: stagingTargetPath,
					TargetPath:        targetPath,
					VolumeCapability:  stdVolCap,
					VolumeId:          "vol-test",
				}

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success fstype",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				mockMounter.EXPECT().MakeDir(gomock.Eq(targetPath)).Return(nil)
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Eq(targetPath)).Return(true, nil)
				mockMounter.EXPECT().Mount(gomock.Eq(stagingTargetPath), gomock.Eq(targetPath), gomock.Eq(FSTypeXfs), gomock.Eq([]string{"bind"})).Return(nil)

				req := &csi.NodePublishVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: stagingTargetPath,
					TargetPath:        targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{
								FsType: FSTypeXfs,
							},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
				}

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success readonly",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				mockMounter.EXPECT().MakeDir(gomock.Eq(targetPath)).Return(nil)
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Eq(targetPath)).Return(true, nil)
				mockMounter.EXPECT().Mount(gomock.Eq(stagingTargetPath), gomock.Eq(targetPath), gomock.Eq(defaultFsType), gomock.Eq([]string{"bind", "ro"})).Return(nil)

				req := &csi.NodePublishVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					Readonly:          true,
					StagingTargetPath: stagingTargetPath,
					TargetPath:        targetPath,
					VolumeCapability:  stdVolCap,
					VolumeId:          "vol-test",
				}

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success mount options",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				mockMounter.EXPECT().MakeDir(gomock.Eq(targetPath)).Return(nil)
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Eq(targetPath)).Return(true, nil)
				mockMounter.EXPECT().Mount(gomock.Eq(stagingTargetPath), gomock.Eq(targetPath), gomock.Eq(defaultFsType), gomock.Eq([]string{"bind", "test-flag"})).Return(nil)

				req := &csi.NodePublishVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: "/dev/fake"},
					StagingTargetPath: stagingTargetPath,
					TargetPath:        targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{
								// this request will call mount with the bind option,
								// adding "bind" here we test that we don't add the
								// same option twice. "test-flag" is a canary to check
								// that the driver calls mount with that flag
								MountFlags: []string{"bind", "test-flag"},
							},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
				}

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success normal [raw block]",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				gomock.InOrder(
					mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(true, nil),
					mockMounter.EXPECT().ExistsPath(gomock.Eq("/test")).Return(false, nil),
				)
				mockMounter.EXPECT().MakeDir(gomock.Eq("/test")).Return(nil)
				mockMounter.EXPECT().MakeFile(targetPath).Return(nil)
				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Eq(targetPath)).Return(true, nil)
				mockMounter.EXPECT().Mount(gomock.Eq(devicePath), gomock.Eq(targetPath), gomock.Eq(""), gomock.Eq([]string{"bind"})).Return(nil)

				req := &csi.NodePublishVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: "/dev/fake"},
					StagingTargetPath: stagingTargetPath,
					TargetPath:        targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
				}

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "fail no device path [raw block]",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodePublishVolumeRequest{
					StagingTargetPath: stagingTargetPath,
					TargetPath:        targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
				}

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail to find deivce path [raw block]",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodePublishVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: "/dev/fake"},
					StagingTargetPath: stagingTargetPath,
					TargetPath:        targetPath,
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
						},
					},
					VolumeId: "vol-test",
				}

				mockMounter.EXPECT().ExistsPath(gomock.Eq(devicePath)).Return(false, errors.New("findDevicePath failed"))

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				expectErr(t, err, codes.Internal)
			},
		},
		{
			name: "fail no VolumeId",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodePublishVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: stagingTargetPath,
					TargetPath:        targetPath,
					VolumeCapability:  stdVolCap,
				}

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail no StagingTargetPath",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodePublishVolumeRequest{
					PublishContext:   map[string]string{DevicePathKey: devicePath},
					TargetPath:       targetPath,
					VolumeCapability: stdVolCap,
					VolumeId:         "vol-test",
				}

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail no TargetPath",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodePublishVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: stagingTargetPath,
					VolumeCapability:  stdVolCap,
					VolumeId:          "vol-test",
				}

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail no VolumeCapability",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodePublishVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: devicePath},
					StagingTargetPath: stagingTargetPath,
					TargetPath:        targetPath,
					VolumeId:          "vol-test",
				}
				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail invalid VolumeCapability",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodePublishVolumeRequest{
					PublishContext:    map[string]string{DevicePathKey: "/dev/fake"},
					StagingTargetPath: "/test/staging/path",
					TargetPath:        "/test/target/path",
					VolumeId:          "vol-test",
					VolumeCapability: &csi.VolumeCapability{
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_UNKNOWN,
						},
					},
				}

				_, err := oscDriver.NodePublishVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestNodeUnpublishVolume(t *testing.T) {
	targetPath := "/test/path"

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeUnpublishVolumeRequest{
					TargetPath: targetPath,
					VolumeId:   "vol-test",
				}

				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Eq(targetPath)).Return(false, nil)
				mockMounter.EXPECT().Unmount(gomock.Eq(targetPath)).Return(nil)
				_, err := oscDriver.NodeUnpublishVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "success normal idempotency",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeUnpublishVolumeRequest{
					TargetPath: targetPath,
					VolumeId:   "vol-test",
				}

				mockMounter.EXPECT().IsLikelyNotMountPoint(gomock.Eq(targetPath)).Return(true, nil)
				_, err := oscDriver.NodeUnpublishVolume(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "fail no VolumeId",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeUnpublishVolumeRequest{
					TargetPath: targetPath,
				}

				_, err := oscDriver.NodeUnpublishVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
		{
			name: "fail no TargetPath",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)

				oscDriver := &nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeUnpublishVolumeRequest{
					VolumeId: "vol-test",
				}

				_, err := oscDriver.NodeUnpublishVolume(context.TODO(), req)
				expectErr(t, err, codes.InvalidArgument)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestNodeGetVolumeStats(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success normal",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)
				VolumePath := t.TempDir()
				err := os.MkdirAll(VolumePath, 0644)
				require.NoError(t, err)

				mockMounter.EXPECT().ExistsPath(VolumePath).Return(true, nil)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeGetVolumeStatsRequest{
					VolumeId:   "vol-test",
					VolumePath: VolumePath,
				}
				_, err = oscDriver.NodeGetVolumeStats(context.TODO(), req)
				require.NoError(t, err)
			},
		},
		{
			name: "fail path not exist",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)
				VolumePath := "/test"

				mockMounter.EXPECT().ExistsPath(VolumePath).Return(false, nil)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeGetVolumeStatsRequest{
					VolumeId:   "vol-test",
					VolumePath: VolumePath,
				}
				_, err := oscDriver.NodeGetVolumeStats(context.TODO(), req)
				expectErr(t, err, codes.NotFound)
			},
		},
		{
			name: "fail can't determine block device",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)
				VolumePath := "/test"

				mockMounter.EXPECT().ExistsPath(VolumePath).Return(true, nil)

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeGetVolumeStatsRequest{
					VolumeId:   "vol-test",
					VolumePath: VolumePath,
				}
				_, err := oscDriver.NodeGetVolumeStats(context.TODO(), req)
				expectErr(t, err, codes.Internal)
			},
		},
		{
			name: "fail error calling existsPath",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				defer mockCtl.Finish()

				mockMetadata := mocks.NewMockMetadataService(mockCtl)
				mockMounter := mocks.NewMockMounter(mockCtl)
				VolumePath := "/test"

				mockMounter.EXPECT().ExistsPath(VolumePath).Return(false, errors.New("get existsPath call fail"))

				oscDriver := nodeService{
					metadata: mockMetadata,
					mounter:  mockMounter,
					inFlight: internal.NewInFlight(),
				}

				req := &csi.NodeGetVolumeStatsRequest{
					VolumeId:   "vol-test",
					VolumePath: VolumePath,
				}
				_, err := oscDriver.NodeGetVolumeStats(context.TODO(), req)
				expectErr(t, err, codes.Internal)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}

}

func TestNodeGetCapabilities(t *testing.T) {
	mockCtl := gomock.NewController(t)
	defer mockCtl.Finish()

	mockMetadata := mocks.NewMockMetadataService(mockCtl)
	mockMounter := mocks.NewMockMounter(mockCtl)

	oscDriver := nodeService{
		metadata: mockMetadata,
		mounter:  mockMounter,
		inFlight: internal.NewInFlight(),
	}

	caps := []*csi.NodeServiceCapability{
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
				},
			},
		},
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
				},
			},
		},
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
				},
			},
		},
	}
	req := &csi.NodeGetCapabilitiesRequest{}
	resp, err := oscDriver.NodeGetCapabilities(context.TODO(), req)
	require.NoError(t, err)
	assert.Equal(t, &csi.NodeGetCapabilitiesResponse{Capabilities: caps}, resp)
}

func TestNodeGetInfo(t *testing.T) {
	testCases := []struct {
		name             string
		instanceID       string
		instanceType     string
		availabilityZone string
		expMaxVolumes    int64
	}{
		{
			name:             "success normal",
			instanceID:       "i-123456789abcdef01",
			instanceType:     "t2.medium",
			availabilityZone: "us-west-2b",
			expMaxVolumes:    defaultMaxBSUVolumes,
		},
		{
			name:             "success normal with NVMe",
			instanceID:       "i-123456789abcdef01",
			instanceType:     "m5d.large",
			availabilityZone: "us-west-2b",
			expMaxVolumes:    defaultMaxBSUVolumes,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtl := gomock.NewController(t)
			defer mockCtl.Finish()

			mockMetadata := mocks.NewMockMetadataService(mockCtl)
			mockMetadata.EXPECT().GetInstanceID().Return(tc.instanceID)
			mockMetadata.EXPECT().GetAvailabilityZone().Return(tc.availabilityZone)

			mockMounter := mocks.NewMockMounter(mockCtl)

			oscDriver := &nodeService{
				metadata: mockMetadata,
				mounter:  mockMounter,
				inFlight: internal.NewInFlight(),
			}

			resp, err := oscDriver.NodeGetInfo(context.TODO(), &csi.NodeGetInfoRequest{})
			require.NoError(t, err)

			assert.Equal(t, tc.instanceID, resp.GetNodeId(), "Invalid node ID")

			at := resp.GetAccessibleTopology()
			assert.Equal(t, tc.availabilityZone, at.Segments[TopologyKey], "Invalid topology")

			assert.Equal(t, tc.expMaxVolumes, resp.GetMaxVolumesPerNode(), "Invalid max volumes per node")
		})
	}
}

func TestFindScsiName(t *testing.T) {
	findScsiNameCase := []struct {
		name                string
		devicePath          string
		scsiName            string
		expTestFindScsiName error
	}{
		{
			name:                "Validate format xvdx",
			devicePath:          "/dev/xvda",
			scsiName:            "scsi-0QEMU_QEMU_HARDDISK_sda",
			expTestFindScsiName: nil,
		},
		{
			name:                "Validate format xvdxy",
			devicePath:          "/dev/xvdaa",
			scsiName:            "scsi-0QEMU_QEMU_HARDDISK_sdaa",
			expTestFindScsiName: nil,
		},
		{
			name:                "Invalide format xvdxyz",
			devicePath:          "/dev/xvdaaa",
			scsiName:            "scsi-0QEMU_QEMU_HARDDISK_sd",
			expTestFindScsiName: fmt.Errorf("devicePath /dev/xvdaaa is not supported"),
		},
	}
	for _, fsnc := range findScsiNameCase {
		t.Run(fsnc.name, func(t *testing.T) {
			scsiName, err := findScsiName(fsnc.devicePath)
			if fsnc.expTestFindScsiName != nil {
				assert.EqualError(t, err, fsnc.expTestFindScsiName.Error())
			} else {
				assert.Equal(t, fsnc.scsiName, scsiName)
			}
		})
	}
}

func expectErr(t *testing.T, actualErr error, expectedCode codes.Code) {
	require.Error(t, actualErr)

	status, ok := status.FromError(actualErr)
	require.True(t, ok)

	assert.Equal(t, expectedCode, status.Code())
}
