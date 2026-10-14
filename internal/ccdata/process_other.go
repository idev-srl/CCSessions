//go:build !windows

package ccdata

import (
	"os"
	"syscall"
)

func isProcessAlive(pid int, _ string) bool {
	p, err := os.FindProcess(pid)
	return err == nil && p.Signal(syscall.Signal(0)) == nil
}

// RunningClaudeProcesses is only implemented on Windows.
func RunningClaudeProcesses() []int { return nil }
