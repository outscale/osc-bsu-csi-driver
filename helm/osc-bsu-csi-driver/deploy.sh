#!/bin/sh

set -euo pipefail

if [[ "${IMAGE_NAME}" == "" ]]; then
	IMAGE_NAME=osc/osc-bsu-csi-driver
fi

if [[ "${IMAGE_TAG}" == "" ]]; then
	IMAGE_TAG=latest
fi

if [[ "${REGION}" == "" ]]; then
	REGION=eu-west-2
fi

helm uninstall osc-bsu-csi-driver  --namespace kube-system

helm install osc-bsu-csi-driver ./osc-bsu-csi-driver \
     --namespace kube-system --set drive.enableVolumeSnapshot=true \
     --set cloud.region=$REGION \
     --set driver.image=$IMAGE_NAME \
     --set driver.tag=$IMAGE_TAG
