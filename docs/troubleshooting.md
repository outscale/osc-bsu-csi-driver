# Troubleshooting

## Shell access to the CSI pod

A busybox shell is available for troubleshooting. You can connect to a CSI pod using:

```
kubectl exec -it [pod name] -c osc-plugin -n kube-system -- /busybox/sh
```

Filesystem management tools are available on CSI node pods.

## XFS

To use XFS volumes, XFS must be enabled on the worker nodes host kernel.
XFS support is included in most recent Linux distributions.

You can check XFS module support with the following commands:

### Check if the XFS module is loaded
`lsmod | grep xfs`

### Load the XFS module if it is not loaded
`sudo modprobe xfs`
