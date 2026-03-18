package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/LubyRuffy/pageviewer"
)

type cliOptions struct {
	url                string
	modes              []string
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

type modeValues []string

func (m *modeValues) String() string {
	return strings.Join(*m, ",")
}

func (m *modeValues) Set(value string) error {
	*m = append(*m, value)
	return nil
}

type jsonOutputEnvelope struct {
	Modes   []string       `json:"modes"`
	URL     string         `json:"url"`
	Results map[string]any `json:"results"`
}

type textResult struct {
	Content string `json:"content"`
}

type rawTextResult struct {
	Body        string              `json:"body"`
	ContentType string              `json:"content_type"`
	StatusCode  int                 `json:"status_code"`
	FinalURL    string              `json:"final_url"`
	Header      map[string][]string `json:"header"`
}

var startClient = func(ctx context.Context, cfg pageviewer.Config) (fetcher, error) {
	return pageviewer.Start(ctx, cfg)
}

const usageText = `Usage:
  pageviewer --url <url> --mode <mode> [options]
  pageviewer --url <url> --json --mode <mode> --mode <mode> [options]

Modes:
  html       Rendered full HTML
  links      Page links text
  article    Readability article markdown / JSON fields
  raw-text   Main document raw text response

Options:
  --url string                  Target URL
  --mode value                  Output mode; repeatable with --json
  --json                        Render JSON output
  --wait-timeout duration       Page wait timeout, e.g. 15s
  --trace-id string             Trace ID for debugging
  --remove-invisible-div        Remove invisible div elements
  --acquire-timeout duration    Worker acquire timeout, e.g. 5s
  --proxy string                Browser proxy
  --no-headless                 Show browser window
  --devtools                    Open DevTools
  -h, --help                    Show this help
`

func parseFlags(args []string) (cliOptions, error) {
	fs := flag.NewFlagSet("pageviewer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts cliOptions
	var modes modeValues
	fs.StringVar(&opts.url, "url", "", "target url")
	fs.Var(&modes, "mode", "output mode: html|links|article|raw-text")
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
	opts.modes = append(opts.modes, modes...)
	if len(opts.modes) == 0 {
		return cliOptions{}, errors.New("--mode is required")
	}
	seenModes := make(map[string]struct{}, len(opts.modes))
	for _, mode := range opts.modes {
		if err := validateMode(mode); err != nil {
			return cliOptions{}, err
		}
		if _, ok := seenModes[mode]; ok {
			return cliOptions{}, fmt.Errorf("duplicate --mode: %s", mode)
		}
		seenModes[mode] = struct{}{}
	}
	if !opts.jsonOutput && len(opts.modes) > 1 {
		return cliOptions{}, errors.New("multiple --mode values require --json")
	}
	return opts, nil
}

func validateMode(mode string) error {
	switch mode {
	case "html", "links", "article", "raw-text":
		return nil
	default:
		return fmt.Errorf("invalid --mode: %s", mode)
	}
}

func buildConfig(opts cliOptions) (pageviewer.Config, []pageviewer.RequestOption) {
	cfg := pageviewer.DefaultConfig()
	cfg.Proxy = opts.proxy
	cfg.NoHeadless = opts.noHeadless
	cfg.DevTools = opts.devTools
	if opts.jsonOutput && len(opts.modes) > 1 {
		cfg.PoolSize = len(opts.modes)
		cfg.Warmup = len(opts.modes)
	}

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
		if errors.Is(err, flag.ErrHelp) {
			writeUsage(stdout)
			return 0
		}
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

	if opts.jsonOutput {
		return runJSONModes(ctx, client, opts, reqOpts, stdout, stderr)
	}

	return runTextMode(ctx, client, opts, reqOpts, stdout, stderr)
}

func runTextMode(ctx context.Context, client fetcher, opts cliOptions, reqOpts []pageviewer.RequestOption, stdout io.Writer, stderr io.Writer) int {
	switch opts.modes[0] {
	case "html":
		content, err := client.HTML(ctx, opts.url, reqOpts...)
		if err != nil {
			return writeFetchError(stderr, err, opts.traceID)
		}
		_, _ = fmt.Fprint(stdout, content)
		return 0
	case "links":
		content, err := client.Links(ctx, opts.url, reqOpts...)
		if err != nil {
			return writeFetchError(stderr, err, opts.traceID)
		}
		_, _ = fmt.Fprint(stdout, content)
		return 0
	case "article":
		article, err := client.ReadabilityArticle(ctx, opts.url, reqOpts...)
		if err != nil {
			return writeFetchError(stderr, err, opts.traceID)
		}
		_, _ = fmt.Fprint(stdout, article.Markdown)
		return 0
	case "raw-text":
		text, err := client.RawText(ctx, opts.url, reqOpts...)
		if err != nil {
			return writeFetchError(stderr, err, opts.traceID)
		}
		_, _ = fmt.Fprint(stdout, text.Body)
		return 0
	default:
		return writeFetchError(stderr, fmt.Errorf("invalid --mode: %s", opts.modes[0]), opts.traceID)
	}
}

func runJSONModes(ctx context.Context, client fetcher, opts cliOptions, reqOpts []pageviewer.RequestOption, stdout io.Writer, stderr io.Writer) int {
	type modeRun struct {
		mode   string
		result any
		err    error
	}

	resultsCh := make(chan modeRun, len(opts.modes))
	var wg sync.WaitGroup

	for _, mode := range opts.modes {
		wg.Add(1)
		go func(mode string) {
			defer wg.Done()
			result, err := fetchModeResult(ctx, client, opts.url, mode, reqOpts)
			resultsCh <- modeRun{mode: mode, result: result, err: err}
		}(mode)
	}
	wg.Wait()
	close(resultsCh)

	results := make(map[string]any, len(opts.modes))
	for result := range resultsCh {
		if result.err != nil {
			return writeFetchError(stderr, result.err, opts.traceID)
		}
		results[result.mode] = result.result
	}

	return writeJSON(stdout, stderr, jsonOutputEnvelope{
		Modes:   append([]string(nil), opts.modes...),
		URL:     opts.url,
		Results: results,
	})
}

func writeUsage(stdout io.Writer) {
	_, _ = io.WriteString(stdout, usageText)
}

func fetchModeResult(ctx context.Context, client fetcher, url, mode string, reqOpts []pageviewer.RequestOption) (any, error) {
	switch mode {
	case "html":
		content, err := client.HTML(ctx, url, reqOpts...)
		if err != nil {
			return nil, err
		}
		return textResult{Content: content}, nil
	case "links":
		content, err := client.Links(ctx, url, reqOpts...)
		if err != nil {
			return nil, err
		}
		return textResult{Content: content}, nil
	case "article":
		article, err := client.ReadabilityArticle(ctx, url, reqOpts...)
		if err != nil {
			return nil, err
		}
		return article, nil
	case "raw-text":
		text, err := client.RawText(ctx, url, reqOpts...)
		if err != nil {
			return nil, err
		}
		return rawTextResult{
			Body:        text.Body,
			ContentType: text.ContentType,
			StatusCode:  text.StatusCode,
			FinalURL:    text.FinalURL,
			Header:      text.Header,
		}, nil
	default:
		return nil, fmt.Errorf("invalid --mode: %s", mode)
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
