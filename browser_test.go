package pageviewer

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err)
	html1, err := browser.RawHTML(s.URL, NewVisitOptions().PageOptions)
	assert.NoError(t, err)
	html2, err := browser.HTML(s.URL, NewVisitOptions().PageOptions)
	assert.NoError(t, err)
	assert.NotEqual(t, html1, html2)
	assert.Contains(t, html2, `<div id="app">动态修改的内容</div>`)

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
	assert.NoError(t, err)

	vo := NewVisitOptions(WithWaitTimeout(time.Second * 20)).PageOptions

	var html string
	err = b.Run(s.URL, func(page *rod.Page) error {
		html = page.MustHTML()
		return nil
	}, vo)
	assert.Error(t, err)
	b.Close()

	b, err = NewBrowser(WithDebug(true),
		WithIgnoreCertErrors(true),
	)
	assert.NoError(t, err)
	vo = NewVisitOptions(WithWaitTimeout(time.Second * 20)).PageOptions

	err = b.Run(s.URL, func(page *rod.Page) error {
		html = page.MustHTML()
		return nil
	}, vo)
	assert.NoError(t, err)
	assert.NotEmpty(t, html)
	assert.Contains(t, html, "helloworld")
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
	assert.NoError(t, err)
	defer b.Close()

	err = b.Run(s.URL, func(page *rod.Page) error {
		html = page.MustHTML()
		return nil
	}, vo)
	assert.NoError(t, err)
	assert.NotEmpty(t, html)
}

func TestBrowser_HTML(t *testing.T) {

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 返回test/aaa.pdf
		http.ServeFile(w, r, "./test/aaa.pdf")
	}))
	defer s.Close()

	browser, err := NewBrowser(WithIgnoreCertErrors(true), WithDebug(true))
	assert.NoError(t, err)
	_, err = browser.HTML(s.URL, NewVisitOptions().PageOptions)
	assert.Error(t, err)

}
