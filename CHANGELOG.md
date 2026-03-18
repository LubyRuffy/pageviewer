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

### Fixed

- 固化 CLI 错误输出契约：错误统一写入标准错误，`--json` 不改变错误输出路径
- 修复 `--help` / `-h` 行为，改为输出完整帮助并以退出码 `0` 返回
