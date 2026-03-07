package pageviewer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoolAcquireAndRelease(t *testing.T) {
	p := newWorkerPool(1)
	w := &worker{id: 1}
	require.NoError(t, p.fill(w))

	got, release, err := p.acquire(context.Background(), 50*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, w, got)

	release(workerStateReady)

	gotAgain, _, err := p.acquire(context.Background(), 50*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, w, gotAgain)
}

func TestPoolAcquireTimeout(t *testing.T) {
	p := newWorkerPool(1)
	require.NoError(t, p.fill(&worker{id: 1}))

	_, release, err := p.acquire(context.Background(), 50*time.Millisecond)
	require.NoError(t, err)
	defer release(workerStateReady)

	_, _, err = p.acquire(context.Background(), 10*time.Millisecond)
	assert.ErrorIs(t, err, ErrAcquireTimeout)
}
