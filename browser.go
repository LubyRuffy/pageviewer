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
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

var (
	defaultBrowser                   *Browser
	once                             sync.Once
	newBrowserWithOptions            = NewBrowser
	browserCloseWaitTimeout          = 2 * time.Second
	browserKillWaitTimeout           = 3 * time.Second
	browserProcessPollInterval       = 50 * time.Millisecond
	browserWaitLoadTimeoutCap        = 15 * time.Second
	browserWaitIdleTimeoutCap        = 5 * time.Second
	browserWaitRequestIdleTimeoutCap = 500 * time.Millisecond
	browserWaitDOMStableTimeoutCap   = 2 * time.Second
)

type PageOptions struct {
	waitTimeout        time.Duration              // 等待超时的设置
	beforeRequest      func(page *rod.Page) error // 在请求之前的回调，做一些
	removeInvisibleDiv bool                       // 是否移除不可见的div
	blockSubresources  bool                       // 是否阻断主文档之外的请求
}

type Browser struct {
	UseUserMode bool
	*rod.Browser
	closeFn         func() error
	launcher        *launcher.Launcher
	leaklessEnabled bool
	closeOnce       sync.Once
	closeErr        error
}

type documentResponseResult struct {
	response *proto.NetworkResponseReceived
	body     string
	err      error
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
	if b == nil {
		return nil
	}

	b.closeOnce.Do(func() {
		if b.closeFn != nil {
			b.closeErr = b.closeFn()
			return
		}

		var closeErr error
		if b.Browser != nil {
			closeErr = b.Browser.Close()
		}

		b.closeErr = errors.Join(closeErr, cleanupManagedLauncherProcess(b.launcher))
	})

	return b.closeErr
}

func cleanupManagedLauncherProcess(l *launcher.Launcher) error {
	if l == nil || l.PID() == 0 {
		return nil
	}

	exited, err := waitForProcessTreeExit(l.PID(), browserCloseWaitTimeout)
	if err != nil {
		return err
	}
	if exited {
		return nil
	}

	l.Kill()

	exited, err = waitForProcessTreeExit(l.PID(), browserKillWaitTimeout)
	if err != nil {
		return err
	}
	if exited {
		return nil
	}

	return fmt.Errorf("browser process tree %d did not exit after kill", l.PID())
}

func forceCleanupManagedLauncherProcess(l *launcher.Launcher) error {
	if l == nil || l.PID() == 0 {
		return nil
	}

	l.Kill()

	exited, err := waitForProcessTreeExit(l.PID(), browserKillWaitTimeout)
	if err != nil {
		return err
	}
	if exited {
		return nil
	}

	return fmt.Errorf("browser process tree %d did not exit after forced cleanup", l.PID())
}

func waitForProcessTreeExit(pid int, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		exists, err := processTreeExists(pid)
		if err != nil {
			return false, err
		}
		if !exists {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(browserProcessPollInterval)
	}
}

func (b *Browser) WaitPage(page *rod.Page, po *PageOptions) error {
	s := time.Now()
	err := page.Timeout(min(po.waitTimeout, browserWaitLoadTimeoutCap)).WaitLoad()
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}

	err = page.WaitIdle(min(po.waitTimeout-time.Since(s), browserWaitIdleTimeoutCap))
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}

	// 等待请求都有响应
	page.WaitRequestIdle(min(po.waitTimeout-time.Since(s), browserWaitRequestIdleTimeoutCap), nil, []string{
		``, // 排除广告部分
	}, nil)()

	// 把差异调整为0.2，放大，不然会一直等待
	if po.waitTimeout-time.Since(s) > 0 {
		err = page.WaitDOMStable(min(po.waitTimeout-time.Since(s), browserWaitDOMStableTimeoutCap), 0.2)
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

	if _, err := b.navigatePage(context.Background(), page, u, po); err != nil {
		_ = page.Close()
		return nil, err
	}
	return page, nil
}

func newUnsupportedDOMContentError(u, mimeType string) error {
	return fmt.Errorf("%w: no html content:%s. The url's MIMEType is:%s", ErrUnsupportedContentType, u, mimeType)
}

