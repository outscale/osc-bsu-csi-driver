# Development

Resources regarding developping this ccm:
- [Cloud Controller Manager architecture](https://kubernetes.io/docs/concepts/architecture/cloud-controller/)
- [Developing Cloud Controller Manager](https://kubernetes.io/docs/tasks/administer-cluster/developing-cloud-controller-manager/)
- [Interfaces](https://github.com/kubernetes/cloud-provider/blob/master/cloud.go)
- [About legacy providers with CCM](https://github.com/kubernetes/legacy-cloud-providers)

# Pre-requisites

You will need a Kubernetes cluster to launch some tests and debug some behaviors.
You can use [osc-k8s-rke-cluster](https://github.com/outscale-dev/osc-k8s-rke-cluster/) for this purpose.

You will also need a registry where to push your dev images. You can use whatever registry you want or install [a private registry](https://github.com/outscale-dev/osc-k8s-rke-cluster/tree/master/addons/docker-registry) which is available with osc-k8s-rke-cluster.

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

Dependencies are managed through go module. To build the project, first turn on go mod using `export GO111MODULE=on`, then build the project using: `VERSION=dev make build-image`

# Push dev image to registry

If you are using the [private registry addon](https://github.com/outscale-dev/osc-k8s-rke-cluster/tree/master/addons/docker-registry), start port-fowarding to access the registry:
```
./start_port_forwarding.sh
```

You can then push your dev image to your registry:
```
docker tag osc/cloud-provider-osc:dev localhost:4242/osc/cloud-provider-osc:dev
docker push localhost:4242/osc/cloud-provider-osc:dev
```

# Deploying

Make sure to copy, edit and deploy your own [secrets.yml](../deploy/secrets.example.yml):
```
kubectl apply -f deploy/secrets.yaml
```

Install/upgrade your CCM with your "dev" image:
```
helm upgrade --install --wait --wait-for-jobs k8s-osc-ccm deploy/k8s-osc-ccm --set oscSecretName=osc-secret --set image.repository=10.0.1.10:32500/osc/cloud-provider-osc --set image.tag=dev
```

Note that `10.0.1.10:32500` is provided by `start_port_forwarding.sh` script.

Check that CCM is deployed with:
```
kubectl get pod -n kube-system -l "app=osc-cloud-controller-manager"
```

# Testing

* To execute all unit tests, run: `make test`
* To execute e2e single az tests, run: 
```bash
export OSC_ACCESS_KEY=YourSecretAccessKeyId
export OSC_SECRET_KEY=YourSecretAccessKey
export E2E_REGION="us-east-2" # default is "eu-west-2"
export E2E_AZ="us-east-2a" # default "eu-west-2a"
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