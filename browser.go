package pageviewer

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"runtime/debug"
	"time"
)

var (
	defaultBrowser *Browser
)

type Browser struct {
	*rod.Browser
}

func (b *Browser) GetPage() (*rod.Page, error) {
	// 默认支持浏览器识别绕过
	return stealth.Page(b.Browser)
}

func (b *Browser) Close() error {
	return b.Browser.Close()
}

func (b *Browser) waitPageReady(u string, vo *VisitOptions) (*rod.Page, error) {
	page, err := b.GetPage()
	if err != nil {
		return nil, err
	}

	if vo.beforeRequest != nil {
		err := vo.beforeRequest(page)
		if err != nil {
			return nil, err
		}
	}

	s := time.Now()
	err = page.Navigate(u)
	if err != nil {
		return nil, err
	}

	err = page.WaitLoad()
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
	}

	err = page.WaitIdle(vo.waitTimeout - time.Since(s))
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
	}

	// 等待请求都有响应
	page.WaitRequestIdle(min(vo.waitTimeout-time.Since(s), 500*time.Millisecond), nil, []string{
		``, // 排除广告部分
	}, nil)()

	// 把差异调整为0.2，放大，不然会一直等待
	err = page.WaitDOMStable(min(vo.waitTimeout-time.Since(s), time.Second*2), 0.2)
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
	}

	//err = page.WaitStable(vo.waitTimeout - time.Since(s))
	//if err != nil {
	//	if !errors.Is(err, context.DeadlineExceeded) {
	//		return err
	//	}
	//}
	return page, nil
}

func (b *Browser) run(u string, onPageLoad func(page *rod.Page) error, vo *VisitOptions) (err error) {
	defer func() {
		if val := recover(); val != nil {
			if val != nil {
				debug.PrintStack()
			}
			switch val.(type) {
			case string:
				err = errors.New(val.(string))
			case error:
				err = val.(error)
			default:
				err = errors.New(fmt.Sprintf("%v", val.(string)))
			}

		}
	}()

	if vo == nil {
		vo = NewVisitOptions()
	}

	page, e := b.waitPageReady(u, vo)
	if e != nil {
		err = e
		return
	}
	defer page.Close()

	return onPageLoad(page)
}

func (b *Browser) Run(url string, onPage func(page *rod.Page) error, vo *VisitOptions) error {
	return b.run(url, onPage, vo)
}

// HTML 获取渲染后的页面
func (b *Browser) HTML(url string, vo *VisitOptions) (string, error) {
	var htmlReturn string
	err := b.run(url, func(page *rod.Page) error {
		var err error
		htmlReturn, err = page.HTML()
		return err
	}, vo)
	return htmlReturn, err
}

// RawHTML 获取原始HTML，不是渲染后的页面
func (b *Browser) RawHTML(url string, vo *VisitOptions) (string, error) {
	var htmlReturn string
	WithBeforeRequest(func(page *rod.Page) error {
		go page.EachEvent(func(e *proto.NetworkLoadingFinished) {
			reply, err := (proto.NetworkGetResponseBody{RequestID: e.RequestID}).Call(page)
			if err == nil && htmlReturn == "" {
				htmlReturn = reply.Body
			}
		})()
		return nil
	})(vo)
	err := b.run(url,
		func(page *rod.Page) error {
			if htmlReturn == "" {
				htmlReturn = page.MustHTML()
			}
			return nil
		},
		vo,
	)
	return htmlReturn, err
}

type browserOptions struct {
	Debug            bool
	Proxy            string
	IgnoreCertErrors bool
	ChromePath       string // 设定后可以复用浏览器cookie
	UserModeBrowser  bool   // 是否使用用户浏览器
}

type BrowserOption func(*browserOptions)

func WithDebug(debug bool) BrowserOption {
	return func(o *browserOptions) {
		o.Debug = debug
	}
}
func WithProxy(proxy string) BrowserOption {
	return func(o *browserOptions) {
		o.Proxy = proxy
	}
}
func WithIgnoreCertErrors(ignoreCertErrors bool) BrowserOption {
	return func(o *browserOptions) {
		o.IgnoreCertErrors = ignoreCertErrors
	}
}
func WithChromePath(chromePath string) BrowserOption {
	return func(o *browserOptions) {
		o.ChromePath = chromePath
	}
}

func WithUserModeBrowser(userModeBrowser bool) BrowserOption {
	return func(o *browserOptions) {
		o.UserModeBrowser = userModeBrowser
	}
}

// NewBrowser 初始化浏览器
func NewBrowser(opts ...BrowserOption) (*Browser, error) {
	// 参数配置
	bo := &browserOptions{}
	for _, o := range opts {
		o(bo)
	}

	browser := rod.New()
	var l *launcher.Launcher
	if bo.UserModeBrowser {
		l = launcher.NewUserMode()
	} else {
		l = launcher.New()
	}

	if bo.Debug {
		l = l.Headless(false).Devtools(true)
	}
	if len(bo.Proxy) > 0 {
		l = l.Proxy(bo.Proxy)
	}
	if len(bo.ChromePath) > 0 {
		l = l.Bin(bo.ChromePath)
	}

	browser = browser.ControlURL(l.MustLaunch())
	if bo.Debug {
		browser = browser.Trace(true)
	}

	if err := browser.Connect(); err != nil {
		return nil, err
	}

	if bo.IgnoreCertErrors {
		err := browser.IgnoreCertErrors(true)
		if err != nil {
			return nil, err
		}
	}

	return &Browser{Browser: browser}, nil
}

// DefaultBrowser 默认浏览器
func DefaultBrowser() *Browser {
	if defaultBrowser == nil {
		var err error
		defaultBrowser, err = NewBrowser()
		if err != nil {
			// 这里错误就直接奔溃退出
			panic(err)
		}
	}
	return defaultBrowser
}