func waitForMainDocumentResponse(ctx context.Context, page *rod.Page, includeBody bool) (func() (documentResponseResult, error), func()) {
	if ctx == nil {
		ctx = context.Background()
	}

	waitPage, cancel := page.WithCancel()
	resultCh := make(chan documentResponseResult, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)

		var response *proto.NetworkResponseReceived
		sent := false
		wait := waitPage.EachEvent(
			func(e *proto.NetworkResponseReceived) {
				if e.Type == proto.NetworkResourceTypeDocument && response == nil {
					response = e
				}
			},
			func(e *proto.NetworkLoadingFinished) bool {
				if response == nil || e.RequestID != response.RequestID {
					return false
				}

				result := documentResponseResult{response: response}
				if includeBody {
					result.body, result.err = readResponseBody(waitPage, response)
				}
				resultCh <- result
				sent = true
				return true
			},
		)
		wait()

		if !sent {
			err := waitPage.GetContext().Err()
			if err == nil {
				err = ErrNavigationFailed
			}
			resultCh <- documentResponseResult{err: err}
		}
	}()

	return func() (documentResponseResult, error) {
			select {
			case <-ctx.Done():
				cancel()
				<-done
				return documentResponseResult{}, ctx.Err()
			case result := <-resultCh:
				return result, result.err
			}
		}, func() {
			cancel()
			<-done
		}
}

func (b *Browser) navigatePage(ctx context.Context, page *rod.Page, u string, po *PageOptions) (*proto.NetworkResponseReceived, error) {
	if po.beforeRequest != nil {
		if err := po.beforeRequest(page); err != nil {
			return nil, err
		}
	}

	waitDocument, stopWaiting := waitForMainDocumentResponse(ctx, page, false)
	defer stopWaiting()

	if err := page.Navigate(u); err != nil {
		return nil, err
	}

	result, err := waitDocument()
	if err != nil {
		return nil, err
	}
	response := result.response
	if response == nil {
		return nil, ErrNavigationFailed
	}
	if !isTextContentType(response.Response.MIMEType) {
		return response, newUnsupportedDOMContentError(u, response.Response.MIMEType)
	}

	if err := b.WaitPage(page, po); err != nil {
		return response, err
	}

	return response, nil
}

func (b *Browser) navigateTextPage(ctx context.Context, page *rod.Page, u string, po *PageOptions) (documentResponseResult, error) {
	if po.beforeRequest != nil {
		if err := po.beforeRequest(page); err != nil {
			return documentResponseResult{}, err
		}
	}

	stopBlocker, err := blockSubresources(page, po)
	if err != nil {
		return documentResponseResult{}, err
	}
	defer stopBlocker()

	waitDocument, stopWaiting := waitForMainDocumentResponse(ctx, page, true)
	defer stopWaiting()

	if err := page.Navigate(u); err != nil {
		return documentResponseResult{}, err
	}

	result, err := waitDocument()
	if err != nil {
		return documentResponseResult{}, err
	}
	if result.response == nil {
		return documentResponseResult{}, ErrNavigationFailed
	}
	if !isTextContentType(result.response.Response.MIMEType) {
		return result, ErrUnsupportedContentType
	}
	if err := b.WaitPage(page, po); err != nil {
		return documentResponseResult{}, err
	}

	return result, nil
}

func blockSubresources(page *rod.Page, po *PageOptions) (func(), error) {
	if po == nil || !po.blockSubresources {
		return func() {}, nil
	}

	router := page.HijackRequests()
	if err := router.Add("*", "", func(h *rod.Hijack) {
		if h.Request.Type() == proto.NetworkResourceTypeDocument {
			h.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}
		h.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
	}); err != nil {
		return nil, err
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		router.Run()
	}()

	return func() {
		_ = router.Stop()
		<-done
	}, nil
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

func (b *Browser) runPage(ctx context.Context, page *rod.Page, u string, po *PageOptions, onPageLoad func(page *rod.Page, response *proto.NetworkResponseReceived) error) (response *proto.NetworkResponseReceived, pageBroken bool, err error) {
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
		po = newDefaultVisitOptions().PageOptions
	}

	response, e := b.navigatePage(ctx, page, u, po)
	if e != nil {
		return response, true, e
	}

	if po.removeInvisibleDiv {
		// 执行 JavaScript 检测并删除不可见的 div
		err = removeInvisibleElements(page)
		if err != nil {
			return response, false, err
		}
	}

	return response, false, onPageLoad(page, response)
}

