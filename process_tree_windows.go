//go:build windows

package pageviewer

import (
	"fmt"
	"os/exec"
	"strings"
)

func processTreeExists(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}

	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	line := strings.TrimSpace(string(output))
	if line == "" || strings.EqualFold(line, "INFO: No tasks are running which match the specified criteria.") {
		return false, nil
	}

	return true, nil
}
