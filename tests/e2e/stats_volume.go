/*
Copyright 2018 The Kubernetes Authors.

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

package e2e

import (
	. "github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"

	"github.com/outscale-dev/osc-bsu-csi-driver/tests/e2e/driver"
	"github.com/outscale-dev/osc-bsu-csi-driver/tests/e2e/testsuites"

	osccloud "github.com/outscale-dev/osc-bsu-csi-driver/pkg/cloud"
	bsucsidriver "github.com/outscale-dev/osc-bsu-csi-driver/pkg/driver"
)

var _ = Describe("[ebs-csi-e2e] [single-az] Stats", func() {
	f := framework.NewDefaultFramework("ebs")

	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		ebsDriver driver.PVTestDriver
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		ebsDriver = driver.InitEbsCSIDriver()
	})

	It("Check stats of a volume created from a deployment object, write and read to it", func() {
		pod := testsuites.PodDetails{
			Cmd: "truncate -s 10000 /mnt/test-1/data && sleep 10 ; while true; do echo running ; sleep 1; done",
			Volumes: []testsuites.VolumeDetails{
				{
					VolumeType: osccloud.VolumeTypeGP2,
					FSType:     bsucsidriver.FSTypeExt3,
					ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedStatsPodTest{
			CSIDriver: ebsDriver,
			Pod:       pod,
		}
		test.Run(cs, ns, f)
	})
})
