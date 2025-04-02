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

type PageOptions struct {
	waitTimeout        time.Duration              // 等待超时的设置
	beforeRequest      func(page *rod.Page) error // 在请求之前的回调，做一些
	removeInvisibleDiv bool                       // 是否移除不可见的div
}

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

func (b *Browser) WaitPage(page *rod.Page, po *PageOptions) error {
	s := time.Now()
	err := page.WaitLoad()
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}

	err = page.WaitIdle(po.waitTimeout - time.Since(s))
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}

	// 等待请求都有响应
	page.WaitRequestIdle(min(po.waitTimeout-time.Since(s), 500*time.Millisecond), nil, []string{
		``, // 排除广告部分
	}, nil)()

	// 把差异调整为0.2，放大，不然会一直等待
	err = page.WaitDOMStable(min(po.waitTimeout-time.Since(s), time.Second*2), 0.2)
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}

	//err = page.WaitStable(vo.waitTimeout - time.Since(s))
	//if err != nil {
	//	if !errors.Is(err, context.DeadlineExceeded) {
	//		return err
	//	}
	//}
	return err
}

func (b *Browser) waitPageReady(u string, po *PageOptions) (*rod.Page, error) {
	page, err := b.GetPage()
	if err != nil {
		return nil, err
	}

	if po.beforeRequest != nil {
		err := po.beforeRequest(page)
		if err != nil {
			return nil, err
		}
	}

	err = page.Navigate(u)
	if err != nil {
		return nil, err
	}

	err = b.WaitPage(page, po)
	return page, err
}

func (b *Browser) run(u string, onPageLoad func(page *rod.Page) error, po *PageOptions) (err error) {
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

	if po == nil {
		po = NewVisitOptions().PageOptions
	}

	page, e := b.waitPageReady(u, po)
	if e != nil {
		err = e
		return
	}
	defer page.Close()

	if po.removeInvisibleDiv {
		// 执行 JavaScript 检测并删除不可见的 div
		_, err = page.Eval(`
		() => {
			// 删除当前 DOM 中的所有注释
			function removeComments() {
			  // 创建 TreeWalker，过滤出注释节点 (Node.COMMENT_NODE = 8)
			  const walker = document.createTreeWalker(
				document.body,           // 从 body 开始遍历
				NodeFilter.SHOW_COMMENT  // 只显示注释节点
			  );
			
			  // 收集所有注释节点
			  const comments = [];
			  let node;
			  while ((node = walker.nextNode())) {
				comments.push(node);
			  }
			
			  // 删除所有找到的注释节点
			  comments.forEach(comment => comment.remove());
			}
			removeComments();

			let toRemove = [];
			toRemove = toRemove.concat(Array.from(document.getElementsByTagName('script')));
			toRemove = toRemove.concat(Array.from(document.getElementsByTagName('style')));
			toRemove = toRemove.concat(Array.from(document.getElementsByTagName('meta')));
			toRemove = toRemove.concat(Array.from(document.getElementsByTagName('link')));
			toRemove = toRemove.concat(Array.from(document.querySelectorAll('input[type="hidden"]')));

			// 获取所有元素
			let elements = [];
			//elements = elements.concat(Array.from(document.getElementsByTagName('div')));
			//elements = elements.concat(Array.from(document.getElementsByTagName('iframe')));
			elements = elements.concat(Array.from(document.getElementsByTagName('*')));

			// 检查每个元素是否不可见
			for (let element of elements) {
				const style = window.getComputedStyle(element);
				const rect = element.getBoundingClientRect();
				
				if (style.display === 'none' || 
					style.visibility === 'hidden' || 
					style.opacity === '0' || 
					rect.width === 0 || 
					rect.height === 0) {
					toRemove.push(element);
				}

				// 删除内联 style 属性
				// 获取所有属性
				const attributes = Array.from(element.attributes);
				attributes.forEach(attr => {
				  // 匹配常见的样式相关属性名
				  if (/^(style|class|width|height|align|valign|bgcolor|border|color|font|margin|padding|data\-|aria\-|id|name|run|role|on|target|cell|page|title)/i.test(attr.name)) {
					element.removeAttribute(attr.name);
				  }
				});
			}

			// 删除不可见的 element
			const deleteLength = toRemove.length;
			toRemove.forEach(element => element.remove());
			
			// 返回删除的 element 数量
			return deleteLength;
		}
	`)
		if err != nil {
			return err
		}
	}

	return onPageLoad(page)
}

func (b *Browser) Run(url string, onPage func(page *rod.Page) error, po *PageOptions) error {
	return b.run(url, onPage, po)
}

// HTML 获取渲染后的页面
func (b *Browser) HTML(url string, po *PageOptions) (string, error) {
	var htmlReturn string
	err := b.run(url, func(page *rod.Page) error {
		var err error
		htmlReturn, err = page.HTML()
		return err
	}, po)
	return htmlReturn, err
}

// RawHTML 获取原始HTML，不是渲染后的页面
func (b *Browser) RawHTML(url string, po *PageOptions) (string, error) {
	var htmlReturn string
	po.beforeRequest = func(page *rod.Page) error {
		go page.EachEvent(func(e *proto.NetworkLoadingFinished) {
			reply, err := (proto.NetworkGetResponseBody{RequestID: e.RequestID}).Call(page)
			if err == nil && htmlReturn == "" {
				htmlReturn = reply.Body
			}
		})()
		return nil
	}

	err := b.run(url,
		func(page *rod.Page) error {
			if htmlReturn == "" {
				htmlReturn = page.MustHTML()
			}
			return nil
		},
		po,
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
