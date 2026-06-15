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

package devicemanager

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"github.com/samber/lo"
)

const (
	devPrefix           = "/dev/xvd"
	assignedMaxDuration = 15 * time.Minute
	expirationDuration  = 24 * time.Hour
)

var (
	Now         = time.Now
	ErrNotFound = errors.New("not found")
)

type Device struct {
	vm           string
	name         string
	volume       string
	fromMappings bool
	linked       bool

	assignedAt  time.Time
	refreshedAt time.Time
}

// MarkAsLinked marks the device as successfully mounted.
func (d *Device) MarkAsLinked() {
	d.linked = true
}

func (d *Device) Linked() bool {
	return d.linked
}

func (d *Device) Path() string {
	return devPrefix + d.name
}

type DeviceManager interface {
	// AssignDevice assigns a new device to a volume.
	// If a device has already been assigned, it is returned.
	AssignDevice(ctx context.Context, vm *osc.Vm, volumeID string) (device *Device, err error)

	// GetDevice returns the device already assigned to the volume.
	GetDevice(ctx context.Context, vm *osc.Vm, volumeID string) (device *Device, err error)

	Release(ctx context.Context, vm *osc.Vm, volumeID string)
}

type deviceManager struct {
	nameAllocator NameAllocator
	mu            sync.Mutex
	volumes       volumeMap
}

var _ DeviceManager = &deviceManager{}

// volumeMap represents the device names being currently attached to nodes.
// A valid pseudo-representation of it would be {"nodeID": {"deviceName": "volumeID"}}.
type volumeMap map[string]*Device

func (m volumeMap) Add(d *Device) {
	d.refreshedAt = Now()
	m[d.volume] = d
}

func (m volumeMap) Del(volume string) {
	delete(m, volume)
}

func (m volumeMap) Get(volume string) (*Device, bool) {
	dev, found := m[volume]
	return dev, found
}

// getDeviceName trims /dev/sd or /dev/xvd from device name
func getDeviceName(path string) string {
	name := strings.TrimPrefix(path, "/dev/sd")
	name = strings.TrimPrefix(name, "/dev/xvd")
	return name
}

func (m volumeMap) refresh(vm *osc.Vm) {
	// remove all previous mappings
	for vol, dev := range m {
		if dev.vm != vm.VmId || !dev.fromMappings {
			continue
		}
		delete(m, vol)
	}

	// add mappings
	for _, blockDevice := range vm.BlockDeviceMappings {
		name := getDeviceName(blockDevice.DeviceName)
		if len(name) < 1 || len(name) > 2 {
			continue
		}
		volume := blockDevice.Bsu.VolumeId
		// delete devices that have been incorrectly assigned to the same name.
		for vol, dev := range m {
			if dev.vm == vm.VmId && dev.name == name && dev.volume != volume {
				delete(m, vol)
			}
		}
		m[volume] = &Device{
			vm:           vm.VmId,
			name:         name,
			volume:       volume,
			fromMappings: true,
			linked:       true,
			refreshedAt:  Now(),
		}
	}

	// prune old entries (including for other vm, as nodes might have been deleted)
	for vol, dev := range m {
		if !dev.assignedAt.IsZero() && time.Since(dev.assignedAt) > assignedMaxDuration {
			delete(m, vol)
		}
		if !dev.refreshedAt.IsZero() && time.Since(dev.refreshedAt) > expirationDuration {
			delete(m, vol)
		}
	}
}

func (m volumeMap) GetUsedNames(vm string) []string {
	return lo.FilterMap(lo.Values(m), func(d *Device, _ int) (string, bool) {
		if d.vm != vm {
			return "", false
		}
		return d.name, true
	})
}

func NewDeviceManager() DeviceManager {
	return &deviceManager{
		nameAllocator: &nameAllocator{},
		volumes:       make(volumeMap),
	}
}

func (d *deviceManager) AssignDevice(ctx context.Context, vm *osc.Vm, volumeID string) (*Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.volumes.refresh(vm)

	// if already assigned to the right vm return the assigned device.
	// if already present, but with another vm, it must have been a missed unlink, reassign,
	if dev, found := d.volumes.Get(volumeID); found && dev.vm == vm.VmId {
		return dev, nil
	}

	// Assign a new name
	inUse := d.volumes.GetUsedNames(vm.VmId)
	name, err := d.nameAllocator.GetNext(inUse)
	if err != nil {
		return nil, err
	}

	dev := &Device{
		name:       name,
		vm:         vm.VmId,
		volume:     volumeID,
		assignedAt: Now(),
	}
	d.volumes.Add(dev)

	return dev, nil
}

func (d *deviceManager) GetDevice(ctx context.Context, vm *osc.Vm, volumeID string) (*Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.volumes.refresh(vm)

	if dev, found := d.volumes.Get(volumeID); found && dev.vm == vm.VmId {
		return dev, nil
	}

	return nil, ErrNotFound
}

func (d *deviceManager) Release(ctx context.Context, vm *osc.Vm, volumeID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if dev, found := d.volumes.Get(volumeID); found && dev.vm == vm.VmId {
		d.volumes.Del(volumeID)
	}
}
