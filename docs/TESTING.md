# 测试说明

## 目标

本项目的测试分成两类：

- 库层测试：验证浏览器管理、worker 池、正文提取和 trace 逻辑
- CLI 测试：验证参数解析、模式分发、错误处理和输出契约

## 常用命令

### 运行 CLI 包测试

```bash
GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -count=1
```

### 运行特定 CLI 测试

参数解析：

```bash
GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -run 'TestParseFlagsRequiresURL|TestParseFlagsDefaultsModeToHTML|TestParseFlagsDefaultsModeToHTMLForJSON|TestParseFlagsNormalizesBareURLToHTTPS|TestParseFlagsRejectsInvalidURL|TestParseFlagsRejectsInvalidMode|TestParseFlagsAllowsRepeatedModesWithJSON|TestParseFlagsRejectsMultipleModesWithoutJSON|TestParseFlagsRejectsDuplicateModes|TestParseFlagsParsesCommonOptions' -count=1
```

错误处理：

```bash
GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -run 'TestRunCLINormalizesBareURLBeforeRequest|TestRunCLIPrintsTraceIDOnFetchError|TestRunCLIJSONFetchErrorStillWritesStderrOnly|TestRunCLIReturnsTwoOnParameterError|TestRunCLIReturnsTwoOnInvalidURL|TestRunCLIReturnsTwoOnMultipleModesWithoutJSON' -count=1
```

### 运行全仓测试

```bash
GOTOOLCHAIN=go1.24.2 go test ./... -count=1
```

### 运行取消语义回归测试

```bash
GOTOOLCHAIN=go1.24.2 go test ./... -run TestClientRawText_ContextDeadlineCancelsBlockedNavigation -count=1
```

当前默认全仓测试应保持在约 `30s` 内；如果明显回升，优先检查是否重新引入了重复浏览器冷启动或新的固定等待。

如果只想确认编译和测试发现阶段是否正常，而不跑完整行为测试：

```bash
GOTOOLCHAIN=go1.24.2 go test ./... -run '^$' -count=1
```

## CLI 手工验证

HTML：

```bash
go run ./cmd/pageviewer --url https://example.com
```

自动补全 scheme：

```bash
go run ./cmd/pageviewer --url ip.bmh.im
```

正文 JSON：

```bash
go run ./cmd/pageviewer --url https://example.com --mode article --json
```

文本响应 JSON：

```bash
go run ./cmd/pageviewer --url https://example.com --mode raw-text --json
```

多模式 JSON：

```bash
go run ./cmd/pageviewer --url https://example.com --json --mode html --mode article
```

排障链路：

```bash
go run ./cmd/pageviewer --url https://example.com --mode html --trace-id chat-20260318-001
```

## 测试约定

- 浏览器集成测试的入口页面和静态资源都应通过 `net/http/httptest` 提供，不依赖真实外网
- 涉及取消语义的回归测试，应优先通过本地 `httptest` 构造“主文档响应已开始但永不完成”的场景，避免依赖外网和浏览器偶发现象
- 常规浏览器测试默认应复用共享浏览器或显式关闭 leakless，避免被外部进程占用的全局锁端口拖慢
- 仅 leakless 专项回归测试依赖 upstream 默认锁端口 `127.0.0.1:2978`；如果该端口已被外部进程占用，测试应快速跳过而不是长时间阻塞
- 非生命周期测试优先复用测试夹具中的共享浏览器和共享 browser-backed client，避免重复拉起 Chromium
- 测试进程会主动收紧页面稳定等待 cap，新增本地页面测试不要再无差别依赖 `20s` 级等待
- 新增 CLI 行为时，优先为 `cmd/pageviewer/app_test.go` 补测试
- 成功结果只允许写标准输出
- 错误结果只允许写标准错误
- `--json` 只改变成功结果格式，不改变错误输出策略
- `--json` 下允许重复传入 `--mode`，非 JSON 下多个 `--mode` 必须返回参数错误
- 退出码约定：
  - `0`：成功
  - `1`：抓取、启动或关闭失败
  - `2`：参数错误
