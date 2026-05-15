//go:build !windows

package mcp

import "os/exec"

// configureSysProcAttr is a no-op on non-Windows platforms.
func configureSysProcAttr(cmd *exec.Cmd) {
	// No platform-specific process attributes needed on Unix.
}
