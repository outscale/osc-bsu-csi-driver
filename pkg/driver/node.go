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
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/outscale-dev/osc-bsu-csi-driver/pkg/cloud"
	"github.com/outscale-dev/osc-bsu-csi-driver/pkg/driver/internal"
	"github.com/outscale-dev/osc-bsu-csi-driver/pkg/driver/luks"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/volume"
	mountutils "k8s.io/mount-utils"
)

const (
	// FSTypeExt2 represents the ext2 filesystem type
	FSTypeExt2 = "ext2"
	// FSTypeExt3 represents the ext3 filesystem type
	FSTypeExt3 = "ext3"
	// FSTypeExt4 represents the ext4 filesystem type
	FSTypeExt4 = "ext4"
	// FSTypeXfs represents te xfs filesystem type
	FSTypeXfs = "xfs"

	// default file system type to be used when it is not provided
	defaultFsType = FSTypeXfs

	// defaultMaxBSUVolumes is the maximum number of volumes that an OSC instance can have attached.
	// https://docs.outscale.com/en/userguide/About-Volumes.html#_volumes_and_instances
	defaultMaxBSUVolumes = 39
)

var (
	ValidFSTypes = []string{FSTypeExt2, FSTypeExt3, FSTypeExt4, FSTypeXfs}
)

var (
	// nodeCaps represents the capability of node service.
	nodeCaps = []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
	}
)

// nodeService represents the node service of CSI driver
type nodeService struct {
	metadata cloud.MetadataService
	mounter  Mounter
	inFlight *internal.InFlight
}

// newNodeService creates a new node service
// it panics if failed to create the service
func newNodeService() nodeService {
	metadata, err := cloud.NewMetadata()
	if err != nil {
		panic(err)
	}

	return nodeService{
		metadata: metadata,
		mounter:  newNodeMounter(),
		inFlight: internal.NewInFlight(),
	}
}

