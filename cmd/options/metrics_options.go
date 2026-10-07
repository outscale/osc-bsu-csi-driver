package options

import (
	"github.com/spf13/pflag"
)

// MetricsOptions contains options and configuration settings for the metrics.
type MetricsOptions struct {
	HTTPEndpoint string

	QPS float64
}

func (s *MetricsOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.HTTPEndpoint, "http-endpoint", "", "HTTP endpoint for metrics")
	fs.Float64Var(&s.QPS, "metrics-qps", 1, "Max QPS allowed on metrics endpoint")
}
