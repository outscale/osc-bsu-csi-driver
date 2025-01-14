# Dynamic Volume Provisioning
This example shows how to create a BSU volume that will be encrypted and consume it from container dynamically.

## Prerequisites

1. Kubernetes 1.20+ (CSI 1.5.0).
2. The [osc-bsu-csi-driver driver](https://github.com/outscale/osc-bsu-csi-driver) is installed.

## Usage

1. Create a sample app along with the StorageClass and the PersistentVolumeClaim:
```
kubectl apply -f specs/
```

2. Validate the volume was created and `volumeHandle` contains an BSU volumeID:
```
kubectl describe pv
```

3. Validate the pod successfully wrote data to the volume:
```
kubectl exec -it app cat /data/out.txt
```

4. Cleanup resources:
```
kubectl delete -f specs/
```
