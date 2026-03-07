# pageviewer

基于 `rod` 的渲染式网页访问库，适合抓取渲染后的 HTML、正文、链接，以及文本型主文档原始响应。

## 特性

- 支持长驻 `Client`，内部维护共享 `Browser` 和 `Page Worker Pool`
- 保留兼容入口 `Visit`，适合一次性调用
- 同一 `Browser` / profile 下可复用会话状态，适合共享 cookie 与登录态
- 默认通过 `stealth.Page` 降低浏览器自动化识别概率
- 支持 `HTML`、`Links`、`ReadabilityArticle`、`RawText`
- 支持请求前回调 `WithBeforeRequest`
- 支持移除不可见内容 `WithRemoveInvisibleDiv`
- 支持通过 `WithTraceID` + `DebugTrace` 做最近请求排障
- 页面稳定等待会尽量容忍 `WaitLoad` / `WaitIdle` / `WaitDOMStable` 的超时，降低慢页面误报

## 安装

```bash
go get github.com/LubyRuffy/pageviewer
```

## 推荐用法

生产环境推荐使用长驻 `Client`：

```go
client, err := pageviewer.Start(ctx, pageviewer.Config{
    PoolSize:       4,
    Warmup:         2,
    AcquireTimeout: 20 * time.Second,
})
if err != nil {
    return err
}
defer client.Close()

html, err := client.HTML(ctx, "https://example.com", pageviewer.WithTraceID("req-123"))
if err != nil {
    if trace, ok := client.DebugTrace("req-123"); ok {
        log.Printf("trace=%+v", trace)
    }
    return err
}

_ = html
```

## 对外 API

推荐优先使用 `Client`：

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

## 配置项

`Config` 主要字段：

- `PoolSize`：worker 池大小，默认 `1`
- `Warmup`：启动时预热的 worker 数量，默认 `1`
- `AcquireTimeout`：借用 worker 的默认超时，默认 `20s`
- `UserDataDir`：指定浏览器用户目录
- `Debug` / `NoHeadless` / `DevTools`
- `Proxy`
- `IgnoreCertErrors`
- `ChromePath`
- `UserModeBrowser`
- `RemoteDebuggingPort`

请求级选项：

- `WithWaitTimeout`
- `WithAcquireTimeout`
- `WithBeforeRequest`
- `WithRemoveInvisibleDiv`
- `WithTraceID`
- `WithBrowser`（兼容入口 `Visit` 使用）

## 数据提取能力

DOM 模式：

- `HTML`：返回渲染后的完整 HTML
- `Links`：返回页面中的文本链接
- `ReadabilityArticle`：返回正文抽取结果，同时附带 Markdown、渲染 HTML、主文档原始 HTML

文本模式：

- `RawText`：只读取主文档响应，不读取二进制内容
- 支持的内容类型为 `text/*`、`application/json`、`application/xml`、`text/xml`

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

## 一键排障链路

如果上层已经有交互 ID，建议直接透传：

```go
interactionID := "chat-20260307-001"

_, err := client.HTML(ctx, targetURL, pageviewer.WithTraceID(interactionID))
if err != nil {
    trace, ok := client.DebugTrace(interactionID)
    if ok {
        log.Printf("trace=%+v", trace)
    }
}
```

`Stats()` 可用于观察当前池状态与最近错误：

```go
stats := client.Stats()
log.Printf("workers=%d idle=%d traces=%d lastErr=%q",
    stats.TotalWorkers,
    stats.IdleWorkers,
    stats.RecentTraces,
    stats.LastError,
)
```

## 开发文档

- 详细设计见 [docs/shared-browser-pool-development.md](docs/shared-browser-pool-development.md)
