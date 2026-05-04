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

// setupProcessGroup sets the child process to run in its own process group
// so that killing the group on timeout also kills all subprocesses (tree-kill).
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// killProcessGroup kills the entire process group (tree-kill).
// On Unix, sending SIGKILL to -pid kills the process group.
func killProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
