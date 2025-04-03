package pageviewer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
)

func TestBrowser_RawHTML(t *testing.T) {

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`
        <html>
            <head>
                <script>
                    // 动态修改 div#app 的内容
                    document.addEventListener("DOMContentLoaded", function() {
                        document.getElementById("app").innerHTML = "动态修改的内容";
                    });
                </script>
            </head>
            <body>
                <div id="app"></div>
            </body>
        </html>
    `))
	}))
	defer s.Close()

	browser, err := NewBrowser(WithIgnoreCertErrors(true))
	if err != nil {
		t.Fatalf("创建浏览器失败: %v", err)
	}

	html1, err := browser.RawHTML(s.URL, NewVisitOptions().PageOptions)
	if err != nil {
		t.Fatalf("获取原始HTML失败: %v", err)
	}

	html2, err := browser.HTML(s.URL, NewVisitOptions().PageOptions)
	if err != nil {
		t.Fatalf("获取渲染后HTML失败: %v", err)
	}

	if html1 == html2 {
		t.Errorf("原始HTML和渲染后HTML不应该相同")
	}

	if !strings.Contains(html2, `<div id="app">动态修改的内容</div>`) {
		t.Errorf("渲染后HTML应包含动态修改的内容")
	}
}

func TestBrowser_WithIgnoreCertErrors(t *testing.T) {
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`helloworld`))
	}))
	s.StartTLS()
	defer s.Close()

	// 复用用户浏览器
	b, err := NewBrowser(WithDebug(true))
	if err != nil {
		t.Fatalf("创建浏览器失败: %v", err)
	}

	vo := NewVisitOptions(WithWaitTimeout(time.Second * 20)).PageOptions

	var html string
	err = b.Run(s.URL, func(page *rod.Page) error {
		html = page.MustHTML()
		return nil
	}, vo)
	if err == nil {
		t.Error("不忽略证书错误时应该返回错误")
	}
	b.Close()

	b, err = NewBrowser(WithDebug(true),
		WithIgnoreCertErrors(true),
	)
	if err != nil {
		t.Fatalf("创建浏览器失败: %v", err)
	}
	defer b.Close()

	vo = NewVisitOptions(WithWaitTimeout(time.Second * 20)).PageOptions

	err = b.Run(s.URL, func(page *rod.Page) error {
		html = page.MustHTML()
		return nil
	}, vo)
	if err != nil {
		t.Fatalf("忽略证书错误后仍然失败: %v", err)
	}

	if len(html) == 0 {
		t.Error("HTML内容不应为空")
	}

	if !strings.Contains(html, "helloworld") {
		t.Error("HTML内容应包含helloworld")
	}
}

func TestBrowser_RemoveInvisibleElements(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html>
<script>console.log('test')</script>
<style>body{}</style>
<div style="display: none;">隐藏的div</div>
<div style="visibility: hidden;">不可见的div</div>
<div style="opacity: 0;">透明的div</div>
<div style="width: 0; height: 0;">零尺寸div</div>
<div>可见的div</div>
</html>`))
	}))
	var html string
	vo := NewVisitOptions(WithWaitTimeout(time.Second*20), WithRemoveInvisibleDiv(true)).PageOptions
	b, err := NewBrowser(WithDebug(true))
	if err != nil {
		t.Fatalf("创建浏览器失败: %v", err)
	}
	defer b.Close()

	err = b.Run(s.URL, func(page *rod.Page) error {
		html = page.MustHTML()
		return nil
	}, vo)
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}

	if len(html) == 0 {
		t.Error("HTML内容不应为空")
	}
}

// TestConnectToLocalManager 测试连接到本地Manager的功能
// 注意：此测试需要在本地7317端口运行Rod Manager
// 可以使用以下Docker命令启动：
// docker run --rm -p 7317:7317 ghcr.io/go-rod/rod
func TestConnectToLocalManager(t *testing.T) {
	// 跳过自动测试，因为需要外部依赖

	// 连接到本地Manager
	browser, err := NewBrowser(
		WithManagerURL("ws://127.0.0.1:7371"),
		WithDebug(true), // 开启调试以便查看详情
	)
	if err != nil {
		t.Fatalf("连接到本地Manager失败: %v", err)
	}
	defer browser.Close()

	// 测试基本功能
	po := &PageOptions{
		waitTimeout: time.Second * 10,
	}

	// 访问示例网站
	html, err := browser.HTML("https://example.com", po)
	if err != nil {
		t.Fatalf("访问网站失败: %v", err)
	}

	// 简单验证返回内容
	if len(html) == 0 {
		t.Errorf("返回的HTML内容为空")
	}
	if !strings.Contains(html, "<title>Example Domain</title>") {
		t.Errorf("HTML内容不符合预期")
	}

	t.Log("成功连接到本地Manager并获取页面内容")
}

// TestConnectToExistingBrowser 测试直接连接到已运行的浏览器
func TestConnectToExistingBrowser(t *testing.T) {
	// 跳过自动测试，因为需要已运行的浏览器实例
	t.Skip("需要已运行的浏览器实例")

	// 替换为实际的WebSocket URL
	wsURL := "ws://127.0.0.1:9222/devtools/browser/123456"

	browser, err := NewBrowser(
		WithControlURL(wsURL),
		WithDebug(true),
	)
	if err != nil {
		t.Fatalf("连接到已运行浏览器失败: %v", err)
	}
	defer browser.Close()

	// 测试基本功能
	po := &PageOptions{
		waitTimeout: time.Second * 10,
	}

	// 访问示例网站
	html, err := browser.HTML("https://example.com", po)
	if err != nil {
		t.Fatalf("访问网站失败: %v", err)
	}

	// 简单验证返回内容
	if len(html) == 0 {
		t.Errorf("返回的HTML内容为空")
	}

	t.Log("成功连接到已运行浏览器并获取页面内容")
}
