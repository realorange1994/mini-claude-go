//go:build unix

package microlisp

import (
	"os/exec"
	"syscall"
)

func setupExecProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func killExecProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	cmd.Process.Kill()
}

// defaultPathEnv returns a sensible default PATH for Unix systems.
func defaultPathEnv() string {
	return "PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
}
