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
	"net/http"
	"os"
	"time"

	dm "github.com/outscale/osc-bsu-csi-driver/pkg/cloud/devicemanager"
	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	osc "github.com/outscale/osc-sdk-go/v2"
	"k8s.io/apimachinery/pkg/util/wait"
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
	// ErrMultiDisks is returned when multiple
	// disks are found with the same volume name.
	ErrMultiDisks = errors.New("Multiple disks with same name")

	// ErrDiskExistsDiffSize is returned if a disk with a given
	// name, but different size, is found.
	ErrDiskExistsDiffSize = errors.New("There is already a disk with same name and different size")

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

// Disk represents a BSU volume
type Disk struct {
	VolumeID         string
	CapacityGiB      int32
	AvailabilityZone string
	SnapshotID       string
}

// DiskOptions represents parameters to create an BSU volume
type DiskOptions struct {
	CapacityBytes    int64
	Tags             map[string]string
	VolumeType       string
	IOPSPerGB        int32
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
	State          string
}

func (s *Snapshot) IsReadyToUse() bool {
	return s.State == "completed"
}

func (s *Snapshot) IsError() bool {
	return s.State == "error"
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

// oscListSnapshotsResponse is a helper struct returned from the Outscale API calling function to the main ListSnapshots function
type oscListSnapshotsResponse struct {
	Snapshots []osc.Snapshot
}

type Cloud interface {
	CreateDisk(ctx context.Context, volumeName string, diskOptions *DiskOptions) (disk Disk, err error)
	DeleteDisk(ctx context.Context, volumeID string) (success bool, err error)
	AttachDisk(ctx context.Context, volumeID string, nodeID string) (devicePath string, err error)
	DetachDisk(ctx context.Context, volumeID string, nodeID string) (err error)
	ResizeDisk(ctx context.Context, volumeID string, reqSize int64) (newSize int64, err error)
	WaitForAttachmentState(ctx context.Context, volumeID, state string) error
	GetDiskByName(ctx context.Context, name string, capacityBytes int64) (disk Disk, err error)
	GetDiskByID(ctx context.Context, volumeID string) (disk Disk, err error)
	IsExistInstance(ctx context.Context, nodeID string) (success bool)
	CreateSnapshot(ctx context.Context, volumeID string, snapshotOptions *SnapshotOptions) (snapshot Snapshot, err error)
	DeleteSnapshot(ctx context.Context, snapshotID string) (success bool, err error)
	GetSnapshotByName(ctx context.Context, name string) (snapshot Snapshot, err error)
	GetSnapshotByID(ctx context.Context, snapshotID string) (snapshot Snapshot, err error)
	ListSnapshots(ctx context.Context, volumeID string, maxResults int32, nextToken string) (listSnapshotsResponse ListSnapshotsResponse, err error)
}

type OscInterface interface {
	CreateVolume(ctx context.Context, localVarOptionals osc.CreateVolumeRequest) (osc.CreateVolumeResponse, *http.Response, error)
	CreateTags(ctx context.Context, localVarOptionals osc.CreateTagsRequest) (osc.CreateTagsResponse, *http.Response, error)
	ReadVolumes(ctx context.Context, localVarOptionals osc.ReadVolumesRequest) (osc.ReadVolumesResponse, *http.Response, error)
	DeleteVolume(ctx context.Context, localVarOptionals osc.DeleteVolumeRequest) (osc.DeleteVolumeResponse, *http.Response, error)
	LinkVolume(ctx context.Context, localVarOptionals osc.LinkVolumeRequest) (osc.LinkVolumeResponse, *http.Response, error)
	UnlinkVolume(ctx context.Context, localVarOptionals osc.UnlinkVolumeRequest) (osc.UnlinkVolumeResponse, *http.Response, error)
	CreateSnapshot(ctx context.Context, localVarOptionals osc.CreateSnapshotRequest) (osc.CreateSnapshotResponse, *http.Response, error)
	ReadSnapshots(ctx context.Context, localVarOptionals osc.ReadSnapshotsRequest) (osc.ReadSnapshotsResponse, *http.Response, error)
	DeleteSnapshot(ctx context.Context, localVarOptionals osc.DeleteSnapshotRequest) (osc.DeleteSnapshotResponse, *http.Response, error)
	ReadSubregions(ctx context.Context, localVarOptionals osc.ReadSubregionsRequest) (osc.ReadSubregionsResponse, *http.Response, error)
	ReadVms(ctx context.Context, localVarOptionals osc.ReadVmsRequest) (osc.ReadVmsResponse, *http.Response, error)
	UpdateVolume(ctx context.Context, localVarOptionals osc.UpdateVolumeRequest) (osc.UpdateVolumeResponse, *http.Response, error)
}

type OscClient struct {
	config *osc.Configuration
	auth   context.Context
	api    *osc.APIClient
}

func (client *OscClient) CreateVolume(ctx context.Context, localVarOptionals osc.CreateVolumeRequest) (osc.CreateVolumeResponse, *http.Response, error) {
	return client.api.VolumeApi.CreateVolume(client.auth).CreateVolumeRequest(localVarOptionals).Execute()
}

func (client *OscClient) CreateTags(ctx context.Context, localVarOptionals osc.CreateTagsRequest) (osc.CreateTagsResponse, *http.Response, error) {
	return client.api.TagApi.CreateTags(client.auth).CreateTagsRequest(localVarOptionals).Execute()
}

func (client *OscClient) ReadVolumes(ctx context.Context, localVarOptionals osc.ReadVolumesRequest) (osc.ReadVolumesResponse, *http.Response, error) {
	return client.api.VolumeApi.ReadVolumes(client.auth).ReadVolumesRequest(localVarOptionals).Execute()
}

func (client *OscClient) DeleteVolume(ctx context.Context, localVarOptionals osc.DeleteVolumeRequest) (osc.DeleteVolumeResponse, *http.Response, error) {
	return client.api.VolumeApi.DeleteVolume(client.auth).DeleteVolumeRequest(localVarOptionals).Execute()
}

func (client *OscClient) LinkVolume(ctx context.Context, localVarOptionals osc.LinkVolumeRequest) (osc.LinkVolumeResponse, *http.Response, error) {
	return client.api.VolumeApi.LinkVolume(client.auth).LinkVolumeRequest(localVarOptionals).Execute()
}

func (client *OscClient) UnlinkVolume(ctx context.Context, localVarOptionals osc.UnlinkVolumeRequest) (osc.UnlinkVolumeResponse, *http.Response, error) {
	return client.api.VolumeApi.UnlinkVolume(client.auth).UnlinkVolumeRequest(localVarOptionals).Execute()
}

func (client *OscClient) CreateSnapshot(ctx context.Context, localVarOptionals osc.CreateSnapshotRequest) (osc.CreateSnapshotResponse, *http.Response, error) {
	return client.api.SnapshotApi.CreateSnapshot(client.auth).CreateSnapshotRequest(localVarOptionals).Execute()
}

func (client *OscClient) ReadSnapshots(ctx context.Context, localVarOptionals osc.ReadSnapshotsRequest) (osc.ReadSnapshotsResponse, *http.Response, error) {
	return client.api.SnapshotApi.ReadSnapshots(client.auth).ReadSnapshotsRequest(localVarOptionals).Execute()
}

func (client *OscClient) DeleteSnapshot(ctx context.Context, localVarOptionals osc.DeleteSnapshotRequest) (osc.DeleteSnapshotResponse, *http.Response, error) {
	return client.api.SnapshotApi.DeleteSnapshot(client.auth).DeleteSnapshotRequest(localVarOptionals).Execute()
}

func (client *OscClient) ReadSubregions(ctx context.Context, localVarOptionals osc.ReadSubregionsRequest) (osc.ReadSubregionsResponse, *http.Response, error) {
	return client.api.SubregionApi.ReadSubregions(client.auth).ReadSubregionsRequest(localVarOptionals).Execute()
}

func (client *OscClient) ReadVms(ctx context.Context, localVarOptionals osc.ReadVmsRequest) (osc.ReadVmsResponse, *http.Response, error) {
	return client.api.VmApi.ReadVms(client.auth).ReadVmsRequest(localVarOptionals).Execute()
}

func (client *OscClient) UpdateVolume(ctx context.Context, localVarOptionals osc.UpdateVolumeRequest) (osc.UpdateVolumeResponse, *http.Response, error) {
	return client.api.VolumeApi.UpdateVolume(client.auth).UpdateVolumeRequest(localVarOptionals).Execute()
}

var _ OscInterface = &OscClient{}

type cloud struct {
	region  string
	dm      dm.DeviceManager
	client  OscInterface
	backoff BackoffPolicyer
}

var _ Cloud = &cloud{}

// CloudOption defines an option for NewCloud.
type CloudOption func(*cloud) error

// WithoutMetadata is a CloudOption that ensures that region is setup to avoid metadata calls.
func WithoutMetadata() CloudOption {
	return func(c *cloud) error {
		if c.region == "" {
			return errors.New("could not get region")
		}
		return nil
	}
}

// WithBackoffPolicy is a CloudOption that replaces the default backoff policy (DefaultBackoffPolicy).
func WithBackoffPolicy(p BackoffPolicyer) CloudOption {
	return func(c *cloud) error {
		c.backoff = p
		return nil
	}
}

// NewCloud returns a new instance of Outscale cloud using the default backoff policy.
// It panics if session is invalid
func NewCloud(region string, opts ...CloudOption) (Cloud, error) {
	client := &OscClient{}
	// Set User-Agent with name and version of the CSI driver
	version := util.GetVersion()
	configEnv := osc.NewConfigEnv()
	config, err := configEnv.Configuration()
	if err != nil {
		return nil, err
	}
	klog.V(1).InfoS("Region: " + region)
	if configEnv.OutscaleApiEndpoint != nil {
		klog.V(1).InfoS("API endpoint: " + *configEnv.OutscaleApiEndpoint)
	}
	if configEnv.ProfileName != nil {
		klog.V(1).InfoS("Profile: " + *configEnv.ProfileName)
	}
	client.config = config
	client.config.Debug = false
	client.config.UserAgent = "osc-bsu-csi-driver/" + version.DriverVersion
	client.api = osc.NewAPIClient(client.config)

	client.auth = context.WithValue(context.Background(), osc.ContextAWSv4, osc.AWSv4{
		AccessKey: os.Getenv("OSC_ACCESS_KEY"),
		SecretKey: os.Getenv("OSC_SECRET_KEY"),
	})
	client.auth = context.WithValue(client.auth, osc.ContextServerIndex, 0)
	client.auth = context.WithValue(client.auth, osc.ContextServerVariables, map[string]string{"region": region})

	c := &cloud{
		region: region,
		dm:     dm.NewDeviceManager(),
		client: client,
	}
	for _, opt := range opts {
		err := opt(c)
		if err != nil {
			return nil, err
		}
	}
	if c.backoff == nil {
		c.backoff = NewBackoffPolicy()
	}
	return c, nil
}

func IsNilDisk(disk Disk) bool {
	return disk.VolumeID == ""
}

func IsNilSnapshot(snapshot Snapshot) bool {
	return snapshot.SnapshotID == ""
}

func (c *cloud) CreateDisk(ctx context.Context, volumeName string, diskOptions *DiskOptions) (Disk, error) {
	var (
		createType string
		iops       int32
		request    osc.CreateVolumeRequest
	)
	capacityGiB := util.BytesToGiB(diskOptions.CapacityBytes)
	request.SetSize(capacityGiB)

	switch diskOptions.VolumeType {
	case VolumeTypeGP2, VolumeTypeSTANDARD:
		createType = diskOptions.VolumeType
	case VolumeTypeIO1:
		createType = diskOptions.VolumeType

		iopsPerGb := diskOptions.IOPSPerGB
		if iopsPerGb > MaxIopsPerGb {
			iopsPerGb = 300
		}

		iops = capacityGiB * iopsPerGb
		if iops < MinTotalIOPS {
			iops = MinTotalIOPS
		}
		if iops > MaxTotalIOPS {
			iops = MaxTotalIOPS
		}

		request.SetIops(iops)
	case "":
		createType = DefaultVolumeType
	default:
		return Disk{}, fmt.Errorf("invalid Outscale VolumeType %q", diskOptions.VolumeType)
	}

	request.SetVolumeType(createType)

	resourceTag := make([]osc.ResourceTag, 0, len(diskOptions.Tags))
	for key, value := range diskOptions.Tags {
		copiedKey := key
		copiedValue := value
		resourceTag = append(resourceTag, osc.ResourceTag{Key: copiedKey, Value: copiedValue})
	}

	zone := diskOptions.AvailabilityZone
	if zone == "" {
		// Create the volume in AZ A by default (See https://docs.outscale.com/en/userguide/Creating-a-Volume.html)
		zone = fmt.Sprintf("%va", c.region)
	}
	request.SetSubregionName(zone)

	// NOT SUPPORTED YET BY Outscale API
	if len(diskOptions.KmsKeyID) > 0 {
		return Disk{}, errors.New("Encryption is not supported yet by Outscale API")
	}

	snapshotID := diskOptions.SnapshotID
	if len(snapshotID) != 0 {
		request.SetSnapshotId(snapshotID)
	}

	var creation osc.CreateVolumeResponse
	createVolumeCallBack := func(ctx context.Context) (bool, error) {
		var httpRes *http.Response
		var err error
		creation, httpRes, err = c.client.CreateVolume(ctx, request)
		logAPICall(ctx, "CreateVolume", request, creation, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}

	waitErr := c.backoff.ExponentialBackoff(ctx, createVolumeCallBack)
	if waitErr != nil {
		return Disk{}, fmt.Errorf("could not create volume: %w", waitErr)
	}

	if !creation.HasVolume() {
		return Disk{}, errors.New("volume is empty when returned by CreateVolume")
	}

	volumeID := creation.Volume.GetVolumeId()
	if len(volumeID) == 0 {
		return Disk{}, errors.New("volume ID was not returned by CreateVolume")
	}

	size := creation.Volume.GetSize()
	if size == 0 {
		return Disk{}, errors.New("disk size was not returned by CreateVolume")
	}

	requestTag := osc.CreateTagsRequest{
		ResourceIds: []string{volumeID},
		Tags:        resourceTag,
	}

	createTagsCallBack := func(ctx context.Context) (bool, error) {
		resTag, httpRes, err := c.client.CreateTags(ctx, requestTag)
		logAPICall(ctx, "CreateTags", requestTag, resTag, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}

	// If this fails, a duplicate volume is created, we need to try harder/longer, especially on TCP errors.
	waitErr = c.backoff.With(RetryOnErrors(), Steps(20)).ExponentialBackoff(ctx, createTagsCallBack)
	if waitErr != nil {
		return Disk{}, fmt.Errorf("unable to add tags: %w", waitErr)
	}

	if err := c.waitForVolume(ctx, volumeID); err != nil {
		return Disk{}, fmt.Errorf("unable to fetch newly created volume: %w", err)
	}

	return Disk{CapacityGiB: size, VolumeID: volumeID, AvailabilityZone: zone, SnapshotID: snapshotID}, nil
}

func (c *cloud) DeleteDisk(ctx context.Context, volumeID string) (bool, error) {
	request := osc.DeleteVolumeRequest{
		VolumeId: volumeID,
	}

	deleteVolumeCallBack := func(ctx context.Context) (bool, error) {
		response, httpRes, err := c.client.DeleteVolume(ctx, request)
		logAPICall(ctx, "DeleteVolume", request, response, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}

	err := c.backoff.ExponentialBackoff(ctx, deleteVolumeCallBack)
	switch {
	case isVolumeNotFoundError(err):
		return false, ErrNotFound
	case err != nil:
		return false, fmt.Errorf("unable to delete disk: %w", err)
	default:
		return true, nil
	}
}

func (c *cloud) AttachDisk(ctx context.Context, volumeID, nodeID string) (string, error) {
	instance, err := c.getInstance(ctx, nodeID)
	if err != nil {
		return "", err
	}

	device, err := c.dm.NewDevice(*instance, volumeID)
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
		linkVolumeCallBack := func(ctx context.Context) (bool, error) {
			resp, httpRes, err := c.client.LinkVolume(ctx, request)
			logAPICall(ctx, "LinkVolume", request, resp, httpRes, err)
			return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
		}

		waitErr := c.backoff.ExponentialBackoff(ctx, linkVolumeCallBack)
		if waitErr != nil {
			return "", fmt.Errorf("could not attach volume: %w", waitErr)
		}
	}

	// This is the only situation where we taint the device
	if err := c.WaitForAttachmentState(ctx, volumeID, "attached"); err != nil {
		device.Taint()
		return "", err
	}

	// TODO: Double check the attachment to be 100% sure we attached the correct volume at the correct mountpoint
	// It could happen otherwise that we see the volume attached from a previous/separate AttachVolume call,
	// which could theoretically be against a different device (or even instance).

	return device.Path, nil
}

func (c *cloud) DetachDisk(ctx context.Context, volumeID, nodeID string) error {
	logger := klog.FromContext(ctx)
	{
		// Check if the volume is attached to VM
		request := osc.ReadVolumesRequest{
			Filters: &osc.FiltersVolume{
				VolumeIds: &[]string{volumeID},
			},
		}

		volume, err := c.getVolume(ctx, request, true)
		if err == nil && volume.HasState() && volume.GetState() == "available" {
			logger.V(4).Info("Volume is already available")
			return nil
		}
	}
	instance, err := c.getInstance(ctx, nodeID)
	if err != nil {
		return err
	}

	// TODO: check if attached
	device := c.dm.GetDevice(*instance, volumeID)
	defer device.Release(true)

	if !device.IsAlreadyAssigned {
		logger.V(4).Info("Volume is not assigned to node")
		return ErrNotFound
	}

	request := osc.UnlinkVolumeRequest{
		VolumeId: volumeID,
	}

	unlinkVolumeCallBack := func(ctx context.Context) (bool, error) {
		resp, httpRes, err := c.client.UnlinkVolume(ctx, request)
		logAPICall(ctx, "UnlinkVolume", request, resp, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}

	waitErr := c.backoff.ExponentialBackoff(ctx, unlinkVolumeCallBack)
	if waitErr != nil {
		return fmt.Errorf("could not detach volume: %w", waitErr)
	}

	if err := c.WaitForAttachmentState(ctx, volumeID, "detached"); err != nil {
		return err
	}

	return nil
}

// WaitForAttachmentState polls until the attachment status is the expected value.
func (c *cloud) WaitForAttachmentState(ctx context.Context, volumeID, state string) error {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("Waiting till attachment state is " + state)
	start := time.Now()
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}
	verifyVolumeFunc := func(ctx context.Context) (bool, error) {
		volume, err := c.getVolume(ctx, request, false)
		if err != nil {
			logger.V(4).Error(err, "cannot check state")
			return false, nil
		}

		if len(volume.GetLinkedVolumes()) == 0 && state == "detached" {
			return true, nil
		}

		for _, a := range volume.GetLinkedVolumes() {
			if a.GetState() == state {
				return true, nil
			}
		}
		return false, nil
	}

	waitErr := wait.PollUntilContextCancel(ctx, 2*time.Second, false, verifyVolumeFunc)
	logger.V(4).Info("End of wait", "success", waitErr == nil, "duration", time.Since(start))
	if waitErr != nil {
		return fmt.Errorf("could not check attachment state: %w", waitErr)
	}
	return nil
}

// FIXME: as the code also checks size, either the name should not be Get or the size should be checked by caller.
func (c *cloud) GetDiskByName(ctx context.Context, name string, capacityBytes int64) (Disk, error) {
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			TagKeys:   &[]string{VolumeNameTagKey},
			TagValues: &[]string{name},
		},
	}

	volume, err := c.getVolume(ctx, request, true)
	if err != nil {
		return Disk{}, err
	}

	if volume.GetSize() != util.BytesToGiB(capacityBytes) {
		return Disk{}, ErrDiskExistsDiffSize
	}

	return Disk{
		VolumeID:         volume.GetVolumeId(),
		CapacityGiB:      volume.GetSize(),
		AvailabilityZone: volume.GetSubregionName(),
		SnapshotID:       volume.GetSnapshotId(),
	}, nil
}

func (c *cloud) GetDiskByID(ctx context.Context, volumeID string) (Disk, error) {
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}

	volume, err := c.getVolume(ctx, request, true)
	if err != nil {
		return Disk{}, err
	}

	return Disk{
		VolumeID:         volume.GetVolumeId(),
		CapacityGiB:      volume.GetSize(),
		AvailabilityZone: volume.GetSubregionName(),
		SnapshotID:       volume.GetSnapshotId(),
	}, nil
}

