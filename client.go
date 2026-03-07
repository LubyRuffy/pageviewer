package pageviewer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	newClientBrowser = newBrowserFromConfig
	newClientWorker  = newWarmWorker
)

type Stats struct {
	TotalWorkers int
}

type Client struct {
	browser      *Browser
	pool         *workerPool
	closed       atomic.Bool
	totalWorkers atomic.Int32
	mu           sync.Mutex
	workers      []*worker
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
		browser: browser,
		pool:    newWorkerPool(cfg.PoolSize),
	}

	defer func() {
		if err != nil {
			_ = client.closeResources()
		}
	}()

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
	if browser != nil {
		if err := browser.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
