
# Coverage 

Integration test verifies the functionality of EBS CSI driver as a standalone server outside Kubernetes. It exercises the lifecycle of the volume by creating, attaching, staging, mounting volumes and the reverse operations. And it verifies data can be written onto an EBS volume without any issue.

The integration test is executed using osc-k8s-tester which is CLI tool for k8s testing on AWS. 
With aws-k8s-tester, it automates the process of provisioning EC2 instance, pulling down and building EBS CSI driver, running the defined integration test and sending test result back. See aws-k8s-tester for more details about how to use it.

## Integration tests (at Outscale)

CSI gRPC call support for viarity of services.

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