func (d *nodeService) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	klog.V(4).Infof("NodeStageVolume: called with args  {VolumeId:%+v, PublishContext:%+v, StagingTargetPath:%+v, VolumeCapability:%+v, VolumeContext:%+v}", req.VolumeId, req.PublishContext, req.StagingTargetPath, req.VolumeCapability, req.VolumeContext)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	target := req.GetStagingTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	if !isValidVolumeCapabilities([]*csi.VolumeCapability{volCap}) {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not supported")
	}

	// If the access type is block, do nothing for stage
	switch volCap.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		return &csi.NodeStageVolumeResponse{}, nil
	}

	mount := volCap.GetMount()
	if mount == nil {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume: mount is nil within volume capability")
	}

	fsType := mount.GetFsType()
	if len(fsType) == 0 {
		fsType = defaultFsType
	}

	var mountOptions []string
	for _, f := range mount.MountFlags {
		if !hasMountOption(mountOptions, f) {
			mountOptions = append(mountOptions, f)
		}
	}

	if ok := d.inFlight.Insert(req); !ok {
		msg := fmt.Sprintf("request to stage volume=%q is already in progress", volumeID)
		return nil, status.Error(codes.Internal, msg)
	}
	defer func() {
		klog.V(4).Infof("NodeStageVolume: volume=%q operation finished", req.GetVolumeId())
		d.inFlight.Delete(req)
		klog.V(4).Info("donedone")
	}()

	devicePath, ok := req.PublishContext[DevicePathKey]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "Device path not provided")
	}

	source, err := d.findDevicePath(devicePath, volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to find device path %s. %v", devicePath, err)
	}

	klog.V(4).Infof("NodeStageVolume: find device path %s -> %s", devicePath, source)

	exists, err := d.mounter.ExistsPath(target)
	if err != nil {
		msg := fmt.Sprintf("failed to check if target %q exists: %v", target, err)
		return nil, status.Error(codes.Internal, msg)
	}
	// When exists is true it means target path was created but device isn't mounted.
	// We don't want to do anything in that case and let the operation proceed.
	// Otherwise we need to create the target directory.
	if !exists {
		// If target path does not exist we need to create the directory where volume will be staged
		klog.V(4).Infof("NodeStageVolume: creating target dir %q", target)
		if err = d.mounter.MakeDir(target); err != nil {
			msg := fmt.Sprintf("could not create target dir %q: %v", target, err)
			return nil, status.Error(codes.Internal, msg)
		}
	}

	// Check if a device is mounted in target directory
	device, _, err := d.mounter.GetDeviceName(target)
	if err != nil {
		msg := fmt.Sprintf("failed to check if volume is already mounted: %v", err)
		return nil, status.Error(codes.Internal, msg)
	}

	isEncrypted := false
	if encrypted, ok := req.PublishContext[EncryptedKey]; ok {
		isEncrypted = encrypted == "true"
	}

	var encryptedDeviceName string
	var encryptedDevicePath string

	if isEncrypted {
		klog.V(4).Info("NodeStageVolume: The device must be encrypted")
		// Check that the device is already mounted
		encryptedDeviceName = fmt.Sprintf("%v_crypt", filepath.Base(source))
		encryptedDevicePath = fmt.Sprintf("/dev/mapper/%v", encryptedDeviceName)

		if device == encryptedDevicePath {
			klog.V(4).Infof("NodeStageVolume: volume=%q already staged (encryption)", volumeID)
			return &csi.NodeStageVolumeResponse{}, nil
		}

		passphrase, ok := req.Secrets[LuksPassphraseKey]
		if !ok {
			return nil, status.Error(codes.InvalidArgument, "no passphrase key has been provided")
		}

		// Check if the disk needs encryption
		if !d.mounter.IsLuks(source) {
			klog.V(4).Info("NodeStageVolume: The device  does not have a luks format")
			// It is not a luks device => format
			luksCipher := req.PublishContext[LuksCipherKey]
			luksHash := req.PublishContext[LuksHashKey]
			luksKeySize := req.PublishContext[LuksKeySizeKey]

			if d.mounter.LuksFormat(source, passphrase, luks.LuksContext{Cipher: luksCipher, KeySize: luksKeySize, Hash: luksHash}) != nil {
				msg := fmt.Sprintf("error while formating luks partition to %v, err: %v", volumeID, err)
				return nil, status.Error(codes.Internal, msg)
			}
		}

		// Check passphrase
		if ok := d.mounter.CheckLuksPassphrase(source, passphrase); !ok {
			msg := fmt.Sprintf("error while checking passphrase to %v, err: %v", volumeID, err)
			return nil, status.Error(codes.Internal, msg)
		}

		// Open disk
		if _, err := d.mounter.LuksOpen(source, encryptedDeviceName, passphrase); err != nil {
			msg := fmt.Sprintf("error while opening luks device to %v, err: %v", volumeID, err)
			return nil, status.Error(codes.Internal, msg)
		}

		source = encryptedDevicePath

	} else {
		// This operation (NodeStageVolume) MUST be idempotent.
		// If the volume corresponding to the volume_id is already staged to the staging_target_path,
		// and is identical to the specified volume_capability the Plugin MUST reply 0 OK.
		if device == source {
			klog.V(4).Infof("NodeStageVolume: volume=%q already staged", volumeID)
			return &csi.NodeStageVolumeResponse{}, nil
		}
	}

	existingFormat, err := d.mounter.GetDiskFormat(source)
	if err != nil {
		msg := ""
		if isEncrypted {
			if closeError := d.mounter.LuksClose(encryptedDeviceName); closeError != nil {
				msg = fmt.Sprintf("error when closing the disk but ignoring (%v) and ", closeError)
			}
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("%vfailed to get disk format of disk %q: %v", msg, source, err))
	}

	if existingFormat != "" && existingFormat != fsType {
		if len(mount.GetFsType()) == 0 {
			// The default FStype will break the disk, switching to existingFormat
			klog.Warningf("NodeStageVolume: The default fstype %q does not match the fstype of the disk %q. Please update your StorageClass.", defaultFsType, existingFormat)
			fsType = existingFormat
		} else {
			msg := ""
			if isEncrypted {
				if closeError := d.mounter.LuksClose(encryptedDeviceName); closeError != nil {
					msg = fmt.Sprintf("error when closing the disk but ignoring (%v) and ", closeError)
				}
			}
			return nil, status.Error(codes.Internal, fmt.Sprintf("NodeStageVolume: %vThe requested fstype %q does not match the fstype of the disk %q", msg, fsType, existingFormat))
		}
	}

	klog.V(5).Infof("NodeStageVolume: formatting %s and mounting at %s with fstype %s", source, target, fsType)
	if FSTypeXfs == fsType {
		if existingFormat == "" {
			argsXfs := []string{source}
			klog.V(5).Infof("NodeStageVolume: xfs case mkfs %v ", argsXfs)
			cmdOut, cmdErr := d.mounter.Command("mkfs.xfs", argsXfs...).CombinedOutput()
			if cmdErr != nil {
				klog.V(5).Infof("NodeStageVolume: continue with failed to run mkfs %v, error: %v, output: %v", argsXfs, cmdErr, cmdOut)
				// but continue with FormatAndMount
			}
		}
	}

	// FormatAndMount will format only if needed
	err = d.mounter.FormatAndMount(source, target, fsType, mountOptions)
	if err != nil {
		msg := ""
		if isEncrypted {
			if closeError := d.mounter.LuksClose(encryptedDeviceName); closeError != nil {
				msg = fmt.Sprintf("error when closing the disk but ignoring (%v) and ", closeError)
			}
		}
		msg = fmt.Sprintf("%vcould not format %q and mount it at %q", msg, source, target)
		return nil, status.Error(codes.Internal, msg)
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (d *nodeService) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	klog.V(4).Infof("NodeUnstageVolume: called with args %+v", *req)
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	target := req.GetStagingTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	// Check if target directory is a mount point. GetDeviceNameFromMount
	// given a mnt point, finds the device from /proc/mounts
	// returns the device name, reference count, and error code
	dev, refCount, err := d.mounter.GetDeviceName(target)
	if err != nil {
		msg := fmt.Sprintf("failed to check if volume is mounted: %v", err)
		return nil, status.Error(codes.Internal, msg)
	}

	// From the spec: If the volume corresponding to the volume_id
	// is not staged to the staging_target_path, the Plugin MUST
	// reply 0 OK.
	if refCount == 0 {
		klog.V(5).Infof("NodeUnstageVolume: %s target not mounted", target)
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	if refCount > 1 {
		klog.Warningf("NodeUnstageVolume: found %d references to device %s mounted at target path %s", refCount, dev, target)
	}

	klog.V(5).Infof("NodeUnstageVolume: unmounting %s", target)
	err = d.mounter.Unmount(target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not unmount target %q: %v", target, err)
	}

	// Check Encryption
	isLuksMapping, mappingName, err := d.mounter.IsLuksMapping(dev)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine if it is a luks mapping for target %q: %v", target, err)
	}

	if isLuksMapping {
		if err = d.mounter.LuksClose(mappingName); err != nil {
			msg := fmt.Sprintf("failed to close device: %v,", err)
			return nil, status.Error(codes.Internal, msg)
		}
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (d *nodeService) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	klog.V(4).Infof("NodeExpandVolume: called with args %+v", *req)
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	volumePath := req.GetVolumePath()
	if len(volumePath) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "Volume Path not provided")
	}

	deviceName, _, err := d.mounter.GetDeviceName(volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine device path: %v", err)
	}

	if len(deviceName) == 0 {
		return nil, status.Errorf(codes.NotFound, "Could not get valid device name for mount path: %q", volumePath)
	}

	devicePath, err := d.findDevicePath(deviceName, volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get valid device path for mount path: %q", req.GetVolumePath())
	}

	isLuksMapping, mappingName, err := d.mounter.IsLuksMapping(devicePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine if it is a luks mapping for target %q: %v", volumeID, err)
	}

	if isLuksMapping {
		if err := d.mounter.LuksResize(mappingName); err != nil {
			return nil, status.Errorf(codes.Internal, "Could not resize Luks volume %q: %v", volumeID, err)
		}
	}

	// TODO: refactor Mounter to expose a mount.SafeFormatAndMount object
	r := mountutils.NewResizeFs(d.mounter)

	// TODO: lock per volume ID to have some idempotency
	if _, err := r.Resize(devicePath, volumePath); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not resize volume %q (%q):  %v", volumeID, devicePath, err)
	}

	return &csi.NodeExpandVolumeResponse{}, nil
}

