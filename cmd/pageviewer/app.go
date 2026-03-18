package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/LubyRuffy/pageviewer"
)

type cliOptions struct {
	url                string
	mode               string
	jsonOutput         bool
	waitTimeout        time.Duration
	traceID            string
	removeInvisibleDiv bool
	acquireTimeout     time.Duration
	proxy              string
	noHeadless         bool
	devTools           bool
}

type fetcher interface {
	Close() error
	HTML(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error)
	Links(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error)
	ReadabilityArticle(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.ReadabilityArticleWithMarkdown, error)
	RawText(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.TextResponse, error)
}

type outputEnvelope struct {
	Mode string `json:"mode"`
	URL  string `json:"url"`
}

type textOutputEnvelope struct {
	outputEnvelope
	Content string `json:"content"`
}

type articleOutputEnvelope struct {
	outputEnvelope
	pageviewer.ReadabilityArticleWithMarkdown
}

type rawTextOutputEnvelope struct {
	outputEnvelope
	Body        string              `json:"body"`
	ContentType string              `json:"content_type"`
	StatusCode  int                 `json:"status_code"`
	FinalURL    string              `json:"final_url"`
	Header      map[string][]string `json:"header"`
}

var startClient = func(ctx context.Context, cfg pageviewer.Config) (fetcher, error) {
	return pageviewer.Start(ctx, cfg)
}

func parseFlags(args []string) (cliOptions, error) {
	fs := flag.NewFlagSet("pageviewer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts cliOptions
	fs.StringVar(&opts.url, "url", "", "target url")
	fs.StringVar(&opts.mode, "mode", "", "output mode: html|links|article|raw-text")
	fs.BoolVar(&opts.jsonOutput, "json", false, "render JSON output")
	fs.DurationVar(&opts.waitTimeout, "wait-timeout", 0, "page wait timeout")
	fs.StringVar(&opts.traceID, "trace-id", "", "trace id")
	fs.BoolVar(&opts.removeInvisibleDiv, "remove-invisible-div", false, "remove invisible div")
	fs.DurationVar(&opts.acquireTimeout, "acquire-timeout", 0, "worker acquire timeout")
	fs.StringVar(&opts.proxy, "proxy", "", "browser proxy")
	fs.BoolVar(&opts.noHeadless, "no-headless", false, "show browser window")
	fs.BoolVar(&opts.devTools, "devtools", false, "open devtools")

	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}
	if opts.url == "" {
		return cliOptions{}, errors.New("--url is required")
	}
	if opts.mode == "" {
		return cliOptions{}, errors.New("--mode is required")
	}
	switch opts.mode {
	case "html", "links", "article", "raw-text":
	default:
		return cliOptions{}, fmt.Errorf("invalid --mode: %s", opts.mode)
	}
	return opts, nil
}

func buildConfig(opts cliOptions) (pageviewer.Config, []pageviewer.RequestOption) {
	cfg := pageviewer.DefaultConfig()
	cfg.Proxy = opts.proxy
	cfg.NoHeadless = opts.noHeadless
	cfg.DevTools = opts.devTools

	reqOpts := make([]pageviewer.RequestOption, 0, 4)
	if opts.waitTimeout > 0 {
		reqOpts = append(reqOpts, pageviewer.WithWaitTimeout(opts.waitTimeout))
	}
	if opts.traceID != "" {
		reqOpts = append(reqOpts, pageviewer.WithTraceID(opts.traceID))
	}
	if opts.removeInvisibleDiv {
		reqOpts = append(reqOpts, pageviewer.WithRemoveInvisibleDiv(true))
	}
	if opts.acquireTimeout > 0 {
		reqOpts = append(reqOpts, pageviewer.WithAcquireTimeout(opts.acquireTimeout))
	}

	return cfg, reqOpts
}

func runCLI(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) (exitCode int) {
	opts, err := parseFlags(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	cfg, reqOpts := buildConfig(opts)
	client, err := startClient(ctx, cfg)
	if err != nil {
		return writeFetchError(stderr, err, opts.traceID)
	}

	defer func() {
		if closeErr := client.Close(); closeErr != nil && exitCode == 0 {
			exitCode = writeError(stderr, closeErr)
		}
	}()

	switch opts.mode {
	case "html":
		content, err := client.HTML(ctx, opts.url, reqOpts...)
		if err != nil {
			return writeFetchError(stderr, err, opts.traceID)
		}
		if opts.jsonOutput {
			return writeJSON(stdout, stderr, textOutputEnvelope{
				outputEnvelope: outputEnvelope{Mode: opts.mode, URL: opts.url},
				Content:        content,
			})
		}
		_, _ = fmt.Fprint(stdout, content)
		return 0
	case "links":
		content, err := client.Links(ctx, opts.url, reqOpts...)
		if err != nil {
			return writeFetchError(stderr, err, opts.traceID)
		}
		if opts.jsonOutput {
			return writeJSON(stdout, stderr, textOutputEnvelope{
				outputEnvelope: outputEnvelope{Mode: opts.mode, URL: opts.url},
				Content:        content,
			})
		}
		_, _ = fmt.Fprint(stdout, content)
		return 0
	case "article":
		article, err := client.ReadabilityArticle(ctx, opts.url, reqOpts...)
		if err != nil {
			return writeFetchError(stderr, err, opts.traceID)
		}
		if opts.jsonOutput {
			return writeJSON(stdout, stderr, articleOutputEnvelope{
				outputEnvelope:                 outputEnvelope{Mode: opts.mode, URL: opts.url},
				ReadabilityArticleWithMarkdown: article,
			})
		}
		_, _ = fmt.Fprint(stdout, article.Markdown)
		return 0
	case "raw-text":
		text, err := client.RawText(ctx, opts.url, reqOpts...)
		if err != nil {
			return writeFetchError(stderr, err, opts.traceID)
		}
		if opts.jsonOutput {
			return writeJSON(stdout, stderr, rawTextOutputEnvelope{
				outputEnvelope: outputEnvelope{Mode: opts.mode, URL: opts.url},
				Body:           text.Body,
				ContentType:    text.ContentType,
				StatusCode:     text.StatusCode,
				FinalURL:       text.FinalURL,
				Header:         text.Header,
			})
		}
		_, _ = fmt.Fprint(stdout, text.Body)
		return 0
	default:
		return writeFetchError(stderr, fmt.Errorf("invalid --mode: %s", opts.mode), opts.traceID)
	}
}

func writeError(stderr io.Writer, err error) int {
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintln(stderr, err)
	return 1
}

func writeFetchError(stderr io.Writer, err error, traceID string) int {
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintln(stderr, err)
	if traceID != "" {
		_, _ = fmt.Fprintf(stderr, "trace_id=%s\n", traceID)
	}
	return 1
}

func writeJSON(stdout io.Writer, stderr io.Writer, value any) int {
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return writeError(stderr, err)
	}
	return 0
}
