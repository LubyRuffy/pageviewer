package pageviewer

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func replaceClientFactories(t *testing.T, browserFactory func(Config) (*Browser, error), workerFactory func(context.Context, *Browser, int) (*worker, error)) {
	t.Helper()

	oldBrowserFactory := newClientBrowser
	oldWorkerFactory := newClientWorker

	newClientBrowser = browserFactory
	newClientWorker = workerFactory

	t.Cleanup(func() {
		newClientBrowser = oldBrowserFactory
		newClientWorker = oldWorkerFactory
	})
}

func replaceNewBrowserWithOptions(t *testing.T, factory func(...BrowserOption) (*Browser, error)) {
	t.Helper()

	oldFactory := newBrowserWithOptions
	newBrowserWithOptions = factory

	t.Cleanup(func() {
		newBrowserWithOptions = oldFactory
	})
}

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

	warmWorker, release, err := client.pool.acquire(context.Background(), 200*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, warmWorker)
	require.NotNil(t, warmWorker.page)
	release(workerStateReady)
}

func TestStartWithCanceledContextSkipsBrowserStartup(t *testing.T) {
	var browserCreated int32

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	replaceClientFactories(t,
		func(Config) (*Browser, error) {
			atomic.AddInt32(&browserCreated, 1)
			return &Browser{}, nil
		},
		func(ctx context.Context, browser *Browser, id int) (*worker, error) {
			return &worker{id: id, closeFn: func() error { return nil }}, nil
		},
	)

	client, err := Start(ctx, Config{PoolSize: 1, Warmup: 1})
	require.Nil(t, client)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(0), atomic.LoadInt32(&browserCreated))
}

func TestStartClampsWarmupToPoolSize(t *testing.T) {
	var created int32

	replaceClientFactories(t,
		func(Config) (*Browser, error) {
			return &Browser{}, nil
		},
		func(ctx context.Context, browser *Browser, id int) (*worker, error) {
			atomic.AddInt32(&created, 1)
			return &worker{id: id, closeFn: func() error { return nil }}, nil
		},
	)

	client, err := Start(context.Background(), Config{
		PoolSize: 1,
		Warmup:   2,
	})
	require.NoError(t, err)
	defer client.Close()

	assert.Equal(t, 1, client.Stats().TotalWorkers)
	assert.Equal(t, int32(1), atomic.LoadInt32(&created))
}

func TestStartCancelsAfterBrowserCreationAndCleansUp(t *testing.T) {
	var browserClosed int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	replaceClientFactories(t,
		func(Config) (*Browser, error) {
			cancel()
			return &Browser{
				closeFn: func() error {
					atomic.AddInt32(&browserClosed, 1)
					return nil
				},
			}, nil
		},
		func(ctx context.Context, browser *Browser, id int) (*worker, error) {
			return &worker{id: id, closeFn: func() error { return nil }}, nil
		},
	)

	client, err := Start(ctx, Config{PoolSize: 1, Warmup: 0})
	require.Nil(t, client)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), atomic.LoadInt32(&browserClosed))
}

func TestStartCancelsDuringWarmupAndCleansUp(t *testing.T) {
	var browserClosed int32
	var workerClosed int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	replaceClientFactories(t,
		func(Config) (*Browser, error) {
			return &Browser{
				closeFn: func() error {
					atomic.AddInt32(&browserClosed, 1)
					return nil
				},
			}, nil
		},
		func(ctx context.Context, browser *Browser, id int) (*worker, error) {
			if id == 1 {
				return &worker{
					id: id,
					closeFn: func() error {
						atomic.AddInt32(&workerClosed, 1)
						return nil
					},
				}, nil
			}

			cancel()
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)

	client, err := Start(ctx, Config{
		PoolSize: 2,
		Warmup:   2,
	})
	require.Nil(t, client)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), atomic.LoadInt32(&browserClosed))
	assert.Equal(t, int32(1), atomic.LoadInt32(&workerClosed))
}

func TestCloseIsIdempotentWithMockedResources(t *testing.T) {
	var browserClosed int32
	var workerClosed int32

	replaceClientFactories(t,
		func(Config) (*Browser, error) {
			return &Browser{
				closeFn: func() error {
					atomic.AddInt32(&browserClosed, 1)
					return nil
				},
			}, nil
		},
		func(ctx context.Context, browser *Browser, id int) (*worker, error) {
			return &worker{
				id: id,
				closeFn: func() error {
					atomic.AddInt32(&workerClosed, 1)
					return nil
				},
			}, nil
		},
	)

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, client.Stats().TotalWorkers)
	require.NoError(t, client.Close())
	assert.Equal(t, 0, client.Stats().TotalWorkers)
	require.NoError(t, client.Close())
	assert.Equal(t, int32(1), atomic.LoadInt32(&browserClosed))
	assert.Equal(t, int32(1), atomic.LoadInt32(&workerClosed))
}

func TestCloseIsIdempotent(t *testing.T) {
	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)

	browser := client.browser

	require.NoError(t, client.Close())
	assert.Equal(t, 0, client.Stats().TotalWorkers)
	require.NoError(t, client.Close())

	_, err = browser.GetPage()
	require.Error(t, err)
}

func TestNewBrowserFromConfigPassesThroughOptions(t *testing.T) {
	var got browserOptions

	replaceNewBrowserWithOptions(t, func(opts ...BrowserOption) (*Browser, error) {
		for _, opt := range opts {
			opt(&got)
		}
		return &Browser{}, nil
	})

	_, err := newBrowserFromConfig(Config{
		Debug:               true,
		NoHeadless:          true,
		DevTools:            true,
		Proxy:               "http://127.0.0.1:7890",
		IgnoreCertErrors:    true,
		ChromePath:          "/tmp/chrome",
		UserModeBrowser:     true,
		RemoteDebuggingPort: 9222,
		UserDataDir:         "/tmp/profile",
	})
	require.NoError(t, err)
	assert.True(t, got.Debug)
	assert.True(t, got.NoHeadless)
	assert.True(t, got.DevTools)
	assert.Equal(t, "http://127.0.0.1:7890", got.Proxy)
	assert.True(t, got.IgnoreCertErrors)
	assert.Equal(t, "/tmp/chrome", got.ChromePath)
	assert.True(t, got.UserModeBrowser)
	assert.Equal(t, 9222, got.RemoteDebuggingPort)
	assert.Equal(t, "/tmp/profile", got.UserDataDir)
}
