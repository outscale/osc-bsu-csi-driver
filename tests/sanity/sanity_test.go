package driver

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sanity "github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	"github.com/outscale/osc-bsu-csi-driver/pkg/driver"
	"github.com/rs/xid"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	testingexec "k8s.io/utils/exec/testing"
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
	config.IDGen = idGenerator{}

	logOptions := logs.NewOptions()
	logOptions.Verbosity = 5
	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		t.Fatalf("unable to setup logger: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	drv, err := driver.NewDriver(ctx,
		driver.WithExtraSnapshotTags(map[string]string{"csi-sanity-test": "true"}),
		driver.WithExtraVolumeTags(map[string]string{"csi-sanity-test": "true"}),
		driver.WithMode(driver.AllMode),
		driver.WithEndpoint(endpoint),
		driver.WithMounter(newFakeMounter()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("recover: %v", r)
		}
	}()
	go func() {
		if err := drv.Run(ctx); err != nil {
			panic(err)
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

func newFakeMounter() driver.Mounter {
	return &fakeNodeMounter{
		NodeMounter: &driver.NodeMounter{
			SafeFormatAndMount: mount.SafeFormatAndMount{
				Interface: mount.NewFakeMounter(nil),
				Exec:      &testingexec.FakeExec{DisableScripts: true},
			},
			Interface: &testingexec.FakeExec{DisableScripts: true},
		},
	}
}

type fakeNodeMounter struct {
	*driver.NodeMounter
}

func (m *fakeNodeMounter) ExistsPath(path string) (bool, error) {
	if strings.HasPrefix(path, "/dev/xvd") {
		return true, nil
	}
	return m.NodeMounter.ExistsPath(path)
}

type idGenerator struct{}

func (idGenerator) GenerateUniqueValidVolumeID() string { return "vol-" + newID() }

func (idGenerator) GenerateInvalidVolumeID() string { return "invalid-vol" }

func (idGenerator) GenerateUniqueValidSnapshotID() string { return "snap-" + newID() }

func (idGenerator) GenerateInvalidSnapshotID() string { return "invalid-snap" }

func (idGenerator) GenerateUniqueValidNodeID() string { return "i-" + newID() }

func (idGenerator) GenerateInvalidNodeID() string { return "invalid-i" }

func newID() string {
	i := xid.New()
	return hex.EncodeToString(i[:])[:8]
}

var _ sanity.IDGenerator = idGenerator{}
