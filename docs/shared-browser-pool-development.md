# Shared Browser Pool 开发文档

## 目标

本次重构把 `pageviewer` 从“一次性抓取工具”整理成“可长驻、可复用、可池化”的 Go package。

目标约束：

- 上层业务可以 `Start` 一次并长期持有
- 包内维护共享会话的 page worker 池
- 进程退出时统一 `Close`
- 保留旧的 `Visit` 兼容入口
- 新增文本型主文档原始返回能力 `RawText`

## 当前架构

核心结构是“单 Browser + Page Worker Pool”：

- `Client`
  - 生命周期入口
  - 负责 `Start`、`Close`、`Stats`、`DebugTrace`
  - 管理共享 browser、worker 池、默认借用超时
- `worker`
  - 持有一个长期复用的 `rod.Page`
  - 由池借出和归还
- `workerPool`
  - 控制池容量
  - 提供超时获取与幂等释放
- `Browser`
  - 封装 rod browser
  - 负责页面导航、等待页面稳定、读取主文档响应

这不是多 browser 进程池。所有 worker 来自同一个 browser/profile，因此默认共享：

- cookie
- localStorage
- 登录态

## 生命周期

### 启动

入口：

```go
client, err := pageviewer.Start(ctx, pageviewer.Config{
    PoolSize:       4,
    Warmup:         2,
    AcquireTimeout: 20 * time.Second,
})
```

启动流程：

1. 归一化 `Config`
2. 创建共享 `Browser`
3. 创建 `workerPool`
4. 按 `Warmup` 预热真实 `rod.Page` worker
5. 后台补建剩余 worker，直到 `TotalWorkers == PoolSize`
6. 任一步失败时统一清理 browser 和已创建 worker

### 关闭

入口：

```go
err := client.Close()
```

关闭语义：

- 幂等
- 不再接受新请求
- 等待在途请求、异步补建与修复任务结束
- 关闭所有已登记 worker
- 关闭 browser
- 重置 `Stats`

## 对外 API

推荐上层使用 `Client`：

```go
func Start(ctx context.Context, cfg Config) (*Client, error)
func (c *Client) Close() error
func (c *Client) Stats() Stats
func (c *Client) DebugTrace(id string) (Trace, bool)

func (c *Client) Visit(ctx context.Context, url string, fn func(page *rod.Page) error, opts ...RequestOption) error
func (c *Client) HTML(ctx context.Context, url string, opts ...RequestOption) (string, error)
func (c *Client) Links(ctx context.Context, url string, opts ...RequestOption) (string, error)
func (c *Client) ReadabilityArticle(ctx context.Context, url string, opts ...RequestOption) (ReadabilityArticleWithMarkdown, error)
func (c *Client) RawText(ctx context.Context, url string, opts ...RequestOption) (TextResponse, error)
```

兼容层仍保留：

```go
func Visit(u string, onPageLoad func(page *rod.Page) error, opts ...VisitOption) error
```

但兼容层内部已经不再直接跑旧的隐式页面逻辑，而是通过临时 `Client` 执行。
这个临时 `Client` 不会为了 one-shot `Visit` 再额外补建 worker，避免请求结束后进入无意义的 repair 路径。

## 配置与请求选项

### `Config`

关键字段：

- `PoolSize`
- `Warmup`
- `AcquireTimeout`
- `UserDataDir`
- `Debug`
- `NoHeadless`
- `DevTools`
- `Proxy`
- `IgnoreCertErrors`
- `ChromePath`
- `UserModeBrowser`
- `RemoteDebuggingPort`

规则：

- `PoolSize <= 0` 时回落到默认值 `1`
- `AcquireTimeout <= 0` 时回落到默认值 `20s`
- `Warmup <= 0` 时回落到默认值 `1`
- `Warmup > PoolSize` 时自动截断到 `PoolSize`
- `Warmup < PoolSize` 时，剩余 worker 由后台补到 `PoolSize`
- 后台 worker 补建/修复使用独立 provisioning timeout，不直接复用 `AcquireTimeout`

### `RequestOptions`

关键字段：

- `WaitTimeout`
- `AcquireTimeout`
- `BeforeRequest`
- `RemoveInvisibleDiv`
- `TraceID`

借用 worker 的实际等待时间取以下最小值：

