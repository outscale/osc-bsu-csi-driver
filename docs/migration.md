# Migration from v0.X.X to 1.0.0

Starting with 1.0.0, we have made one breaking change:
- renaming the plugin from `ebs.csi.aws.com` to `bsu.csi.outscale.com`

To make things smooth, here a guide to migrate from the two versions

## Prerequisites
Having a 0.X.X CSI driver installed
## Installation
> **Note**: You should be able to install both 0.X.X and 1.0.0 plugins as drivers names differ

The difference between the installation explained in the [Deploy](./deploy.md) for a standard upgrade and for this migration is that we need to change the `liveness` port of the `csi-node` pod. To do that, change the name of the helm (do not use `osc-bsu-csi-driver` if the old version already use it) add the following in the helm command
```shell
--set sidecars.livenessProbeImage.port=9809
```

This is to ensure that it would not have conflict between pods from both plugins.

## Migrate the Volume
The next step is to migrate all volume from using the old csi driver to the new one. The approach that we will explain is to make snapshot from the previous volumeand create a new one from the snapshot.

1. Scale all the application to 0
> **_Warning:_** Before shutting down the pod that uses volumes, you need to make sure that the `Retain Policy` is set to `Retain`
2. Create the `Snapshot Class`
   > See this [example](../examples/kubernetes/snapshot/specs/classes/storageclass.yaml)
3. Make a snapshot from the PVC
   > Change the `persistentVolumeClaimName` from this example [example](../examples/kubernetes/snapshot/specs/snapshot/snapshot.yaml)
4. Create the PVC from snapshot
   > Change the `dataSource.name` from this [example](../examples/kubernetes/snapshot/specs/snapshot-restore/claim.yaml) 
5. Change the PVC for the pod and scale up again

6. Once all the volume have been migrated and you check that it work
   - Check that the `ClaimName` in the pod (`kubectl describe pod <POD_NAME> -n <NAMESPACE>`) is the new one
   - Check that the all new PVs use the new StorageClass (`kubectl get pv`)

7. Remove all previous PVCs of the old `StorageClass`
8. Remove the old `StorageClass`
9. Uninstall the old version driver with 
   ```shell
   helm uninstall osc-bsu-csi-driver --namespace kube-system
   ```

## Migrating PVC using Korb

[korb](https://github.com/BeryJu/korb) can be used to ease your PVC migration.
This method is easier to run but may take time as it performs data copy.

Steps:
1. Have CSI v0 and v1 installed 
2. Stop pod from using your pvc
3. Run korb to migrate your data to a pvc with the same name but with the new storage class
4. Start pod

As an example (tested with `examples/kubernetes/dynamic-provisioning`), we are migrating `ebs-claim` PVC using old storage class `ebs-sc` to the new storage class `bsu-sc`:
```bash
korb ebs-claim \
    --kube-config ~/your/kube_config_cluster.yml \
    --new-pvc-storage-class bsu-sc \
    --source-namespace dynamic-p \
    --strategy copy-twice-name
```

Note: The `copy-twice-name` strategy will copy the PVC to the new Storage class and with new size and a new name, delete the old PVC, and copy it back to the old name.