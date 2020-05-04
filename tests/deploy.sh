#!/bin/bash

IMAGE_SECRET=registry-dockerconfigjson
SECRET_NAME=osc-secret

IMAGE_NAME=
if [[ "${IMAGE_NAME}" == "" ]]; then
	IMAGE_NAME=registry.kube-system:5001/osc/cloud-provider-osc
fi

if [[ "${IMAGE_TAG}" == "" ]]; then
	IMAGE_TAG=dev
fi


helm del --purge k8s-osc-ccm --tls || true
helm install --name k8s-osc-ccm --set  \
	imagePullSecrets=$IMAGE_SECRET  \
	--set oscSecretName=$SECRET_NAME  \
	--set image.repository=$IMAGE_NAME  \
	--set image.tag=$IMAGE_TAG ./deploy/k8s-osc-ccm --tls
