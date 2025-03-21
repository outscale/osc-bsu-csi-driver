package cloud_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/outscale/osc-bsu-csi-driver/pkg/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestEnvBackoff(t *testing.T) {
	var tcs = []struct {
		name    string
		env     []string
		backoff wait.Backoff
	}{{
		name: "default values",
		backoff: wait.Backoff{
			Duration: 750 * time.Millisecond,
			Factor:   1.4,
			Steps:    3,
		},
	}, {
		name: "environnement based config",
		env:  []string{"BACKOFF_DURATION=2s", "BACKOFF_FACTOR=1.6", "BACKOFF_STEPS=4"},
		backoff: wait.Backoff{
			Duration: 2 * time.Second,
			Factor:   1.6,
			Steps:    4,
		},
	}, {
		name: "compatibility with numeric durations",
		env:  []string{"BACKOFF_DURATION=2"},
		backoff: wait.Backoff{
			Duration: 2 * time.Second,
			Factor:   1.4,
			Steps:    3,
		},
	}}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			for _, env := range tc.env {
				kv := strings.Split(env, "=")
				t.Setenv(kv[0], kv[1])
			}
			bo := cloud.EnvBackoff()
			assert.Equal(t, tc.backoff.Duration, bo.Duration)
			assert.InEpsilon(t, tc.backoff.Factor, bo.Factor, 0.01)
			assert.Equal(t, tc.backoff.Steps, bo.Steps)
		})
	}
}

func TestBackoffPolicy_ExponentialBackoff(t *testing.T) {
	var count int
	fn := func(context.Context) (bool, error) {
		count++
		return false, nil
	}
	bo := cloud.NewBackoffPolicy(cloud.WithBackoff(wait.Backoff{
		Duration: time.Millisecond,
		Steps:    2,
	}))
	t.Run("When called multiple times, backoff is triggered again", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			err := bo.ExponentialBackoff(context.TODO(), fn)
			require.Error(t, err, "It should return a timeout error")
			assert.Equal(t, (i+1)*2, count)
		}
	})
}
