# Browser Process Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修复 `pageviewer.NewBrowser()` 默认关闭 leakless 导致的 Chromium 残留问题，并让 `Browser.Close()` 对自身拉起的浏览器进程树提供兜底回收。

**Architecture:** 保持 `NewBrowser` 继续封装 rod/launcher，但默认恢复 leakless，并把 launcher 元信息保存在 `Browser` 上。`Close()` 先尝试优雅关闭，再在超时后 kill 自己启动的进程组。测试通过子进程退出和显式 `Close()` 两条路径覆盖回归。

**Tech Stack:** Go 1.24, rod launcher, Chromium child-process inspection, testify

---

### Task 1: 写出默认 leakless 的失败回归测试

**Files:**
- Create: `browser_process_cleanup_test.go`
- Modify: `browser_option.go`

**Step 1: Write the failing test**

新增子进程测试，让 helper 进程创建 browser、访问本地 `httptest` 页面后直接退出且不调用 `Close()`；父进程断言带唯一 `user-data-dir` 标记的 Chromium 进程最终不应残留。

**Step 2: Run test to verify it fails**

Run: `go test -run TestNewBrowser_ChildExitDoesNotLeakChromium ./...`
Expected: FAIL，因为默认 `Leakless(false)` 时 child 退出后仍会留下 rod 拉起的 Chromium 进程。

### Task 2: 实现默认 leakless 与 Close 兜底清理

**Files:**
- Modify: `browser.go`
- Modify: `browser_option.go`
- Create: `process_tree_unix.go`
- Create: `process_tree_windows.go`

**Step 1: Write minimal implementation**

新增显式 `WithLeakless(bool)` 选项，`NewBrowser` 默认启用 leakless，并将 launcher/配置保存到 `Browser`。`Close()` 对自身启动的浏览器进程树执行“优雅关闭 + 超时 kill”的兜底清理。

**Step 2: Run focused tests**

Run: `go test -run 'TestNewBrowser_ChildExitDoesNotLeakChromium|TestBrowser_CloseStopsManagedChromium|TestNewBrowser_WithLeaklessOption' ./...`
Expected: PASS

### Task 3: 补充显式 Close 回归测试并完成验证

**Files:**
- Modify: `browser_process_cleanup_test.go`

**Step 1: Add Close regression**

新增本地 `httptest` 用例：创建 browser、执行一次最小访问、调用 `Close()`，断言带唯一 `user-data-dir` 标记的 Chromium 进程最终全部退出。

**Step 2: Run full verification**

Run: `go test ./...`
Expected: PASS，且回归测试不再检测到残留的 rod Chromium 进程。
