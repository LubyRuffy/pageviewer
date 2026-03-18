package pageviewer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	browserCleanupHelperEnv        = "PAGEVIEWER_BROWSER_CLEANUP_HELPER"
	browserCleanupUserDataDirEnv   = "PAGEVIEWER_BROWSER_CLEANUP_USER_DATA_DIR"
	browserCleanupWaitTimeout      = 10 * time.Second
	browserCleanupWaitPollInterval = 100 * time.Millisecond
)

type processMatch struct {
	PID     string
	Command string
}

func TestNewBrowser_ChildExitDoesNotLeakChromium(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 下未实现基于 ps 的进程命令行断言")
	}

	if os.Getenv(browserCleanupHelperEnv) == "1" {
		require.NoError(t, runBrowserCleanupHelper(os.Getenv(browserCleanupUserDataDirEnv), false))
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
	)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	var (
		remaining []processMatch
		findErr   error
	)
	assert.Eventually(t, func() bool {
		remaining, findErr = findProcessesWithArg(userDataDir)
		return findErr == nil && len(remaining) == 0
	}, browserCleanupWaitTimeout, browserCleanupWaitPollInterval, "child 退出后仍有 Chromium 残留: err=%v, output=%s, remaining=%v", findErr, string(output), remaining)
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

	browser, err := NewBrowser(WithUserDataDir(userDataDir))
	require.NoError(t, err)

	_, err = browser.HTML(server.URL, NewVisitOptions(WithWaitTimeout(10*time.Second)).PageOptions)
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

func runBrowserCleanupHelper(userDataDir string, closeBrowser bool) error {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div>ok</div></body></html>`))
	}))
	defer server.Close()

	browser, err := NewBrowser(WithUserDataDir(userDataDir))
	if err != nil {
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
