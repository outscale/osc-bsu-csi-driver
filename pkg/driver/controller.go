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
	"slices"
	"strconv"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud"
	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog/v2"
)

var (
	// supportedVolumeModes is the list of supported access modes (SINGLE_NODE_WRITER).
	// BSU volumes can only be attached to a single node at any given time.
	supportedVolumeModes = []csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}

	// controllerCaps represents the capability of controller service
	controllerCaps = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_MODIFY_VOLUME,
	}
)

// controllerService represents the controller service of CSI driver
type controllerService struct {
	cloud         cloud.Cloud
	driverOptions *DriverOptions

	csi.UnimplementedControllerServer
}

// newControllerService creates a new controller service
// it panics if failed to create the service
func newControllerService(ctx context.Context, driverOptions *DriverOptions) controllerService {
	cloud, err := cloud.NewCloud(ctx, driverOptions.cloudOptions)
	if err != nil {
		panic(err)
	}

	return controllerService{
		cloud:         cloud,
		driverOptions: driverOptions,
	}
}

func (c *controllerService) Start(ctx context.Context) error {
	if c.cloud == nil {
		return nil
	}
	c.cloud.Start(ctx)
	return nil
}

func (d *controllerService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	volName := req.GetName()
	if volName == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume name not provided")
	}

	volSizeBytes, err := getVolSizeBytes(req)
	if err != nil {
		return nil, err
	}

	volCaps := req.GetVolumeCapabilities()
	if len(volCaps) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities not provided")
	}

	if !isValidVolumeCapabilities(volCaps) {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities not supported")
	}

	var (
		volumeType         osc.VolumeType
		iopsPerGB          int64
		isEncrypted        bool
		kmsKeyID           string
		luksCipher         string
		luksHash           string
		luksKeySize        string
		volumeContextExtra map[string]string
	)

	for key, value := range util.GetUpdatedParameters(req) {
		switch strings.ToLower(key) {
		case "fstype":
			klog.FromContext(ctx).V(2).Info(`"fstype" is deprecated, please use "csi.storage.k8s.io/fstype" instead`)
		case VolumeTypeKey:
			volumeType = osc.VolumeType(value)
		case IopsPerGBKey:
			iopsPerGB, err = strconv.ParseInt(value, 10, 32)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "Could not parse invalid iopsPerGB: %v", err)
			}
		case EncryptedKey:
			if value == "true" {
				isEncrypted = true
			} else {
				isEncrypted = false
			}
		case KmsKeyIDKey:
			kmsKeyID = value
		case LuksCipherKey:
			luksCipher = value
		case LuksKeySizeKey:
			luksKeySize = value
		case LuksHashKey:
			luksHash = value
		default:
			return nil, status.Errorf(codes.InvalidArgument, "Invalid parameter key %s for CreateVolume", key)
		}
	}

	// Check for encryption parameters
	if isEncrypted {
		volumeContextExtra = map[string]string{
			EncryptedKey:   strconv.FormatBool(isEncrypted),
			LuksHashKey:    luksHash,
			LuksCipherKey:  luksCipher,
			LuksKeySizeKey: luksKeySize,
		}
	} else {
		volumeContextExtra = map[string]string{}
	}

	var snapshotID *string
	volumeSource := req.GetVolumeContentSource()
	if volumeSource != nil {
		if _, ok := volumeSource.GetType().(*csi.VolumeContentSource_Snapshot); !ok {
			return nil, status.Error(codes.InvalidArgument, "Unsupported volumeContentSource type")
		}
		sourceSnapshot := volumeSource.GetSnapshot()
		if sourceSnapshot == nil {
			return nil, status.Error(codes.InvalidArgument, "Error retrieving snapshot from the volumeContentSource")
		}
		snapshotID = ptr.To(sourceSnapshot.GetSnapshotId())
	}

	vol, err := d.cloud.CheckCreatedVolume(ctx, volName)
	switch {
	case errors.Is(err, cloud.ErrNotFound):
	case errors.Is(err, cloud.ErrMultiVolumes):
		return nil, status.Error(codes.Internal, err.Error())
	case err != nil:
		return nil, status.Error(codes.Internal, err.Error())
	case vol.CapacityGiB != util.BytesToGiB(volSizeBytes):
		return nil, status.Error(codes.AlreadyExists, "Volume already exists, but with different size")
	case !ptr.Equal(vol.SnapshotID, snapshotID):
		return nil, status.Error(codes.AlreadyExists, "Volume already exists, but was restored from a different snapshot")
	default:
		klog.FromContext(ctx).V(3).Info("Volume already exists")
		return newCreateVolumeResponse(vol, volumeContextExtra), nil
	}

	// create a new volume
	zone := pickAvailabilityZone(req.GetAccessibilityRequirements())

	opts := &cloud.VolumeOptions{
		CapacityBytes: volSizeBytes,
		Tags:          d.driverOptions.extraVolumeTags,
		VolumeType:    volumeType,
		IOPSPerGB:     int(iopsPerGB),
		SubRegion:     zone,
		Encrypted:     isEncrypted,
		KmsKeyID:      kmsKeyID,
		SnapshotID:    snapshotID,
	}

	vol, err = d.cloud.CreateVolume(ctx, volName, opts)
	if err != nil {
		return nil, status.Errorf(cloud.GRPCCode(err), "Could not create volume %q: %v", volName, err)
	}
	return newCreateVolumeResponse(vol, volumeContextExtra), nil
}

