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

package main

import (
	"flag"

	"github.com/outscale/osc-bsu-csi-driver/pkg/driver"

	"k8s.io/klog/v2"
)

func main() {
	fs := flag.NewFlagSet("osc-bsu-csi-driver", flag.ExitOnError)
	options := GetOptions(fs)

	drv, err := driver.NewDriver(
		driver.WithEndpoint(options.ServerOptions.Endpoint),
		driver.WithExtraVolumeTags(options.ControllerOptions.ExtraVolumeTags),
		driver.WithExtraSnapshotTags(options.ControllerOptions.ExtraSnapshotTags),
		driver.WithLuksOpenFlags(options.NodeOptions.LuksOpenFlags),
		driver.WithMode(options.DriverMode),
	)
	if err != nil {
		klog.Fatalln(err)
	}
	if err := drv.Run(); err != nil {
		klog.Fatalln(err)
	}
}
