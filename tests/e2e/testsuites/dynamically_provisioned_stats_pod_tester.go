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
	"bufio"
	"context"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"

	"github.com/outscale/osc-bsu-csi-driver/tests/e2e/driver"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"

	e2edeployment "k8s.io/kubernetes/test/e2e/framework/deployment"
)

var reDf = regexp.MustCompile(`[^\s]+\s+([0-9]+)\s+([0-9]+)\s+([0-9]+)`)

func getDf(data string) string {
	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Scan()
	if !scanner.Scan() {
		return ""
	}
	subm := reDf.FindStringSubmatch(scanner.Text())
	if len(subm) == 0 {
		return ""
	}
	return strings.Join(subm[1:], " ")
}

func getMetrics(data string, ns string, pvc string) string {
	scanner := bufio.NewScanner(strings.NewReader(data))
	// The target is to find and get data from following lines
	// kubelet_volume_stats_available_bytes{namespace="dynamic-p",persistentvolumeclaim="ebs-claim"} 4.12649472e+09
	// kubelet_volume_stats_capacity_bytes{namespace="dynamic-p",persistentvolumeclaim="ebs-claim"} 4.160421888e+09
	// kubelet_volume_stats_used_bytes{namespace="dynamic-p",persistentvolumeclaim="ebs-claim"} 1.7149952e+07
	// kubelet_volume_stats_inodes{namespace="dynamic-p",persistentvolumeclaim="ebs-claim"} 262144
	// kubelet_volume_stats_inodes_free{namespace="dynamic-p",persistentvolumeclaim="ebs-claim"} 262132
	// kubelet_volume_stats_inodes_used{namespace="dynamic-p",persistentvolumeclaim="ebs-claim"} 12
	KUBELET_VOLUME_PREFIX := "kubelet_volume_stats_"
	var kubelet_volume_stats_available_bytes,
		kubelet_volume_stats_capacity_bytes,
		kubelet_volume_stats_used_bytes string
	stop := 0
	for scanner.Scan() {
		text := scanner.Text()
		if stop == 3 {
			return fmt.Sprintf("%s %s %s",
				kubelet_volume_stats_capacity_bytes,
				kubelet_volume_stats_used_bytes,
				kubelet_volume_stats_available_bytes)
		}
		if strings.HasPrefix(text, KUBELET_VOLUME_PREFIX) &&
			strings.Contains(text, "namespace=\""+ns+"\"") &&
			strings.Contains(text, "persistentvolumeclaim=\""+pvc+"\"") {
			fields := strings.Split(text, "}")
			if len(fields) > 1 {
				value_str := strings.TrimSpace(strings.Split(text, "}")[1])
				flt, _, err := big.ParseFloat(value_str, 10, 0, big.ToNearestEven)
				if err != nil {
					panic(err)
				}

				value, _ := flt.Int(nil)

				switch {
				case strings.Contains(text, "kubelet_volume_stats_available_bytes"):
					kubelet_volume_stats_available_bytes = fmt.Sprintf("%d", value)
					stop++
				case strings.Contains(text, "kubelet_volume_stats_capacity_bytes"):
					kubelet_volume_stats_capacity_bytes = fmt.Sprintf("%d", value)
					stop++
				case strings.Contains(text, "kubelet_volume_stats_used_bytes"):
					kubelet_volume_stats_used_bytes = fmt.Sprintf("%d", value)
					stop++
				}
			}
		}
	}
	return ""
}

type DynamicallyProvisionedStatsPodTest struct {
	CSIDriver driver.DynamicPVTestDriver
	Pod       PodDetails
}

func (t *DynamicallyProvisionedStatsPodTest) Run(client clientset.Interface, namespace *v1.Namespace, f *framework.Framework) {
	customImage := "busybox"
	tDeployment, cleanup := t.Pod.SetupDeployment(client, namespace, t.CSIDriver, customImage)
	// defer must be called here for resources not get removed before using them
	for i := range cleanup {
		defer cleanup[i]()
	}

	By("deploying the deployment")
	tDeployment.Create()

	By("checking that the pod is running")
	tDeployment.WaitForPodReady()

	pods, err := e2edeployment.GetPodsForDeployment(context.Background(), client, tDeployment.deployment)
	framework.ExpectNoError(err)

	pod_host_ip := pods.Items[0].Status.HostIP
	pvc_ns := tDeployment.namespace.Name
	pvc_name := tDeployment.deployment.Spec.Template.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName

	By("checking volume stats using /metrics ")
	metrics_kubelet_volume_stats := ""
	df_stats := ""
	for i := 0; i < 20; i++ {
		// Retrieve stats using /metrics
		cmd := []string{
			"wget",
			"-qO-",
			fmt.Sprintf("http://%s:10255/metrics", pod_host_ip),
		}
		metricsStdout, metricsStderr, metricsErr := e2epod.ExecCommandInContainerWithFullOutput(f, tDeployment.podName, pods.Items[0].Spec.Containers[0].Name, cmd...)
		// fmt.Printf("Metrics: stdout %v, stderr %v, err %v\n", metricsStdout, metricsStderr, metricsErr)
		fmt.Printf("Metrics: stderr %v, err %v\n", metricsStderr, metricsErr)

		// Retrieve stats using df
		dfCmd := []string{
			"df",
			"-B1",
			"/mnt/test-1",
		}
		dfStdout, dfStderr, dfErr := e2epod.ExecCommandInContainerWithFullOutput(f, tDeployment.podName, pods.Items[0].Spec.Containers[0].Name, dfCmd...)
		fmt.Printf("df: stderr %v, err %v\n", dfStderr, dfErr)

		if dfErr != nil || metricsErr != nil {
			panic("Unable to retrieve the metrics")
		}

		// Process the output
		df_stats = getDf(dfStdout)
		metrics_kubelet_volume_stats = getMetrics(metricsStdout, pvc_ns, pvc_name)

		fmt.Printf("df_stats %q\n", df_stats)
		fmt.Printf("metrics_kubelet_volume_stats %q\n", metrics_kubelet_volume_stats)

		// Check equality
		if df_stats == metrics_kubelet_volume_stats {
			return
		}

		time.Sleep(10 * time.Second)
	}

	panic("Timeout, did not got the same stats")
}
