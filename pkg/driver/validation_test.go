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
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud"
	"github.com/stretchr/testify/require"
)

func stringMap(n int) map[string]string {
	result := map[string]string{}
	for i := 0; i < n; i++ {
		result[strconv.Itoa(i)] = "foo"
	}
	return result
}

func TestValidateTags(t *testing.T) {
	testCases := []struct {
		name   string
		tags   map[string]string
		expErr error
	}{
		{
			name: "valid tags",
			tags: map[string]string{
				"extra-tag-key": "extra-tag-value",
			},
			expErr: nil,
		},
		{
			name: "invalid tag: key too long",
			tags: map[string]string{
				strings.Repeat("a", cloud.MaxTagKeyLength+1): "extra-tag-value",
			},
			expErr: fmt.Errorf("tag key too long (actual: %d, limit: %d)", cloud.MaxTagKeyLength+1, cloud.MaxTagKeyLength),
		},
		{
			name: "invalid tag: value too long",
			tags: map[string]string{
				"extra-tag-key": strings.Repeat("a", cloud.MaxTagValueLength+1),
			},
			expErr: fmt.Errorf("tag value too long (actual: %d, limit: %d)", cloud.MaxTagValueLength+1, cloud.MaxTagValueLength),
		},
		{
			name: "invalid tag: reserved CSI key",
			tags: map[string]string{
				cloud.VolumeNameTagKey: "extra-tag-value",
			},
			expErr: fmt.Errorf("tag key '%s' is reserved", cloud.VolumeNameTagKey),
		},
		{
			name: "invalid tag: reserved Kubernetes key prefix",
			tags: map[string]string{
				cloud.KubernetesTagKeyPrefix + "/cluster": "extra-tag-value",
			},
			expErr: fmt.Errorf("tag key prefix '%s' is reserved", cloud.KubernetesTagKeyPrefix),
		},
		{
			name: "invalid tag: reserved Osc key prefix",
			tags: map[string]string{
				cloud.OscTagKeyPrefix + "foo": "extra-tag-value",
			},
			expErr: fmt.Errorf("tag key prefix '%s' is reserved", cloud.OscTagKeyPrefix),
		},
		{
			name:   "invalid tag: too many volume tags",
			tags:   stringMap(cloud.MaxNumTagsPerResource + 1),
			expErr: fmt.Errorf("too many tags (actual: %d, limit: %d)", cloud.MaxNumTagsPerResource+1, cloud.MaxNumTagsPerResource),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTags(tc.tags)
			if tc.expErr == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expErr.Error())
			}
		})
	}
}

func TestValidateMode(t *testing.T) {
	testCases := []struct {
		name   string
		mode   Mode
		expErr error
	}{
		{
			name:   "valid mode: all",
			mode:   AllMode,
			expErr: nil,
		},
		{
			name:   "valid mode: controller",
			mode:   ControllerMode,
			expErr: nil,
		},
		{
			name:   "valid mode: node",
			mode:   NodeMode,
			expErr: nil,
		},
		{
			name:   "invalid mode: unknown",
			mode:   Mode("unknown"),
			expErr: fmt.Errorf("mode is not supported (actual: unknown, supported: %v)", []Mode{AllMode, ControllerMode, NodeMode}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMode(tc.mode)
			if tc.expErr == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expErr.Error())
			}
		})
	}
}

func TestValidateDriverOptions(t *testing.T) {
	testCases := []struct {
		name              string
		mode              Mode
		extraVolumeTags   map[string]string
		extraSnapshotTags map[string]string
		expErr            error
	}{
		{
			name:   "success",
			mode:   AllMode,
			expErr: nil,
		},
		{
			name:   "fail because validateMode fails",
			mode:   Mode("unknown"),
			expErr: fmt.Errorf("invalid mode: mode is not supported (actual: unknown, supported: %v)", []Mode{AllMode, ControllerMode, NodeMode}),
		},
		{
			name: "fail because of invalid volume tags",
			mode: AllMode,
			extraVolumeTags: map[string]string{
				strings.Repeat("a", cloud.MaxTagKeyLength+1): "extra-tag-value",
			},
			expErr: fmt.Errorf("invalid extra volume tags: tag key too long (actual: %d, limit: %d)", cloud.MaxTagKeyLength+1, cloud.MaxTagKeyLength),
		},
		{
			name: "fail because of invalid snapshot tags",
			mode: AllMode,
			extraSnapshotTags: map[string]string{
				strings.Repeat("a", cloud.MaxTagKeyLength+1): "extra-tag-value",
			},
			expErr: fmt.Errorf("invalid extra snapshot tags: tag key too long (actual: %d, limit: %d)", cloud.MaxTagKeyLength+1, cloud.MaxTagKeyLength),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDriverOptions(&DriverOptions{
				extraVolumeTags:   tc.extraVolumeTags,
				extraSnapshotTags: tc.extraSnapshotTags,
				mode:              tc.mode,
			})
			if tc.expErr == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expErr.Error())
			}
		})
	}
}
