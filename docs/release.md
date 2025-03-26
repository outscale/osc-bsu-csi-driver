# Release Process
1. Update `CHANGELOG-1.x.md`
2. Update chart version (if necessary) in `osc-bsu-csi-driver/Chart.yaml` and driver version in  `osc-bsu-csi-driver/values.yaml`
3. Update `docs/README.md`
 - Update the version of the plugin
 - Update the CSI spec version
 - Update the kubernetes minimal and recommended version
4. Generate helm doc `make helm-docs`
5. Tag the release
```shell
git tag -a vX.X.X -m "🔖 Release vX.X.X"
```
6. Generate the docker image with `make build-image`
7. Push the docker image to the registry
8. Make the release on Github 