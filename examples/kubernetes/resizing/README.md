## Volume Resizing
This example shows how to resize BSU persistence volume using volume resizing features.

**Note**
- BSU has a limitation with volume modification only when state is available. Refer to [API documentation](https://docs.outscale.com/api#updatevolume) for more details.

## Usage
1. Add `allowVolumeExpansion: true` in the StorageClass spec in [example manifest](./spec/sc.yaml) to enable volume expansion. You can only expand a PVC if its storage class’s allowVolumeExpansion field is set to true

2. Deploy the example:

```
kubectl apply -f specs/
``` 

3. Verify the volume is created:

```
kubectl get pv 
```

4. Expand the volume size by increasing the capacity in PVC's `spec.resources.requests.storage`:
```
 kubectl edit pvc -n resizing-p ebs-claim
```
Save the result at the end of the edit.

5. Verify that the persistence volume are resized:
```
kubectl get pv
```
You should see the new value relfected in the capacity fields.


7. Cleanup resources:
```
kubectl delete -f specs/
```
