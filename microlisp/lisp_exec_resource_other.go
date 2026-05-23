//go:build !windows && !unix

package microlisp

import "os/exec"

// wrapWithResourceLimits is a no-op on unsupported platforms.
// Returns nil (no restore needed).
func wrapWithResourceLimits(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) func() {
	return nil
}

// setupWindowsJobObject is a no-op on non-Windows platforms.
func setupWindowsJobObject(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) error {
	return nil
}