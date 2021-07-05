# Configuring StorageClass
This example shows how to configure Kubernetes storageclass to provision BSU volumes with various configuration parameters. For the full BSU plugin parameters, please refer to [StorageClass Parameter](../docs/sc.md).

## Usage
1. Edit the StorageClass spec in [example manifest](./specs/example.yaml) and update storageclass parameters to desired value. In this example, a `io1` BSU volume will be created and formatted to `xfs`.

2. Deploy the example:
```sh
kubectl apply -f specs/
```

3. Verify the volume is created:
```sh
kubectl describe pv
```

4. Validate the pod successfully wrote data to the volume:
```sh
kubectl exec -it app cat /data/out.txt
```

5. Cleanup resources:
```sh
kubectl delete -f specs/
```
