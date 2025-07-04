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

// . "github.com/onsi/ginkgo/v2"

// This test requires kubelet to have the unauthenticated read-only port (tcp/10255) enabled, and it is disabled by default.
// Activating this test would require to enable the unauthenticated port.
// var _ = Describe("[bsu-csi-e2e] [single-az] Stats", func() {
// 	f := framework.NewDefaultFramework("bsu")
// 	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

// 	var (
// 		cs        clientset.Interface
// 		ns        *v1.Namespace
// 		bsuDriver driver.PVTestDriver
// 	)

// 	BeforeEach(func() {
// 		cs = f.ClientSet
// 		ns = f.Namespace
// 		bsuDriver = driver.InitBsuCSIDriver()
// 	})

// 	It("Check stats of a volume created from a deployment object, write and read to it", func() {
// 		pod := testsuites.PodDetails{
// 			Cmd: "truncate -s 10000 /mnt/test-1/data && sleep 10 ; while true; do echo running ; sleep 1; done",
// 			Volumes: []testsuites.VolumeDetails{
// 				{
// 					VolumeType: osccloud.VolumeTypeGP2,
// 					FSType:     bsucsidriver.FSTypeExt3,
// 					ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
// 					VolumeMount: testsuites.VolumeMountDetails{
// 						NameGenerate:      "test-volume-",
// 						MountPathGenerate: "/mnt/test-",
// 					},
// 				},
// 			},
// 		}
// 		test := testsuites.DynamicallyProvisionedStatsPodTest{
// 			CSIDriver: bsuDriver,
// 			Pod:       pod,
// 		}
// 		test.Run(cs, ns, f)
// 	})
// })