func (d *nodeService) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.V(4).Infof("NodePublishVolume: called with args %+v", *req)
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	source := req.GetStagingTargetPath()
	if len(source) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	if !isValidVolumeCapabilities([]*csi.VolumeCapability{volCap}) {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not supported")
	}

	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	switch mode := volCap.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		if err := d.nodePublishVolumeForBlock(req, mountOptions); err != nil {
			return nil, err
		}
	case *csi.VolumeCapability_Mount:
		if err := d.nodePublishVolumeForFileSystem(req, mountOptions, mode); err != nil {
			return nil, err
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *nodeService) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(4).Infof("NodeUnpublishVolume: called with args %+v", *req)
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	isMounted, err := d.isMounted(target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not check if %q is mounted: %v", target, err)

	}

	if isMounted {
		klog.V(5).Infof("NodeUnpublishVolume: unmounting %s", target)
		err = d.mounter.Unmount(target)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not unmount %q: %v", target, err)
		}

	}

	// Remove the path
	if _, err1 := os.Stat(target); !os.IsNotExist(err1) {
		if err := os.Remove(target); err != nil {
			return nil, status.Errorf(codes.Internal, "Could not remove folder %q: %v", target, err)
		}
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (d *nodeService) isBlockDevice(filename string) (bool, error) {
	// Use stat to determine the kind of file this is.
	var stat unix.Stat_t

	err := unix.Stat(filename, &stat)
	if err != nil {
		return false, err
	}

	return (stat.Mode & unix.S_IFMT) == unix.S_IFBLK, nil
}

func (d *nodeService) getBlockSizeBytes(devicePath string) (int64, error) {
	cmd := d.mounter.Command("blockdev", "--getsize64", devicePath)
	output, err := cmd.Output()
	if err != nil {
		return -1, fmt.Errorf("error when getting size of block volume at path %s: output: %s, err: %v", devicePath, string(output), err)
	}
	strOut := strings.TrimSpace(string(output))
	gotSizeBytes, err := strconv.ParseInt(strOut, 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse size %s as int", strOut)
	}
	return gotSizeBytes, nil
}

func (d *nodeService) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {

	klog.V(4).Infof("NodeGetVolumeStats: called with args %+v", *req)
	if len(req.VolumeId) == 0 {
		klog.V(4).Infof("NodeGetVolumeStats: called with args")
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats empty Volume ID")
	}

	if len(req.VolumePath) == 0 {
		klog.V(4).Infof("NodeGetVolumeStats empty Volume path")
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats empty Volume path")
	}

	exists, err := d.mounter.ExistsPath(req.VolumePath)
	if err != nil {
		klog.V(4).Infof("unknown error when stat")
		return nil, status.Errorf(codes.Internal, "unknown error when stat on %s: %v", req.VolumePath, err)
	}
	if !exists {
		klog.V(4).Infof("path does not exist")
		return nil, status.Errorf(codes.NotFound, "path %s does not exist", req.VolumePath)
	}

	isBlock, err := d.isBlockDevice(req.VolumePath)
	klog.V(4).Infof("isBlockDevice %v, %v", isBlock, err)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to determine whether %s is block device: %v", req.VolumePath, err)
	}

	if isBlock {
		bcap, err := d.getBlockSizeBytes(req.VolumePath)
		if err != nil {
			klog.V(4).Infof("failed to get block capacity on path")
			return nil, status.Errorf(codes.Internal, "failed to get block capacity on path %s: %v", req.VolumePath, err)
		}
		klog.V(4).Infof("bcap %+v", csi.NodeGetVolumeStatsResponse{
			Usage: []*csi.VolumeUsage{
				{
					Unit:  csi.VolumeUsage_BYTES,
					Total: bcap,
				},
			},
		})
		return &csi.NodeGetVolumeStatsResponse{
			Usage: []*csi.VolumeUsage{
				{
					Unit:  csi.VolumeUsage_BYTES,
					Total: bcap,
				},
			},
		}, nil
	}

	metricsProvider := volume.NewMetricsStatFS(req.VolumePath)
	metrics, err := metricsProvider.GetMetrics()
	if err != nil {
		klog.V(4).Infof("failed to get fs info on path %s: %v", req.VolumePath, err)
		return nil, status.Errorf(codes.Internal, "failed to get fs info on path %s: %v", req.VolumePath, err)
	}

	klog.V(4).Infof("NodeGetVolumeStatsResponse: %+v", csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Unit:      csi.VolumeUsage_BYTES,
				Available: metrics.Available.AsDec().UnscaledBig().Int64(),
				Total:     metrics.Capacity.AsDec().UnscaledBig().Int64(),
				Used:      metrics.Used.AsDec().UnscaledBig().Int64(),
			},
			{
				Unit:      csi.VolumeUsage_INODES,
				Available: metrics.InodesFree.AsDec().UnscaledBig().Int64(),
				Total:     metrics.Inodes.AsDec().UnscaledBig().Int64(),
				Used:      metrics.InodesUsed.AsDec().UnscaledBig().Int64(),
			},
		},
	})

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Unit:      csi.VolumeUsage_BYTES,
				Available: metrics.Available.AsDec().UnscaledBig().Int64(),
				Total:     metrics.Capacity.AsDec().UnscaledBig().Int64(),
				Used:      metrics.Used.AsDec().UnscaledBig().Int64(),
			},
			{
				Unit:      csi.VolumeUsage_INODES,
				Available: metrics.InodesFree.AsDec().UnscaledBig().Int64(),
				Total:     metrics.Inodes.AsDec().UnscaledBig().Int64(),
				Used:      metrics.InodesUsed.AsDec().UnscaledBig().Int64(),
			},
		},
	}, nil
}

