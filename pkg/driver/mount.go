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
	"os"

	"github.com/outscale/osc-bsu-csi-driver/pkg/driver/luks"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

// Mounter is an interface for mount operations
type Mounter interface {
	mount.Interface
	exec.Interface
	luks.LuksService
	FormatAndMount(source string, target string, fstype string, options []string) error
	GetDiskFormat(disk string) (string, error)
	GetDeviceName(mountPath string) (string, int, error)
	MakeFile(pathname string) error
	MakeDir(pathname string) error
	ExistsPath(filename string) (bool, error)
	IsCorruptedMnt(error) bool
}

type NodeMounter struct {
	mount.SafeFormatAndMount
	exec.Interface
}

func newNodeMounter() Mounter {
	return &NodeMounter{
		SafeFormatAndMount: mount.SafeFormatAndMount{
			Interface: mount.New(""),
			Exec:      exec.New(),
		},
		Interface: exec.New(),
	}
}

func (m *NodeMounter) GetDeviceName(mountPath string) (string, int, error) {
	return mount.GetDeviceNameFromMount(m, mountPath)
}

func (m NodeMounter) IsCorruptedMnt(err error) bool {
	return mount.IsCorruptedMnt(err)
}

func (m *NodeMounter) MakeFile(pathname string) error {
	f, err := os.OpenFile(pathname, os.O_CREATE, 0644) //nolint: gosec
	switch {
	case os.IsExist(err):
		return nil
	case err != nil:
		return err
	default:
		return f.Close()
	}
}

func (m *NodeMounter) MakeDir(pathname string) error {
	err := os.MkdirAll(pathname, 0755) //nolint: gosec
	if err != nil && !os.IsExist(err) {
		return err
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

func (m *NodeMounter) IsLuks(devicePath string) bool {
	return IsLuks(m, devicePath)
}

func (m *NodeMounter) LuksFormat(devicePath, passphrase string, context luks.LuksContext) error {
	return LuksFormat(m, devicePath, passphrase, context)
}

func (m *NodeMounter) CheckLuksPassphrase(devicePath, passphrase string) error {
	return CheckLuksPassphrase(m, devicePath, passphrase)
}

func (m *NodeMounter) LuksOpen(devicePath, encryptedDeviceName, passphrase string, luksOpenFlags ...string) (bool, error) {
	return LuksOpen(m, devicePath, encryptedDeviceName, passphrase, luksOpenFlags...)
}

func (m *NodeMounter) IsLuksMapping(devicePath string) (bool, string, error) {
	return IsLuksMapping(m, devicePath)
}

func (m *NodeMounter) LuksResize(deviceName, passphrase string) error {
	return LuksResize(m, deviceName, passphrase)
}

func (m *NodeMounter) LuksClose(deviceName string) error {
	return LuksClose(m, deviceName)
}
