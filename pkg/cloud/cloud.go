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
	_nethttp "net/http"
	"time"

	"os"

	osc "github.com/outscale/osc-sdk-go/v2"

	"github.com/aws/aws-sdk-go/aws/awserr"
	dm "github.com/outscale-dev/osc-bsu-csi-driver/pkg/cloud/devicemanager"
	"github.com/outscale-dev/osc-bsu-csi-driver/pkg/util"
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
	//(for example, starting a file system)
	VolumeTypeGP2 = "gp2"
	//Workloads where you must access data frequently (for example, a database)
	//Critical business applications that can be blocked by a low performance when accessing data stored on the volume
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
	// ErrMultiDisks is an error that is returned when multiple
	// disks are found with the same volume name.
	ErrMultiDisks = errors.New("Multiple disks with same name")

	// ErrDiskExistsDiffSize is an error that is returned if a disk with a given
	// name, but different size, is found.
	ErrDiskExistsDiffSize = errors.New("There is already a disk with same name and different size")

	// ErrNotFound is returned when a resource is not found.
	ErrNotFound = errors.New("Resource was not found")

	// ErrAlreadyExists is returned when a resource is already existent.
	ErrAlreadyExists = errors.New("Resource already exists")

	// ErrMultiSnapshots is returned when multiple snapshots are found
	// with the same ID
	ErrMultiSnapshots = errors.New("Multiple snapshots with the same name found")

	// ErrInvalidMaxResults is returned when a MaxResults pagination parameter is between 1 and 4
	ErrInvalidMaxResults = errors.New("MaxResults parameter must be 0 or greater than or equal to 5")
)

// Disk represents a BSU volume
type Disk struct {
	VolumeID         string
	CapacityGiB      int64
	AvailabilityZone string
	SnapshotID       string
}

// DiskOptions represents parameters to create an BSU volume
type DiskOptions struct {
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
	ReadyToUse     bool
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
	ListSnapshots(ctx context.Context, volumeID string, maxResults int64, nextToken string) (listSnapshotsResponse ListSnapshotsResponse, err error)
}

type OscInterface interface {
	CreateVolume(ctx context.Context, localVarOptionals osc.CreateVolumeRequest) (osc.CreateVolumeResponse, *_nethttp.Response, error)
	CreateTags(ctx context.Context, localVarOptionals osc.CreateTagsRequest) (osc.CreateTagsResponse, *_nethttp.Response, error)
	ReadVolumes(ctx context.Context, localVarOptionals osc.ReadVolumesRequest) (osc.ReadVolumesResponse, *_nethttp.Response, error)
	DeleteVolume(ctx context.Context, localVarOptionals osc.DeleteVolumeRequest) (osc.DeleteVolumeResponse, *_nethttp.Response, error)
	LinkVolume(ctx context.Context, localVarOptionals osc.LinkVolumeRequest) (osc.LinkVolumeResponse, *_nethttp.Response, error)
	UnlinkVolume(ctx context.Context, localVarOptionals osc.UnlinkVolumeRequest) (osc.UnlinkVolumeResponse, *_nethttp.Response, error)
	CreateSnapshot(ctx context.Context, localVarOptionals osc.CreateSnapshotRequest) (osc.CreateSnapshotResponse, *_nethttp.Response, error)
	ReadSnapshots(ctx context.Context, localVarOptionals osc.ReadSnapshotsRequest) (osc.ReadSnapshotsResponse, *_nethttp.Response, error)
	DeleteSnapshot(ctx context.Context, localVarOptionals osc.DeleteSnapshotRequest) (osc.DeleteSnapshotResponse, *_nethttp.Response, error)
	ReadSubregions(ctx context.Context, localVarOptionals osc.ReadSubregionsRequest) (osc.ReadSubregionsResponse, *_nethttp.Response, error)
	ReadVms(ctx context.Context, localVarOptionals osc.ReadVmsRequest) (osc.ReadVmsResponse, *_nethttp.Response, error)
	UpdateVolume(ctx context.Context, localVarOptionals osc.UpdateVolumeRequest) (osc.UpdateVolumeResponse, *_nethttp.Response, error)
}

type OscClient struct {
	config *osc.Configuration
	auth   context.Context
	api    *osc.APIClient
}

func (client *OscClient) CreateVolume(ctx context.Context, localVarOptionals osc.CreateVolumeRequest) (osc.CreateVolumeResponse, *_nethttp.Response, error) {
	return client.api.VolumeApi.CreateVolume(client.auth).CreateVolumeRequest(localVarOptionals).Execute()
}

func (client *OscClient) CreateTags(ctx context.Context, localVarOptionals osc.CreateTagsRequest) (osc.CreateTagsResponse, *_nethttp.Response, error) {
	return client.api.TagApi.CreateTags(client.auth).CreateTagsRequest(localVarOptionals).Execute()
}

