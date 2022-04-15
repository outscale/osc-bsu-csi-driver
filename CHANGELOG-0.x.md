# v0.0.15
> **_NOTE:_** In the future major version, the default FsType will change from EXT4 to XFS. You can start using it by changing the `defaultFsType` in the helm chart
## Notable changes
* Remove Snapshot Controller and CRD from the chart (See [Deployment Snapshot](https://kubernetes-csi.github.io/docs/snapshot-controller.html#deployment))
* Set FsType of the PV if no FsType is specified in the StorageClass
* Add the support of custom labels on the pod ([#101](https://github.com/outscale-dev/osc-bsu-csi-driver/pull/101))
* Update sidecars to the latest version. 
  * Impacts:
    * CSI spec v1.5.0
    * Minimal kubernetes version is now v1.20
* Update to kubernetes library to v1.23.4
## Bugfixes
* Make NodePublishVolume and NodeUnpublishVolume idempotent ([#163](https://github.com/outscale-dev/osc-bsu-csi-driver/pull/163))
# v0.0.14beta
## Notable changes
* Make Max BSU Volumes value custom

# v0.0.13beta
## Notable changes
* Update default Max BSU Volumes value

# v0.0.12beta
## Notable changes
* Update to k8s pkg 1.21.5
* update to csi 1.3.0
* Update charts and sidecars versions
* update e2e tests

# v0.0.11beta
## Notable changes
* Add fsGroupPolicy specific e2e test

# v0.0.10beta
## Notable changes
* Add fsGroupPolicy field to CSIDriver object and customize it with chart values

# v0.0.9beta
## Notable changes
* Implement ControllerExpandVolume using UpdateVolume api call
* Fix regression in detach disk toleration
* customise sidecars conatiner verbosity and timeout

# v0.0.8beta
## Notable changes
* Implement NodeGetVolumeStats

# v0.0.7beta
## Notable changes
* Use [osc-sdk-go](https://github.com/outscale/osc-sdk-go) instead of aws-sdk-go
* Update to helm3
* Update of sidecar images

# v0.0.6beta
## Notable changes
* Enable API sdk logs 


