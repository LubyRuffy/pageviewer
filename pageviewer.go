package pageviewer

import (
	"time"

	"github.com/go-rod/rod"
)

var (
	DefaultWaitStableTimeout = time.Second * 20
)

func NewVisitOptions(opts ...VisitOption) *VisitOptions {
	// 生成配置项
	vo := &VisitOptions{
		PageOptions: &PageOptions{
			beforeRequest: nil,
			waitTimeout:   DefaultWaitStableTimeout,
		},
		browser: nil,
	}
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
	browser *Browser // 浏览器对象，只在Visit调用时有效
}

// VisitOption 访问配置项
type VisitOption func(vo *VisitOptions)

// WithWaitTimeout 设置等待超时时间
func WithWaitTimeout(timeout time.Duration) VisitOption {
	return func(vo *VisitOptions) {
		vo.waitTimeout = timeout
	}
}

// WithBrowser 设置浏览器对象
func WithBrowser(browser *Browser) VisitOption {
	return func(vo *VisitOptions) {
		vo.browser = browser
	}
}

// WithBeforeRequest 在请求之前的回调，做一些预处理操作
func WithBeforeRequest(f func(page *rod.Page) error) VisitOption {
	return func(vo *VisitOptions) {
		vo.beforeRequest = f
	}
}

// WithRemoveInvisibleDiv 移除不可见的div
func WithRemoveInvisibleDiv(removeInvisibleDiv bool) VisitOption {
	return func(vo *VisitOptions) {
		vo.removeInvisibleDiv = removeInvisibleDiv
	}
}

// Visit 访问页面
func Visit(u string, onPageLoad func(page *rod.Page) error, opts ...VisitOption) (err error) {
	// 生成配置项
	vo := NewVisitOptions(opts...)

	if vo.browser == nil {
		vo.browser = DefaultBrowser()
	}

	return vo.browser.run(u, onPageLoad, vo.PageOptions)
}
