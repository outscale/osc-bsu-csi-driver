package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"
)

type LoggingCollector interface {
	SetLogger(logger klog.Logger)
	prometheus.Collector
}
