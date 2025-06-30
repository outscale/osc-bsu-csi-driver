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

package testsuites

import (
	"context"
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint
	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud"
	"github.com/outscale/osc-bsu-csi-driver/tests/e2e/driver"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

// DynamicallyProvisionedModifyVolumeTest will provision required StorageClass(es), PVC(s) and Pod(s)
// Waiting for the PV provisioner to create a new PV
// Update pvc storage size
// Waiting for new PVC and PV to be ready
// And finally attach pvc to the pod and wait for pod to be ready.
type DynamicallyProvisionedModifyVolumeTest struct {
	CSIDriver driver.DynamicPVTestDriver
	Pod       PodDetails
	Cloud     cloud.Cloud
	Online    bool
}

func (t *DynamicallyProvisionedModifyVolumeTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	volume := t.Pod.Volumes[0]
	baseType := "gp2"
	baseIops := "100"
	tvac, _ := volume.SetupVolumeAttributesClass(client, namespace, volume.VolumeAttributeClass, baseType, baseIops, t.CSIDriver)
	defer tvac.Cleanup()
	tpvc, _ := volume.SetupDynamicPersistentVolumeClaim(client, namespace, t.CSIDriver)
	defer tpvc.Cleanup()

	pvcName := tpvc.persistentVolumeClaim.Name
	pvc, _ := client.CoreV1().PersistentVolumeClaims(namespace.Name).Get(context.TODO(), pvcName, metav1.GetOptions{})

	if t.Online {
		By("Validate volume can be attached")
		tpod := NewTestPod(client, namespace, t.Pod.Cmd)

		tpod.SetupVolume(tpvc.persistentVolumeClaim, volume.VolumeMount.NameGenerate+"1", volume.VolumeMount.MountPathGenerate+"1", volume.VolumeMount.ReadOnly)

		By("deploying the pod")
		tpod.Create()
		By("checking that the pod is running")
		tpod.WaitForRunning()

		defer tpod.Cleanup()
	}

	By("updating the VolumeAttributesClass")
	updatedName := volume.VolumeAttributeClass + "-new"
	updatedType := "io1"
	updatedIops := "200"
	ntvac, _ := volume.SetupVolumeAttributesClass(client, namespace, updatedName, updatedType, updatedIops, t.CSIDriver)
	defer ntvac.Cleanup()
	pvc.Spec.VolumeAttributesClassName = &updatedName
	_, err := client.CoreV1().PersistentVolumeClaims(namespace.Name).Update(context.TODO(), pvc, metav1.UpdateOptions{})
	if err != nil {
		framework.ExpectNoError(err, fmt.Sprintf("fail to Modify PVC(%s): %v", pvc.Name, err))
	}

	err = t.WaitForPvToModify(client, namespace, pvc.Spec.VolumeName, updatedType, updatedIops, 10*time.Minute, 10*time.Second)
	framework.ExpectNoError(err)

	if !t.Online {
		By("Validate volume can be attached")
		tpod := NewTestPod(client, namespace, t.Pod.Cmd)

		tpod.SetupVolume(tpvc.persistentVolumeClaim, volume.VolumeMount.NameGenerate+"1", volume.VolumeMount.MountPathGenerate+"1", volume.VolumeMount.ReadOnly)

		By("deploying the pod")
		tpod.Create()
		By("checking that the pod is successful")
		tpod.WaitForSuccess()

		defer tpod.Cleanup()
	}
}

// WaitForPvToModify waiting for pvc size to be Modifyd to desired size
func (t *DynamicallyProvisionedModifyVolumeTest) WaitForPvToModify(client clientset.Interface, ns *v1.Namespace, pvName string, desiredType, desiredIops string, timeout time.Duration, interval time.Duration) error {
	By(fmt.Sprintf("Waiting up to %v for pv in namespace %q to be complete", timeout, ns.Name))
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(interval) {
		newPv, err := client.CoreV1().PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
		if err != nil {
			continue
		}
		if newPv.Spec.CSI != nil {
			dsk, err := t.Cloud.GetDiskByID(context.TODO(), newPv.Spec.CSI.VolumeHandle)
			if err != nil {
				continue
			}
			By(fmt.Sprintf("volumeType %q iops %d", dsk.VolumeType, dsk.IOPSPerGB))
			if dsk.VolumeType != desiredType || strconv.FormatInt(int64(dsk.IOPSPerGB), 10) != desiredIops {
				continue
			}
			return nil
		}
	}
	return fmt.Errorf("gave up after waiting %v for pv %q to update", timeout, pvName)
}
