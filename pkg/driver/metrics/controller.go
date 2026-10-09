package metrics

import (
	"context"
	"sync"

	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud"
	"github.com/outscale/osc-bsu-csi-driver/pkg/driver/k8s"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	listercorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
)

const (
	volumeSubsystem  = "volume"
	metricVolumeInfo = namespace + "_" + volumeSubsystem + "_iops"
)

type ControllerCollector struct {
	ctx context.Context

	driver string

	cloud cloud.Cloud
	pvs   listercorev1.PersistentVolumeLister

	metrics map[string]*prometheus.Desc

	logger klog.Logger
}

func NewControllerCollector(ctx context.Context, driver string, cloud cloud.Cloud, sifVolumes informers.SharedInformerFactory) *ControllerCollector {
	return &ControllerCollector{
		ctx:    ctx,
		driver: driver,
		cloud:  cloud,
		pvs:    sifVolumes.Core().V1().PersistentVolumes().Lister(),
	}
}

func (c *ControllerCollector) SetLogger(logger klog.Logger) {
	c.logger = logger
}

func (c *ControllerCollector) ConfigureMetrics(constLabels prometheus.Labels) {
	variableLabels := []string{"pv", "pvc", "namespace", "storage_class", "volume_attribute_class", "volume_id", "volume_type"}
	ms := map[string]*prometheus.Desc{
		metricVolumeInfo: prometheus.NewDesc(metricVolumeInfo, "Volume information.", variableLabels, constLabels),
	}
	c.metrics = ms
}

func (c *ControllerCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.metrics {
		ch <- desc
	}
}

func (c *ControllerCollector) Collect(ch chan<- prometheus.Metric) {
	logger := c.logger.V(5)
	logger.Info("Collecting metrics")

	// read PVs
	pvs, err := k8s.ListPersistentVolumes(c.driver, c.pvs, c.logger)
	if err != nil {
		logger.Error(err, "Cannot list PersistentVolumes")
		return
	}

	// read API volumes
	ids := lo.FilterMap(pvs, func(pv *corev1.PersistentVolume, _ int) (string, bool) {
		if pv.Spec.CSI == nil || pv.Spec.CSI.VolumeHandle == "" || pv.Spec.CSI.Driver != c.driver {
			return "", false
		}
		return pv.Spec.CSI.VolumeHandle, true
	})
	volChan := make(chan *cloud.Volume, len(ids))
	wg := sync.WaitGroup{}
	for _, id := range ids {
		wg.Go(func() {
			// GetVolumeByID batches calls, this should generate only 1 ReadVolumes OAPI call.
			vol, err := c.cloud.GetVolumeByID(c.ctx, id)
			if err != nil {
				logger.Error(err, "Unable to collect volume type/iops", "volume", id)
			}
			volChan <- vol
		})
	}
	wg.Wait()
	close(volChan)

	// transform channel contents, having nil values to a map[volumeID]Volume with only valid values
	vols := lo.SliceToMap(
		lo.Filter(lo.ChannelToSlice(volChan), func(v *cloud.Volume, _ int) bool {
			return v != nil
		}),
		func(v *cloud.Volume) (string, *cloud.Volume) {
			return v.VolumeID, v
		},
	)

	for _, pv := range pvs {
		pvc, namespace := "-", "-"
		if pv.Spec.ClaimRef != nil {
			pvc = pv.Spec.ClaimRef.Name
			namespace = pv.Spec.ClaimRef.Namespace
		}
		volumeID := "-"
		if pv.Spec.CSI != nil && pv.Spec.CSI.VolumeHandle != "" {
			volumeID = pv.Spec.CSI.VolumeHandle
		}
		storageClass := pv.Spec.StorageClassName
		volumeAttributeClass := ptr.From(pv.Spec.VolumeAttributesClassName, "-")
		volumeType, iops := "-", 0
		if v, ok := vols[volumeID]; ok {
			volumeType = string(v.VolumeType)
			iops = v.IOPS
		}
		ch <- prometheus.MustNewConstMetric(c.metrics[metricVolumeInfo], prometheus.GaugeValue, float64(iops), pv.Name, pvc, namespace, storageClass, volumeAttributeClass, volumeID, volumeType)
	}
}

var _ Collector = (*ControllerCollector)(nil)
