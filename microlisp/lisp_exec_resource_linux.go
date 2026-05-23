//go:build linux

package microlisp

import (
	"os/exec"
	"syscall"
)

// wrapWithResourceLimits returns the command and args unchanged.
// Resource limits are enforced via prlimit on the child process after Start().
func wrapWithResourceLimits(command string, args []string, maxMemoryMB int64, maxCPUMS int64) (string, []string, error) {
	return command, args, nil
}

// applyPrlimitToProcess uses prlimit(2) to set RLIMIT_AS and RLIMIT_CPU
// on the child process. Unlike setrlimit, this targets a specific PID
// without affecting the calling process.
func applyPrlimitToProcess(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	pid := cmd.Process.Pid

	if maxMemoryMB > 0 {
		limit := uint64(maxMemoryMB) * 1024 * 1024
		newRlimit := syscall.Rlimit{Cur: limit, Max: limit}
		_ = syscall.Prlimit(pid, syscall.RLIMIT_AS, &newRlimit, nil)
	}

	if maxCPUMS > 0 {
		sec := maxCPUMS / 1000
		if sec == 0 {
			sec = 1
		}
		newRlimit := syscall.Rlimit{Cur: uint64(sec), Max: uint64(sec)}
		_ = syscall.Prlimit(pid, syscall.RLIMIT_CPU, &newRlimit, nil)
	}
}

// startResourceMonitor is a no-op on Linux when prlimit is available,
// since limits are enforced by the kernel directly.
// Returns nil channel (never triggers in select).
func startResourceMonitor(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) <-chan string {
	// Apply prlimit to child process after it has been forked.
	// This is more reliable than goroutine polling: the kernel enforces
	// limits immediately, no delay, no PID reuse issues.
	applyPrlimitToProcess(cmd, maxMemoryMB, maxCPUMS)
	return nil
}
