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

func TestPoolAcquireReturnsContextCanceled(t *testing.T) {
	p := newWorkerPool(1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := p.acquire(ctx, 50*time.Millisecond)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPoolFillReturnsErrorWhenFull(t *testing.T) {
	p := newWorkerPool(1)
	require.NoError(t, p.fill(&worker{id: 1}))

	done := make(chan error, 1)
	go func() {
		done <- p.fill(&worker{id: 2})
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, errWorkerPoolFull)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("fill blocked when pool was full")
	}
}

func TestPoolReleaseBrokenDoesNotReturnWorker(t *testing.T) {
	p := newWorkerPool(1)
	w := &worker{id: 1}
	require.NoError(t, p.fill(w))

	got, release, err := p.acquire(context.Background(), 50*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, w, got)

	release(workerStateBroken)

	_, _, err = p.acquire(context.Background(), 10*time.Millisecond)
	assert.ErrorIs(t, err, ErrAcquireTimeout)
}

func TestPoolReleaseDoesNotBlockWhenPoolIsFull(t *testing.T) {
	p := newWorkerPool(1)
	w1 := &worker{id: 1}
	w2 := &worker{id: 2}
	require.NoError(t, p.fill(w1))

	got, release, err := p.acquire(context.Background(), 50*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, w1, got)

	require.NoError(t, p.fill(w2))

	done := make(chan struct{}, 1)
	go func() {
		release(workerStateReady)
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("release blocked when pool was full")
	}

	gotAgain, releaseAgain, err := p.acquire(context.Background(), 50*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, w2, gotAgain)
	releaseAgain(workerStateBroken)

	_, _, err = p.acquire(context.Background(), 10*time.Millisecond)
	assert.ErrorIs(t, err, ErrAcquireTimeout)
}

func TestPoolReleaseReadyDoesNotReturnSameWorkerTwice(t *testing.T) {
	p := newWorkerPool(2)
	w := &worker{id: 1}
	require.NoError(t, p.fill(w))

	got, release, err := p.acquire(context.Background(), 50*time.Millisecond)
	require.NoError(t, err)
	assert.Same(t, w, got)

	release(workerStateReady)
	release(workerStateReady)

	gotAgain, releaseAgain, err := p.acquire(context.Background(), 50*time.Millisecond)
	require.NoError(t, err)
	assert.Same(t, w, gotAgain)
	releaseAgain(workerStateBroken)

	_, _, err = p.acquire(context.Background(), 10*time.Millisecond)
	assert.ErrorIs(t, err, ErrAcquireTimeout)
}
