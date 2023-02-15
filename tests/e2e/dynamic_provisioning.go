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
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	clientset "k8s.io/client-go/kubernetes"
	restclientset "k8s.io/client-go/rest"
	"k8s.io/kubernetes/test/e2e/framework"
	admissionapi "k8s.io/pod-security-admission/api"

	"github.com/outscale-dev/osc-bsu-csi-driver/tests/e2e/driver"
	"github.com/outscale-dev/osc-bsu-csi-driver/tests/e2e/testsuites"

	"github.com/outscale-dev/osc-bsu-csi-driver/pkg/cloud"
	osccloud "github.com/outscale-dev/osc-bsu-csi-driver/pkg/cloud"
	bsucsidriver "github.com/outscale-dev/osc-bsu-csi-driver/pkg/driver"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const OSC_REGION = "OSC_REGION"

var _ = Describe("[bsu-csi-e2e] [single-az] Dynamic Provisioning", func() {
	f := framework.NewDefaultFramework("bsu")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		bsuDriver driver.PVTestDriver
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		bsuDriver = driver.InitBsuCSIDriver()
	})

	for _, t := range osccloud.ValidVolumeTypes {
		for _, fs := range bsucsidriver.ValidFSTypes {
			volumeType := t
			fsType := fs
			It(fmt.Sprintf("should create a volume on demand with volume type %q and fs type %q", volumeType, fsType), func() {
				pods := []testsuites.PodDetails{
					{
						Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
						Volumes: []testsuites.VolumeDetails{
							{
								VolumeType: volumeType,
								FSType:     fsType,
								ClaimSize:  driver.MinimumSizeForVolumeType(volumeType),
								VolumeMount: testsuites.VolumeMountDetails{
									NameGenerate:      "test-volume-",
									MountPathGenerate: "/mnt/test-",
								},
							},
						},
					},
				}
				test := testsuites.DynamicallyProvisionedCmdVolumeTest{
					CSIDriver: bsuDriver,
					Pods:      pods,
				}
				test.Run(cs, ns)
			})
		}
	}

	for _, t := range osccloud.ValidVolumeTypes {
		volumeType := t
		It(fmt.Sprintf("should create a volume on demand with volumeType %q and encryption", volumeType), func() {
			Skip("Volume encryption is not supported for volume")
			pods := []testsuites.PodDetails{
				{
					Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
					Volumes: []testsuites.VolumeDetails{
						{
							VolumeType: volumeType,
							FSType:     bsucsidriver.FSTypeExt4,
							ClaimSize:  driver.MinimumSizeForVolumeType(volumeType),
							VolumeMount: testsuites.VolumeMountDetails{
								NameGenerate:      "test-volume-",
								MountPathGenerate: "/mnt/test-",
							},
						},
					},
				},
			}
			test := testsuites.DynamicallyProvisionedCmdVolumeTest{
				CSIDriver: bsuDriver,
				Pods:      pods,
			}
			test.Run(cs, ns)
		})
	}

	It("should create a volume on demand with provided mountOptions", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeType:   osccloud.VolumeTypeGP2,
						FSType:       bsucsidriver.FSTypeExt4,
						MountOptions: []string{"rw"},
						ClaimSize:    driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	It("should create multiple PV objects, bind to PVCs and attach all to a single pod", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && echo 'hello world' > /mnt/test-2/data && grep 'hello world' /mnt/test-1/data  && grep 'hello world' /mnt/test-2/data",
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
					{
						VolumeType: osccloud.VolumeTypeIO1,
						FSType:     bsucsidriver.FSTypeExt4,
						ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeIO1),
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	It("should create multiple PV objects, bind to PVCs and attach all to different pods", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
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
			},
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeType: osccloud.VolumeTypeIO1,
						FSType:     bsucsidriver.FSTypeExt4,
						ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeIO1),
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	It("should create a raw block volume on demand", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "dd if=/dev/zero of=/dev/xvda bs=1024k count=100",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeType: osccloud.VolumeTypeGP2,
						FSType:     bsucsidriver.FSTypeExt4,
						ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
						VolumeMode: testsuites.Block,
						VolumeDevice: testsuites.VolumeDeviceDetails{
							NameGenerate: "test-block-volume-",
							DevicePath:   "/dev/xvda",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	It("should create a raw block volume and a filesystem volume on demand and bind to the same pod", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "dd if=/dev/zero of=/dev/xvda bs=1024k count=100 && echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeType: osccloud.VolumeTypeIO1,
						FSType:     bsucsidriver.FSTypeExt4,
						ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeIO1),
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
					{
						VolumeType:   osccloud.VolumeTypeGP2,
						FSType:       bsucsidriver.FSTypeExt4,
						MountOptions: []string{"rw"},
						ClaimSize:    driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
						VolumeMode:   testsuites.Block,
						VolumeDevice: testsuites.VolumeDeviceDetails{
							NameGenerate: "test-block-volume-",
							DevicePath:   "/dev/xvda",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	It("should create multiple PV objects, bind to PVCs and attach all to different pods on the same node", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "while true; do echo $(date -u) >> /mnt/test-1/data; sleep 1; done",
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
			},
			{
				Cmd: "while true; do echo $(date -u) >> /mnt/test-1/data; sleep 1; done",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeType: osccloud.VolumeTypeIO1,
						FSType:     bsucsidriver.FSTypeExt4,
						ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeIO1),
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCollocatedPodTest{
			CSIDriver:    bsuDriver,
			Pods:         pods,
			ColocatePods: true,
		}
		test.Run(cs, ns)
	})

	// Track issue https://github.com/kubernetes/kubernetes/issues/70505
	It("should create a volume on demand and mount it as readOnly in a pod", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "touch /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeType: osccloud.VolumeTypeGP2,
						FSType:     bsucsidriver.FSTypeExt4,
						ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
							ReadOnly:          true,
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedReadOnlyVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	It("should create a volume with too many iops", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "touch /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeType: osccloud.VolumeTypeIO1,
						FSType:     bsucsidriver.FSTypeExt4,
						IopsPerGB:  fmt.Sprintf("%v", osccloud.MaxIopsPerGb),
						ClaimSize:  "44Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
							ReadOnly:          true,
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedReadOnlyVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	It("should create a volume with a ratio iops/size too high", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "touch /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeType: osccloud.VolumeTypeIO1,
						FSType:     bsucsidriver.FSTypeExt4,
						IopsPerGB:  fmt.Sprintf("%v", osccloud.MaxIopsPerGb+1),
						ClaimSize:  "4Gi",
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
							ReadOnly:          true,
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedReadOnlyVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	It(fmt.Sprintf("should delete PV with reclaimPolicy %q", v1.PersistentVolumeReclaimDelete), func() {
		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		volumes := []testsuites.VolumeDetails{
			{
				VolumeType:    osccloud.VolumeTypeGP2,
				FSType:        bsucsidriver.FSTypeExt4,
				ClaimSize:     driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
				ReclaimPolicy: &reclaimPolicy,
			},
		}
		test := testsuites.DynamicallyProvisionedReclaimPolicyTest{
			CSIDriver: bsuDriver,
			Volumes:   volumes,
		}
		test.Run(cs, ns)
	})

	It(fmt.Sprintf("[env] should retain PV with reclaimPolicy %q", v1.PersistentVolumeReclaimRetain), func() {
		if os.Getenv(awsAvailabilityZonesEnv) == "" {
			Skip(fmt.Sprintf("env %q not set", awsAvailabilityZonesEnv))
		}
		reclaimPolicy := v1.PersistentVolumeReclaimRetain
		volumes := []testsuites.VolumeDetails{
			{
				VolumeType:    osccloud.VolumeTypeGP2,
				FSType:        bsucsidriver.FSTypeExt4,
				ClaimSize:     driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
				ReclaimPolicy: &reclaimPolicy,
			},
		}
		availabilityZones := strings.Split(os.Getenv(awsAvailabilityZonesEnv), ",")
		availabilityZone := availabilityZones[rand.Intn(len(availabilityZones))]
		region := availabilityZone[0 : len(availabilityZone)-1]
		cloud, err := osccloud.NewCloudWithoutMetadata(region)
		if err != nil {
			Fail(fmt.Sprintf("could not get NewCloud: %v", err))
		}

		test := testsuites.DynamicallyProvisionedReclaimPolicyTest{
			CSIDriver: bsuDriver,
			Volumes:   volumes,
			Cloud:     cloud,
		}
		test.Run(cs, ns)
	})

	It("should create a deployment object, write and read to it, delete the pod and write and read to it again", func() {
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' >> /mnt/test-1/data && while true; do sleep 1; done",
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
		test := testsuites.DynamicallyProvisionedDeletePodTest{
			CSIDriver: bsuDriver,
			Pod:       pod,
			PodCheck: &testsuites.PodExecCheck{
				Cmd:            []string{"cat", "/mnt/test-1/data"},
				ExpectedString: "hello world\nhello world\n", // pod will be restarted so expect to see 2 instances of string
			},
		}
		test.Run(cs, ns)
	})
	It("should create a volume on demand and resize it", func() {
		allowVolumeExpansion := true
		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' >> /mnt/test-1/data && grep 'hello world' /mnt/test-1/data && sync",
			Volumes: []testsuites.VolumeDetails{
				{
					VolumeType: osccloud.VolumeTypeGP2,
					FSType:     bsucsidriver.FSTypeExt4,
					ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
					AllowVolumeExpansion: &allowVolumeExpansion,
				},
			},
		}
		test := testsuites.DynamicallyProvisionedResizeVolumeTest{
			CSIDriver: bsuDriver,
			Pod:       pod,
		}
		test.Run(cs, ns)
	})

	It("FSGROUP test should create a volume and check if pod security context is applied", func() {
		fsGroup := int64(5000)
		runAsGroup := int64(4000)
		runAsUser := int64(2000)
		podSecurityContext := v1.PodSecurityContext{
			RunAsUser:  &runAsUser,
			RunAsGroup: &runAsGroup,
			FSGroup:    &fsGroup,
		}
		podSc, err := podSecurityContext.Marshal()
		if err != nil {
			Fail(fmt.Sprintf("error encoding: %v, %v", podSecurityContext, err))
		}
		allowPrivilegeEscalation := false
		securityContext := v1.SecurityContext{
			AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		}
		sc, err := securityContext.Marshal()
		if err != nil {
			Fail(fmt.Sprintf("error encoding: %v, %v", securityContext, err))
		}

		pod := testsuites.PodDetails{
			Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data && while true; do echo running ; sleep 1; done",
			Volumes: []testsuites.VolumeDetails{
				{
					VolumeType: osccloud.VolumeTypeGP2,
					FSType:     bsucsidriver.FSTypeExt4,
					ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
			CustomizedPod: []string{
				string(podSc),
				string(sc),
			},
		}
		podCmds := []testsuites.PodCmds{
			{
				Cmd: []string{
					"stat",
					"-c",
					"%g",
					"/mnt/test-1",
				},
				ExpectedString: fmt.Sprintf("%d", fsGroup),
			},
		}
		test := testsuites.DynamicallyProvisionedCustomPodTest{
			CSIDriver: bsuDriver,
			Pod:       pod,
			PodCmds:   podCmds,
		}
		log.Printf("test: %+v\n", test)

		test.Run(cs, ns, f)
	})

	It("should create a volume, delete it from outside and release the volume", func() {
		binding := storagev1.VolumeBindingImmediate
		retain := v1.PersistentVolumeReclaimDelete
		volume := testsuites.VolumeDetails{
			VolumeType:        osccloud.VolumeTypeGP2,
			FSType:            bsucsidriver.FSTypeExt4,
			ClaimSize:         driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
			VolumeBindingMode: &binding,
			ReclaimPolicy:     &retain,
		}

		By("Create the PVC")
		tpvc, cleanups := volume.SetupDynamicPersistentVolumeClaim(cs, ns, bsuDriver)
		for i := range cleanups {
			defer cleanups[i]()
		}

		tpvc.WaitForBound()

		if os.Getenv(OSC_REGION) == "" {
			Skip(fmt.Sprintf("env %q not set", OSC_REGION))
		}

		By("Create the cloud")

		oscCloud, err := osccloud.NewCloudWithoutMetadata(os.Getenv(OSC_REGION))
		framework.ExpectNoError(err, "Error while creating a cloud configuration")

		By("Keep delete the disk until error")
		tpvc.DeleteBackingVolume(oscCloud)

		pv := tpvc.GetPersistentVolume()
		for i := 1; i < 100; i++ {
			_, err := oscCloud.DeleteDisk(context.Background(), pv.Spec.CSI.VolumeHandle)
			if err == cloud.ErrNotFound {
				break
			}
			fmt.Println("Disk still present, waiting")
			time.Sleep(1 * time.Second)
		}

	})
})

var _ = Describe("[bsu-csi-e2e] [single-az] Snapshot", func() {
	f := framework.NewDefaultFramework("bsu")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs          clientset.Interface
		snapshotrcs restclientset.Interface
		ns          *v1.Namespace
		bsuDriver   driver.PVTestDriver
	)

	BeforeEach(func() {
		cs = f.ClientSet
		var err error
		snapshotrcs, err = restClient(testsuites.SnapshotAPIGroup, testsuites.APIVersionv1)
		if err != nil {
			Fail(fmt.Sprintf("could not get rest clientset: %v", err))
		}
		ns = f.Namespace
		bsuDriver = driver.InitBsuCSIDriver()
	})

	It("should create a pod, write and read to it, take a volume snapshot, and create another pod from the snapshot", func() {
		pod := testsuites.PodDetails{
			// sync before taking a snapshot so that any cached data is written to the BSU volume
			Cmd: "echo 'hello world' >> /mnt/test-1/data && grep 'hello world' /mnt/test-1/data && sync",
			Volumes: []testsuites.VolumeDetails{
				{
					VolumeType: osccloud.VolumeTypeGP2,
					FSType:     bsucsidriver.FSTypeExt4,
					ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		restoredPod := testsuites.PodDetails{
			Cmd: "grep 'hello world' /mnt/test-1/data",
			Volumes: []testsuites.VolumeDetails{
				{
					VolumeType: osccloud.VolumeTypeGP2,
					FSType:     bsucsidriver.FSTypeExt4,
					ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
					VolumeMount: testsuites.VolumeMountDetails{
						NameGenerate:      "test-volume-",
						MountPathGenerate: "/mnt/test-",
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedVolumeSnapshotTest{
			CSIDriver:   bsuDriver,
			Pod:         pod,
			RestoredPod: restoredPod,
		}
		test.Run(cs, snapshotrcs, ns)
	})

	It("should create a snapshot, delete it from outside and release the snapshot", func() {
		binding := storagev1.VolumeBindingImmediate
		retain := v1.PersistentVolumeReclaimDelete
		volume := testsuites.VolumeDetails{
			VolumeType:        osccloud.VolumeTypeGP2,
			FSType:            bsucsidriver.FSTypeExt4,
			ClaimSize:         driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
			VolumeBindingMode: &binding,
			ReclaimPolicy:     &retain,
		}

		By("Create the PVC")
		tpvc, cleanups := volume.SetupDynamicPersistentVolumeClaim(cs, ns, bsuDriver)
		for i := range cleanups {
			defer cleanups[i]()
		}

		pvc := tpvc.WaitForBound()

		if os.Getenv(OSC_REGION) == "" {
			Skip(fmt.Sprintf("env %q not set", OSC_REGION))
		}

		By("Create the Snapshot")
		tvsc, cleanup := testsuites.CreateVolumeSnapshotClass(snapshotrcs, ns, bsuDriver)
		defer cleanup()
		snapshot := tvsc.CreateSnapshot(&pvc)
		defer tvsc.DeleteSnapshot(snapshot)
		tvsc.ReadyToUse(snapshot)

		By("Create the cloud")
		oscCloud, err := osccloud.NewCloudWithoutMetadata(os.Getenv(OSC_REGION))
		framework.ExpectNoError(err, "Error while creating a cloud configuration")

		By("Retrieve the snapshot")
		snap, err := oscCloud.GetSnapshotByName(context.Background(), fmt.Sprintf("snapshot-%v", snapshot.UID))
		framework.ExpectNoError(err, fmt.Sprintf("Error while retrieving snapshot %v", snapshot.UID))

		By("Deleting the snapshot")
		_, err = oscCloud.DeleteSnapshot(context.Background(), snap.SnapshotID)
		framework.ExpectNoError(err, fmt.Sprintf("Error while deleting snapshot %v", snap.SnapshotID))

		By("Keep deleting the snapshot until error")
		for i := 1; i < 100; i++ {
			_, err := oscCloud.DeleteSnapshot(context.Background(), snap.SnapshotID)
			if err == cloud.ErrNotFound {
				break
			}
			fmt.Println("Snapshot still present, waiting")
			time.Sleep(1 * time.Second)
		}

	})
})

var _ = Describe("[bsu-csi-e2e] [multi-az] Dynamic Provisioning", func() {
	f := framework.NewDefaultFramework("bsu")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		bsuDriver driver.DynamicPVTestDriver
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		bsuDriver = driver.InitBsuCSIDriver()
	})

	It("should allow for topology aware volume scheduling", func() {
		volumeBindingMode := storagev1.VolumeBindingWaitForFirstConsumer
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeType:        osccloud.VolumeTypeGP2,
						FSType:            bsucsidriver.FSTypeExt4,
						ClaimSize:         driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
						VolumeBindingMode: &volumeBindingMode,
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedTopologyAwareVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	// Requires env AWS_AVAILABILITY_ZONES, a comma separated list of AZs
	It("[env] should allow for topology aware volume with specified zone in allowedTopologies", func() {
		if os.Getenv(awsAvailabilityZonesEnv) == "" {
			Skip(fmt.Sprintf("env %q not set", awsAvailabilityZonesEnv))
		}
		allowedTopologyZones := strings.Split(os.Getenv(awsAvailabilityZonesEnv), ",")
		volumeBindingMode := storagev1.VolumeBindingWaitForFirstConsumer
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeType:            osccloud.VolumeTypeGP2,
						FSType:                bsucsidriver.FSTypeExt4,
						ClaimSize:             driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
						VolumeBindingMode:     &volumeBindingMode,
						AllowedTopologyValues: allowedTopologyZones,
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedTopologyAwareVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})
})

func restClient(group string, version string) (restclientset.Interface, error) {
	// setup rest client
	config, err := framework.LoadConfig()
	if err != nil {
		Fail(fmt.Sprintf("could not load config: %v", err))
	}
	gv := schema.GroupVersion{Group: group, Version: version}
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: serializer.NewCodecFactory(runtime.NewScheme())}
	return restclientset.RESTClientFor(config)
}

var _ = Describe("[bsu-csi-e2e] [single-az] [encryption] Dynamic Provisioning", func() {
	f := framework.NewDefaultFramework("bsu")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		bsuDriver driver.PVTestDriver
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		bsuDriver = driver.InitBsuCSIDriver()
	})

	It("should create A PV that will be encrypted", func() {

		pods := []testsuites.PodDetails{
			{
				Cmd: "mount | grep ' /mnt/test-1 ' | awk '{ print $1}' | grep  '^/dev/mapper/.*_crypt$'",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeType: osccloud.VolumeTypeGP2,
						FSType:     bsucsidriver.FSTypeExt4,
						ClaimSize:  driver.MinimumSizeForVolumeType(osccloud.VolumeTypeGP2),
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
						Encrypted:       true,
						SecretName:      "secret-1",
						SecretNamespace: ns.Name,
						Passphrase:      "ThisIsSecret",
					},
				},
			},
		}
		test := testsuites.DynamicallyProvisionedCmdVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}

		test.Run(cs, ns)
	})

})
