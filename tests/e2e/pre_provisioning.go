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
	"math/rand"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	osccloud "github.com/outscale/osc-bsu-csi-driver/pkg/cloud"
	bsucsidriver "github.com/outscale/osc-bsu-csi-driver/pkg/driver"
	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	"github.com/outscale/osc-bsu-csi-driver/tests/e2e/driver"
	"github.com/outscale/osc-bsu-csi-driver/tests/e2e/testsuites"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	admissionapi "k8s.io/pod-security-admission/api"
)

const (
	defaultDiskSize         = 4
	defaultVolumeType       = osccloud.VolumeTypeGP2
	awsAvailabilityZonesEnv = "AWS_AVAILABILITY_ZONES"
	dummyVolumeName         = "pre-provisioned"
)

var (
	defaultDiskSizeBytes int64 = util.GiBToBytes(defaultDiskSize)
)

var _ = Describe("[bsu-csi-e2e] [single-az] Pre-Provisioned", func() {
	f := framework.NewDefaultFramework("bsu")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs        clientset.Interface
		ns        *v1.Namespace
		bsuDriver driver.PreProvisionedVolumeTestDriver
		cloud     osccloud.Cloud
		volumeID  string
		diskSize  string
		// Set to true if the volume should be deleted automatically after test
		skipManuallyDeletingVolume bool
		cancel                     func()
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
		bsuDriver = driver.InitBsuCSIDriver()

		var availabilityZone, region string
		switch {
		case os.Getenv(awsAvailabilityZonesEnv) != "":
			availabilityZones := strings.Split(os.Getenv(awsAvailabilityZonesEnv), ",")
			availabilityZone = availabilityZones[rand.Intn(len(availabilityZones))] //nolint: gosec
			region = availabilityZone[0 : len(availabilityZone)-1]
		case os.Getenv("OSC_REGION") != "":
			region := os.Getenv("OSC_REGION")
			availabilityZone = region + "a"
		default:
			Skip(fmt.Sprintf("env %q not set", awsAvailabilityZonesEnv))
		}

		diskOptions := &osccloud.DiskOptions{
			CapacityBytes:    defaultDiskSizeBytes,
			VolumeType:       defaultVolumeType,
			AvailabilityZone: availabilityZone,
			Tags:             map[string]string{osccloud.VolumeNameTagKey: dummyVolumeName},
		}
		var err error
		cloud, err = osccloud.NewCloud(region, osccloud.WithoutMetadata())
		if err != nil {
			Fail(fmt.Sprintf("could not get NewCloud: %v", err))
		}
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		cloud.Start(ctx)
		By("Provisioning volume")
		disk, err := cloud.CreateDisk(context.Background(), "", diskOptions)
		if err != nil {
			Fail(fmt.Sprintf("could not provision a volume: %v", err))
		}
		volumeID = disk.VolumeID
		diskSize = fmt.Sprintf("%dGi", defaultDiskSize)
		By(fmt.Sprintf("Successfully provisioned volume: %q", volumeID))
	})

	AfterEach(func() {
		if !skipManuallyDeletingVolume {
			By("Waiting until volume is detached")
			err := cloud.WaitForAttachmentState(context.Background(), volumeID, "detached")
			if err != nil {
				Fail(fmt.Sprintf("could not detach volume %q: %v", volumeID, err))
			}
			By("Deleting volume")
			ok, err := cloud.DeleteDisk(context.Background(), volumeID)
			if err != nil || !ok {
				Fail(fmt.Sprintf("could not delete volume %q: %v", volumeID, err))
			}
		}
		cancel()
	})

	It("[env] should write and read to a pre-provisioned volume", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeID:  volumeID,
						FSType:    bsucsidriver.FSTypeExt4,
						ClaimSize: diskSize,
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.PreProvisionedVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	It("[env] should use a pre-provisioned volume and mount it as readOnly in a pod", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeID:  volumeID,
						FSType:    bsucsidriver.FSTypeExt4,
						ClaimSize: diskSize,
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
							ReadOnly:          true,
						},
					},
				},
			},
		}
		test := testsuites.PreProvisionedReadOnlyVolumeTest{
			CSIDriver: bsuDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})

	It(fmt.Sprintf("[env] should use a pre-provisioned volume and retain PV with reclaimPolicy %q", v1.PersistentVolumeReclaimRetain), func() {
		reclaimPolicy := v1.PersistentVolumeReclaimRetain
		volumes := []testsuites.VolumeDetails{
			{
				VolumeID:      volumeID,
				FSType:        bsucsidriver.FSTypeExt4,
				ClaimSize:     diskSize,
				ReclaimPolicy: &reclaimPolicy,
			},
		}
		test := testsuites.PreProvisionedReclaimPolicyTest{
			CSIDriver: bsuDriver,
			Volumes:   volumes,
		}
		test.Run(cs, ns)
	})

	It(fmt.Sprintf("[env] should use a pre-provisioned volume and delete PV with reclaimPolicy %q", v1.PersistentVolumeReclaimDelete), func() {
		reclaimPolicy := v1.PersistentVolumeReclaimDelete
		skipManuallyDeletingVolume = true
		volumes := []testsuites.VolumeDetails{
			{
				VolumeID:      volumeID,
				FSType:        bsucsidriver.FSTypeExt4,
				ClaimSize:     diskSize,
				ReclaimPolicy: &reclaimPolicy,
			},
		}
		test := testsuites.PreProvisionedReclaimPolicyTest{
			CSIDriver: bsuDriver,
			Volumes:   volumes,
		}
		test.Run(cs, ns)
	})
})
