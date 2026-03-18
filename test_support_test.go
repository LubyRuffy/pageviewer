package pageviewer

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	sharedBrowserOnce sync.Once
	sharedBrowserBase *Browser
	sharedBrowserErr  error
)

func TestMain(m *testing.M) {
	DefaultWaitStableTimeout = 750 * time.Millisecond
	browserWaitLoadTimeoutCap = 750 * time.Millisecond
	browserWaitIdleTimeoutCap = 100 * time.Millisecond
	browserWaitRequestIdleTimeoutCap = 50 * time.Millisecond
	browserWaitDOMStableTimeoutCap = 100 * time.Millisecond
	browserCloseWaitTimeout = 500 * time.Millisecond
	browserKillWaitTimeout = 750 * time.Millisecond
	browserProcessPollInterval = 25 * time.Millisecond

	code := m.Run()

	if sharedBrowserBase != nil {
		_ = sharedBrowserBase.Close()
	}

	os.Exit(code)
}

func sharedTestBrowser(t *testing.T) *Browser {
	t.Helper()

	sharedBrowserOnce.Do(func() {
		sharedBrowserBase, sharedBrowserErr = NewBrowser()
	})

	require.NoError(t, sharedBrowserErr)
	require.NotNil(t, sharedBrowserBase)

	return &Browser{
		Browser:         sharedBrowserBase.Browser,
		UseUserMode:     sharedBrowserBase.UseUserMode,
		closeFn:         func() error { return nil },
		launcher:        sharedBrowserBase.launcher,
		leaklessEnabled: sharedBrowserBase.leaklessEnabled,
	}
}

func newTestClient(t *testing.T, cfg Config) *Client {
	t.Helper()

	cfg = cfg.withDefaults()
	browser := sharedTestBrowser(t)
	client := &Client{
		browser:        browser,
		pool:           newWorkerPool(cfg.PoolSize),
		traces:         newTraceRecorder(defaultTraceCapacity),
		poolSize:       cfg.PoolSize,
		acquireTimeout: cfg.AcquireTimeout,
		repairWorkers:  true,
		closeCh:        make(chan struct{}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), workerProvisionTimeout)
	defer cancel()

	for i := 0; i < cfg.Warmup; i++ {
		worker, err := newClientWorker(ctx, browser, client.allocateWorkerID())
		require.NoError(t, err)
		require.NoError(t, client.pool.fill(worker))
		client.addWorker(worker)
	}

	client.scheduleFillToPool()

	t.Cleanup(func() {
		err := client.Close()
		if err != nil && !errors.Is(err, context.Canceled) {
			require.NoError(t, err)
		}
	})

	return client
}