func (d *nodeService) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	klog.V(4).Infof("NodeGetCapabilities: called with args %+v", *req)
	var caps []*csi.NodeServiceCapability
	for _, cap := range nodeCaps {
		c := &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: cap,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.NodeGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (d *nodeService) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	klog.V(4).Infof("NodeGetInfo: called with args %+v", *req)

	topology := &csi.Topology{
		Segments: map[string]string{TopologyKey: d.metadata.GetAvailabilityZone()},
	}

	return &csi.NodeGetInfoResponse{
		NodeId:             d.metadata.GetInstanceID(),
		MaxVolumesPerNode:  d.getVolumesLimit(),
		AccessibleTopology: topology,
	}, nil
}

func (d *nodeService) nodePublishVolumeForBlock(req *csi.NodePublishVolumeRequest, mountOptions []string) error {
	target := req.GetTargetPath()
	volumeID := req.GetVolumeId()

	devicePath, exists := req.PublishContext[DevicePathKey]
	if !exists {
		return status.Error(codes.InvalidArgument, "Device path not provided")
	}
	source, err := d.findDevicePath(devicePath, volumeID)
	if err != nil {
		return status.Errorf(codes.Internal, "Failed to find device path %s. %v", devicePath, err)
	}

	klog.V(4).Infof("NodePublishVolume [block]: find device path %s -> %s", devicePath, source)

	globalMountPath := filepath.Dir(target)

	// create the global mount path if it is missing
	// Path in the form of /var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/{volumeName}
	exists, err = d.mounter.ExistsPath(globalMountPath)
	if err != nil {
		return status.Errorf(codes.Internal, "Could not check if path exists %q: %v", globalMountPath, err)
	}

	if !exists {
		if err := d.mounter.MakeDir(globalMountPath); err != nil {
			return status.Errorf(codes.Internal, "Could not create dir %q: %v", globalMountPath, err)
		}
	}

	// Create the mount point as a file since bind mount device node requires it to be a file
	klog.V(5).Infof("NodePublishVolume [block]: making target file %s", target)
	err = d.mounter.MakeFile(target)
	if err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return status.Errorf(codes.Internal, "Could not remove mount target %q: %v", target, removeErr)
		}
		return status.Errorf(codes.Internal, "Could not create file %q: %v", target, err)
	}

	isMounted, err := d.isMounted(target)
	if err != nil {
		return status.Errorf(codes.Internal, "Could not check if %q is mounted: %v", target, err)
	}

	if isMounted {
		return nil
	}

	klog.V(5).Infof("NodePublishVolume [block]: mounting %s at %s", source, target)
	if err := d.mounter.Mount(source, target, "", mountOptions); err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return status.Errorf(codes.Internal, "Could not remove mount target %q: %v", target, removeErr)
		}
		return status.Errorf(codes.Internal, "Could not mount %q at %q: %v", source, target, err)
	}

	return nil
}

