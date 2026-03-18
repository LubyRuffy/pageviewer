package pageviewer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

var (
	newClientBrowser       = newBrowserFromConfig
	newClientWorker        = newWarmWorker
	workerProvisionTimeout = 30 * time.Second
	workerRepairRetryDelay = 50 * time.Millisecond
)

type Stats struct {
	TotalWorkers int
	IdleWorkers  int
	RecentTraces int
	LastError    string
}

type Client struct {
	browser        *Browser
	pool           *workerPool
	traces         *traceRecorder
	poolSize       int
	closed         atomic.Bool
	fillScheduled  atomic.Bool
	nextWorkerID   atomic.Int32
	totalWorkers   atomic.Int32
	acquireTimeout time.Duration
	ownsBrowser    bool
	repairWorkers  bool
	closeCh        chan struct{}
	inflight       sync.WaitGroup
	stateMu        sync.RWMutex
	fillMu         sync.Mutex
	mu             sync.Mutex
	workers        []*worker
}

func Start(ctx context.Context, cfg Config) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cfg = cfg.withDefaults()

	browser, err := newClientBrowser(cfg)
	if err != nil {
		return nil, err
	}

	client := &Client{
		browser:        browser,
		pool:           newWorkerPool(cfg.PoolSize),
		traces:         newTraceRecorder(defaultTraceCapacity),
		poolSize:       cfg.PoolSize,
		acquireTimeout: cfg.AcquireTimeout,
		ownsBrowser:    true,
		repairWorkers:  true,
		closeCh:        make(chan struct{}),
	}

	defer func() {
		if err != nil {
			_ = client.closeResources()
		}
	}()

	if err = ctx.Err(); err != nil {
		return nil, err
	}

	for i := 0; i < cfg.Warmup; i++ {
		w, workerErr := newClientWorker(ctx, browser, client.allocateWorkerID())
		if workerErr != nil {
			err = workerErr
			return nil, err
		}
		if fillErr := client.pool.fill(w); fillErr != nil {
			err = errors.Join(fillErr, w.close())
			return nil, err
		}
		client.addWorker(w)
	}

	client.scheduleFillToPool()

	return client, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.stateMu.Lock()
	if c.closed.Swap(true) {
		c.stateMu.Unlock()
		return nil
	}
	if c.closeCh != nil {
		close(c.closeCh)
	}
	c.stateMu.Unlock()

	c.inflight.Wait()
	return c.closeResources()
}

func (c *Client) Stats() Stats {
	if c == nil {
		return Stats{}
	}

	idleWorkers := 0
	c.mu.Lock()
	if c.pool != nil {
		idleWorkers = len(c.pool.ch)
	}
	c.mu.Unlock()

	recentTraces, lastError := c.traceStats()

	return Stats{
		TotalWorkers: int(c.totalWorkers.Load()),
		IdleWorkers:  idleWorkers,
		RecentTraces: recentTraces,
		LastError:    lastError,
	}
}

func (c *Client) DebugTrace(id string) (Trace, bool) {
	if c == nil || c.traces == nil || id == "" {
		return Trace{}, false
	}
	return c.traces.get(id)
}

func newCompatibilityClient(ctx context.Context, browser *Browser, acquireTimeout time.Duration) (*Client, error) {
	if acquireTimeout <= 0 {
		acquireTimeout = DefaultConfig().AcquireTimeout
	}

	client := &Client{
		browser:        browser,
		pool:           newWorkerPool(1),
		traces:         newTraceRecorder(defaultTraceCapacity),
		poolSize:       1,
		acquireTimeout: acquireTimeout,
		repairWorkers:  false,
		closeCh:        make(chan struct{}),
	}

	worker, err := newClientWorker(ctx, browser, client.allocateWorkerID())
	if err != nil {
		return nil, err
	}
	if err := client.pool.fill(worker); err != nil {
		_ = worker.close()
		return nil, err
	}
	client.addWorker(worker)

	return client, nil
}

func (c *Client) Visit(ctx context.Context, url string, fn func(page *rod.Page) error, opts ...RequestOption) error {
	return c.visitWithOptions(ctx, url, NewRequestOptions(opts...), false, func(page *rod.Page, _ *proto.NetworkResponseReceived) error {
		return fn(page)
	})
}

func (c *Client) HTML(ctx context.Context, url string, opts ...RequestOption) (string, error) {
	var html string
	err := c.visitWithOptions(ctx, url, NewRequestOptions(opts...), true, func(page *rod.Page, _ *proto.NetworkResponseReceived) error {
		var err error
		html, err = page.HTML()
		return err
	})
	return html, err
}

func (c *Client) Links(ctx context.Context, url string, opts ...RequestOption) (string, error) {
	var links string
	err := c.visitWithOptions(ctx, url, NewRequestOptions(opts...), true, func(page *rod.Page, _ *proto.NetworkResponseReceived) error {
		var err error
		links, err = collectLinks(page)
		return err
	})
	return links, err
}

