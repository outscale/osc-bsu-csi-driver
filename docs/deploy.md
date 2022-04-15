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
```shell
# ENV VARS 
export OSC_ACCESS_KEY=XXXXX
export OSC_SECRET_KEY=XXXXX
export OSC_REGION=XXXXX

## set the secrets
curl https://raw.githubusercontent.com/outscale-dev/osc-bsu-csi-driver/v0.0.15/deploy/kubernetes/secret.yaml > secret.yaml
cat secret.yaml | \
    sed "s/secret_key: \"\"/secret_key: \"$OSC_SECRET_KEY\"/g" | \
    sed "s/access_key: \"\"/access_key: \"$OSC_ACCESS_KEY\"/g" > osc-secret.yaml
kubectl delete -f osc-secret.yaml --namespace=kube-system
kubectl apply -f osc-secret.yaml --namespace=kube-system

## deploy the pod
git clone git@github.com:outscale-dev/osc-bsu-csi-driver.git -b v0.0.15
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