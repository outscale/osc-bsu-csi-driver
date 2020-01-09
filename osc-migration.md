
# Coverage 

Integration test verifies the functionality of EBS CSI driver as a standalone server outside Kubernetes. It exercises the lifecycle of the volume by creating, attaching, staging, mounting volumes and the reverse operations. And it verifies data can be written onto an EBS volume without any issue.

The integration test is executed using osc-k8s-tester which is CLI tool for k8s testing on AWS. 
With aws-k8s-tester, it automates the process of provisioning EC2 instance, pulling down and building EBS CSI driver, running the defined integration test and sending test result back. See aws-k8s-tester for more details about how to use it.

E2E SingleAZ tests exercises various driver functionalities in Kubernetes cluster

E2E test verifies the funcitonality of EBS CSI driver in the context of Kubernetes. It exercises driver feature e2e including static provisioning, dynamic provisioning, volume scheduling, mount options, etc.



## Integration tests (at Outscale)

CSI gRPC call support for variety of services.

Legend:
- :sparkles:: interface is implemented and test pass
- :fire:: interface is implemented but test fails
- :question:: interface is implemented but not tested
- :ghost:: interface is not implemented

### Identity Service

|     gRPC Call         |  Support  |
| :-------------------- | :-------: |
| GetPluginInfo         | :question:  |
| GetPluginCapabilities | :question:  |
| Probe                 | :question:  |

### Contoller Service 

| gRPC Call                  | Support   |
| :------------------------- | :-------: |
| CreateVolume               | :sparkles:   |
| DeleteVolume               | :sparkles:   |
| ControllerPublishVolume    | :sparkles:   |
| ControllerUnpublishVolume  | :sparkles:   |
| ValidateVolumeCapabilities | :question:  |
| ControllerGetCapabilities  | :question:  |
| CreateSnapshot             | :question:  |
| DeleteSnapshot             | :question:  |
| ListSnapshots              | :question:  |
| ControllerExpandVolume     | :question:  |
| ListVolumes                | :ghost:   |
| GetCapacity                | :ghost:   |

### Node Service

| gRPC Call           | Support   |
| :------------------ | :-------: |
| NodeStageVolume     | :sparkles:   |
| NodeUnstageVolume   | :sparkles:   |
| NodeUnstageVolume   | :sparkles:   |
| NodePublishVolume   | :sparkles:   |
| NodeUnpublishVolume | :sparkles:   |
| NodeExpandVolume    | :question:  |
| NodeGetCapabilities | :question:  |
| NodeGetCapabilities | :question:  |
| NodeGetInfo         | :question:  |
| NodeGetVolumeStats  | :ghost:   |
  
# Running Int test with osc cloud env

## Define the following env var 


```

OSC_ACCOUNT_ID=XXXXX : the osc user id
OSC_ACCOUNT_IAM=xxxx: eim user name 
OSC_USER_ID=XXXX: the eim user id
OSC_ARN="XXXXX" : the eim user orn
AWS_ACCESS_KEY_ID=XXXX : the  AK
AWS_SECRET_ACCESS_KEY=XXXX : the SK
AWS_DEFAULT_REGION=XXX: the Region to be used

```

## Run the int test

```

cd /the_path_to/osc-ebs-csi-driver/
./run_int_test.sh


```
## Check the result

wait until the test finished and you will get an output like the following in case it pass

```
........

• [SLOW TEST:24.186 seconds]
EBS CSI Driver
/home/outscale/go/src/github.com/kubernetes-sigs/osc-ebs-csi-driver/tests/integration/integration_test.go:50
  Should create, attach, stage and mount volume, check if it's writable, unmount, unstage, detach, delete, and check if it's deleted
  /home/outscale/go/src/github.com/kubernetes-sigs/osc-ebs-csi-driver/tests/integration/integration_test.go:52
------------------------------

Ran 1 of 1 Specs in 25.119 seconds
SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 0 Skipped
--- PASS: TestIntegration (25.12s)
PASS

....

```


****

## E2E single AZ tests (at Outscale)

### k8s platform 

