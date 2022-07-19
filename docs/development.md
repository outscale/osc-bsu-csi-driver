# Development

Resources regarding developping this ccm:
- [Cloud Controller Manager architecture](https://kubernetes.io/docs/concepts/architecture/cloud-controller/)
- [Developing Cloud Controller Manager](https://kubernetes.io/docs/tasks/administer-cluster/developing-cloud-controller-manager/)
- [Interfaces](https://github.com/kubernetes/cloud-provider/blob/master/cloud.go)
- [About legacy providers with CCM](https://github.com/kubernetes/legacy-cloud-providers)

# Building

`make` provide a quick reminder to all available commands:
```shell
$ make
help:
  - build              : build binary
  - build-image        : build Docker image
  - dockerlint         : check Dockerfile
  - verify             : check code
  - test               : run all tests
  - test-e2e-single-az : run e2e tests
```

Dependencies are managed through go module. To build the project, first turn on go mod using `export GO111MODULE=on`, then build the project using: `make build-image`

# Testing

* To execute all unit tests, run: `make test`
* To execute e2e single az tests, run: 
```bash
export OSC_ACCESS_KEY=YourSecretAccessKeyId
export OSC_SECRET_KEY=YourSecretAccessKey
export KC=$(base64 -w 0 path/to/kube_config.yaml)
make test-e2e
```

# Release

1.  Update [CHANGELOG.md](CHANGELOG.md)
2.  Update chart version (if needed) in [Chart.yaml](../deploy/k8s-osc-ccm/Chart.yaml)
3.  Update cloud-provider-osc version in [values.yaml](../deploy/k8s-osc-ccm/values.yaml)
4.  Update prerequisites section in [deploy/README.md](../deploy/README.md)
5.  Commit version with `git commit -am "cloud-controller-manager vX.Y.Z"`
6.  Make docker image with `make build-image`
7.  Tag commit with `git tag vX.Y.Z`
8.  Push commit and tag on Github
9.  Push the docker image to the registry
10. Make the release on Github