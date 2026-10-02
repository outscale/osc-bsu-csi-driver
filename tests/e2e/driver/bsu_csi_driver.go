/*
Copyright 2018 The Kubernetes Authors.

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

package driver

import (
	"fmt"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	bsucsidriver "github.com/outscale/osc-bsu-csi-driver/pkg/driver"
	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	True = "true"
)

// Implement DynamicPVTestDriver interface
type bsuCSIDriver struct {
	driverName string
}

// InitBsuCSIDriver returns bsuCSIDriver that implements DynamicPVTestDriver interface
func InitBsuCSIDriver() PVTestDriver {
	return &bsuCSIDriver{
		driverName: bsucsidriver.DriverName,
	}
}

func (d *bsuCSIDriver) GetDynamicProvisionStorageClass(parameters map[string]string, mountOptions []string, reclaimPolicy *corev1.PersistentVolumeReclaimPolicy, volumeExpansion *bool, bindingMode *storagev1.VolumeBindingMode, allowedTopologyValues []string, namespace string) *storagev1.StorageClass {
	provisioner := d.driverName
	generateName := fmt.Sprintf("%s-%s-dynamic-sc-", namespace, provisioner)
	allowedTopologies := []corev1.TopologySelectorTerm{}
	if len(allowedTopologyValues) > 0 {
		allowedTopologies = []corev1.TopologySelectorTerm{
			{
				MatchLabelExpressions: []corev1.TopologySelectorLabelRequirement{
					{
						Key:    bsucsidriver.TopologyKey,
						Values: allowedTopologyValues,
					},
				},
			},
		}
	}
	return getStorageClass(generateName, provisioner, parameters, mountOptions, reclaimPolicy, volumeExpansion, bindingMode, allowedTopologies)
}

func (d *bsuCSIDriver) GetVolumeSnapshotClass(namespace string) *volumesnapshotv1.VolumeSnapshotClass {
	provisioner := d.driverName
	generateName := fmt.Sprintf("%s-%s-dynamic-sc-", namespace, provisioner)
	return getVolumeSnapshotClass(generateName, provisioner)
}

func (d *bsuCSIDriver) GetPersistentVolume(volumeID string, fsType string, size string, reclaimPolicy *corev1.PersistentVolumeReclaimPolicy, namespace string) *corev1.PersistentVolume {
	provisioner := d.driverName
	generateName := fmt.Sprintf("%s-%s-preprovisioned-pv-", namespace, provisioner)
	// Default to Retain ReclaimPolicy for pre-provisioned volumes
	pvReclaimPolicy := corev1.PersistentVolumeReclaimRetain
	if reclaimPolicy != nil {
		pvReclaimPolicy = *reclaimPolicy
	}
	return &corev1.PersistentVolume{
		GenerateName: generateName,
		Namespace:    namespace,
		// TODO remove if https://github.com/kubernetes-csi/external-provisioner/issues/202 is fixed
		Annotations: map[string]string{
			"pv.kubernetes.io/provisioned-by": provisioner,
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(size),
			},
			PersistentVolumeReclaimPolicy: pvReclaimPolicy,
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       provisioner,
					VolumeHandle: volumeID,
					FSType:       fsType,
				},
			},
		},
	}
}

// GetParameters returns the parameters specific for this driver
func GetParameters(volumeType osc.VolumeType, fsType, iops string, absoluteIops, encrypted bool, secretName, secretNamespace string) map[string]string {
	parameters := map[string]string{
		bsucsidriver.VolumeTypeKey:  string(volumeType),
		"csi.storage.k8s.io/fstype": fsType,
	}

	if iops == "" {
		iops = IOPSPerGBForVolumeType(volumeType)
	}
	switch {
	case iops != "" && absoluteIops:
		parameters[bsucsidriver.IopsKey] = iops
	case iops != "" && !absoluteIops:
		parameters[bsucsidriver.IopsPerGBKey] = iops
	}

	if encrypted {
		parameters[bsucsidriver.EncryptedKey] = True
	}
	if len(secretName) != 0 {
		parameters["csi.storage.k8s.io/node-stage-secret-name"] = secretName
	}
	if len(secretNamespace) != 0 {
		parameters["csi.storage.k8s.io/node-stage-secret-namespace"] = secretNamespace
	}
	return parameters
}

// MinimumSizeForVolumeType returns the minimum disk size for each volumeType
func MinimumSizeForVolumeType(volumeType osc.VolumeType) string {
	switch volumeType {
	case osc.VolumeTypeGp2:
		return "1Gi"
	case osc.VolumeTypeIo1:
		return "4Gi"
	case osc.VolumeTypeStandard:
		return "10Gi"
	default:
		return "1Gi"
	}
}

// IOPSPerGBForVolumeType returns 25 for io1 volumeType
// Otherwise returns an empty string
func IOPSPerGBForVolumeType(volumeType osc.VolumeType) string {
	if volumeType == osc.VolumeTypeIo1 {
		// Minimum disk size is 4, minimum IOPS is 100
		return "25"
	}
	return ""
}

func (d *bsuCSIDriver) GetPassphraseSecret(name string, passphrase string) *corev1.Secret {
	return &corev1.Secret{
		Name: name,
		StringData: map[string]string{
			bsucsidriver.LuksPassphraseKey: passphrase,
		},
	}
}

func (d *bsuCSIDriver) GetVolumeAttributesClass(namespace, name string, volumeType osc.VolumeType, iops bool, iopsPerGB string) *storagev1.VolumeAttributesClass {
	if iops {
		return &storagev1.VolumeAttributesClass{
			Name:       name,
			Namespace:  namespace,
			DriverName: d.driverName,
			Parameters: map[string]string{
				bsucsidriver.VolumeTypeKey: string(volumeType),
				bsucsidriver.IopsKey:       iopsPerGB,
			},
		}
	}
	return &storagev1.VolumeAttributesClass{
		Name:       name,
		Namespace:  namespace,
		DriverName: d.driverName,
		Parameters: map[string]string{
			bsucsidriver.VolumeTypeKey: string(volumeType),
			bsucsidriver.IopsPerGBKey:  iopsPerGB,
		},
	}
}