- pre-installed k8s platform under outscale cloud with 3 masters and 2 workers on vm with `t2.medium` type
- prepare the machine from which you will run the single az test
```
    # ENV VARS 
    export OSC_ACCOUNT_ID=XXXXX
    export OSC_ACCOUNT_IAM=XXXX
    export OSC_USER_ID=XXXXXX
    export OSC_ARN="arn:aws:iam::XXXXX:user/XXX"
    
    export AWS_ACCESS_KEY_ID="XXXXXXX"
    export AWS_SECRET_ACCESS_KEY="XXXXXXX"
    export AWS_DEFAULT_REGION="eu-west-2"
    export IMAGE_NAME=registry.kube-system:5001/osc/osc-ebs-csi-driver
    export IMAGE_TAG="teste2e"
    IMAGE=osc/osc-ebs-csi-driver
    REGISTRY=registry.kube-system:5001

    # Build the plugin image
    git clone -b OSC-MIGRATION git@github.com:outscale-dev/osc-ebs-csi-driver.git

    make image IMAGE=$IMAGE IMAGE_TAG=$IMAGE_TAG REGISTRY=$REGISTRY && \
    make image-tag IMAGE=$IMAGE IMAGE_TAG=$IMAGE_TAG REGISTRY=$REGISTRY  && \
    make push IMAGE=$IMAGE IMAGE_TAG=$IMAGE_TAG REGISTRY=$REGISTRY
    
    # Deploy the plugin
    
    ## set the secrets
    export IMAGE_SECRET=registry-dockerconfigjson
    /usr/local/bin/kubectl apply -f /home/jenkins/402-registry-secret.yaml --namespace=kube-system
    curl https://raw.githubusercontent.com/kubernetes-sigs/aws-ebs-csi-driver/master/deploy/kubernetes/secret.yaml > $HOME/secret_aws_template.yaml
    cat $HOME/secret_aws_template.yaml | \
        sed "s/access_key: \"\"/access_key: \"$AWS_SECRET_ACCESS_KEY\"/g" | \
        sed "s/key_id: \"\"/key_id: \"$AWS_ACCESS_KEY_ID\"/g" > secret_aws.yaml
    echo "  aws_default_region: \""$AWS_DEFAULT_REGION"\"" >> secret_aws.yaml
    echo "  osc_account_id: \""$OSC_ACCOUNT_ID"\"" >> secret_aws.yaml
    echo "  osc_account_iam: \""$OSC_ACCOUNT_IAM"\"" >> secret_aws.yaml
    echo "  osc_user_id: \""$OSC_USER_ID"\"" >> secret_aws.yaml
    echo "  osc_arn: \""$OSC_ARN "\"" >> secret_aws.yaml
    /usr/local/bin/kubectl delete -f secret_aws.yaml --namespace=kube-system
    /usr/local/bin/kubectl apply -f secret_aws.yaml --namespace=kube-system
    
    ## deploy the pod
    cd osc-ebs-csi-driver
    helm del --purge aws-ebs-csi-driver --tls
    helm install --name aws-ebs-csi-driver \
                --set enableVolumeScheduling=true \
                --set enableVolumeResizing=true \
                --set enableVolumeSnapshot=true \
                --set image.repository=$IMAGE_NAME \
                --set image.tag=$IMAGE_TAG \
                --set imagePullSecrets=$IMAGE_SECRET \
                ./aws-ebs-csi-driver --tls
    
    ## Check the pod is running
    kubectl get pods -o wide -A  | grep csi

    # Run the e2e Test
    cd osc-ebs-csi-driver
    wget https://dl.google.com/go/go1.12.7.linux-amd64.tar.gz
    tar -C /usr/local -xzf go1.12.7.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    export GOPATH="/root/go"
    
    go get -v -u github.com/onsi/ginkgo/ginkgo
    export KUBECONFIG=$HOME/.kube/config
    export AWS_AVAILABILITY_ZONES=eu-west-2b
    ARTIFACTS=$PWD/single_az_test_e2e_report
    mkdir -p $ARTIFACTS
    export NODES=4
    $GOPATH/bin/ginkgo -debug -p -nodes=$NODES -v --focus="\[ebs-csi-e2e\] \[single-az\]" tests/e2e -- -report-dir=$ARTIFACTS
        
```
### status e2e tests [ebs-csi-e2e] [single-az]

```
7 Failures:

[Fail] [ebs-csi-e2e] [single-az] Dynamic Provisioning [It] should create a volume on demand with volumeType "standard" and encryption
/home/outscale/poc_csi/osc-ebs-csi-driver/tests/e2e/testsuites/testsuites.go:271

[Fail] [ebs-csi-e2e] [single-az] Dynamic Provisioning [It] should create a volume on demand with volumeType "gp2" and encryption
/home/outscale/poc_csi/osc-ebs-csi-driver/tests/e2e/testsuites/testsuites.go:271


[Fail] [ebs-csi-e2e] [single-az] Dynamic Provisioning [It] should create a volume on demand with volumeType "io1" and encryption
/home/outscale/poc_csi/osc-ebs-csi-driver/tests/e2e/testsuites/testsuites.go:271

[Fail] [ebs-csi-e2e] [single-az] Dynamic Provisioning [It] should create a volume on demand with volume type "io1" and fs type "xfs"
/home/outscale/poc_csi/osc-ebs-csi-driver/tests/e2e/testsuites/testsuites.go:513

[Fail] [ebs-csi-e2e] [single-az] Dynamic Provisioning [It] should create a volume on demand with volume type "gp2" and fs type "xfs"
/home/outscale/poc_csi/osc-ebs-csi-driver/tests/e2e/testsuites/testsuites.go:513

[Fail] [ebs-csi-e2e] [single-az] Dynamic Provisioning [It] should create a volume on demand with volume type "standard" and fs type "xfs"
/home/outscale/poc_csi/osc-ebs-csi-driver/tests/e2e/testsuites/testsuites.go:513

Ran 30 of 32 Specs in 1570.987 seconds
FAIL! -- 23 Passed | 7 Failed | 0 Pending | 2 Skipped

```
### 
