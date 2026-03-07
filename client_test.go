package pageviewer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartCreatesClientWithWarmWorkers(t *testing.T) {
	client, err := Start(context.Background(), Config{
		PoolSize:       1,
		Warmup:         1,
		AcquireTimeout: time.Second,
	})
	require.NoError(t, err)
	defer client.Close()

	stats := client.Stats()
	assert.Equal(t, 1, stats.TotalWorkers)
}

func TestCloseIsIdempotent(t *testing.T) {
	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	require.NoError(t, client.Close())
	require.NoError(t, client.Close())
}
