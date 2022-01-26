
**WARNING**: This driver is currently in Beta release and should not be used in performance critical applications.

# Outscale Block Storage Unit (BSU) CSI driver

## Overview

The Outscale Block Storage Unit Container Storage Interface (CSI) Driver provides a [CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md) interface used by Container Orchestrators to manage the lifecycle of 3DS outscale BSU volumes.

## CSI Specification Compability Matrix

| OSC BSU CSI Driver \ CSI Version       | v1.3.0 | v1.5.0 |
|----------------------------------------|--------|--------|
| <=  v0.0.14beta                        | yes    | no     |
| v0.0.15                                | no     | yes    |


## Features
The following CSI gRPC calls are implemented:
* **Controller Service**: CreateVolume, DeleteVolume, ControllerPublishVolume, ControllerUnpublishVolume, ControllerGetCapabilities, ControllerExpandVolume, ValidateVolumeCapabilities, CreateSnapshot, DeleteSnapshot, ListSnapshots
* **Node Service**: NodeStageVolume, NodeUnstageVolume, NodePublishVolume, NodeUnpublishVolume, NodeExpandVolume, NodeGetCapabilities, NodeGetInfo, NodeGetVolumeStats
* **Identity Service**: GetPluginInfo, GetPluginCapabilities, Probe

### CreateVolume Parameters
There are several optional parameters that could be passed into `CreateVolumeRequest.parameters` map:

| Parameters                  | Values                | Default  | Description                                                   |
|-----------------------------|-----------------------|----------|-------------------------------------------------------------- |
| "csi.storage.k8s.io/fsType" | xfs, ext2, ext3, ext4 | ext4     |File system type that will be formatted during volume creation |
| "type"                      | io1, gp2, standard    | gp2      |BSU volume type                                                |
| "iopsPerGB"                 |                       |          |I/O operations per second per GiB. Required when io1 volume type is specified |
| "encrypted"                 |                       |          |Not supported | 
| "kmsKeyId"                  |                       |          |Not supported |

**Notes**:
* The parameters are case insensitive.

# BSU CSI Driver on Kubernetes
Following sections are Kubernetes specific. If you are Kubernetes user, use followings for driver features, installation steps and examples.

## Kubernetes Version Compability Matrix
| OSC BSU CSI Driver \ Kubernetes Version|v1.21.5|
|----------------------------------------|-------|
| OSC-MIGRATION branch                   | yes   |


## Container Images:
|OSC BSU CSI Driver Version | Image                                     |
|---------------------------|-------------------------------------------|
| OSC-MIGRATION branch      |outscale/osc-ebs-csi-driver:v0.0.14beta    |

## Features
* **Static Provisioning** - create a new or migrating existing BSU volumes, then create persistence volume (PV) from the BSU volume and consume the PV from container using persistence volume claim (PVC).
* **Dynamic Provisioning** - uses persistence volume claim (PVC) to request the Kuberenetes to create the BSU volume on behalf of user and consumes the volume from inside container.
* **Mount Option** - mount options could be specified in persistence volume (PV) to define how the volume should be mounted.
* **Block Volume** (beta since 1.14) - consumes the BSU volume as a raw block device for latency sensitive application eg. MySql
* **Volume Snapshot** (beta) - creating volume snapshots and restore volume from snapshot.
* **Volume Encryption** - Not supported yet.

## Prerequisites