func (d *controllerService) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	if _, err := d.cloud.DeleteVolume(ctx, volumeID); err != nil {
		if errors.Is(err, cloud.ErrNotFound) {
			klog.FromContext(ctx).V(3).Info("Volume already deleted")
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, status.Errorf(cloud.GRPCCode(err), "Could not delete volume ID %q: %v", volumeID, err)
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (d *controllerService) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	nodeID := req.GetNodeId()
	if nodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Node ID not provided")
	}

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	caps := []*csi.VolumeCapability{volCap}
	if !isValidVolumeCapabilities(caps) {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not supported")
	}

	if !d.cloud.ExistsInstance(ctx, nodeID) {
		return nil, status.Errorf(codes.NotFound, "Instance %q not found", nodeID)
	}

	devicePath, err := d.cloud.AttachVolume(ctx, volumeID, nodeID)
	if err != nil {
		return nil, status.Errorf(cloud.GRPCCode(err), "Could not attach volume %q to node %q: %v", volumeID, nodeID, err)
	}
	klog.FromContext(ctx).V(3).Info("Volume attached", "device", devicePath)

	volumeContext := req.GetVolumeContext()
	if volumeContext == nil {
		volumeContext = map[string]string{}
	}
	volumeContext[DevicePathKey] = devicePath
	return &csi.ControllerPublishVolumeResponse{PublishContext: volumeContext}, nil
}

func (d *controllerService) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	nodeID := req.GetNodeId()
	if nodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Node ID not provided")
	}

	if err := d.cloud.DetachVolume(ctx, volumeID, nodeID); err != nil {
		if errors.Is(err, cloud.ErrNotFound) {
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, status.Errorf(cloud.GRPCCode(err), "Could not detach volume %q from node %q: %v", volumeID, nodeID, err)
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (d *controllerService) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	caps := make([]*csi.ControllerServiceCapability, 0, len(controllerCaps))
	for _, cap := range controllerCaps {
		c := &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (d *controllerService) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	volCaps := req.GetVolumeCapabilities()
	if len(volCaps) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities not provided")
	}

	if _, err := d.cloud.GetVolumeByID(ctx, volumeID); err != nil {
		return nil, status.Errorf(cloud.GRPCCode(err), "Could not get volume with ID %q: %v", volumeID, err)
	}

	var confirmed *csi.ValidateVolumeCapabilitiesResponse_Confirmed
	if isValidVolumeCapabilities(volCaps) {
		confirmed = &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: volCaps}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: confirmed,
	}, nil
}

func (d *controllerService) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	capRange := req.GetCapacityRange()
	if capRange == nil {
		return nil, status.Error(codes.InvalidArgument, "Capacity range not provided")
	}

	newSize := util.RoundUpBytes(capRange.GetRequiredBytes())
	maxVolSize := capRange.GetLimitBytes()
	if maxVolSize > 0 && maxVolSize < newSize {
		return nil, status.Error(codes.InvalidArgument, "After round-up, volume size exceeds the limit specified")
	}

	actualSize, err := d.cloud.ResizeVolume(ctx, volumeID, newSize)
	if err != nil {
		return nil, status.Errorf(cloud.GRPCCode(err), "Could not resize volume %q: %v", volumeID, err)
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         actualSize,
		NodeExpansionRequired: true,
	}, nil
}

