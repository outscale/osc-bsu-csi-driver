package driver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	sanity "github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud"
	"github.com/outscale/osc-bsu-csi-driver/pkg/driver/internal"
	"github.com/outscale/osc-bsu-csi-driver/pkg/driver/luks"
	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	"k8s.io/utils/exec"
	exectesting "k8s.io/utils/exec/testing"
	"k8s.io/utils/mount"
)

func TestSanity(t *testing.T) {
	dir := t.TempDir()

	targetPath := filepath.Join(dir, "target")
	stagingPath := filepath.Join(dir, "staging")
	endpoint := "unix://" + filepath.Join(dir, "csi.sock")

	config := sanity.NewTestConfig()
	config.Address = endpoint
	config.TargetPath = targetPath
	config.StagingPath = stagingPath
	config.CreateTargetDir = createDir
	config.CreateStagingDir = createDir
	config.CheckPath = checkPath
	config.IdempotentCount = 2

	driverOptions := &DriverOptions{
		endpoint: endpoint,
		mode:     AllMode,
	}

	drv := &Driver{
		options: driverOptions,
		controllerService: controllerService{
			cloud:         newFakeCloudProvider(),
			driverOptions: driverOptions,
		},
		nodeService: nodeService{
			metadata: &cloud.Metadata{
				InstanceID:       "instanceID",
				Region:           "region",
				AvailabilityZone: "az",
			},
			maxVolumes: 39,
			mounter:    newFakeMounter(),
			inFlight:   internal.NewInFlight(),
		},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("recover: %v", r)
		}
	}()
	go func() {
		if err := drv.Run(); err != nil {
			panic(fmt.Sprintf("%v", err))
		}
	}()

	// Now call the test suite
	sanity.Test(t, config)
}

func createDir(targetPath string) (string, error) {
	if err := os.MkdirAll(targetPath, 0700); err != nil {
		if os.IsNotExist(err) {
			return "", err
		}
	}
	return targetPath, nil
}

func checkPath(targetPath string) (sanity.PathKind, error) {
	stat, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return sanity.PathIsNotFound, nil
		}
		return sanity.PathIsNotFound, err
	}

	if stat.IsDir() {
		return sanity.PathIsDir, nil
	}

	return sanity.PathIsFile, nil
}

type fakeCloudProvider struct {
	id atomic.Uint64

	disks map[string]*fakeDisk
	// snapshots contains mapping from snapshot ID to snapshot
	snapshots map[string]*fakeSnapshot
	pub       map[string]string
	tokens    map[string]int32
	m         *cloud.Metadata
}

type fakeDisk struct {
	cloud.Disk
	tags map[string]string
}

type fakeSnapshot struct {
	cloud.Snapshot
	tags map[string]string
}

func newFakeCloudProvider() *fakeCloudProvider {
	return &fakeCloudProvider{
		disks:     make(map[string]*fakeDisk),
		snapshots: make(map[string]*fakeSnapshot),
		pub:       make(map[string]string),
		tokens:    make(map[string]int32),
		m: &cloud.Metadata{
			InstanceID:       "instanceID",
			Region:           "region",
			AvailabilityZone: "az",
		},
	}
}

func (c *fakeCloudProvider) CreateDisk(ctx context.Context, volumeName string, diskOptions *cloud.DiskOptions) (cloud.Disk, error) {
	if len(diskOptions.SnapshotID) > 0 {
		if _, ok := c.snapshots[diskOptions.SnapshotID]; !ok {
			return cloud.Disk{}, cloud.ErrNotFound
		}
	}
	id := c.id.Add(1)
	d := &fakeDisk{
		Disk: cloud.Disk{
			VolumeID:         fmt.Sprintf("vol-%06d", id),
			CapacityGiB:      util.BytesToGiB(diskOptions.CapacityBytes),
			AvailabilityZone: diskOptions.AvailabilityZone,
			SnapshotID:       diskOptions.SnapshotID,
		},
		tags: diskOptions.Tags,
	}
	c.disks[d.VolumeID] = d
	return d.Disk, nil
}

