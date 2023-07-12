# Changelog

## [v0.2.2]
### Bugfixes
* Update osc-sdk-go package in order not to check region ([#762](https://github.com/outscale/osc-bsu-csi-driver/pull/762))

## [v0.2.1]
### Bugfixes
* Handle 39 volumes for scsi device per node ([#733](https://github.com/outscale/osc-bsu-csi-driver/issues/733))

## [v0.2.0]
### Features
* Increase maximum volumes per node (from 25 to 40) ([#657](https://github.com/outscale/osc-bsu-csi-driver/pull/657))
* Upgrade plugin to support 1.25 Kubernetes version cluster ([#613](https://github.com/outscale/osc-bsu-csi-driver/pull/613))

## Notable Changes
* Volume scheduling is enabled by default in the helm chart
## [v0.1.2]
### Bugfixes
* Fix deployement when using helm-git ([#520](https://github.com/outscale-dev/osc-bsu-csi-driver/issues/520))
## [v0.1.1]
### Features
* Add alternative to generate the Kubernetes secret using helm ([#370](https://github.com/outscale-dev/osc-bsu-csi-driver/pull/370))
* Add logs about request cache ([#476](https://github.com/outscale-dev/osc-bsu-csi-driver/pull/476))
### Bugfixes
* Fix the Iops regarding of the storage class parameter iopsPerGb, Outscale maximum iops and Outscale ratio iops/Gb ([#386](https://github.com/outscale-dev/osc-bsu-csi-driver/issues/386), [#394](https://github.com/outscale-dev/osc-bsu-csi-driver/issues/386))
* Fix idempotency on ControllerUnpublishVolume ([#409](https://github.com/outscale-dev/osc-bsu-csi-driver/issues/409))
* Fix idempotency on DeleteVolume and DeleteSnapshot ([#448](https://github.com/outscale-dev/osc-bsu-csi-driver/issues/448))
## [v0.1.0]
> **_NOTE:_** In case no topology is provided by the CO (not the case for Kubernetes > v1.17), the volume will be created in the AZ A.
### Notable changes
* Image is now based on Alpine 
* Add http-endpoint on side-cars ([#190](https://github.com/outscale-dev/osc-bsu-csi-driver/pull/190))
* Migration to Outscale SDK v2
* Upgrade sanity test framework to check v1.5.0 CSI spec compliance ([#290](https://github.com/outscale-dev/osc-bsu-csi-driver/issues/284)) 
* We can disable meta-data server access on controller pod by providing cluster's region during deployment
## [v0.0.15]
> **_NOTE:_** In the future major version, the default FsType will change from EXT4 to XFS. You can start using it by changing the `defaultFsType` in the helm chart
### Notable changes
* Remove Snapshot Controller and CRD from the chart (See [Deployment Snapshot](https://kubernetes-csi.github.io/docs/snapshot-controller.html#deployment))
* Set FsType of the PV if no FsType is specified in the StorageClass
* Add the support of custom labels on the pod ([#101](https://github.com/outscale-dev/osc-bsu-csi-driver/pull/101))
* Update sidecars to the latest version. 
  * Impacts:
    * CSI spec v1.5.0
    * Minimal kubernetes version is now v1.20
* Update to kubernetes library to v1.23.4
### Bugfixes
* Make NodePublishVolume and NodeUnpublishVolume idempotent ([#163](https://github.com/outscale-dev/osc-bsu-csi-driver/pull/163))
## [v0.0.14beta]
### Notable changes
* Make Max BSU Volumes value custom

## [v0.0.13beta]
### Notable changes
* Update default Max BSU Volumes value

## [v0.0.12beta]
### Notable changes
* Update to k8s pkg 1.21.5
* update to csi 1.3.0
* Update charts and sidecars versions
* update e2e tests

## [v0.0.11beta]
### Notable changes
* Add fsGroupPolicy specific e2e test

## [v0.0.10beta]
### Notable changes
* Add fsGroupPolicy field to CSIDriver object and customize it with chart values

## [v0.0.9beta]
### Notable changes
* Implement ControllerExpandVolume using UpdateVolume api call
* Fix regression in detach disk toleration
* customise sidecars conatiner verbosity and timeout

## [v0.0.8beta]
### Notable changes
* Implement NodeGetVolumeStats

## [v0.0.7beta]
### Notable changes
* Use [osc-sdk-go](https://github.com/outscale/osc-sdk-go) instead of aws-sdk-go
* Update to helm3
* Update of sidecar images

## [v0.0.6beta]
### Notable changes
* Enable API sdk logs 