func (d *controllerService) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	var (
		volumeType osc.VolumeType
		iopsPerGB  int64
	)

	for key, value := range req.GetMutableParameters() {
		switch strings.ToLower(key) {
		case VolumeTypeKey:
			volumeType = osc.VolumeType(value)
		case IopsPerGBKey:
			var err error
			iopsPerGB, err = strconv.ParseInt(value, 10, 32)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "Invalid iopsPerGB value: %v", err)
			}
		default:
			return nil, status.Errorf(codes.InvalidArgument, "Invalid parameter key %s for ModifyVolume", key)
		}
	}

	err := d.cloud.UpdateVolume(ctx, volumeID, volumeType, int(iopsPerGB))
	if err != nil {
		return nil, status.Errorf(cloud.GRPCCode(err), "Could not modify volume %q: %v", volumeID, err)
	}

	return &csi.ControllerModifyVolumeResponse{}, nil
}

func isValidVolumeCapabilities(volCaps []*csi.VolumeCapability) bool {
	for _, c := range volCaps {
		if c.GetAccessMode() != nil && !slices.Contains(supportedVolumeModes, c.GetAccessMode().GetMode()) {
			return false
		}
	}
	return true
}

func (d *controllerService) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	snapshotName := req.GetName()
	if snapshotName == "" {
		return nil, status.Error(codes.InvalidArgument, "Snapshot name not provided")
	}

	volumeID := req.GetSourceVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Snapshot volume source ID not provided")
	}
	snap, err := d.cloud.CheckCreatedSnapshot(ctx, snapshotName)
	switch {
	case errors.Is(err, cloud.ErrNotFound):
		// No snapshot found, continue with creation.
	case err != nil:
		return nil, err
	case snap.SourceVolumeID != volumeID:
		return nil, status.Errorf(codes.AlreadyExists, "Snapshot %s already exists for different volume (%s)", snapshotName, snap.SourceVolumeID)
	case snap.IsError():
		return nil, status.Errorf(codes.ResourceExhausted, "Snapshot %q is in error", snap.SnapshotID)
	default:
		klog.FromContext(ctx).V(3).Info("Snapshot already exists")
		return newCreateSnapshotResponse(snap), nil
	}

	opts := &cloud.SnapshotOptions{
		Tags: d.driverOptions.extraSnapshotTags,
	}
	snap, err = d.cloud.CreateSnapshot(ctx, snapshotName, volumeID, opts)
	switch {
	case err != nil:
		return nil, status.Errorf(cloud.GRPCCode(err), "Could not create snapshot %q: %v", snapshotName, err)
	case snap.IsError():
		return nil, status.Errorf(codes.ResourceExhausted, "Snapshot %q is in error", snap.SnapshotID)
	default:
		return newCreateSnapshotResponse(snap), nil
	}
}

func (d *controllerService) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	snapshotID := req.GetSnapshotId()
	if snapshotID == "" {
		return nil, status.Error(codes.InvalidArgument, "Snapshot ID not provided")
	}

	if _, err := d.cloud.DeleteSnapshot(ctx, snapshotID); err != nil {
		if errors.Is(err, cloud.ErrNotFound) {
			klog.FromContext(ctx).V(3).Info("Snapshot already deleted")
			return &csi.DeleteSnapshotResponse{}, nil
		}
		return nil, status.Errorf(cloud.GRPCCode(err), "Could not delete snapshot ID %q: %v", snapshotID, err)
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

func (d *controllerService) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	snapshotID := req.GetSnapshotId()
	if snapshotID != "" {
		snapshot, err := d.cloud.GetSnapshotByID(ctx, snapshotID)
		if err != nil {
			if errors.Is(err, cloud.ErrNotFound) {
				klog.FromContext(ctx).V(4).Info("Snapshot does not exist")
				return &csi.ListSnapshotsResponse{}, nil
			}
			return nil, status.Errorf(cloud.GRPCCode(err), "Could not get snapshot ID %q: %v", snapshotID, err)
		}
		response := newListSnapshotsResponse(cloud.ListSnapshotsResponse{
			Snapshots: []cloud.Snapshot{*snapshot},
		})
		return response, nil
	}

	volumeID := req.GetSourceVolumeId()
	nextToken := req.GetStartingToken()
	maxEntries := req.GetMaxEntries()

	cloudSnapshots, err := d.cloud.ListSnapshots(ctx, volumeID, int(maxEntries), nextToken)
	if err != nil {
		return nil, status.Errorf(cloud.GRPCCode(err), "Could not list snapshots: %v", err)
	}

	return newListSnapshotsResponse(cloudSnapshots), nil
}

