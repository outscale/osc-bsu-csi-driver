/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package osc

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"

	"k8s.io/apimachinery/pkg/util/wait"
	volerr "k8s.io/cloud-provider/volume/errors"
	"k8s.io/klog"
)

// ********************* CCM awsDisk Def & functions *********************

type awsDisk struct {
	ec2 EC2

	// Name in k8s
	name KubernetesVolumeID
	// id in AWS
	awsID EBSVolumeID
}

// Gets the full information about this volume from the EC2 API
func (d *awsDisk) describeVolume() (*ec2.Volume, error) {
	volumeID := d.awsID

	request := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{volumeID.awsString()},
	}

	volumes, err := d.ec2.DescribeVolumes(request)
	if err != nil {
		return nil, err
	}
	if len(volumes) == 0 {
		return nil, fmt.Errorf("no volumes found")
	}
	if len(volumes) > 1 {
		return nil, fmt.Errorf("multiple volumes found")
	}
	return volumes[0], nil
}

func (d *awsDisk) describeVolumeModification() (*ec2.VolumeModification, error) {
	volumeID := d.awsID
	request := &ec2.DescribeVolumesModificationsInput{
		VolumeIds: []*string{volumeID.awsString()},
	}
	volumeMods, err := d.ec2.DescribeVolumeModifications(request)

	if err != nil {
		return nil, fmt.Errorf("error describing volume modification %s with %v", volumeID, err)
	}

	if len(volumeMods) == 0 {
		return nil, fmt.Errorf("no volume modifications found for %s", volumeID)
	}
	lastIndex := len(volumeMods) - 1
	return volumeMods[lastIndex], nil
}

func (d *awsDisk) modifyVolume(requestGiB int64) (int64, error) {
	volumeID := d.awsID

	request := &ec2.ModifyVolumeInput{
		VolumeId: volumeID.awsString(),
		Size:     aws.Int64(requestGiB),
	}
	output, err := d.ec2.ModifyVolume(request)
	if err != nil {
		modifyError := fmt.Errorf("AWS modifyVolume failed for %s with %v", volumeID, err)
		return requestGiB, modifyError
	}

	volumeModification := output.VolumeModification

	if aws.StringValue(volumeModification.ModificationState) == ec2.VolumeModificationStateCompleted {
		return aws.Int64Value(volumeModification.TargetSize), nil
	}

	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   2,
		Steps:    10,
	}

	checkForResize := func() (bool, error) {
		volumeModification, err := d.describeVolumeModification()

		if err != nil {
			return false, err
		}

		// According to https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/monitoring_mods.html
		// Size changes usually take a few seconds to complete and take effect after a volume is in the Optimizing state.
		if aws.StringValue(volumeModification.ModificationState) == ec2.VolumeModificationStateOptimizing {
			return true, nil
		}
		return false, nil
	}
	waitWithErr := wait.ExponentialBackoff(backoff, checkForResize)
	return requestGiB, waitWithErr
}

// waitForAttachmentStatus polls until the attachment status is the expected value
// On success, it returns the last attachment state.
func (d *awsDisk) waitForAttachmentStatus(status string) (*ec2.VolumeAttachment, error) {
	backoff := wait.Backoff{
		Duration: volumeAttachmentStatusInitialDelay,
		Factor:   volumeAttachmentStatusFactor,
		Steps:    volumeAttachmentStatusSteps,
	}

	// Because of rate limiting, we often see errors from describeVolume
	// So we tolerate a limited number of failures.
	// But once we see more than 10 errors in a row, we return the error
	describeErrorCount := 0
	var attachment *ec2.VolumeAttachment

	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		info, err := d.describeVolume()
		if err != nil {
			// The VolumeNotFound error is special -- we don't need to wait for it to repeat
			if isAWSErrorVolumeNotFound(err) {
				if status == "detached" {
					// The disk doesn't exist, assume it's detached, log warning and stop waiting
					klog.Warningf("Waiting for volume %q to be detached but the volume does not exist", d.awsID)
					stateStr := "detached"
					attachment = &ec2.VolumeAttachment{
						State: &stateStr,
					}
					return true, nil
				}
				if status == "attached" {
					// The disk doesn't exist, complain, give up waiting and report error
					klog.Warningf("Waiting for volume %q to be attached but the volume does not exist", d.awsID)
					return false, err
				}
			}
			describeErrorCount++
			if describeErrorCount > volumeAttachmentStatusConsecutiveErrorLimit {
				// report the error
				return false, err
			}

			klog.Warningf("Ignoring error from describe volume for volume %q; will retry: %q", d.awsID, err)
			return false, nil
		}

		describeErrorCount = 0

		if len(info.Attachments) > 1 {
			// Shouldn't happen; log so we know if it is
			klog.Warningf("Found multiple attachments for volume %q: %v", d.awsID, info)
		}
		attachmentStatus := ""
		for _, a := range info.Attachments {
			if attachmentStatus != "" {
				// Shouldn't happen; log so we know if it is
				klog.Warningf("Found multiple attachments for volume %q: %v", d.awsID, info)
			}
			if a.State != nil {
				attachment = a
				attachmentStatus = *a.State
			} else {
				// Shouldn't happen; log so we know if it is
				klog.Warningf("Ignoring nil attachment state for volume %q: %v", d.awsID, a)
			}
		}
		if attachmentStatus == "" {
			attachmentStatus = "detached"
		}
		if attachmentStatus == status {
			// Attachment is in requested state, finish waiting
			return true, nil
		}
		// continue waiting
		klog.V(2).Infof("Waiting for volume %q state: actual=%s, desired=%s", d.awsID, attachmentStatus, status)
		return false, nil
	})

	return attachment, err
}

// Deletes the EBS disk
func (d *awsDisk) deleteVolume() (bool, error) {
	request := &ec2.DeleteVolumeInput{VolumeId: d.awsID.awsString()}
	_, err := d.ec2.DeleteVolume(request)
	if err != nil {
		if isAWSErrorVolumeNotFound(err) {
			return false, nil
		}
		if awsError, ok := err.(awserr.Error); ok {
			if awsError.Code() == "VolumeInUse" {
				return false, volerr.NewDeletedVolumeInUseError(err.Error())
			}
		}
		return false, fmt.Errorf("error deleting EBS volume %q: %q", d.awsID, err)
	}
	return true, nil
}