func (c *fakeCloudProvider) DeleteDisk(ctx context.Context, volumeID string) (bool, error) {
	delete(c.disks, volumeID)
	return true, nil
}

func (c *fakeCloudProvider) AttachDisk(ctx context.Context, volumeID, nodeID string) (string, error) {
	c.pub[volumeID] = nodeID
	return "/tmp", nil
}

func (c *fakeCloudProvider) DetachDisk(ctx context.Context, volumeID, nodeID string) error {
	delete(c.pub, volumeID)
	return nil
}

func (c *fakeCloudProvider) WaitForAttachmentState(ctx context.Context, volumeID, state string) error {
	return nil
}

func (c *fakeCloudProvider) GetDiskByName(ctx context.Context, name string, capacityBytes int64) (cloud.Disk, error) {
	var disks []*fakeDisk
	for _, d := range c.disks {
		for key, value := range d.tags {
			if key == cloud.VolumeNameTagKey && value == name {
				disks = append(disks, d)
			}
		}
	}
	if len(disks) > 1 {
		return cloud.Disk{}, cloud.ErrMultiDisks
	} else if len(disks) == 1 {
		if capacityBytes != int64(disks[0].Disk.CapacityGiB)*util.GiB {
			return cloud.Disk{}, cloud.ErrDiskExistsDiffSize
		}
		return disks[0].Disk, nil
	}
	return cloud.Disk{}, nil
}

func (c *fakeCloudProvider) GetDiskByID(ctx context.Context, volumeID string) (cloud.Disk, error) {
	if d, found := c.disks[volumeID]; found {
		return d.Disk, nil
	}
	return cloud.Disk{}, cloud.ErrNotFound
}

func (c *fakeCloudProvider) IsExistInstance(ctx context.Context, nodeID string) bool {
	return nodeID == "instanceID"
}

func (c *fakeCloudProvider) CreateSnapshot(ctx context.Context, volumeID string, snapshotOptions *cloud.SnapshotOptions) (snapshot cloud.Snapshot, err error) {
	id := c.id.Add(1)
	snapshotID := fmt.Sprintf("snapshot-%06d", id)

	for _, existingSnapshot := range c.snapshots {
		if existingSnapshot.Snapshot.SnapshotID == snapshotID && existingSnapshot.Snapshot.SourceVolumeID == volumeID {
			return cloud.Snapshot{}, cloud.ErrAlreadyExists
		}
	}

	s := &fakeSnapshot{
		Snapshot: cloud.Snapshot{
			SnapshotID:     snapshotID,
			SourceVolumeID: volumeID,
			Size:           1,
			CreationTime:   time.Now(),
			State:          "completed",
		},
		tags: snapshotOptions.Tags,
	}
	c.snapshots[snapshotID] = s
	return s.Snapshot, nil
}

func (c *fakeCloudProvider) DeleteSnapshot(ctx context.Context, snapshotID string) (success bool, err error) {
	delete(c.snapshots, snapshotID)
	return true, nil
}

func (c *fakeCloudProvider) GetSnapshotByName(ctx context.Context, name string) (snapshot cloud.Snapshot, err error) {
	var snapshots []*fakeSnapshot
	for _, s := range c.snapshots {
		if s.tags[cloud.SnapshotNameTagKey] == name {
			snapshots = append(snapshots, s)
		}
	}
	if len(snapshots) == 0 {
		return cloud.Snapshot{}, cloud.ErrNotFound
	}

	return snapshots[0].Snapshot, nil
}

func (c *fakeCloudProvider) GetSnapshotByID(ctx context.Context, snapshotID string) (snapshot cloud.Snapshot, err error) {
	ret, exists := c.snapshots[snapshotID]
	if !exists {
		return cloud.Snapshot{}, cloud.ErrNotFound
	}

	return ret.Snapshot, nil
}

