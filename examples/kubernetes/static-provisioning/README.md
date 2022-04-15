# Static Provisioning 
This example shows how to create and consume persistence volume from exising BSU using static provisioning. 

## Usage
1. Edit the PersistentVolume spec in [example manifest](./specs/example.yaml). Update `volumeHandle` with BSU volume ID that you are going to use, and update the `fsType` with the filesystem type of the volume. In this example, I have a pre-created BSU  volume in eu-west-2 availability zone and it is formatted with xfs filesystem.

```
apiVersion: v1
kind: PersistentVolume
metadata:
  name: test-pv
spec:
  capacity:
    storage: 50Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteOnce
  storageClassName: bsu-sc
  csi:
    driver: bsu.csi.outscale.com
    volumeHandle: {volumeId} 
    fsType: xfs
  nodeAffinity:
    required:
      nodeSelectorTerms:
      - matchExpressions:
        - key: topology.bsu.csi.outscale.com/zone
          operator: In
          values:
          - eu-west-2a
```
Note that node affinity is used here since BSU volume is created in us-east-1c, hence only node in the same AZ can consume this persisence volume. 

2. Deploy the example:
```sh
kubectl apply -f specs/
```

3. Verify application pod is running:
```sh
kubectl describe po app
```

4. Validate the pod successfully wrote data to the volume:
```sh
kubectl exec -it app cat /data/out.txt
```

5. Cleanup resources:
```sh
kubectl delete -f specs/
```
