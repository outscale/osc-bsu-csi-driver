# Volume Snapshots

## Overview

This driver implements basic volume snapshotting functionality using the [external snapshotter](https://github.com/kubernetes-csi/external-snapshotter) sidecar and creates snapshots of BSU volumes using the `VolumeSnapshot` custom resources.

## Prerequisites

### 1. Kubernetes 1.21 and Later

In Kubernetes versions 1.21 and later, VolumeSnapshotDataSource has been enabled by default. If your cluster is running Kubernetes 1.21 or higher, you do not need to manually set the --feature-gates=VolumeSnapshotDataSource=true flag, as it should already be enabled. The snapshot feature will work out of the box without the need for additional configuration at the API server level.

### 2. Kubernetes 1.20 and Earlier
In Kubernetes versions 1.20 and earlier, the VolumeSnapshotDataSource feature is not enabled by default, so you would need to manually set the --feature-gates=VolumeSnapshotDataSource=true flag on the API server to enable volume snapshots.

### 3. Install the Latest CRDs for Volume Snapshotting
You can apply the CRDs (if not already installed) using the following commands:

```
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml
```

This will set up the necessary CRDs for volume snapshotting.

### 4. Install the Snapshot Controller

The snapshot controller is responsible for managing snapshots in your cluster. You can install it using the following commands:

```
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.0/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.0/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml
```
This ensures that the snapshot controller is set up correctly to manage volume snapshots and restorations.

### 5. Verify the Configuration

After applying the CRDs and deploying the snapshot controller, ensure everything is set up correctly:

```
kubectl get crds | grep snapshot
```

### 6. Ensure the snapshot controller pods are running

```
kubectl get pods -n kube-system | grep snapshot
```
### 7. Check if the VolumeSnapshotClass and VolumeSnapshot resources are available

```
kubectl get volumesnapshotclass
kubectl get volumesnapshot
```

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
