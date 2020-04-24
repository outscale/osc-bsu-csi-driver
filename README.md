**WARNING**: This driver is currently in Beta release and should not be used in performance critical applications.

# Cloud Provider 3DS Outscale CCM (cloud-provider-osc)
The OSC cloud provider provides the interface between a Kubernetes cluster and 3DS Outscale service APIs. 
This project allows a Kubernetes cluster to provision, monitor and remove AWS resources necessary for operation of the cluster.

# Cloud Provider 3DS OSC CCM on Kubernetes

## Requirements
* Golang 1.13.7+
* Docker 18.09.2+ 
* K8s v1.16.4+

## Build image

``` 
	make build-image  IMAGE=osc/cloud-provider-osc IMAGE_VERSION=version
	make tag-image	  IMAGE=osc/cloud-provider-osc IMAGE_VERSION=version REGISTRY=registry.hub 
	make push-release IMAGE=osc/cloud-provider-osc IMAGE_VERSION=version REGISTRY=registry.hub 
``` 


## Container Images:
|OSC EBS CSI Driver Version | Image                                     |
|---------------------------|-------------------------------------------|
|OSC-MIGRATION branch       |outscale/cloud-provider-osc:v0.0.0beta     |


## Flags
The flag `--cloud-provider=external` `must` be passed to kubelet, kube-apiserver, and kube-controller-manager.
You  **must** pass the --cloud-provider flag to `osc-cloud-controller-manager`.


## Installation
Please go through [DEPLOY](./deploy/README.md)


## Prerequisites Kubernetes cluster

- The k8s cluster used for development and tests is a pre-installed k8s platform under outscale cloud with 3 masters and 2 workers on vm with `t2.medium` type, this VMs are running on a VPC
- Tests has been done also using a k8s cluster **outside** a VPC using RKE(v0.2.10) with 1 master and 1 worker on vm with `t2.medium` type. Cloud provider OSC has been adapted to run in such config. 

### Prerequisites for 'all' k8s cluster cloud resources
- You **must** set a clusterID to be used for tagging all ressources
- You Must **tag** all cluster resources (VPC, Instances, SG, subnets, route tables ....)  with the following tag
	* The tag key = `OscK8sClusterID/clusterID`
	* The tag value is an ownership value with the following possible values 
    	- "shared": resource is shared between multiple clusters, and should not be destroyed
     	- "owned": the resource is considered owned and managed by the cluster
	* example of tag
```     
	{
		"key": "OscK8sClusterID/k8s-dev-ccm",
		"value": "shared"
 	}
```
### Prerequisites for 'Instances'
- You Must Tag all All cluster nodes with the following tag :
	* Tag key `OscK8sNodeName`
	* Tag values must be the k8s host name `kubernetes.io/hostname`
	
```     
	{
		"key": "OscK8sNodeName",
		"value": "the value of kubernetes.io/hostname"
	}
```
 
### Prerequisites for pre-created 'SG'
 > **If** you want to use a pre-created `sg` to be applied to be attached/associated to the LBU 
   it must be Tagged with `OscK8sMainSG/clusterID` and setted to `True`
	
```     
	{
		"key": "OscK8sMainSG/k8s-dev-ccm",
		"value": "True"
	}
```
 > **Else** an LB will be created automatically and attached to all Nodes




## Examples
- [simple-lb](./examples/simple-lb)
- [2048](./examples/2048)
- [simple-internal](./examples/simple-internal)
- [advanced-lb](./examples/advanced-lb)


## Note
* All the EBS volume plugin related logic will be in maintenance mode. For new feature request or bug fixes, please create issue or pull request in [EBS CSI Driver](https://github.com/outscale-dev/osc-ebs-csi-driver)
