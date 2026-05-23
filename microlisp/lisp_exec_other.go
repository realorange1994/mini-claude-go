//go:build !windows && !unix

package microlisp

import (
	"os/exec"
	"syscall"
)

func setupExecProcessGroupAttr() *syscall.SysProcAttr {
	return nil
}

func killExecProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	cmd.Process.Kill()
}

// defaultPathEnv returns a sensible default PATH for other platforms.
func defaultPathEnv() string {
	return "PATH=/usr/local/bin:/usr/bin:/bin"
}