func (d *nodeService) nodePublishVolumeForFileSystem(req *csi.NodePublishVolumeRequest, mountOptions []string, mode *csi.VolumeCapability_Mount) error {
	target := req.GetTargetPath()
	source := req.GetStagingTargetPath()
	if m := mode.Mount; m != nil {
		for _, f := range m.MountFlags {
			if !hasMountOption(mountOptions, f) {
				mountOptions = append(mountOptions, f)
			}
		}
	}

	klog.V(5).Infof("NodePublishVolume: creating dir %s", target)
	if err := d.mounter.MakeDir(target); err != nil {
		return status.Errorf(codes.Internal, "Could not create dir %q: %v", target, err)
	}

	fsType := mode.Mount.GetFsType()
	if len(fsType) == 0 {
		fsType = defaultFsType
	}

	isMounted, err := d.isMounted(target)
	if err != nil {
		return status.Errorf(codes.Internal, "Could not check if %q is mounted: %v", target, err)
	}

	if isMounted {
		return nil
	}

	klog.V(5).Infof("NodePublishVolume: mounting %s at %s with option %s as fstype %s", source, target, mountOptions, fsType)
	if err := d.mounter.Mount(source, target, fsType, mountOptions); err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return status.Errorf(codes.Internal, "Could not remove mount target %q: %v", target, err)
		}
		return status.Errorf(codes.Internal, "Could not mount %q at %q: %v", source, target, err)
	}

	return nil
}