func (c *Client) ReadabilityArticle(ctx context.Context, url string, opts ...RequestOption) (ReadabilityArticleWithMarkdown, error) {
	var article ReadabilityArticleWithMarkdown
	err := c.visitWithOptions(ctx, url, NewRequestOptions(opts...), true, func(page *rod.Page, response *proto.NetworkResponseReceived) error {
		if rawHTML, err := readResponseBody(page, response); err == nil {
			article.RawHTML = rawHTML
		}
		return fillReadabilityArticle(page, url, &article)
	})
	return article, err
}

func newWarmWorker(ctx context.Context, browser *Browser, id int) (*worker, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if browser == nil {
		return nil, ErrBrowserUnavailable
	}

	type getPageResult struct {
		page *rod.Page
		err  error
	}

	resultCh := make(chan getPageResult, 1)
	go func() {
		page, err := browser.GetPage()
		resultCh <- getPageResult{page: page, err: err}
	}()

	var result getPageResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		go func() {
			result := <-resultCh
			if result.err == nil && result.page != nil {
				_ = result.page.Close()
			}
		}()
		return nil, ctx.Err()
	}
	if result.err != nil {
		return nil, result.err
	}
	if err := ctx.Err(); err != nil {
		_ = result.page.Close()
		return nil, err
	}

	return &worker{
		id:      id,
		page:    result.page,
		closeFn: result.page.Close,
	}, nil
}

func (c *Client) addWorker(w *worker) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.addWorkerLocked(w)
}

func (c *Client) addWorkerLocked(w *worker) {
	c.workers = append(c.workers, w)
	c.totalWorkers.Store(int32(len(c.workers)))
}

func (c *Client) allocateWorkerID() int {
	if c == nil {
		return 0
	}
	return int(c.nextWorkerID.Add(1))
}

func (c *Client) closeResources() error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	workers := c.workers
	browser := c.browser
	ownsBrowser := c.ownsBrowser
	c.workers = nil
	c.browser = nil
	c.pool = nil
	c.totalWorkers.Store(0)
	c.mu.Unlock()

	var errs []error
	for _, w := range workers {
		if w == nil {
			continue
		}
		if err := w.close(); err != nil {
			errs = append(errs, err)
		}
	}
	if ownsBrowser && browser != nil {
		if err := browser.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (c *Client) visitWithOptions(ctx context.Context, url string, ro RequestOptions, reuseWorker bool, onPageLoad func(page *rod.Page, response *proto.NetworkResponseReceived) error) (err error) {
	trace := c.beginTrace(ro.TraceID, traceModeDOM, url)
	defer func() {
		trace.finish(err)
	}()

	if err = c.beginTrackedOperation(); err != nil {
		return err
	}
	defer c.inflight.Done()

	if err = ctx.Err(); err != nil {
		return err
	}
	if c.browser == nil || c.pool == nil {
		return ErrBrowserUnavailable
	}

	acquireCtx, stopAcquire := c.acquireContext(ctx)
	defer stopAcquire()

	acquireStart := time.Now()
	worker, release, err := c.pool.acquire(acquireCtx, c.acquireWorkerTimeout(ctx, ro))
	trace.setAcquireWait(time.Since(acquireStart))
	if err != nil {
		if errors.Is(err, context.Canceled) && c.closed.Load() && ctx.Err() == nil {
			err = ErrClosed
		}
		return err
	}
	trace.setWorkerID(worker.id)
	if c.closed.Load() {
		release(workerStateReady)
		return ErrClosed
	}

	state := workerStateReady
	defer func() {
		release(state)
		if state == workerStateBroken {
			trace.markBrokenWorker()
			if c.repairWorkers {
				c.scheduleRepair(worker)
				return
			}
			c.retireWorker(worker)
		}
	}()

	response, pageBroken, err := c.browser.runPage(ctx, worker.page, url, ro.pageOptions(), func(page *rod.Page, response *proto.NetworkResponseReceived) error {
		return onPageLoad(page, response)
	})
	trace.setResponse(response)
	if pageBroken || !reuseWorker || !isReusableWorkerPage(worker.page) {
		state = workerStateBroken
	}

	return err
}

func (c *Client) beginTrackedOperation() error {
	if c == nil {
		return ErrBrowserUnavailable
	}

	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	if c.closed.Load() {
		return ErrClosed
	}
	c.inflight.Add(1)
	return nil
}

func (c *Client) startBackgroundTask(task func()) bool {
	if c == nil {
		return false
	}

	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	if c.closed.Load() {
		return false
	}
	c.inflight.Add(1)

	go func() {
		defer c.inflight.Done()
		task()
	}()
	return true
}

func (c *Client) acquireContext(ctx context.Context) (context.Context, func()) {
	if c == nil || c.closeCh == nil {
		return ctx, func() {}
	}

	acquireCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)
		select {
		case <-acquireCtx.Done():
		case <-c.closeCh:
			cancel()
		}
	}()

	return acquireCtx, func() {
		cancel()
		<-done
	}
}

