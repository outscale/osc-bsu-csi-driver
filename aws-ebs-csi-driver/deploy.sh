#!/bin/sh

set -euo pipefail

if [[ "${IMAGE_NAME}" == "" ]]; then
	IMAGE_NAME=registry.kube-system:5001/osc/osc-ebs-csi-driver
fi

if [[ "${IMAGE_TAG}" == "" ]]; then
	IMAGE_TAG=latest
fi

helm uninstall osc-bsu-csi-driver  --namespace kube-system

helm install osc-bsu-csi-driver ./aws-ebs-csi-driver \
     --namespace kube-system --set enableVolumeScheduling=true \
     --set enableVolumeResizing=true --set enableVolumeSnapshot=true \
     --set region=eu-west-2 \
    --set image.repository=$IMAGE_NAME \
    --set image.tag=$IMAGE_TAG
