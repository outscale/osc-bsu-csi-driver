#!/bin/sh

IMAGE_NAME="TEST-aws-ebs-csi-driver-osc"
IMAGE="osc/osc-ebs-csi-driver:latest"

if [ -z "${SRC_HOME}"  ];then
    SRC_HOME="/home/outscale"
fi

copy_data()
{
rsync -avze ssh  --exclude='/home/anisz/poc-cloud-provider/aws-ebs-csi-driver/.git*'    --exclude='/home/anisz/poc-cloud-provider/aws-k8s-tester/.git*'   /home/anisz/poc-cloud-provider/ outscale@109.232.235.201:/home/outscale/poc-cloud-provider/

#rsync -avze ssh  --exclude='/home/anisz/poc-cloud-provider/aws-k8s-tester/.git*' /home/anisz/poc-cloud-provider/aws-k8s-tester/ outscale@109.232.235.201:/home/outscale/poc-cloud-provider/aws-k8s-tester/


#rsync -avze ssh  --exclude='/home/anisz/poc-cloud-provider/aws-k8s-tester/.git*' /home/anisz/poc-cloud-provider/aws-k8s-tester/ outscale@109.232.235.201:/home/outscale/poc-cloud-provider/aws-k8s-tester/

}



install_pkg()
{
    sudo yum -y update
    sudo yum -y install git
    sudo yum -y install -y yum-utils \
                device-mapper-persistent-data \
                lvm2
    sudo yum-config-manager \
                 --add-repo \
                 https://download.docker.com/linux/centos/docker-ce.repo
    sudo yum-config-manager --disable docker-ce-nightly
    sudo yum -y erase docker-common
    sudo yum -y install docker-ce docker-ce-cli containerd.io

}


run_docker()
{
    sudo docker run -it  -v $SRC_HOME/poc-cloud-provider/aws-ebs-csi-driver:/go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver  \
                -v $SRC_HOME/poc-cloud-provider/aws-k8s-tester:/go/src/github.com/aws/aws-k8s-tester  \
                -v $SRC_HOME/poc-cloud-provider/pkg:/go/pkg/  \
                -v $SRC_HOME/poc-cloud-provider/aws-ebs-csi-driver/.aws/:/root/.aws  \
                --cap-add=SYS_PTRACE --security-opt seccomp=unconfined \
                --name=$IMAGE_NAME --entrypoint /bin/bash --rm  $IMAGE
}


run_container()
{
     sudo docker run -it  -v $SRC_HOME/poc-cloud-provider/aws-ebs-csi-driver:/go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver  \
                     --cap-add=SYS_PTRACE --security-opt seccomp=unconfined \
                     --name=$IMAGE_NAME --entrypoint /bin/bash --rm golang:1.12.7-stretch
}

run_test()
{
 cd /go/src/github.com/aws/aws-k8s-tester &&  make -j 4 &&     cp /go/src/github.com/aws/aws-k8s-tester/bin/aws-k8s-tester-*-linux-amd64 /bin/aws-k8s-tester-osc && cd /go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver &&  AWS_K8S_TESTER_VPC_ID="vpc-5eb020f8" make test-integration
}

run_gdb()
{
 gdb --args /go/src/github.com/aws/aws-k8s-tester/bin/aws-k8s-tester-b447c0112f44-linux-amd64 csi test integration --terminate-on-exit=true --timeout=20m "" "vpc-5eb020f8"
}

if [ "$1" == "install_pkg"  ]; then
    install_pkg
elif [ "$1" == "copy_data"  ]; then
    copy_data
elif [ "$1" == "run_docker"  ]; then
    run_docker
elif [ "$1" == "run_container"  ]; then
    run_container
elif [ "$1" == "run_test"  ]; then
    run_test
fi
