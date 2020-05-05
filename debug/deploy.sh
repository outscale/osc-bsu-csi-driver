#!/bin/bash
set -x -e

container=$1

echo "Test build with container"$container
docker exec -it $container make 
IMAGE_SECRET=registry-dockerconfigjson
IMAGE_NAME=registry.kube-system:5001/osc/cloud-provider-osc
IMAGE_TAG=dev
SECRET_NAME=osc-secret
make build-image IMAGE_VERSION=$IMAGE_TAG
make tag-image IMAGE_VERSION=$IMAGE_TAG
make push-release IMAGE_VERSION=$IMAGE_TAG
helm del --purge k8s-osc-ccm --tls || true
helm install --name k8s-osc-ccm --set imagePullSecrets=$IMAGE_SECRET  \
	--set oscSecretName=$SECRET_NAME --set image.repository=$IMAGE_NAME  \
	--set image.tag=$IMAGE_TAG deploy/k8s-osc-ccm --tls

