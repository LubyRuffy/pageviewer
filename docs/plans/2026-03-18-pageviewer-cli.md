# Pageviewer CLI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为 `pageviewer` 新增 `cmd/pageviewer` 命令行工具，支持通过 `--url` 和 `--mode` 抓取网页内容，默认输出纯内容，`--json` 输出结构化结果。

**Architecture:** 在 `cmd/pageviewer` 下新增一个轻量 CLI 层，使用标准库 `flag` 做参数解析，并将参数映射到现有 `pageviewer.Config` 和请求级 `RequestOption`。CLI 内部拆分为参数解析、配置构造、抓取分发和结果渲染四层，通过可替换的启动函数和输出 writer 保障单元测试可控。

**Tech Stack:** Go 1.24.2、标准库 `flag`/`encoding/json`/`io`、现有 `pageviewer` package、testify

---

### Task 1: 搭建可测试的 CLI 骨架和参数解析

**Files:**
- Create: `cmd/pageviewer/main.go`
- Create: `cmd/pageviewer/app.go`
- Create: `cmd/pageviewer/app_test.go`

**Step 1: Write the failing test**

```go
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
	assert.Equal(t, "article", opts.mode)
	assert.True(t, opts.jsonOutput)
	assert.Equal(t, 15*time.Second, opts.waitTimeout)
	assert.Equal(t, "trace-1", opts.traceID)
	assert.True(t, opts.removeInvisibleDiv)
	assert.Equal(t, 5*time.Second, opts.acquireTimeout)
	assert.Equal(t, "http://127.0.0.1:8080", opts.proxy)
	assert.True(t, opts.noHeadless)
	assert.True(t, opts.devTools)
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -run 'TestParseFlagsRequiresURLAndMode|TestParseFlagsRejectsInvalidMode|TestParseFlagsParsesCommonOptions' -count=1`

Expected: FAIL with `undefined: parseFlags` and `undefined: cliOptions`

**Step 3: Write minimal implementation**

```go
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

func main() {
	os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
```

