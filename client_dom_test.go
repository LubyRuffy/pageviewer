package pageviewer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientHTMLUsesSharedBrowser(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="app">ok</div></body></html>`))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	html, err := client.HTML(context.Background(), s.URL)
	require.NoError(t, err)
	assert.Contains(t, html, `id="app"`)
}

func TestVisitWithBrowserKeepsBrowserReusable(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="app">ok</div></body></html>`))
	}))
	defer s.Close()

	browser, err := NewBrowser()
	require.NoError(t, err)
	defer browser.Close()

	err = Visit(s.URL, func(page *rod.Page) error {
		_, err := page.HTML()
		return err
	}, WithBrowser(browser))
	require.NoError(t, err)

	page, err := browser.GetPage()
	require.NoError(t, err)
	require.NoError(t, page.Close())

	err = Visit(s.URL, func(page *rod.Page) error {
		_, err := page.HTML()
		return err
	}, WithBrowser(browser))
	require.NoError(t, err)
}
