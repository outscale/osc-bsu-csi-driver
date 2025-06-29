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
	"strings"

	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud"
)

func ValidateDriverOptions(options *DriverOptions) error {
	if err := validateExtraVolumeTags(options.extraVolumeTags); err != nil {
		return fmt.Errorf("invalid extra volume tags: %w", err)
	}

	if err := validateMode(options.mode); err != nil {
		return fmt.Errorf("invalid mode: %w", err)
	}

	return nil
}

func validateExtraVolumeTags(tags map[string]string) error {
	if len(tags) > cloud.MaxNumTagsPerResource {
		return fmt.Errorf("too many volume tags (actual: %d, limit: %d)", len(tags), cloud.MaxNumTagsPerResource)
	}

	for k, v := range tags {
		if len(k) > cloud.MaxTagKeyLength {
			return fmt.Errorf("volume tag key too long (actual: %d, limit: %d)", len(k), cloud.MaxTagKeyLength)
		}
		if len(v) > cloud.MaxTagValueLength {
			return fmt.Errorf("volume tag value too long (actual: %d, limit: %d)", len(v), cloud.MaxTagValueLength)
		}
		if k == cloud.VolumeNameTagKey {
			return fmt.Errorf("volume tag key '%s' is reserved", cloud.VolumeNameTagKey)
		}
		if strings.HasPrefix(k, cloud.KubernetesTagKeyPrefix) {
			return fmt.Errorf("volume tag key prefix '%s' is reserved", cloud.KubernetesTagKeyPrefix)
		}
		if strings.HasPrefix(k, cloud.OscTagKeyPrefix) {
			return fmt.Errorf("volume tag key prefix '%s' is reserved", cloud.OscTagKeyPrefix)
		}
	}

	return nil
}

func validateMode(mode Mode) error {
	if mode != AllMode && mode != ControllerMode && mode != NodeMode {
		return fmt.Errorf("mode is not supported (actual: %s, supported: %v)", mode, []Mode{AllMode, ControllerMode, NodeMode})
	}

	return nil
}
