
**WARNING**: This driver is currently in Beta release and should not be used in performance critical applications.

# Outscale Block Storage Unit (BSU) CSI driver

## Overview

The Outscale Block Storage Unit Container Storage Interface (CSI) Driver provides a [CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md) interface used by Container Orchestrators to manage the lifecycle of Amazon EBS volumes.

## CSI Specification Compability Matrix

| OSC BSU CSI Driver \ CSI Version       |  v1.1.0|
|----------------------------------------|--------|
| master branch                          | yes    |


## Features
The following CSI gRPC calls are implemented:
* **Controller Service**: CreateVolume, DeleteVolume, ControllerPublishVolume, ControllerUnpublishVolume, ControllerGetCapabilities, ValidateVolumeCapabilities, CreateSnapshot, DeleteSnapshot, ListSnapshots
* **Node Service**: NodeStageVolume, NodeUnstageVolume, NodePublishVolume, NodeUnpublishVolume, NodeGetCapabilities, NodeGetInfo
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

# EBS CSI Driver on Kubernetes
Following sections are Kubernetes specific. If you are Kubernetes user, use followings for driver features, installation steps and examples.

## Kubernetes Version Compability Matrix
| OSC EBS CSI Driver \ Kubernetes Version|v1.15.4| 
|----------------------------------------|-------|
| master branch                          | yes   |


## Container Images:
|OSC EBS CSI Driver Version | Image                               |
|---------------------------|-------------------------------------|
|master branch              |amazon/aws-ebs-csi-driver:latest     |
|OSC-MIGRATION              |amazon/aws-ebs-csi-driver:v0.0.0-beta|

## Features
* **Static Provisioning** - create a new or migrating existing EBS volumes, then create persistence volume (PV) from the EBS volume and consume the PV from container using persistence volume claim (PVC).
* **Dynamic Provisioning** - uses persistence volume claim (PVC) to request the Kuberenetes to create the EBS volume on behalf of user and consumes the volume from inside container.
* **Mount Option** - mount options could be specified in persistence volume (PV) to define how the volume should be mounted.
* **Block Volume** (beta since 1.14) - consumes the EBS volume as a raw block device for latency sensitive application eg. MySql
* **Volume Snapshot** (alpha) - creating volume snapshots and restore volume from snapshot.
* **Volume Resizing** (alpha) - expand the volume size.

## Prerequisites

* Get yourself familiar with how to setup Kubernetes on AWS and have a working Kubernetes cluster:
  * Enable flag `--allow-privileged=true` for `kubelet` and `kube-apiserver`
  * Enable `kube-apiserver` feature gates `--feature-gates=CSINodeInfo=true,CSIDriverRegistry=true,CSIBlockVolume=true,VolumeSnapshotDataSource=true`
  * Enable `kubelet` feature gates `--feature-gates=CSINodeInfo=true,CSIDriverRegistry=true,CSIBlockVolume=true`

## Installation

- pre-installed k8s platform under outscale cloud with 3 masters and 2 workers on vm with `t2.medium` type
- prepare the machine from which you will run deploy the osc ebs csi plugin

