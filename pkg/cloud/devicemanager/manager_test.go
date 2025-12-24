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
	"testing"

	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
)

func TestNewDevice(t *testing.T) {
	testCases := []struct {
		name               string
		instanceID         string
		existingDevicePath string
		existingVolumeID   string
		volumeID           string
	}{
		{
			name:               "success: normal",
			instanceID:         "instance-1",
			existingDevicePath: "/dev/xvdbc",
			existingVolumeID:   "vol-1",
			volumeID:           "vol-2",
		},
		{
			name:               "success: parallel same instance but different volumes",
			instanceID:         "instance-1",
			existingDevicePath: "/dev/xvdbc",
			existingVolumeID:   "vol-1",
			volumeID:           "vol-4",
		},
		{
			name:               "success: parallel different instances but same volume",
			instanceID:         "instance-2",
			existingDevicePath: "/dev/xvdbc",
			existingVolumeID:   "vol-1",
			volumeID:           "vol-4",
		},
	}
	// Use a shared DeviceManager to make sure that there are no race conditions
	dm := NewDeviceManager()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should fail if instance is nil
			fakeInstance := newFakeInstance(tc.instanceID, tc.existingVolumeID, tc.existingDevicePath)

			// Should create valid Device with valid path
			dev1, err := dm.NewDevice(fakeInstance, tc.volumeID)
			assertDevice(t, dev1, false, err)

			// Devices with same instance and volume should have same paths
			dev2, err := dm.NewDevice(fakeInstance, tc.volumeID)
			assertDevice(t, dev2, true /*IsAlreadyAssigned*/, err)
			if dev1.Path != dev2.Path {
				t.Fatalf("Expected equal paths, got %v and %v", dev1.Path, dev2.Path)
			}

			// Should create new Device with the same path after releasing
			dev2.Release(false)
			dev3, err := dm.NewDevice(fakeInstance, tc.volumeID)
			assertDevice(t, dev3, false, err)
			if dev3.Path != dev1.Path {
				t.Fatalf("Expected equal paths, got %v and %v", dev1.Path, dev3.Path)
			}
			dev3.Release(false)
		})
	}
}

func TestGetDevice(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "success: normal",
			testFunc: func(t *testing.T) {
				instanceID := "instance-1"
				existingDevicePath := "/dev/xvdbc"
				existingVolumeID := "vol-1"
				volumeID := "vol-2"

				dm := NewDeviceManager()
				fakeInstance := newFakeInstance(instanceID, existingVolumeID, existingDevicePath)

				// Should create valid Device with valid path
				dev1, err := dm.NewDevice(fakeInstance, volumeID)
				assertDevice(t, dev1, false /*IsAlreadyAssigned*/, err)

				// Devices with same instance and volume should have same paths
				dev2 := dm.GetDevice(fakeInstance, volumeID)
				assertDevice(t, dev2, true /*IsAlreadyAssigned*/, err)
				if dev1.Path != dev2.Path {
					t.Fatalf("Expected equal paths, got %v and %v", dev1.Path, dev2.Path)
				}
			},
		},
		{
			name: "success: device not attached",
			testFunc: func(t *testing.T) {
				instanceID := "instance-1"
				existingDevicePath := "/dev/xvdbc"
				existingVolumeID := "vol-1"
				volumeID := "vol-2"

				dm := NewDeviceManager()
				fakeInstance := newFakeInstance(instanceID, existingVolumeID, existingDevicePath)

				// Devices with same instance and volume should have same paths
				dev2 := dm.GetDevice(fakeInstance, volumeID)
				assertDevice(t, dev2, false /*IsAlreadyAssigned*/, nil)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestReleaseDevice(t *testing.T) {
	testCases := []struct {
		name               string
		instanceID         string
		existingDevicePath string
		existingVolumeID   string
		volumeID           string
	}{
		{
			name:               "success: normal",
			instanceID:         "instance-1",
			existingDevicePath: "/dev/xvdbc",
			existingVolumeID:   "vol-1",
			volumeID:           "vol-2",
		},
	}

	dm := NewDeviceManager()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeInstance := newFakeInstance(tc.instanceID, tc.existingVolumeID, tc.existingDevicePath)

			// Should get assigned Device after releasing tainted device
			dev, err := dm.NewDevice(fakeInstance, tc.volumeID)
			assertDevice(t, dev, false /*IsAlreadyAssigned*/, err)
			dev.Taint()
			dev.Release(false)
			dev2 := dm.GetDevice(fakeInstance, tc.volumeID)
			assertDevice(t, dev2, true /*IsAlreadyAssigned*/, nil)
			if dev.Path != dev2.Path {
				t.Fatalf("Expected device to be already assigned, got unassigned")
			}

			// Should release tainted device if force=true is passed in
			dev2.Release(true)
			dev3 := dm.GetDevice(fakeInstance, tc.volumeID)
			assertDevice(t, dev3, false /*IsAlreadyAssigned*/, nil)
		})
	}
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

func assertDevice(t *testing.T, d Device, assigned bool, err error) {
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if d.IsAlreadyAssigned != assigned {
		t.Fatalf("Expected IsAlreadyAssigned to be %v, got %v", assigned, d.IsAlreadyAssigned)
	}
}
