# Configuring VolumeAttributeClass

This example shows how to modify a volume using VolumeAttributeClass.
It requires Kubernetes 1.31 or above.

## Usage

Deploy the example:
```sh
kubectl apply -f specs/
```

Update the pvc:
```yaml
spec:
  volumeAttributesClass: bsu-vac-after
```
