
# Outscale Block Storage Unit (BSU) CSI driver

## Overview

The Outscale Block Storage Unit Container Storage Interface (CSI) Driver provides a [CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md) interface used by Container Orchestrators to manage the lifecycle of 3DS outscale BSU volumes.

## CSI Specification Compability Matrix

| Plugin Version | Compatible with CSI Version                                                       | Min K8s Version | Recommended K8s Version |
| -------------- | --------------------------------------------------------------------------------- | --------------- | ----------------------- |
| <= v0.0.14beta | [v1.3.0](https://github.com/container-storage-interface/spec/releases/tag/v1.3.0) | 1.16            | 1.22                    |
| v0.0.15        | [v1.5.0](https://github.com/container-storage-interface/spec/releases/tag/v1.5.0) | 1.20            | 1.23                    |
| v0.1.0         | [v1.5.0](https://github.com/container-storage-interface/spec/releases/tag/v1.5.0) | 1.20            | 1.23                    |

## Features
The following CSI gRPC calls are implemented:
* **Controller Service**: CreateVolume, DeleteVolume, ControllerPublishVolume, ControllerUnpublishVolume, ControllerGetCapabilities, ControllerExpandVolume, ValidateVolumeCapabilities, CreateSnapshot, DeleteSnapshot, ListSnapshots
* **Node Service**: NodeStageVolume, NodeUnstageVolume, NodePublishVolume, NodeUnpublishVolume, NodeExpandVolume, NodeGetCapabilities, NodeGetInfo, NodeGetVolumeStats
* **Identity Service**: GetPluginInfo, GetPluginCapabilities, Probe

The following CSI gRPC calls are **not yet** implemented:
* **Controller Service**: GetCapacity, ListVolumes, ControllerGetVolume
* **Node Service**: N/A
* **Identity Service**: N/A

### CreateVolume Parameters
There are several optional parameters that could be passed into `CreateVolumeRequest.parameters` map:

| Parameters                  | Values                | Default | Description                                                                   |
| --------------------------- | --------------------- | ------- | ----------------------------------------------------------------------------- |
| "csi.storage.k8s.io/fstype" | xfs, ext2, ext3, ext4 | ext4    | File system type that will be formatted during volume creation                |
| "type"                      | io1, gp2, standard    | gp2     | BSU volume type                                                               |
| "iopsPerGB"                 |                       |         | I/O operations per second per GiB. Required when io1 volume type is specified |
| "encrypted"                 |                       |         | Not supported                                                                 |
| "kmsKeyId"                  |                       |         | Not supported                                                                 |

**Notes**:
* The parameters are case sensitive.

## Use with Kubernetes
Following sections are Kubernetes specific. If you are Kubernetes user, use followings for driver features, installation steps and examples.

### Features
* **Static Provisioning** - create a new or migrating existing BSU volumes, then create persistence volume (PV) from the BSU volume and consume the PV from container using persistence volume claim (PVC).
* **Dynamic Provisioning** - uses persistence volume claim (PVC) to request the Kuberenetes to create the BSU volume on behalf of user and consumes the volume from inside container.
* **Mount Option** - mount options could be specified in persistence volume (PV) to define how the volume should be mounted.
* **Block Volume** (beta since 1.14) - consumes the BSU volume as a raw block device for latency sensitive application eg. MySql
* **Volume Snapshot** - creating volume snapshots and restore volume from snapshot.
* **Volume Encryption** - Not supported yet.
### Prerequisites
- Cluster K8S with compatible version (See [Version](README.md#csi-specification-compability-matrix))
- The plugin needs AK/SK to interact with Outscale BSU API, so you can create an AK/SK using an eim user, for example, with a proper permission by attaching [a policy like](./example-eim-policy.json) 
### Chart Configuration
See [Helm Chart Configuration](helm.md)
### Installation
See [Deploy](deploy.md)
## Examples
Make sure you follow the [Prerequisites](README.md#Prerequisites) before the examples:
* [Dynamic Provisioning](../examples/kubernetes/dynamic-provisioning)
* [Block Volume](../examples/kubernetes/block-volume)
* [Volume Snapshot](../examples/kubernetes/snapshot)
* [Configure StorageClass](../examples/kubernetes/storageclass)
* [Volume Resizing](../examples/kubernetes/resizing)

## Development
See [Development Process](development.md)