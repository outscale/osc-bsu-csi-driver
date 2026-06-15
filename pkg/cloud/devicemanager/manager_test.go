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

package devicemanager_test

import (
	"strconv"
	"testing"

	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud/devicemanager"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestAssignDevice(t *testing.T) {
	vm := newFakeInstance("i-foo", "vol-foo", "/dev/xvdbc")
	t.Run("AssignDevice should assign a new device to a new volume", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		dev, err := dm.AssignDevice(t.Context(), vm, "vol-bar")
		require.NoError(t, err)
		assert.NotEqual(t, "/dev/xvdbc", dev.Path())
	})
	t.Run("AssignDevice should return an already mounted device", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		dev, err := dm.AssignDevice(t.Context(), vm, "vol-foo")
		require.NoError(t, err)
		assert.Equal(t, "/dev/xvdbc", dev.Path())
	})
	t.Run("AssignDevice should reassign the same volume to the same device", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		dev1, err := dm.AssignDevice(t.Context(), vm, "vol-bar")
		require.NoError(t, err)
		dev2, err := dm.AssignDevice(t.Context(), vm, "vol-bar")
		require.NoError(t, err)
		assert.Equal(t, dev1.Path(), dev2.Path())
	})
	t.Run("AssignDevice should not assign the same device as a previously assigned device", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		dev1, err := dm.AssignDevice(t.Context(), vm, "vol-bar")
		require.NoError(t, err)
		dev2, err := dm.AssignDevice(t.Context(), vm, "vol-baz")
		require.NoError(t, err)
		assert.NotEqual(t, dev1.Path(), dev2.Path())
	})
	t.Run("AssignDevice should reassign to another device if the device has been linked in-between", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		dev1, err := dm.AssignDevice(t.Context(), vm, "vol-bar")
		require.NoError(t, err)
		assert.NotEqual(t, "/dev/xvdbc", dev1.Path())

		vmUpdated := newFakeInstance("i-foo", "vol-baz", dev1.Path())
		dev2, err := dm.AssignDevice(t.Context(), vmUpdated, "vol-bar")
		require.NoError(t, err)
		assert.NotEqual(t, dev1.Path(), dev2.Path())
	})
	t.Run("Multiple concurrent requests should assign different devices", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		wg := errgroup.Group{}
		count := 10
		wg.SetLimit(count)
		out := make(chan string, count)
		for i := range count {
			wg.Go(func() error {
				dev, err := dm.AssignDevice(t.Context(), vm, "vol-"+strconv.Itoa(i))
				if err != nil {
					return err
				}
				out <- dev.Path()
				return nil
			})
		}
		err := wg.Wait()
		require.NoError(t, err)
		close(out)
		paths := lo.ChannelToSlice(out)
		assert.Len(t, lo.Uniq(paths), count)
	})
}

func TestGetDevice(t *testing.T) {
	vm1 := newFakeInstance("i-foo", "vol-foo", "/dev/xvdbc")
	vm2 := newFakeInstance("i-bar", "vol-bar", "/dev/xvdbc")
	t.Run("GetDevice should return a mounted device", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		dev, err := dm.GetDevice(t.Context(), vm1, "vol-foo")
		require.NoError(t, err)
		assert.Equal(t, "/dev/xvdbc", dev.Path())
	})
	t.Run("GetDevice should return an assigned device", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		dev1, err := dm.AssignDevice(t.Context(), vm1, "vol-bar")
		require.NoError(t, err)
		dev2, err := dm.GetDevice(t.Context(), vm1, "vol-bar")
		require.NoError(t, err)
		assert.Equal(t, dev1.Path(), dev2.Path())
	})
	t.Run("GetDevice should return an error if volume does not exist", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		_, err := dm.GetDevice(t.Context(), vm1, "vol-bar")
		require.ErrorIs(t, err, devicemanager.ErrNotFound)
	})
	t.Run("GetDevice should return an error if volume is mounted on another vm", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		_, err := dm.GetDevice(t.Context(), vm2, "vol-foo")
		require.ErrorIs(t, err, devicemanager.ErrNotFound)
	})
}

func TestReleaseDevice(t *testing.T) {
	vm1 := newFakeInstance("i-foo", "vol-foo", "/dev/xvdbc")
	vm2 := newFakeInstance("i-bar", "vol-bar", "/dev/xvdbc")
	t.Run("ReleaseDevice should release an assigned device", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		_, err := dm.AssignDevice(t.Context(), vm1, "vol-bar")
		require.NoError(t, err)
		dm.Release(t.Context(), vm1, "vol-bar")
		_, err = dm.GetDevice(t.Context(), vm1, "vol-bar")
		require.ErrorIs(t, err, devicemanager.ErrNotFound)
	})
	t.Run("ReleaseDevice should not release a device from another vm", func(t *testing.T) {
		dm := devicemanager.NewDeviceManager()
		_, err := dm.GetDevice(t.Context(), vm2, "vol-bar")
		require.NoError(t, err)
		dm.Release(t.Context(), vm1, "vol-bar")
		_, err = dm.GetDevice(t.Context(), vm2, "vol-bar")
		require.NoError(t, err)
	})
}

func newFakeInstance(instanceID, volumeID, devicePath string) *osc.Vm {
	return &osc.Vm{
		VmId: instanceID,
		BlockDeviceMappings: []osc.BlockDeviceMappingCreated{
			{
				DeviceName: devicePath,
				Bsu:        osc.BsuCreated{VolumeId: volumeID},
			},
		},
	}
}
