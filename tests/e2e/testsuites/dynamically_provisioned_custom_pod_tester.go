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
	"fmt"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	"github.com/outscale-dev/osc-bsu-csi-driver/tests/e2e/driver"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2edeployment "k8s.io/kubernetes/test/e2e/framework/deployment"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
)

type DynamicallyProvisionedCustomPodTest struct {
	CSIDriver driver.DynamicPVTestDriver
	Pod       PodDetails
	PodCmds   []PodCmds
}

type PodCmds struct {
	Cmd            []string
	ExpectedString string
}

func customizePod(customizePod []string, deployment *TestDeployment) *TestDeployment {
	podSc := v1.PodSecurityContext{}
	sc := v1.SecurityContext{}
	for _, custom := range customizePod {
		fmt.Printf("custom: %+v\n", custom)
		err := podSc.Unmarshal([]byte(custom))
		if err == nil {
			fmt.Printf("add PodSecurityContext\n")
			deployment.deployment.Spec.Template.Spec.SecurityContext = &podSc
		} else {
			err := sc.Unmarshal([]byte(custom))
			if err == nil {
				fmt.Printf("add SecurityContext\n")
				deployment.deployment.Spec.Template.Spec.Containers[0].SecurityContext = &sc
			} else {
				fmt.Printf("ignore custom: %+v\n", custom)
			}
		}
	}
	return deployment
}

func (t *DynamicallyProvisionedCustomPodTest) Run(client clientset.Interface, namespace *v1.Namespace, f *framework.Framework) {
	customImage := "busybox"
	tDeployment, cleanup := t.Pod.SetupDeployment(client, namespace, t.CSIDriver, customImage)
	// defer must be called here for resources not get removed before using them
	for i := range cleanup {
		defer cleanup[i]()
	}

	By("customize Pod Deployment")
	fmt.Printf("Before tDeployment: %+v\n", tDeployment)
	customizePod(t.Pod.CustomizedPod, tDeployment)
	fmt.Printf("After tDeployment: %+v\n", tDeployment)

	By("deploying the deployment")
	tDeployment.Create()

	By("checking that the pod is running")
	tDeployment.WaitForPodReady()

	pods, err := e2edeployment.GetPodsForDeployment(client, tDeployment.deployment)
	framework.ExpectNoError(err)
	singleSpacePattern := regexp.MustCompile(`\s+`)
	for _, podCmd := range t.PodCmds {
		By(fmt.Sprintf("Extended pod and volumes checks: %v", podCmd.Cmd))
		stdout, stderr, err := e2epod.ExecCommandInContainerWithFullOutput(f, tDeployment.podName, pods.Items[0].Spec.Containers[0].Name, podCmd.Cmd...)
		fmt.Printf("stdout %v, stderr %v, err %v\n", stdout, stderr, err)
		if err != nil {
			panic(err.Error())
		}
		framework.ExpectEqual(singleSpacePattern.ReplaceAllString(stdout, " "), podCmd.ExpectedString)
	}
}
