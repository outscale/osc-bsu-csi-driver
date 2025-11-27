# osc-bsu-csi-driver

![Version: 2.0.0](https://img.shields.io/badge/Version-2.0.0-informational?style=flat-square) ![AppVersion: v1.8.0](https://img.shields.io/badge/AppVersion-v1.8.0-informational?style=flat-square)

A Helm chart for the Outscale BSU CSI Driver

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
| cloud.backoff.duration | string | `"1s"` | Wait before the first retry |
| cloud.backoff.factor | string | `"2"` | Factor by which to multiply the duration between each try |
| cloud.backoff.steps | string | `"5"` | Maximum number of tries in case on failure |
| cloud.caBundle.key | string | `""` | Entry where the CA bundle can be found in the secret |
| cloud.caBundle.name | string | `""` | Secret name containing additional certificates authorities |
| cloud.credentials.accessKey | string | `nil` | AK to use when creating secret. |
| cloud.credentials.create | bool | `false` | Do we need to create the secret ? (if not set, we expect that the secret already exists) |
| cloud.credentials.secretKey | string | `nil` | SK to use when creating secret. |
| cloud.credentials.secretName | string | `"osc-csi-bsu"` | Secret where AK/SK are stored. |
| cloud.customEndpoint | string | `""` | Use customEndpoint (url with protocol) ex: https://api.eu-west-2.outscale.com/api/v1 |
| cloud.httpsProxy | string | `""` | Value used to create environment variable HTTPS_PROXY |
| cloud.noProxy | string | `""` | Value used to create environment variable NO_PROXY |
| cloud.region | string | `""` | Region to use, otherwise it will be looked up via metadata. By providing this parameter, the controller will not require to access the metadata. |
| controller.affinity | object | `{}` | Affinity settings |
| controller.nodeSelector | object | `{}` | Node selector used to deploy controller pods. |
| controller.podAnnotations | object | `{}` | Annotations for controller pod |
| controller.podLabels | object | `{}` | Labels for controller pod |
| controller.replicas | int | `2` | Number of replicas to deploy |
| controller.resources | object | `{}` | Specify limits of resources used by the pod containers |
| controller.securityContext | object | `{}` | Security context for the controller container. |
| controller.tolerations | list | `[{"key":"CriticalAddonsOnly","operator":"Exists"},{"effect":"NoExecute","operator":"Exists","tolerationSeconds":300}]` | Pod tolerations |
| controller.updateStrategy | object | `{"type":"RollingUpdate"}` | Controller deployment update strategy. |
| driver.defaultFsType | string | `"ext4"` | Default filesystem for the volume if no `FsType` is set in `StorageClass` |
| driver.enableSnapshotCrossNamespace | bool | `false` | Enable cross namespace snapshots |
| driver.enableVolumeAttributesClass | bool | `false` | Enable volume updates using VolumeAttributesClass |
| driver.enableVolumeSnapshot | bool | `false` | Enable volume snapshots |
| driver.extraSnapshotTags | object | `{}` | Add extra tags on snapshots |
| driver.extraVolumeTags | object | `{}` | Add extra tags on volumes |
| driver.fsGroupPolicy | string | `"File"` | Filesystem group policy (see [Docs](https://kubernetes-csi.github.io/docs/support-fsgroup.html#supported-modes)) |
| driver.image | string | `"outscale/osc-bsu-csi-driver"` | Container image to use |
| driver.imagePullPolicy | string | `"IfNotPresent"` | Container image pull policy |
| driver.maxBsuVolumes | string | `""` | Maximum number of volumes that can be attached to a node, autocomputed by default (see [Docs](https://docs.outscale.com/en/userguide/About-Volumes.html)) |
| driver.name | string | `"bsu.csi.outscale.com"` |  |
| driver.tag | string | `"v1.8.0"` | Container image tag to deploy |
| imagePullSecrets | list | `[]` | Specify image pull secrets |
| logs.format | string | `"text"` | Format of logs: text or json |
| logs.verbosity | int | `3` | Verbosity level of the plugin |
| node.additionalArgs | list | `[]` | Node controller command line additional args |
| node.podAnnotations | object | `{}` | Annotations for node controller pod |
| node.podLabels | object | `{}` | Labels for node controller pod |
| node.resources | object | `{}` | Node controller DaemonSet resources. If not set, the top-level resources will be used. |
| node.securityContext | object | `{"allowPrivilegeEscalation":true,"privileged":true,"readOnlyRootFilesystem":false,"seccompProfile":{"type":"Unconfined"}}` | Security context for the node container. |
| node.tolerations | list | `[]` | Pod tolerations |
| node.updateStrategy | object | `{"type":"RollingUpdate"}` | Node controller DaemonSet update strategy |
| serviceAccount.annotations | object | `{}` |  |
| sidecars.attacher.additionalArgs | list | `[]` |  |
| sidecars.attacher.image | string | `"registry.k8s.io/sig-storage/csi-attacher"` |  |
| sidecars.attacher.metricsPort | string | `"8090"` | Port of the metrics endpoint |
| sidecars.attacher.resources | object | `{}` | Sidecar resources. If not set, the top-level resources will be used. |
| sidecars.attacher.tag | string | `"v4.9.0"` |  |
| sidecars.attacher.workerThreads | int | `100` |  |
| sidecars.kubeAPI.QPS | int | `20` | Maximum allowed number of queries per second to the Kubernetes API |
| sidecars.kubeAPI.burst | int | `100` | Allowed burst over QPS |
| sidecars.leaderElection | object | `{"leaseDuration":null,"renewDeadline":null,"retryPeriod":null}` | leaderElection config for all sidecars |
| sidecars.livenessProbe.image | string | `"registry.k8s.io/sig-storage/livenessprobe"` |  |
| sidecars.livenessProbe.port | string | `"9808"` | Port of the liveness of the main container |
| sidecars.livenessProbe.resources | object | `{}` | Sidecar resources. If not set, the node or top-level resources will be used. |
| sidecars.livenessProbe.tag | string | `"v2.16.0"` |  |
| sidecars.metrics | bool | `false` | activates the metrics HTTP endpoint on sidecars. See each sidecar for port definition. |
| sidecars.nodeDriverRegistrar.image | string | `"registry.k8s.io/sig-storage/csi-node-driver-registrar"` |  |
| sidecars.nodeDriverRegistrar.tag | string | `"v2.14.0"` |  |
| sidecars.provisioner.additionalArgs | list | `[]` |  |
| sidecars.provisioner.image | string | `"registry.k8s.io/sig-storage/csi-provisioner"` |  |
| sidecars.provisioner.metricsPort | string | `"8089"` | Port of the metrics endpoint |
| sidecars.provisioner.resources | object | `{}` |  |
| sidecars.provisioner.tag | string | `"v5.3.0"` |  |
| sidecars.provisioner.workerThreads | int | `100` |  |
| sidecars.resizer.additionalArgs | list | `[]` |  |
| sidecars.resizer.image | string | `"registry.k8s.io/sig-storage/csi-resizer"` |  |
| sidecars.resizer.metricsPort | string | `"8092"` | Port of the metrics endpoint |
| sidecars.resizer.resources | object | `{}` | Sidecar resources. If not set, the top-level resources will be used. |
| sidecars.resizer.tag | string | `"v1.14.0"` |  |
| sidecars.resizer.workerThreads | int | `100` |  |
| sidecars.resources | object | `{}` | Default sidecar resources, unless set at the sidecar level. |
| sidecars.securityContext | object | `{"allowPrivilegeEscalation":false,"readOnlyRootFilesystem":true,"seccompProfile":{"type":"RuntimeDefault"}}` | securityContext config for all sidecars. |
| sidecars.snapshotter.additionalArgs | list | `[]` |  |
| sidecars.snapshotter.image | string | `"registry.k8s.io/sig-storage/csi-snapshotter"` |  |
| sidecars.snapshotter.metricsPort | string | `"8091"` | Port of the metrics endpoint |
| sidecars.snapshotter.resources | object | `{}` | Sidecar resources. If not set, the top-level resources will be used. |
| sidecars.snapshotter.tag | string | `"v8.3.0"` |  |
| sidecars.snapshotter.workerThreads | int | `100` |  |
| sidecars.timeout | string | `"5m"` | Timeout for sidecars calls to the CSI driver |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.11.0](https://github.com/norwoodj/helm-docs/releases/v1.11.0)
