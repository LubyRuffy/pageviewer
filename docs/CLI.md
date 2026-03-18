# CLI 使用说明

## 概览

`pageviewer` 提供一个单命令 CLI，用来快速验证网页渲染结果、排查抓取问题和做简单脚本集成。

命令入口：

```bash
go run ./cmd/pageviewer --url https://example.com
```

也可以先构建二进制：

```bash
go build -o bin/pageviewer ./cmd/pageviewer
./bin/pageviewer --url https://example.com
```

查看帮助：

```bash
go run ./cmd/pageviewer --help
```

## 参数

必填参数：

- `--url`：目标 URL

可选参数：

- `--mode`：输出模式，支持 `html`、`links`、`article`、`raw-text`，默认 `html`
- `--json`：输出 JSON
- `--wait-timeout`：页面等待超时，例如 `15s`
- `--trace-id`：透传排障 ID
- `--remove-invisible-div`：请求时移除不可见 `div`
- `--acquire-timeout`：worker 借用超时，例如 `5s`
- `--proxy`：浏览器代理地址
- `--no-headless`：显示浏览器窗口
- `--devtools`：打开 DevTools
- `-h` / `--help`：显示帮助并退出

`--mode` 的规则：

- 默认不传时使用 `html`
- 非 JSON 场景最多只能得到一个 mode 结果
- 启用 `--json` 后允许重复传入 `--mode`
- 重复传入相同 mode 会报参数错误
- 不启用 `--json` 时，如果传了多个 `--mode`，会报参数错误

## 输出模式

### `html`

默认输出渲染后的完整 HTML：

```bash
go run ./cmd/pageviewer --url https://example.com
```

`--json` 输出：

```json
{
  "modes": ["html"],
  "url": "https://example.com",
  "results": {
    "html": {
      "content": "<html>...</html>"
    }
  }
}
```

### `links`

默认输出页面中的文本链接结果：

```bash
go run ./cmd/pageviewer --url https://example.com --mode links
```

`--json` 输出：

```json
{
  "modes": ["links"],
  "url": "https://example.com",
  "results": {
    "links": {
      "content": "https://example.com"
    }
  }
}
```

### `article`

默认输出正文 Markdown：

```bash
go run ./cmd/pageviewer --url https://example.com/article --mode article
```

`--json` 输出会返回 `modes`、`url` 以及 `results.article` 下的 `ReadabilityArticleWithMarkdown` 字段，例如：

```json
{
  "modes": ["article"],
  "url": "https://example.com/article",
  "results": {
    "article": {
      "title": "Example",
      "markdown": "# Example",
      "html": "<article>...</article>",
      "raw_html": "<html>...</html>"
    }
  }
}
```

### `raw-text`

默认输出主文档响应正文：

```bash
go run ./cmd/pageviewer --url https://example.com/api --mode raw-text
```

`--json` 输出会附带响应元数据：

```json
{
  "modes": ["raw-text"],
  "url": "https://example.com/api",
  "results": {
    "raw-text": {
      "body": "{\"ok\":true}",
      "content_type": "application/json",
      "status_code": 200,
      "final_url": "https://example.com/api",
      "header": {
        "Content-Type": [
          "application/json"
        ]
      }
    }
  }
}
```

## JSON 多模式

启用 `--json` 后，可以重复传入 `--mode`，一次拿到多个结果：

```bash
go run ./cmd/pageviewer \
  --url https://example.com \
  --json \
  --mode html \
  --mode article
```

返回结构：

```json
{
  "modes": ["html", "article"],
  "url": "https://example.com",
  "results": {
    "html": {
      "content": "<html>...</html>"
    },
    "article": {
      "title": "Example",
      "markdown": "# Example"
    }
  }
}
```

这个结构的设计目的是方便脚本直接按 `results.html`、`results.article` 做对比分析。

## 退出码

- `0`：成功
- `1`：抓取失败、启动 `Client` 失败或关闭 `Client` 失败
- `2`：参数错误

常见参数错误包括：

- 缺少 `--url`
- 非 JSON 场景下传入多个 `--mode`
- 传入重复的 `--mode`
- 传入不支持的 mode 值

## 排障示例

如果上层已经有交互 ID，可以直接传入：

```bash
go run ./cmd/pageviewer \
  --url https://example.com \
  --mode html \
  --trace-id chat-20260318-001
```

抓取失败时：

- 错误信息写入标准错误
- 如果提供了 `--trace-id`，会额外输出一行 `trace_id=<value>`
- 即使加了 `--json`，错误仍然不会写入标准输出

示例：

```text
navigation failed
trace_id=chat-20260318-001
```
