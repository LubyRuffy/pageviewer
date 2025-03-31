package pageviewer

import (
	"github.com/go-rod/rod"
	"time"
)

var (
	DefaultWaitStableTimeout = time.Second * 20
)

func NewVisitOptions(opts ...VisitOption) *VisitOptions {
	// 生成配置项
	vo := &VisitOptions{
		waitTimeout: DefaultWaitStableTimeout,
		browser:     DefaultBrowser(),
	}
	for _, opt := range opts {
		opt(vo)
	}
	return vo
}

type VisitOptions struct {
	waitTimeout   time.Duration              // 等待超时的设置
	browser       *Browser                   // 浏览器对象，只在Visit调用时有效
	beforeRequest func(page *rod.Page) error // 在请求之前的回调，做一些
}

// VisitOption 访问配置项
type VisitOption func(vo *VisitOptions)

func WithWaitTimeout(timeout time.Duration) VisitOption {
	return func(vo *VisitOptions) {
		vo.waitTimeout = timeout
	}
}
func WithBrowser(browser *Browser) VisitOption {
	return func(vo *VisitOptions) {
		vo.browser = browser
	}
}

func WithBeforeRequest(f func(page *rod.Page) error) VisitOption {
	return func(vo *VisitOptions) {
		vo.beforeRequest = f
	}
}

func Visit(u string, onPageLoad func(page *rod.Page) error, opts ...VisitOption) (err error) {
	// 生成配置项
	vo := NewVisitOptions(opts...)

	if vo.browser == nil {
		vo.browser = DefaultBrowser()
	}

	return vo.browser.run(u, onPageLoad, vo)
}
