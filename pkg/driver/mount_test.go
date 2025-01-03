/*
Copyright 2020 The Kubernetes Authors.

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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeDir(t *testing.T) {
	// Setup the full driver and its environment
	dir, err := os.MkdirTemp(t.TempDir(), "mount-bsu-csi")
	require.NoError(t, err)

	targetPath := filepath.Join(dir, "targetdir")

	mountObj := newNodeMounter()

	err = mountObj.MakeDir(targetPath)
	require.NoError(t, err)

	err = mountObj.MakeDir(targetPath)
	require.NoError(t, err)

	exists, err := mountObj.ExistsPath(targetPath)
	require.NoError(t, err)
	assert.True(t, exists, "The directory must have been created")
}

func TestMakeFile(t *testing.T) {
	// Setup the full driver and its environment
	dir, err := os.MkdirTemp(t.TempDir(), "mount-bsu-csi")
	require.NoError(t, err)

	targetPath := filepath.Join(dir, "targetfile")

	mountObj := newNodeMounter()

	err = mountObj.MakeFile(targetPath)
	require.NoError(t, err)

	err = mountObj.MakeFile(targetPath)
	require.NoError(t, err)

	exists, err := mountObj.ExistsPath(targetPath)
	require.NoError(t, err)
	assert.True(t, exists, "The file must have been created")
}

func TestExistsPath(t *testing.T) {
	// Setup the full driver and its environment
	dir, err := os.MkdirTemp(t.TempDir(), "mount-bsu-csi")
	require.NoError(t, err)

	targetPath := filepath.Join(dir, "notafile")

	mountObj := newNodeMounter()

	exists, err := mountObj.ExistsPath(targetPath)
	require.NoError(t, err)
	assert.False(t, exists, "The path must not exist")
}

func TestGetDeviceName(t *testing.T) {
	// Setup the full driver and its environment
	dir, err := os.MkdirTemp(t.TempDir(), "mount-bsu-csi")
	require.NoError(t, err)

	targetPath := filepath.Join(dir, "notafile")

	mountObj := newNodeMounter()

	_, _, err = mountObj.GetDeviceName(targetPath)
	require.NoError(t, err)
}
