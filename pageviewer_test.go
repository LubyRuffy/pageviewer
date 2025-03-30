package pageviewer

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

	"github.com/go-rod/rod"
)

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
			if browser == nil {
				t.Error("GetBrowser() returned nil")
			}
		})
	}
}

func TestGetPage(t *testing.T) {
	tests := []struct {
		name    string
		browser *Browser
	}{
		{
			name:    "nil browser",
			browser: nil,
		},
		{
			name:    "existing browser",
			browser: DefaultBrowser(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := tt.browser.GetPage()
			assert.NoError(t, err)
			if page == nil {
				t.Error("GetPage() returned nil")
			}
		})
	}
}

func TestVisit(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid url",
			url:     "https://example.com",
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
	b, err := NewBrowser(WithDebug(true),
		WithChromePath("/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"),
		WithUserModeBrowser(true),
		WithIgnoreCertErrors(true),
	)
	assert.NoError(t, err)

	err = Visit("https://fofa.info/", func(page *rod.Page) error {
		t.Log(page.MustHTML())
		return nil
	}, WithBrowser(b), WithWaitTimeout(time.Second*20))
	assert.NoError(t, err)
}
