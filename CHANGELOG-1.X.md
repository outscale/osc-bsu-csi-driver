# Changelog

## [v1.5.2]
### 🛠️ Changed
* 🔊 errors: better OAPI error reporting by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/926
* 📝 doc: updated deploy & release docs by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/922
### 🛠️ Fixed
* 🐛 fix: missing xfs_growfs binary by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/925

## [v1.5.1]
### 🛠️ Changed
* ♻️ env: allow using OSC_REGION instead of AWS_REGION by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/917
* 🔧 helm: allow fine-grained resource configuration by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/918
* 🔧 helm: add updateStrategy=RollingUpdate to node DaemonSet by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/919

## [v1.5.0]
### ✨ Added
* ✨ feat: custom extra tags on snapshots by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/875
### 🛠️ Changed
* 🚨 Gofmt fixes by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/846
* ✅ Pkg: test fixes & cleanup by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/845
* 📝 doc: fix version in release doc by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/848
* 👷 ci: bump versions in e2e action, only trigger on pr by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/859
* 👷 ci: bump rke & k8s versions by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/863
* 👷 ci: disable dependabot on OSC-MIGRATION by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/864
* 🔊 logs: migrate to structured/contextual logging by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/847
* 👷 ci: switch to official golangci-lint action by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/867
* ♻️ refacto: use github.com/outscale instead of github.com/outscale-dev by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/869
* ⬆️  go.mod: bump k8s to 1.30.7 & Go to 1.23.4 by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/865
* 💚 fix: use backoff in waitForVolume to fix e2e test failures by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/872
* 🔊 logs: more structured logging by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/871
* 💚 e2e tests: no need to wait for a 'deleting' snapshot to be really deleted by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/876
* ♻️ refacto: use int32 for GiB sizes and iops by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/874
* 🥅 errors: better error reporting in luks layer by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/880
* ✨ feat: support custom backoff policies by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/873
* ⬆️ deploy: switch to distroless image, strip binary by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/881
* 🚨 linter fixes by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/882
* 👷 ci: enable trivy by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/890
* update documentation for volumesnapshot handle by @outscale-hmi in https://github.com/outscale/osc-bsu-csi-driver/pull/888
* 👷 ci: bump ubuntu versions by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/895
* ♻️ refacto: use wait.PollUntilContextCancel for wait loops, sync snaphot creation by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/902
* 🔧 go.mod: fix go version by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/906
* 👷 ci: add release notes template by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/908
* 👷 ci: multiple runner support by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/907
* ⚡️ perfs: backoff & readiness loop tuning by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/903
### 🐛 Fixed
* ✨ feat: honor maxEntries in ListSnapshots by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/870
* 🐛 fix: fix pagination on ListSnapshots by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/884
* 🐛 fix: recreate errored snapshots by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/886
* 🐛 fix: stop backoff when context is cancelled by @jfbus in https://github.com/outscale/osc-bsu-csi-driver/pull/883

