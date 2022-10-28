#!/bin/bash

set -e

make helm-manifest

git diff --exit-code deploy/osc-ccm-manifest.yml