package pageviewer

import (
	"context"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

var (
	DefaultWaitStableTimeout = time.Second * 20
)

func newDefaultVisitOptions() *VisitOptions {
	return &VisitOptions{
		PageOptions: &PageOptions{
			waitTimeout: DefaultWaitStableTimeout,
		},
	}
}

func NewVisitOptions(opts ...VisitOption) *VisitOptions {
	vo := newDefaultVisitOptions()
	for _, opt := range opts {
		opt(vo)
	}
	if vo.browser == nil {
		vo.browser = DefaultBrowser()
	}
	return vo
}

// VisitOptions 访问配置项
type VisitOptions struct {
	*PageOptions
	browser        *Browser // 浏览器对象，只在Visit调用时有效
	acquireTimeout time.Duration
	traceID        string
}

// VisitOption 访问配置项
type VisitOption func(vo *VisitOptions)

// WithWaitTimeout 设置等待超时时间
func WithWaitTimeout(timeout time.Duration) VisitOption {
	return func(vo *VisitOptions) {
		vo.PageOptions.waitTimeout = timeout
	}
}

// WithBrowser 设置浏览器对象
func WithBrowser(browser *Browser) VisitOption {
	return func(vo *VisitOptions) {
		vo.browser = browser
	}
}

// WithTraceID 设置排障链路 TraceID
func WithTraceID(traceID string) VisitOption {
	return func(vo *VisitOptions) {
		vo.traceID = traceID
	}
}

// WithBeforeRequest 在请求之前的回调，做一些预处理操作
func WithBeforeRequest(f func(page *rod.Page) error) VisitOption {
	return func(vo *VisitOptions) {
		vo.PageOptions.beforeRequest = f
	}
}

// WithRemoveInvisibleDiv 移除不可见的div
func WithRemoveInvisibleDiv(removeInvisibleDiv bool) VisitOption {
	return func(vo *VisitOptions) {
		vo.PageOptions.removeInvisibleDiv = removeInvisibleDiv
	}
}

// Visit 访问页面
func Visit(u string, onPageLoad func(page *rod.Page) error, opts ...VisitOption) (err error) {
	vo := NewVisitOptions(opts...)
	clientCtx, cancel := context.WithTimeout(context.Background(), workerProvisionTimeout)
	defer cancel()

	client, err := newCompatibilityClient(clientCtx, vo.browser, vo.acquireTimeout)
	if err != nil {
		return err
	}
	defer client.Close()

	return client.visitWithOptions(context.Background(), u, vo.toRequestOptions(), true, func(page *rod.Page, _ *proto.NetworkResponseReceived) error {
		return onPageLoad(page)
	})
}
