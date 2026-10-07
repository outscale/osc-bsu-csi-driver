package k8s

import (
	"fmt"
	"time"

	"github.com/samber/lo"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
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

// GetVolumeInformerFactory returns an informer factory that can be used to build an informer monitoring VolumeAttachment resources.
func GetVolumeInformerFactory(node string, kubeClient kubernetes.Interface) informers.SharedInformerFactory {
	return informers.NewSharedInformerFactoryWithOptions(
		kubeClient, 30*time.Minute,
		informers.WithTransform(func(v any) (any, error) {
			// Strip some unused fields before caching to limit memory usage
			if va, ok := v.(*storagev1.VolumeAttachment); ok {
				va.TypeMeta = metav1.TypeMeta{}
				va.ResourceVersion = ""
				va.Finalizers = nil
				va.Annotations = nil
				if va.Spec.NodeName != node {
					va.Spec.Source = storagev1.VolumeAttachmentSource{}
					va.Status = storagev1.VolumeAttachmentStatus{}
				}
			}
			return v, nil
		}),
	)
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
