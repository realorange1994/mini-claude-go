//go:build windows

package tools

import "os/exec"

// getSignalExitCode returns -1 on Windows because Windows does not use
// Unix signals. Processes are terminated via TerminateProcess, not signals.
func getSignalExitCode(_ *exec.ExitError) int {
	return -1
}

// setupProcessGroup is a no-op on Windows.
// Windows does not support Unix-style process groups in the same way.
func setupProcessGroup(_ *exec.Cmd) {}

// killProcessGroup is a no-op on Windows.
// Windows process termination via Process.Kill() already terminates the process tree.
func killProcessGroup(_ int) {}
