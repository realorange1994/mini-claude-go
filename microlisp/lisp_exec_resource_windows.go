//go:build windows

package microlisp

import "os/exec"

// wrapWithResourceLimits is a no-op on Windows (Job Objects would be used).
// Returns nil (no restore needed).
func wrapWithResourceLimits(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) func() {
	return nil
}

// setupWindowsJobObject creates a Windows Job Object with memory/CPU limits.
// Not yet fully implemented.
func setupWindowsJobObject(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) error {
	return nil
}