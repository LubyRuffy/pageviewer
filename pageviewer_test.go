package pageviewer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetDefaultBrowserForTest(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		if defaultBrowser != nil {
			_ = defaultBrowser.Close()
		}
		defaultBrowser = nil
		once = sync.Once{}
	})
}

func TestGetBrowser(t *testing.T) {
	tests := []struct {
		name             string
		debug            bool
		proxy            string
		ignoreCertErrors bool
	}{
		{
			name:             "default settings",
			debug:            false,
			proxy:            "",
			ignoreCertErrors: false,
		},
		{
			name:             "with debug",
			debug:            true,
			proxy:            "",
			ignoreCertErrors: false,
		},
		{
			name:             "with proxy",
			debug:            false,
			proxy:            "http://localhost:8080",
			ignoreCertErrors: false,
		},
		{
			name:             "with ignore cert errors",
			debug:            false,
			proxy:            "",
			ignoreCertErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			browser, err := NewBrowser(WithDebug(tt.debug), WithProxy(tt.proxy), WithIgnoreCertErrors(tt.ignoreCertErrors))
			assert.NoError(t, err)
			if browser != nil {
				defer browser.Close()
			}
			if browser == nil {
				t.Error("GetBrowser() returned nil")
			}
		})
	}
}

func TestGetPage(t *testing.T) {
	browser, err := NewBrowser()
	assert.NoError(t, err)
	if browser != nil {
		defer browser.Close()
	}

	page, err := browser.GetPage()
	assert.NoError(t, err)
	if page != nil {
		defer page.Close()
	}
	if page == nil {
		t.Error("GetPage() returned nil")
	}
}

func TestVisit(t *testing.T) {
	resetDefaultBrowserForTest(t)

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="app">ok</div></body></html>`))
	}))
	defer s.Close()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid url",
			url:     s.URL,
			wantErr: false,
		},
		{
			name:    "invalid url",
			url:     "invalid://url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Visit(tt.url, func(page *rod.Page) error {
				// 测试回调函数
				return nil
			})
			if (err != nil) != tt.wantErr {
				t.Errorf("Visit() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVisitWithOptions(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="app">custom browser</div></body></html>`))
	}))
	defer s.Close()

	b, err := NewBrowser()
	assert.NoError(t, err)
	if b != nil {
		defer b.Close()
	}

	var html string
	err = Visit(s.URL, func(page *rod.Page) error {
		html = page.MustHTML()
		return nil
	}, WithBrowser(b), WithWaitTimeout(time.Second*20))
	assert.NoError(t, err)
	assert.Contains(t, html, "custom browser")
}

func TestVisitDoesNotRepairSuccessfulCompatibilityCall(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="app">ok</div></body></html>`))
	}))
	defer s.Close()

	browser, err := NewBrowser()
	require.NoError(t, err)
	defer browser.Close()

	oldWorkerFactory := newClientWorker
	var created int32
	newClientWorker = func(ctx context.Context, browser *Browser, id int) (*worker, error) {
		if atomic.AddInt32(&created, 1) == 1 {
			return oldWorkerFactory(ctx, browser, id)
		}

		<-ctx.Done()
		return nil, ctx.Err()
	}
	t.Cleanup(func() {
		newClientWorker = oldWorkerFactory
	})

	err = Visit(s.URL, func(page *rod.Page) error {
		return nil
	}, WithBrowser(browser), WithAcquireTimeout(20*time.Millisecond))
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&created))
}

func TestVisitInitialWorkerCreationUsesProvisionTimeout(t *testing.T) {
	oldWorkerFactory := newClientWorker
	newClientWorker = func(ctx context.Context, browser *Browser, id int) (*worker, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	t.Cleanup(func() {
		newClientWorker = oldWorkerFactory
	})
	replaceWorkerProvisionTimeout(t, 20*time.Millisecond)

	done := make(chan error, 1)
	go func() {
		done <- Visit("https://example.com", func(page *rod.Page) error {
			return nil
		}, WithBrowser(&Browser{}))
	}()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Visit did not stop waiting for the initial worker after provisioning timeout")
	}
}
