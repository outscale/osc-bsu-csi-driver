# Release Process
1. Update `CHANGELOG-0.x.md`
2. Update chart version (if necessary) in `osc-bsu-csi-driver/Chart.yaml` and driver version in  `osc-bsu-csi-driver/values.yaml`
3. Update `docs/README.md`
 - Update the version of the plugin
 - Update the CSI spec version
 - Update the kubernetes mininmal and recommended version
4. Tag the release
```shell
git tag -a vX.X.X -m "Release vX.X.X"
```
5. Generate the docker image with `make build-image`
6. Push the ocker image to the registry
7. Make the release on Github 