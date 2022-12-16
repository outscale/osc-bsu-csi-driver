# Changelog

## [v1.1.0]
### Features
* Support encryption on volumes ([#85](https://github.com/outscale-dev/osc-bsu-csi-driver/issues/85))
* Add alternative to generate the Kubernetes secret using helm ([#370](https://github.com/outscale-dev/osc-bsu-csi-driver/pull/371))
* Add logs about request cache ([#475](https://github.com/outscale-dev/osc-bsu-csi-driver/pull/475))
### Bugfixes
* Fix the Iops regarding of the storage class parameter iopsPerGb, Outscale maximum iops and Outscale ratio iops/Gb ([#386](https://github.com/outscale-dev/osc-bsu-csi-driver/issues/386), [#394](https://github.com/outscale-dev/osc-bsu-csi-driver/issues/386))
* Fix idempotency on ControllerUnpublishVolume ([#409](https://github.com/outscale-dev/osc-bsu-csi-driver/issues/409))
* Fix idempotency on DeleteVolume and DeleteSnapshot ([#448](https://github.com/outscale-dev/osc-bsu-csi-driver/issues/448))
## [v1.0.0]
> **_NOTE:_** If you want to migrate from v0.X.X to this version, please read the migration process otherwise you will loose the management of your current PVC
### Breaking changes
* Rename driver name to "bsu.csi.outscale.com"