package pageviewer

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	browserCleanupHelperEnv        = "PAGEVIEWER_BROWSER_CLEANUP_HELPER"
	browserCleanupUserDataDirEnv   = "PAGEVIEWER_BROWSER_CLEANUP_USER_DATA_DIR"
	browserCleanupPIDFileEnv       = "PAGEVIEWER_BROWSER_CLEANUP_PID_FILE"
	browserCleanupWaitTimeout      = 10 * time.Second
	browserCleanupWaitPollInterval = 100 * time.Millisecond
	leaklessDefaultLockPort        = 2978
)

type processMatch struct {
	PID     string
	Command string
}

func TestNewBrowser_ChildExitDoesNotLeakChromium(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 下未实现基于 ps 的进程命令行断言")
	}
	skipIfLeaklessLockPortBusy(t)

	if os.Getenv(browserCleanupHelperEnv) == "1" {
		require.NoError(t, runBrowserCleanupHelper(
			os.Getenv(browserCleanupUserDataDirEnv),
			os.Getenv(browserCleanupPIDFileEnv),
			false,
		))
		return
	}

	userDataDir := t.TempDir()
	t.Cleanup(func() {
		killProcessesWithArg(t, userDataDir)
	})

	cmd := exec.Command(os.Args[0], "-test.run", "^TestNewBrowser_ChildExitDoesNotLeakChromium$")
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("%s=1", browserCleanupHelperEnv),
		fmt.Sprintf("%s=%s", browserCleanupUserDataDirEnv, userDataDir),
		fmt.Sprintf("%s=%s", browserCleanupPIDFileEnv, filepath.Join(t.TempDir(), "browser.pid")),
	)

	pidFilePath := envValue(cmd.Env, browserCleanupPIDFileEnv)

	outputPath := filepath.Join(t.TempDir(), "child-output.log")
	outputFile, err := os.Create(outputPath)
	require.NoError(t, err)

	cmd.Stdout = outputFile
	cmd.Stderr = outputFile
	err = cmd.Run()
	require.NoError(t, outputFile.Close())

	output, readErr := os.ReadFile(outputPath)
	require.NoError(t, readErr)
	require.NoError(t, err, string(output))

	browserPID, err := readPIDFile(pidFilePath)
	require.NoError(t, err)
	require.Positive(t, browserPID)

	var pidExistsErr error
	assert.Eventually(t, func() bool {
		exists, existsErr := processTreeExists(browserPID)
		pidExistsErr = existsErr
		if existsErr != nil {
			return false
		}
		return !exists
	}, browserCleanupWaitTimeout, browserCleanupWaitPollInterval, "child 退出后浏览器进程仍未退出: pid=%d, err=%v, output=%s", browserPID, pidExistsErr, string(output))
}

func skipIfLeaklessLockPortBusy(t *testing.T) {
	t.Helper()

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", leaklessDefaultLockPort))
	if err != nil {
		t.Skipf("跳过 leakless 子进程退出测试：默认锁端口 %d 被外部进程占用: %v", leaklessDefaultLockPort, err)
		return
	}
	require.NoError(t, listener.Close())
}

func TestBrowser_CloseStopsManagedChromium(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 下未实现基于 ps 的进程命令行断言")
	}

	userDataDir := t.TempDir()
	t.Cleanup(func() {
		killProcessesWithArg(t, userDataDir)
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div>ok</div></body></html>`))
	}))
	defer server.Close()

	browser, err := NewBrowser(WithUserDataDir(userDataDir), WithLeakless(false))
	require.NoError(t, err)

	po := newDefaultVisitOptions().PageOptions
	po.waitTimeout = 10 * time.Second

	_, err = browser.HTML(server.URL, po)
	require.NoError(t, err)
	require.NoError(t, browser.Close())

	var (
		remaining []processMatch
		findErr   error
	)
	assert.Eventually(t, func() bool {
		remaining, findErr = findProcessesWithArg(userDataDir)
		return findErr == nil && len(remaining) == 0
	}, browserCleanupWaitTimeout, browserCleanupWaitPollInterval, "Close 后仍有 Chromium 残留: err=%v, remaining=%v", findErr, remaining)
}

func TestNewBrowser_WithLeaklessOption(t *testing.T) {
	opts := &browserOptions{}
	WithLeakless(false)(opts)

	require.True(t, opts.LeaklessSet)
	require.False(t, opts.Leakless)
}

func runBrowserCleanupHelper(userDataDir, pidFilePath string, closeBrowser bool) error {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div>ok</div></body></html>`))
	}))
	defer server.Close()

	browser, err := NewBrowser(WithUserDataDir(userDataDir))
	if err != nil {
		return err
	}
	if err := writePIDFile(pidFilePath, browser.launcher.PID()); err != nil {
		return err
	}

	_, err = browser.HTML(server.URL, NewVisitOptions(WithWaitTimeout(10*time.Second)).PageOptions)
	if err != nil {
		return err
	}

	if closeBrowser {
		return browser.Close()
	}

	return nil
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func writePIDFile(path string, pid int) error {
	if path == "" {
		return nil
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
}

func readPIDFile(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(raw)))
}

func findProcessesWithArg(arg string) ([]processMatch, error) {
	cmd := exec.Command("ps", "-ax", "-o", "pid=,command=")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var matches []processMatch
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, arg) {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		matches = append(matches, processMatch{
			PID:     fields[0],
			Command: strings.Join(fields[1:], " "),
		})
	}

	return matches, nil
}

func killProcessesWithArg(t *testing.T, arg string) {
	t.Helper()

	matches, err := findProcessesWithArg(arg)
	if err != nil {
		t.Logf("查找残留进程失败: %v", err)
		return
	}

	for _, match := range matches {
		if killErr := exec.Command("kill", "-9", match.PID).Run(); killErr != nil {
			t.Logf("清理残留进程失败 pid=%s err=%v", match.PID, killErr)
		}
	}
}
