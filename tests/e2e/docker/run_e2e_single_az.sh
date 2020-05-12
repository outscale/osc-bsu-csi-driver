#!/bin/bash
set -eu pipefail
set -x

MANDATORY_DIR="/e2e-env/.kube/ /etc/kubectl/ /root/aws-ebs-csi-driver"
MANDATORY_DIR=(${MANDATORY_DIR})
for (( dir=0; dir<${#MANDATORY_DIR[@]}; dir++ )); do
	dir_name=${MANDATORY_DIR[${dir}]}
	if [ -z "$(ls -A ${dir_name})" ]; then
		echo "unexpected Empty ${dir_name}"
		exit 1
	fi
done


export PATH=$PATH:/usr/local/go/bin
export GOPATH="/go"
make aws-ebs-csi-driver -j 4
go get -v -u github.com/onsi/ginkgo/ginkgo@v1.11.0
export KUBECONFIG=/e2e-env/.kube/config
export AWS_AVAILABILITY_ZONES=eu-west-2a
export ARTIFACTS=./single_az_test_e2e_report
mkdir -p $ARTIFACTS
export NODES=4

FOCUS_REGEXP="\[ebs-csi-e2e\] \[single-az\]"
SKIP_REGEXP="and encryption"
$GOPATH/bin/ginkgo -dryRun --progress -debug -p -nodes=$NODES \
					-v --focus=${FOCUS_REGEXP} --skip=${SKIP_REGEXP} \
					tests/e2e -- -report-dir=$ARTIFACTS
