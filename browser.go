package pageviewer

import (
	"context"
	"errors"
	"fmt"
	"github.com/LubyRuffy/pageviewer/js"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

var (
	defaultBrowser *Browser
	once           sync.Once
)

type PageOptions struct {
	waitTimeout        time.Duration              // 等待超时的设置
	beforeRequest      func(page *rod.Page) error // 在请求之前的回调，做一些
	removeInvisibleDiv bool                       // 是否移除不可见的div
}

type Browser struct {
	UseUserMode bool
	*rod.Browser
}

func (b *Browser) GetPage() (*rod.Page, error) {
	// 使用用户的浏览器说明不需要模拟设备
	if b.UseUserMode {
		return b.Browser.Page(proto.TargetCreateTarget{})
	}
	// 支持浏览器识别绕过
	return stealth.Page(b.Browser)
}

func (b *Browser) Close() error {
	return b.Browser.Close()
}

func (b *Browser) WaitPage(page *rod.Page, po *PageOptions) error {
	s := time.Now()
	err := page.Timeout(min(po.waitTimeout, time.Second*15)).WaitLoad()
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}

	err = page.WaitIdle(min(po.waitTimeout-time.Since(s), time.Second*5))
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
	if po.waitTimeout-time.Since(s) > 0 {
		err = page.WaitDOMStable(min(po.waitTimeout-time.Since(s), time.Second*2), 0.2)
		if err != nil {
			if !errors.Is(err, context.DeadlineExceeded) {
				return err
			}
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

	// 添加网络响应监听器
	isHtml := false
	var lastErr error
	//var mimeType sync.Map
	go page.EachEvent(func(e *proto.NetworkResponseReceived) {
		//mimeType.LoadOrStore(e.Response.MIMEType, struct{}{})
		if strings.Contains(e.Response.MIMEType, "text/") || strings.Contains(e.Response.MIMEType, "application/json") {
			isHtml = true
		} else {
			if !isHtml {
				//mimeType.Range(func(key, value any) bool {
				//	log.Println(key)
				//	return true
				//})
				//log.Println(errors.New("no html content:" + u))
				lastErr = errors.New("no html content:" + u + ". The url's MIMEType is:" + e.Response.MIMEType)
				page.Close()
			}
		}
	})()

	err = page.Navigate(u)
	if err != nil {
		return nil, err
	}

	err = b.WaitPage(page, po)
	if err != nil {
		if lastErr != nil {
			return nil, lastErr
		}
		return page, err
	}
	return page, nil
}

func removeInvisibleElements(page *rod.Page) error {
	_, err := page.Eval(`
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
				  if (/^(style|class|width|height|align|valign|bgcolor|border|color|font|margin|padding|data\-|aria\-|id|name|run|role|on|target|cell|page|title|rel|src|link)/i.test(attr.name)) {
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
	return err
}

func (b *Browser) run(u string, onPageLoad func(page *rod.Page) error, po *PageOptions) (err error) {
	defer func() {
		if val := recover(); val != nil {
			if val != nil {
				debug.PrintStack()
			}
			switch val := val.(type) {
			case string:
				err = errors.New(val)
			case error:
				err = val
			default:
				err = fmt.Errorf("%v", val)
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
		err = removeInvisibleElements(page)
		if err != nil {
			return err
		}
	}

	return onPageLoad(page)
}

// Run 执行页面操作
// onPageLoad 页面加载完毕后执行的函数
func (b *Browser) Run(url string, onPage func(page *rod.Page) error, po *PageOptions) error {
	return b.run(url, onPage, po)
}

type ReadbilityArticle struct {
	Title         string `json:"title"`
	Byline        string `json:"byline"`
	Dir           string `json:"dir"`
	Lang          string `json:"lang"`
	Content       string `json:"content"`
	TextContent   string `json:"textContent"`
	Length        int    `json:"length"`
	Excerpt       string `json:"excerpt"`
	SiteName      string `json:"siteName"`
	PublishedTime string `json:"publishedTime"`
}

type ReadabilityArticleWithMarkdown struct {
	ReadbilityArticle
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	RawHTML  string `json:"raw_html"`
}

// ReadabilityArticle 获取渲染后页面的主体
func (b *Browser) ReadabilityArticle(url string) (ReadabilityArticleWithMarkdown, error) {
	var articleMarkdown ReadabilityArticleWithMarkdown

	po := NewVisitOptions(WithBeforeRequest(func(page *rod.Page) error {
		go page.EachEvent(func(e *proto.NetworkLoadingFinished) {
			reply, err := (proto.NetworkGetResponseBody{RequestID: e.RequestID}).Call(page)
			if err == nil && articleMarkdown.RawHTML == "" {
				// 获取原始HTML
				articleMarkdown.RawHTML = reply.Body
			}
		})()
		return nil
	})).PageOptions

	err := b.run(url, func(page *rod.Page) error {
		var err error

		// 获取渲染后的HTML
		articleMarkdown.HTML, err = page.HTML()
		if err != nil {
			return err
		}

		// 执行 readability
		jsContent := "() => {\r\n" + strings.Join([]string{
			js.Readability,
			js.Shadowdom,
			//js.Turndown,
			`
            const documentClone = deepCloneDocumentWithShadowDOM(
                document,
                {
                  // excludeClasses: [translationTargetClass, translationTargetDividerClass, translationTargetInnerClass],
                  excludeTags: ['aisidebar-container', 'script', 'style', 'link', 'meta', 'svg', 'canvas', 'iframe', 'object', 'embed'],
                },
            )
            const article = new Readability(documentClone, {charThreshold: MIN_CONTENT_LENGTH}).parse();
			
			return article;
		`}, "\r\n//===\r\n") + "\r\n}"
		r, err := page.Eval(jsContent)
		if err != nil {
			return err
		}
		if err = r.Value.Unmarshal(&articleMarkdown); err != nil {
			return err
		}

		// 生成markdown
		articleMarkdown.Markdown, err = htmltomarkdown.ConvertString(articleMarkdown.Content)
		if err != nil {
			return err
		}

		return nil
	}, po)
	return articleMarkdown, err
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

// Links 获取所有链接
func (b *Browser) Links(url string, po *PageOptions) (string, error) {
	var htmlReturn string
	err := b.run(url, func(page *rod.Page) error {
		var err error
		elements, err := page.Elements("a")
		if err != nil {
			return err
		}

		var links []string
		for _, el := range elements {
			t, err := el.Text()
			if err != nil {
				continue
			}
			if t == "" {
				continue
			}

			href, err := el.Property("href")
			if err != nil {
				continue
			}
			if !href.Nil() && href.String() != "" {
				link := href.String()
				// 不是javascript开头
				if !strings.HasPrefix(link, "javascript:") {
					links = append(links, fmt.Sprintf(`<a href="%s">`+t+`</a>`, href.String()))
				}
			}
		}

		htmlReturn = strings.Join(links, "\n")
		return err
	}, po)
	return htmlReturn, err
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

	l = l.Leakless(false).
		Delete("enable-automation").
		Delete("disable-blink-features").
		Delete("disable-blink-features=AutomationControlled")

	if bo.Debug {
		l = l.Headless(false).Devtools(true)
	} else {
		if bo.NoHeadless {
			l = l.Headless(false)
		}
		if bo.DevTools {
			l = l.Devtools(true)
		}
	}

	if len(bo.Proxy) > 0 {
		l = l.Proxy(bo.Proxy)
	}
	if len(bo.ChromePath) > 0 {
		l = l.Bin(bo.ChromePath)
	}
	if bo.RemoteDebuggingPort > 0 {
		l = l.RemoteDebuggingPort(bo.RemoteDebuggingPort)
	}
	if len(bo.UserDataDir) > 0 {
		l = l.UserDataDir(bo.UserDataDir)
	}
	l = l.NoSandbox(true)
	browser = browser.ControlURL(l.MustLaunch())
	if bo.Debug {
		browser = browser.Trace(true)
	}

	// 使用用户的浏览器说明不需要模拟设备
	if bo.UserModeBrowser {
		browser = browser.NoDefaultDevice()
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

	return &Browser{
		Browser:     browser,
		UseUserMode: bo.UserModeBrowser,
	}, nil
}

// DefaultBrowser 默认浏览器
func DefaultBrowser() *Browser {
	once.Do(func() {
		if defaultBrowser == nil {
			var err error
			defaultBrowser, err = NewBrowser()
			if err != nil {
				// 这里错误就直接奔溃退出
				panic(err)
			}
		}
	})
	return defaultBrowser
}
