package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/LubyRuffy/pageviewer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlagsRequiresURLAndMode(t *testing.T) {
	_, err := parseFlags([]string{"--mode", "html"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--url is required")

	_, err = parseFlags([]string{"--url", "https://example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--mode is required")
}

func TestParseFlagsRejectsInvalidMode(t *testing.T) {
	_, err := parseFlags([]string{"--url", "https://example.com", "--mode", "pdf"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --mode")
}

func TestParseFlagsAllowsRepeatedModesWithJSON(t *testing.T) {
	opts, err := parseFlags([]string{
		"--url", "https://example.com",
		"--json",
		"--mode", "html",
		"--mode", "article",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"html", "article"}, opts.modes)
}

func TestParseFlagsRejectsMultipleModesWithoutJSON(t *testing.T) {
	_, err := parseFlags([]string{
		"--url", "https://example.com",
		"--mode", "html",
		"--mode", "article",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple --mode values require --json")
}

func TestParseFlagsRejectsDuplicateModes(t *testing.T) {
	_, err := parseFlags([]string{
		"--url", "https://example.com",
		"--json",
		"--mode", "html",
		"--mode", "html",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate --mode: html")
}

func TestParseFlagsParsesCommonOptions(t *testing.T) {
	opts, err := parseFlags([]string{
		"--url", "https://example.com",
		"--mode", "article",
		"--json",
		"--wait-timeout", "15s",
		"--trace-id", "trace-1",
		"--remove-invisible-div",
		"--acquire-timeout", "5s",
		"--proxy", "http://127.0.0.1:8080",
		"--no-headless",
		"--devtools",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", opts.url)
	assert.Equal(t, []string{"article"}, opts.modes)
	assert.True(t, opts.jsonOutput)
	assert.Equal(t, 15*time.Second, opts.waitTimeout)
	assert.Equal(t, "trace-1", opts.traceID)
	assert.True(t, opts.removeInvisibleDiv)
	assert.Equal(t, 5*time.Second, opts.acquireTimeout)
	assert.Equal(t, "http://127.0.0.1:8080", opts.proxy)
	assert.True(t, opts.noHeadless)
	assert.True(t, opts.devTools)
}

func TestBuildConfigMapsBrowserAndRequestOptions(t *testing.T) {
	opts := cliOptions{
		url:                "https://example.com",
		modes:              []string{"html"},
		waitTimeout:        12 * time.Second,
		traceID:            "trace-2",
		removeInvisibleDiv: true,
		acquireTimeout:     4 * time.Second,
		proxy:              "socks5://127.0.0.1:1080",
		noHeadless:         true,
		devTools:           true,
	}

	cfg, reqOpts := buildConfig(opts)
	assert.Equal(t, "socks5://127.0.0.1:1080", cfg.Proxy)
	assert.True(t, cfg.NoHeadless)
	assert.True(t, cfg.DevTools)
	assert.Len(t, reqOpts, 4)
}

func TestBuildConfigExpandsPoolForJSONMultiMode(t *testing.T) {
	opts := cliOptions{
		url:        "https://example.com",
		modes:      []string{"html", "links", "raw-text"},
		jsonOutput: true,
	}

	cfg, reqOpts := buildConfig(opts)
	assert.Equal(t, 3, cfg.PoolSize)
	assert.Equal(t, 3, cfg.Warmup)
	assert.Empty(t, reqOpts)
}

func TestRunCLIRendersModes(t *testing.T) {
	cases := []struct {
		name     string
		mode     string
		expected string
	}{
		{name: "html", mode: "html", expected: "<html><body>html</body></html>"},
		{name: "links", mode: "links", expected: "https://example.com"},
		{name: "article", mode: "article", expected: "# Article"},
		{name: "raw-text", mode: "raw-text", expected: "raw body"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := startClient
			startClient = func(ctx context.Context, cfg pageviewer.Config) (fetcher, error) {
				return &fakeFetcher{
					htmlFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error) {
						return "<html><body>html</body></html>", nil
					},
					linksFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error) {
						return "https://example.com", nil
					},
					articleFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.ReadabilityArticleWithMarkdown, error) {
						return pageviewer.ReadabilityArticleWithMarkdown{Markdown: "# Article"}, nil
					},
					rawTextFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.TextResponse, error) {
						return pageviewer.TextResponse{Body: "raw body"}, nil
					},
				}, nil
			}
			t.Cleanup(func() { startClient = original })

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			code := runCLI(context.Background(), []string{"--url", "https://example.com", "--mode", tc.mode}, &stdout, &stderr)
			require.Equal(t, 0, code)
			assert.Empty(t, stderr.String())
			assert.Equal(t, tc.expected, strings.TrimSpace(stdout.String()))
		})
	}
}

func TestRunCLIJSONIncludesModesAndResultsForSingleMode(t *testing.T) {
	original := startClient
	startClient = func(ctx context.Context, cfg pageviewer.Config) (fetcher, error) {
		return &fakeFetcher{
			rawTextFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.TextResponse, error) {
				return pageviewer.TextResponse{
					Body:        "raw body",
					ContentType: "text/plain",
					StatusCode:  200,
					FinalURL:    "https://example.com/final",
					Header:      http.Header{"Content-Type": []string{"text/plain"}},
				}, nil
			},
		}, nil
	}
	t.Cleanup(func() { startClient = original })

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runCLI(context.Background(), []string{"--url", "https://example.com", "--mode", "raw-text", "--json"}, &stdout, &stderr)
	require.Equal(t, 0, code)
	assert.Empty(t, stderr.String())

	var got struct {
		Modes   []string `json:"modes"`
		URL     string   `json:"url"`
		Results map[string]struct {
			Body        string              `json:"body"`
			ContentType string              `json:"content_type"`
			StatusCode  int                 `json:"status_code"`
			FinalURL    string              `json:"final_url"`
			Header      map[string][]string `json:"header"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Equal(t, []string{"raw-text"}, got.Modes)
	assert.Equal(t, "https://example.com", got.URL)
	assert.Equal(t, "raw body", got.Results["raw-text"].Body)
	assert.Equal(t, "text/plain", got.Results["raw-text"].ContentType)
	assert.Equal(t, 200, got.Results["raw-text"].StatusCode)
	assert.Equal(t, "https://example.com/final", got.Results["raw-text"].FinalURL)
	assert.Equal(t, []string{"text/plain"}, got.Results["raw-text"].Header["Content-Type"])
}

func TestRunCLIJSONSupportsMultipleModes(t *testing.T) {
	original := startClient
	startClient = func(ctx context.Context, cfg pageviewer.Config) (fetcher, error) {
		return &fakeFetcher{
			htmlFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error) {
				return "<html><body>html</body></html>", nil
			},
			articleFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.ReadabilityArticleWithMarkdown, error) {
				return pageviewer.ReadabilityArticleWithMarkdown{
					ReadbilityArticle: pageviewer.ReadbilityArticle{Title: "Example"},
					Markdown:          "# Example",
				}, nil
			},
		}, nil
	}
	t.Cleanup(func() { startClient = original })

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runCLI(context.Background(), []string{
		"--url", "https://example.com",
		"--json",
		"--mode", "html",
		"--mode", "article",
	}, &stdout, &stderr)
	require.Equal(t, 0, code)
	assert.Empty(t, stderr.String())

	var got struct {
		Modes   []string `json:"modes"`
		URL     string   `json:"url"`
		Results map[string]struct {
			Content  string `json:"content"`
			Title    string `json:"title"`
			Markdown string `json:"markdown"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Equal(t, []string{"html", "article"}, got.Modes)
	assert.Equal(t, "https://example.com", got.URL)
	assert.Equal(t, "<html><body>html</body></html>", got.Results["html"].Content)
	assert.Equal(t, "Example", got.Results["article"].Title)
	assert.Equal(t, "# Example", got.Results["article"].Markdown)
}

func TestRunCLIPrintsTraceIDOnFetchError(t *testing.T) {
	original := startClient
	startClient = func(ctx context.Context, cfg pageviewer.Config) (fetcher, error) {
		return &fakeFetcher{
			htmlFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error) {
				return "", errors.New("navigation failed")
			},
		}, nil
	}
	t.Cleanup(func() { startClient = original })

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runCLI(context.Background(), []string{
		"--url", "https://example.com",
		"--mode", "html",
		"--trace-id", "req-123",
	}, &stdout, &stderr)

	require.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "navigation failed")
	assert.Contains(t, stderr.String(), "trace_id=req-123")
}

func TestRunCLIJSONFetchErrorStillWritesStderrOnly(t *testing.T) {
	original := startClient
	startClient = func(ctx context.Context, cfg pageviewer.Config) (fetcher, error) {
		return &fakeFetcher{
			htmlFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error) {
				return "", errors.New("json navigation failed")
			},
		}, nil
	}
	t.Cleanup(func() { startClient = original })

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runCLI(context.Background(), []string{
		"--url", "https://example.com",
		"--mode", "html",
		"--json",
		"--trace-id", "req-json",
	}, &stdout, &stderr)

	require.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "json navigation failed")
	assert.Contains(t, stderr.String(), "trace_id=req-json")
}

func TestRunCLIReturnsTwoOnParameterError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runCLI(context.Background(), []string{"--mode", "html"}, &stdout, &stderr)

	require.Equal(t, 2, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "--url is required")
}

func TestRunCLIReturnsTwoOnMultipleModesWithoutJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runCLI(context.Background(), []string{
		"--url", "https://example.com",
		"--mode", "html",
		"--mode", "article",
	}, &stdout, &stderr)

	require.Equal(t, 2, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "multiple --mode values require --json")
}

type fakeFetcher struct {
	closeFn   func() error
	htmlFn    func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error)
	linksFn   func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error)
	articleFn func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.ReadabilityArticleWithMarkdown, error)
	rawTextFn func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.TextResponse, error)
}

func (f *fakeFetcher) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func (f *fakeFetcher) HTML(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error) {
	if f.htmlFn != nil {
		return f.htmlFn(ctx, url, opts...)
	}
	return "", errors.New("html not configured")
}

func (f *fakeFetcher) Links(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error) {
	if f.linksFn != nil {
		return f.linksFn(ctx, url, opts...)
	}
	return "", errors.New("links not configured")
}

func (f *fakeFetcher) ReadabilityArticle(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.ReadabilityArticleWithMarkdown, error) {
	if f.articleFn != nil {
		return f.articleFn(ctx, url, opts...)
	}
	return pageviewer.ReadabilityArticleWithMarkdown{}, errors.New("article not configured")
}

func (f *fakeFetcher) RawText(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.TextResponse, error) {
	if f.rawTextFn != nil {
		return f.rawTextFn(ctx, url, opts...)
	}
	return pageviewer.TextResponse{}, errors.New("raw text not configured")
}
