## Storage Classes
This [document](https://kubernetes.io/docs/concepts/storage/storage-classes/) describes the concept of a StorageClass in Kubernetes

## StorageClass Resource 
This [document](https://kubernetes.io/docs/concepts/storage/storage-classes/#the-storageclass-resource)  describe StorageClass fields.

## Parameters
Storage Classes have parameters that describe volumes belonging to the storage class.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: slow
provisioner: bsu.csi.outscale.com
parameters:
  type: io1
  iopsPerGB: "10"
  csi.storage.k8s.io/fstype: ext4
```

* `type`: `standard`, `gp2`, `io1`. See
  [Outscale docs](https://docs.outscale.com/en/userguide/About-Volumes.html#AboutVolumes-VolumeTypesVolumeTypesandIOPS)
  for details. Default: `gp2`.
* `iopsPerGB`: only for `io1` volumes. I/O operations per second per GiB. 
  [Outscale docs](https://docs.outscale.com/en/userguide/About-Volumes.html#AboutVolumes-VolumeTypesVolumeTypesandIOPS).
  A string is expected here, i.e. `"10"`, not `10`.
* `fsType`: fsType that is supported by kubernetes. Default: `"ext4"`.