func (client *OscClient) ReadVolumes(ctx context.Context, localVarOptionals osc.ReadVolumesRequest) (osc.ReadVolumesResponse, *_nethttp.Response, error) {
	return client.api.VolumeApi.ReadVolumes(client.auth).ReadVolumesRequest(localVarOptionals).Execute()
}

func (client *OscClient) DeleteVolume(ctx context.Context, localVarOptionals osc.DeleteVolumeRequest) (osc.DeleteVolumeResponse, *_nethttp.Response, error) {
	return client.api.VolumeApi.DeleteVolume(client.auth).DeleteVolumeRequest(localVarOptionals).Execute()
}

func (client *OscClient) LinkVolume(ctx context.Context, localVarOptionals osc.LinkVolumeRequest) (osc.LinkVolumeResponse, *_nethttp.Response, error) {
	return client.api.VolumeApi.LinkVolume(client.auth).LinkVolumeRequest(localVarOptionals).Execute()
}

func (client *OscClient) UnlinkVolume(ctx context.Context, localVarOptionals osc.UnlinkVolumeRequest) (osc.UnlinkVolumeResponse, *_nethttp.Response, error) {
	return client.api.VolumeApi.UnlinkVolume(client.auth).UnlinkVolumeRequest(localVarOptionals).Execute()
}

func (client *OscClient) CreateSnapshot(ctx context.Context, localVarOptionals osc.CreateSnapshotRequest) (osc.CreateSnapshotResponse, *_nethttp.Response, error) {
	return client.api.SnapshotApi.CreateSnapshot(client.auth).CreateSnapshotRequest(localVarOptionals).Execute()
}

func (client *OscClient) ReadSnapshots(ctx context.Context, localVarOptionals osc.ReadSnapshotsRequest) (osc.ReadSnapshotsResponse, *_nethttp.Response, error) {
	return client.api.SnapshotApi.ReadSnapshots(client.auth).ReadSnapshotsRequest(localVarOptionals).Execute()
}

func (client *OscClient) DeleteSnapshot(ctx context.Context, localVarOptionals osc.DeleteSnapshotRequest) (osc.DeleteSnapshotResponse, *_nethttp.Response, error) {
	return client.api.SnapshotApi.DeleteSnapshot(client.auth).DeleteSnapshotRequest(localVarOptionals).Execute()
}

func (client *OscClient) ReadSubregions(ctx context.Context, localVarOptionals osc.ReadSubregionsRequest) (osc.ReadSubregionsResponse, *_nethttp.Response, error) {
	return client.api.SubregionApi.ReadSubregions(client.auth).ReadSubregionsRequest(localVarOptionals).Execute()
}

func (client *OscClient) ReadVms(ctx context.Context, localVarOptionals osc.ReadVmsRequest) (osc.ReadVmsResponse, *_nethttp.Response, error) {
	return client.api.VmApi.ReadVms(client.auth).ReadVmsRequest(localVarOptionals).Execute()
}

func (client *OscClient) UpdateVolume(ctx context.Context, localVarOptionals osc.UpdateVolumeRequest) (osc.UpdateVolumeResponse, *_nethttp.Response, error) {
	return client.api.VolumeApi.UpdateVolume(client.auth).UpdateVolumeRequest(localVarOptionals).Execute()
}

var _ OscInterface = &OscClient{}

type cloud struct {
	region string
	dm     dm.DeviceManager
	client OscInterface
}

var _ Cloud = &cloud{}

// NewCloud returns a new instance of Outscale cloud
// It panics if session is invalid
func NewCloud(region string) (Cloud, error) {
	return newOscCloud(region)
}

func newOscCloud(region string) (Cloud, error) {
	client := &OscClient{}
	// Set User-Agent with name and version of the CSI driver
	version := util.GetVersion()

	client.config = osc.NewConfiguration()
	client.config.Debug = true
	client.config.UserAgent = fmt.Sprintf("osc-bsu-csi-driver/%s", version.DriverVersion)

	client.api = osc.NewAPIClient(client.config)

	client.auth = context.WithValue(context.Background(), osc.ContextAWSv4, osc.AWSv4{
		AccessKey: os.Getenv("OSC_ACCESS_KEY"),
		SecretKey: os.Getenv("OSC_SECRET_KEY"),
	})
	client.auth = context.WithValue(client.auth, osc.ContextServerIndex, 0)
	client.auth = context.WithValue(client.auth, osc.ContextServerVariables, map[string]string{"region": region})

	return &cloud{
		region: region,
		dm:     dm.NewDeviceManager(),
		client: client,
	}, nil
}

func IsNilDisk(disk Disk) bool {
	return disk.VolumeID == ""
}

func IsNilSnapshot(snapshot Snapshot) bool {
	return snapshot.SnapshotID == ""
}

