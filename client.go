package pageviewer

import (
	"context"
	"sync/atomic"
)

type Stats struct {
	TotalWorkers int
}

type Client struct {
	browser      *Browser
	pool         *workerPool
	closed       atomic.Bool
	totalWorkers atomic.Int32
}

func Start(ctx context.Context, cfg Config) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cfg = cfg.withDefaults()

	browser, err := newBrowserFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	client := &Client{
		browser: browser,
		pool:    newWorkerPool(cfg.PoolSize),
	}

	for i := 0; i < cfg.Warmup; i++ {
		if err := client.pool.fill(&worker{id: i + 1}); err != nil {
			_ = client.browser.Close()
			return nil, err
		}
		client.totalWorkers.Add(1)
	}

	return client, nil
}

func (c *Client) Close() error {
	if c == nil || c.closed.Swap(true) {
		return nil
	}
	if c.browser == nil {
		return nil
	}
	return c.browser.Close()
}

func (c *Client) Stats() Stats {
	if c == nil {
		return Stats{}
	}

	return Stats{
		TotalWorkers: int(c.totalWorkers.Load()),
	}
}
