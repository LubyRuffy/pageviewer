# 配置说明

## 概览

`pageviewer` 的配置分为两层：

- 浏览器 / Client 级配置：通过 `pageviewer.Config` 控制
- 请求级配置：通过 `RequestOption` 或 CLI 参数控制

## `pageviewer.Config`

库层可通过 `pageviewer.Start(ctx, cfg)` 传入配置。

常用字段：

- `PoolSize`：worker 池大小，默认 `1`
- `Warmup`：启动预热 worker 数量，默认 `1`
- `AcquireTimeout`：默认 worker 借用超时，默认 `20s`
- `Proxy`：浏览器代理
- `NoHeadless`：是否显示浏览器窗口
- `DevTools`：是否打开 DevTools
- `UserDataDir`：指定浏览器用户目录
- `IgnoreCertErrors`：忽略证书错误
- `ChromePath`：指定 Chrome 可执行文件
- `UserModeBrowser`：复用用户浏览器
- `RemoteDebuggingPort`：指定远程调试端口

浏览器启动补充：

- 默认会优先启用 leakless
- 如果 upstream leakless 的固定锁端口不可用，`NewBrowser` 会快速降级为非 leakless，避免启动阶段无限等待
- 如果你需要显式关闭 leakless，可使用 `WithLeakless(false)`

示例：

```go
client, err := pageviewer.Start(ctx, pageviewer.Config{
	PoolSize:       2,
	Warmup:         1,
	AcquireTimeout: 20 * time.Second,
	Proxy:          "http://127.0.0.1:8080",
	NoHeadless:     true,
})
```

## 请求级配置

常用请求选项：

- `WithWaitTimeout`
- `WithAcquireTimeout`
- `WithTraceID`
- `WithRemoveInvisibleDiv`
- `WithBeforeRequest`

请求行为补充：

- `RawText` 会默认阻断主文档之外的子资源请求，例如图片、样式、字体、脚本和其他二进制资源
- 调用方 `ctx` 的取消和 deadline 会传播到主文档响应等待阶段；如果主文档完成事件缺失，请求会返回 `context.Canceled` 或 `context.DeadlineExceeded`

示例：

```go
html, err := client.HTML(
	ctx,
	"https://example.com",
	pageviewer.WithWaitTimeout(15*time.Second),
	pageviewer.WithTraceID("req-123"),
	pageviewer.WithRemoveInvisibleDiv(true),
)
```

## CLI 参数到配置的映射

CLI 当前支持的配置映射如下：

- `--proxy` -> `pageviewer.Config.Proxy`
- `--no-headless` -> `pageviewer.Config.NoHeadless`
- `--devtools` -> `pageviewer.Config.DevTools`
- `--wait-timeout` -> `pageviewer.WithWaitTimeout`
- `--trace-id` -> `pageviewer.WithTraceID`
- `--remove-invisible-div` -> `pageviewer.WithRemoveInvisibleDiv`
- `--acquire-timeout` -> `pageviewer.WithAcquireTimeout`

额外规则：

- 如果启用 `--json` 且传入多个 `--mode`，CLI 会自动把 `PoolSize` 和 `Warmup` 提升到 mode 数量，用于并发抓取多个结果

## 排障建议

- 需要关联上层请求时，优先传入 `--trace-id` 或 `WithTraceID`
- 如果页面渲染慢，可先提高 `--wait-timeout`
- 如果资源借用冲突，可调整 `AcquireTimeout` 或 `PoolSize`