func (c *cloud) CreateDisk(ctx context.Context, volumeName string, diskOptions *DiskOptions) (Disk, error) {
	klog.Infof("Debug CreateDisk: %+v, %v", volumeName, diskOptions)
	var (
		createType string
		iops       int64
		request    osc.CreateVolumeRequest
	)
	capacityGiB := util.BytesToGiB(diskOptions.CapacityBytes)
	request.SetSize(int32(capacityGiB))

	switch diskOptions.VolumeType {
	case VolumeTypeGP2, VolumeTypeSTANDARD:
		createType = diskOptions.VolumeType
	case VolumeTypeIO1:
		createType = diskOptions.VolumeType

		iopsPerGb := diskOptions.IOPSPerGB
		if iopsPerGb > MaxIopsPerGb {
			iopsPerGb = 300
		}

		iops = capacityGiB * int64(iopsPerGb)
		if iops < MinTotalIOPS {
			iops = MinTotalIOPS
		}
		if iops > MaxTotalIOPS {
			iops = MaxTotalIOPS
		}

		request.SetIops(int32(iops))
	case "":
		createType = DefaultVolumeType
	default:
		return Disk{}, fmt.Errorf("invalid Outscale VolumeType %q", diskOptions.VolumeType)
	}

	request.SetVolumeType(createType)

	var resourceTag []osc.ResourceTag
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
		return Disk{}, fmt.Errorf("Encryption is not supported yet by Outscale API")
	}

	snapshotID := diskOptions.SnapshotID
	if len(snapshotID) != 0 {
		request.SetSnapshotId(snapshotID)
	}

	var creation osc.CreateVolumeResponse
	createVolumeCallBack := func() (bool, error) {
		var httpRes *_nethttp.Response
		var err error
		creation, httpRes, err = c.client.CreateVolume(ctx, request)
		klog.Infof("Debug response CreateVolume: response(%+v), err(%v), httpRes(%v)", creation, err, httpRes)
		if err != nil {
			if httpRes != nil {
				fmt.Fprintln(os.Stderr, httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, fmt.Errorf("could not create volume in Outscale: %v", err)
		}
		return true, nil
	}

	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, createVolumeCallBack)
	if waitErr != nil {
		return Disk{}, waitErr
	}

	if !creation.HasVolume() {
		return Disk{}, fmt.Errorf("volume is empty when returned by CreateVolume")
	}

	volumeID := creation.Volume.GetVolumeId()
	if len(volumeID) == 0 {
		return Disk{}, fmt.Errorf("volume ID was not returned by CreateVolume")
	}

	size := creation.Volume.GetSize()
	if size == 0 {
		return Disk{}, fmt.Errorf("disk size was not returned by CreateVolume")
	}

	requestTag := osc.CreateTagsRequest{
		ResourceIds: []string{volumeID},
		Tags:        resourceTag,
	}

	createTagsCallBack := func() (bool, error) {
		resTag, httpRes, err := c.client.CreateTags(ctx, requestTag)
		klog.Infof("Debug response CreateTags: response(%+v), err(%v), httpRes(%v)", resTag, err, httpRes)
		if err != nil {
			if httpRes != nil {
				fmt.Fprintln(os.Stderr, httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, fmt.Errorf("error creating tags %v of volume %v: %v, http Status: %v", resTag, volumeID, err, httpRes)
		}
		return true, nil
	}

	backoff = util.EnvBackoff()
	waitErr = wait.ExponentialBackoff(backoff, createTagsCallBack)
	if waitErr != nil {
		return Disk{}, waitErr
	}

	if err := c.waitForVolume(ctx, volumeID); err != nil {
		return Disk{}, fmt.Errorf("failed to get an available volume in Outscale: %v", err)
	}

	return Disk{CapacityGiB: int64(size), VolumeID: volumeID, AvailabilityZone: zone, SnapshotID: snapshotID}, nil
}

func (c *cloud) DeleteDisk(ctx context.Context, volumeID string) (bool, error) {
	klog.Infof("Debug DeleteDisk: %+v", volumeID)
	request := osc.DeleteVolumeRequest{
		VolumeId: volumeID,
	}

	deleteVolumeCallBack := func() (bool, error) {
		response, httpRes, err := c.client.DeleteVolume(ctx, request)
		klog.Infof("Debug response DeleteVolume: response(%+v), err(%v), httpRes(%v)", response, err, httpRes)
		if err != nil {
			if httpRes != nil {
				fmt.Fprintln(os.Stderr, httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, fmt.Errorf("DeleteDisk could not delete volume in Outscale: %v", err)
		}
		return true, nil
	}

	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, deleteVolumeCallBack)
	if waitErr != nil {
		return false, waitErr
	}

	return true, nil
}

func (c *cloud) AttachDisk(ctx context.Context, volumeID, nodeID string) (string, error) {
	klog.Infof("Debug AttachDisk: %+v, %v\n", volumeID, nodeID)
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
		var resp osc.LinkVolumeResponse
		var httpRes *_nethttp.Response
		linkVolumeCallBack := func() (bool, error) {
			resp, httpRes, err = c.client.LinkVolume(ctx, request)
			klog.Infof("Debug response AttachVolume: response(%+v), err(%v), httpRes(%v)\n", resp, err, httpRes)
			if err != nil {
				if httpRes != nil {
					fmt.Fprintln(os.Stderr, httpRes.Status)
				}
				requestStr := fmt.Sprintf("%v", request)
				if keepRetryWithError(
					requestStr,
					err,
					[]string{"RequestLimitExceeded"}) {
					return false, nil
				}
				return false, fmt.Errorf("could not attach volume %q to node %q: %v", volumeID, nodeID, err)
			}
			return true, nil
		}

		backoff := util.EnvBackoff()
		waitErr := wait.ExponentialBackoff(backoff, linkVolumeCallBack)
		if waitErr != nil {
			return "", waitErr
		}

		klog.V(5).Infof("AttachVolume volume=%q instance=%q request returned %v", volumeID, nodeID, resp)

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
	klog.Infof("Debug DetachDisk: %+v, %v\n", volumeID, nodeID)
	{
		klog.Infof("Check Volume state before detaching")
		//Check if the volume is attached to VM
		request := osc.ReadVolumesRequest{
			Filters: &osc.FiltersVolume{
				VolumeIds: &[]string{volumeID},
			},
		}

		volume, err := c.getVolume(ctx, request)
		klog.Infof("Check Volume state before detaching volume: %+v err: %+v",
			volume, err)
		if err == nil && volume.HasState() && volume.GetState() == "available" {
			klog.Warningf("Tolerate DetachDisk called on available volume: %s on %s",
				volumeID, nodeID)
			return nil
		}
	}
	klog.Infof("Debug Continue DetachDisk: %+v, %v\n", volumeID, nodeID)
	instance, err := c.getInstance(ctx, nodeID)
	if err != nil {
		return err
	}

	// TODO: check if attached
	device := c.dm.GetDevice(*instance, volumeID)
	defer device.Release(true)

	if !device.IsAlreadyAssigned {
		klog.Warningf("DetachDisk called on non-attached volume: %s", volumeID)
		return ErrNotFound
	}

	request := osc.UnlinkVolumeRequest{
		VolumeId: volumeID,
	}

	unlinkVolumeCallBack := func() (bool, error) {
		resp, httpRes, err := c.client.UnlinkVolume(ctx, request)
		klog.Infof("Debug response DetachVolume: response(%+v), err(%v) httpRes(%v)\n", resp, err, httpRes)
		if err != nil {
			if httpRes != nil {
				fmt.Fprintln(os.Stderr, httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, fmt.Errorf("could not detach volume %q from node %q: %v", volumeID, nodeID, err)
		}
		return true, nil
	}

	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, unlinkVolumeCallBack)
	if waitErr != nil {
		return waitErr
	}

	if err := c.WaitForAttachmentState(ctx, volumeID, "detached"); err != nil {
		return err
	}

	return nil
}

// WaitForAttachmentState polls until the attachment status is the expected value.
func (c *cloud) WaitForAttachmentState(ctx context.Context, volumeID, state string) error {
	klog.Infof("Debug WaitForAttachmentState: %+v, %v\n", volumeID, state)
	// Most attach/detach operations on Outscale finish within 1-4 seconds.
	// By using 1 second starting interval with a backoff of 1.8,
	// we get [1, 1.8, 3.24, 5.832000000000001, 10.4976].
	// In total we wait for 2601 seconds.
	verifyVolumeFunc := func() (bool, error) {
		request := osc.ReadVolumesRequest{
			Filters: &osc.FiltersVolume{
				VolumeIds: &[]string{volumeID},
			},
		}

		volume, err := c.getVolume(ctx, request)
		if err != nil {
			return false, err
		}

		if len(volume.GetLinkedVolumes()) == 0 {
			if state == "detached" {
				return true, nil
			}
		}

		for _, a := range volume.GetLinkedVolumes() {
			if a.GetState() == "" {
				klog.Warningf("Ignoring nil attachment state for volume %q: %v", volumeID, a)
				continue
			}
			if a.GetState() == state {
				return true, nil
			}
		}
		return false, nil
	}

	backoff := util.EnvBackoff()
	return wait.ExponentialBackoff(backoff, verifyVolumeFunc)
}

func (c *cloud) GetDiskByName(ctx context.Context, name string, capacityBytes int64) (Disk, error) {
	klog.Infof("Debug GetDiskByName: %+v, %v\n", name, capacityBytes)
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			TagKeys:   &[]string{VolumeNameTagKey},
			TagValues: &[]string{name},
		},
	}

	volume, err := c.getVolume(ctx, request)
	if err != nil {
		return Disk{}, err
	}

	volSizeBytes := volume.GetSize()
	if int64(volSizeBytes) != util.BytesToGiB(capacityBytes) {
		return Disk{}, ErrDiskExistsDiffSize
	}

	return Disk{
		VolumeID:         volume.GetVolumeId(),
		CapacityGiB:      int64(volSizeBytes),
		AvailabilityZone: volume.GetSubregionName(),
	}, nil
}

func (c *cloud) GetDiskByID(ctx context.Context, volumeID string) (Disk, error) {
	klog.Infof("Debug GetDiskByID : %+v\n", volumeID)
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}

	volume, err := c.getVolume(ctx, request)
	if err != nil {
		return Disk{}, err
	}

	return Disk{
		VolumeID:         volume.GetVolumeId(),
		CapacityGiB:      int64(volume.GetSize()),
		AvailabilityZone: volume.GetSubregionName(),
	}, nil
}

func (c *cloud) IsExistInstance(ctx context.Context, nodeID string) bool {
	klog.Infof("Debug IsExistInstance : %+v\n", nodeID)
	_, err := c.getInstance(ctx, nodeID)
	return err == nil
}

func (c *cloud) CreateSnapshot(ctx context.Context, volumeID string, snapshotOptions *SnapshotOptions) (snapshot Snapshot, err error) {
	descriptions := "Created by Outscale BSU CSI driver for volume " + volumeID
	klog.Infof("Debug CreateSnapshot : %+v, %+v\n", volumeID, snapshotOptions)

	var resourceTag []osc.ResourceTag
	for key, value := range snapshotOptions.Tags {
		resourceTag = append(resourceTag, osc.ResourceTag{Key: key, Value: value})
	}
	klog.Infof("Debug tags = append( %+v ) \n", resourceTag)

	request := osc.CreateSnapshotRequest{
		VolumeId:    &volumeID,
		Description: &descriptions,
	}

	klog.Infof("Debug request := CreateSnapshotInput %+v  \n", request)
	var res osc.CreateSnapshotResponse
	createSnapshotCallBack := func() (bool, error) {
		var httpRes *_nethttp.Response
		res, httpRes, err = c.client.CreateSnapshot(ctx, request)
		klog.Infof("Debug response CreateSnapshot: response(%+v), err(%v), httpRes(%v)\n", res, err, httpRes)
		if err != nil {
			if httpRes != nil {
				fmt.Fprintln(os.Stderr, httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, createSnapshotCallBack)
	if waitErr != nil {
		return Snapshot{}, waitErr
	}

	if !res.HasSnapshot() {
		return Snapshot{}, fmt.Errorf("nil CreateSnapshotResponse")
	}
	klog.Infof("Debug res, err := c.ec2.CreateSnapshotWithContext(ctx, request) : %+v\n", res)

	requestTag := osc.CreateTagsRequest{
		ResourceIds: []string{res.Snapshot.GetSnapshotId()},
		Tags:        resourceTag,
	}

	klog.Infof("Debug requestTag(%+v)\n", requestTag)
	var resTag osc.CreateTagsResponse
	createTagCallback := func() (bool, error) {
		var httpResTag *_nethttp.Response
		var errTag error
		resTag, httpResTag, errTag = c.client.CreateTags(ctx, requestTag)
		klog.Infof("Debug resTag( %+v ) errTag( %+v ), httpResTag(%+v)\n", resTag, errTag, httpResTag)
		if errTag != nil {
			if httpResTag != nil {
				fmt.Fprintln(os.Stderr, httpResTag.Status)
			}
			requestStr := fmt.Sprintf("%v", resTag)
			if keepRetryWithError(
				requestStr,
				errTag,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, errTag
		}
		return true, nil
	}

	backoff = util.EnvBackoff()
	waitErr = wait.ExponentialBackoff(backoff, createTagCallback)
	if waitErr != nil {
		return Snapshot{}, waitErr
	}

	return c.oscSnapshotResponseToStruct(res.GetSnapshot()), nil
}

func (c *cloud) DeleteSnapshot(ctx context.Context, snapshotID string) (success bool, err error) {
	klog.Infof("Debug DeleteSnapshot : %+v\n", snapshotID)

	request := osc.DeleteSnapshotRequest{
		SnapshotId: snapshotID,
	}

	deleteSnapshotCallBack := func() (bool, error) {
		response, httpRes, err := c.client.DeleteSnapshot(ctx, request)
		klog.Infof("Debug response DeleteSnapshot: response(%+v), err(%v), httpRes(%v)\n", response, err, httpRes)
		if err != nil {
			if httpRes != nil {
				fmt.Fprintln(os.Stderr, httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, fmt.Errorf("DeleteSnapshot could not delete volume: %v", err)
		}
		return true, nil
	}

	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, deleteSnapshotCallBack)
	if waitErr != nil {
		return false, waitErr
	}

	return true, nil
}

func (c *cloud) GetSnapshotByName(ctx context.Context, name string) (snapshot Snapshot, err error) {
	klog.Infof("Debug GetSnapshotByName : %+v\n", name)
	request := osc.ReadSnapshotsRequest{
		Filters: &osc.FiltersSnapshot{
			TagKeys:   &[]string{SnapshotNameTagKey},
			TagValues: &[]string{name},
		},
	}

	oscsnapshot, err := c.getSnapshot(ctx, request)
	if err != nil {
		return Snapshot{}, err
	}

	return c.oscSnapshotResponseToStruct(oscsnapshot), nil
}

func (c *cloud) GetSnapshotByID(ctx context.Context, snapshotID string) (snapshot Snapshot, err error) {
	klog.Infof("Debug GetSnapshotByID : %+v\n", snapshotID)
	request := osc.ReadSnapshotsRequest{
		Filters: &osc.FiltersSnapshot{
			SnapshotIds: &[]string{snapshotID},
		},
	}

	oscsnapshot, err := c.getSnapshot(ctx, request)
	if err != nil {
		return Snapshot{}, err
	}

	return c.oscSnapshotResponseToStruct(oscsnapshot), nil
}

// ListSnapshots retrieves Outscale BSU snapshots for an optionally specified volume ID.  If maxResults is set, it will return up to maxResults snapshots.  If there are more snapshots than maxResults,
// a next token value will be returned to the client as well.  They can use this token with subsequent calls to retrieve the next page of results.
// Pagination not supported
func (c *cloud) ListSnapshots(ctx context.Context, volumeID string, maxResults int64, nextToken string) (listSnapshotsResponse ListSnapshotsResponse, err error) {
	klog.Infof("Debug ListSnapshots : %+v, %+v, %+v\n", volumeID, maxResults, nextToken)

	request := osc.ReadSnapshotsRequest{
		Filters: &osc.FiltersSnapshot{
			VolumeIds: &[]string{},
		},
	}

	if len(volumeID) != 0 {
		request = osc.ReadSnapshotsRequest{
			Filters: &osc.FiltersSnapshot{
				VolumeIds: &[]string{volumeID},
			},
		}
	}

	oscSnapshotsResponse, err := c.listSnapshots(ctx, request)
	if err != nil {
		return ListSnapshotsResponse{}, err
	}
	var snapshots []Snapshot
	for _, oscSnapshot := range oscSnapshotsResponse.Snapshots {
		snapshots = append(snapshots, c.oscSnapshotResponseToStruct(oscSnapshot))
	}

	if len(snapshots) == 0 {
		return ListSnapshotsResponse{}, ErrNotFound
	}

	return ListSnapshotsResponse{
		Snapshots: snapshots,
		NextToken: nextToken,
	}, nil
}

func (c *cloud) oscSnapshotResponseToStruct(oscSnapshot osc.Snapshot) Snapshot {
	klog.Infof("Debug oscSnapshotResponseToStruct : %+v\n", oscSnapshot)
	if !oscSnapshot.HasSnapshotId() ||
		!oscSnapshot.HasVolumeId() ||
		!oscSnapshot.HasState() {
		return Snapshot{}
	}
	snapshotSize := util.GiBToBytes(int64(oscSnapshot.GetVolumeSize()))
	snapshot := Snapshot{
		SnapshotID:     oscSnapshot.GetSnapshotId(),
		SourceVolumeID: oscSnapshot.GetVolumeId(),
		Size:           snapshotSize,
		//No StartTime for osc.Snapshot
		//CreationTime:   oscSnapshot.StartTime,
	}
	if oscSnapshot.GetState() == "completed" {
		snapshot.ReadyToUse = true
	} else {
		snapshot.ReadyToUse = false
	}

	return snapshot
}

func keepRetryWithError(requestStr string, err error, allowedErrors []string) bool {
	if awsError, ok := err.(awserr.Error); ok {
		for _, v := range allowedErrors {
			if awsError.Code() == v {
				klog.Warningf(
					"Retry even if got (%v) error on request (%s)",
					awsError.Code(),
					requestStr)
				return true
			}
		}
	}
	return false
}

// Pagination not supported
func (c *cloud) getVolume(ctx context.Context, request osc.ReadVolumesRequest) (*osc.Volume, error) {
	klog.Infof("Debug getVolume : %+v\n", request)
	var volume osc.Volume
	getVolumeCallback := func() (bool, error) {
		var volumes []osc.Volume

		var response osc.ReadVolumesResponse
		var httpRes *_nethttp.Response
		var err error

		response, httpRes, err = c.client.ReadVolumes(ctx, request)
		klog.Infof("Debug response ReadVolumes: response(%+v), err(%v)\n", response, err)

		if err != nil {
			if httpRes != nil {
				fmt.Fprintln(os.Stderr, httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, err
		}
		volumes = append(volumes, response.GetVolumes()...)

		if l := len(volumes); l > 1 {
			return false, ErrMultiDisks
		} else if l < 1 {
			return false, ErrNotFound
		}

		volume = volumes[0]
		return true, nil
	}

	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, getVolumeCallback)

	if waitErr != nil {
		return nil, waitErr
	}

	return &volume, nil
}

// Pagination not supported
func (c *cloud) getInstance(ctx context.Context, vmID string) (*osc.Vm, error) {
	klog.Infof("Debug  getInstance : %+v\n", vmID)
	var instances []osc.Vm

	request := osc.ReadVmsRequest{
		Filters: &osc.FiltersVm{
			VmIds: &[]string{vmID},
		},
	}

	getInstanceCallback := func() (bool, error) {
		response, httpRes, err := c.client.ReadVms(ctx, request)
		klog.Infof("Debug response DescribeInstances: response(%+v), err(%v), httpRes(%v)\n", response, err, httpRes)
		if err != nil {
			if httpRes != nil {
				fmt.Fprintln(os.Stderr, httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, fmt.Errorf("error listing Outscale instances: %q", err)
		}

		instances = append(instances, response.GetVms()...)

		return true, nil
	}

	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, getInstanceCallback)
	if waitErr != nil {
		return nil, waitErr
	}

	if l := len(instances); l > 1 {
		return nil, fmt.Errorf("found %d instances with ID %q", l, vmID)
	} else if l < 1 {
		return nil, ErrNotFound
	}

	return &instances[0], nil
}

// Pagination not supported
func (c *cloud) getSnapshot(ctx context.Context, request osc.ReadSnapshotsRequest) (osc.Snapshot, error) {
	klog.Infof("Debug  getSnapshot: %+v\n", request)
	var snapshots []osc.Snapshot
	getSnapshotsCallback := func() (bool, error) {
		response, httpRes, err := c.client.ReadSnapshots(ctx, request)
		klog.Infof("Debug response DescribeSnapshots: response(%+v), err(%v)\n", response, err)
		if err != nil {
			if httpRes != nil {
				fmt.Fprintln(os.Stderr, httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, err
		}
		snapshots = append(snapshots, response.GetSnapshots()...)
		return true, nil
	}

	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, getSnapshotsCallback)
	if waitErr != nil {
		return osc.Snapshot{}, waitErr
	}
	klog.Infof("Debug snapshots: %+v, len(snapshots): %+v\n", snapshots, len(snapshots))
	if l := len(snapshots); l > 1 {
		return osc.Snapshot{}, ErrMultiSnapshots
	} else if l < 1 {
		return osc.Snapshot{}, ErrNotFound
	}
	klog.Infof("Debug (snapshots[0]): %+v\n", snapshots[0])
	return snapshots[0], nil
}

// listSnapshots returns all snapshots based from a request
// Pagination not supported
func (c *cloud) listSnapshots(ctx context.Context, request osc.ReadSnapshotsRequest) (oscListSnapshotsResponse, error) {
	klog.Infof("Debug listSnapshots : %+v\n", request)
	var snapshots []osc.Snapshot
	var response osc.ReadSnapshotsResponse
	var httpRes *_nethttp.Response
	var err error
	listSnapshotsCallBack := func() (bool, error) {
		response, httpRes, err = c.client.ReadSnapshots(ctx, request)
		klog.Infof("Debug response DescribeSnapshots: response(%+v), err(%v), httpRes(%v)\n", response, err, httpRes)
		if err != nil {
			if httpRes != nil {
				fmt.Fprintln(os.Stderr, httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, err
		}
		fmt.Printf("Debug  response : %+v\n", response)
		return true, nil
	}
	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, listSnapshotsCallBack)

	if waitErr != nil {
		return oscListSnapshotsResponse{}, waitErr
	}

	klog.Infof("Debug response.Snapshots : %+v\n", response.Snapshots)
	snapshots = append(snapshots, response.GetSnapshots()...)

	return oscListSnapshotsResponse{
		Snapshots: snapshots,
	}, nil
}

// waitForVolume waits for volume to be in the "available" state.
// On a random Outscale account (shared among several developers) it took 4s on average.
func (c *cloud) waitForVolume(ctx context.Context, volumeID string) error {
	klog.Infof("Debug waitForVolume : %+v\n", volumeID)
	var (
		checkInterval = 3 * time.Second
		// This timeout can be "ovewritten" if the value returned by ctx.Deadline()
		// comes sooner. That value comes from the external provisioner controller.
		checkTimeout = 1 * time.Minute
	)

	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}

	err := wait.Poll(checkInterval, checkTimeout, func() (done bool, err error) {
		vol, err := c.getVolume(ctx, request)
		if err != nil {
			return true, err
		}
		if vol.GetState() != "" {
			return vol.GetState() == "available", nil
		}
		return false, nil
	})

	return err
}

// ResizeDisk resizes an BSU volume in GiB increments, rouding up to the next possible allocatable unit.
// It returns the volume size after this call or an error if the size couldn't be determined.
func (c *cloud) ResizeDisk(ctx context.Context, volumeID string, newSizeBytes int64) (int64, error) {
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}
	volume, err := c.getVolume(ctx, request)
	klog.Infof("Check Volume state before resizing volume: %+v err: %+v", volume, err)
	if err != nil {
		klog.Errorf("Empty or error during getting the volume %s", volumeID)
		return 0, err
	}

	if volume.GetState() != "available" {
		return 0, fmt.Errorf("could not modify Outscale volume in non 'available' state: %v", volume)
	}

	//resizes in chunks of GiB (not GB)
	newSizeGiB := int32(util.RoundUpGiB(newSizeBytes))
	oldSizeGiB := volume.GetSize()

	// Even if existing volume size is greater than user requested size, we should ensure that there are no pending
	// volume modifications objects or volume has completed previously issued modification request.
	if oldSizeGiB >= newSizeGiB {
		klog.V(5).Infof("Volume %q current size (%d GiB) is greater or equal to the new size (%d GiB)", volumeID, oldSizeGiB, newSizeGiB)
		return int64(oldSizeGiB), nil
	}

	klog.Infof("expanding volume %q to size %d", volumeID, newSizeGiB)
	reqSize := int32(newSizeGiB)
	req := osc.UpdateVolumeRequest{
		Size:     &reqSize,
		VolumeId: volumeID,
	}

	updateVolumeCallBack := func() (bool, error) {
		response, httpRes, err := c.client.UpdateVolume(ctx, req)
		klog.Infof("Debug response UpdateVolume: response(%+v), err(%v), httpRes(%v)", response, err, httpRes)
		if err != nil {
			if httpRes != nil {
				fmt.Fprintln(os.Stderr, httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, fmt.Errorf("could not modify volume %q: %v", volumeID, err)
		}
		return true, nil
	}

	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, updateVolumeCallBack)
	if waitErr != nil {
		return 0, waitErr
	}
	delay := 5
	klog.Infof("Waiting %v sec for the effective modification.", delay)
	time.Sleep(time.Duration(delay) * time.Second)
	return c.checkDesiredSize(ctx, volumeID, int64(newSizeGiB))
}

// Checks for desired size on volume by also verifying volume size by describing volume.
// This is to get around potential eventual consistency problems with describing volume modifications
// objects and ensuring that we read two different objects to verify volume state.
func (c *cloud) checkDesiredSize(ctx context.Context, volumeID string, newSizeGiB int64) (int64, error) {
	request := osc.ReadVolumesRequest{
		Filters: &osc.FiltersVolume{
			VolumeIds: &[]string{volumeID},
		},
	}
	volume, err := c.getVolume(ctx, request)
	if err != nil {
		return 0, err
	}

	//resizes in chunks of GiB (not GB)
	oldSizeGiB := volume.GetSize()
	if oldSizeGiB >= int32(newSizeGiB) {
		return int64(oldSizeGiB), nil
	}
	return int64(oldSizeGiB), fmt.Errorf("volume %q is still being expanded to %d size", volumeID, newSizeGiB)
}

// NewCloudWithoutMetadata to instantiate a cloud object outside osc instances
func NewCloudWithoutMetadata(region string) (Cloud, error) {
	if len(region) == 0 {
		return nil, fmt.Errorf("could not get region")
	}

	// Set User-Agent with name and version of the CSI driver
	version := util.GetVersion()

	client := &OscClient{}
	client.config = osc.NewConfiguration()
	client.config.Debug = true
	client.config.UserAgent = fmt.Sprintf("osc-bsu-csi-driver/%s", version.DriverVersion)

	client.api = osc.NewAPIClient(client.config)

	client.auth = context.WithValue(context.Background(), osc.ContextAWSv4, osc.AWSv4{
		AccessKey: os.Getenv("OSC_ACCESS_KEY"),
		SecretKey: os.Getenv("OSC_SECRET_KEY"),
	})

	client.auth = context.WithValue(client.auth, osc.ContextServerIndex, 0)
	client.auth = context.WithValue(client.auth, osc.ContextServerVariables, map[string]string{"region": region})

	return &cloud{
		region: region,
		dm:     dm.NewDeviceManager(),
		client: client,
	}, nil
}
