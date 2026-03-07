package pageviewer

import (
	"context"
	"errors"
	"time"
)

var errWorkerPoolFull = errors.New("pageviewer: worker pool full")

type worker struct {
	id int
}

type workerState int

const (
	workerStateReady workerState = iota
	workerStateBroken
)

type workerPool struct {
	ch chan *worker
}

func newWorkerPool(size int) *workerPool {
	return &workerPool{ch: make(chan *worker, size)}
}

func (p *workerPool) fill(w *worker) error {
	select {
	case p.ch <- w:
		return nil
	default:
		return errWorkerPoolFull
	}
}

func (p *workerPool) acquire(ctx context.Context, timeout time.Duration) (*worker, func(workerState), error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case w := <-p.ch:
		return w, func(state workerState) {
			if state == workerStateReady {
				select {
				case p.ch <- w:
				default:
				}
			}
		}, nil
	case <-ctx.Done():
		return nil, nil, ErrAcquireTimeout
	}
}
