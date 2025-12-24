/*
Copyright 2019 The Kubernetes Authors.
Copyright 2025 Outscale SAS.

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
	"strings"
	"time"

	"github.com/outscale/goutils/k8s/batch"
	"github.com/outscale/goutils/k8s/sdk"
	"github.com/outscale/goutils/sdk/ptr"
	dm "github.com/outscale/osc-bsu-csi-driver/pkg/cloud/devicemanager"
	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/samber/lo"
	"k8s.io/klog/v2"
)

// Outscale volume types
const (
	// Cold workloads where you do not need to access data frequently
	// Cases in which the lowest storage cost highly matters.
	VolumeTypeSTANDARD = "standard"
	// Most workloads that require moderate performance with moderate costs
	// Applications that require high performance for a short period of time
	// (for example, starting a file system)
	VolumeTypeGP2 = "gp2"
	// Workloads where you must access data frequently (for example, a database)
	// Critical business applications that can be blocked by a low performance when accessing data stored on the volume
	VolumeTypeIO1 = "io1"
)

var (
	// ValidVolumeTypes = []string{VolumeTypeIO1, VolumeTypeGP2,             VolumeTypeSC1, VolumeTypeST1}
	ValidVolumeTypes = []string{VolumeTypeIO1, VolumeTypeGP2, VolumeTypeSTANDARD}
)

// Outscale provisioning limits.
// Source: https://docs.outscale.com/en/userguide/About-Volumes.html#_volume_types_and_iops
const (
	// MinTotalIOPS represents the minimum Input Output per second.
	MinTotalIOPS = 100
	// MaxTotalIOPS represents the maximum Input Output per second.
	MaxTotalIOPS = 13000
	// MaxIopsPerGb represents the maximum Input Output per GigaBits
	MaxIopsPerGb = 300
	// MaxNumTagsPerResource represents the maximum number of tags per Outscale resource.
	MaxNumTagsPerResource = 50
	// MaxTagKeyLength represents the maximum key length for a tag.
	MaxTagKeyLength = 128
	// MaxTagValueLength represents the maximum value length for a tag.
	MaxTagValueLength = 256
)

// Defaults
const (
	// DefaultVolumeSize represents the default volume size.
	DefaultVolumeSize int64 = 100 * util.GiB
	// DefaultVolumeType specifies which storage to use for newly created Volumes.
	DefaultVolumeType = VolumeTypeGP2
)

// Tags
const (
	// VolumeNameTagKey is the key value that refers to the volume's name.
	VolumeNameTagKey = "CSIVolumeName"
	// SnapshotNameTagKey is the key value that refers to the snapshot's name.
	SnapshotNameTagKey = "CSIVolumeSnapshotName"
	// KubernetesTagKeyPrefix is the prefix of the key value that is reserved for Kubernetes.
	KubernetesTagKeyPrefix = "kubernetes.io"
	// OscTagKeyPrefix is the prefix of the key value that is reserved for Outscale.
	OscTagKeyPrefix = "osc:"
)

var (
	// ErrMultiVolumes is returned when multiple
	// Volumes are found with the same volume name.
	ErrMultiVolumes = errors.New("Multiple Volumes with same name")

	// ErrVolumeExistsDiffSize is returned if a Volume with a given
	// name, but different size, is found.
	ErrVolumeExistsDiffSize = errors.New("There is already a Volume with same name and different size")

	// ErrNotFound is returned when a resource is not found.
	ErrNotFound = errors.New("Resource was not found")

	// ErrAlreadyExists is returned when a resource already exists.
	ErrAlreadyExists = errors.New("Resource already exists")

	// ErrMultiSnapshots is returned when multiple snapshots are found
	// with the same ID or name
	ErrMultiSnapshots = errors.New("Multiple snapshots with the same name/id found")

	// ErrMultiVMs is returned when multiple VMs are found with the same name.
	ErrMultiVMs = errors.New("Multiple VMs with the same ID found")
)

// Volume represents a BSU volume
type Volume struct {
	VolumeID         string
	CapacityGiB      int
	AvailabilityZone string
	SnapshotID       *string
	VolumeType       string
	IOPSPerGB        int
}

// VolumeOptions represents parameters to create an BSU volume
type VolumeOptions struct {
	CapacityBytes    int64
	Tags             map[string]string
	VolumeType       string
	IOPSPerGB        int
	AvailabilityZone string
	Encrypted        bool
	// KmsKeyID represents a fully qualified resource name to the key to use for encryption.
	// example: arn:aws:kms:us-east-1:012345678910:key/abcd1234-a123-456a-a12b-a123b4cd56ef
	KmsKeyID   string
	SnapshotID string
}

// Snapshot represents an BSU volume snapshot
type Snapshot struct {
	SnapshotID     string
	SourceVolumeID string
	Size           int64
	CreationTime   time.Time
	State          osc.SnapshotState
}

func (s *Snapshot) IsReadyToUse() bool {
	return s.State == osc.SnapshotStateCompleted
}

func (s *Snapshot) IsError() bool {
	return s.State == osc.SnapshotStateError
}

// ListSnapshotsResponse is the container for our snapshots along with a pagination token to pass back to the caller
type ListSnapshotsResponse struct {
	Snapshots []Snapshot
	NextToken string
}

// SnapshotOptions represents parameters to create an BSU volume
type SnapshotOptions struct {
	Tags map[string]string
}

type Cloud interface {
	Start(ctx context.Context)

	CreateVolume(ctx context.Context, volumeName string, volumeOptions *VolumeOptions) (vol *Volume, err error)
	DeleteVolume(ctx context.Context, volumeID string) (success bool, err error)
	AttachVolume(ctx context.Context, volumeID string, nodeID string) (devicePath string, err error)
	DetachVolume(ctx context.Context, volumeID string, nodeID string) (err error)
	ResizeVolume(ctx context.Context, volumeID string, reqSize int64) (newSize int64, err error)
	UpdateVolume(ctx context.Context, volumeID string, volumeType string, iopsPerGB int) (err error)
	WaitForAttachmentState(ctx context.Context, volumeID string, state osc.LinkedVolumeState) error
	CheckCreatedVolume(ctx context.Context, name string, capacityBytes int64) (vol *Volume, err error)
	GetVolumeByID(ctx context.Context, volumeID string) (vol *Volume, err error)

	ExistsInstance(ctx context.Context, nodeID string) (success bool)

	CreateSnapshot(ctx context.Context, volumeID string, snapshotOptions *SnapshotOptions) (snapshot *Snapshot, err error)
	DeleteSnapshot(ctx context.Context, snapshotID string) (success bool, err error)
	CheckCreatedSnapshot(ctx context.Context, name string) (snapshot *Snapshot, err error)
	GetSnapshotByID(ctx context.Context, snapshotID string) (snapshot *Snapshot, err error)
	ListSnapshots(ctx context.Context, volumeID string, maxResults int, nextToken string) (listSnapshotsResponse ListSnapshotsResponse, err error)
}

type cloud struct {
	region string
	dm     dm.DeviceManager
	client osc.ClientInterface

	snapshotWatcher *batch.BatcherByID[osc.Snapshot]
	volumeWatcher   *batch.BatcherByID[osc.Volume]
}

var _ Cloud = &cloud{}

// CloudOption defines an option for NewCloud.
type CloudOption func(*cloud) error

// NewCloud returns a new instance of Outscale cloud using the default backoff policy.
// It panics if session is invalid
func NewCloud(ctx context.Context, opts ...CloudOption) (Cloud, error) {
	// Set User-Agent with name and version of the CSI driver
	version := util.GetVersion()
	v := version.DriverVersion
	if !strings.HasPrefix(v, "v") {
		v = "dev"
	}
	ua := "osc-bsu-csi-driver/" + v
	profile, client, err := sdk.NewSDKClient(ctx, ua)

	klog.V(1).InfoS("Region: " + profile.Region)

	interval, err := time.ParseDuration(util.Getenv("READ_STATUS_INTERVAL", "2s"))
	if err != nil {
		interval = 2 * time.Second
	}
	c := &cloud{
		region:          profile.Region,
		dm:              dm.NewDeviceManager(),
		client:          client,
		snapshotWatcher: batch.NewSnapshotBatcherByID(interval, client),
		volumeWatcher:   batch.NewVolumeBatcherByID(interval, client),
	}
	for _, opt := range opts {
		err := opt(c)
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *cloud) Start(ctx context.Context) {
	go c.snapshotWatcher.Run(ctx)
	go c.volumeWatcher.Run(ctx)
}

func iops(iopsPerGB, capacityGiB int) int {
	if iopsPerGB > MaxIopsPerGb {
		iopsPerGB = 300
	}

	iops := capacityGiB * iopsPerGB
	if iops < MinTotalIOPS {
		iops = MinTotalIOPS
	}
	if iops > MaxTotalIOPS {
		iops = MaxTotalIOPS
	}
	return iops
}

func (c *cloud) CreateVolume(ctx context.Context, volumeName string, VolumeOptions *VolumeOptions) (*Volume, error) {
	var (
		createType string
		request    osc.CreateVolumeRequest
	)

	if volumeName != "" {
		request.ClientToken = &volumeName
	}

	capacityGiB := util.BytesToGiB(VolumeOptions.CapacityBytes)
	request.Size = &capacityGiB

	switch VolumeOptions.VolumeType {
	case VolumeTypeGP2, VolumeTypeSTANDARD:
		createType = VolumeOptions.VolumeType
	case VolumeTypeIO1:
		createType = VolumeOptions.VolumeType
		iops := iops(VolumeOptions.IOPSPerGB, capacityGiB)
		request.Iops = &iops
	case "":
		createType = DefaultVolumeType
	default:
		return nil, fmt.Errorf("invalid Outscale VolumeType %q", VolumeOptions.VolumeType)
	}

	request.VolumeType = &createType

	resourceTag := make([]osc.ResourceTag, 0, len(VolumeOptions.Tags))
	for key, value := range VolumeOptions.Tags {
		resourceTag = append(resourceTag, osc.ResourceTag{Key: key, Value: value})
	}

	zone := VolumeOptions.AvailabilityZone
	if zone == "" {
		// Create the volume in AZ A by default (See https://docs.outscale.com/en/userguide/Creating-a-Volume.html)
		zone = fmt.Sprintf("%va", c.region)
	}
	request.SubregionName = zone

	// NOT SUPPORTED YET BY Outscale API
	if len(VolumeOptions.KmsKeyID) > 0 {
		return nil, errors.New("Encryption is not supported yet by Outscale API")
	}

	snapshotID := VolumeOptions.SnapshotID
	if len(snapshotID) != 0 {
		request.SnapshotId = &snapshotID
	}

	resp, err := c.client.CreateVolume(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("could not create volume: %w", err)
	}

	vol := resp.Volume

	_, err = c.client.CreateTags(ctx, osc.CreateTagsRequest{
		ResourceIds: []string{vol.VolumeId},
		Tags:        resourceTag,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to add tags: %w", err)
	}

	if err := c.waitForNewVolume(ctx, resp.Volume); err != nil {
		return nil, fmt.Errorf("unable to fetch newly created volume: %w", err)
	}

	return &Volume{
		CapacityGiB: vol.Size, VolumeID: vol.VolumeId, AvailabilityZone: zone, SnapshotID: vol.SnapshotId,
		VolumeType: vol.VolumeType, IOPSPerGB: vol.Iops / vol.Size,
	}, nil
}

// waitForNewVolume waits for volume to be in the "available" state.
// On a random Outscale account (shared among several developers) it took 4s on average.
func (c *cloud) waitForNewVolume(ctx context.Context, vol *osc.Volume) error {
	klog.FromContext(ctx).V(4).Info("Waiting until volume is available")
	testVolume := func(vol *osc.Volume) (bool, error) {
		if vol == nil {
			return false, errors.New("volume not found")
		}
		switch vol.State {
		case osc.VolumeStateError, osc.VolumeStateDeleting:
			return false, fmt.Errorf("volume is in %q state", vol.State)
		case osc.VolumeStateCreating:
			return false, nil
		default: // available, in-use
			return true, nil
		}
	}
	if ok, err := testVolume(vol); ok || err != nil {
		return err
	}
	_, err := c.volumeWatcher.WaitUntil(ctx, vol.VolumeId, testVolume)
	if err != nil {
		return fmt.Errorf("unable to wait for volume: %w", err)
	}
	return nil
}

func (c *cloud) DeleteVolume(ctx context.Context, volumeID string) (bool, error) {
	_, err := c.client.DeleteVolume(ctx, osc.DeleteVolumeRequest{
		VolumeId: volumeID,
	})
	switch {
	case isVolumeNotFoundError(err):
		return false, ErrNotFound
	case err != nil:
		return false, fmt.Errorf("unable to delete Volume: %w", err)
	default:
		return true, nil
	}
}

func (c *cloud) AttachVolume(ctx context.Context, volumeID, nodeID string) (string, error) {
	instance, err := c.getInstance(ctx, nodeID)
	if err != nil {
		return "", err
	}

	device, err := c.dm.NewDevice(instance, volumeID)
	if err != nil {
		return "", err
	}
	defer device.Release(false)

	if !device.IsAlreadyAssigned {
		request := osc.LinkVolumeRequest{
			DeviceName: device.Path,
			VmId:       nodeID,
			VolumeId:   volumeID,
		}
		_, err := c.client.LinkVolume(ctx, request)
		if err != nil {
			return "", fmt.Errorf("could not attach volume: %w", err)
		}
	}

	// This is the only situation where we taint the device
	if err := c.waitForAttachedVolume(ctx, volumeID); err != nil {
		device.Taint()
		return "", err
	}

	// TODO: Double check the attachment to be 100% sure we attached the correct volume at the correct mountpoint
	// It could happen otherwise that we see the volume attached from a previous/separate AttachVolume call,
	// which could theoretically be against a different device (or even instance).

	return device.Path, nil
}

// waitForAttachedVolume waits for volume to be attached state.
func (c *cloud) waitForAttachedVolume(ctx context.Context, volumeID string) error {
	klog.FromContext(ctx).V(4).Info("Waiting until volume is attached")
	testVolume := func(vol *osc.Volume) (bool, error) {
		if vol == nil || vol.State == osc.VolumeStateDeleting {
			return false, errors.New("volume not found")
		}
		for _, a := range *vol.LinkedVolumes {
			if *a.State == osc.LinkedVolumeStateAttached {
				return true, nil
			}
		}
		return false, nil
	}
	_, err := c.volumeWatcher.WaitUntil(ctx, volumeID, testVolume)
	if err != nil {
		return fmt.Errorf("unable to wait for volume: %w", err)
	}
	return nil
}

func (c *cloud) DetachVolume(ctx context.Context, volumeID, nodeID string) error {
	logger := klog.FromContext(ctx)
	{
		// Check if the volume is attached to VM
		request := osc.ReadVolumesRequest{
			Filters: &osc.FiltersVolume{
				VolumeIds: &[]string{volumeID},
			},
		}

		volume, err := c.getVolume(ctx, request)
		if err == nil && volume.State == osc.VolumeStateAvailable {
			logger.V(4).Info("Volume is already available")
			return nil
		}
	}
	instance, err := c.getInstance(ctx, nodeID)
	if err != nil {
		return err
	}

	// TODO: check if attached
	device := c.dm.GetDevice(instance, volumeID)
	defer device.Release(true)

	if !device.IsAlreadyAssigned {
		logger.V(4).Info("Volume is not assigned to node")
		return ErrNotFound
	}

	request := osc.UnlinkVolumeRequest{
		VolumeId: volumeID,
	}

	_, err = c.client.UnlinkVolume(ctx, request)
	if err != nil {
		return fmt.Errorf("could not detach volume: %w", err)
	}

	if err := c.waitForDetachedVolume(ctx, volumeID); err != nil {
		return err
	}

	return nil
}

// waitForDetachedVolume waits for volume to be attached state.
func (c *cloud) waitForDetachedVolume(ctx context.Context, volumeID string) error {
	klog.FromContext(ctx).V(4).Info("Waiting until volume is detached")
	testVolume := func(vol *osc.Volume) (bool, error) {
		return vol == nil || vol.State == osc.VolumeStateDeleting || len(*vol.LinkedVolumes) == 0, nil
	}
	_, err := c.volumeWatcher.WaitUntil(ctx, volumeID, testVolume)
	if err != nil {
		return fmt.Errorf("unable to wait for volume: %w", err)
	}
	return nil
}

// WaitForAttachmentState polls until the attachment status is the expected value.
func (c *cloud) WaitForAttachmentState(ctx context.Context, volumeID string, state osc.LinkedVolumeState) error {
	switch state {
	case osc.LinkedVolumeStateAttached:
		return c.waitForAttachedVolume(ctx, volumeID)
	case "detached":
		return c.waitForDetachedVolume(ctx, volumeID)
	}
	return nil
}

// FIXME: as the code also checks size, either the name should not be Get or the size should be checked by caller.
func (c *cloud) CheckCreatedVolume(ctx context.Context, name string, capacityBytes int64) (*Volume, error) {
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			TagKeys:   &[]string{VolumeNameTagKey},
			TagValues: &[]string{name},
		},
	}

	volume, err := c.getVolume(ctx, request)
	if err != nil {
		return nil, err
	}

	if volume.Size != util.BytesToGiB(capacityBytes) {
		return nil, ErrVolumeExistsDiffSize
	}

	if err := c.waitForNewVolume(ctx, volume); err != nil {
		return nil, fmt.Errorf("unable to fetch created volume: %w", err)
	}

	return &Volume{
		VolumeID:         volume.VolumeId,
		CapacityGiB:      volume.Size,
		AvailabilityZone: volume.SubregionName,
		SnapshotID:       volume.SnapshotId,
		VolumeType:       volume.VolumeType,
		IOPSPerGB:        volume.Iops / volume.Size,
	}, nil
}

func (c *cloud) GetVolumeByID(ctx context.Context, volumeID string) (*Volume, error) {
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}

	volume, err := c.getVolume(ctx, request)
	if err != nil {
		return nil, err
	}

	return &Volume{
		VolumeID:         volume.VolumeId,
		CapacityGiB:      volume.Size,
		AvailabilityZone: volume.SubregionName,
		SnapshotID:       volume.SnapshotId,
		VolumeType:       volume.VolumeType,
		IOPSPerGB:        volume.Iops / volume.Size,
	}, nil
}

// FiXME: errors are not properly handled.
func (c *cloud) ExistsInstance(ctx context.Context, nodeID string) bool {
	_, err := c.getInstance(ctx, nodeID)
	return err == nil
}

func (c *cloud) CreateSnapshot(ctx context.Context, volumeID string, snapshotOptions *SnapshotOptions) (snapshot *Snapshot, err error) {
	description := "Created by Outscale BSU CSI driver for volume " + volumeID

	request := osc.CreateSnapshotRequest{
		VolumeId:    &volumeID,
		Description: &description,
	}

	name := snapshotOptions.Tags[SnapshotNameTagKey]
	if name != "" {
		request.ClientToken = &name
	}
	resp, err := c.client.CreateSnapshot(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("unable to create snapshot: %w", err)
	}

	resourceTag := make([]osc.ResourceTag, 0, len(snapshotOptions.Tags))
	for key, value := range snapshotOptions.Tags {
		resourceTag = append(resourceTag, osc.ResourceTag{Key: key, Value: value})
	}
	requestTag := osc.CreateTagsRequest{
		ResourceIds: []string{resp.Snapshot.SnapshotId},
		Tags:        resourceTag,
	}

	_, err = c.client.CreateTags(ctx, requestTag)
	if err != nil {
		return nil, fmt.Errorf("unable to add tags: %w", err)
	}
	snap, err := c.waitForSnapshot(ctx, resp.Snapshot)
	if err != nil {
		return nil, fmt.Errorf("wait error: %w", err)
	}

	return oscSnapshotResponseToStruct(snap), nil
}

// waitForSnapshot waits for a snapshot to be in the "completed" state.
func (c *cloud) waitForSnapshot(ctx context.Context, snap *osc.Snapshot) (*osc.Snapshot, error) {
	klog.FromContext(ctx).V(4).Info("Waiting until snapshot is ready")
	testSnapshot := func(snap *osc.Snapshot) (ok bool, err error) {
		if snap == nil {
			return false, errors.New("snapshot not found")
		}
		switch snap.State {
		case osc.SnapshotStateError, osc.SnapshotStateDeleting:
			return false, fmt.Errorf("snapshot is in %q state", snap.State)
		case osc.SnapshotStateCompleted:
			return true, nil
		default: // "in-queue", "pending"
			return false, nil
		}
	}
	if ok, err := testSnapshot(snap); ok || err != nil {
		return snap, err
	}
	snap, err := c.snapshotWatcher.WaitUntil(ctx, snap.SnapshotId, testSnapshot)
	if err != nil {
		return snap, fmt.Errorf("unable to wait for snapshot: %w", err)
	}
	return snap, nil
}

func (c *cloud) DeleteSnapshot(ctx context.Context, snapshotID string) (success bool, err error) {
	_, err = c.client.DeleteSnapshot(ctx, osc.DeleteSnapshotRequest{
		SnapshotId: snapshotID,
	})
	switch {
	case isSnapshotNotFoundError(err):
		return false, ErrNotFound
	case err != nil:
		return false, fmt.Errorf("cound not delete snapshot: %w", err)
	default:
		return true, nil
	}
}

func (c *cloud) CheckCreatedSnapshot(ctx context.Context, name string) (snapshot *Snapshot, err error) {
	request := osc.ReadSnapshotsRequest{
		Filters: &osc.FiltersSnapshot{
			TagKeys:   &[]string{SnapshotNameTagKey},
			TagValues: &[]string{name},
		},
	}

	snap, err := c.getSnapshot(ctx, request, true)
	if err != nil {
		return nil, err
	}

	snap, err = c.waitForSnapshot(ctx, snap)
	if err != nil {
		return nil, fmt.Errorf("wait error: %w", err)
	}

	return oscSnapshotResponseToStruct(snap), nil
}

func (c *cloud) GetSnapshotByID(ctx context.Context, snapshotID string) (snapshot *Snapshot, err error) {
	request := osc.ReadSnapshotsRequest{
		Filters: &osc.FiltersSnapshot{
			SnapshotIds: &[]string{snapshotID},
		},
	}

	snap, err := c.getSnapshot(ctx, request, true)
	if err != nil {
		return nil, err
	}

	return oscSnapshotResponseToStruct(snap), nil
}

const maxResultsLimit = 1000

// ListSnapshots retrieves Outscale BSU snapshots for an optionally specified volume ID.  If maxResults is set, it will return up to maxResults snapshots.  If there are more snapshots than maxResults,
// a next token value will be returned to the client as well.  They can use this token with subsequent calls to retrieve the next page of results.
// Pagination not supported
func (c *cloud) ListSnapshots(ctx context.Context, volumeID string, maxResults int, nextToken string) (listSnapshotsResponse ListSnapshotsResponse, err error) {
	if maxResults > maxResultsLimit {
		maxResults = maxResultsLimit
	}

	req := osc.ReadSnapshotsRequest{}
	if maxResults > 0 {
		req.ResultsPerPage = &maxResults
	}
	if nextToken != "" {
		req.NextPageToken = ptr.To([]byte(nextToken))
	}
	if len(volumeID) != 0 {
		req.Filters = &osc.FiltersSnapshot{
			VolumeIds: &[]string{volumeID},
		}
	}

	resp, err := c.client.ReadSnapshots(ctx, req)
	if err != nil {
		return ListSnapshotsResponse{}, fmt.Errorf("error listing snapshots: %w", err)
	}
	snapshots := lo.Map(*resp.Snapshots, func(s osc.Snapshot, _ int) Snapshot { return *oscSnapshotResponseToStruct(&s) })
	klog.FromContext(ctx).V(5).Info(fmt.Sprintf("%d snapshots found", len(snapshots)))
	return ListSnapshotsResponse{
		Snapshots: snapshots,
		NextToken: nextToken,
	}, nil
}

func oscSnapshotResponseToStruct(s *osc.Snapshot) *Snapshot {
	return &Snapshot{
		SnapshotID:     s.SnapshotId,
		SourceVolumeID: s.VolumeId,
		Size:           util.GiBToBytes(s.VolumeSize),
		State:          s.State,
		CreationTime:   s.CreationDate.Time,
	}
}

// Pagination not supported
func (c *cloud) getVolume(ctx context.Context, request osc.ReadVolumesRequest) (*osc.Volume, error) {
	resp, err := c.client.ReadVolumes(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("unable to read volume: %w", err)
	}

	volumes := *resp.Volumes
	switch len(volumes) {
	case 0:
		return nil, ErrNotFound
	case 1:
		return &volumes[0], nil
	default:
		return nil, ErrMultiVolumes
	}
}

// Pagination not supported
func (c *cloud) getInstance(ctx context.Context, vmID string) (*osc.Vm, error) {
	resp, err := c.client.ReadVms(ctx, osc.ReadVmsRequest{
		Filters: &osc.FiltersVm{
			VmIds: &[]string{vmID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error listing instances: %w", err)
	}

	vms := *resp.Vms
	switch len(vms) {
	case 0:
		return nil, ErrNotFound
	case 1:
		return &vms[0], nil
	default:
		return nil, ErrMultiVMs
	}
}

// Pagination not supported
func (c *cloud) getSnapshot(ctx context.Context, request osc.ReadSnapshotsRequest, backoff bool) (*osc.Snapshot, error) {
	resp, err := c.client.ReadSnapshots(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("error listing snapshots: %w", err)
	}
	snapshots := *resp.Snapshots
	switch len(snapshots) {
	case 0:
		return nil, ErrNotFound
	case 1:
		return &snapshots[0], nil
	default:
		return nil, ErrMultiSnapshots
	}
}

// ResizeVolume resizes an BSU volume in GiB increments, rouding up to the next possible allocatable unit.
// It returns the volume size in bytes after this call or an error if the size couldn't be determined.
func (c *cloud) ResizeVolume(ctx context.Context, volumeID string, newSizeBytes int64) (int64, error) {
	logger := klog.FromContext(ctx)
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}
	volume, err := c.getVolume(ctx, request)
	if err != nil {
		return 0, err
	}

	// resizes in chunks of GiB (not GB)
	newSizeGiB := util.RoundUpGiB(newSizeBytes)
	oldSizeGiB := volume.Size

	// Even if existing volume size is greater than user requested size, we should ensure that there are no pending
	// volume modifications objects or volume has completed previously issued modification request.
	if oldSizeGiB >= newSizeGiB {
		logger.V(4).Info(fmt.Sprintf("Volume current size (%d GiB) is greater or equal to the new size (%d GiB)", oldSizeGiB, newSizeGiB))
		return util.GiBToBytes(oldSizeGiB), nil
	}

	logger.V(4).Info(fmt.Sprintf("Expanding volume to %dGiB", newSizeGiB))
	_, err = c.client.UpdateVolume(ctx, osc.UpdateVolumeRequest{
		Size:     &newSizeGiB,
		VolumeId: volumeID,
	})
	if err != nil {
		return 0, fmt.Errorf("could not modify volume: %w", err)
	}
	return c.waitForResize(ctx, volumeID, newSizeGiB)
}

func (c *cloud) waitForResize(ctx context.Context, volumeID string, newSizeGiB int) (newSize int64, err error) {
	klog.FromContext(ctx).V(4).Info("Waiting until volume is resized")
	testVolume := func(vol *osc.Volume) (ok bool, err error) {
		if vol.Size >= newSizeGiB {
			return true, nil
		}
		return false, nil
	}
	vol, err := c.volumeWatcher.WaitUntil(ctx, volumeID, testVolume)
	if err != nil {
		return 0, err
	}
	return util.GiBToBytes(vol.Size), nil
}

// UpdateVolume updates the type and iops of a volume.
func (c *cloud) UpdateVolume(ctx context.Context, volumeID string, volumeType string, iopsPerGB int) (err error) {
	logger := klog.FromContext(ctx)
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}
	volume, err := c.getVolume(ctx, request)
	if err != nil {
		return err
	}

	iops := iops(iopsPerGB, volume.Size)
	logger.V(4).Info(fmt.Sprintf("Updating type to %s and iops to %d", volumeType, iops))
	_, err = c.client.UpdateVolume(ctx, osc.UpdateVolumeRequest{
		VolumeId:   volumeID,
		VolumeType: &volumeType,
		Iops:       &iops,
	})
	if err != nil {
		return fmt.Errorf("could not modify volume: %w", err)
	}
	return nil
}
