# pageviewer

基于 `rod` 的渲染式网页访问库，也提供 `cmd/pageviewer` 命令行工具。适合抓取渲染后的 HTML、正文、链接，以及文本型主文档原始响应。

## 项目简介

- 库层提供长驻 `Client`，内部维护共享 `Browser` 和 `Page Worker Pool`
- 保留兼容入口 `Visit`，适合一次性调用
- 同一 `Browser` / profile 下可复用会话状态，适合共享 cookie 与登录态
- 默认通过 `stealth.Page` 降低浏览器自动化识别概率
- 支持 `HTML`、`Links`、`ReadabilityArticle`、`RawText`
- 支持请求前回调 `WithBeforeRequest`
- 支持移除不可见内容 `WithRemoveInvisibleDiv`
- 支持通过 `WithTraceID` + `DebugTrace` 做最近请求排障
- 页面稳定等待会尽量容忍 `WaitLoad` / `WaitIdle` / `WaitDOMStable` 的超时，降低慢页面误报

## 快速启动

安装库：

```bash
go get github.com/LubyRuffy/pageviewer
```

运行测试：

```bash
GOTOOLCHAIN=go1.24.2 go test ./...
```

构建 CLI：

```bash
go build -o bin/pageviewer ./cmd/pageviewer
```

查看 CLI 帮助：

```bash
go run ./cmd/pageviewer --help
```

## 使用示例

推荐在生产环境使用长驻 `Client`：

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

CLI 示例：

```bash
go run ./cmd/pageviewer --url https://example.com
go run ./cmd/pageviewer --url ip.bmh.im
go run ./cmd/pageviewer --url https://example.com --mode article --json
go run ./cmd/pageviewer --url https://example.com --json --mode html --mode article
go run ./cmd/pageviewer --url https://example.com --mode html --trace-id req-123
```

## 常见使用方式

- `html`：抓取渲染后的完整 HTML
- 默认不传 `--mode` 时，按 `html` 处理
- `--url` 不带 scheme 时，会先按 `https://` 规范化
- `links`：抓取页面中的文本链接
- `article`：抓取正文并输出 Markdown
- `raw-text`：只读取主文档响应，适合文本型接口
- `--json`：输出结构化结果，并支持重复传入 `--mode` 一次拿到多种结果
- `--trace-id`：把一次交互 ID 传入请求，便于失败后追踪
- 参数、输出结构和退出码详见 [docs/CLI.md](docs/CLI.md)

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

## 开发文档

- 项目约定见 [AGENTS.md](AGENTS.md)
- 架构说明见 [ARCHITECTURE.md](ARCHITECTURE.md)
- CLI 说明见 [docs/CLI.md](docs/CLI.md)
- 配置说明见 [docs/CONFIG.md](docs/CONFIG.md)
- 测试说明见 [docs/TESTING.md](docs/TESTING.md)
- 变更记录见 [CHANGELOG.md](CHANGELOG.md)
- 详细设计见 [docs/shared-browser-pool-development.md](docs/shared-browser-pool-development.md)