`runCLI` 先保留桩实现，返回 `0` 或最小错误码即可，后续任务再补完。

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -run 'TestParseFlagsRequiresURLAndMode|TestParseFlagsRejectsInvalidMode|TestParseFlagsParsesCommonOptions' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/pageviewer/main.go cmd/pageviewer/app.go cmd/pageviewer/app_test.go
git commit -m "feat: add pageviewer cli flag parsing skeleton"
```

### Task 2: 实现配置映射、模式分发和输出渲染

**Files:**
- Modify: `cmd/pageviewer/app.go`
- Modify: `cmd/pageviewer/app_test.go`

**Step 1: Write the failing test**

```go
func TestBuildConfigMapsBrowserAndRequestOptions(t *testing.T) {
	opts := cliOptions{
		url:                "https://example.com",
		mode:               "html",
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

func TestRunFetchesArticleMarkdownByDefault(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	startClient = func(ctx context.Context, cfg pageviewer.Config) (fetcher, error) {
		return fakeFetcher{
			articleFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.ReadabilityArticleWithMarkdown, error) {
				return pageviewer.ReadabilityArticleWithMarkdown{
					ReadbilityArticle: pageviewer.ReadbilityArticle{Title: "Example"},
					Markdown:          "# Example",
				}, nil
			},
		}, nil
	}
	t.Cleanup(func() { startClient = realStartClient })

	code := runCLI(context.Background(), []string{"--url", "https://example.com", "--mode", "article"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Equal(t, "# Example", strings.TrimSpace(stdout.String()))
	assert.Empty(t, stderr.String())
}

func TestRunJSONRendersRawTextEnvelope(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	startClient = func(ctx context.Context, cfg pageviewer.Config) (fetcher, error) {
		return fakeFetcher{
			rawTextFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.TextResponse, error) {
				return pageviewer.TextResponse{
					Body:        "{\"ok\":true}",
					ContentType: "application/json",
					StatusCode:  200,
					FinalURL:    url,
					Header:      http.Header{"Content-Type": []string{"application/json"}},
				}, nil
			},
		}, nil
	}
	t.Cleanup(func() { startClient = realStartClient })

	code := runCLI(context.Background(), []string{"--url", "https://example.com/api", "--mode", "raw-text", "--json"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "\"mode\":\"raw-text\"")
	assert.Contains(t, stdout.String(), "\"body\":\"{\\\"ok\\\":true}\"")
	assert.Empty(t, stderr.String())
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -run 'TestBuildConfigMapsBrowserAndRequestOptions|TestRunFetchesArticleMarkdownByDefault|TestRunJSONRendersRawTextEnvelope' -count=1`

Expected: FAIL with `undefined: buildConfig`, `undefined: startClient`, or incorrect zero-value output

**Step 3: Write minimal implementation**

```go
type fetcher interface {
	Close() error
	HTML(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error)
	Links(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error)
	ReadabilityArticle(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.ReadabilityArticleWithMarkdown, error)
	RawText(ctx context.Context, url string, opts ...pageviewer.RequestOption) (pageviewer.TextResponse, error)
}

var realStartClient = func(ctx context.Context, cfg pageviewer.Config) (fetcher, error) {
	return pageviewer.Start(ctx, cfg)
}

var startClient = realStartClient

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

func runCLI(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	opts, err := parseFlags(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	cfg, reqOpts := buildConfig(opts)
	client, err := startClient(ctx, cfg)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer client.Close()

	switch opts.mode {
	case "html":
		content, err := client.HTML(ctx, opts.url, reqOpts...)
		return writeContent(stdout, stderr, opts, content, err)
	case "links":
		content, err := client.Links(ctx, opts.url, reqOpts...)
		return writeContent(stdout, stderr, opts, content, err)
	case "article":
		article, err := client.ReadabilityArticle(ctx, opts.url, reqOpts...)
		return writeArticle(stdout, stderr, opts, article, err)
	case "raw-text":
		text, err := client.RawText(ctx, opts.url, reqOpts...)
		return writeText(stdout, stderr, opts, text, err)
	default:
		fmt.Fprintln(stderr, "invalid --mode")
		return 2
	}
}
```

纯文本模式下：

- `html` / `links` 输出字符串本体
- `article` 输出 `article.Markdown`
- `raw-text` 输出 `text.Body`

JSON 模式下统一使用 `json.NewEncoder(stdout).Encode(...)`，并至少包含 `mode` 和 `url` 字段。

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -run 'TestBuildConfigMapsBrowserAndRequestOptions|TestRunFetchesArticleMarkdownByDefault|TestRunJSONRendersRawTextEnvelope' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/pageviewer/app.go cmd/pageviewer/app_test.go
git commit -m "feat: add pageviewer cli mode dispatch and rendering"
```

### Task 3: 完善错误路径和排障输出

**Files:**
- Modify: `cmd/pageviewer/app.go`
- Modify: `cmd/pageviewer/app_test.go`

**Step 1: Write the failing test**

```go
func TestRunPrintsTraceIDOnFetchFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	startClient = func(ctx context.Context, cfg pageviewer.Config) (fetcher, error) {
		return fakeFetcher{
			htmlFn: func(ctx context.Context, url string, opts ...pageviewer.RequestOption) (string, error) {
				return "", errors.New("navigation failed")
			},
		}, nil
	}
	t.Cleanup(func() { startClient = realStartClient })

	code := runCLI(context.Background(), []string{
		"--url", "https://example.com",
		"--mode", "html",
		"--trace-id", "req-123",
	}, &stdout, &stderr)

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "navigation failed")
	assert.Contains(t, stderr.String(), "trace_id=req-123")
}

func TestRunReturnsParameterExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runCLI(context.Background(), []string{"--mode", "html"}, &stdout, &stderr)
	assert.Equal(t, 2, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "--url is required")
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -run 'TestRunPrintsTraceIDOnFetchFailure|TestRunReturnsParameterExitCode' -count=1`

Expected: FAIL because stderr does not include `trace_id=req-123` or exit code handling is incomplete

**Step 3: Write minimal implementation**

```go
func writeFetchError(stderr io.Writer, err error, traceID string) int {
	fmt.Fprintln(stderr, err)
	if traceID != "" {
		fmt.Fprintf(stderr, "trace_id=%s\n", traceID)
	}
	return 1
}

func writeContent(stdout io.Writer, stderr io.Writer, opts cliOptions, content string, err error) int {
	if err != nil {
		return writeFetchError(stderr, err, opts.traceID)
	}
	if opts.jsonOutput {
		return writeJSON(stdout, map[string]any{
			"mode":    opts.mode,
			"url":     opts.url,
			"content": content,
		}, stderr)
	}
	_, _ = fmt.Fprint(stdout, content)
	return 0
}
```

确保所有错误都只写 `stderr`，成功结果只写 `stdout`。

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -run 'TestRunPrintsTraceIDOnFetchFailure|TestRunReturnsParameterExitCode' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add cmd/pageviewer/app.go cmd/pageviewer/app_test.go
git commit -m "feat: add cli error handling and trace id output"
```

### Task 4: 补齐 README、CLI 文档、开发文档和变更记录

**Files:**
- Modify: `README.md`
- Create: `docs/CLI.md`
- Create: `ARCHITECTURE.md`
- Create: `docs/TESTING.md`
- Create: `CHANGELOG.md`

**Step 1: Write the failing doc/test check**

```bash
test -f ARCHITECTURE.md
test -f docs/CLI.md
test -f docs/TESTING.md
test -f CHANGELOG.md
rg -n "cmd/pageviewer|--mode|--json" README.md docs/CLI.md ARCHITECTURE.md docs/TESTING.md CHANGELOG.md
```

**Step 2: Run check to verify it fails**

Run: `test -f ARCHITECTURE.md && test -f docs/CLI.md && test -f docs/TESTING.md && test -f CHANGELOG.md`

Expected: FAIL because one or more files do not exist yet

**Step 3: Write minimal documentation**

`README.md` 增加：

```md
## CLI

```bash
go run ./cmd/pageviewer --url https://example.com --mode html
go run ./cmd/pageviewer --url https://example.com --mode article --json
```
```

`docs/CLI.md` 说明：

- 参数列表
- 四种 `mode` 的默认输出
- `--json` 输出示例
- `trace-id` 排障示例

`ARCHITECTURE.md` 说明：

- 库层与 CLI 层关系
- 核心模块：browser、client、CLI
- 请求流：参数解析 -> client 启动 -> mode 分发 -> 输出

`docs/TESTING.md` 说明：

- `go test ./cmd/pageviewer`
- `go test ./...`
- CLI 关键覆盖点

`CHANGELOG.md` 至少包含：

```md
## [Unreleased]

### Added
- add cmd/pageviewer CLI for html, links, article, and raw-text modes
- add CLI usage and testing documentation

### Changed
- document CLI architecture and invocation flow

### Fixed
- n/a
```

**Step 4: Run verification**

Run: `GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer ./... -count=1`

Expected: PASS

再运行：

Run: `go run ./cmd/pageviewer --url https://example.com --mode raw-text --json`

Expected: 输出包含 `mode`、`url`、`body`

**Step 5: Commit**

```bash
git add README.md docs/CLI.md ARCHITECTURE.md docs/TESTING.md CHANGELOG.md
git commit -m "docs: add pageviewer cli documentation and changelog"
```
