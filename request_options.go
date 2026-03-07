package pageviewer

import (
	"time"

	"github.com/go-rod/rod"
)

type RequestOptions struct {
	WaitTimeout        time.Duration
	AcquireTimeout     time.Duration
	BeforeRequest      func(page *rod.Page) error
	RemoveInvisibleDiv bool
	TraceID            string

	browser *Browser
}

type RequestOption func(ro *RequestOptions)

func NewRequestOptions(opts ...RequestOption) RequestOptions {
	ro := RequestOptions{
		WaitTimeout: DefaultWaitStableTimeout,
	}
	for _, opt := range opts {
		opt(&ro)
	}
	return ro
}

func WithAcquireTimeout(timeout time.Duration) RequestOption {
	return func(ro *RequestOptions) {
		ro.AcquireTimeout = timeout
	}
}

func (ro RequestOptions) toPageOptions() *PageOptions {
	return &PageOptions{
		waitTimeout:        ro.WaitTimeout,
		beforeRequest:      ro.BeforeRequest,
		removeInvisibleDiv: ro.RemoveInvisibleDiv,
	}
}
