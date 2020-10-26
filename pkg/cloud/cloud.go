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

	"github.com/antihax/optional"
	"github.com/outscale/osc-sdk-go/osc"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	dm "github.com/kubernetes-sigs/aws-ebs-csi-driver/pkg/cloud/devicemanager"
	"github.com/kubernetes-sigs/aws-ebs-csi-driver/pkg/util"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"

	"reflect"
)

// AWS volume types
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

// AWS provisioning limits.
// Source: http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSVolumeTypes.html
const (
	// MinTotalIOPS represents the minimum Input Output per second.
	MinTotalIOPS = 100
	// MaxTotalIOPS represents the maximum Input Output per second.
	MaxTotalIOPS = 20000
	// MaxNumTagsPerResource represents the maximum number of tags per AWS resource.
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
	// AWSTagKeyPrefix is the prefix of the key value that is reserved for AWS.
	AWSTagKeyPrefix = "aws:"
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

// Disk represents a EBS volume
type Disk struct {
	VolumeID         string
	CapacityGiB      int64
	AvailabilityZone string
	SnapshotID       string
}

// DiskOptions represents parameters to create an EBS volume
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

// Snapshot represents an EBS volume snapshot
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

// SnapshotOptions represents parameters to create an EBS volume
type SnapshotOptions struct {
	Tags map[string]string
}

// oscListSnapshotsResponse is a helper struct returned from the AWS API calling function to the main ListSnapshots function
type oscListSnapshotsResponse struct {
	Snapshots []osc.Snapshot
}

type Cloud interface {
	GetMetadata() MetadataService
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
	CreateVolume(ctx context.Context, localVarOptionals *osc.CreateVolumeOpts) (osc.CreateVolumeResponse, *_nethttp.Response, error)
	CreateTags(ctx context.Context, localVarOptionals *osc.CreateTagsOpts) (osc.CreateTagsResponse, *_nethttp.Response, error)
	ReadVolumes(ctx context.Context, localVarOptionals *osc.ReadVolumesOpts) (osc.ReadVolumesResponse, *_nethttp.Response, error)
	DeleteVolume(ctx context.Context, localVarOptionals *osc.DeleteVolumeOpts) (osc.DeleteVolumeResponse, *_nethttp.Response, error)
	LinkVolume(ctx context.Context, localVarOptionals *osc.LinkVolumeOpts) (osc.LinkVolumeResponse, *_nethttp.Response, error)
	UnlinkVolume(ctx context.Context, localVarOptionals *osc.UnlinkVolumeOpts) (osc.UnlinkVolumeResponse, *_nethttp.Response, error)
	CreateSnapshot(ctx context.Context, localVarOptionals *osc.CreateSnapshotOpts) (osc.CreateSnapshotResponse, *_nethttp.Response, error)
	ReadSnapshots(ctx context.Context, localVarOptionals *osc.ReadSnapshotsOpts) (osc.ReadSnapshotsResponse, *_nethttp.Response, error)
	DeleteSnapshot(ctx context.Context, localVarOptionals *osc.DeleteSnapshotOpts) (osc.DeleteSnapshotResponse, *_nethttp.Response, error)
	ReadSubregions(ctx context.Context, localVarOptionals *osc.ReadSubregionsOpts) (osc.ReadSubregionsResponse, *_nethttp.Response, error)
	ReadVms(ctx context.Context, localVarOptionals *osc.ReadVmsOpts) (osc.ReadVmsResponse, *_nethttp.Response, error)
}

type OscClient struct {
	config *osc.Configuration
	auth   context.Context
	api    *osc.APIClient
}

func (client *OscClient) CreateVolume(ctx context.Context, localVarOptionals *osc.CreateVolumeOpts) (osc.CreateVolumeResponse, *_nethttp.Response, error) {
	return client.api.VolumeApi.CreateVolume(client.auth, localVarOptionals)
}
func (client *OscClient) CreateTags(ctx context.Context, localVarOptionals *osc.CreateTagsOpts) (osc.CreateTagsResponse, *_nethttp.Response, error) {
	return client.api.TagApi.CreateTags(client.auth, localVarOptionals)
}

func (client *OscClient) ReadVolumes(ctx context.Context, localVarOptionals *osc.ReadVolumesOpts) (osc.ReadVolumesResponse, *_nethttp.Response, error){
	return client.api.VolumeApi.ReadVolumes(client.auth, localVarOptionals)
}

func(client *OscClient) DeleteVolume(ctx context.Context, localVarOptionals *osc.DeleteVolumeOpts) (osc.DeleteVolumeResponse, *_nethttp.Response, error){
    return client.api.VolumeApi.DeleteVolume(client.auth, localVarOptionals)
}

func (client *OscClient) LinkVolume(ctx context.Context, localVarOptionals *osc.LinkVolumeOpts) (osc.LinkVolumeResponse, *_nethttp.Response, error) {
    return client.api.VolumeApi.LinkVolume(client.auth, localVarOptionals)
}

func (client *OscClient) UnlinkVolume(ctx context.Context, localVarOptionals *osc.UnlinkVolumeOpts) (osc.UnlinkVolumeResponse, *_nethttp.Response, error) {
    return client.api.VolumeApi.UnlinkVolume(client.auth, localVarOptionals)
}

func (client *OscClient) CreateSnapshot(ctx context.Context, localVarOptionals *osc.CreateSnapshotOpts) (osc.CreateSnapshotResponse, *_nethttp.Response, error){
     return client.api.SnapshotApi.CreateSnapshot(client.auth, localVarOptionals)
}

func(client *OscClient) ReadSnapshots(ctx context.Context, localVarOptionals *osc.ReadSnapshotsOpts) (osc.ReadSnapshotsResponse, *_nethttp.Response, error){
    return client.api.SnapshotApi.ReadSnapshots(client.auth, localVarOptionals)
}

func (client *OscClient) DeleteSnapshot(ctx context.Context, localVarOptionals *osc.DeleteSnapshotOpts) (osc.DeleteSnapshotResponse, *_nethttp.Response, error){
    return client.api.SnapshotApi.DeleteSnapshot(client.auth, localVarOptionals)
}

func (client *OscClient) ReadSubregions(ctx context.Context, localVarOptionals *osc.ReadSubregionsOpts) (osc.ReadSubregionsResponse, *_nethttp.Response, error){
    return client.api.SubregionApi.ReadSubregions(client.auth, localVarOptionals)
}

func (client *OscClient) ReadVms(ctx context.Context, localVarOptionals *osc.ReadVmsOpts) (osc.ReadVmsResponse, *_nethttp.Response, error){
    return client.api.VmApi.ReadVms(client.auth, localVarOptionals)
}




var _ OscInterface = &OscClient{}

type cloud struct {
	region   string
	metadata MetadataService
	dm       dm.DeviceManager
	client OscInterface
}

var _ Cloud = &cloud{}

// NewCloud returns a new instance of AWS cloud
// It panics if session is invalid
func NewCloud(region string) (Cloud, error) {
	return newOscCloud(region)
}

func newOscCloud(region string) (Cloud, error) {
	svc := newEC2MetadataSvc()

	metadata, err := NewMetadataService(svc)
	if err != nil {
		return nil, fmt.Errorf("could not get metadata from AWS: %v", err)
	}
	useRegion := region
	if len(useRegion) == 0 {
		useRegion = metadata.GetRegion()
	}

	client := &OscClient{}
	client.config = osc.NewConfiguration()
	client.config.BasePath, _ = client.config.ServerUrl(0, map[string]string{"region": useRegion})
	client.api = osc.NewAPIClient(client.config)
	client.auth = context.WithValue(context.Background(), osc.ContextAWSv4, osc.AWSv4{
		AccessKey: os.Getenv("OSC_ACCESS_KEY"),
		SecretKey: os.Getenv("OSC_SECRET_KEY"),
	})

	return &cloud{
		region:   useRegion,
		metadata: metadata,
		dm:       dm.NewDeviceManager(),
		client: client,
	}, nil
}

func newEC2MetadataSvc() *ec2metadata.EC2Metadata {

	sess := session.Must(session.NewSession(&aws.Config{
		EndpointResolver: endpoints.ResolverFunc(util.OscSetupMetadataResolver()),
	}))
	return ec2metadata.New(sess)
}

func (c *cloud) GetMetadata() MetadataService {
	fmt.Printf("Debug GetMetadata\n")
	return c.metadata
}

func (c *cloud) CreateDisk(ctx context.Context, volumeName string, diskOptions *DiskOptions) (Disk, error) {
	fmt.Printf("Debug CreateDisk: %+v, %v\n", volumeName, diskOptions)
	var (
		createType string
		iops       int64
	)
	capacityGiB := util.BytesToGiB(diskOptions.CapacityBytes)

	switch diskOptions.VolumeType {
	case VolumeTypeGP2, VolumeTypeSTANDARD:
		createType = diskOptions.VolumeType
	case VolumeTypeIO1:
		createType = diskOptions.VolumeType
		iops = capacityGiB * int64(diskOptions.IOPSPerGB)
		if iops < MinTotalIOPS {
			iops = MinTotalIOPS
		}
		if iops > MaxTotalIOPS {
			iops = MaxTotalIOPS
		}
	case "":
		createType = DefaultVolumeType
	default:
		return Disk{}, fmt.Errorf("invalid AWS VolumeType %q", diskOptions.VolumeType)
	}

	var resourceTag []osc.ResourceTag
	for key, value := range diskOptions.Tags {
		copiedKey := key
		copiedValue := value
		resourceTag = append(resourceTag, osc.ResourceTag{Key: copiedKey, Value: copiedValue})
	}

	zone := diskOptions.AvailabilityZone
	if zone == "" {
		klog.V(5).Infof("AZ is not provided. Using node AZ [%s]", zone)
		var err error
		zone, err = c.randomAvailabilityZone(ctx, c.region)
		if err != nil {
			return Disk{}, fmt.Errorf("failed to get availability zone %s", err)
		}
	}

	// NOT SUPPORTED YET BY OSC API
	//if len(diskOptions.KmsKeyID) > 0 {
	//	request.KmsKeyId = aws.String(diskOptions.KmsKeyID)
	//	request.Encrypted = aws.Bool(true)
	//}

	snapshotID := diskOptions.SnapshotID
	request := osc.CreateVolumeOpts{
		CreateVolumeRequest: optional.NewInterface(
			osc.CreateVolumeRequest{
				Size:          int32(capacityGiB),
				VolumeType:    createType,
				SubregionName: zone,
				Iops: int32(iops),
				SnapshotId: snapshotID,
			}),
	}

	creation, httpRes, err := c.client.CreateVolume(ctx, &request)

	if err != nil {
		if httpRes != nil {
			return Disk{}, fmt.Errorf(httpRes.Status)
		}
		return Disk{}, fmt.Errorf("could not create volume in EC2: %v", err)
	}

	volumeID := creation.Volume.VolumeId
	if len(volumeID) == 0 {
		return Disk{}, fmt.Errorf("volume ID was not returned by CreateVolume")
	}

	size := creation.Volume.Size
	if size == 0 {
		return Disk{}, fmt.Errorf("disk size was not returned by CreateVolume")
	}

	requestTag := osc.CreateTagsOpts{
		CreateTagsRequest: optional.NewInterface(
			osc.CreateTagsRequest{
				ResourceIds: []string{creation.Volume.VolumeId},
				Tags:        resourceTag,
			}),
	}

	fmt.Printf("Debug requestTag := osc.CreateTagsOpts{ : %+v\n", requestTag)

	resTag, httpRes, err := c.client.CreateTags(ctx, &requestTag)
	if err != nil {
		return Disk{}, fmt.Errorf("error creating tags of volume %v: %v", volumeID, err)
	}

	// TO UPDATE resTag == nil
	if (resTag == osc.CreateTagsResponse{}) {
		fmt.Printf("resTag: %+v\n", resTag)
		//return Disk{}, fmt.Errorf("nil CreateTags")
	}

	 if err := c.waitForVolume(ctx, volumeID); err != nil {
		return Disk{}, fmt.Errorf("failed to get an available volume in EC2: %v", err)
	 }

	return Disk{CapacityGiB: int64(size), VolumeID: volumeID, AvailabilityZone: zone, SnapshotID: snapshotID}, nil
}

func (c *cloud) DeleteDisk(ctx context.Context, volumeID string) (bool, error) {
	fmt.Printf("Debug DeleteDisk: %+v\n", volumeID)
	request := osc.DeleteVolumeOpts{
		DeleteVolumeRequest: optional.NewInterface(
			osc.DeleteVolumeRequest{
				VolumeId: volumeID,
			}),
	}
	if _, httpRes, err := c.client.DeleteVolume(ctx, &request); err != nil {
		fmt.Errorf("Error while deleting volume ")
		if httpRes != nil {
			fmt.Errorf(httpRes.Status)
		}
		return false, fmt.Errorf("DeleteDisk could not delete volume: %v", err)
	}
	return true, nil
}

func (c *cloud) AttachDisk(ctx context.Context, volumeID, nodeID string) (string, error) {
	fmt.Printf("Debug AttachDisk: %+v, %v\n", volumeID, nodeID)
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
		request := osc.LinkVolumeOpts{
			LinkVolumeRequest: optional.NewInterface(
				osc.LinkVolumeRequest{
					DeviceName: device.Path,
					VmId:       nodeID,
					VolumeId:   volumeID,
				}),
		}

		resp, httpRes, err := c.client.LinkVolume(ctx, &request)
		if err != nil {
			if httpRes != nil {
				return "", fmt.Errorf(httpRes.Status)
			}
			return "", fmt.Errorf("could not attach volume %q to node %q: %v", volumeID, nodeID, err)
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
		request := osc.ReadVolumesOpts{
			ReadVolumesRequest: optional.NewInterface(
				osc.ReadVolumesRequest{
					Filters: osc.FiltersVolume{
						VolumeIds: []string{volumeID},
					},
				}),
		}

		volume, err := c.getVolume(ctx, &request)
		klog.Infof("Check Volume state before detaching volume: %+v err: %+v",
			volume, err)
		if err == nil && reflect.DeepEqual(volume, osc.Volume{}) {
			if volume.State != "" && volume.State == "available" {
				klog.Warningf("Tolerate DetachDisk called on available volume: %s on %s",
					volumeID, nodeID)
				return nil
			}
		}
	}
	klog.Infof("Debug Continue DetachDisk: %+v, %v\n", volumeID, nodeID)
	instance, err := c.getInstance(ctx, nodeID)
	if err != nil {
		return err
	}

	// TODO: check if attached
	device, err := c.dm.GetDevice(instance, volumeID)
	if err != nil {
		return err
	}
	defer device.Release(true)

	if !device.IsAlreadyAssigned {
		klog.Warningf("DetachDisk called on non-attached volume: %s", volumeID)
	}

	request := osc.UnlinkVolumeOpts{
		UnlinkVolumeRequest: optional.NewInterface(
			osc.UnlinkVolumeRequest{
				VolumeId: volumeID,
			}),
	}

	var httpRes *_nethttp.Response
	_, httpRes, err = c.client.UnlinkVolume(ctx, &request)
	if err != nil {
		if httpRes != nil {
			fmt.Errorf(httpRes.Status)
		}
		return fmt.Errorf("could not detach volume %q from node %q: %v", volumeID, nodeID, err)
	}

	if err := c.WaitForAttachmentState(ctx, volumeID, "detached"); err != nil {
		return err
	}

	return nil
}

// WaitForAttachmentState polls until the attachment status is the expected value.
func (c *cloud) WaitForAttachmentState(ctx context.Context, volumeID, state string) error {
	fmt.Printf("Debug WaitForAttachmentState: %+v, %v\n", volumeID, state)
	// Most attach/detach operations on AWS finish within 1-4 seconds.
	// By using 1 second starting interval with a backoff of 1.8,
	// we get [1, 1.8, 3.24, 5.832000000000001, 10.4976].
	// In total we wait for 2601 seconds.
	verifyVolumeFunc := func() (bool, error) {
		request := osc.ReadVolumesOpts{
			ReadVolumesRequest: optional.NewInterface(
				osc.ReadVolumesRequest{
					Filters: osc.FiltersVolume{
						VolumeIds: []string{volumeID},
					},
				}),
		}

		volume, err := c.getVolume(ctx, &request)
		if err != nil {
			return false, err
		}

		if len(volume.LinkedVolumes) == 0 {
			if state == "detached" {
				return true, nil
			}
		}

		for _, a := range volume.LinkedVolumes {
			if a.State == "" {
				klog.Warningf("Ignoring nil attachment state for volume %q: %v", volumeID, a)
				continue
			}
			if a.State == state {
				return true, nil
			}
		}
		return false, nil
	}

	backoff := util.EnvBackoff()
	return wait.ExponentialBackoff(backoff, verifyVolumeFunc)
}

func (c *cloud) GetDiskByName(ctx context.Context, name string, capacityBytes int64) (Disk, error) {
	request := osc.ReadVolumesOpts{
		ReadVolumesRequest: optional.NewInterface(
			osc.ReadVolumesRequest{
				Filters: osc.FiltersVolume{
					TagKeys:   []string{"tag:" + VolumeNameTagKey},
					TagValues: []string{name},
				},
			}),
	}

	volume, err := c.getVolume(ctx, &request)
	if err != nil {
		return Disk{}, err
	}

	volSizeBytes := volume.Size
	if int64(volSizeBytes) != util.BytesToGiB(capacityBytes) {
		return Disk{}, ErrDiskExistsDiffSize
	}

	return Disk{
		VolumeID:         volume.VolumeId,
		CapacityGiB:      int64(volSizeBytes),
		AvailabilityZone: volume.SubregionName,
	}, nil
}

func (c *cloud) GetDiskByID(ctx context.Context, volumeID string) (Disk, error) {
	fmt.Printf("Debug GetDiskByID : %+v\n", volumeID)

	request := osc.ReadVolumesOpts{
		ReadVolumesRequest: optional.NewInterface(
			osc.ReadVolumesRequest{
				Filters: osc.FiltersVolume{
					VolumeIds: []string{volumeID},
				},
			}),
	}

	volume, err := c.getVolume(ctx, &request)
	if err != nil {
		return Disk{}, err
	}

	return Disk{
		VolumeID:         volume.VolumeId,
		CapacityGiB:      int64(volume.Size),
		AvailabilityZone: volume.SubregionName,
	}, nil
}

func (c *cloud) IsExistInstance(ctx context.Context, nodeID string) bool {
	fmt.Printf("Debug IsExistInstance : %+v\n", nodeID)
	instance, err := c.getInstance(ctx, nodeID)
	if err != nil || reflect.DeepEqual(instance, osc.Vm{}) {
		return false
	}
	return true
}

func (c *cloud) CreateSnapshot(ctx context.Context, volumeID string, snapshotOptions *SnapshotOptions) (snapshot Snapshot, err error) {
	descriptions := "Created by AWS EBS CSI driver for volume " + volumeID
	fmt.Printf("Debug CreateSnapshot : %+v, %+v\n", volumeID, snapshotOptions)

	var resourceTag []osc.ResourceTag
	for key, value := range snapshotOptions.Tags {
		resourceTag = append(resourceTag, osc.ResourceTag{Key: key, Value: value})
	}
	fmt.Printf("Debug tags = append( : %+v  \n", resourceTag)

	request := osc.CreateSnapshotOpts{
		CreateSnapshotRequest: optional.NewInterface(
			osc.CreateSnapshotRequest{
				VolumeId:    volumeID,
				DryRun:      false,
				Description: descriptions,
			}),
	}

	fmt.Printf("Debug request := &ec2.CreateSnapshotInput{: %+v  \n", request)
	var res osc.CreateSnapshotResponse
	var httpRes *_nethttp.Response
	createSnapshotCallBack := func() (bool, error) {
		res, httpRes, err = c.client.CreateSnapshot(ctx, &request)
		if err != nil {
			if httpRes != nil {
				fmt.Errorf(httpRes.Status)
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

	err = waitErr
	if err != nil {
		return Snapshot{}, fmt.Errorf("error creating snapshot of volume %s: %v", volumeID, err)
	}
	if reflect.DeepEqual(res, osc.CreateSnapshotResponse{}) {
		return Snapshot{}, fmt.Errorf("nil CreateSnapshotResponse")
	}
	fmt.Printf("Debug res, err := c.ec2.CreateSnapshotWithContext(ctx, request) : %+v\n", res)

	requestTag := osc.CreateTagsOpts{
		CreateTagsRequest: optional.NewInterface(
			osc.CreateTagsRequest{
				ResourceIds: []string{res.Snapshot.SnapshotId},
				Tags:        resourceTag,
			}),
	}

	fmt.Printf("Debug requestTag := &ec2.CreateTagsInput{ : %+v\n", requestTag)
	var resTag osc.CreateTagsResponse
	var httpResTag *_nethttp.Response
	var errTag error
	createTagCallback := func() (bool, error) {
		resTag, httpResTag, errTag = c.client.CreateTags(ctx, &requestTag)
		if errTag != nil {
			if httpResTag != nil {
				fmt.Errorf(httpRes.Status)
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

	err = waitErr
	if err != nil {
		return Snapshot{}, fmt.Errorf("error creating tags of snapshot %v: %v", res.Snapshot.SnapshotId, err)
	}
	if (resTag == osc.CreateTagsResponse{}) {
		fmt.Printf("resTag: %+v\n", resTag)
		//return Snapshot{}, fmt.Errorf("nil CreateTags")
	}
	fmt.Printf("Debug resTag, err := c.ec2.CreateTagsWithContext(ctx, requestTag) %+v\n", resTag)
	return c.oscSnapshotResponseToStruct(res.Snapshot), nil
}


func (c *cloud) DeleteSnapshot(ctx context.Context, snapshotID string) (success bool, err error) {
	fmt.Printf("Debug DeleteSnapshot : %+v\n", snapshotID)
	request := osc.DeleteSnapshotOpts{
		DeleteSnapshotRequest: optional.NewInterface(
			osc.DeleteSnapshotRequest{
				SnapshotId: snapshotID,
				DryRun:     false,
			}),
	}

	var httpRes *_nethttp.Response

	_, httpRes, err = c.client.DeleteSnapshot(ctx, &request)
	if err != nil {
		if httpRes != nil {
			fmt.Errorf(httpRes.Status)
		}
		return false, fmt.Errorf("DeleteSnapshot could not delete volume: %v", err)
	}
	return true, nil
}

func (c *cloud) GetSnapshotByName(ctx context.Context, name string) (snapshot Snapshot, err error) {
	fmt.Printf("Debug GetSnapshotByName : %+v\n", name)

	request := osc.ReadSnapshotsOpts{
		ReadSnapshotsRequest: optional.NewInterface(
			osc.ReadSnapshotsRequest{
				Filters: osc.FiltersSnapshot{
					TagKeys:   []string{"tag:" + SnapshotNameTagKey},
					TagValues: []string{name},
				},
			}),
	}

	oscsnapshot, err := c.getSnapshot(ctx, &request)
	if err != nil {
		return Snapshot{}, err
	}

	return c.oscSnapshotResponseToStruct(oscsnapshot), nil
}

func (c *cloud) GetSnapshotByID(ctx context.Context, snapshotID string) (snapshot Snapshot, err error) {
	fmt.Printf("Debug GetSnapshotByID : %+v\n", snapshotID)

	request := osc.ReadSnapshotsOpts{
		ReadSnapshotsRequest: optional.NewInterface(
			osc.ReadSnapshotsRequest{
				Filters: osc.FiltersSnapshot{
					SnapshotIds: []string{snapshotID},
				},
			}),
	}

	oscsnapshot, err := c.getSnapshot(ctx, &request)
	if err != nil {
		return Snapshot{}, err
	}

	return c.oscSnapshotResponseToStruct(oscsnapshot), nil
}

// ListSnapshots retrieves AWS EBS snapshots for an optionally specified volume ID.  If maxResults is set, it will return up to maxResults snapshots.  If there are more snapshots than maxResults,
// a next token value will be returned to the client as well.  They can use this token with subsequent calls to retrieve the next page of results.  If maxResults is not set (0),
// there will be no restriction up to 1000 results (https://docs.aws.amazon.com/sdk-for-go/api/service/ec2/#DescribeSnapshotsInput).
// Pagination not supported
func (c *cloud) ListSnapshots(ctx context.Context, volumeID string, maxResults int64, nextToken string) (listSnapshotsResponse ListSnapshotsResponse, err error) {
	fmt.Printf("Debug ListSnapshots : %+v, %+v, %+v\n", volumeID, maxResults, nextToken)

	request := osc.ReadSnapshotsOpts{
		ReadSnapshotsRequest: optional.NewInterface(
			osc.ReadSnapshotsRequest{
				Filters: osc.FiltersSnapshot{
					VolumeIds: []string{},
				},
			}),
	}

	if len(volumeID) != 0 {
		request = osc.ReadSnapshotsOpts{
			ReadSnapshotsRequest: optional.NewInterface(
				osc.ReadSnapshotsRequest{
					Filters: osc.FiltersSnapshot{
						VolumeIds: []string{volumeID},
					},
				}),
		}
	}

	oscSnapshotsResponse, err := c.listSnapshots(ctx, &request)
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
	fmt.Printf("Debug oscSnapshotResponseToStruct : %+v\n", oscSnapshot)
	if reflect.DeepEqual(oscSnapshot, osc.Snapshot{}) {
		return Snapshot{}
	}
	snapshotSize := util.GiBToBytes(int64(oscSnapshot.VolumeSize))
	snapshot := Snapshot{
		SnapshotID:     oscSnapshot.SnapshotId,
		SourceVolumeID: oscSnapshot.VolumeId,
		Size:           snapshotSize,
		//No StartTime for osc.Snapshot
		//CreationTime:   oscSnapshot.StartTime,
	}
	if oscSnapshot.State == "completed" {
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
func (c *cloud) getVolume(ctx context.Context, request *osc.ReadVolumesOpts) (osc.Volume, error) {
	fmt.Printf("Debug getVolume : %+v\n", request)
	var volume osc.Volume
	getVolumeCallback := func() (bool, error) {
		var volumes []osc.Volume

		var response osc.ReadVolumesResponse
		var httpRes *_nethttp.Response
		var err error



		response, httpRes, err = c.client.ReadVolumes(ctx, request)
		if err != nil {
			if httpRes != nil {
				fmt.Errorf(httpRes.Status)
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
		volumes = append(volumes, response.Volumes...)



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
		return osc.Volume{}, waitErr
	}

	return volume, nil
}

// Pagination not supported
func (c *cloud) getInstance(ctx context.Context, vmID string) (osc.Vm, error) {
	fmt.Printf("Debug  getInstance : %+v\n", vmID)
	var instances []osc.Vm

	request := osc.ReadVmsOpts{
		ReadVmsRequest: optional.NewInterface(
			osc.ReadVmsRequest{
				Filters: osc.FiltersVm{
					VmIds: []string{vmID},
				},
			}),
	}

	getInstanceCallback := func() (bool, error) {
		response, httpRes, err := c.client.ReadVms(ctx, &request)
		if err != nil {
			if httpRes != nil {
				fmt.Errorf(httpRes.Status)
			}
			requestStr := fmt.Sprintf("%v", request)
			if keepRetryWithError(
				requestStr,
				err,
				[]string{"RequestLimitExceeded"}) {
				return false, nil
			}
			return false, fmt.Errorf("error listing AWS instances: %q", err)
		}

			instances = append(instances, response.Vms...)


		return true, nil
	}

	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, getInstanceCallback)
	if waitErr != nil {
		return osc.Vm{}, waitErr
	}

	if l := len(instances); l > 1 {
		return osc.Vm{}, fmt.Errorf("found %d instances with ID %q", l, vmID)
	} else if l < 1 {
		return osc.Vm{}, ErrNotFound
	}

	return instances[0], nil
}

// Pagination not supported
func (c *cloud) getSnapshot(ctx context.Context, request *osc.ReadSnapshotsOpts) (osc.Snapshot, error) {
	fmt.Printf("Debug  getSnapshot(ctx context.Context : %+v\n", request)
	var snapshots []osc.Snapshot

	getSnapshotsCallback := func() (bool, error) {
		response, httpRes, err := c.client.ReadSnapshots(ctx, request)
		if err != nil {
			if httpRes != nil {
				fmt.Errorf(httpRes.Status)
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
			snapshots = append(snapshots, response.Snapshots...)

		return true, nil
	}

	backoff := util.EnvBackoff()
	waitErr := wait.ExponentialBackoff(backoff, getSnapshotsCallback)
	if waitErr != nil {
		return osc.Snapshot{}, waitErr
	}

	fmt.Printf("Debug  len(snapshots): %+v\n", len(snapshots))
	fmt.Printf("Debug  (snapshots): %+v\n", &snapshots)

	if l := len(snapshots); l > 1 {
		return osc.Snapshot{}, ErrMultiSnapshots
	} else if l < 1 {
		return osc.Snapshot{}, ErrNotFound
	}
	fmt.Printf("Debug  (snapshots[0]): %+v\n", snapshots[0])
	return snapshots[0], nil
}

// listSnapshots returns all snapshots based from a request
// Pagination not supported
func (c *cloud) listSnapshots(ctx context.Context, request *osc.ReadSnapshotsOpts) (oscListSnapshotsResponse, error) {
	fmt.Printf("Debug listSnapshots : %+v\n", request)

	var snapshots []osc.Snapshot
	fmt.Printf("Debug  listSnapshots(ctx context.Context : %+v\n", request)

	var response osc.ReadSnapshotsResponse
	var httpRes *_nethttp.Response
	var err error
	listSnapshotsCallBack := func() (bool, error) {
		response, httpRes, err = c.client.ReadSnapshots(ctx, request)
		if err != nil {
			if httpRes != nil {
				fmt.Errorf(httpRes.Status)
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

	fmt.Printf("Debug  response.Snapshots : %+v\n", response.Snapshots)
	snapshots = append(snapshots, response.Snapshots...)

	return oscListSnapshotsResponse{
		Snapshots: snapshots,
	}, nil
}

// waitForVolume waits for volume to be in the "available" state.
// On a random AWS account (shared among several developers) it took 4s on average.
func (c *cloud) waitForVolume(ctx context.Context, volumeID string) error {
	fmt.Printf("Debug waitForVolume : %+v\n", volumeID)
	var (
		checkInterval = 3 * time.Second
		// This timeout can be "ovewritten" if the value returned by ctx.Deadline()
		// comes sooner. That value comes from the external provisioner controller.
		checkTimeout = 1 * time.Minute
	)

	request := osc.ReadVolumesOpts{
		ReadVolumesRequest: optional.NewInterface(
			osc.ReadVolumesRequest{
				Filters: osc.FiltersVolume{
					VolumeIds: []string{volumeID},
				},
			}),
	}

	err := wait.Poll(checkInterval, checkTimeout, func() (done bool, err error) {
		vol, err := c.getVolume(ctx, &request)
		if err != nil {
			return true, err
		}
		if &vol.State != nil {
			return vol.State == "available", nil
		}
		return false, nil
	})

	return err
}

//MODIFICATION NOT SUPPORTED
// ResizeDisk resizes an EBS volume in GiB increments, rouding up to the next possible allocatable unit.
// It returns the volume size after this call or an error if the size couldn't be determined.
func (c *cloud) ResizeDisk(ctx context.Context, volumeID string, newSizeBytes int64) (int64, error) {
     return 0, nil
// 	fmt.Printf("Debug ResizeDisk : %+v\n", volumeID)
// 	request := &osc.ReadVolumesOpts{
// 		ReadVolumesRequest: optional.NewInterface(
// 			osc.ReadVolumesRequest{
// 				Filters: osc.FiltersVolume{
// 					VolumeIds: []*string{volumeID},
// 				},
// 			}),
// 	}
//
// 	volume, err := c.getVolume(ctx, request)
// 	if err != nil {
// 		return 0, err
// 	}
//
// 	// AWS resizes in chunks of GiB (not GB)
// 	newSizeGiB := util.RoundUpGiB(newSizeBytes)
// 	oldSizeGiB := aws.Int64Value(volume.Size)
//
// 	if oldSizeGiB >= newSizeGiB {
// 		klog.V(5).Infof("Volume %q's current size (%d GiB) is greater or equal to the new size (%d GiB)", volumeID, oldSizeGiB, newSizeGiB)
// 		return oldSizeGiB, nil
// 	}
//
// 	req := &ec2.ModifyVolumeInput{
// 		VolumeId: aws.String(volumeID),
// 		Size:     aws.Int64(newSizeGiB),
// 	}
//
// 	var mod *ec2.VolumeModification
// 	response, err := c.ec2.ModifyVolumeWithContext(ctx, req)
// 	if err != nil {
// 		if !isAWSErrorIncorrectModification(err) {
// 			return 0, fmt.Errorf("could not modify AWS volume %q: %v", volumeID, err)
// 		}
//
// 		m, err := c.getLatestVolumeModification(ctx, volumeID)
// 		if err != nil {
// 			return 0, err
// 		}
// 		mod = m
// 	}
//
// 	if mod == nil {
// 		mod = response.VolumeModification
// 	}
//
// 	state := aws.StringValue(mod.ModificationState)
// 	if state == ec2.VolumeModificationStateCompleted || state == ec2.VolumeModificationStateOptimizing {
// 		return aws.Int64Value(mod.TargetSize), nil
// 	}
//
// 	return c.waitForVolumeSize(ctx, volumeID)
}

// waitForVolumeSize waits for a volume modification to finish and return its size.
func (c *cloud) waitForVolumeSize(ctx context.Context, volumeID string) (int64, error) {
     return 0, nil
// 	fmt.Printf("Debug waitForVolumeSize : %+v\n", volumeID)
// 	var modVolSizeGiB int64
// 	backoff := util.EnvBackoff()
// 	waitErr := wait.ExponentialBackoff(backoff, func() (bool, error) {
// 		m, err := c.getLatestVolumeModification(ctx, volumeID)
// 		if err != nil {
// 			return false, err
// 		}
//
// 		state := aws.StringValue(m.ModificationState)
// 		if state == ec2.VolumeModificationStateCompleted || state == ec2.VolumeModificationStateOptimizing {
// 			modVolSizeGiB = aws.Int64Value(m.TargetSize)
// 			return true, nil
// 		}
//
// 		return false, nil
// 	})
//
// 	if waitErr != nil {
// 		return 0, waitErr
// 	}
//
// 	return modVolSizeGiB, nil
}

// getLatestVolumeModification returns the last modification of the volume.
func (c *cloud) getLatestVolumeModification(ctx context.Context, volumeID string) (int64 /**ec2.VolumeModification*/, error) {
    return 0, nil
// 	fmt.Printf("Debug getLatestVolumeModification : %+v\n", volumeID)
// 	request := &ec2.DescribeVolumesModificationsInput{
// 		VolumeIds: []*string{
// 			aws.String(volumeID),
// 		},
// 	}
// 	var err error
// 	var mod *ec2.DescribeVolumesModificationsOutput
// 	describeVolModCallback := func() (bool, error) {
// 		mod, err = c.ec2.DescribeVolumesModificationsWithContext(ctx, request)
// 		if err != nil {
// 			requestStr := fmt.Sprintf("%v", request)
// 			if keepRetryWithError(
// 				requestStr,
// 				err,
// 				[]string{"RequestLimitExceeded"}) {
// 				return false, nil
// 			}
// 			return false, fmt.Errorf("error describing modifications in volume %q: %v", volumeID, err)
// 		}
// 		return true, nil
// 	}
//
// 	backoff := util.EnvBackoff()
// 	waitErr := wait.ExponentialBackoff(backoff, describeVolModCallback)
//
// 	if waitErr != nil {
// 		return nil, waitErr
// 	}
//
// 	volumeMods := mod.VolumesModifications
// 	if len(volumeMods) == 0 {
// 		return nil, fmt.Errorf("could not find any modifications for volume %q", volumeID)
// 	}
//
// 	return volumeMods[len(volumeMods)-1], nil
}

// randomAvailabilityZone returns a random zone from the given region
// the randomness relies on the response of DescribeAvailabilityZones
func (c *cloud) randomAvailabilityZone(ctx context.Context, region string) (string, error) {
	fmt.Printf("Debug randomAvailabilityZone: %+v\n", region)
	zone := c.metadata.GetAvailabilityZone()
	if zone != "" {
		return zone, nil
	}

	response, httpRes, err := c.client.ReadSubregions(ctx, nil)

	if err != nil {
		if httpRes != nil {
			fmt.Errorf(httpRes.Status)
		}
		return "", err
	}

	zones := []string{}
	for _, zone := range response.Subregions {
		zones = append(zones, zone.SubregionName)
	}

	return zones[0], nil
}