// FiXME: errors are not properly handled.
func (c *cloud) IsExistInstance(ctx context.Context, nodeID string) bool {
	_, err := c.getInstance(ctx, nodeID)
	return err == nil
}

func (c *cloud) CreateSnapshot(ctx context.Context, volumeID string, snapshotOptions *SnapshotOptions) (snapshot Snapshot, err error) {
	descriptions := "Created by Outscale BSU CSI driver for volume " + volumeID

	request := osc.CreateSnapshotRequest{
		VolumeId:    &volumeID,
		Description: &descriptions,
	}

	var res osc.CreateSnapshotResponse
	createSnapshotCallBack := func(ctx context.Context) (bool, error) {
		var httpRes *http.Response
		res, httpRes, err = c.client.CreateSnapshot(ctx, request)
		logAPICall(ctx, "CreateSnapshot", request, res, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}

	waitErr := c.backoff.ExponentialBackoff(ctx, createSnapshotCallBack)
	if waitErr != nil {
		return Snapshot{}, fmt.Errorf("unable to create snapshot: %w", waitErr)
	}

	if !res.HasSnapshot() {
		return Snapshot{}, errors.New("nil CreateSnapshotResponse")
	}

	resourceTag := make([]osc.ResourceTag, 0, len(snapshotOptions.Tags))
	for key, value := range snapshotOptions.Tags {
		resourceTag = append(resourceTag, osc.ResourceTag{Key: key, Value: value})
	}
	requestTag := osc.CreateTagsRequest{
		ResourceIds: []string{res.Snapshot.GetSnapshotId()},
		Tags:        resourceTag,
	}

	var resTag osc.CreateTagsResponse
	createTagCallback := func(ctx context.Context) (bool, error) {
		var httpRes *http.Response
		var err error
		resTag, httpRes, err = c.client.CreateTags(ctx, requestTag)
		logAPICall(ctx, "CreateTags", requestTag, resTag, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}

	// If this fails, a duplicate snapshot is created, we need to try harder/longer, especially on TCP errors.
	waitErr = c.backoff.With(RetryOnErrors(), Steps(20)).ExponentialBackoff(ctx, createTagCallback)
	if waitErr != nil {
		return Snapshot{}, fmt.Errorf("unable to add tags: %w", waitErr)
	}

	err = c.waitForSnapshot(ctx, *res.GetSnapshot().SnapshotId)
	if err != nil {
		return Snapshot{}, fmt.Errorf("wait error: %w", err)
	}

	return c.oscSnapshotResponseToStruct(res.GetSnapshot()), nil
}

func (c *cloud) DeleteSnapshot(ctx context.Context, snapshotID string) (success bool, err error) {
	request := osc.DeleteSnapshotRequest{
		SnapshotId: snapshotID,
	}

	deleteSnapshotCallBack := func(ctx context.Context) (bool, error) {
		response, httpRes, err := c.client.DeleteSnapshot(ctx, request)
		logAPICall(ctx, "DeleteSnapshot", request, response, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}

	err = c.backoff.ExponentialBackoff(ctx, deleteSnapshotCallBack)
	switch {
	case isSnapshotNotFoundError(err):
		return false, ErrNotFound
	case err != nil:
		return false, fmt.Errorf("cound not delete snapshot: %w", err)
	default:
		return true, nil
	}
}

// waitForSnapshot waits for a snapshot to be in the "completed" state.
func (c *cloud) waitForSnapshot(ctx context.Context, snapshotID string) error {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("Waiting for snapshot to be completed")
	start := time.Now()

	request := osc.ReadSnapshotsRequest{
		Filters: &osc.FiltersSnapshot{
			SnapshotIds: &[]string{snapshotID},
		},
	}
	testVolume := func(ctx context.Context) (done bool, err error) {
		snap, err := c.getSnapshot(ctx, request, false)
		if err != nil {
			logger.V(4).Error(err, "cannot check state")
			return true, nil
		}
		switch snap.GetState() {
		case "error", "deleting":
			return false, fmt.Errorf("snapshot is in %q state", snap.GetState())
		case "completed":
			return true, nil
		default: // "in-queue", "pending"
			return false, nil
		}
	}
	err := wait.PollUntilContextCancel(ctx, 2*time.Second, false, testVolume)
	logger.V(4).Info("End of wait", "success", err == nil, "duration", time.Since(start))
	if err != nil {
		return fmt.Errorf("unable to wait for snapshot: %w", err)
	}
	return nil
}

func (c *cloud) GetSnapshotByName(ctx context.Context, name string) (snapshot Snapshot, err error) {
	request := osc.ReadSnapshotsRequest{
		Filters: &osc.FiltersSnapshot{
			TagKeys:   &[]string{SnapshotNameTagKey},
			TagValues: &[]string{name},
		},
	}

	oscsnapshot, err := c.getSnapshot(ctx, request, true)
	if err != nil {
		return Snapshot{}, err
	}

	return c.oscSnapshotResponseToStruct(oscsnapshot), nil
}

func (c *cloud) GetSnapshotByID(ctx context.Context, snapshotID string) (snapshot Snapshot, err error) {
	request := osc.ReadSnapshotsRequest{
		Filters: &osc.FiltersSnapshot{
			SnapshotIds: &[]string{snapshotID},
		},
	}

	oscsnapshot, err := c.getSnapshot(ctx, request, true)
	if err != nil {
		return Snapshot{}, err
	}

	return c.oscSnapshotResponseToStruct(oscsnapshot), nil
}

const maxResultsLimit = 1000

// ListSnapshots retrieves Outscale BSU snapshots for an optionally specified volume ID.  If maxResults is set, it will return up to maxResults snapshots.  If there are more snapshots than maxResults,
// a next token value will be returned to the client as well.  They can use this token with subsequent calls to retrieve the next page of results.
// Pagination not supported
func (c *cloud) ListSnapshots(ctx context.Context, volumeID string, maxResults int32, nextToken string) (listSnapshotsResponse ListSnapshotsResponse, err error) {
	if maxResults > maxResultsLimit {
		maxResults = maxResultsLimit
	}

	req := osc.ReadSnapshotsRequest{}
	if maxResults > 0 {
		req.ResultsPerPage = &maxResults
	}
	if nextToken != "" {
		req.NextPageToken = &nextToken
	}
	if len(volumeID) != 0 {
		req.Filters = &osc.FiltersSnapshot{
			VolumeIds: &[]string{volumeID},
		}
	}

	resp, err := c.listSnapshots(ctx, req)
	if err != nil {
		return ListSnapshotsResponse{}, err
	}
	snapshots := make([]Snapshot, 0, len(resp.Snapshots))
	for _, oscSnapshot := range resp.Snapshots {
		snapshots = append(snapshots, c.oscSnapshotResponseToStruct(oscSnapshot))
	}
	klog.FromContext(ctx).V(5).Info(fmt.Sprintf("%d snapshots found", len(snapshots)))
	return ListSnapshotsResponse{
		Snapshots: snapshots,
		NextToken: nextToken,
	}, nil
}

func (c *cloud) oscSnapshotResponseToStruct(oscSnapshot osc.Snapshot) Snapshot {
	if !oscSnapshot.HasSnapshotId() ||
		!oscSnapshot.HasVolumeId() ||
		!oscSnapshot.HasState() {
		return Snapshot{}
	}
	snapshotSize := util.GiBToBytes(oscSnapshot.GetVolumeSize())
	return Snapshot{
		SnapshotID:     oscSnapshot.GetSnapshotId(),
		SourceVolumeID: oscSnapshot.GetVolumeId(),
		Size:           snapshotSize,
		State:          oscSnapshot.GetState(),
	}
}

// Pagination not supported
func (c *cloud) getVolume(ctx context.Context, request osc.ReadVolumesRequest, backoff bool) (*osc.Volume, error) {
	var response osc.ReadVolumesResponse
	getVolumeCallback := func(ctx context.Context) (bool, error) {
		var httpRes *http.Response
		var err error

		response, httpRes, err = c.client.ReadVolumes(ctx, request)
		logAPICall(ctx, "ReadVolumes", request, response, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}

	var err error
	if backoff {
		err = c.backoff.ExponentialBackoff(ctx, getVolumeCallback)
	} else {
		_, err = getVolumeCallback(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("unable to read volume: %w", err)
	}

	volumes := response.GetVolumes()
	switch len(volumes) {
	case 0:
		return nil, ErrNotFound
	case 1:
		return &volumes[0], nil
	default:
		return nil, ErrMultiDisks
	}
}

// Pagination not supported
func (c *cloud) getInstance(ctx context.Context, vmID string) (*osc.Vm, error) {
	request := osc.ReadVmsRequest{
		Filters: &osc.FiltersVm{
			VmIds: &[]string{vmID},
		},
	}
	var response osc.ReadVmsResponse

	getInstanceCallback := func(ctx context.Context) (bool, error) {
		var httpRes *http.Response
		var err error
		response, httpRes, err = c.client.ReadVms(ctx, request)
		logAPICall(ctx, "ReadVms", request, response, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}

	waitErr := c.backoff.ExponentialBackoff(ctx, getInstanceCallback)
	if waitErr != nil {
		return nil, fmt.Errorf("error listing instances: %w", waitErr)
	}

	vms := *response.Vms
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
func (c *cloud) getSnapshot(ctx context.Context, request osc.ReadSnapshotsRequest, backoff bool) (osc.Snapshot, error) {
	var snapshots []osc.Snapshot
	var response osc.ReadSnapshotsResponse
	getSnapshotsCallback := func(ctx context.Context) (bool, error) {
		var httpRes *http.Response
		var err error
		response, httpRes, err = c.client.ReadSnapshots(ctx, request)
		logAPICall(ctx, "ReadSnapshots", request, response, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}

	var err error
	if backoff {
		err = c.backoff.ExponentialBackoff(ctx, getSnapshotsCallback)
	} else {
		_, err = getSnapshotsCallback(ctx)
	}

	if err != nil {
		return osc.Snapshot{}, fmt.Errorf("error listing snapshots: %w", err)
	}
	snapshots = response.GetSnapshots()
	switch len(snapshots) {
	case 0:
		return osc.Snapshot{}, ErrNotFound
	case 1:
		return snapshots[0], nil
	default:
		return osc.Snapshot{}, ErrMultiSnapshots
	}
}

// listSnapshots returns all snapshots based from a request
// Pagination not supported
func (c *cloud) listSnapshots(ctx context.Context, request osc.ReadSnapshotsRequest) (oscListSnapshotsResponse, error) {
	logger := klog.FromContext(ctx)
	logger.V(4).Info(fmt.Sprintf("getSnapshot: %+v", request))
	var response osc.ReadSnapshotsResponse
	listSnapshotsCallBack := func(ctx context.Context) (bool, error) {
		var httpRes *http.Response
		var err error
		response, httpRes, err = c.client.ReadSnapshots(ctx, request)
		logAPICall(ctx, "ReadSnapshots", request, response, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}
	waitErr := c.backoff.ExponentialBackoff(ctx, listSnapshotsCallBack)

	if waitErr != nil {
		return oscListSnapshotsResponse{}, fmt.Errorf("error listing snapshots: %w", waitErr)
	}

	return oscListSnapshotsResponse{
		Snapshots: response.GetSnapshots(),
	}, nil
}

// waitForVolume waits for volume to be in the "available" state.
// On a random Outscale account (shared among several developers) it took 4s on average.
func (c *cloud) waitForVolume(ctx context.Context, volumeID string) error {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("Waiting for volume to be available")
	start := time.Now()

	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}
	testVolume := func(ctx context.Context) (done bool, err error) {
		vol, err := c.getVolume(ctx, request, false)
		if err != nil {
			logger.V(4).Error(err, "cannot check state")
			return true, nil
		}
		switch vol.GetState() {
		case "error", "deleting":
			return false, fmt.Errorf("volume is in %q state", vol.GetState())
		case "available":
			return true, nil
		default:
			return false, nil
		}
	}
	err := wait.PollUntilContextCancel(ctx, 2*time.Second, false, testVolume)
	logger.V(4).Info("End of wait", "success", err == nil, "duration", time.Since(start))
	if err != nil {
		return fmt.Errorf("unable to wait for volume: %w", err)
	}
	return nil
}

// ResizeDisk resizes an BSU volume in GiB increments, rouding up to the next possible allocatable unit.
// It returns the volume size in bytes after this call or an error if the size couldn't be determined.
func (c *cloud) ResizeDisk(ctx context.Context, volumeID string, newSizeBytes int64) (int64, error) {
	logger := klog.FromContext(ctx)
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}
	logger.V(4).Info("Check Volume state.")
	volume, err := c.getVolume(ctx, request, true)
	if err != nil {
		return 0, err
	}

	if volume.GetState() != "available" {
		return 0, fmt.Errorf("could not modify volume in non 'available' state: %v", volume)
	}

	// resizes in chunks of GiB (not GB)
	newSizeGiB := util.RoundUpGiB(newSizeBytes)
	oldSizeGiB := volume.GetSize()

	// Even if existing volume size is greater than user requested size, we should ensure that there are no pending
	// volume modifications objects or volume has completed previously issued modification request.
	if oldSizeGiB >= newSizeGiB {
		logger.V(4).Info(fmt.Sprintf("Volume current size (%d GiB) is greater or equal to the new size (%d GiB)", oldSizeGiB, newSizeGiB))
		return util.GiBToBytes(oldSizeGiB), nil
	}

	logger.V(4).Info(fmt.Sprintf("Expanding volume to %dGiB", newSizeGiB))
	reqSize := newSizeGiB
	req := osc.UpdateVolumeRequest{
		Size:     &reqSize,
		VolumeId: volumeID,
	}

	updateVolumeCallBack := func(ctx context.Context) (bool, error) {
		response, httpRes, err := c.client.UpdateVolume(ctx, req)
		logAPICall(ctx, "UpdateVolume", req, response, httpRes, err)
		return c.backoff.OAPIResponseBackoff(ctx, httpRes, err)
	}

	waitErr := c.backoff.ExponentialBackoff(ctx, updateVolumeCallBack)
	if waitErr != nil {
		return 0, fmt.Errorf("could not modify volume: %w", waitErr)
	}
	delay := 5
	logger.V(4).Info(fmt.Sprintf("Waiting %d sec for the effective modification.", delay))
	time.Sleep(time.Duration(delay) * time.Second)
	return c.checkDesiredSize(ctx, volumeID, newSizeGiB)
}

// Checks for desired size on volume by also verifying volume size by describing volume.
// This is to get around potential eventual consistency problems with describing volume modifications
// objects and ensuring that we read two different objects to verify volume state.
func (c *cloud) checkDesiredSize(ctx context.Context, volumeID string, newSizeGiB int32) (int64, error) {
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}
	volume, err := c.getVolume(ctx, request, true)
	if err != nil {
		return 0, err
	}

	// resizes in chunks of GiB (not GB)
	szGiB := volume.GetSize()
	if szGiB >= newSizeGiB {
		return util.GiBToBytes(szGiB), nil
	}
	return util.GiBToBytes(szGiB), fmt.Errorf("volume %q is still being expanded to %d size", volumeID, newSizeGiB)
}
