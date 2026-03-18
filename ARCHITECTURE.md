# 架构说明

## 系统整体架构

`pageviewer` 由库层和 CLI 层两部分组成：

- 库层：提供浏览器管理、worker 池、页面访问、正文提取和排障能力
- CLI 层：位于 `cmd/pageviewer`，负责把命令行参数映射到库层调用，并把结果渲染到标准输出

整体关系：

```text
CLI args
-> cmd/pageviewer
-> pageviewer.Client
-> Browser + Page Worker Pool
-> rod / stealth / network response
-> stdout / stderr
```

## 核心模块

### `browser.go`

- 管理浏览器实例生命周期
- 负责页面创建、导航、稳定等待和正文抽取
- 提供 `HTML`、`Links`、`ReadabilityArticle`、`RawText` 等底层访问能力

### `client.go`

- 提供长驻 `Client`
- 统一封装浏览器、worker 池和请求调用
- 对外暴露 `HTML`、`Links`、`ReadabilityArticle`、`RawText`

### `pool.go`

- 管理 worker 借用和归还
- 控制并发访问页面的上限

### `trace.go`

- 记录最近请求的调试信息
- 通过 `WithTraceID` + `DebugTrace` 支持排障

### `cmd/pageviewer`

- `main.go`：CLI 入口
- `app.go`：参数解析、配置构造、模式分发、输出渲染

## CLI 内部结构

`cmd/pageviewer/app.go` 当前按四层职责组织：

1. `parseFlags`
   解析 `--url`、`--mode`、`--json`、`--wait-timeout` 等参数
2. `buildConfig`
   把 CLI 参数映射到 `pageviewer.Config` 和请求级 `RequestOption`
3. `runCLI`
   启动 `pageviewer.Client` 并按 `mode` 分发到库方法
4. `writeJSON` / `writeError` / `writeFetchError`
   负责把成功结果写到标准输出，把错误写到标准错误

## 请求 / 数据流

### 成功路径

1. 用户执行 `go run ./cmd/pageviewer --url ... --mode ...`
2. `parseFlags` 解析并校验参数
3. `buildConfig` 生成浏览器级配置和请求级选项
4. `runCLI` 调用 `pageviewer.Start(...)`
5. 根据 `mode` 选择 `HTML`、`Links`、`ReadabilityArticle` 或 `RawText`
6. 默认模式把主要内容写到标准输出
7. 如果启用 `--json`，统一用 `json.Encoder` 输出结构化结果

### 失败路径

1. 参数错误：直接写 `stderr`，返回退出码 `2`
2. 抓取错误：写 `stderr`，返回退出码 `1`
3. 如果存在 `--trace-id`，抓取失败时额外输出 `trace_id=<value>`

## 一键排障链路

CLI 层只做最小排障透传：

- 接收 `--trace-id`
- 在调用库层时传入 `pageviewer.WithTraceID`
- 错误时把 `trace_id=<value>` 输出到 `stderr`

更完整的 trace 详情仍应由上层使用库接口的 `DebugTrace` 拉取。
