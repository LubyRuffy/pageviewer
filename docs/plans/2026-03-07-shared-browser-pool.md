# Shared Browser Pool Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 把 `pageviewer` 重构为一个可长驻、可池化、共享登录态的 Go package，并新增稳定的文本原始返回能力。

**Architecture:** 采用单个长驻 `Browser` 加 `Page Worker Pool` 的结构，由 `Client` 管理生命周期、调度、故障恢复和排障链路。保留现有 DOM 提取能力，但统一收口到 `Client`，并新增只支持文本类型的 `RawText` 主文档抓取能力。

**Tech Stack:** Go 1.24.2、rod、stealth、net/http/httptest、testify

---

### Task 1: 引入公共配置、错误类型和返回结构

**Files:**
- Create: `config.go`
- Create: `errors.go`
- Create: `text_response.go`
- Create: `config_test.go`

**Step 1: Write the failing test**

```go
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 1, cfg.PoolSize)
	assert.Equal(t, 20*time.Second, cfg.AcquireTimeout)
}

func TestIsTextContentType(t *testing.T) {
	assert.True(t, isTextContentType("text/html; charset=utf-8"))
	assert.True(t, isTextContentType("application/json"))
	assert.False(t, isTextContentType("application/pdf"))
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestDefaultConfig|TestIsTextContentType' -count=1`

Expected: FAIL with `undefined: DefaultConfig`, `undefined: isTextContentType`

**Step 3: Write minimal implementation**

```go
type Config struct {
	PoolSize       int
	AcquireTimeout time.Duration
	UserDataDir    string
	Warmup         int
}

func DefaultConfig() Config {
	return Config{
		PoolSize:       1,
		AcquireTimeout: 20 * time.Second,
	}
}

var (
	ErrClosed                 = errors.New("pageviewer: client closed")
	ErrAcquireTimeout         = errors.New("pageviewer: acquire timeout")
	ErrBrowserUnavailable     = errors.New("pageviewer: browser unavailable")
	ErrNavigationFailed       = errors.New("pageviewer: navigation failed")
	ErrUnsupportedContentType = errors.New("pageviewer: unsupported content type")
	ErrWorkerBroken           = errors.New("pageviewer: worker broken")
)

type TextResponse struct {
	Body        string
	ContentType string
	StatusCode  int
	FinalURL    string
	Header      http.Header
}
```

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestDefaultConfig|TestIsTextContentType' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add config.go errors.go text_response.go config_test.go
git commit -m "feat: add client config and text response primitives"
```

### Task 2: 实现请求级选项并替换一次性 Visit 配置拼装

**Files:**
- Create: `request_options.go`
- Create: `request_options_test.go`
- Modify: `pageviewer.go`

**Step 1: Write the failing test**

```go
func TestNewRequestOptionsUsesDefaults(t *testing.T) {
	opts := NewRequestOptions()
	assert.Equal(t, DefaultWaitStableTimeout, opts.WaitTimeout)
	assert.Empty(t, opts.TraceID)
}

func TestRequestOptionsOverrideAcquireTimeout(t *testing.T) {
	opts := NewRequestOptions(WithAcquireTimeout(3 * time.Second))
	assert.Equal(t, 3*time.Second, opts.AcquireTimeout)
}

