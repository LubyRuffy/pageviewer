package pageviewer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/defaults"
	"github.com/stretchr/testify/require"
)

var (
	sharedBrowserOnce sync.Once
	sharedBrowserBase *Browser
	sharedBrowserErr  error
)

const testBrowserLockPortEnv = "PAGEVIEWER_TEST_BROWSER_LOCK_PORT"

func TestMain(m *testing.M) {
	if err := configureTestBrowserLockPort(); err != nil {
		panic(err)
	}

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

func configureTestBrowserLockPort() error {
	port, err := configuredTestBrowserLockPort()
	if err != nil {
		return err
	}
	defaults.LockPort = port
	return nil
}

func configuredTestBrowserLockPort() (int, error) {
	if value := os.Getenv(testBrowserLockPortEnv); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("parse %s: %w", testBrowserLockPortEnv, err)
		}
		return port, nil
	}

	port, err := pickTestBrowserLockPort(defaults.LockPort)
	if err != nil {
		return 0, err
	}
	if err := os.Setenv(testBrowserLockPortEnv, strconv.Itoa(port)); err != nil {
		return 0, err
	}
	return port, nil
}

func pickTestBrowserLockPort(preferred int) (int, error) {
	if preferred > 0 {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferred))
		if err == nil {
			port := listener.Addr().(*net.TCPAddr).Port
			_ = listener.Close()
			return port, nil
		}
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port, nil
}

func sharedTestBrowser(t *testing.T) *Browser {
	t.Helper()

	sharedBrowserOnce.Do(func() {
		sharedBrowserBase, sharedBrowserErr = NewBrowser(WithLeakless(false))
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

func TestPickTestBrowserLockPortFallsBackWhenPreferredPortIsOccupied(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	occupiedPort := listener.Addr().(*net.TCPAddr).Port
	port, err := pickTestBrowserLockPort(occupiedPort)
	require.NoError(t, err)
	require.NotEqual(t, occupiedPort, port)
}

func TestPickTestBrowserLockPortUsesConfiguredEnv(t *testing.T) {
	t.Setenv(testBrowserLockPortEnv, "43123")

	port, err := configuredTestBrowserLockPort()
	require.NoError(t, err)
	require.Equal(t, 43123, port)
}
