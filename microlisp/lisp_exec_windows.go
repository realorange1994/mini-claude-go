//go:build windows

package microlisp

import (
	"os/exec"
	"strconv"
	"syscall"
)

func setupExecProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func killExecProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	cmd.Process.Kill()
}

// defaultPathEnv returns a sensible default PATH for Windows systems.
func defaultPathEnv() string {
	return "PATH=C:\\Windows\\System32;C:\\Windows;C:\\Windows\\System32\\Wbem"
}