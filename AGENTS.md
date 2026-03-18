# AGENTS

## 项目开发规则

- 默认开发语言为 Go
- 修改 Go 代码后必须确保可以编译
- 新增和修改的函数需要补充或更新单元测试
- 文档视为实现的一部分，代码行为变化时必须同步更新 README、开发文档和 CHANGELOG
- 排障链路优先复用 `trace_id` / `WithTraceID`
- 不要让单个文件无限膨胀；复杂逻辑优先拆成可测试的小函数

## 构建、运行、测试命令

构建 CLI：

```bash
go build -o bin/pageviewer ./cmd/pageviewer
```

运行 CLI：

```bash
go run ./cmd/pageviewer --url https://example.com --mode html
```

运行 CLI 测试：

```bash
GOTOOLCHAIN=go1.24.2 go test ./cmd/pageviewer -count=1
```

运行全仓测试：

```bash
GOTOOLCHAIN=go1.24.2 go test ./... -count=1
```

## 项目开发约定

- CLI 核心逻辑位于 `cmd/pageviewer`，保持参数解析、模式分发和输出渲染职责清晰
- 成功结果只写标准输出，错误只写标准错误
- `--json` 只改变成功输出格式，不改变错误输出策略
- 退出码约定：
  - `0`：成功
  - `1`：抓取、启动或关闭失败
  - `2`：参数错误
- 使用者文档放在 `README.md`、`docs/CLI.md`、`docs/CONFIG.md`
- 开发者文档放在 `AGENTS.md`、`ARCHITECTURE.md`、`docs/TESTING.md`
