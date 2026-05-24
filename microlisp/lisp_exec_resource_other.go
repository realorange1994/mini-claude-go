//go:build !unix && !windows

package microlisp

import "os/exec"

// wrapWithResourceLimits returns the command and args unchanged.
// Resource limits are not enforced on unsupported platforms.
func wrapWithResourceLimits(command string, args []string, maxMemoryMB int64, maxCPUMS int64) (string, []string, error) {
	return command, args, nil
}

// [STUB] startResourceMonitor is a no-op on unsupported platforms.
func startResourceMonitor(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) <-chan string {
	return nil // nil channel never becomes ready in select
}