- `ctx` deadline
- `RequestOptions.AcquireTimeout`
- `Config.AcquireTimeout`

## Stats 与 Trace

当前 `Stats` 暴露的最小观测字段：

- `TotalWorkers`
- `IdleWorkers`
- `RecentTraces`
- `LastError`

调用方如果已经有自己的交互 id，推荐直接透传：

```go
html, err := client.HTML(ctx, targetURL, pageviewer.WithTraceID(interactionID))
if err != nil {
    trace, ok := client.DebugTrace(interactionID)
    if ok {
        log.Printf("trace=%+v", trace)
    }
}
```

`Trace` 目前会记录：

- `TraceID`
- `URL`
- `Mode` (`dom` / `text`)
- `WorkerID`
- `AcquireWait`
- `StatusCode`
- `ContentType`
- `FinalURL`
- `ErrorMessage`
- `BrokenWorker`

如果同一个 `TraceID` 被重复使用：

- 顶层字段表示“最新开始的那次请求”的摘要
- `AttemptCount` 表示同 id 已记录的请求次数
- `Attempts` 按请求开始顺序保留链路，便于排查失败后重试或并发重入

## DOM 模式

以下能力走 DOM 模式：

- `Visit`
- `HTML`
- `Links`
- `ReadabilityArticle`

执行流程：

1. 从 `workerPool` 借一个 worker
2. 在复用 page 上导航
3. 等待主文档响应
4. 等待页面 load / idle / dom stable
5. 执行提取逻辑
6. 根据 page 状态决定归还或重建 worker

为了适配“复用同一个 page”的模型，主文档响应监听已经重构成“一次性 wait + 显式 cancel”，避免在复用 page 上持续累积 `EachEvent` 监听器。

## Text 模式

`RawText` 只支持文本型主文档响应。

支持的类型：

- `text/*`
- `application/json`
- `application/xml`
- `text/xml`

返回结构：

```go
type TextResponse struct {
    Body        string
    ContentType string
    StatusCode  int
    FinalURL    string
    Header      http.Header
}
```

行为约束：

- 只读取主文档响应
- 不读取二进制内容
- 非支持类型返回 `ErrUnsupportedContentType`

## 错误模型

当前可判定错误：

- `ErrClosed`
- `ErrAcquireTimeout`
- `ErrBrowserUnavailable`
- `ErrNavigationFailed`
- `ErrUnsupportedContentType`
- `ErrWorkerBroken`

上层应优先用 `errors.Is` 判断，而不是匹配错误字符串。

## Worker 修复

以下情况会把 worker 视为不可复用：

- 导航/执行时 page 已损坏
- DOM 路径明确要求不复用当前 page
- `page.Info()` 检查失败

被判定为损坏的 worker：

- 不再归还池
- 关闭旧 page
- 后台异步补建新 worker

## 测试策略

本次调整后的测试规则：

- 默认测试只使用 `httptest`
- 默认测试不依赖外网站点
- 默认测试不依赖固定的本地 `user-data-dir`
- 默认测试完成后不应残留 Chromium 进程

已验证命令：

```bash
GOTOOLCHAIN=go1.24.2 go test ./... -count=1
GOTOOLCHAIN=go1.24.2 go test ./... -race -count=1
```

并且在验证后确认：

- rod headless Chromium 残留数为 `0`
- `/tmp/pageviewer_data` 这一类测试浏览器残留数为 `0`

## 开发注意事项

1. 不要在测试中随手调用 `NewVisitOptions()` 只为了拿默认 `PageOptions`。
   `NewVisitOptions()` 会补默认 browser，可能意外拉起全局 `DefaultBrowser`。

2. 如果只想拿默认页面等待配置，应使用：

```go
newDefaultVisitOptions().PageOptions
```

3. 新增基于复用 page 的逻辑时，不要直接照搬旧的 `go page.EachEvent(... )()` 持续监听写法。
   这会在 worker 长期复用场景下累积监听器。

4. 如果新增测试会启动真实 browser，必须显式 `Close()`，并在必要时重置全局 `defaultBrowser`。

5. 生产推荐只使用 `Client`，包级 `Visit` 仅作为兼容入口保留。

6. 如果上层已经有 interaction id，请统一通过 `WithTraceID(id)` 透传，问题排查时再用 `client.DebugTrace(id)` 取回最近链路。
