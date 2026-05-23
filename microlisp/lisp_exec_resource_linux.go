//go:build linux

package microlisp

import (
	"os/exec"
	"syscall"
	"unsafe"
)

// wrapWithResourceLimits returns the command and args unchanged.
// Resource limits are enforced via prlimit on the child process after Start().
func wrapWithResourceLimits(command string, args []string, maxMemoryMB int64, maxCPUMS int64) (string, []string, error) {
	return command, args, nil
}

// prlimitPid calls the prlimit64 syscall (SYS_PRLIMIT64) on a specific PID.
// This sets resource limits on the child process without affecting the parent.
// Returns an error if the syscall fails.
func prlimitPid(pid int, resource int, cur, max uint64) error {
	// syscall.Prlimit is not available in Go's syscall package,
	// so we call prlimit64 directly via RawSyscall6.
	//
	// int prlimit(pid_t pid, int resource, const struct rlimit *new, struct rlimit *old)
	// glibc wraps prlimit64 as prlimit.
	var newRlimit [2]uint64 // [Cur, Max]
	newRlimit[0] = cur
	newRlimit[1] = max

	_, _, errno := syscall.RawSyscall6(
		syscall.SYS_PRLIMIT64,
		uintptr(pid),
		uintptr(resource),
		uintptr(unsafe.Pointer(&newRlimit[0])),
		0, // old_rlimit = NULL
		0, 0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}

// applyPrlimitToProcess uses prlimit64(2) to set RLIMIT_AS and RLIMIT_CPU
// on the child process. Unlike setrlimit, this targets a specific PID
// without affecting the calling process.
func applyPrlimitToProcess(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	pid := cmd.Process.Pid

	if maxMemoryMB > 0 {
		limit := uint64(maxMemoryMB) * 1024 * 1024
		_ = prlimitPid(pid, syscall.RLIMIT_AS, limit, limit)
	}

	if maxCPUMS > 0 {
		sec := maxCPUMS / 1000
		if sec == 0 {
			sec = 1
		}
		_ = prlimitPid(pid, syscall.RLIMIT_CPU, uint64(sec), uint64(sec))
	}
}

// startResourceMonitor applies prlimit to the child process after fork.
// Since limits are enforced by the kernel directly, no goroutine polling
// is needed. Returns nil channel (never triggers in select).
func startResourceMonitor(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) <-chan string {
	applyPrlimitToProcess(cmd, maxMemoryMB, maxCPUMS)
	return nil
}
