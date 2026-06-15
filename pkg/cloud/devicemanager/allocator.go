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
	"slices"
)

// NameAllocator finds available device name, taking into account already
// assigned device names. It tries to find the next unused device name.
type NameAllocator interface {
	// GetNext returns a free device name or error when there is no free device
	// name. Only the device name is returned, e.g. "ba" for "/dev/xvdba".
	// It's up to the called to add appropriate "/dev/sd" or "/dev/xvd" prefix.
	GetNext(existing []string) (name string, err error)
}

type nameAllocator struct{}

var _ NameAllocator = &nameAllocator{}

// GetNext gets next available device name.
// This function iterates through the device names in deterministic order of:
//
//	b ... z, aa ... az
//
// and return the first one that is not used yet.
// Note: a is reserved for the root volume.
func (d *nameAllocator) GetNext(existing []string) (string, error) {
	deviceMap := [51]string{"b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "aa", "ab", "ac", "ad", "ae", "af", "ag", "ah", "ai", "aj", "ak", "al", "am", "an", "ao", "ap", "aq", "ar", "as", "at", "au", "av", "aw", "ax", "ay", "az"}
	for _, name := range deviceMap {
		if !slices.Contains(existing, name) {
			return name, nil
		}
	}
	return "", errors.New("there are no names available")
}