// pickAvailabilityZone selects 1 zone given topology requirement.
// if not found, empty string is returned.
func pickAvailabilityZone(requirement *csi.TopologyRequirement) string {
	if requirement == nil {
		return ""
	}
	for _, topology := range requirement.GetPreferred() {
		zone, exists := topology.GetSegments()[TopologyKey]
		if exists {
			return zone
		}
		zone, exists = topology.GetSegments()[TopologyK8sKey]
		if exists {
			return zone
		}
	}
	for _, topology := range requirement.GetRequisite() {
		zone, exists := topology.GetSegments()[TopologyKey]
		if exists {
			return zone
		}
		zone, exists = topology.GetSegments()[TopologyK8sKey]
		if exists {
			return zone
		}
	}
	return ""
}

func newCreateVolumeResponse(vol *cloud.Volume, volumeContextExtra map[string]string) *csi.CreateVolumeResponse {
	var src *csi.VolumeContentSource
	if vol.SnapshotID != nil {
		src = &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: *vol.SnapshotID,
				},
			},
		}
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vol.VolumeID,
			CapacityBytes: util.GiBToBytes(vol.CapacityGiB),
			VolumeContext: volumeContextExtra,
			AccessibleTopology: []*csi.Topology{
				{
					Segments: map[string]string{TopologyKey: vol.AvailabilityZone},
				},
			},
			ContentSource: src,
		},
	}
}

func newCreateSnapshotResponse(snapshot *cloud.Snapshot) *csi.CreateSnapshotResponse {
	ts := timestamppb.New(snapshot.CreationTime)
	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SnapshotId:     snapshot.SnapshotID,
			SourceVolumeId: snapshot.SourceVolumeID,
			SizeBytes:      snapshot.Size,
			CreationTime:   ts,
			ReadyToUse:     snapshot.IsReadyToUse(),
		},
	}
}

func newListSnapshotsResponse(resp cloud.ListSnapshotsResponse) *csi.ListSnapshotsResponse {
	return &csi.ListSnapshotsResponse{
		Entries:   lo.Map(resp.Snapshots, newListSnapshotsResponseEntry),
		NextToken: resp.NextToken,
	}
}

func newListSnapshotsResponseEntry(snapshot cloud.Snapshot, _ int) *csi.ListSnapshotsResponse_Entry {
	ts := timestamppb.New(snapshot.CreationTime)
	return &csi.ListSnapshotsResponse_Entry{
		Snapshot: &csi.Snapshot{
			SnapshotId:     snapshot.SnapshotID,
			SourceVolumeId: snapshot.SourceVolumeID,
			SizeBytes:      snapshot.Size,
			CreationTime:   ts,
			ReadyToUse:     snapshot.IsReadyToUse(),
		},
	}
}

func getVolSizeBytes(req *csi.CreateVolumeRequest) (int64, error) {
	var volSizeBytes int64
	capRange := req.GetCapacityRange()
	if capRange == nil {
		volSizeBytes = cloud.DefaultVolumeSize
	} else {
		volSizeBytes = util.RoundUpBytes(capRange.GetRequiredBytes())
		maxVolSize := capRange.GetLimitBytes()
		if maxVolSize > 0 && maxVolSize < volSizeBytes {
			return 0, status.Error(codes.InvalidArgument, "After round-up, volume size exceeds the limit specified")
		}
	}
	return volSizeBytes, nil
}

var _ csi.ControllerServer = (*controllerService)(nil)