func (b *Browser) runWithResponse(u string, po *PageOptions, onPageLoad func(page *rod.Page, response *proto.NetworkResponseReceived) error) (err error) {
	page, err := b.GetPage()
	if err != nil {
		return err
	}
	defer page.Close()

	_, _, err = b.runPage(context.Background(), page, u, po, onPageLoad)
	return err
}

func (b *Browser) run(u string, onPageLoad func(page *rod.Page) error, po *PageOptions) error {
	return b.runWithResponse(u, po, func(page *rod.Page, _ *proto.NetworkResponseReceived) error {
		return onPageLoad(page)
	})
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

	err := b.runWithResponse(url, newDefaultVisitOptions().PageOptions, func(page *rod.Page, response *proto.NetworkResponseReceived) error {
		if rawHTML, err := readResponseBody(page, response); err == nil {
			articleMarkdown.RawHTML = rawHTML
		}
		return fillReadabilityArticle(page, url, &articleMarkdown)
	})
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
		htmlReturn, err = collectLinks(page)
		return err
	}, po)
	return htmlReturn, err
}

func readResponseBody(page *rod.Page, response *proto.NetworkResponseReceived) (string, error) {
	if response == nil {
		return "", nil
	}

	var lastErr error
	for range 10 {
		body, err := getResponseBody(page, response.RequestID)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "No resource with given identifier found") {
			return "", err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return "", lastErr
}

func getResponseBody(page *rod.Page, requestID proto.NetworkRequestID) (string, error) {
	reply, err := (proto.NetworkGetResponseBody{RequestID: requestID}).Call(page)
	if err != nil {
		return "", err
	}
	return reply.Body, nil
}

func fillReadabilityArticle(page *rod.Page, url string, article *ReadabilityArticleWithMarkdown) error {
	var err error

	article.HTML, err = page.HTML()
	if err != nil {
		return err
	}

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
	if err = r.Value.Unmarshal(article); err != nil {
		return err
	}

	article.Markdown, err = htmltomarkdown.ConvertString(article.Content,
		converter.WithDomain(url))
	return err
}

func collectLinks(page *rod.Page) (string, error) {
	elements, err := page.Elements("a")
	if err != nil {
		return "", err
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
			if !strings.HasPrefix(link, "javascript:") {
				links = append(links, fmt.Sprintf(`<a href="%s">`+t+`</a>`, href.String()))
			}
		}
	}

	return strings.Join(links, "\n"), nil
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

	leaklessEnabled := true
	if bo.LeaklessSet {
		leaklessEnabled = bo.Leakless
	}

	l = l.Leakless(leaklessEnabled).
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
	controlURL, err := l.Launch()
	if err != nil {
		return nil, err
	}
	browser = browser.ControlURL(controlURL)
	if bo.Debug {
		browser = browser.Trace(true)
	}

	// 使用用户的浏览器说明不需要模拟设备
	if bo.UserModeBrowser {
		browser = browser.NoDefaultDevice()
	}

	ownsUserDataDir := !bo.UserModeBrowser && bo.UserDataDir == ""
	cleanupLaunch := func() {
		l.Kill()
		if ownsUserDataDir {
			l.Cleanup()
		}
	}
	cleanupLaunchError := func(err error) error {
		cleanupErr := forceCleanupManagedLauncherProcess(l)
		if ownsUserDataDir {
			l.Cleanup()
		}
		return errors.Join(err, cleanupErr)
	}
	if err := browser.Connect(); err != nil {
		cleanupLaunch()
		return nil, cleanupLaunchError(err)
	}

	closeBrowser := func() error {
		var closeErr error
		if browser != nil {
			closeErr = browser.Close()
		}
		cleanupErr := cleanupManagedLauncherProcess(l)
		if ownsUserDataDir {
			l.Cleanup()
		}
		return errors.Join(closeErr, cleanupErr)
	}

	if bo.IgnoreCertErrors {
		err := browser.IgnoreCertErrors(true)
		if err != nil {
			return nil, cleanupLaunchError(err)
		}
	}

	return &Browser{
		Browser:         browser,
		UseUserMode:     bo.UserModeBrowser,
		closeFn:         closeBrowser,
		launcher:        l,
		leaklessEnabled: leaklessEnabled,
	}, nil
}

func newBrowserFromConfig(cfg Config) (*Browser, error) {
	return newBrowserWithOptions(cfg.browserOptions()...)
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
