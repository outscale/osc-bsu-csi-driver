package metrics_test

import (
	"errors"
	"os"
	"testing"

	"github.com/outscale/osc-bsu-csi-driver/pkg/driver/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadStats(t *testing.T) {
	// Skipping this test if it is not run on an Outscale VM
	if _, err := os.Stat("/sys/block/sda/stat"); errors.Is(err, os.ErrNotExist) {
		t.SkipNow()
	}
	bs, err := metrics.ReadStats("sda")
	require.NoError(t, err)
	require.NotNil(t, bs)
	assert.NotEmpty(t, *bs)
}
