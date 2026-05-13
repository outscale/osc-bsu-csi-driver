# Deployment

## Deploy the CSI CRDs

```shell
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.3/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.3/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.3/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.3/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-8.3/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml
```

## Add credentials

```shell
export OSC_ACCESS_KEY=XXXXX
export OSC_SECRET_KEY=XXXXX
export OSC_REGION=XXXXX

kubectl create secret generic osc-csi-bsu \
    --from-literal=access_key=$OSC_ACCESS_KEY --from-literal=secret_key=$OSC_SECRET_KEY \
    -n kube-system
```

## Install the driver

```shell
helm upgrade --install osc-bsu-csi-driver oci://docker.io/outscalehelm/osc-bsu-csi-driver \
    --namespace kube-system \
    --set driver.enableVolumeSnapshot=true \
    --set cloud.region=$OSC_REGION
```

> **_NOTE:_** If region is not defined, the controller will need to access the [metadata server](https://docs.outscale.com/en/userguide/Accessing-the-Metadata-and-User-Data-of-an-Instance.html) in order to get information. Access to `169.254.169.254/32` on TCP port 80 (http) must be allowed.

## Check that pods are running

```shell
kubectl get pod -n kube-system -l "app.kubernetes.io/name=osc-bsu-csi-driver"
```

## Setting volume limits

Up to 39 volumes, including PVCs and host-level mounts, can be attached to a node.

The CSI driver automatically counts how many volumes are mounted by the host and reports the calculated volume limit to Kubernetes.

If the automatic calculation is not suitable, you can set a manual limit.

### Reserving slots for host-level mounts

If you need to mount additional volumes at the host level, you may configure the `driver.reservedBsuVolumes` Helm value with the number of expected host-level mounts (excluding the root volume).

The limit reported by the CSI driver will be:

`39 - max(number of mounted volumes, driver.reservedBsuVolumes)`

### Global limit

The `driver.maxBsuVolumes` Helm value can be used to set a global limit. All nodes will use this value.

> Note: `driver.reservedBsuVolumes` is ignored when setting `driver.maxBsuVolumes`.

### Per node limit (v1.11.0 and later)

The `bsu.csi.outscale.com/maxvolumes` annotation can be set on nodes and its value will be used as the limit for the corresponding node.

If both limits are set, the annotation limit takes precedence.

> Note: `driver.reservedBsuVolumes` is ignored when setting an annotation limit.

### Dynamic values (v1.11.0 and later)

If you activate the [`MutableCSINodeAllocatableCount` feature gate](https://kubernetes.io/blog/2025/05/02/kubernetes-1-33-mutable-csi-node-allocatable-count/), Kubernetes periodically refreshes the limit by asking the CSI driver to compute a new automatic limit and check for an updated node annotation.
