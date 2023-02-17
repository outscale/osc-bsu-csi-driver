package e2eutils

import (
	"context"
	"fmt"

	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2edeployment "k8s.io/kubernetes/test/e2e/framework/deployment"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
)

func int32Ptr(i int32) *int32 {
	return &i
}

// ListDeployment list deployement
func ListDeployment(client clientset.Interface, namespace *v1.Namespace) {
	deploymentsClient := client.AppsV1().Deployments(namespace.Name)
	fmt.Printf("Listing deployments in namespace %q:\n", namespace.Name)
	list, err := deploymentsClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err)
	}
	for _, d := range list.Items {
		fmt.Printf(" * %s (%d replicas)\n", d.Name, *d.Spec.Replicas)
	}
}

// CreateDeployment create a deployement
func CreateDeployment(client clientset.Interface, namespace *v1.Namespace, cmd string) *apps.Deployment {
	deploymentsClient := client.AppsV1().Deployments(namespace.Name)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "echoheaders",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "echoheaders",
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "echoheaders",
					},
				},
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Name:  "echoheaders",
							Image: "gcr.io/google_containers/echoserver:1.10",
							Ports: []apiv1.ContainerPort{
								{
									Name:          "tcp",
									Protocol:      apiv1.ProtocolTCP,
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
		},
	}

	if len(cmd) > 0 {
		deployment.Spec.Template.Spec.Containers[0].Command = []string{"/bin/sh"}
		deployment.Spec.Template.Spec.Containers[0].Args = []string{"-c", cmd}
	}

	// Create Deployment
	fmt.Printf("Creating deployment...\n")
	result, err := deploymentsClient.Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Created deployment %q.\n", result.GetObjectMeta().GetName())
	return result
}

// DeleteDeployment delete a Deployment
func DeleteDeployment(client clientset.Interface, namespace *v1.Namespace, deployment *apps.Deployment) {
	deploymentsClient := client.AppsV1().Deployments(namespace.Name)
	deletePolicy := metav1.DeletePropagationForeground
	if err := deploymentsClient.Delete(context.TODO(), deployment.GetObjectMeta().GetName(), metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		panic(err)
	}
	fmt.Printf("Deleted deployment.\n")
}

// WaitForDeployementReady wait for a Deployement
func WaitForDeployementReady(client clientset.Interface, namespace *v1.Namespace, deployment *apps.Deployment) {
	err := e2edeployment.WaitForDeploymentComplete(client, deployment)
	framework.ExpectNoError(err)

	pods, err := e2edeployment.GetPodsForDeployment(client, deployment)
	framework.ExpectNoError(err)
	fmt.Printf("pods:  %v\n", len(pods.Items))
	// always get first pod as there should only be one
	pod := pods.Items[0]
	err = e2epod.WaitForPodRunningInNamespace(client, &pod)
	framework.ExpectNoError(err)
}
