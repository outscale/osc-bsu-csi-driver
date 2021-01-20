#!/bin/bash
set -eu pipefail
set -x

MANDATORY_DIR="/e2e-env/.kube/ /root/aws-ebs-csi-driver"
MANDATORY_DIR=(${MANDATORY_DIR})
for (( dir=0; dir<${#MANDATORY_DIR[@]}; dir++ )); do
	dir_name=${MANDATORY_DIR[${dir}]}
	if [ -z "$(ls -A ${dir_name})" ]; then
		echo "unexpected Empty ${dir_name}"
		exit 1
	fi
done

if [ -z ${AWS_AVAILABILITY_ZONES}  ];then
	echo "AWS_AVAILABILITY_ZONES is mandatory to make test pass"
	exit 1
fi

#need this for pre-provisioned tests
curl -o /usr/local/bin/kubectl -LO https://storage.googleapis.com/kubernetes-release/release/v1.19.4/bin/linux/amd64/kubectl && chmod +x /usr/local/bin/kubectl

export PATH=$PATH:/usr/local/go/bin
export GOPATH="/go"
make aws-ebs-csi-driver -j 4
go get -v -u github.com/onsi/ginkgo/ginkgo@v1.14.2
export KUBECONFIG=/e2e-env/.kube/config
export ARTIFACTS=./single_az_test_e2e_report
mkdir -p $ARTIFACTS
export NODES=4

FOCUS_REGEXP="\[ebs-csi-e2e\] \[single-az\]"
SKIP_REGEXP="and encryption"
$GOPATH/bin/ginkgo --progress -debug -p -nodes=$NODES \
					-v --focus="${FOCUS_REGEXP}" --skip="${SKIP_REGEXP}" \
					tests/e2e -- -report-dir=$ARTIFACTS
