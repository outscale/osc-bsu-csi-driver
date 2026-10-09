package k8s

import (
	"fmt"
	"time"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listercorev1 "k8s.io/client-go/listers/core/v1"
	listerstoragev1 "k8s.io/client-go/listers/storage/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

// GetNodeInformerFactory returns an informer factory that can be used to build an informer monitoring the current node.
func GetNodeInformerFactory(node string, kubeClient kubernetes.Interface) informers.SharedInformerFactory {
	return informers.NewSharedInformerFactoryWithOptions(
		kubeClient, 30*time.Minute,
		informers.WithTweakListOptions(func(lo *metav1.ListOptions) {
			lo.FieldSelector = "metadata.name=" + node
		}),
	)
}

// GetVolumeInformerFactory returns an informer factory that can be used to build an informer monitoring VolumeAttachment/PersistentVolume resources.
func GetVolumeInformerFactory(node string, kubeClient kubernetes.Interface) informers.SharedInformerFactory {
	return informers.NewSharedInformerFactoryWithOptions(
		kubeClient, 30*time.Minute,
		informers.WithTransform(func(v any) (any, error) {
			// Strip some unused fields before caching to limit memory usage
			switch v := v.(type) {
			case *storagev1.VolumeAttachment:
				v.TypeMeta = metav1.TypeMeta{}
				v.ResourceVersion = ""
				v.Finalizers = nil
				v.Annotations = nil
				if v.Spec.NodeName != node {
					v.Spec.Source = storagev1.VolumeAttachmentSource{}
					v.Status = storagev1.VolumeAttachmentStatus{}
				}
			case *corev1.PersistentVolume:
				v.TypeMeta = metav1.TypeMeta{}
				v.ResourceVersion = ""
				v.Finalizers = nil
				v.Annotations = nil
				v.Spec.NodeAffinity = nil
			}
			return v, nil
		}),
	)
}

func ListPersistentVolumes(driver string, pvs listercorev1.PersistentVolumeLister, logger klog.Logger) ([]*corev1.PersistentVolume, error) {
	lst, err := pvs.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("list volume attachments: %w", err)
	}
	l := lo.Filter(lst, func(pv *corev1.PersistentVolume, _ int) bool {
		if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != driver {
			return false
		}
		return true
	})
	return l, err
}

func ListAttachedVolumes(node, driver string, volumeAttachments listerstoragev1.VolumeAttachmentLister, logger klog.Logger) ([]*storagev1.VolumeAttachment, error) {
	logger = logger.V(5)
	lst, err := volumeAttachments.List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("list volume attachments: %w", err)
	}
	l := lo.Filter(lst, func(va *storagev1.VolumeAttachment, _ int) bool {
		if va.Spec.NodeName != node || va.Spec.Attacher != driver || !va.Status.Attached {
			return false
		}
		logger.Info("Attached PVC", "pvc", ptr.Deref(va.Spec.Source.PersistentVolumeName, "-"))
		return true
	})
	return l, err
}
