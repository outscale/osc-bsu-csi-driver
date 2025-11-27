package cloud

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/outscale/osc-bsu-csi-driver/pkg/util"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// RetryOnHTTPCodes defines the list of HTTP codes for which we backoff.
var RetryOnHTTPCodes = []int{429, 500, 502, 503, 504}

type BackoffOpt func(*BackoffPolicy)

func RetryOnErrors() BackoffOpt {
	return func(bp *BackoffPolicy) {
		bp.retryOnErrors = true
	}
}

func WithBackoff(bo wait.Backoff) BackoffOpt {
	return func(bp *BackoffPolicy) {
		bp.backoff = bo
	}
}

func Steps(s int) BackoffOpt {
	return func(bp *BackoffPolicy) {
		bp.backoff.Steps = s
	}
}

type BackoffPolicyer interface {
	With(opts ...BackoffOpt) BackoffPolicyer
	ExponentialBackoff(ctx context.Context, fn func(ctx context.Context) (bool, error)) error
	OAPIResponseBackoff(ctx context.Context, resp *http.Response, err error) (bool, error)
}

type BackoffPolicy struct {
	retryOnErrors bool
	backoff       wait.Backoff
}

func NewBackoffPolicy(opts ...BackoffOpt) *BackoffPolicy {
	bp := &BackoffPolicy{
		backoff: EnvBackoff(),
	}
	for _, opt := range opts {
		opt(bp)
	}
	return bp
}

// With creates a new BackoffPolicy with different options.
func (bp *BackoffPolicy) With(opts ...BackoffOpt) BackoffPolicyer {
	nbp := *bp
	for _, opt := range opts {
		opt(&nbp)
	}
	return &nbp
}

// ExponentialBackoff repeats a condition check with exponential backoff.
// It stops if context is cancelled.
func (bp *BackoffPolicy) ExponentialBackoff(ctx context.Context, fn func(ctx context.Context) (bool, error)) error {
	// bp.backoff is not a pointer, a copy is used each time, ensuring that backoff restarts at 0 each time.
	return wait.ExponentialBackoffWithContext(ctx, bp.backoff, fn)
}

// OAPIResponseBackoff decides if an OAPI response requires a backoff. It retries only on RetryOnHTTPCodes http statuses.
// It retries on errors only if retryOnErrors is set.
func (bp *BackoffPolicy) OAPIResponseBackoff(ctx context.Context, resp *http.Response, err error) (bool, error) {
	switch {
	case resp != nil && slices.Contains(RetryOnHTTPCodes, resp.StatusCode):
		klog.FromContext(ctx).V(5).Info("Retrying...")
		return false, nil
	case err != nil && bp.retryOnErrors:
		klog.FromContext(ctx).V(5).Error(err, "Retrying...")
		return false, nil
	case err != nil:
		return false, extractOAPIError(err, resp)
	default:
		return true, nil
	}
}

var _ BackoffPolicyer = (*BackoffPolicy)(nil)

func EnvBackoff() wait.Backoff {
	// BACKOFF_DURATION duration The initial duration.
	// Fallback as int/duration in seconds.
	dur := util.Getenv("BACKOFF_DURATION", "1s")
	duration, err := time.ParseDuration(dur)
	if err != nil {
		d, derr := strconv.Atoi(dur)
		duration = time.Duration(d) * time.Second
		err = derr
	}
	if err != nil {
		duration = time.Second
	}

	// BACKOFF_FACTOR float Duration is multiplied by factor each iteration
	factor, err := strconv.ParseFloat(util.Getenv("BACKOFF_FACTOR", "2"), 32)
	if err != nil {
		factor = 2
	}

	// BACKOFF_STEPS integer : The remaining number of iterations in which
	// the duration parameter may change
	steps, err := strconv.Atoi(util.Getenv("BACKOFF_STEPS", "5"))
	if err != nil {
		steps = 5
	}
	return wait.Backoff{
		Duration: duration,
		Factor:   factor,
		Steps:    steps,
	}
}
