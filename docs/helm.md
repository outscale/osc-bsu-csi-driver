# osc-bsu-csi-driver

![Version: 2.0.0](https://img.shields.io/badge/Version-2.0.0-informational?style=flat-square) ![AppVersion: v1.7.0](https://img.shields.io/badge/AppVersion-v1.7.0-informational?style=flat-square)

A Helm chart for Outscale BSU CSI Driver

**Homepage:** <https://github.com/outscale/osc-bsu-csi-driver>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| 3DS Outscale | <support@outscale.com> |  |

## Source Code

* <https://github.com/outscale/osc-bsu-csi-driver>

## Requirements

Kubernetes: `>=1.20`

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| cloud.backoff.duration | string | `"750ms"` | Initial duraction of backoff |
| cloud.backoff.factor | string | `"1.4"` | Factor multiplied by Duration for each iteration |
| cloud.backoff.steps | string | `"3"` | Remaining number of iterations in which the duration parameter may change |
| cloud.caBundle.key | string | `""` | Entry key in secret used to store additional certificates authorities |
| cloud.caBundle.name | string | `""` | Secret name containing additional certificates authorities |
| cloud.credentials.accessKey | string | `nil` | If creating a secret, put this AK inside. |
| cloud.credentials.create | bool | `false` | Actually create a secret in the deployment for AK/SK (else, only reference it) |
| cloud.credentials.secretKey | string | `nil` | If creating a secret, put this SK inside. |
| cloud.credentials.secretName | string | `"osc-csi-bsu"` | Use AK/SK from this secret |
| cloud.customEndpoint | string | `""` | Use customEndpoint (url with protocol) ex: https://api.eu-west-2.outscale.com/api/v1 |
| cloud.httpsProxy | string | `""` | Value used to create environment variable HTTPS_PROXY |
| cloud.noProxy | string | `""` | Value used to create environment variable NO_PROXY |
| cloud.region | string | `""` | Region to use, otherwise it will be looked up via metadata. By providing this parameter, the controller will not require to access the metadata. |
| controller.affinity | object | `{}` | Affinity settings |
| controller.nodeSelector | object | `{}` |  |
| controller.podAnnotations | object | `{}` | Annotations for controller pod |
| controller.podLabels | object | `{}` | Labels for controller pod |
| controller.replica | int | `2` | Number of replicas to deploy |
| controller.resources | object | `{}` | Specify limits of resources used by the pod containers |
| controller.tolerations | list | `[{"key":"CriticalAddonsOnly","operator":"Exists"},{"effect":"NoExecute","operator":"Exists","tolerationSeconds":300}]` | Pod tolerations |
| controller.updateStrategy | object | `{"rollingUpdate":{"maxUnavailable":1},"type":"RollingUpdate"}` | Controller deployment update strategy. |
| driver.defaultFsType | string | `"ext4"` | Default filesystem for the volume if no `FsType` is set in `StorageClass` |
| driver.enableVolumeSnapshot | bool | `false` | Enable volume snapshot True if enable volume snapshot |
| driver.extraSnapshotTags | object | `{}` | Add extra tags on snapshots |
| driver.extraVolumeTags | object | `{}` | Add extra tags on volumes |
| driver.fsGroupPolicy | string | `"File"` | Policy of the FileSystem (see [Docs](https://kubernetes-csi.github.io/docs/support-fsgroup.html#supported-modes)) |
| driver.image.pullPolicy | string | `"IfNotPresent"` | Container pull policy |
| driver.image.repository | string | `"outscale/osc-bsu-csi-driver"` | Container image to use |
| driver.image.tag | string | `"v1.6.0"` | Container image tag to deploy |
| driver.maxBsuVolumes | string | `""` | Maximum number of volumes that can be attached to a node, autocomputed by default (see [Docs](https://docs.outscale.com/en/userguide/About-Volumes.html)) |
| driver.name | string | `"osc-bsu-csi-driver"` |  |
| imagePullSecrets | list | `[]` | Specify image pull secrets |
| logs.format | string | `"text"` | Format of logs: test or json |
| logs.verbosity | int | `3` | Verbosity level of the plugin |
| node.args | list | `[]` | Node controller command line additional args |
| node.containerSecurityContext.allowPrivilegeEscalation | bool | `true` |  |
| node.containerSecurityContext.privileged | bool | `true` |  |
| node.containerSecurityContext.readOnlyRootFilesystem | bool | `false` |  |
| node.containerSecurityContext.seccompProfile.type | string | `"Unconfined"` |  |
| node.podAnnotations | object | `{}` | Annotations for node controller pod |
| node.podLabels | object | `{}` | Labels for node controller pod |
| node.resources | object | `{}` | Node controller DaemonSet resources. If not set, the top-level resources will be used. |
| node.tolerations | list | `[]` | Pod tolerations |
| node.updateStrategy | object | `{"rollingUpdate":{"maxSurge":0,"maxUnavailable":"10%"},"type":"RollingUpdate"}` | Node controller DaemonSet update strategy |
| serviceAccount.controller.annotations | object | `{}` |  |
| serviceAccount.snapshot.annotations | object | `{}` |  |
| sidecars.attacher.additionalArgs | list | `[]` |  |
| sidecars.attacher.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.attacher.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.attacher.httpEndpointPort | string | `"8090"` | Port of the http endpoint |
| sidecars.attacher.image | string | `"registry.k8s.io/sig-storage/csi-attacher"` |  |
| sidecars.attacher.resources | object | `{}` | Sidecar resources. If not set, the top-level resources will be used. |
| sidecars.attacher.tag | string | `"v4.9.0"` |  |
| sidecars.attacher.workerThreads | int | `50` |  |
| sidecars.leaderElection | object | `{}` | leaderElection config for all sidecars, you can specify `leaseDuration`, `renewDeadline` and/or `retryPeriod`. Each value must be in an acceptable time.ParseDuration format.(Ref: https://pkg.go.dev/flag#Duration) |
| sidecars.livenessProbe.image | string | `"registry.k8s.io/sig-storage/livenessprobe"` |  |
| sidecars.livenessProbe.port | string | `"9808"` | Port of the liveness of the main container |
| sidecars.livenessProbe.resources | object | `{}` | Sidecar resources. If not set, the node or top-level resources will be used. |
| sidecars.livenessProbe.tag | string | `"v2.16.0"` |  |
| sidecars.nodeDriverRegistrar.image | string | `"registry.k8s.io/sig-storage/csi-node-driver-registrar"` |  |
| sidecars.nodeDriverRegistrar.tag | string | `"v2.14.0"` |  |
| sidecars.provisioner.additionalArgs | list | `[]` |  |
| sidecars.provisioner.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.provisioner.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.provisioner.httpEndpointPort | string | `"8089"` | Port of the http endpoint |
| sidecars.provisioner.image | string | `"registry.k8s.io/sig-storage/csi-provisioner"` |  |
| sidecars.provisioner.resources | object | `{}` |  |
| sidecars.provisioner.tag | string | `"v5.3.0"` |  |
| sidecars.provisioner.workerThreads | int | `50` |  |
| sidecars.resizer.additionalArgs | list | `[]` |  |
| sidecars.resizer.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.resizer.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.resizer.httpEndpointPort | string | `"8092"` | Port of the http endpoint |
| sidecars.resizer.image | string | `"registry.k8s.io/sig-storage/csi-resizer"` |  |
| sidecars.resizer.resources | object | `{}` | Sidecar resources. If not set, the top-level resources will be used. |
| sidecars.resizer.tag | string | `"v1.14.0"` |  |
| sidecars.resizer.workerThreads | int | `50` |  |
| sidecars.resources | object | `{}` | Default sidecar resources, unless set at the sidecar level. |
| sidecars.securityContext | object | `{"allowPrivilegeEscalation":false,"readOnlyRootFilesystem":true,"seccompProfile":{"type":"RuntimeDefault"}}` | securityContext config for all sidecars. |
| sidecars.snapshotter.additionalArgs | list | `[]` |  |
| sidecars.snapshotter.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.snapshotter.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.snapshotter.httpEndpointPort | string | `"8091"` | Port of the http endpoint |
| sidecars.snapshotter.image | string | `"registry.k8s.io/sig-storage/csi-snapshotter"` |  |
| sidecars.snapshotter.resources | object | `{}` | Sidecar resources. If not set, the top-level resources will be used. |
| sidecars.snapshotter.tag | string | `"v8.3.0"` |  |
| sidecars.snapshotter.workerThreads | int | `50` |  |
| sidecars.timeout | string | `"60s"` | Timeout for sidecars calls to the CSI driver |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.11.0](https://github.com/norwoodj/helm-docs/releases/v1.11.0)
