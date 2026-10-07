package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"
)

type Collector interface {
	ConfigureMetrics(labels prometheus.Labels)
	SetLogger(logger klog.Logger)
	prometheus.Collector
}