// findDevicePath finds path of device and verifies its existence
// if the device is not nvme, return the path directly
// if the device is nvme, finds and returns the nvme device path eg. /dev/nvme1n1
func (d *nodeService) findDevicePath(devicePath, volumeID string) (string, error) {
	exists, err := d.mounter.ExistsPath(devicePath)
	if err != nil {
		return "", err
	}

	// If the path exists, assume it is not nvme device
	if exists {
		return devicePath, nil
	}

	// assumption it is a scsi volume for 3DS env
	scsiName, err := findScsiName(devicePath)
	if err != nil {
		return "", err
	}

	klog.V(4).Infof("findDevicePath: check if scsi device for %s is %s and return the device", devicePath, scsiName)
	return findScsiVolume(scsiName)
}

func findScsiName(devicePath string) (string, error) {
	myreg := regexp.MustCompile(`^/dev/xvd(?P<suffix>[a-z]{1,2})$`)
	match := myreg.FindStringSubmatch(devicePath)
	result := make(map[string]string)
	if myreg.MatchString(devicePath) {
		for i, name := range myreg.SubexpNames() {
			if i != 0 && name != "" {
				result[name] = match[i]
			}
		}
		scsiName := "scsi-0QEMU_QEMU_HARDDISK_sd" + result["suffix"]
		return scsiName, nil
	}
	return "", fmt.Errorf("devicePath %s is not supported", devicePath)
}

