package pageviewer

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-rod/rod"
)

var errWorkerPoolFull = errors.New("pageviewer: worker pool full")

type worker struct {
	id      int
	page    *rod.Page
	closeFn func() error
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

func (w *worker) close() error {
	if w == nil {
		return nil
	}
	if w.closeFn != nil {
		return w.closeFn()
	}
	if w.page != nil {
		return w.page.Close()
	}
	return nil
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
	if timeout <= 0 {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
			return nil, nil, ErrAcquireTimeout
		}
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case w := <-p.ch:
		var releaseOnce sync.Once
		return w, func(state workerState) {
			releaseOnce.Do(func() {
				if state == workerStateReady {
					select {
					case p.ch <- w:
					default:
					}
				}
			})
		}, nil
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-timer.C:
		return nil, nil, ErrAcquireTimeout
	}
}
