#!/bin/bash
set -euo pipefail

make int-test-image
docker run -it  \
            -e OSC_ACCOUNT_ID=$OSC_ACCOUNT_ID \
            -e OSC_ACCOUNT_IAM=$OSC_ACCOUNT_IAM \
            -e OSC_USER_ID=$OSC_USER_ID \
            -e OSC_ARN=$OSC_ARN \
            -e AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID \
            -e AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY \
            -e AWS_DEFAULT_REGION=$AWS_DEFAULT_REGION \
            --name int-test-csi \
            --rm osc/osc-ebs-csi-driver-int:latest
