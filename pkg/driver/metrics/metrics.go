package metrics

import (
	"net/http"
	"time"

	"github.com/outscale/osc-bsu-csi-driver/cmd/options"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"golang.org/x/time/rate"
	"k8s.io/component-base/metrics"
	"k8s.io/klog/v2"
)

const (
	namespace = "osc_csi_"
)

type Manager struct {
	registry metrics.KubeRegistry
	opts     options.MetricsOptions
	logger   klog.Logger
}

func NewManager(opts options.MetricsOptions) Manager {
	m := Manager{
		registry: metrics.NewKubeRegistry(),
		opts:     opts,
		logger:   klog.LoggerWithName(klog.Background(), "metrics"),
	}
	return m
}

func (m *Manager) GetRegistry() metrics.KubeRegistry {
	return m.registry
}

func (m *Manager) RegisterRuntimeMetrics() {
	m.registry.RawMustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

func (m *Manager) Register(collectors ...LoggingCollector) {
	for _, c := range collectors {
		c.SetLogger(m.logger)
		m.registry.RawMustRegister(c)
	}
}

// Server represents any type that could serve HTTP requests for the metrics
// endpoint.
type Server interface {
	Handle(pattern string, handler http.Handler)
}

// RegisterToServer registers an HTTP handler for this metrics manager to the
// given server at the specified address/path.
func (m *Manager) registerToServer(s Server) {
	limiter := rate.NewLimiter(
		rate.Every(time.Duration(float64(time.Second)/m.opts.QPS)),
		5,
	)
	s.Handle("/metrics", rateLimit(limiter, metrics.HandlerFor(
		m.GetRegistry(),
		metrics.HandlerOpts{
			ErrorHandling: metrics.ContinueOnError,
		},
	)))
}

// StartHttp sets up a server and creates a handler for metrics .
func (m *Manager) StartHttp() {
	if m.opts.HTTPEndpoint == "" {
		klog.Infof("No HTTP endpoint defined for metrics")
		return
	}
	mux := http.NewServeMux()
	m.registerToServer(mux)
	go func() {
		klog.Infof("Starting metrics server on %q", m.opts.HTTPEndpoint)
		srv := &http.Server{
			Addr:              m.opts.HTTPEndpoint,
			ReadHeaderTimeout: time.Second,
			Handler:           mux,
		}
		if err := srv.ListenAndServe(); err != nil {
			klog.Fatal("Failed to start metric server", "address", m.opts.HTTPEndpoint, "err", err.Error())
		}
	}()
}
