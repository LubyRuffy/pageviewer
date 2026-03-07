# pageviewer 长驻共享 Browser 包设计

## 背景

当前仓库已经具备基础网页渲染、正文提取和链接提取能力，但整体形态仍偏向“一次性调用”：

- 入口以包级 `Visit` 和默认浏览器为主
- 生命周期不显式，长驻复用能力不足
- 测试虽然覆盖率不低，但过度依赖真实浏览器和外网环境
- 生产排障链路、错误模型和可观测性不足

目标是把项目调整成一个可独立复用的 Go package，供上层业务系统长期持有和复用。上层启动一次 `Start` 后，包内维护长驻实例；进程退出时统一 `Close`。

## 目标

- 提供显式 `Start` / `Close` 生命周期
- 以包内 worker 池形式复用共享浏览器会话
- 默认共享 `cookie/localStorage/登录态`
- 池满时按超时等待，超时后返回可判定错误
- 单个 worker 损坏时当前请求失败，但后台自动修复，后续请求恢复
- 支持稳定的文本原始返回能力，覆盖 `html/json/xml/text`
- 为生产使用补齐一键排障链路和基础可观测性

## 非目标

- 不支持二进制原始返回
- 不做多 Browser 进程池
- 不默认做多租户会话隔离
- 不在本阶段引入远程 Browser Daemon 模式

## 方案选择

### 方案 1：单 Browser 进程 + Page Worker Pool

单个长驻 Browser，共享同一个 profile，包内预热多个 page worker，请求时从池中借用 page，执行完成后归还。

优点：

- 完全符合共享登录态目标
- 启动成本和资源占用较低
- 故障恢复粒度细，可以只重建 page
- API 清晰，最适合作为独立 package 对外暴露

缺点：

- Browser 进程级故障会影响全部在途请求

### 方案 2：多 Browser 实例池 + 手工同步会话

每个 worker 自带独立 Browser，通过同步 cookie 等方式尽量共享会话。

优点：

- 隔离性更强

缺点：

- `localStorage/sessionStorage` 无法稳定同步
- 复杂度和维护成本明显高于收益

### 方案 3：外置 Browser Daemon + 包内连接管理

由外部进程托管 Browser，package 只负责连接和调度。

优点：

- 运维和调试更灵活

缺点：

- 破坏“独立可复用 package”的自包含目标

## 选择结果

采用方案 1：单 Browser 进程 + Page Worker Pool。

这里的“池”明确指 page worker 池，而不是 browser 进程池。这样可以在共享登录态的前提下，实现稳定的并发复用和简化后的生命周期管理。

## 架构设计

### 核心对象

- `Client`
  - 对外主入口
  - 负责 `Start`、`Close`、`Stats`
  - 持有 browser、pool、supervisor、trace recorder
- `worker`
  - 持有一个长期复用的 `rod.Page`
  - 负责单次任务执行前后的清理
- `pool`
  - 负责 worker 借还、超时等待、池容量控制
- `supervisor`
  - 负责 page/browser 故障恢复和补建
- `trace recorder`
  - 记录最近请求的执行链路，支持按 `TraceID` 排障

### 生命周期

1. `Start(ctx, Config)` 启动 Browser
2. 按配置预热固定数量的 page worker
3. 请求进入后从池中借用 worker
4. 执行 DOM 模式或 Text 模式
5. 成功则归还 worker
6. worker 损坏则摘除并后台补建
7. `Close()` 停止接收新请求，等待在途任务收拢后统一关闭 page 和 browser

## 对外 API 设计

### 启动与关闭

```go
func Start(ctx context.Context, cfg Config) (*Client, error)
func (c *Client) Close() error
func (c *Client) Stats() Stats
func (c *Client) DebugTrace(id string) (Trace, bool)
```

### 请求入口

```go
func (c *Client) Visit(ctx context.Context, url string, fn func(page *rod.Page) error, opts ...RequestOption) error
func (c *Client) HTML(ctx context.Context, url string, opts ...RequestOption) (string, error)
func (c *Client) RawText(ctx context.Context, url string, opts ...RequestOption) (TextResponse, error)
func (c *Client) Links(ctx context.Context, url string, opts ...RequestOption) (string, error)
func (c *Client) ReadabilityArticle(ctx context.Context, url string, opts ...RequestOption) (ReadabilityArticleWithMarkdown, error)
```

### 兼容策略

- 现有包级 `Visit` 可以短期保留，作为便捷接口
- 生产用法应迁移到 `Start` 返回的 `Client`
- 文档中明确说明：包级接口适合临时调用，不适合作为服务主链路

## 配置设计

### 启动配置 `Config`

