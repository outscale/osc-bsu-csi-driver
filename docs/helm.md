# osc-bsu-csi-driver

![Version: 1.8.0](https://img.shields.io/badge/Version-1.8.0-informational?style=flat-square) ![AppVersion: v1.6.0](https://img.shields.io/badge/AppVersion-v1.6.0-informational?style=flat-square)

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
| affinity | object | `{}` | Affinity settings |
| backoff.duration | string | `"750ms"` | Initial duraction of backoff |
| backoff.factor | string | `"1.4"` | Factor multiplied by Duration for each iteration |
| backoff.steps | string | `"3"` | Remaining number of iterations in which the duration parameter may change |
| caBundle.key | string | `""` | Entry key in secret used to store additional certificates authorities |
| caBundle.name | string | `""` | Secret name containing additional certificates authorities |
| credentials.accessKey | string | `nil` | If creating a secret, put this AK inside. |
| credentials.create | bool | `false` | Actually create a secret in the deployment for AK/SK (else, only reference it) |
| credentials.secretKey | string | `nil` | If creating a secret, put this SK inside. |
| credentials.secretName | string | `"osc-csi-bsu"` | Use AK/SK from this secret |
| csiDriver.fsGroupPolicy | string | `"File"` | Policy of the FileSystem (see [Docs](https://kubernetes-csi.github.io/docs/support-fsgroup.html#supported-modes)) |
| customEndpoint | string | `""` | Use customEndpoint (url with protocol) ex: https://api.eu-west-2.outscale.com/api/v1 |
| defaultFsType | string | `"ext4"` | Default filesystem for the volume if no `FsType` is set in `StorageClass` |
| enableVolumeResizing | bool | `false` | Enable volume resizing True if enable volume resizing |
| enableVolumeScheduling | bool | `true` | Enable schedule volume for dynamic volume provisioning True if enable volume scheduling for dynamic volume provisioning |
| enableVolumeSnapshot | bool | `false` | Enable volume snapshot True if enable volume snapshot |
| extraCreateMetadata | bool | `false` | Add pv/pvc metadata to plugin create requests as parameters |
| extraSnapshotTags | object | `{}` | Add extra tags on snapshots |
| extraVolumeTags | object | `{}` | Add extra tags on volumes |
| httpsProxy | string | `""` | Value used to create environment variable HTTPS_PROXY |
| image.pullPolicy | string | `"IfNotPresent"` | Container pull policy |
| image.repository | string | `"outscale/osc-bsu-csi-driver"` | Container image to use |
| image.tag | string | `"v1.6.0"` | Container image tag to deploy |
| imagePullSecrets | list | `[]` | Specify image pull secrets |
| maxBsuVolumes | string | `"39"` | Maximum volume to attach to a node (see [Docs](https://docs.outscale.com/en/userguide/About-Volumes.html)) |
| nameOverride | string | `""` | Override name of the app (instead of `osc-bsu-csi-driver`) |
| noProxy | string | `""` | Value used to create environment variable NO_PROXY |
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
| nodeSelector | object | `{}` |  |
| podAnnotations | object | `{}` | Annotations for controller pod |
| podLabels | object | `{}` | Labels for controller pod |
| region | string | `""` | Region to use, otherwise it will be looked up via metadata. By providing this parameter, the controller will not require to access the metadata. |
| replicaCount | int | `2` | Number of replicas to deploy |
| resources | object | `{}` | Specify limits of resources used by the pod containers |
| serviceAccount.controller.annotations | object | `{}` |  |
| serviceAccount.snapshot.annotations | object | `{}` |  |
| sidecars.attacherImage.additionalArgs | list | `[]` |  |
| sidecars.attacherImage.additionalClusterRoleRules | string | `nil` | Grant additional permissions to csi-attacher |
| sidecars.attacherImage.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.attacherImage.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.attacherImage.httpEndpointPort | string | `"8090"` | Port of the http endpoint |
| sidecars.attacherImage.leaderElection | object | `{}` | Customize leaderElection, you can specify `leaseDuration`, `renewDeadline` and/or `retryPeriod`. Each value must be in an acceptable time.ParseDuration format.(Ref: https://pkg.go.dev/flag#Duration) |
| sidecars.attacherImage.repository | string | `"registry.k8s.io/sig-storage/csi-attacher"` |  |
| sidecars.attacherImage.resources | object | `{}` | Sidecar resources. If not set, the top-level resources will be used. |
| sidecars.attacherImage.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| sidecars.attacherImage.securityContext.readOnlyRootFilesystem | bool | `true` |  |
| sidecars.attacherImage.securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| sidecars.attacherImage.tag | string | `"v4.8.1"` |  |
| sidecars.livenessProbeImage.port | string | `"9808"` | Port of the liveness of the main container |
| sidecars.livenessProbeImage.repository | string | `"registry.k8s.io/sig-storage/livenessprobe"` |  |
| sidecars.livenessProbeImage.resources | object | `{}` | Sidecar resources. If not set, the node or top-level resources will be used. |
| sidecars.livenessProbeImage.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| sidecars.livenessProbeImage.securityContext.readOnlyRootFilesystem | bool | `true` |  |
| sidecars.livenessProbeImage.securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| sidecars.livenessProbeImage.tag | string | `"v2.13.1"` |  |
| sidecars.nodeDriverRegistrarImage.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.nodeDriverRegistrarImage.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.nodeDriverRegistrarImage.httpEndpointPort | string | `"8093"` | Port of the http endpoint |
| sidecars.nodeDriverRegistrarImage.repository | string | `"registry.k8s.io/sig-storage/csi-node-driver-registrar"` |  |
| sidecars.nodeDriverRegistrarImage.resources | object | `{}` | Sidecar resources. If not set, the node or top-level resources will be used. |
| sidecars.nodeDriverRegistrarImage.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| sidecars.nodeDriverRegistrarImage.securityContext.readOnlyRootFilesystem | bool | `true` |  |
| sidecars.nodeDriverRegistrarImage.securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| sidecars.nodeDriverRegistrarImage.tag | string | `"v2.12.0"` |  |
| sidecars.provisionerImage.additionalArgs | list | `[]` |  |
| sidecars.provisionerImage.additionalClusterRoleRules | string | `nil` |  |
| sidecars.provisionerImage.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.provisionerImage.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.provisionerImage.httpEndpointPort | string | `"8089"` | Port of the http endpoint |
| sidecars.provisionerImage.leaderElection | object | `{}` | Customize leaderElection, you can specify `leaseDuration`, `renewDeadline` and/or `retryPeriod`. Each value must be in an acceptable time.ParseDuration format.(Ref: https://pkg.go.dev/flag#Duration) |
| sidecars.provisionerImage.repository | string | `"registry.k8s.io/sig-storage/csi-provisioner"` |  |
| sidecars.provisionerImage.resources | object | `{}` |  |
| sidecars.provisionerImage.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| sidecars.provisionerImage.securityContext.readOnlyRootFilesystem | bool | `true` |  |
| sidecars.provisionerImage.securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| sidecars.provisionerImage.tag | string | `"v5.1.0"` |  |
| sidecars.resizerImage.additionalArgs | list | `[]` |  |
| sidecars.resizerImage.additionalClusterRoleRules | string | `nil` | Grant additional permissions to csi-resizer |
| sidecars.resizerImage.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.resizerImage.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.resizerImage.httpEndpointPort | string | `"8092"` | Port of the http endpoint |
| sidecars.resizerImage.leaderElection | object | `{}` | Customize leaderElection, you can specify `leaseDuration`, `renewDeadline` and/or `retryPeriod`. Each value must be in an acceptable time.ParseDuration format.(Ref: https://pkg.go.dev/flag#Duration) |
| sidecars.resizerImage.repository | string | `"registry.k8s.io/sig-storage/csi-resizer"` |  |
| sidecars.resizerImage.resources | object | `{}` | Sidecar resources. If not set, the top-level resources will be used. |
| sidecars.resizerImage.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| sidecars.resizerImage.securityContext.readOnlyRootFilesystem | bool | `true` |  |
| sidecars.resizerImage.securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| sidecars.resizerImage.tag | string | `"v1.11.2"` |  |
| sidecars.snapshotterImage.additionalArgs | list | `[]` |  |
| sidecars.snapshotterImage.additionalClusterRoleRules | string | `nil` | Grant additional permissions to csi-snapshotter |
| sidecars.snapshotterImage.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.snapshotterImage.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.snapshotterImage.httpEndpointPort | string | `"8091"` | Port of the http endpoint |
| sidecars.snapshotterImage.leaderElection | object | `{}` | Customize leaderElection, you can specify `leaseDuration`, `renewDeadline` and/or `retryPeriod`. Each value must be in an acceptable time.ParseDuration format.(Ref: https://pkg.go.dev/flag#Duration) |
| sidecars.snapshotterImage.repository | string | `"registry.k8s.io/sig-storage/csi-snapshotter"` |  |
| sidecars.snapshotterImage.resources | object | `{}` | Sidecar resources. If not set, the top-level resources will be used. |
| sidecars.snapshotterImage.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| sidecars.snapshotterImage.securityContext.readOnlyRootFilesystem | bool | `true` |  |
| sidecars.snapshotterImage.securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| sidecars.snapshotterImage.tag | string | `"v8.1.1"` |  |
| timeout | string | `"60s"` | Timeout for sidecars |
| tolerations | list | `[{"key":"CriticalAddonsOnly","operator":"Exists"},{"effect":"NoExecute","operator":"Exists","tolerationSeconds":300}]` | Pod tolerations |
| updateStrategy | object | `{"rollingUpdate":{"maxUnavailable":1},"type":"RollingUpdate"}` | Controller deployment update strategy. |
| verbosity | int | `3` | Verbosity level of the plugin |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.11.0](https://github.com/norwoodj/helm-docs/releases/v1.11.0)
