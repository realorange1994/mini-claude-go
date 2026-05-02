//go:build unix

package tools

import (
	"os/exec"
	"syscall"
)

// getSignalExitCode returns the signal-based exit code (128 + signal number)
// for processes terminated by a signal on Unix systems.
func getSignalExitCode(exitErr *exec.ExitError) int {
	if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
		if ws.Signaled() {
			return 128 + int(ws.Signal())
		}
	}
	return -1
}
