# Changelog

## [Unreleased]

### Added

- 新增 `cmd/pageviewer` 命令行工具，支持 `html`、`links`、`article`、`raw-text` 四种模式
- 新增 `--json`、`--trace-id`、`--wait-timeout`、`--acquire-timeout`、`--remove-invisible-div`、`--proxy`、`--no-headless`、`--devtools` 参数
- 新增 CLI 使用文档 [`docs/CLI.md`](docs/CLI.md)
- 新增配置说明 [`docs/CONFIG.md`](docs/CONFIG.md)
- 新增开发者架构文档 [`ARCHITECTURE.md`](ARCHITECTURE.md)
- 新增开发代理说明 [`AGENTS.md`](AGENTS.md)
- 新增测试说明 [`docs/TESTING.md`](docs/TESTING.md)

### Changed

- 更新 [`README.md`](README.md)，补充 CLI 快速启动、使用示例和常见使用方式
- 明确 CLI 的退出码和排障链路文档
- `--json` 输出统一为 `modes + url + results` 结构，并支持重复传入 `--mode` 一次获取多个结果
- `cmd/pageviewer` 的 `--mode` 改为默认 `html`，不再要求显式传入单模式
- `raw-text` 模式调整为只放行主文档请求，默认阻断图片、样式、字体、脚本和其他子资源加载
- 默认测试夹具改为复用共享浏览器，并在测试进程里下调本地页面等待上限，将 `go test ./... -count=1` 压到约 `30s`

### Fixed

- 固化 CLI 错误输出契约：错误统一写入标准错误，`--json` 不改变错误输出路径
- 修复 `--help` / `-h` 行为，改为输出完整帮助并以退出码 `0` 返回
- `--url` 现在会在请求前先规范化和校验：无 scheme 时默认补 `https://`，明显非法输入直接在参数阶段报错
- 修复浏览器测试中的外链资源依赖，改为由本地 `httptest` 提供，避免测试期间额外访问公网资源
- 修复主文档响应完成事件缺失时导航链路不响应调用方 `ctx` 取消的问题，避免 `RawText` 等请求偶发无限挂起
- 修复测试套件对全局 leakless 锁端口的隐式依赖：常规浏览器测试不再无关地走 leakless，leakless 专项回归在外部进程占用 `127.0.0.1:2978` 时会快速跳过，避免全仓测试长期阻塞
