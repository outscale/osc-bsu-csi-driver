package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"k8s.io/klog/v2"
)

const maxResponseLength = 500

func clean(buf []byte) string {
	return strings.ReplaceAll(string(buf), `"`, ``)
}

func truncatedBody(httpResp *http.Response) string {
	body, err := io.ReadAll(httpResp.Body)
	if err == nil {
		str := []rune(clean(body))
		if len(str) > maxResponseLength {
			return string(str[:maxResponseLength/2]) + " [truncated] " + string(str[len(str)-maxResponseLength/2:])
		}
		return string(str)
	}
	return "(unable to fetch body)"
}

func logAPICall(ctx context.Context, call string, request, resp any, httpResp *http.Response, err error) {
	logger := klog.FromContext(ctx)
	if logger.V(5).Enabled() {
		logger = logger.WithCallDepth(1)
		buf, _ := json.Marshal(request)

		logger.Info("OAPI request: "+clean(buf), "OAPI", call)
		switch {
		case err != nil:
			logger.Error(err, "OAPI error", call, "OAPI", call)
		case httpResp.StatusCode > 299:
			logger.Info("OAPI error response: "+truncatedBody(httpResp), "OAPI", call, "http_status", httpResp.Status)
		default:
			logger.Info("OAPI response: "+truncatedBody(httpResp), "OAPI", call)
		}
	}
}
