//go:build !windows

package mcp

import "os/exec"

// [STUB] configureSysProcAttr is a no-op on non-Windows platforms.
func configureSysProcAttr(cmd *exec.Cmd) {
	// [STUB] No platform-specific process attributes needed on Unix.
}
