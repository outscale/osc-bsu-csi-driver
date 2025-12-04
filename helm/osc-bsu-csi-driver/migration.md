# Migration from v1 chart to v2 chart

Most variables have been renamed, and `helm upgrade` will fail if you reuse v1 values.

You can call `helm get values osc-bsu-csi-driver --namespace kube-system` to display configured values.
You will need to set each value again using the new value names.

For example, if you installed the CSI driver using:

```shell
helm install [...] \
    --set enableVolumeSnapshot=true \
    --set region=$OSC_REGION \
    --set verbosity=5
```

You will need to upgrade with:

```shell
helm upgrade [...] \
    --set driver.enableVolumeSnapshot=true \
    --set cloud.region=$OSC_REGION \
    --set logs.verbosity=5
```

## List of renamed values

| V1 value | V2 value |
| ----------- | ----------- |
| affinity | controller.affinity |
| caBundle | cloud.caBundle |
| credentials | cloud.credentials |
| csiDriver.fsGroupPolicy | driver.fsGroupPolicy |
| customEndpoint | cloud.customEndpoint |
| defaultFsType | driver.defaultFsType |
| enableVolumeSnapshot | driver.enableVolumeSnapshot |
| enableSnapshotCrossNamespace | driver.enableSnapshotCrossNamespace |
| enableVolumeAttributesClass | driver.enableVolumeAttributesClass |
| extraVolumeTags | driver.extraVolumeTags |
| extraSnapshotTags | driver.extraSnapshotTags |
| httpsProxy | cloud.httpsProxy |
| image.repository | driver.image |
| image.tag | driver.tag |
| image.pullPolicy | driver.imagePullPolicy |
| nameOverride | driver.name |
| node.containerSecurityContext | node.securityContext |
| node.args | node.additionalArgs |
| nodeSelector | controller.nodeSelector |
| noProxy | cloud.noProxy |
| podAnnotations | controller.podAnnotations |
| podLabels | controller.podLabels |
| region | cloud.region |
| replicaCount | controller.replicas |
| resources | controller.resources |
| sidecars.provisionerImage | sidecars.provisioner |
| sidecars.attacherImage | sidecars.attacher |
| sidecars.snapshotterImage | sidecars.snapshotter |
| sidecars.livenessProbeImage | sidecars.livenessProbe |
| sidecars.resizerImage | sidecars.resizer |
| sidecars.nodeDriverRegistrarImage | sidecars.nodeDriverRegistrar |
| timeout | sidecars.timeout |
| tolerations | controller.tolerations |
| tolerateAllTaints | controller.tolerateAllTaints |
| updateStrategy | controller.updateStrategy |
| verbosity | logs.verbosity |

## Performance tuning

You may now tune the performance of the driver by setting the following values:

| Name | Description |
| -------- | ----------- |
| `controller.readStatusInterval` | The interval between consecutive volume/snapshot checks; raise it if you see throttling errors in ReadSnapshot/ReadVolumes calls. |
| `sidecars.timeout` | The maximum time a sidecar (provisioner, attacher, resizer, snapshotter) will wait for the CSI driver to process a query. Raise it if your volumes/snapshots are very large and you encounter timeouts. |
| `sidecars.kubeAPI.QPS` | The maximum of requests per seconds that a sidecar may make to the Kubernetes API server. |
| `sidecars.kubeAPI.burst` | The burst above `sidecars.kubeAPI.QPS` allowed for short periods of time. |
| `sidecars.provisioner.workerThreads` | The number of simultaneous volume provisioning requests the provisioner sidecar can process. |
| `sidecars.attacher.workerThreads` | The number of simultaneous volume attachment requests the attacher sidecar can process. |
| `sidecars.resizer.workerThreads` | The number of simultaneous volume resizing requests the resizer sidecar can process. |
| `sidecars.snapshotter.workerThreads` | The number of simultaneous snapshot requests the snapshotter sidecar can process. |

> Please be aware that there is a limit in the number of API calls you are allowed to make to the Outscale API.
If you raise `workerThreads` too much, you may decrease the performance of the CSI driver by being throttled by the Outscale API.
Because throttling limits are applied per call, raising `controller.readStatusInterval` will not increase volume and snapshot creation throughput.
