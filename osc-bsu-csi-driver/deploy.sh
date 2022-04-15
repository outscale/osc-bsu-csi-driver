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
     --namespace kube-system --set enableVolumeScheduling=true \
     --set enableVolumeResizing=true --set enableVolumeSnapshot=true \
     --set region=$REGION \
    --set image.repository=$IMAGE_NAME \
    --set image.tag=$IMAGE_TAG
