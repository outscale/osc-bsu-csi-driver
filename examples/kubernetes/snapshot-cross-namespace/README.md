# Snapshot Cross Namespace

## Overview
* [Kubernetes Blog – Cross-Namespace Data Sources (Alpha)](https://kubernetes.io/blog/2023/01/02/cross-namespace-data-sources-alpha)
* [CSI Documentation – Cross-Namespace Data Sources](https://kubernetes-csi.github.io/docs/cross-namespace-data-sources.html)

This feature enables cross-namespace volume snapshot restoration, allowing a PersistentVolumeClaim (PVC) to reference and restore from a snapshot located in a different namespace.

## Prerequisites

This feature is available **only in Kubernetes versions >1.26**.

### 1. Enable Volume Snapshot Support

Follow the instructions [here](../snapshot/README.md) to enable VolumeSnapshot functionality in your cluster.

### 2. Add the CRD for `ReferenceGrants`

Check whether the `ReferenceGrants` resource is available:
```bash
kubectl get referencegrants.gateway.networking.k8s.io -A
```

If you see an error like:
```bash
error: the server doesn't have a resource type "referencegrants"
```

You need to install the required CRD:
```bash
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.3.0/standard-install.yaml
```

### 3. Configure kube-apiserver and kube-controller-manager

In current Kubernetes versions (<=1.33), the following feature is **not enabled by default**:
    
- `CrossNamespaceVolumeDataSource`

You must manually enable it by setting the following flag on both the **kube-apiserver** and **kube-controller-manager**:

```bash
--feature-gates=CrossNamespaceVolumeDataSource=true
```

### 4. Deploy the Helm Chart with enableSnapshotCrossNamespace=true
```bash
helm install --upgrade osc-bsu-csi-driver oci://docker.io/outscalehelm/osc-bsu-csi-driver \
  --namespace kube-system \
  --set enableVolumeSnapshot=true \
  --set enableSnapshotCrossNamespace=true \   # Enable cross-namespace snapshot support
  --set region=$OSC_REGION
```

## Usage
1. **Create the required classes**:
  * [StorageClass](specs/storageclass.yaml)
  * [VolumeSnapshotClass](specs/volumesnapshotclass.yaml) _(if needed)_

2. **Create a source PVC** in the **source namespace**:
  [PersistentVolumeClaim](specs/persistentvolumeclaim-source.yaml)

3. **Create a snapshot** in the source namespace:
  [VolumeSnapshot](specs/volumesnapshot.yaml)

    Wait until the `ReadyToUse` attribute is true
    ```bash
    kubectl -n my-source-namespace get volumesnapshot my-source-snapshot -o jsonpath='ReadyToUse: {.status.readyToUse}{"\n"}'
    ```

4. **Create a `ReferenceGrant`** in the **source namespace** to allow access from the destination namespace:
  [ReferenceGrant](specs/referencegrant.yaml)

    ⚠️ This step is crucial. If the source and destination namespaces or names are incorrect, you'll encounter an error like:
    ```yaml
    failed to provision volume with StorageClass "gp2-delete-immediate": accessing my-source-namespace/my-source-snapshot of VolumeSnapshot dataSource from my-destination-namespace/my-destination-pvc isn't allowed
    ```

5. **Create the destination PVC** referencing the snapshot in the **destination namespace**:
  [PersistentVolumeClaim](specs/persistentvolumeclaim-destination.yaml)
