#!/bin/bash
set -e
echo "Test build with container"$container
docker exec -it $container make 
IMAGE_SECRET=registry-dockerconfigjson
IMAGE_NAME=registry.kube-system:5001/osc/cloud-provider-osc  
IMAGE_TAG=v1 
SECRET_NAME=osc-secret
make build-image IMAGE_VERSION=v1
make tag-image IMAGE_VERSION=v1
make push-release IMAGE_VERSION=v1
helm del --purge k8s-osc-ccm --tls || true
helm install --name k8s-osc-ccm --set imagePullSecrets=$IMAGE_SECRET  \
	--set oscSecretName=$SECRET_NAME --set image.repository=$IMAGE_NAME  \
	--set image.tag=$IMAGE_TAG deploy/k8s-osc-ccm --tls

