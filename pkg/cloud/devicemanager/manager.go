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
	"errors"
	"fmt"
	"strings"
	"sync"

	osc "github.com/outscale/osc-sdk-go/v2"

	"k8s.io/klog/v2"
)

const devPreffix = "/dev/xvd"

type Device struct {
	Instance          osc.Vm
	Path              string
	VolumeID          string
	IsAlreadyAssigned bool

	isTainted   bool
	releaseFunc func() error
}

func (d *Device) Release(force bool) {
	if !d.isTainted || force {
		if err := d.releaseFunc(); err != nil {
			// FIXME: low level function should never log
			klog.Errorf("Error releasing device: %v", err)
		}
	}
}

// Taint marks the device as no longer reusable
func (d *Device) Taint() {
	d.isTainted = true
}

func IsNilDevice(d Device) bool {
	return !d.Instance.HasVmId()
}

func IsNilVm(vm osc.Vm) bool {
	return !vm.HasVmId()
}

type DeviceManager interface {
	// NewDevice retrieves the device if the device is already assigned.
	// Otherwise it creates a new device with next available device name
	// and mark it as unassigned device.
	NewDevice(instance osc.Vm, volumeID string) (device Device, err error)

	// GetDevice returns the device already assigned to the volume.
	GetDevice(instance osc.Vm, volumeID string) (device Device)
}

type deviceManager struct {
	// nameAllocator assigns new device name
	nameAllocator NameAllocator

	// We keep an active list of devices we have assigned but not yet
	// attached, to avoid a race condition where we assign a device mapping
	// and then get a second request before we attach the volume.
	mux      sync.Mutex
	inFlight inFlightAttaching
}

var _ DeviceManager = &deviceManager{}

// inFlightAttaching represents the device names being currently attached to nodes.
// A valid pseudo-representation of it would be {"nodeID": {"deviceName: "volumeID"}}.
type inFlightAttaching map[string]map[string]string

func (i inFlightAttaching) Add(nodeID, volumeID, name string) {
	attaching := i[nodeID]
	if attaching == nil {
		attaching = make(map[string]string)
		i[nodeID] = attaching
	}
	attaching[name] = volumeID
}

func (i inFlightAttaching) Del(nodeID, name string) {
	delete(i[nodeID], name)
}

func (i inFlightAttaching) GetNames(nodeID string) map[string]string {
	return i[nodeID]
}

func (i inFlightAttaching) GetVolume(nodeID, name string) string {
	return i[nodeID][name]
}

func NewDeviceManager() DeviceManager {
	return &deviceManager{
		nameAllocator: &nameAllocator{},
		inFlight:      make(inFlightAttaching),
	}
}

func (d *deviceManager) NewDevice(instance osc.Vm, volumeID string) (Device, error) {
	d.mux.Lock()
	defer d.mux.Unlock()

	if IsNilVm(instance) {
		return Device{}, errors.New("instance is nil")
	}

	// Get device names being attached and already attached to this instance
	inUse := d.getDeviceNamesInUse(instance)

	// Check if this volume is already assigned a device on this machine
	if path := d.getPath(inUse, volumeID); path != "" {
		return d.newBlockDevice(instance, volumeID, path, true), nil
	}

	nodeID, err := getInstanceID(instance)
	if err != nil {
		return Device{}, err
	}

	name, err := d.nameAllocator.GetNext(inUse)
	if err != nil {
		return Device{}, fmt.Errorf("could not get a free device name to assign to node %s", nodeID)
	}

	// Add the chosen device and volume to the "attachments in progress" map
	d.inFlight.Add(nodeID, volumeID, name)

	return d.newBlockDevice(instance, volumeID, devPreffix+name, false), nil
}

func (d *deviceManager) GetDevice(instance osc.Vm, volumeID string) Device {
	d.mux.Lock()
	defer d.mux.Unlock()

	inUse := d.getDeviceNamesInUse(instance)

	if path := d.getPath(inUse, volumeID); path != "" {
		return d.newBlockDevice(instance, volumeID, path, true)
	}

	return d.newBlockDevice(instance, volumeID, "", false)
}

func (d *deviceManager) newBlockDevice(instance osc.Vm, volumeID string, path string, isAlreadyAssigned bool) Device {
	device := Device{
		Instance:          instance,
		Path:              path,
		VolumeID:          volumeID,
		IsAlreadyAssigned: isAlreadyAssigned,

		isTainted: false,
	}
	device.releaseFunc = func() error {
		return d.release(device)
	}
	return device
}

func (d *deviceManager) release(device Device) error {
	nodeID, err := getInstanceID(device.Instance)
	if err != nil {
		return err
	}

	d.mux.Lock()
	defer d.mux.Unlock()

	var name string
	if len(device.Path) > 2 {
		name = strings.TrimPrefix(device.Path, devPreffix)
	}

	existingVolumeID := d.inFlight.GetVolume(nodeID, name)
	if len(existingVolumeID) == 0 {
		// Attaching is not in progress, so there's nothing to release
		return nil
	}

	if device.VolumeID != existingVolumeID {
		// This actually can happen, because GetNext combines the inFlightAttaching map with the volumes
		// attached to the instance (as reported by the EC2 API).  So if release comes after
		// a 10 second poll delay, we might as well have had a concurrent request to allocate a mountpoint,
		// which because we allocate sequentially is very likely to get the immediately freed volume.
		return fmt.Errorf("release on device %q assigned to different volume: %q vs %q", device.Path, device.VolumeID, existingVolumeID)
	}

	// klog.V(5).Infof("Releasing in-process attachment entry: %v -> volume %s", device.Path, device.VolumeID)
	d.inFlight.Del(nodeID, name)

	return nil
}

// getDeviceNamesInUse returns the device to volume ID mapping
// the mapping includes both already attached and being attached volumes
func (d *deviceManager) getDeviceNamesInUse(instance osc.Vm) map[string]string {
	nodeID := instance.GetVmId()
	inUse := map[string]string{}
	for _, blockDevice := range instance.GetBlockDeviceMappings() {
		name := blockDevice.GetDeviceName()
		// trims /dev/sd or /dev/xvd from device name
		name = strings.TrimPrefix(name, "/dev/sd")
		name = strings.TrimPrefix(name, "/dev/xvd")

		if len(name) < 1 || len(name) > 2 {
			klog.Warningf("Unexpected BSU DeviceName: %q", *blockDevice.DeviceName)
		}
		inUse[name] = blockDevice.Bsu.GetVolumeId()
	}

	// klog.V(5).Infof("DeviceNameInUse: APIDevice: %v, CacheDevice: %v", inUse, d.inFlight.GetNames(nodeID))
	for name, volumeID := range d.inFlight.GetNames(nodeID) {
		inUse[name] = volumeID
	}

	return inUse
}

func (d *deviceManager) getPath(inUse map[string]string, volumeID string) string {
	for name, volID := range inUse {
		if volumeID == volID {
			return devPreffix + name
		}
	}
	return ""
}

func getInstanceID(instance osc.Vm) (string, error) {
	if IsNilVm(instance) {
		return "", errors.New("can't get ID from a nil instance")
	}
	return instance.GetVmId(), nil
}