## [v1.4.1]
### Bugfixes
* Fix PV will be encrypted failing due to restictive securityContext ([#835](https://github.com/outscale/osc-bsu-csi-driver/pull/835))
* Run skipped test ([#836](https://github.com/outscale/osc-bsu-csi-driver/pull/836))
* Remove duplicate sc and misplaced containerSecurityContext ([#838](https://github.com/outscale/osc-bsu-csi-driver/pull/838))
* Fix resize luks volume  ([#839](https://github.com/outscale/osc-bsu-csi-driver/pull/839))
* Replace deprecated ioutil.TempDir ([#840](https://github.com/outscale/osc-bsu-csi-driver/pull/840))
* Add kernel Minimum Requirements for XFS Support ([#841](https://github.com/outscale/osc-bsu-csi-driver/pull/841))


## [v1.4.0]
### Features
* Add support for multiple feature-gates arguments for the csi-provisioner([#810](https://github.com/outscale/osc-bsu-csi-driver/pull/810/))
* Upgrade plugin to support 1.30 Kubernetes version cluster and sideCars versions ([#814](https://github.com/outscale/osc-bsu-csi-driver/pull/814)
* Clean way to set imagePullSecrets and respect list  ([#815](https://github.com/outscale/osc-bsu-csi-driver/pull/815))
* Increase default provisioner, resizer, snapshotter retry-interval-max ([#820](https://github.com/outscale/osc-bsu-csi-driver/pull/820))
* Reduce verbosity level ([#823](https://github.com/outscale/osc-bsu-csi-driver/pull/823))
* Support Volume Group Snapshots k8s 1.27 ([#827](https://github.com/outscale/osc-bsu-csi-driver/pull/827))
* Add default kube-api-qps, burst, and worker-threads values in CSI driver ([#826](https://github.com/outscale/osc-bsu-csi-driver/pull/826))
* Set RuntimeDefault as default seccompProfile in securityContext ([#828](https://github.com/outscale/osc-bsu-csi-driver/pull/828))

### Bugfixes
* Fix extra arg([#818](https://github.com/outscale/osc-bsu-csi-driver/pull/818)

## [v1.3.0]
### Features
* Support standard topology annotation for Volumes ([#787](https://github.com/outscale/osc-bsu-csi-driver/pull/787))
* Upgrade plugin to support 1.26 Kubernetes version cluster ([#800](https://github.com/outscale/osc-bsu-csi-driver/pull/800))
* Override controller toleration in chart ([#804](https://github.com/outscale/osc-bsu-csi-driver/pull/804))
### Bugfixes
* Fix idempotency problem for Volumes created from Snapshot ([#799](https://github.com/outscale/osc-bsu-csi-driver/pull/799))

## [v1.2.4]
### Bugfixes
* xfs as fstype: missing xfsgrowfs in $PATH ([#777](https://github.com/outscale/osc-bsu-csi-driver/pull/777))
### Features
* Configure https proxy + ca bundle ([#761](https://github.com/outscale/osc-bsu-csi-driver/pull/761))

## [v1.2.3]
### Bugfixes
* Set custom endpoint ([#767]https://github.com/outscale/osc-bsu-csi-driver/pull/767))

## [v1.2.2]
### Bugfixes
* Update osc-sdk-go package in order not to check region ([#762](https://github.com/outscale/osc-bsu-csi-driver/pull/762))

## [v1.2.1]
### Bugfixes
* Handle 39 volumes for scsi device per node ([#733](https://github.com/outscale/osc-bsu-csi-driver/issues/733))

## [v1.2.0]
### Features
* Increase maximum volumes per node (from 25 to 40) ([#650](https://github.com/outscale/osc-bsu-csi-driver/pull/650))
* Upgrade plugin to support 1.25 Kubernetes version cluster ([#611](https://github.com/outscale/osc-bsu-csi-driver/pull/611))
### Notable Changes
* Volume scheduling is enabled by default in the helm chart
## [v1.1.1]
### Bugfixes
* Fix deployement when using helm-git ([#520](https://github.com/outscale/osc-bsu-csi-driver/issues/520))
## [v1.1.0]
### Features
* Support encryption on volumes ([#85](https://github.com/outscale/osc-bsu-csi-driver/issues/85))
* Add alternative to generate the Kubernetes secret using helm ([#370](https://github.com/outscale/osc-bsu-csi-driver/pull/371))
* Add logs about request cache ([#475](https://github.com/outscale/osc-bsu-csi-driver/pull/475))
### Bugfixes
* Fix the Iops regarding of the storage class parameter iopsPerGb, Outscale maximum iops and Outscale ratio iops/Gb ([#386](https://github.com/outscale/osc-bsu-csi-driver/issues/386), [#394](https://github.com/outscale/osc-bsu-csi-driver/issues/386))
* Fix idempotency on ControllerUnpublishVolume ([#409](https://github.com/outscale/osc-bsu-csi-driver/issues/409))
* Fix idempotency on DeleteVolume and DeleteSnapshot ([#448](https://github.com/outscale/osc-bsu-csi-driver/issues/448))
## [v1.0.0]
> **_NOTE:_** If you want to migrate from v0.X.X to this version, please read the migration process otherwise you will loose the management of your current PVC
### Breaking changes
* Rename driver name to "bsu.csi.outscale.com"
