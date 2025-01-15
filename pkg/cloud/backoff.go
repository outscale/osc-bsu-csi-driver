package cloud

import (
	"context"
	"net/http"
	"slices"

	"k8s.io/klog/v2"
)

// RetryOnHTTPCodes defines the list of HTTP codes for which we backoff.
var RetryOnHTTPCodes = []int{429, 500, 502, 503, 504}

type BackoffPolicy func(ctx context.Context, resp *http.Response, err error) (bool, error)

// NoRetryOnErrors is the default backoff policy: retry only on RetryOnHTTPCodes http statuses.
// No retry on errors.
func NoRetryOnErrors(ctx context.Context, resp *http.Response, err error) (bool, error) {
	switch {
	case resp != nil && slices.Contains(RetryOnHTTPCodes, resp.StatusCode):
		klog.FromContext(ctx).V(5).Info("Retrying...")
		return false, nil
	case err != nil:
		return false, err
	default:
		return true, nil
	}
}

// NoRetryOnErrors is an alternate policy that retries on all errors.
func RetryOnErrors(ctx context.Context, resp *http.Response, err error) (bool, error) {
	switch {
	case resp != nil && slices.Contains(RetryOnHTTPCodes, resp.StatusCode):
		klog.FromContext(ctx).V(5).Info("Retrying...")
		return false, nil
	case err != nil:
		klog.FromContext(ctx).V(5).Error(err, "Retrying...")
		return false, nil
	default:
		return true, nil
	}
}

var _ BackoffPolicy = NoRetryOnErrors
var _ BackoffPolicy = RetryOnErrors

// DefaultBackoffPolicy is the default BackoffPolicy (NoRetryOnErrors)
var DefaultBackoffPolicy = NoRetryOnErrors