func (c *fakeCloudProvider) ListSnapshots(ctx context.Context, volumeID string, maxResults int32, nextToken string) (listSnapshotsResponse cloud.ListSnapshotsResponse, err error) {
	var snapshots []cloud.Snapshot
	var retToken string
	for _, fakeSnapshot := range c.snapshots {
		if fakeSnapshot.Snapshot.SourceVolumeID == volumeID || len(volumeID) == 0 {
			snapshots = append(snapshots, fakeSnapshot.Snapshot)
		}
	}
	var offset int32
	if nextToken != "" {
		offset = c.tokens[nextToken]
		snapshots = snapshots[offset:]
	}
	if maxResults > 0 {
		id := c.id.Add(1)
		retToken = fmt.Sprintf("token-%06d", id)
		c.tokens[retToken] = offset + maxResults
		snapshots = snapshots[0:maxResults]
		fmt.Printf("%v\n", snapshots)
	}
	return cloud.ListSnapshotsResponse{
		Snapshots: snapshots,
		NextToken: retToken,
	}, nil
}

func (c *fakeCloudProvider) ResizeDisk(ctx context.Context, volumeID string, newSize int64) (int64, error) {
	if d, found := c.disks[volumeID]; found {
		d.CapacityGiB = util.RoundUpGiB(newSize)
		return int64(util.RoundUpGiB(newSize)) * util.GiB, nil
	}
	return 0, cloud.ErrNotFound
}

func (c *fakeCloudProvider) UpdateDisk(ctx context.Context, volumeID string, volumeType string, iops int32) error {
	if _, found := c.disks[volumeID]; !found {
		return cloud.ErrNotFound
	}
	return nil
}

// GetMetadata mocks base method
func (c *fakeCloudProvider) GetMetadata() cloud.MetadataService {
	return c.m
}

type fakeMounter struct {
	exec.Interface
	mount.SafeFormatAndMount
}

func newFakeMounter() *fakeMounter {
	localMounter := &mount.FakeMounter{MountPoints: []mount.MountPoint{}}
	localExec := &exectesting.FakeExec{DisableScripts: true}
	return &fakeMounter{
		&exectesting.FakeExec{DisableScripts: true},
		mount.SafeFormatAndMount{Interface: localMounter, Exec: localExec},
	}
}

func (f *fakeMounter) FormatAndMount(source string, target string, fstype string, options []string) error {
	return nil
}

func (f *fakeMounter) GetDeviceName(mountPath string) (string, int, error) {
	return mount.GetDeviceNameFromMount(f.SafeFormatAndMount.Interface, mountPath)
}

func (f *fakeMounter) MakeFile(pathname string) error {
	file, err := os.OpenFile(pathname, os.O_CREATE, os.FileMode(0644))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	if err = file.Close(); err != nil {
		return err
	}
	return nil
}

func (f *fakeMounter) MakeDir(pathname string) error {
	err := os.MkdirAll(pathname, os.FileMode(0755))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func (f *fakeMounter) ExistsPath(filename string) (bool, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func (f *fakeMounter) GetDiskFormat(disk string) (string, error) {
	return "", nil
}

func (f *fakeMounter) MountSensitive(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return nil
}

func (f *fakeMounter) IsCorruptedMnt(err error) bool {
	return false
}

func (m *fakeMounter) IsLuks(devicePath string) bool {
	return false
}

func (m *fakeMounter) LuksFormat(devicePath, passphrase string, context luks.LuksContext) error {
	return nil
}

func (m *fakeMounter) CheckLuksPassphrase(devicePath, passphrase string) error {
	return nil
}

func (m *fakeMounter) LuksOpen(devicePath, encryptedDeviceName, passphrase string, luksOpenFlags ...string) (bool, error) {
	return true, nil
}

func (m *fakeMounter) IsLuksMapping(devicePath string) (bool, string, error) {
	return false, "", nil
}

func (m *fakeMounter) LuksResize(deviceName string, passphrase string) error {
	return nil
}

func (m *fakeMounter) LuksClose(deviceName string) error {
	return nil
}
