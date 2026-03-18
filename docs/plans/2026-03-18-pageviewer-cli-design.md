# Pageviewer CLI Design

## 背景

当前仓库提供 `pageviewer` Go 库，支持渲染式访问网页并提取 HTML、链接、正文和文本响应，但没有面向终端用户的命令行入口。为便于手动排查、脚本调用和快速验证，需要新增 `cmd/pageviewer` 命令行工具。

本次设计的目标是增加一个单命令 CLI：输入 URL 和对应命令行参数，按指定模式输出网页访问结果，并兼顾人类阅读和程序消费。

## 目标

- 提供 `cmd/pageviewer` 命令行入口
- 使用 `--mode` 单选输出模式
- 默认输出纯内容，`--json` 时输出结构化结果
- 首版支持常用参数：`--wait-timeout`、`--trace-id`、`--remove-invisible-div`、`--acquire-timeout`、`--proxy`、`--no-headless`、`--devtools`
- 与现有 `pageviewer.Client` 能力直接对齐，不修改底层公开 API

## 非目标

- 不在首版引入 `cobra` 或其他 CLI 框架
- 不新增多子命令结构
- 不在首版输出完整 `DebugTrace` 详情
- 不在首版暴露全部底层浏览器配置

## 设计决策

### 方案比较

#### 方案 A：单命令 + 标准库 `flag`

- 形式：`pageviewer --url https://example.com --mode html`
- 优点：依赖最少、实现简单、与当前仓库体量匹配
- 缺点：后续如果扩很多子命令，参数管理会逐渐变复杂

#### 方案 B：单命令 + `cobra`

- 优点：帮助信息、参数组织和未来扩展更标准
- 缺点：当前需求只有单命令，框架引入成本偏高

#### 方案 C：多子命令

- 形式：`pageviewer html URL`
- 优点：命令语义直观
- 缺点：与已确认的 `--mode` 单选模型不一致

### 选择

选择方案 A。首版使用标准库 `flag` 实现单命令 CLI，并在内部按“参数解析、配置映射、执行分发、结果输出”分层，保留后续平滑升级为更完整 CLI 框架的空间。

## 命令行契约

### 命令形式

```bash
pageviewer --url https://example.com --mode html
```

### 参数

必填参数：

- `--url`
- `--mode`

`--mode` 支持枚举值：

- `html`
- `links`
- `article`
- `raw-text`

可选参数：

- `--json`
- `--wait-timeout`
- `--trace-id`
- `--remove-invisible-div`
- `--acquire-timeout`
- `--proxy`
- `--no-headless`
- `--devtools`
- `--help`

### 参数映射

浏览器级参数映射到 `pageviewer.Config`：

- `--proxy`
- `--no-headless`
- `--devtools`

请求级参数映射到 `pageviewer.RequestOption`：

- `--wait-timeout`
- `--trace-id`
- `--remove-invisible-div`
- `--acquire-timeout`

## 输出契约

### 默认输出

默认模式只向标准输出打印主要结果，适合人工查看和 shell 管道。

- `html`：输出渲染后的完整 HTML
- `links`：输出链接文本结果
- `article`：输出 `Markdown`
- `raw-text`：输出 `TextResponse.Body`

### JSON 输出

启用 `--json` 后，输出结构化 JSON，适合程序消费。

- `html` / `links`：统一返回 `mode`、`url`、`content`
- `article`：返回 `mode`、`url` 以及 `ReadabilityArticleWithMarkdown` 字段
- `raw-text`：返回 `mode`、`url`、`body`、`content_type`、`status_code`、`final_url`、`header`

## 模块划分

建议在 `cmd/pageviewer` 内部拆成四层职责：

1. `parseFlags`
   负责解析和校验参数，生成 `cliOptions`
2. `buildConfig`
   负责将 `cliOptions` 映射成 `pageviewer.Config` 和请求选项
3. `run`
   负责创建 `pageviewer.Client` 并按 `mode` 调用对应库方法
4. `renderOutput`
   负责输出纯文本或 JSON

调用链：

```text
main
-> parseFlags
-> buildConfig
-> run
-> renderOutput
```

## 数据流

1. 用户执行 `pageviewer --url ... --mode ...`
2. CLI 解析参数并校验
3. CLI 基于参数构造 `pageviewer.Config` 和请求选项
4. CLI 启动 `pageviewer.Client`
5. CLI 根据 `mode` 调用 `HTML`、`Links`、`ReadabilityArticle` 或 `RawText`
6. CLI 按输出模式写入标准输出
7. 如果出错，错误写入标准错误并返回非零退出码

## 错误处理

- 参数错误：立即返回明确错误信息，例如 `--url is required`
- 非法模式：返回 `invalid --mode`
- duration 解析失败：直接返回原始解析错误
- 抓取失败：透传底层错误
- 如果用户提供 `--trace-id`，失败时在标准错误补充 `trace_id`
- `--json` 仅影响成功结果输出格式，错误仍走标准错误和非零退出码

## 一键排障链路

首版保持最小实现：

- 支持透传 `--trace-id`
- 调用底层请求时设置 `pageviewer.WithTraceID`
- 请求失败时在标准错误提示 `trace_id`
- 暂不输出完整 `DebugTrace` 详情

## 测试策略

### 参数解析测试

覆盖：

- 缺少 `--url`
- 缺少 `--mode`
- 非法 `--mode`
- 非法 `--wait-timeout`
- 非法 `--acquire-timeout`
- `--json` 和布尔参数组合

### 输出测试

覆盖四种模式在两种输出形态下的行为：

- 默认输出是否只打印主要内容
- `--json` 是否输出稳定字段
- `article` 默认是否输出 `Markdown`
- `raw-text` 默认是否输出 `Body`

### 分发测试

通过可替换执行层验证：

- `html` 分发到 `client.HTML`
- `links` 分发到 `client.Links`
- `article` 分发到 `client.ReadabilityArticle`
- `raw-text` 分发到 `client.RawText`
- 错误是否正确传递到 CLI 退出路径

## 文档影响

本次实现需要同步更新或新增以下文档：

- `README.md`
- `docs/CLI.md`
- `ARCHITECTURE.md`
- `docs/TESTING.md`
- `CHANGELOG.md`

## 验收标准

- 能通过 `go run ./cmd/pageviewer --url <url> --mode <mode>` 正常执行
- 默认输出可直接用于终端查看
- `--json` 输出可被 `jq` 等工具稳定消费
- 参数非法时返回明确错误和非零退出码
- 新增 CLI 代码具备单元测试覆盖
- 文档和变更记录与实现保持一致
