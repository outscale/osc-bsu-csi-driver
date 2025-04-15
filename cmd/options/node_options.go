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
	"flag"

	cliflag "k8s.io/component-base/cli/flag"
)

// NodeOptions contains options and configuration settings for the node service.
type NodeOptions struct {
	// LuksOpenFlags is a list of flags to add to cryptsetup luksOpen.
	// It is a comma separated list of strings '--perf-no_read_workqueue,--perf-no_write_workqueue'.
	LuksOpenFlags []string
}

func (s *NodeOptions) AddFlags(fs *flag.FlagSet) {
	fs.Var(cliflag.NewStringSlice(&s.LuksOpenFlags), "luks-open-flags", "Flag to add to cryptsetup luksOpen. It may be specified multiple times to add multiple flags, for example: '--luks-open-flags=--perf-no_read_workqueue --luks-open-flags=--perf-no_write_workqueue'")
}
