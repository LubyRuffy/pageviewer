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

type RequestOption = VisitOption

func NewRequestOptions(opts ...RequestOption) RequestOptions {
	vo := newDefaultVisitOptions()
	for _, opt := range opts {
		opt(vo)
	}
	return vo.toRequestOptions()
}

func WithAcquireTimeout(timeout time.Duration) RequestOption {
	return func(vo *VisitOptions) {
		vo.acquireTimeout = timeout
	}
}

func (vo *VisitOptions) toRequestOptions() RequestOptions {
	return RequestOptions{
		WaitTimeout:        vo.PageOptions.waitTimeout,
		AcquireTimeout:     vo.acquireTimeout,
		BeforeRequest:      vo.PageOptions.beforeRequest,
		RemoveInvisibleDiv: vo.PageOptions.removeInvisibleDiv,
		TraceID:            vo.traceID,
		browser:            vo.browser,
	}
}
