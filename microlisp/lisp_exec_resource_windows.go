//go:build windows

package microlisp

import "os/exec"

// wrapWithResourceLimits is a no-op on Windows (Job Objects would be used).
func wrapWithResourceLimits(command string, args []string, maxMemoryMB int64, maxCPUMS int64) (string, []string, error) {
	return command, args, nil
}

// setupWindowsJobObject creates a Windows Job Object with memory/CPU limits.
// Not yet fully implemented.
func setupWindowsJobObject(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) error {
	return nil
}