- `PoolSize int`
- `AcquireTimeout time.Duration`
- `UserDataDir string`
- `HealthCheckInterval time.Duration`
- `Warmup int`
- `BrowserOptions BrowserLaunchOptions`
- `Logger Logger`
- `OnEvent func(Event)`

说明：

- `PoolSize` 表示 page worker 数量
- `UserDataDir` 非空时允许保留共享登录态
- `Warmup` 控制启动时预建 worker 数量
- `AcquireTimeout` 作为全局默认借用等待时长

### 单次请求配置 `RequestOptions`

- `WaitTimeout time.Duration`
- `BeforeRequest func(page *rod.Page) error`
- `RemoveInvisibleDiv bool`
- `AcquireTimeout time.Duration`
- `TraceID string`

说明：

- 单次请求允许覆盖等待超时和借用超时
- `TraceID` 可由上层传入；为空时由包内生成

## 并发与池行为

### 借用规则

实际等待时间取以下三者最小值：

- `ctx` 的 deadline
- `RequestOptions.AcquireTimeout`
- `Config.AcquireTimeout`

若超时则返回 `ErrAcquireTimeout`。

### 会话共享

所有 worker 都来自同一个 Browser 和同一个 profile：

- 共享 `cookie`
- 共享 `localStorage`
- 共享登录态

不支持隔离式多租户会话。

## 执行模式设计

### DOM 模式

适用于：

- `Visit`
- `HTML`
- `Links`
- `ReadabilityArticle`

流程：

1. 借 worker
2. 执行 `BeforeRequest`
3. 导航到目标 URL
4. 等待页面 load / idle / dom stable
5. 读取 DOM 或执行提取逻辑
6. 归还或重建 worker

### Text 模式

适用于：

- `RawText`

行为边界：

- 支持 `text/*`
- 支持 `application/json`
- 支持 `application/xml`
- 支持 `text/xml`
- 非文本类型返回 `ErrUnsupportedContentType`

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

实现要求：

- 只抓取主文档响应
- 不再使用“第一个完成的网络响应体”作为原始返回
- 不支持二进制数据自动转文本

## 错误模型

定义可判定错误，供上层业务和重试逻辑使用：

- `ErrClosed`
- `ErrAcquireTimeout`
- `ErrBrowserUnavailable`
- `ErrNavigationFailed`
- `ErrUnsupportedContentType`
- `ErrWorkerBroken`

错误原则：

- 当前请求失败时应尽量返回稳定错误类型
- 不依赖字符串匹配判断错误原因

## 故障恢复策略

### worker 级异常

- 当前请求返回错误
- 受损 worker 不归还池
- 后台异步补建新 worker
- 后续请求恢复

### browser 级异常

- 当前请求返回错误
- supervisor 重建 browser 和整个 worker 池
- 后续请求恢复

## 一键排障链路

每次请求都要有 `TraceID`，并能通过 id 还原最小必要链路。

`Trace` 至少记录：

- `TraceID`
- URL
- 模式（DOM/Text）
- worker id
- browser generation
- 借 worker 等待时长
- 导航状态码
- `Content-Type`
- `FinalURL`
- `load / idle / dom stable` 耗时
- 最终错误类型和消息
- 是否发生 worker/browser 重建

这满足“给一次交互 id，后端快速还原整个流程”的排障要求。

## 测试策略

### 单元测试

覆盖：

- `Config` 和 `RequestOptions`
- 错误分类
- 池满超时
- `Close` 幂等
- trace 记录和查询

### 集成测试

全部基于 `httptest`：

- HTML 页面
- JSON 页面
- XML 页面
- text/plain 页面
- redirect
- 共享登录态
- worker 重建
- browser 恢复后的后续成功请求

### 约束

- 默认测试不得依赖外网站点
- 外网 smoke test 必须显式开关，例如 `E2E=1`
- 所有新增和修改函数都应补充单测

## 生产准入标准

- `go1.24.2` 下 `go build ./...` 通过
- `go test ./...` 通过
- `go test ./... -race` 通过
- 默认测试不访问外网
- 核心 package 覆盖率不少于 `85%`
- `Start` / `Close` 幂等
- 压测后无 page/browser/goroutine 明显泄漏
- 文本原始返回仅接受声明支持的文本类型

## 迁移建议

第一阶段：

- 引入新的 `Client` 生命周期对象
- 保留现有 API 兼容上层调用

第二阶段：

- 将现有 DOM 抓取能力迁移到 `Client`
- 新增 `RawText`
- 建立 trace 和错误模型

第三阶段：

- 清理默认外网依赖测试
- 建立稳定的集成测试和生产准入基线

