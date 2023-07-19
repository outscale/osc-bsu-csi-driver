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
	"fmt"
	"strings"
	"time"

	e2eutils "github.com/outscale-dev/cloud-provider-osc/tests/e2e/utils"

	elbApi "github.com/aws/aws-sdk-go/service/elb"
	"github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2esvc "k8s.io/kubernetes/test/e2e/framework/service"
	admissionapi "k8s.io/pod-security-admission/api"
)

// TestParam customize e2e tests and lb annotations
type TestParam struct {
	Title       string
	Annotations map[string]string
	Cmd         string
}

var _ = ginkgo.Describe("[ccm-e2e] SVC-LB", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs clientset.Interface
		ns *v1.Namespace
	)

	ginkgo.BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
	})

	params := []TestParam{
		{
			Title:       "Create LB",
			Cmd:         "",
			Annotations: map[string]string{},
		},
		{
			Title: "Create LB With proxy protocol enabled",
			Cmd: "sed -i 's/listen 8080 default_server reuseport/listen 8080 default_server reuseport proxy_protocol/g' /etc/nginx/nginx.conf; " +
				"sed -i 's/listen 8443 default_server ssl http2 reuseport/listen 8443 default_server ssl http2 reuseport proxy_protocol/g' /etc/nginx/nginx.conf ; " +
				"/usr/local/bin/run.sh",
			Annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-proxy-protocol": "*",
			},
		},
		{
			Title: "Create LB with hc customized",
			Cmd:   "",
			Annotations: map[string]string{
				"service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold":   "3",
				"service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold": "7",
				"service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout":             "6",
				"service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval":            "11",
			},
		},
	}

	for _, param := range params {
		title := param.Title
		cmd := param.Cmd
		annotations := param.Annotations
		ginkgo.It(title, func() {
			fmt.Printf("Create Simple LB :  %v\n", ns)
			fmt.Printf("Cs :  %v\n", cs)
			fmt.Printf("Params :  %v / %v / %v\n", title, cmd, annotations)

			ginkgo.By("Create Deployment")
			deployement := e2eutils.CreateDeployment(cs, ns, cmd)
			defer e2eutils.DeleteDeployment(cs, ns, deployement)
			defer e2eutils.ListDeployment(cs, ns)

			ginkgo.By("checking that the pod is running")
			e2eutils.WaitForDeployementReady(cs, ns, deployement)

			ginkgo.By("listDeployment")
			e2eutils.ListDeployment(cs, ns)

			ginkgo.By("Create Svc")
			svc := e2eutils.CreateSvc(cs, ns, annotations)
			fmt.Printf("Created Service %q.\n", svc)
			defer e2eutils.ListSvc(cs, ns)
			defer e2eutils.DeleteSvc(cs, ns, svc)

			ginkgo.By("checking that the svc is ready")
			e2eutils.WaitForSvc(cs, ns, svc)

			ginkgo.By("Listing svc")
			e2eutils.ListSvc(cs, ns)

			ginkgo.By("Get Updated svc")
			count := 0
			var updatedSvc *v1.Service
			for count < 10 {
				updatedSvc = e2eutils.GetSvc(cs, ns, svc.GetObjectMeta().GetName())
				fmt.Printf("Ingress:  %v\n", updatedSvc.Status.LoadBalancer.Ingress)
				if len(updatedSvc.Status.LoadBalancer.Ingress) > 0 {
					break
				}
				count++
				time.Sleep(30 * time.Second)
			}
			address := updatedSvc.Status.LoadBalancer.Ingress[0].Hostname
			lbName := strings.Split(address, "-")[0]
			fmt.Printf("address:  %v\n", address)

			ginkgo.By("Test Connection (wait to have endpoint ready)")
			e2esvc.TestReachableHTTP(address, 80, 600*time.Second)

			ginkgo.By("Remove Instances from lbu")
			elb, err := e2eutils.ElbAPI()
			framework.ExpectNoError(err)

			lb, err := e2eutils.GetLb(elb, lbName)
			framework.ExpectNoError(err)

			lbInstances := []*elbApi.Instance{}
			for _, lbInstance := range lb.Instances {
				lbInstanceItem := &elbApi.Instance{}
				lbInstanceItem.InstanceId = lbInstance.InstanceId
				lbInstances = append(lbInstances, lbInstanceItem)
			}
			framework.ExpectNotEqual(len(lbInstances), 0)

			err = e2eutils.RemoveLbInst(elb, lbName, lbInstances)
			framework.ExpectNoError(err)

			lb, err = e2eutils.GetLb(elb, lbName)
			framework.ExpectNoError(err)
			framework.ExpectEqual(len(lb.Instances), 0)

			ginkgo.By("Add port to force update of LB")
			port := v1.ServicePort{
				Name:       "tcp2",
				Protocol:   v1.ProtocolTCP,
				TargetPort: intstr.FromInt(8443),
				Port:       443,
			}
			svc = e2eutils.UpdateSvcPorts(cs, ns, updatedSvc, port)
			fmt.Printf("svc updated:  %v\n", svc)

			ginkgo.By("Test LB updated(wait to have vm registred)")
			count = 0
			for count < 10 {
				lb, err = e2eutils.GetLb(elb, lbName)
				if err == nil && len(lb.Instances) != 0 {
					break
				}
				count++
				time.Sleep(30 * time.Second)
			}
			lb, err = e2eutils.GetLb(elb, lbName)

			framework.ExpectNoError(err)
			framework.ExpectNotEqual(len(lb.Instances), 0)

			ginkgo.By("TestReachableHTTP after update")
			e2esvc.TestReachableHTTP(address, 80, 240*time.Second)
		})

	}
})