func (c *Client) acquireWorkerTimeout(ctx context.Context, ro RequestOptions) time.Duration {
	timeout := c.acquireTimeout
	if timeout <= 0 {
		timeout = DefaultConfig().AcquireTimeout
	}
	if ro.AcquireTimeout > 0 && ro.AcquireTimeout < timeout {
		timeout = ro.AcquireTimeout
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	return timeout
}

func (ro RequestOptions) pageOptions() *PageOptions {
	return &PageOptions{
		waitTimeout:        ro.WaitTimeout,
		beforeRequest:      ro.BeforeRequest,
		removeInvisibleDiv: ro.RemoveInvisibleDiv,
	}
}

func (c *Client) traceStats() (int, string) {
	if c == nil || c.traces == nil {
		return 0, ""
	}
	return c.traces.stats()
}

func (c *Client) retireWorker(target *worker) {
	if target == nil {
		return
	}
	_ = target.close()
	target.page = nil
	target.closeFn = nil
}

func (c *Client) repairWorker(target *worker) {
	if c == nil || target == nil {
		return
	}

	_ = target.close()

	for {
		c.mu.Lock()
		index := c.workerIndexLocked(target)
		browser := c.browser
		closed := c.closed.Load()
		c.mu.Unlock()

		if index == -1 || closed || browser == nil || c.pool == nil {
			return
		}

		repairCtx, cancel := c.provisionContext()
		replacement, err := newClientWorker(repairCtx, browser, target.id)
		cancel()
		if err != nil {
			if !sleepWithClose(c.closeCh, workerRepairRetryDelay) {
				return
			}
			continue
		}
		c.mu.Lock()
		currentIndex := c.workerIndexLocked(target)
		if currentIndex == -1 || c.closed.Load() || c.browser == nil || c.pool == nil {
			c.mu.Unlock()
			_ = replacement.close()
			return
		}
		if err := c.pool.fill(replacement); err != nil {
			c.mu.Unlock()
			_ = replacement.close()
			if !sleepWithClose(c.closeCh, workerRepairRetryDelay) {
				return
			}
			continue
		}
		c.workers[currentIndex] = replacement
		c.totalWorkers.Store(int32(len(c.workers)))
		c.mu.Unlock()
		return
	}
}

func (c *Client) workerIndexLocked(target *worker) int {
	for i, worker := range c.workers {
		if worker == target {
			return i
		}
	}
	return -1
}

func (c *Client) removeWorkerAtLocked(index int) {
	c.workers = append(c.workers[:index], c.workers[index+1:]...)
	c.totalWorkers.Store(int32(len(c.workers)))
}

func isReusableWorkerPage(page *rod.Page) bool {
	if page == nil {
		return false
	}
	_, err := page.Info()
	return err == nil
}

func (c *Client) scheduleRepair(target *worker) {
	if c == nil || target == nil {
		return
	}
	c.startBackgroundTask(func() {
		c.repairWorker(target)
	})
}

func (c *Client) scheduleFillToPool() {
	if c == nil || c.poolSize <= 0 {
		return
	}
	if !c.fillScheduled.CompareAndSwap(false, true) {
		return
	}
	if !c.startBackgroundTask(func() {
		defer c.fillScheduled.Store(false)
		c.fillToPool()
	}) {
		c.fillScheduled.Store(false)
	}
}

func (c *Client) fillToPool() {
	if c == nil {
		return
	}

	c.fillMu.Lock()
	defer c.fillMu.Unlock()

	for {
		c.mu.Lock()
		browser := c.browser
		pool := c.pool
		workerCount := len(c.workers)
		poolSize := c.poolSize
		closed := c.closed.Load()
		c.mu.Unlock()

		if closed || browser == nil || pool == nil || workerCount >= poolSize {
			return
		}

		repairCtx, cancel := c.provisionContext()
		worker, err := newClientWorker(repairCtx, browser, c.allocateWorkerID())
		cancel()
		if err != nil {
			if !sleepWithClose(c.closeCh, workerRepairRetryDelay) {
				return
			}
			continue
		}

		c.mu.Lock()
		if c.closed.Load() || c.browser == nil || c.pool == nil || len(c.workers) >= c.poolSize {
			c.mu.Unlock()
			_ = worker.close()
			return
		}
		if err := c.pool.fill(worker); err != nil {
			c.mu.Unlock()
			_ = worker.close()
			if !sleepWithClose(c.closeCh, workerRepairRetryDelay) {
				return
			}
			continue
		}
		c.addWorkerLocked(worker)
		c.mu.Unlock()
	}
}

func (c *Client) provisionContext() (context.Context, func()) {
	provisionCtx, cancel := context.WithTimeout(context.Background(), workerProvisionTimeout)
	if c == nil || c.closeCh == nil {
		return provisionCtx, cancel
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-provisionCtx.Done():
		case <-c.closeCh:
			cancel()
		}
	}()

	return provisionCtx, func() {
		cancel()
		<-done
	}
}

func sleepWithClose(closeCh <-chan struct{}, d time.Duration) bool {
	if closeCh == nil {
		time.Sleep(d)
		return true
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-closeCh:
		return false
	case <-timer.C:
		return true
	}
}
