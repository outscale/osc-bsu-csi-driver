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

curl https://raw.githubusercontent.com/outscale/osc-bsu-csi-driver/master/deploy/kubernetes/secret.yaml | \
    sed "s/secret_key: \"\"/secret_key: \"$OSC_SECRET_KEY\"/g" | \
    sed "s/access_key: \"\"/access_key: \"$OSC_ACCESS_KEY\"/g" > osc-secret.yaml
kubectl delete -f osc-secret.yaml --namespace=kube-system
kubectl apply -f osc-secret.yaml --namespace=kube-system
```

## Install the driver

```shell
helm install --upgrade osc-bsu-csi-driver oci://docker.io/outscalehelm/osc-bsu-csi-driver \
    --namespace kube-system \
    --set enableVolumeScheduling=true \
    --set enableVolumeResizing=true \
    --set enableVolumeSnapshot=true \
    --set region=$OSC_REGION
```

> **_NOTE:_** If region is not defined, the controller will need to access the [metadata server](https://docs.outscale.com/en/userguide/Accessing-the-Metadata-and-User-Data-of-an-Instance.html) in order to get information. Access to `169.254.169.254/32` on TCP port 80 (http) must be allowed.

## Check that pods are running

```shell
kubectl get pod -n kube-system -l "app.kubernetes.io/name=osc-bsu-csi-driver"
```