* Get yourself familiar with how to setup Kubernetes and have a working Kubernetes cluster:
  * To Use fsGroupPolicy field kor k8s version greater than or equal to 1.19.x start `kube-apiserver` and `kubelet` with `CSIVolumeFSGroupPolicy` feature gate enabled `--feature-gates=CSIVolumeFSGroupPolicy=true` and to control the behaviour of this field use the [fsGroupPolicy](https://github.com/outscale-dev/osc-bsu-csi-driver/blob/OSC-MIGRATION/osc-bsu-csi-driver/values.yaml#L117) chart values, detailled docs are [here](https://kubernetes-csi.github.io/docs/support-fsgroup.html)
  * To Enable snapshot.storage.k8s.io/v1beta1 please follow :
   	* https://kubernetes.io/blog/2019/12/09/kubernetes-1-17-feature-cis-volume-snapshot-beta/
  * For k8s version lower than v1.15.4
   	* Enable flag `--allow-privileged=true` for `kubelet` and `kube-apiserver`
   	* Enable `kube-apiserver` feature gates `--feature-gates=CSINodeInfo=true,CSIDriverRegistry=true,CSIBlockVolume=true,VolumeSnapshotDataSource=true`
   	* Enable `kubelet` feature gates `--feature-gates=CSINodeInfo=true,CSIDriverRegistry=true,CSIBlockVolume=true`

## Installation

- pre-installed k8s platform under outscale cloud with 3 masters and 2 workers on vm with `tinav2.c2r4p3` type
- prepare the machine from which you will run deploy the osc bsu csi plugin
- The bsu csi plugin needs AK/SK to interact with Outscale BSU API, so you can create an AK/SK using an eim user, for example, with a proper permission by attaching [a policy like](./example-eim-policy.json) 

```
    # ENV VARS 
    export OSC_ACCESS_KEY=XXXXX
    export OSC_SECRET_KEY=XXXXX
    export OSC_REGION=eu-west-2

    ## set the secrets
    curl https://raw.githubusercontent.com/outscale-dev/osc-bsu-csi-driver/OSC-MIGRATION/deploy/kubernetes/secret.yaml > secret.yaml
    cat secret.yaml | \
        sed "s/secret_key: \"\"/secret_key: \"$OSC_SECRET_KEY\"/g" | \
        sed "s/access_key: \"\"/access_key: \"$OSC_ACCESS_KEY\"/g" > osc-secret.yaml
    /usr/local/bin/kubectl delete -f osc-secret.yaml --namespace=kube-system
    /usr/local/bin/kubectl apply -f osc-secret.yaml --namespace=kube-system
    
    ## deploy the pod
    export IMAGE_NAME=outscale/osc-ebs-csi-driver
    export IMAGE_TAG="v0.0.14beta"
    git clone git@github.com:outscale-dev/osc-bsu-csi-driver.git
    cd osc-bsu-csi-driver
    helm uninstall osc-bsu-csi-driver  --namespace kube-system
    helm install osc-bsu-csi-driver ./osc-bsu-csi-driver \
         --namespace kube-system --set enableVolumeScheduling=true \
         --set enableVolumeResizing=true --set enableVolumeSnapshot=true \
         --set region=$OSC_REGION \
        --set image.repository=$IMAGE_NAME \
        --set image.tag=$IMAGE_TAG
                
    ## Check the pod is running
    kubectl get pods -o wide -A  -n kube-system
```

## Examples
Make sure you follow the [Prerequisites](README.md#Prerequisites) before the examples:
* [Dynamic Provisioning](../examples/kubernetes/dynamic-provisioning)
* [Block Volume](../examples/kubernetes/block-volume)
* [Volume Snapshot](../examples/kubernetes/snapshot)
* [Configure StorageClass](../examples/kubernetes/storageclass)
* [Volume Resizing](../examples/kubernetes/resizing)

## Development
Please go through [CSI Spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) and [General CSI driver development guideline](https://kubernetes-csi.github.io/docs/introduction.html?highlight=Deve#development-and-deployment) to get some basic understanding of CSI driver before you start.

### Requirements
* Golang 1.15.6
* [Ginkgo](https://github.com/onsi/ginkgo) in your PATH for integration testing and end-to-end testing
* Docker 18.09.2+ for releasing
* k8s v1.15.4+
* helm v3.5.0+

### Dependency
Dependencies are managed through go module. To build the project, first turn on go mod using `export GO111MODULE=on`, then build the project using: `make`

### Testing
* To execute all unit tests, run: `make test`
* To execute e2e single az tests, run: 
```
    cd osc-bsu-csi-driver
    export OSC_ACCESS_KEY=XXXX ; export OSC_SECRET_KEY=XXX ; export E2E_AZ="eu-west-2a"
    make test-e2e-single-az
```
