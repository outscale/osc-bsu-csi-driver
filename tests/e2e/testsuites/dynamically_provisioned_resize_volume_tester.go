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
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	"github.com/outscale/osc-bsu-csi-driver/tests/e2e/driver"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epodoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"

	. "github.com/onsi/ginkgo/v2"
	clientset "k8s.io/client-go/kubernetes"
)

// DynamicallyProvisionedResizeVolumeTest will provision required StorageClass(es), PVC(s) and Pod(s)
// Waiting for the PV provisioner to create a new PV
// Update pvc storage size
// Waiting for new PVC and PV to be ready
// And finally attach pvc to the pod and wait for pod to be ready.
type DynamicallyProvisionedResizeVolumeTest struct {
	CSIDriver driver.DynamicPVTestDriver
	Pod       PodDetails
	Online    bool
}

func (t *DynamicallyProvisionedResizeVolumeTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	volume := t.Pod.Volumes[0]
	tpvc, _ := volume.SetupDynamicPersistentVolumeClaim(client, namespace, t.CSIDriver)
	defer tpvc.Cleanup()

	pvcName := tpvc.persistentVolumeClaim.Name
	pvc, _ := client.CoreV1().PersistentVolumeClaims(namespace.Name).Get(context.TODO(), pvcName, metav1.GetOptions{})
	By(fmt.Sprintf("Get pvc name: %v", pvc.Name))

	var tpod *TestPod
	if t.Online {
		By("Validate volume can be attached")
		tpod = NewTestPod(client, namespace, t.Pod.Cmd)

		tpod.SetupVolume(tpvc.persistentVolumeClaim, volume.VolumeMount.NameGenerate+"1", volume.VolumeMount.MountPathGenerate+"1", volume.VolumeMount.ReadOnly)

		By("deploying the pod")
		tpod.Create()
		By("checking that the pod is running")
		tpod.WaitForRunning()

		defer tpod.Cleanup()
	}

	newSize := pvc.Spec.Resources.Requests["storage"]
	delta := resource.Quantity{}
	delta.Set(util.GiBToBytes(3))
	newSize.Add(delta)
	pvc.Spec.Resources.Requests["storage"] = newSize

	By("resizing the pvc")
	updatedPvc, err := client.CoreV1().PersistentVolumeClaims(namespace.Name).Update(context.TODO(), pvc, metav1.UpdateOptions{})
	if err != nil {
		framework.ExpectNoError(err, fmt.Sprintf("fail to resize pvc(%s): %v", pvcName, err))
	}
	updatedSize := updatedPvc.Spec.Resources.Requests["storage"]

	By("waiting for the resize")
	err = waitForPvToResize(client, namespace, updatedPvc.Spec.VolumeName, updatedSize, 5*time.Minute, 10*time.Second)
	framework.ExpectNoError(err)

	if t.Online {
		err = waitForMountToResize(tpod, updatedSize, 5*time.Minute, 30*time.Second)
		framework.ExpectNoError(err)
	}

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

// waitForPvToResize waiting for pvc size to be resized to desired size
func waitForPvToResize(c clientset.Interface, ns *v1.Namespace, pvName string, desiredSize resource.Quantity, timeout time.Duration, interval time.Duration) error {
	By(fmt.Sprintf("Waiting up to %v for pv to be resized", timeout))
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(interval) {
		newPv, _ := c.CoreV1().PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
		newPvSize := newPv.Spec.Capacity["storage"]
		_, _ = fmt.Fprintf(GinkgoWriter, "storage capacity: %v\n", newPvSize.String())
		if desiredSize.Equal(newPvSize) {
			By("PV size is updated")
			return nil
		}
	}
	return fmt.Errorf("gave up waiting for pv %q to resize", pvName)
}

func waitForMountToResize(tpod *TestPod, desiredSize resource.Quantity, timeout time.Duration, interval time.Duration) error {
	By(fmt.Sprintf("Waiting up to %v for mount to be resized", timeout))
	sz, _ := desiredSize.AsInt64()
	szG := util.BytesToGiB(sz)
	_, _ = fmt.Fprintf(GinkgoWriter, "expected size %dGB\n", szG)
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(interval) {
		res, err := e2epodoutput.LookForStringInPodExec(tpod.pod.Namespace, tpod.pod.Name, []string{"df", "-m", "/mnt/test-1"}, "/mnt/test-", execTimeout)
		framework.ExpectNoError(err)
		blocks, err := strconv.Atoi(strings.Fields(res)[8])
		_, _ = fmt.Fprintf(GinkgoWriter, "FS size: %dMB\n", blocks)
		framework.ExpectNoError(err)
		if int32(math.Round(float64(blocks)/1024)) == szG {
			By(fmt.Sprintf("Mount is updated to %dMB", blocks))
			return nil
		}
	}
	return errors.New("gave up waiting for mount to resize")
}
