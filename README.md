# pageviewer

实现渲染的网页浏览。

## 特性
- [x] 支持渲染
- [x] 默认支持浏览器识别绕过
- [x] 支持渲染等待超时后，如果页面已经正常渲染，可以返回页面内容，而不是报错
  - 默认情况下调用WaitStable后有可能已经渲染成功，但是后台还在执行任务，导致报错
  - 这里涉及到不同的事件，比较有难度，最终的方案是：模拟了WaitDOMStable，但是时间缩短
- [x] 可以选择复用已有浏览器，确保cookie共享，避免登录限制问题
- [x] 支持长驻 `Client` 和共享 page worker 池
- [x] 支持请求前的回调
- [x] 支持删除不显示的内容，减少返回大小，方便做ai agent
- [x] 支持文本主文档原始返回 `RawText`
- [x] 支持通过 `TraceID` 做最近请求排障

## 文档

- 开发文档见 [docs/shared-browser-pool-development.md](docs/shared-browser-pool-development.md)

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
