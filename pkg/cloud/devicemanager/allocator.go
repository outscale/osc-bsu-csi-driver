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

import "errors"

// ExistingNames is a map of assigned device names. Presence of a key with a device
// name in the map means that the device is allocated. Value is irrelevant and
// can be used for anything that NameAllocator user wants.  Only the relevant
// part of device name should be in the map, e.g. "ba" for "/dev/xvdba".
type ExistingNames map[string]string

// On Outscale, we should assign new (not yet used) device names to attached volumes.
// If we reuse a previously used name, we may get the volume "attaching" forever.
// NameAllocator finds available device name, taking into account already
// assigned device names from ExistingNames map. It tries to find the next
// device name to the previously assigned one (from previous NameAllocator
// call), so all available device names are used eventually and it minimizes
// device name reuse.
type NameAllocator interface {
	// GetNext returns a free device name or error when there is no free device
	// name. Only the device name is returned, e.g. "ba" for "/dev/xvdba".
	// It's up to the called to add appropriate "/dev/sd" or "/dev/xvd" prefix.
	GetNext(existingNames ExistingNames) (name string, err error)
}

type nameAllocator struct{}

var _ NameAllocator = &nameAllocator{}

// GetNext gets next available device given existing names that are being used
// This function iterate through the device names in deterministic order of:
//
//	ba, ... ,bz, ca, ... , cz
//
// and return the first one that is not used yet.
func (d *nameAllocator) GetNext(existingNames ExistingNames) (string, error) {
	device_map := [40]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "aa", "ab", "ac", "ad", "ae", "af", "ag", "ah", "ai", "aj", "ak", "al", "am", "an"}
	for i := 1; i < 40; i++ {
		name := device_map[i]
		if _, found := existingNames[name]; !found {
			return name, nil
		}
	}
	return "", errors.New("there are no names available")
}
