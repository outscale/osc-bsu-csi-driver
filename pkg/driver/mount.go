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
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
	"strconv"

	"golang.org/x/sys/unix"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

type volumeStats struct {
	availBytes, totalBytes, usedBytes    int64
	availInodes, totalInodes, usedInodes int64
}

// Mounter is an interface for mount operations
type Mounter interface {
	mount.Interface
	exec.Interface
	FormatAndMount(source string, target string, fstype string, options []string) error
	GetDeviceName(mountPath string) (string, int, error)
	MakeFile(pathname string) error
	MakeDir(pathname string) error
	ExistsPath(filename string) (bool, error)
	GetStatistics(filename string) (volumeStats, error)
	IsBlockDevice(filename string) (bool, error)
}

type NodeMounter struct {
	mount.SafeFormatAndMount
	exec.Interface
}


func newNodeMounter() Mounter {
	return &NodeMounter{
		mount.SafeFormatAndMount{
			Interface: mount.New(""),
			Exec:      exec.New(),
		},
		exec.New(),
	}
}

func (m *NodeMounter) GetDeviceName(mountPath string) (string, int, error) {
	return mount.GetDeviceNameFromMount(m, mountPath)
}

func (m *NodeMounter) MakeFile(pathname string) error {
	f, err := os.OpenFile(pathname, os.O_CREATE, os.FileMode(0644))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	defer f.Close()
	return nil
}

func (m *NodeMounter) MakeDir(pathname string) error {
	err := os.MkdirAll(pathname, os.FileMode(0755))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func (m *NodeMounter) ExistsPath(filename string) (bool, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}


func (m *NodeMounter) GetStatistics(filename string) (volumeStats, error) {
	isBlock, err := m.IsBlockDevice(filename)
	if err != nil {
		return volumeStats{}, fmt.Errorf("unable to determine if volume %s is a block device: %v", filename, err)
	}

	// We'll use the blockdev command to get the block device size.
	if isBlock {
		output, err := osexec.Command("blockdev", "getsize64", filename).CombinedOutput()
		if err != nil {
			return volumeStats{}, fmt.Errorf("error when getting size of block volume at path %s: output: %s, err: %v", filename, string(output), err)
		}

		strSize := strings.TrimSpace(string(output))
		sizeBytes, err := strconv.ParseInt(strSize, 10, 64)
		if err != nil {
			return volumeStats{}, fmt.Errorf("size %s is not int", strSize)
		}

		return volumeStats{
			totalBytes: sizeBytes,
		}, nil
	}

	var statfs unix.Statfs_t

	err = unix.Statfs(filename, &statfs)
	if err != nil {
		return volumeStats{}, err
	}

	volStats := volumeStats{
		totalBytes:  int64(statfs.Blocks) * int64(statfs.Bsize),
		availBytes:  int64(statfs.Bavail) * int64(statfs.Bsize),
		usedBytes:   (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize),
		totalInodes: int64(statfs.Files),
		availInodes: int64(statfs.Ffree),
		usedInodes:  int64(statfs.Files) - int64(statfs.Ffree),
	}

	return volStats, nil
}

func (m *NodeMounter) IsBlockDevice(filename string) (bool, error) {
	// Use stat to determine the kind of file this is.
	var stat unix.Stat_t

	err := unix.Stat(filename, &stat)
	if err != nil {
		return false, err
	}

	return (stat.Mode & unix.S_IFMT) == unix.S_IFBLK, nil
}
