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
	newClientBrowser = newBrowserFromConfig
	newClientWorker  = newWarmWorker
)

type Stats struct {
	TotalWorkers int
}

type Client struct {
	browser        *Browser
	pool           *workerPool
	closed         atomic.Bool
	totalWorkers   atomic.Int32
	acquireTimeout time.Duration
	ownsBrowser    bool
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
		acquireTimeout: cfg.AcquireTimeout,
		ownsBrowser:    true,
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
		w, workerErr := newClientWorker(ctx, browser, i+1)
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

	return client, nil
}

func (c *Client) Close() error {
	if c == nil || c.closed.Swap(true) {
		return nil
	}
	return c.closeResources()
}

func (c *Client) Stats() Stats {
	if c == nil {
		return Stats{}
	}

	return Stats{
		TotalWorkers: int(c.totalWorkers.Load()),
	}
}

func newCompatibilityClient(ctx context.Context, browser *Browser, acquireTimeout time.Duration) (*Client, error) {
	if acquireTimeout <= 0 {
		acquireTimeout = DefaultConfig().AcquireTimeout
	}

	client := &Client{
		browser:        browser,
		pool:           newWorkerPool(1),
		acquireTimeout: acquireTimeout,
	}

	worker, err := newClientWorker(ctx, browser, 1)
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
	return c.visitWithOptions(ctx, url, NewRequestOptions(opts...), func(page *rod.Page, _ *proto.NetworkResponseReceived) error {
		return fn(page)
	})
}

func (c *Client) HTML(ctx context.Context, url string, opts ...RequestOption) (string, error) {
	var html string
	err := c.visitWithOptions(ctx, url, NewRequestOptions(opts...), func(page *rod.Page, _ *proto.NetworkResponseReceived) error {
		var err error
		html, err = page.HTML()
		return err
	})
	return html, err
}

func (c *Client) Links(ctx context.Context, url string, opts ...RequestOption) (string, error) {
	var links string
	err := c.visitWithOptions(ctx, url, NewRequestOptions(opts...), func(page *rod.Page, _ *proto.NetworkResponseReceived) error {
		var err error
		links, err = collectLinks(page)
		return err
	})
	return links, err
}

func (c *Client) ReadabilityArticle(ctx context.Context, url string, opts ...RequestOption) (ReadabilityArticleWithMarkdown, error) {
	var article ReadabilityArticleWithMarkdown
	err := c.visitWithOptions(ctx, url, NewRequestOptions(opts...), func(page *rod.Page, response *proto.NetworkResponseReceived) error {
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

	page, err := browser.GetPage()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = page.Close()
		return nil, err
	}

	return &worker{
		id:      id,
		page:    page,
		closeFn: page.Close,
	}, nil
}

func (c *Client) addWorker(w *worker) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.workers = append(c.workers, w)
	c.totalWorkers.Store(int32(len(c.workers)))
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

func (c *Client) visitWithOptions(ctx context.Context, url string, ro RequestOptions, onPageLoad func(page *rod.Page, response *proto.NetworkResponseReceived) error) error {
	if c == nil {
		return ErrBrowserUnavailable
	}
	if c.closed.Load() {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.browser == nil || c.pool == nil {
		return ErrBrowserUnavailable
	}

	worker, release, err := c.pool.acquire(ctx, c.acquireWorkerTimeout(ctx, ro))
	if err != nil {
		return err
	}

	state := workerStateReady
	defer func() {
		release(state)
		if state == workerStateBroken {
			c.repairWorker(worker)
		}
	}()

	pageBroken, err := c.browser.runPage(worker.page, url, ro.pageOptions(), onPageLoad)
	if pageBroken {
		state = workerStateBroken
	}

	return err
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

func (c *Client) repairWorker(target *worker) {
	if c == nil || target == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	index := c.workerIndexLocked(target)
	if index == -1 {
		_ = target.close()
		return
	}

	_ = target.close()
	if c.closed.Load() || c.browser == nil || c.pool == nil {
		c.removeWorkerAtLocked(index)
		return
	}

	replacement, err := newClientWorker(context.Background(), c.browser, target.id)
	if err != nil {
		c.removeWorkerAtLocked(index)
		return
	}
	if err := c.pool.fill(replacement); err != nil {
		_ = replacement.close()
		c.removeWorkerAtLocked(index)
		return
	}

	c.workers[index] = replacement
	c.totalWorkers.Store(int32(len(c.workers)))
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