func TestRequestOptionsKeepBeforeRequest(t *testing.T) {
	called := false
	opts := NewRequestOptions(WithBeforeRequest(func(page *rod.Page) error {
		called = true
		return nil
	}))
	require.NotNil(t, opts.BeforeRequest)
	assert.False(t, called)
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestNewRequestOptionsUsesDefaults|TestRequestOptionsOverrideAcquireTimeout|TestRequestOptionsKeepBeforeRequest' -count=1`

Expected: FAIL with `undefined: RequestOptions`

**Step 3: Write minimal implementation**

```go
type RequestOptions struct {
	WaitTimeout        time.Duration
	AcquireTimeout     time.Duration
	BeforeRequest      func(page *rod.Page) error
	RemoveInvisibleDiv bool
	TraceID            string
}

func NewRequestOptions(opts ...RequestOption) RequestOptions {
	ro := RequestOptions{
		WaitTimeout: DefaultWaitStableTimeout,
	}
	for _, opt := range opts {
		opt(&ro)
	}
	return ro
}

func WithAcquireTimeout(timeout time.Duration) RequestOption {
	return func(ro *RequestOptions) {
		ro.AcquireTimeout = timeout
	}
}
```

把现有 `VisitOption` 适配到 `RequestOption`，并确保包级 `Visit` 只是兼容层，不再直接扩展内部状态。

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestNewRequestOptionsUsesDefaults|TestRequestOptionsOverrideAcquireTimeout|TestRequestOptionsKeepBeforeRequest' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add request_options.go request_options_test.go pageviewer.go
git commit -m "refactor: split request options from legacy visit options"
```

### Task 3: 先做纯内存 worker pool 和超时控制

**Files:**
- Create: `pool.go`
- Create: `pool_test.go`

**Step 1: Write the failing test**

```go
func TestPoolAcquireAndRelease(t *testing.T) {
	p := newWorkerPool(1)
	w := &worker{id: 1}
	require.NoError(t, p.fill(w))

	got, release, err := p.acquire(context.Background(), 50*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, w, got)

	release(workerStateReady)

	gotAgain, _, err := p.acquire(context.Background(), 50*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, w, gotAgain)
}

func TestPoolAcquireTimeout(t *testing.T) {
	p := newWorkerPool(1)
	require.NoError(t, p.fill(&worker{id: 1}))

	_, release, err := p.acquire(context.Background(), 50*time.Millisecond)
	require.NoError(t, err)
	defer release(workerStateReady)

	_, _, err = p.acquire(context.Background(), 10*time.Millisecond)
	assert.ErrorIs(t, err, ErrAcquireTimeout)
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestPoolAcquireAndRelease|TestPoolAcquireTimeout' -count=1`

Expected: FAIL with `undefined: newWorkerPool`

**Step 3: Write minimal implementation**

```go
type worker struct {
	id int
}

type workerState int

const (
	workerStateReady workerState = iota
	workerStateBroken
)

type workerPool struct {
	ch chan *worker
}

func newWorkerPool(size int) *workerPool {
	return &workerPool{ch: make(chan *worker, size)}
}

func (p *workerPool) fill(w *worker) error {
	p.ch <- w
	return nil
}

func (p *workerPool) acquire(ctx context.Context, timeout time.Duration) (*worker, func(workerState), error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case w := <-p.ch:
		return w, func(state workerState) {
			if state == workerStateReady {
				p.ch <- w
			}
		}, nil
	case <-ctx.Done():
		return nil, nil, ErrAcquireTimeout
	}
}
```

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestPoolAcquireAndRelease|TestPoolAcquireTimeout' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add pool.go pool_test.go
git commit -m "feat: add basic worker pool with timeout"
```

### Task 4: 建立 Client 生命周期并让 Start/Close 可测

**Files:**
- Create: `client.go`
- Create: `client_test.go`
- Modify: `browser.go`

**Step 1: Write the failing test**

```go
func TestStartCreatesClientWithWarmWorkers(t *testing.T) {
	client, err := Start(context.Background(), Config{
		PoolSize:       1,
		Warmup:         1,
		AcquireTimeout: time.Second,
	})
	require.NoError(t, err)
	defer client.Close()

	stats := client.Stats()
	assert.Equal(t, 1, stats.TotalWorkers)
}

func TestCloseIsIdempotent(t *testing.T) {
	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	require.NoError(t, client.Close())
	require.NoError(t, client.Close())
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestStartCreatesClientWithWarmWorkers|TestCloseIsIdempotent' -count=1`

Expected: FAIL with `undefined: Start`

**Step 3: Write minimal implementation**

```go
type Client struct {
	browser *Browser
	pool    *workerPool
	closed  atomic.Bool
}

func Start(ctx context.Context, cfg Config) (*Client, error) {
	b, err := NewBrowser()
	if err != nil {
		return nil, err
	}
	pool := newWorkerPool(cfg.PoolSize)
	// 预热 page worker
	return &Client{browser: b, pool: pool}, nil
}

func (c *Client) Close() error {
	if c == nil || c.closed.Swap(true) {
		return nil
	}
	return c.browser.Close()
}
```

补上 `Stats` 结构，并把 `NewBrowser` 的配置透传到 `Config`。

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestStartCreatesClientWithWarmWorkers|TestCloseIsIdempotent' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add client.go client_test.go browser.go
git commit -m "feat: add client lifecycle with start and close"
```

### Task 5: 把 DOM 能力迁移到 Client，并保留兼容 Visit

**Files:**
- Modify: `browser.go`
- Modify: `pageviewer.go`
- Create: `client_dom_test.go`

**Step 1: Write the failing test**

```go
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
	assert.Contains(t, html, "app")
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestClientHTMLUsesSharedBrowser' -count=1`

Expected: FAIL with `client.HTML undefined`

**Step 3: Write minimal implementation**

```go
func (c *Client) HTML(ctx context.Context, url string, opts ...RequestOption) (string, error) {
	var out string
	err := c.Visit(ctx, url, func(page *rod.Page) error {
		var err error
		out, err = page.HTML()
		return err
	}, opts...)
	return out, err
}
```

把 `Links`、`ReadabilityArticle` 和 `Visit` 都收口为“借 worker -> 导航 -> 等待 -> 执行回调 -> 归还/重建”的统一路径。保留原有包级 `Visit`，但内部改为通过一个临时 `Client` 或默认兼容适配层调用，避免继续走旧的隐式全局逻辑。

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestClientHTMLUsesSharedBrowser|TestBrowser_Links|TestBrowser_ReadabilityArticle' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add browser.go pageviewer.go client_dom_test.go browser_test.go
git commit -m "refactor: move dom operations under client"
```

### Task 6: 实现只支持文本类型的 RawText 主文档抓取

**Files:**
- Create: `text.go`
- Create: `text_test.go`
- Modify: `browser.go`
- Modify: `client.go`

**Step 1: Write the failing test**

```go
func TestClientRawTextReturnsJSON(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.RawText(context.Background(), s.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.Equal(t, "application/json", resp.ContentType)
	assert.JSONEq(t, `{"ok":true}`, resp.Body)
}

func TestClientRawTextRejectsPDF(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("%PDF-1.7"))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	_, err = client.RawText(context.Background(), s.URL)
	assert.ErrorIs(t, err, ErrUnsupportedContentType)
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestClientRawTextReturnsJSON|TestClientRawTextRejectsPDF' -count=1`

Expected: FAIL with `client.RawText undefined`

**Step 3: Write minimal implementation**

```go
func (c *Client) RawText(ctx context.Context, url string, opts ...RequestOption) (TextResponse, error) {
	var out TextResponse
	err := c.withWorker(ctx, opts, func(page *rod.Page, ro RequestOptions) error {
		// 监听主文档 response，读取 body、status、header、final URL
		return nil
	})
	return out, err
}
```

实现时必须：

- 只读取主文档响应
- 验证 `Content-Type`
- 对 `text/*`、`application/json`、`application/xml`、`text/xml` 返回正文
- 对非文本类型返回 `ErrUnsupportedContentType`

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestClientRawTextReturnsJSON|TestClientRawTextRejectsPDF' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add text.go text_test.go browser.go client.go
git commit -m "feat: add raw text response support"
```

### Task 7: 加入 TraceID 排障链路和 Stats 观测

**Files:**
- Create: `trace.go`
- Create: `trace_test.go`
- Modify: `client.go`

**Step 1: Write the failing test**

```go
func TestClientDebugTraceReturnsRecentTrace(t *testing.T) {
	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	traceID := "trace-123"
	_, err = client.HTML(context.Background(), "http://127.0.0.1:1", WithTraceID(traceID))
	require.Error(t, err)

	trace, ok := client.DebugTrace(traceID)
	require.True(t, ok)
	assert.Equal(t, traceID, trace.TraceID)
	assert.NotEmpty(t, trace.ErrorMessage)
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestClientDebugTraceReturnsRecentTrace' -count=1`

Expected: FAIL with `client.DebugTrace undefined`

**Step 3: Write minimal implementation**

```go
type Trace struct {
	TraceID      string
	URL          string
	Mode         string
	WorkerID     int
	ErrorMessage string
}

type traceRecorder struct {
	mu    sync.RWMutex
	items map[string]Trace
}
```

把 trace 记录接入 `Visit` 和 `RawText`，并在 `Stats` 中增加：

- `TotalWorkers`
- `IdleWorkers`
- `BrokenWorkers`
- `BrowserGeneration`
- `LastError`

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestClientDebugTraceReturnsRecentTrace' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add trace.go trace_test.go client.go
git commit -m "feat: add trace recording and client stats"
```

### Task 8: 做 worker 损坏摘除与后台补建

**Files:**
- Create: `supervisor.go`
- Create: `supervisor_test.go`
- Modify: `client.go`
- Modify: `pool.go`

**Step 1: Write the failing test**

```go
func TestBrokenWorkerIsReplaced(t *testing.T) {
	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	first := client.Stats().TotalWorkers
	err = client.markWorkerBrokenForTest(1)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		stats := client.Stats()
		return stats.TotalWorkers == first && stats.BrokenWorkers == 0
	}, 5*time.Second, 100*time.Millisecond)
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestBrokenWorkerIsReplaced' -count=1`

Expected: FAIL with `undefined: (*Client).markWorkerBrokenForTest`

**Step 3: Write minimal implementation**

```go
func (c *Client) handleBrokenWorker(w *worker) {
	go func() {
		_ = c.rebuildWorker(w.id)
	}()
}
```

关键点：

- 当前请求报错时不要把坏 worker 放回池
- 补建成功后再重新放回池
- `Stats` 和 `Trace` 要记录重建行为

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestBrokenWorkerIsReplaced' -count=1`

Expected: PASS

**Step 5: Commit**

```bash
git add supervisor.go supervisor_test.go client.go pool.go
git commit -m "feat: rebuild broken workers in background"
```

### Task 9: 清理默认外网依赖测试并补齐共享会话集成测试

**Files:**
- Modify: `browser_test.go`
- Modify: `pageviewer_test.go`
- Create: `integration_shared_session_test.go`

**Step 1: Write the failing test**

```go
func TestClientSharesSessionAcrossRequests(t *testing.T) {
	var seen string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("token"); err == nil {
			seen = c.Value
		}
		http.SetCookie(w, &http.Cookie{Name: "token", Value: "abc"})
		_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	_, err = client.HTML(context.Background(), s.URL)
	require.NoError(t, err)
	_, err = client.HTML(context.Background(), s.URL)
	require.NoError(t, err)

	assert.Equal(t, "abc", seen)
}
```

**Step 2: Run test to verify it fails**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -run 'TestClientSharesSessionAcrossRequests' -count=1`

Expected: FAIL until共享会话真正生效或测试路径尚未迁移

**Step 3: Write minimal implementation**

将当前默认测试中的外网站点依赖改成：

- 默认不跑外网
- 外网用例放入 `if os.Getenv("E2E") == "1"` 分支
- 默认测试全部使用 `httptest`

**Step 4: Run test to verify it passes**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -count=1`

Expected: PASS without public internet access

**Step 5: Commit**

```bash
git add browser_test.go pageviewer_test.go integration_shared_session_test.go
git commit -m "test: remove default external dependencies and cover shared session"
```

### Task 10: 完成最终验证和文档更新

**Files:**
- Modify: `README.md`

**Step 1: Write the failing test**

这个任务不新增失败用例，直接做验证和文档补齐。

**Step 2: Run verification commands**

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -count=1`

Expected: PASS

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -race -count=1`

Expected: PASS

Run: `GOTOOLCHAIN=go1.24.2 go test ./... -coverprofile=coverage.out -count=1`

Expected: PASS with coverage `>= 85%`

Run: `GOTOOLCHAIN=go1.24.2 go tool cover -func=coverage.out`

Expected: 核心实现文件达到预期覆盖率，未覆盖部分有明确理由

**Step 3: Update docs**

在 `README.md` 中补齐：

- `Start` / `Close` 用法
- `RawText` 支持的内容类型
- `TraceID` 排障方式
- 默认测试不访问外网的说明

**Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document long-lived client usage and verification"
```