func findScsiVolume(findName string) (device string, err error) {
	p := filepath.Join("/dev/disk/by-id/", findName)
	stat, err := os.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			klog.V(5).Infof("scsi path %q not found", p)
			return "", fmt.Errorf("scsi path %q not found", p)
		}
		return "", fmt.Errorf("error getting stat of %q: %v", p, err)
	}

	if stat.Mode()&os.ModeSymlink != os.ModeSymlink {
		klog.Warningf("scsi file %q found, but was not a symlink", p)
		return "", fmt.Errorf("scsi file %q found, but was not a symlink", p)
	}
	// Find the target, resolving to an absolute path
	// scsi-0QEMU_QEMU_HARDDISK_sdb -> ../../sdb
	// scsi-0QEMU_QEMU_HARDDISK_sde -> ../../sda
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("error reading target of symlink %q: %v", p, err)
	}

	if !strings.HasPrefix(resolved, "/dev") {
		return "", fmt.Errorf("resolved symlink for %q was unexpected: %q", p, resolved)
	}
	return resolved, nil
}

// getVolumesLimit returns the limit of volumes that the node supports
func (d *nodeService) getVolumesLimit() int64 {
	value := os.Getenv("MAX_BSU_VOLUMES")
	if value == "" {
		return defaultMaxBSUVolumes
	}
	max_value, err := strconv.Atoi(value)
	if err != nil {
		return defaultMaxBSUVolumes
	}
	return int64(max_value)
}

// hasMountOption returns a boolean indicating whether the given
// slice already contains a mount option. This is used to prevent
// passing duplicate option to the mount command.
func hasMountOption(options []string, opt string) bool {
	for _, o := range options {
		if o == opt {
			return true
		}
	}
	return false
}

// isMounted checks if target is mounted. It does NOT return an error if target
// doesn't exist.
func (d *nodeService) isMounted(target string) (bool, error) {
	/*
		Checking if it's a mount point using IsLikelyNotMountPoint. There are three different return values,
		1. true, err when the directory does not exist or corrupted.
		2. false, nil when the path is already mounted with a device.
		3. true, nil when the path is not mounted with any device.
	*/
	notMnt, err := d.mounter.IsLikelyNotMountPoint(target)
	if err != nil && !os.IsNotExist(err) {
		//Checking if the path exists and error is related to Corrupted Mount, in that case, the system could unmount and mount.
		_, pathErr := d.mounter.ExistsPath(target)
		if pathErr != nil && d.mounter.IsCorruptedMnt(pathErr) {
			klog.V(4).Infof("NodePublishVolume: Target path %q is a corrupted mount. Trying to unmount.", target)
			if mntErr := d.mounter.Unmount(target); mntErr != nil {
				return false, status.Errorf(codes.Internal, "Unable to unmount the target %q : %v", target, mntErr)
			}
			//After successful unmount, the device is ready to be mounted.
			return false, nil
		}
		return false, status.Errorf(codes.Internal, "Could not check if %q is a mount point: %v, %v", target, err, pathErr)
	}

	// Do not return os.IsNotExist error. Other errors were handled above.  The
	// Existence of the target should be checked by the caller explicitly and
	// independently because sometimes prior to mount it is expected not to exist
	// (in Windows, the target must NOT exist before a symlink is created at it)
	// and in others it is an error (in Linux, the target mount directory must
	// exist before mount is called on it)
	if err != nil && os.IsNotExist(err) {
		klog.V(5).Infof("[Debug] NodePublishVolume: Target path %q does not exist", target)
		return false, nil
	}

	if !notMnt {
		klog.V(4).Infof("NodePublishVolume: Target path %q is already mounted", target)
	}

	return !notMnt, nil
}
