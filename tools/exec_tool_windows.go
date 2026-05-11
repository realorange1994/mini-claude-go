//go:build windows

package tools

import (
	"fmt"
	"os/exec"
	"syscall"
)

// getSignalExitCode returns -1 on Windows because Windows does not use
// Unix signals. Processes are terminated via TerminateProcess, not signals.
func getSignalExitCode(_ *exec.ExitError) int {
	return -1
}

// setupProcessGroup creates the child process in a new process group on Windows.
// This prevents Ctrl+C from being automatically forwarded to the child process
// (we handle interrupt ourselves via context cancellation), and enables tree-kill
// via taskkill /T which targets the specified process and all its descendants.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killProcessGroup kills the entire process tree on Windows using taskkill.
// Go's Process.Kill() only terminates the direct child process, not its
// descendants. On Windows, commands are run through a shell (PowerShell/cmd),
// so Process.Kill() only kills the shell while the actual command continues.
// Using taskkill /F /T /PID ensures all descendant processes are terminated.
func killProcessGroup(pid int) {
	// Use taskkill to kill the process tree.
	// /F = force termination, /T = kill child processes (tree kill)
	_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).Run()
}
