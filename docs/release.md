# Release Process

## Helm release

1. In [CHANGELOG-1.x.md](CHANGELOG-1.x.md), add a new vX.Y.Z-helm version
2. Update the chart and driver versions in `helm/osc-bsu-csi-driver/Chart.yaml`
3. Update the driver version in `helm/osc-bsu-csi-driver/values.yaml`
4. Generate helm docs:
```shell
make helm-docs
```
5. Open a pull request with the above changes and merge it once approved.
6. Tag and push the Helm chart release:
```shell
export HELM_VERSION=vX.Y.Z-helm
git tag -a $HELM_VERSION -m "🔖 Helm $HELM_VERSION"
git push origin $HELM_VERSION
```
7. Publish the Github release

## Container release

1. In [CHANGELOG-1.x.md](CHANGELOG-1.x.md), add a new vX.Y.Z version
2. Update `docs/README.md`
 - Update the version of the plugin
 - Update the CSI spec version
 - Update the kubernetes minimal version
3. Open a pull request with the above changes and merge it once approved.
4. Tag and push the container release:
```shell
export VERSION=vX.Y.Z
git tag -a $VERSION -m "🔖 CSI $VERSION"
git push origin $VERSION
```
5. Publish the Github release
