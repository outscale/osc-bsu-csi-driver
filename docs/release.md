# Release Process
1. Update `CHANGELOG-1.x.md`
2. Update chart version & driver version in `osc-bsu-csi-driver/Chart.yaml` and driver version in  `osc-bsu-csi-driver/values.yaml`
3. Update `docs/README.md`
 - Update the version of the plugin
 - Update the CSI spec version
 - Update the kubernetes minimal and recommended version
4. Generate helm doc `make helm-docs`
5. PR with the above changes, and, once approved a merged
6. Tag the release
```shell
git tag -a vX.X.X -m "🔖 Release vX.X.X"
```
7. Push the tag
```shell
git push origin vX.X.X
```
8. Publish the Github release