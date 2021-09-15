#!/bin/bash
set -eu pipefail

MANDATORY_DIR="/e2e-env/.kube/ /root/osc-bsu-csi-driver"
export KUBECONFIG=/e2e-env/.kube/config

if [ ! -z "${KC}"  ];then
      mkdir -p $HOME/.kube
      echo "${KC}" | base64 --decode > $HOME/.kube/config
      export KUBECONFIG=$HOME/.kube/config
      MANDATORY_DIR="/root/osc-bsu-csi-driver"
fi

MANDATORY_DIR=(${MANDATORY_DIR})
for (( dir=0; dir<${#MANDATORY_DIR[@]}; dir++ )); do
	dir_name=${MANDATORY_DIR[${dir}]}
	if [ -z "$(ls -A ${dir_name})" ]; then
		echo "unexpected Empty ${dir_name}"
		exit 1
	fi
done

if [ -z ${OSC_AVAILABILITY_ZONES}  ];then
	echo "OSC_AVAILABILITY_ZONES is mandatory to make test pass"
	exit 1
fi

count_rs=`kubectl get rs -n kube-system -l "app.kubernetes.io/name=osc-bsu-csi-driver" | wc -l`
if [ "$count_rs" -eq "0" ]; then
   echo "osc-bsu-csi-driver rs not found";
   exit 1
fi

count_ds=`kubectl get ds -n kube-system -l "app.kubernetes.io/name=osc-bsu-csi-driver" | wc -l`
if [ "$count_ds" -eq "0" ]; then
   echo "osc-bsu-csi-driver ds not found";
   exit 1
fi

count_pods=`kubectl get pod -n kube-system -l "app.kubernetes.io/name=osc-bsu-csi-driver" | wc -l`
if [ "$count_pods" -eq "0" ]; then
   echo "osc-bsu-csi-driver pods not found";
   exit 1
fi

kubectl describe rs -n kube-system -l "app.kubernetes.io/name=osc-bsu-csi-driver"
kubectl describe ds -n kube-system -l "app.kubernetes.io/name=osc-bsu-csi-driver"
kubectl describe pod -n kube-system -l "app.kubernetes.io/name=osc-bsu-csi-driver"

export PATH=$PATH:/usr/local/go/bin
export GOPATH="/go"
export ARTIFACTS=./single_az_test_e2e_report
mkdir -p $ARTIFACTS
export NODES=4

FOCUS_REGEXP="\[bsu-csi-e2e\] \[single-az\]"
SKIP_REGEXP="and encryption"
$GOPATH/bin/ginkgo build -r tests/e2e
$GOPATH/bin/ginkgo --progress -debug -p -nodes=$NODES \
					--slowSpecThreshold=120 \
					-v --focus="${FOCUS_REGEXP}" --skip="${SKIP_REGEXP}" \
					tests/e2e -- -report-dir=$ARTIFACTS
