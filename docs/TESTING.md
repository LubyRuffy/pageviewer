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
GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -run 'TestParseFlagsRequiresURLAndMode|TestParseFlagsRejectsInvalidMode|TestParseFlagsAllowsRepeatedModesWithJSON|TestParseFlagsRejectsMultipleModesWithoutJSON|TestParseFlagsRejectsDuplicateModes|TestParseFlagsParsesCommonOptions' -count=1
```

错误处理：

```bash
GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -run 'TestRunCLIPrintsTraceIDOnFetchError|TestRunCLIJSONFetchErrorStillWritesStderrOnly|TestRunCLIReturnsTwoOnParameterError|TestRunCLIReturnsTwoOnMultipleModesWithoutJSON' -count=1
```

### 运行全仓测试

```bash
GOTOOLCHAIN=go1.24.2 go test ./... -count=1
```

如果只想确认编译和测试发现阶段是否正常，而不跑完整行为测试：

```bash
GOTOOLCHAIN=go1.24.2 go test ./... -run '^$' -count=1
```

## CLI 手工验证

HTML：

```bash
go run ./cmd/pageviewer --url https://example.com --mode html
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

- 新增 CLI 行为时，优先为 `cmd/pageviewer/app_test.go` 补测试
- 成功结果只允许写标准输出
- 错误结果只允许写标准错误
- `--json` 只改变成功结果格式，不改变错误输出策略
- `--json` 下允许重复传入 `--mode`，非 JSON 下多个 `--mode` 必须返回参数错误
- 退出码约定：
  - `0`：成功
  - `1`：抓取、启动或关闭失败
  - `2`：参数错误
