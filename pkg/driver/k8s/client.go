package k8s

import (
	"context"
	"time"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

var (
	backoff = wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   2.0,
		Steps:    5,
	}

	GetClient = func() (kubernetes.Interface, error) {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		return kubernetes.NewForConfig(cfg)
	}
)

func GetNode(ctx context.Context, name string) (*corev1.Node, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	var node *corev1.Node
	// ignoring error returned by backoff (usually "timeout waiting")
	_ = wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		node, err = client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			return false, err
		case err != nil:
			return false, nil //nolint: nilerr
		default:
			return true, nil
		}
	})
	return node, err
}

func CountVolumeAttachments(ctx context.Context, node, driver string) (int, error) {
	client, err := GetClient()
	if err != nil {
		return -1, err
	}

	var lst *storagev1.VolumeAttachmentList
	_ = wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		lst, err = client.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
		switch {
		case err != nil:
			return false, nil //nolint: nilerr
		default:
			return true, nil
		}
	})
	if err != nil {
		return -1, err
	}
	logger := klog.FromContext(ctx).V(5)
	cnt := lo.CountBy(lst.Items, func(va storagev1.VolumeAttachment) bool {
		if va.Spec.NodeName != node || va.Spec.Attacher != driver || !va.Status.Attached {
			return false
		}
		logger.Info("Attached volume", "pvc", ptr.Deref(va.Spec.Source.PersistentVolumeName, "-"))
		return true
	})
	return cnt, err
}
