# Deployment
> **_NOTE:_**  Starting from the version v0.0.15, the snapshot-controller and the CRD will no longer be included in the chart. If you need it, you will have to install it manually as following
```
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-5.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-5.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-5.0/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-5.0/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/release-5.0/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml
```

## Steps
> **_NOTE:_**  By default all pods need to be able to access [metadata server](https://docs.outscale.com/en/userguide/Accessing-the-Metadata-and-User-Data-of-an-Instance.html) in order to get information about its machine (region, vmId). To do this, node controller need to be able to access `169.254.169.254/32` through TCP port 80 (http). This metadata server access can be disabled for controller pod by providing in the helm command line the region `--region=<OSC_REGION>`

```shell
# ENV VARS 
export OSC_ACCESS_KEY=XXXXX
export OSC_SECRET_KEY=XXXXX
export OSC_REGION=XXXXX

## set the secrets
curl https://raw.githubusercontent.com/outscale-dev/osc-bsu-csi-driver/v1.0.0/deploy/kubernetes/secret.yaml > secret.yaml
cat secret.yaml | \
    sed "s/secret_key: \"\"/secret_key: \"$OSC_SECRET_KEY\"/g" | \
    sed "s/access_key: \"\"/access_key: \"$OSC_ACCESS_KEY\"/g" > osc-secret.yaml
kubectl delete -f osc-secret.yaml --namespace=kube-system
kubectl apply -f osc-secret.yaml --namespace=kube-system

## deploy the pod
git clone git@github.com:outscale-dev/osc-bsu-csi-driver.git -b v1.0.0
cd osc-bsu-csi-driver
helm uninstall osc-bsu-csi-driver  --namespace kube-system
helm install osc-bsu-csi-driver ./osc-bsu-csi-driver \
    --namespace kube-system \
    --set enableVolumeScheduling=true \
    --set enableVolumeResizing=true \
    --set enableVolumeSnapshot=true \
    --set region=$OSC_REGION
            
## Check the pod is running
kubectl get pods -o wide -A  -n kube-system
```