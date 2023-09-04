# osc-bsu-csi-driver

![Version: 1.3.0](https://img.shields.io/badge/Version-1.3.0-informational?style=flat-square) ![AppVersion: v1.2.3](https://img.shields.io/badge/AppVersion-v1.2.3-informational?style=flat-square)

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
| affinity | object | `{}` | Affinity settings   |
| backoff.duration | string | `"1"` | Initial duraction of backoff    |
| backoff.factor | string | `"1.9"` | Factor multiplied by Duration for each iteration |
| backoff.steps | string | `"20"` | Remaining number of iterations in which the duration parameter may change |
| caBundle.key | string | `""` | Entry key in secret used to store additional certificates authorities |
| caBundle.name | string | `""` | Secret name containing additional certificates authorities |
| credentials.accessKey | string | `nil` | If creating a secret, put this AK inside. |
| credentials.create | bool | `false` | Actually create a secret in the deployment for AK/SK (else, only reference it) |
| credentials.secretKey | string | `nil` | If creating a secret, put this SK inside. |
| credentials.secretName | string | `"osc-csi-bsu"` | Use AK/SK from this secret  |
| csiDriver.fsGroupPolicy | string | `"File"` | Policy of the FileSystem (see [Docs](https://kubernetes-csi.github.io/docs/support-fsgroup.html#supported-modes)) |
| customEndpoint | string | `""` | Use customEndpoint (url with protocol) ex: https://api.eu-west-2.outscale.com/api/v1 |
| defaultFsType | string | `"ext4"` | Default filesystem for the volume if no `FsType` is set in `StorageClass` |
| enableVolumeResizing | bool | `false` | Enable volume resizing  True if enable volume resizing |
| enableVolumeScheduling | bool | `true` | Enable schedule volume for dynamic volume provisioning True if enable volume scheduling for dynamic volume provisioning |
| enableVolumeSnapshot | bool | `false` | Enable volume snapshot  True if enable volume snapshot |
| extraCreateMetadata | bool | `false` | Add pv/pvc metadata to plugin create requests as parameters |
| extraVolumeTags | object | `{}` | Add extra tags on volume |
| httpsProxy | string | `""` | Value used to create environment variable HTTPS_PROXY |
| image.pullPolicy | string | `"IfNotPresent"` | Container pull policy |
| image.repository | string | `"outscale/osc-bsu-csi-driver"` | Container image to use    |
| image.tag | string | `"v1.2.3"` | Container image tag to deploy |
| imagePullSecrets | list | `[]` | Specify image pull secrets  |
| maxBsuVolumes | string | `"39"` | Maximum volume to attach to a node (see [Docs](https://docs.outscale.com/en/userguide/About-Volumes.html)) |
| nameOverride | string | `""` | Override name of the app (instead of `osc-bsu-csi-driver`) |
| noProxy | string | `""` | Value used to create environment variable NO_PROXY |
| node.podAnnotations | object | `{}` | Annotations for controller pod |
| node.podLabels | object | `{}` | Labels for controller pod |
| node.tolerations | list | `[]` | Pod tolerations |
| nodeSelector | object | `{}` |  |
| podAnnotations | object | `{}` | Annotations for controller pod |
| podLabels | object | `{}` | Labels for controller pod |
| region | string | `""` | Region to use, otherwise it will be looked up via metadata. By providing this parameter, the controller will not require to access the metadata. |
| replicaCount | int | `2` | Number of replicas to deploy |
| resources | object | `{}` | Specify limits of resources used by the pod |
| serviceAccount.controller.annotations | object | `{}` | Annotations to add to the Controller ServiceAccount |
| serviceAccount.snapshot.annotations | object | `{}` | Annotations to add to the Snapshot ServiceAccount |
| sidecars.attacherImage.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.attacherImage.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.attacherImage.httpEndpointPort | string | `"8090"` | Port of the http endpoint |
| sidecars.attacherImage.repository | string | `"registry.k8s.io/sig-storage/csi-attacher"` |  |
| sidecars.attacherImage.tag | string | `"v3.3.0"` |  |
| sidecars.livenessProbeImage.port | string | `"9808"` | Port of the liveness of the main container |
| sidecars.livenessProbeImage.repository | string | `"registry.k8s.io/sig-storage/livenessprobe"` |  |
| sidecars.livenessProbeImage.tag | string | `"v2.5.0"` |  |
| sidecars.nodeDriverRegistrarImage.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.nodeDriverRegistrarImage.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.nodeDriverRegistrarImage.httpEndpointPort | string | `"8093"` | Port of the http endpoint |
| sidecars.nodeDriverRegistrarImage.repository | string | `"registry.k8s.io/sig-storage/csi-node-driver-registrar"` |  |
| sidecars.nodeDriverRegistrarImage.tag | string | `"v2.3.0"` |  |
| sidecars.provisionerImage.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.provisionerImage.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.provisionerImage.httpEndpointPort | string | `"8089"` | Port of the http endpoint |
| sidecars.provisionerImage.repository | string | `"registry.k8s.io/sig-storage/csi-provisioner"` |  |
| sidecars.provisionerImage.tag | string | `"v3.0.0"` |  |
| sidecars.resizerImage.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.resizerImage.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.resizerImage.httpEndpointPort | string | `"8092"` | Port of the http endpoint |
| sidecars.resizerImage.repository | string | `"registry.k8s.io/sig-storage/csi-resizer"` |  |
| sidecars.resizerImage.tag | string | `"v1.3.0"` |  |
| sidecars.snapshotterImage.enableHttpEndpoint | bool | `false` | Enable http endpoint to get metrics of the container |
| sidecars.snapshotterImage.enableLivenessProbe | bool | `false` | Enable liveness probe for the container |
| sidecars.snapshotterImage.httpEndpointPort | string | `"8091"` | Port of the http endpoint |
| sidecars.snapshotterImage.repository | string | `"registry.k8s.io/sig-storage/csi-snapshotter"` |  |
| sidecars.snapshotterImage.tag | string | `"v4.2.1"` |  |
| timeout | string | `"60s"` | Timeout for sidecars |
| tolerations | list | `[]` | Pod tolerations |
| verbosity | int | `10` | Verbosity level of the plugin |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.11.0](https://github.com/norwoodj/helm-docs/releases/v1.11.0)
