#!/bin/sh

set -euo pipefail

IMAGE_SECRET=registry-dockerconfigjson
SECRET_NAME=osc-secret


if [[ "${IMAGE_NAME}" == "" ]]; then
	IMAGE_NAME=registry.kube-system:5001/osc/osc-ebs-csi-driver
fi

if [[ "${IMAGE_TAG}" == "" ]]; then
	IMAGE_TAG=latest
fi


helm del --purge aws-ebs-csi-driver --tls
helm install --name aws-ebs-csi-driver \
            --set enableVolumeScheduling=true \
            --set enableVolumeResizing=true \
            --set enableVolumeSnapshot=true \
            --set image.repository=$IMAGE_NAME \
            --set image.tag=$IMAGE_TAG \
            ./aws-ebs-csi-driver --tls