```
    # ENV VARS 
    export OSC_ACCOUNT_ID=XXXXX
    export OSC_ACCOUNT_IAM=XXXX
    export OSC_USER_ID=XXXXXX
    export OSC_ARN="arn:aws:iam::XXXXX:user/XXX"
    export AWS_ACCESS_KEY_ID="XXXXXXX"
    export AWS_SECRET_ACCESS_KEY="XXXXXXX"
    export AWS_DEFAULT_REGION="eu-west-2"
    
    export IMAGE_NAME=outscale/osc-ebs-csi-driver
    export IMAGE_TAG="v0.0.0beta"
    
    ## set the secrets
    curl https://raw.githubusercontent.com/kubernetes-sigs/aws-ebs-csi-driver/master/deploy/kubernetes/secret.yaml > $HOME/secret_aws_template.yaml
    cat secret_aws_template.yaml | \
        sed "s/access_key: \"\"/access_key: \"$AWS_SECRET_ACCESS_KEY\"/g" | \
        sed "s/key_id: \"\"/key_id: \"$AWS_ACCESS_KEY_ID\"/g" > secret_aws.yaml
    echo "  aws_default_region: \""$AWS_DEFAULT_REGION"\"" >> secret_aws.yaml
    echo "  osc_account_id: \""$OSC_ACCOUNT_ID"\"" >> secret_aws.yaml
    echo "  osc_account_iam: \""$OSC_ACCOUNT_IAM"\"" >> secret_aws.yaml
    echo "  osc_user_id: \""$OSC_USER_ID"\"" >> secret_aws.yaml
    echo "  osc_arn: \""$OSC_ARN "\"" >> secret_aws.yaml
    /usr/local/bin/kubectl delete -f secret_aws.yaml --namespace=kube-system
    /usr/local/bin/kubectl apply -f secret_aws.yaml --namespace=kube-system
    ## deploy the pod
    git clone git@github.com:outscale-dev/osc-ebs-csi-driver.git
    cd osc-ebs-csi-driver
    helm del --purge aws-ebs-csi-driver --tls
    helm install --name aws-ebs-csi-driver \
                --set enableVolumeScheduling=true \
                --set enableVolumeResizing=true \
                --set enableVolumeSnapshot=true \
                --set image.repository=$IMAGE_NAME \
                --set image.tag=$IMAGE_TAG \
                ./aws-ebs-csi-driver --tls
                
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
Please go through [CSI Spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) and [General CSI driver development guideline](https://kubernetes-csi.github.io/docs/Development.html) to get some basic understanding of CSI driver before you start.

### Requirements
* Golang 1.12.7+
* [Ginkgo](https://github.com/onsi/ginkgo) in your PATH for integration testing and end-to-end testing
* Docker 18.09.2+ for releasing
* K8s v1.15.4+

### Dependency
Dependencies are managed through go module. To build the project, first turn on go mod using `export GO111MODULE=on`, then build the project using: `make`

### Testing
* To execute all unit tests, run: `make test`
* To execute sanity test run: `make test-sanity`
* To execute integration tests, run:
```
OSC_ACCOUNT_ID=XXXXX : the osc user id
OSC_ACCOUNT_IAM=xxxx: eim user name 
OSC_USER_ID=XXXX: the eim user id
OSC_ARN="XXXXX" : the eim user orn
AWS_ACCESS_KEY_ID=XXXX : the  AK
AWS_SECRET_ACCESS_KEY=XXXX : the SK
AWS_DEFAULT_REGION=XXX: the Region to be used

./run_int_test.sh

```

* To execute e2e single az tests, run: 
```
    cd osc-ebs-csi-driver
    wget https://dl.google.com/go/go1.12.7.linux-amd64.tar.gz
    tar -C /usr/local -xzf go1.12.7.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    export GOPATH="/root/go"
    
    go get -v -u github.com/onsi/ginkgo/ginkgo
    export KUBECONFIG=$HOME/.kube/config
    export AWS_AVAILABILITY_ZONES=eu-west-2b
    ARTIFACTS=$PWD/single_az_test_e2e_report
    mkdir -p $ARTIFACTS
    export NODES=4
    $GOPATH/bin/ginkgo -debug -p -nodes=$NODES -v --focus="\[ebs-csi-e2e\] \[single-az\]" tests/e2e -- -report-dir=$ARTIFACTS
    
```
**Notes**:
* Sanity tests make sure the driver complies with the CSI specification
* EC2 instance is required to run integration test, since it is exercising the actual flow of creating EBS volume, attaching it and read/write on the disk. See [Ingetration Testing](../tests/integration/README.md) for more details.
* E22 tests exercises various driver functionalities in Kubernetes cluster. See [E2E Testing](../tests/e2e/README.md) for more details.

