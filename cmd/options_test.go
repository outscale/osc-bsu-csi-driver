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

package main

import (
	"flag"
	"os"
	"testing"

	"github.com/outscale/osc-bsu-csi-driver/pkg/driver"
	"github.com/stretchr/testify/require"
)

func TestGetOptions(t *testing.T) {
	testFunc := func(
		t *testing.T,
		additionalArgs []string,
		withServerOptions bool,
		withControllerOptions bool,
		withNodeOptions bool,
	) *Options {
		flagSet := flag.NewFlagSet("test-flagset", flag.ContinueOnError)

		endpointFlagName := "endpoint"
		endpoint := "foo"

		extraVolumeTagsFlagName := "extra-volume-tags"
		extraVolumeTagKey := "bar"
		extraVolumeTagValue := "baz"
		extraVolumeTags := map[string]string{
			extraVolumeTagKey: extraVolumeTagValue,
		}

		extraSnapshotTagsFlagName := "extra-snapshot-tags"
		extraSnapshotTagKey := "bar"
		extraSnapshotTagValue := "baz"
		extraSnapshotTags := map[string]string{
			extraSnapshotTagKey: extraSnapshotTagValue,
		}

		args := append([]string{
			"osc-bsu-csi-driver",
		}, additionalArgs...)

		if withServerOptions {
			args = append(args, "-"+endpointFlagName+"="+endpoint)
		}
		if withControllerOptions {
			args = append(args, "-"+extraVolumeTagsFlagName+"="+extraVolumeTagKey+"="+extraVolumeTagValue)
			args = append(args, "-"+extraSnapshotTagsFlagName+"="+extraSnapshotTagKey+"="+extraSnapshotTagValue)
		}

		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()
		os.Args = args

		options := GetOptions(flagSet)

		if withServerOptions {
			endpointFlag := flagSet.Lookup(endpointFlagName)
			require.NotNil(t, endpointFlag)
			require.Equal(t, endpoint, options.ServerOptions.Endpoint)
		}

		if withControllerOptions {
			extraVolumeTagsFlag := flagSet.Lookup(extraVolumeTagsFlagName)
			require.NotNil(t, extraVolumeTagsFlag)
			require.Equal(t, extraVolumeTags, options.ControllerOptions.ExtraVolumeTags)

			extraSnapshotTagsFlag := flagSet.Lookup(extraSnapshotTagsFlagName)
			require.NotNil(t, extraSnapshotTagsFlag)
			require.Equal(t, extraSnapshotTags, options.ControllerOptions.ExtraSnapshotTags)
		}

		return options
	}

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "no controller mode given - expect all mode",
			testFunc: func(t *testing.T) {
				options := testFunc(t, nil, true, true, true)

				if options.DriverMode != driver.AllMode {
					t.Fatalf("expected driver mode to be %q but it is %q", driver.AllMode, options.DriverMode)
				}
			},
		},
		{
			name: "all mode given - expect all mode",
			testFunc: func(t *testing.T) {
				options := testFunc(t, []string{"all"}, true, true, true)

				if options.DriverMode != driver.AllMode {
					t.Fatalf("expected driver mode to be %q but it is %q", driver.AllMode, options.DriverMode)
				}
			},
		},
		{
			name: "controller mode given - expect controller mode",
			testFunc: func(t *testing.T) {
				options := testFunc(t, []string{"controller"}, true, true, false)

				if options.DriverMode != driver.ControllerMode {
					t.Fatalf("expected driver mode to be %q but it is %q", driver.ControllerMode, options.DriverMode)
				}
			},
		},
		{
			name: "node mode given - expect node mode",
			testFunc: func(t *testing.T) {
				options := testFunc(t, []string{"node"}, true, false, true)

				if options.DriverMode != driver.NodeMode {
					t.Fatalf("expected driver mode to be %q but it is %q", driver.NodeMode, options.DriverMode)
				}
			},
		},
		{
			name: "version flag specified",
			testFunc: func(t *testing.T) {
				oldOSExit := osExit
				defer func() { osExit = oldOSExit }()

				var exitCode int
				testExit := func(code int) {
					exitCode = code
				}
				osExit = testExit

				oldArgs := os.Args
				defer func() { os.Args = oldArgs }()
				os.Args = []string{
					"osc-bsu-csi-driver",
					"-version",
				}

				flagSet := flag.NewFlagSet("test-flagset", flag.ContinueOnError)
				_ = GetOptions(flagSet)

				require.Zero(t, exitCode)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}
