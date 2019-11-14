
# Coverage 

Integration test verifies the functionality of EBS CSI driver as a standalone server outside Kubernetes. It exercises the lifecycle of the volume by creating, attaching, staging, mounting volumes and the reverse operations. And it verifies data can be written onto an EBS volume without any issue.

The integration test is executed using osc-k8s-tester which is CLI tool for k8s testing on AWS. 
With aws-k8s-tester, it automates the process of provisioning EC2 instance, pulling down and building EBS CSI driver, running the defined integration test and sending test result back. See aws-k8s-tester for more details about how to use it.

##  Calls covered by the int test
This plugin implement the following  CSI gRPC calls:
* **Controller Service**: CreateVolume, DeleteVolume, ControllerPublishVolume, ControllerUnpublishVolume, ControllerGetCapabilities, ValidateVolumeCapabilities, CreateSnapshot, DeleteSnapshot, ListSnapshots
* **Node Service**: NodeStageVolume, NodeUnstageVolume, NodePublishVolume, NodeUnpublishVolume, NodeGetCapabilities, NodeGetInfo
* **Identity Service**: GetPluginInfo, GetPluginCapabilities, Probe


### .... Identity service ..... 
#### Covered byt int test
    
#### Uncovered byt int test
    GetPluginInfo:  NO
    GetPluginCapabilities:  NO
    Probe : NO

### .... Controller service ....
#### Covered byt int test
    CreateVolume: YES
    DeleteVolume: YES
    ControllerPublishVolume: Yes
    ControllerUnpublishVolume: Yes
    
#### Uncovered byt int test
    ValidateVolumeCapabilities: NO
    ControllerGetCapabilities: NO
    CreateSnapshot: NO 
    DeleteSnapshot: NO
    ListSnapshots: : NO
    ControllerExpandVolume: NO

#### **NOT IMPLEMENTED**
    ListVolumes: NOT IMPLEMENTED
    GetCapacity: NOT IMPLEMENTED


### .... Node service ....

#### Covered byt int test
    NodeStageVolume: Yes
    NodeUnstageVolume: Yes
    NodePublishVolume: Yes
    NodeUnpublishVolume: Yes

#### Uncovered byt int test
    NodeExpandVolume: No
    NodeGetCapabilities:No
    NodeGetCapabilities: No
    NodeGetInfo: No
    
#### **NOT IMPLEMENTED**
    NodeGetVolumeStats: NOT IMPLEMENTED
    

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


