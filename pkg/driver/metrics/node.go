package metrics

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"

	"github.com/outscale/goutils/sdk/ptr"
	"github.com/outscale/osc-bsu-csi-driver/pkg/driver/consts"
	"github.com/outscale/osc-bsu-csi-driver/pkg/driver/k8s"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/informers"
	listerstoragev1 "k8s.io/client-go/listers/storage/v1"
	"k8s.io/klog/v2"
)

const (
	nodeSubsystem     = "blockdevice"
	metricReadOps     = namespace + "_" + nodeSubsystem + "_read_ops_total"
	metricWriteOps    = namespace + "_" + nodeSubsystem + "_write_ops_total"
	metricInFlight    = namespace + "_" + nodeSubsystem + "_in_flight"
	metricTimeInQueue = namespace + "_" + nodeSubsystem + "_time_in_queue_total"
)

type NodeCollector struct {
	node, driver string

	volumeAttachments listerstoragev1.VolumeAttachmentLister

	metrics map[string]*prometheus.Desc

	logger klog.Logger
}

func NewNodeCollector(node, driver string, sifVolumes informers.SharedInformerFactory) *NodeCollector {
	return &NodeCollector{
		node:              node,
		driver:            driver,
		volumeAttachments: sifVolumes.Storage().V1().VolumeAttachments().Lister(),
	}
}

func (c *NodeCollector) SetLogger(logger klog.Logger) {
	c.logger = logger
}

func (c *NodeCollector) ConfigureMetrics(constLabels prometheus.Labels) {
	variableLabels := []string{"pvc", "device"}
	nodeMetrics := map[string]*prometheus.Desc{
		metricReadOps:     prometheus.NewDesc(metricReadOps, "The total number of completed read operations.", variableLabels, constLabels),
		metricWriteOps:    prometheus.NewDesc(metricWriteOps, "The total number of completed write operations.", variableLabels, constLabels),
		metricInFlight:    prometheus.NewDesc(metricInFlight, "The number of I/Os currently in flight.", variableLabels, constLabels),
		metricTimeInQueue: prometheus.NewDesc(metricTimeInQueue, "The total wait time, in milliseconds, for all requests.", variableLabels, constLabels),
	}
	c.metrics = nodeMetrics
}

func (c *NodeCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.metrics {
		ch <- desc
	}
}

func (c *NodeCollector) Collect(ch chan<- prometheus.Metric) {
	logger := c.logger.V(5)
	logger.Info("Collecting node stats")
	vas, err := k8s.ListAttachedVolumes(c.node, c.driver, c.volumeAttachments, c.logger)
	if err != nil {
		logger.Error(err, "Cannot list VolumeAttachments")
		return
	}
	for _, va := range vas {
		pv := ptr.From(va.Spec.Source.PersistentVolumeName)
		devicePath := va.Status.AttachmentMetadata[consts.DevicePathKey]
		device := devicePath
		if info, err := os.Lstat(device); err == nil && (info.Mode()&os.ModeSymlink != 0) {
			device, err = os.Readlink(device)
			if err != nil {
				logger.Error(err, "Cannot resolve device path link")
				continue
			}
		}
		device = filepath.Base(device)
		stats, err := ReadStats(device)
		switch {
		case errors.Is(err, os.ErrNotExist):
			logger.Info("No block stats for device", "pv", pv)
			continue
		case err != nil:
			logger.Error(err, "Cannot read block stats", "pv", pv)
			continue
		}
		logger.Info(fmt.Sprintf("PV stats: %+v", stats), "pv", pv, "device", device)

		ch <- prometheus.MustNewConstMetric(c.metrics[metricReadOps], prometheus.CounterValue, float64(stats.ReadIOs), pv, devicePath)
		ch <- prometheus.MustNewConstMetric(c.metrics[metricWriteOps], prometheus.CounterValue, float64(stats.WriteIOs), pv, devicePath)
		ch <- prometheus.MustNewConstMetric(c.metrics[metricInFlight], prometheus.GaugeValue, float64(stats.InFlight), pv, devicePath)
		ch <- prometheus.MustNewConstMetric(c.metrics[metricTimeInQueue], prometheus.CounterValue, float64(stats.TimeInQueue), pv, devicePath)
	}
}

type blockStats struct {
	ReadIOs        uint64 // read I/Os       requests      number of read I/Os processed
	ReadMerges     uint64 // read merges     requests      number of read I/Os merged with in-queue I/O
	ReadSectors    uint64 // read sectors    sectors       number of sectors read
	ReadTicks      uint64 // read ticks      milliseconds  total wait time for read requests
	WriteIOs       uint64 // write I/Os      requests      number of write I/Os processed
	WriteMerges    uint64 // write merges    requests      number of write I/Os merged with in-queue I/O
	WriteSectors   uint64 // write sectors   sectors       number of sectors written
	WriteTicks     uint64 // write ticks     milliseconds  total wait time for write requests
	InFlight       uint64 // in_flight       requests      number of I/Os currently in flight
	IOTicks        uint64 // io_ticks        milliseconds  total time this block device has been active
	TimeInQueue    uint64 // time_in_queue   milliseconds  total wait time for all requests
	DiscardIOs     uint64 // discard I/Os    requests      number of discard I/Os processed
	DiscardMerges  uint64 // discard merges  requests      number of discard I/Os merged with in-queue I/O
	DiscardSectors uint64 // discard sectors sectors       number of sectors discarded
	DiscardTicks   uint64 // discard ticks   milliseconds  total wait time for discard requests
}

func ReadStats(device string) (*blockStats, error) {
	path := filepath.Join("/sys/block", device, "stat")
	fd, err := os.Open(path) //nolint: gosec
	if err != nil {
		return nil, fmt.Errorf("read stats: %w", err)
	}
	scanner := bufio.NewScanner(fd)
	scanner.Split(bufio.ScanWords)
	bs := &blockStats{}
	bsv := reflect.Indirect(reflect.ValueOf(bs))
	for i := range bsv.NumField() {
		if !scanner.Scan() {
			break
		}
		f := bsv.Field(i)
		v, err := strconv.ParseUint(scanner.Text(), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error reading %s: %w", bsv.Type().Field(i).Name, err)
		}
		f.SetUint(v)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return bs, nil
}

var _ Collector = (*NodeCollector)(nil)
