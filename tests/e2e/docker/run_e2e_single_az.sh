#!/bin/bash
set -eu pipefail

MANDATORY_DIR="/go/src/cloud-provider-osc"
export KUBECONFIG=/e2e-env/.kube/config

if [ ! -z "${KC}"  ];then
      mkdir -p $HOME/.kube
      echo "${KC}" | base64 --decode > $HOME/.kube/config
      export KUBECONFIG=$HOME/.kube/config
      MANDATORY_DIR="/go/src/cloud-provider-osc"
fi

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
count_ds=`kubectl get ds -n kube-system -l "app=osc-cloud-controller-manager" | wc -l`
if [ "$count_ds" -eq "0" ]; then
   echo "osc-cloud-controller-manager ds not found";
   exit 1
fi
count_pods=`kubectl get pod -n kube-system -l "app=osc-cloud-controller-manager" | wc -l`
if [ "$count_pods" -eq "0" ]; then
   echo "osc-cloud-controller-manager pods not found";
   exit 1
fi

kubectl describe ds -n kube-system -l "app=osc-cloud-controller-manager"
kubectl describe pod -n kube-system -l "app=osc-cloud-controller-manager"

export PATH=$PATH:/usr/local/go/bin
export GOPATH="/go"
export ARTIFACTS=./single_az_test_e2e_report
mkdir -p $ARTIFACTS

$GOPATH/bin/ginkgo build -r tests/e2e
$GOPATH/bin/ginkgo --show-node-events -v tests/e2e -- -report-dir=$ARTIFACTS
