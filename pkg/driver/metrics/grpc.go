package metrics

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

const (
	grpcSubsystem    = "grpc"
	metricOperations = "operations"
)

type GRPCCollector struct {
	*prometheus.CounterVec
}

func NewGRPCCollector() *GRPCCollector {
	return &GRPCCollector{}
}

func (c *GRPCCollector) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		result, err := handler(ctx, req)
		c.RecordOperation(info.FullMethod, err)
		return result, err
	}
}

func (c *GRPCCollector) SetLogger(logger klog.Logger) {
}

func (c *GRPCCollector) ConfigureMetrics(constLabels prometheus.Labels) {
	c.CounterVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   namespace,
			Subsystem:   grpcSubsystem,
			Name:        metricOperations,
			Help:        "CSI driver operations",
			ConstLabels: constLabels,
		},
		[]string{"method", "status"},
	)
}

func (m *GRPCCollector) RecordOperation(fullMethodName string, err error) {
	statusCode := "-"
	if status, ok := status.FromError(err); ok {
		statusCode = status.Code().String()
	}
	m.CounterVec.WithLabelValues(fullMethodName, statusCode).Inc()
}

var _ Collector = (*GRPCCollector)(nil)
