# Development
Please go through [CSI Spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) and [General CSI driver development guideline](https://kubernetes-csi.github.io/docs/introduction.html?highlight=Deve#development-and-deployment) to get some basic understanding of CSI driver before you start.

## Requirements
* Golang 1.17.6
* Docker 20.10.5+ for releasing

## Dependency
Dependencies are managed through go module. To build the project, first turn on go mod using `export GO111MODULE=on`, then build the project using: `make build-image`

## Release
See [Release Process](release.md)

##  Testing
* To execute all unit tests, run: `make test`
* To execute e2e single az tests, run: 
```
    export OSC_ACCESS_KEY=XXX
	export OSC_SECRET_KEY=XXX
	export KC=$(base64 -w 0 path/to/kube_config.yaml)
    make test-e2e-single-az
```