// Test to check that the issue 68 is solved
var _ = ginkgo.Describe("[ccm-e2e] SVC-LB", func() {
	f := framework.NewDefaultFramework("ccm")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	var (
		cs clientset.Interface
	)

	ginkgo.BeforeEach(func() {
		cs = f.ClientSet
	})

	title := "Issue #68"
	cmd := ""
	annotations := map[string]string{}
	ginkgo.It(title, func() {
		nsSvc1, err := f.CreateNamespace("svc1", map[string]string{})
		framework.ExpectNoError(err)
		nsSvc2, err := f.CreateNamespace("svc2", map[string]string{})
		framework.ExpectNoError(err)

		fmt.Printf("Params :  %v / %v / %v\n", title, cmd, annotations)

		ginkgo.By("Create Deployment 1")
		deployementSvc1 := e2eutils.CreateDeployment(cs, nsSvc1, cmd)
		defer e2eutils.DeleteDeployment(cs, nsSvc1, deployementSvc1)
		defer e2eutils.ListDeployment(cs, nsSvc1)

		ginkgo.By("Create Deployment 2")
		deployementSvc2 := e2eutils.CreateDeployment(cs, nsSvc2, cmd)
		defer e2eutils.DeleteDeployment(cs, nsSvc1, deployementSvc2)
		defer e2eutils.ListDeployment(cs, nsSvc1)

		ginkgo.By("checking that pods are running")
		e2eutils.WaitForDeployementReady(cs, nsSvc1, deployementSvc1)
		e2eutils.WaitForDeployementReady(cs, nsSvc2, deployementSvc2)

		ginkgo.By("listDeployment")
		e2eutils.ListDeployment(cs, nsSvc1)
		e2eutils.ListDeployment(cs, nsSvc2)

		ginkgo.By("Create Svc 1")
		svc1 := e2eutils.CreateSvc(cs, nsSvc1, annotations)
		fmt.Printf("Created Service %q.\n", svc1)
		defer e2eutils.ListSvc(cs, nsSvc1)

		ginkgo.By("Create Svc 2")
		svc2 := e2eutils.CreateSvc(cs, nsSvc2, annotations)
		fmt.Printf("Created Service %q.\n", svc2)
		defer e2eutils.ListSvc(cs, nsSvc2)
		defer e2eutils.DeleteSvc(cs, nsSvc2, svc2)

		ginkgo.By("checking that svc are ready")
		e2eutils.WaitForSvc(cs, nsSvc1, svc1)
		e2eutils.WaitForSvc(cs, nsSvc2, svc2)

		ginkgo.By("Listing svc")
		e2eutils.ListSvc(cs, nsSvc1)
		e2eutils.ListSvc(cs, nsSvc2)

		ginkgo.By("Get Updated svc")
		addresses := [2]string{}
		lbNames := [2]string{}
		nss := []*v1.Namespace{nsSvc1, nsSvc2}
		svcs := []*v1.Service{svc1, svc2}
		for i := 0; i < 2; i++ {
			count := 0
			var updatedSvc *v1.Service
			for count < 10 {
				updatedSvc = e2eutils.GetSvc(cs, nss[i], svcs[i].GetObjectMeta().GetName())
				fmt.Printf("Ingress:  %v\n", updatedSvc.Status.LoadBalancer.Ingress)
				if len(updatedSvc.Status.LoadBalancer.Ingress) > 0 {
					break
				}
				count++
				time.Sleep(30 * time.Second)
			}

			addresses[i] = updatedSvc.Status.LoadBalancer.Ingress[0].Hostname
			lbNames[i] = strings.Split(addresses[i], "-")[0]
			fmt.Printf("address:  %v\n", addresses[i])
		}

		ginkgo.By("Test Connection (wait to have endpoint ready)")
		for i := 0; i < 2; i++ {
			e2esvc.TestReachableHTTP(addresses[i], 80, 600*time.Second)
		}

		ginkgo.By("Remove SVC 1")
		e2eutils.DeleteSvc(cs, nsSvc1, svc1)

		e2eutils.WaitForDeletedSvc(cs, nsSvc1, svc1)

		fmt.Printf("Wait to have stable sg")
		time.Sleep(120 * time.Second)

		ginkgo.By("Test SVC 2")
		e2esvc.TestReachableHTTP(addresses[1], 80, 240*time.Second)

	})

})
