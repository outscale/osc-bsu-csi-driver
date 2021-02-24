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

	"github.com/outscale-dev/cloud-provider-osc/tests/e2e/utils"

	elbApi "github.com/aws/aws-sdk-go/service/elb"
	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2esvc "k8s.io/kubernetes/test/e2e/framework/service"
)

var _ = ginkgo.Describe("[ccm-e2e] Simple-LB", func() {
	f := framework.NewDefaultFramework("ccm")

	var (
		cs clientset.Interface
		ns *v1.Namespace
	)

	ginkgo.BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace
	})
	//This example creates a deployment named echoheaders on your cluster, which will run a single replica
	//of the echoserver container, listening on port 8080.
	//Then create a Service that exposes our new application to the Internet over an Outscale Load Balancer unit (LBU).

	ginkgo.It("Create Simple LB", func() {
		fmt.Printf("Create Simple LB :  %v\n", ns)

		ginkgo.By("Create Deployment")
		deployement := e2eutils.CreateDeployment(cs, ns)
		defer e2eutils.DeleteDeployment(cs, ns, deployement)
		defer e2eutils.ListDeployment(cs, ns)

		ginkgo.By("checking that the pod is running")
		e2eutils.WaitForDeployementReady(cs, ns, deployement)

		ginkgo.By("listDeployment")
		e2eutils.ListDeployment(cs, ns)

		ginkgo.By("Create Svc")
		svc := e2eutils.CreateSvc(cs, ns)
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
		for count < 3 {
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
		ginkgo.By("Test Connection wait to have endpoint ready")
		time.Sleep(60 * time.Second)
		e2esvc.TestReachableHTTP(address, 80, 60*time.Second)

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
			TargetPort: intstr.FromInt(8081),
			Port:       81,
		}
		svc = e2eutils.UpdateSvcPorts(cs, ns, updatedSvc, port)
		fmt.Printf("svc updated:  %v\n", svc)

		ginkgo.By("Test Connection wait to have endpoint ready")
		time.Sleep(60 * time.Second)
		lb, err = e2eutils.GetLb(elb, lbName)
		framework.ExpectNoError(err)
		framework.ExpectNotEqual(len(lb.Instances), 0)

		ginkgo.By("TestReachableHTTP after update")
		e2esvc.TestReachableHTTP(address, 80, 60*time.Second)
	})
})
