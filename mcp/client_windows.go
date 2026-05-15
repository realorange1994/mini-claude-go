//go:build windows

package mcp

import (
	"os/exec"
	"syscall"
)

// configureSysProcAttr sets Windows-specific process attributes.
// Creates a new process group to prevent Ctrl+C from being forwarded
// to the child process and to isolate the child's console state.
func configureSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
