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

package options

import (
	"github.com/outscale/goutils/k8s/sdk"
	"github.com/spf13/pflag"
	cliflag "k8s.io/component-base/cli/flag"
)

type CloudOptions struct {
	SDKOptions sdk.Options
}

// ControllerOptions contains options and configuration settings for the controller service.
type ControllerOptions struct {
	CloudOptions
	// ExtraVolumeTags is a map of tags that will be attached to each dynamically provisioned
	// volume.
	ExtraVolumeTags map[string]string
	// ExtraSnapshotTags is a map of tags that will be attached to each snapshot created.
	ExtraSnapshotTags map[string]string
}

func (s *ControllerOptions) AddFlags(fs *pflag.FlagSet) {
	fs.Var(cliflag.NewMapStringString(&s.ExtraVolumeTags), "extra-volume-tags", "Extra volume tags to attach to each dynamically provisioned volume. It is a comma separated list of key value pairs like '<key1>=<value1>,<key2>=<value2>'")
	fs.Var(cliflag.NewMapStringString(&s.ExtraSnapshotTags), "extra-snapshot-tags", "Extra snapshot tags to attach to each created snapshot. It is a comma separated list of key value pairs like '<key1>=<value1>,<key2>=<value2>'")
	s.SDKOptions.AddFlags(fs)
}
