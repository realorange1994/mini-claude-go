//go:build windows

package tools

import "os/exec"

// getSignalExitCode returns -1 on Windows because Windows does not use
// Unix signals. Processes are terminated via TerminateProcess, not signals.
func getSignalExitCode(_ *exec.ExitError) int {
	return -1
}
