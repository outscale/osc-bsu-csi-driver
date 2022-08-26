# Volume Snapshots

## Overview

This driver implements basic volume snapshotting functionality using the [external snapshotter](https://github.com/kubernetes-csi/external-snapshotter) sidecar and creates snapshots of BSU volumes using the `VolumeSnapshot` custom resources.

## Prerequisites

1. Kubernetes 1.20+ (CSI 1.5.0).
2. The `VolumeSnapshotDataSource` must be set in `--feature-gates=` in the `kube-apiserver`.
3. The [osc-bsu-csi-driver driver](https://github.com/outscale-dev/osc-bsu-csi-driver) is installed.

### Usage

1. Create the `StorageClass` and `VolumeSnapshotClass`:
```
kubectl apply -f specs/classes/
```

2. Create a sample app and the `PersistentVolumeClaim`: 
```
kubectl apply -f specs/app/
```

3. Validate the volume was created and `volumeHandle` contains an BSU volumeID: 
```
kubectl describe pv
```

4. Validate the pod successfully wrote data to the volume, taking note of the timestamp of the first entry:
```
kubectl exec -it app cat /data/out.txt
```

5. Create a `VolumeSnapshot` referencing the `PersistentVolumeClaim` name:
```
kubectl apply -f specs/snapshot/
```

6. Wait for the `Ready To Use:  true` attribute of the `VolumeSnapshot`: 
```
kubectl describe volumesnapshot.snapshot.storage.k8s.io bsu-volume-snapshot
```

7. Delete the existing app:
```
kubectl delete -f specs/app/
```

8. Restore a volume from the snapshot with a `PersistentVolumeClaim` referencing the `VolumeSnapshot` in its `dataSource`:
```
kubectl apply -f specs/snapshot-restore/
```

9. Validate the new pod has the restored data by comparing the timestamp of the first entry to that of in step 4:
```
kubectl exec -it app cat /data/out.txt
```

10. Cleanup resources:
```
kubectl delete -f specs/snapshot-restore
kubectl delete -f specs/snapshot
kubectl delete -f specs/classes
```
