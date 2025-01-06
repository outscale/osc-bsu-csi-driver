package driver

import (
	"context"
	"fmt"
	"strings"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/rs/xid"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

func LoggingInterceptor(version string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// no need to log identity requests, too many of them...
		if strings.HasPrefix(info.FullMethod, "/csi.v1.Identity/") {
			return handler(ctx, req)
		}
		kv := loggingContext(req, info, version)
		logger := klog.Background().WithValues(kv...)
		ctx = klog.NewContext(ctx, logger)
		if sreq, ok := req.(fmt.Stringer); ok {
			logger.V(5).Info("Request: " + sreq.String())
		} else {
			logger.V(5).Info("Request")
		}
		start := time.Now()
		resp, err := handler(ctx, req)
		dur := time.Since(start)
		if err == nil {
			logger.V(2).Info("Success", "duration", dur)
			if sresp, ok := resp.(fmt.Stringer); ok {
				logger.V(5).Info("Response: " + sresp.String())
			}
		} else {
			logger.V(2).Error(err, "Failure", "duration", dur)
		}
		return resp, err
	}
}

func loggingContext(req any, info *grpc.UnaryServerInfo, version string) []any {
	kv := []any{"span_id", xid.New().String(), "method", info.FullMethod, "version", version}
	switch req := req.(type) {
	case *csi.CreateVolumeRequest:
		kv = append(kv, "volume_name", req.GetName())
	case *csi.DeleteVolumeRequest:
		kv = append(kv, "volume_id", req.GetVolumeId())
	case *csi.ControllerPublishVolumeRequest:
		kv = append(kv, "volume_id", req.GetVolumeId(), "node_id", req.GetNodeId())
	case *csi.ControllerUnpublishVolumeRequest:
		kv = append(kv, "volume_id", req.GetVolumeId(), "node_id", req.GetNodeId())
	case *csi.ControllerExpandVolumeRequest:
		kv = append(kv, "volume_id", req.VolumeId)
	case *csi.CreateSnapshotRequest:
		kv = append(kv, "snapshot_name", req.GetName(), "volume_id", req.GetSourceVolumeId())
	case *csi.DeleteSnapshotRequest:
		kv = append(kv, "snapshot_id", req.GetSnapshotId())
	}
	return kv
}
