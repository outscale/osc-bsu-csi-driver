package e2eutils

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
	e2esvc "k8s.io/kubernetes/test/e2e/framework/service"
)

//getAnnotations return Annotations
func getAnnotations() map[string]string {
	return map[string]string{
		//Tags
		"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "testKey1=Val1,testKey2=Val2",
		//ConnectionDraining
		"service.beta.kubernetes.io/aws-load-balancer-connection-draining-enabled": "true",
		"service.beta.kubernetes.io/aws-load-balancer-connection-draining-timeout": "30",
		//ConnectionSettings
		"service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout": "65",
	}
}

//CreateSvc create an svc
func CreateSvc(client clientset.Interface, namespace *v1.Namespace, additional map[string]string) *v1.Service {
	fmt.Printf("Creating Service...\n")
	svcClient := client.CoreV1().Services(namespace.Name)

	annotations := getAnnotations()
	for k, v := range additional {
		annotations[k] = v
	}

	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "echoheaders-lb-public",
			Namespace:   namespace.Name,
			Annotations: annotations,
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
			Selector: map[string]string{
				"app": "echoheaders",
			},
			Ports: []v1.ServicePort{
				{
					Name:       "tcp",
					Protocol:   v1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
					Port:       80,
				},
			},
		},
	}

	result, err := svcClient.Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Created Svc %q.\n", result.GetObjectMeta().GetName())
	return result
}

//UpdateSvcPorts update an svc
func UpdateSvcPorts(client clientset.Interface, namespace *v1.Namespace, service *v1.Service, port v1.ServicePort) *v1.Service {
	fmt.Printf("Updating Service ports %v\n", port)
	svcClient := client.CoreV1().Services(namespace.Name)
	service.Spec.Ports = append(service.Spec.Ports, port)
	fmt.Printf("Updating Service %v\n", service)
	fmt.Printf("Updating new Service ports %v\n", service.Spec.Ports)

	result, err := svcClient.Update(context.TODO(), service, metav1.UpdateOptions{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Udpated SVC %q.\n", result.GetObjectMeta().GetName())
	return result
}

//DeleteSvc delete an svc
func DeleteSvc(client clientset.Interface, namespace *v1.Namespace, svc *v1.Service) {
	fmt.Printf("Deleting Service...")
	svcClient := client.CoreV1().Services(namespace.Name)
	err := svcClient.Delete(context.TODO(), svc.GetObjectMeta().GetName(), metav1.DeleteOptions{})

	if err != nil {
		panic(err)
	}
	fmt.Printf("Deleted Service.")
}

//ListSvc list and svc
func ListSvc(client clientset.Interface, namespace *v1.Namespace) {
	svcClient := client.CoreV1().Services(namespace.Name)
	fmt.Printf("Listing Services in namespace %q:\n", namespace.Name)
	list, err := svcClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("svc:  %v\n", list.Items)
}

//GetSvc return an svc
func GetSvc(client clientset.Interface, namespace *v1.Namespace, name string) (result *v1.Service) {
	svcClient := client.CoreV1().Services(namespace.Name)
	fmt.Printf("Getting Services in namespace %q:\n", namespace.Name)
	result, err := svcClient.Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Get Svc:  %v\n", result)
	return result
}

//WaitForSvc wait for an svc to be ready
func WaitForSvc(client clientset.Interface, namespace *v1.Namespace, svc *v1.Service) {
	e2esvc.WaitForServiceUpdatedWithFinalizer(client, namespace.Name, svc.GetObjectMeta().GetName(), true)
}

// WaitForDeletedSvc waits for an svc to be deleted
func WaitForDeletedSvc(client clientset.Interface, namespace *v1.Namespace, svc *v1.Service) {
	e2esvc.WaitForServiceDeletedWithFinalizer(client, namespace.Name, svc.GetObjectMeta().GetName())
}
