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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNameAllocator(t *testing.T) {
	allocator := nameAllocator{}
	existingNames := []string{}
	expectedNames := "bcdefghijklmnopqrstuvwxyz"

	for i := range expectedNames {
		expectedName := expectedNames[i : i+1]
		t.Run(expectedName, func(t *testing.T) {
			actual, err := allocator.GetNext(existingNames)
			require.NoError(t, err)
			assert.Equal(t, expectedName, actual)
			existingNames = append(existingNames, actual)
		})
	}
}

func TestNameAllocatorError(t *testing.T) {
	allocator := nameAllocator{}
	existingNames := []string{} //nolint

	for range 77 {
		name, err := allocator.GetNext(existingNames)
		require.NoError(t, err)
		existingNames = append(existingNames, name)
	}
	name, err := allocator.GetNext(existingNames)
	require.Errorf(t, err, "expected error, got device %q", name)
}
