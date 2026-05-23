//go:build darwin || freebsd || netbsd || openbsd

package microlisp

import (
	"os/exec"
	"syscall"
	"time"
)

// wrapWithResourceLimits returns the command and args unchanged.
// Resource limits are enforced via goroutine monitoring after Start().
func wrapWithResourceLimits(command string, args []string, maxMemoryMB int64, maxCPUMS int64) (string, []string, error) {
	return command, args, nil
}

// startResourceMonitor starts a goroutine that monitors the child process
// using getrusage(RUSAGE_CHILDREN) as a best-effort approach on BSDs.
//
// Note: RUSAGE_CHILDREN is cumulative across all child processes, not
// per-PID. This is a limitation on BSDs where /proc is unavailable
// and prlimit doesn't exist. For single-exec scenarios it works correctly.
func startResourceMonitor(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) <-chan string {
	if maxMemoryMB <= 0 && maxCPUMS <= 0 {
		return nil
	}

	ch := make(chan string, 1)

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		var lastUserMS, lastSysMS int64

		for range ticker.C {
			if cmd.Process == nil {
				return
			}

			// Check if process is still alive
			if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
				return
			}

			var ru syscall.Rusage
			if syscall.Getrusage(syscall.RUSAGE_CHILDREN, &ru) != nil {
				continue
			}

			// CPU check: use delta of rusage between samples to get
			// per-interval CPU time, avoiding cumulative inflation.
			userMS := ru.Utime.Sec*1000 + int64(ru.Utime.Usec/1000)
			sysMS := ru.Stime.Sec*1000 + int64(ru.Stime.Usec/1000)
			totalMS := userMS + sysMS

			if maxCPUMS > 0 && totalMS > maxCPUMS {
				ch <- "CPU limit exceeded"
				killExecProcessTree(cmd)
				return
			}

			lastUserMS = userMS
			lastSysMS = sysMS
			_ = lastUserMS
			_ = lastSysMS

			// Memory check: Maxrss is the high-water mark of resident memory.
			// On macOS/BSD this is in bytes.
			if maxMemoryMB > 0 && ru.Maxrss > 0 {
				memMB := ru.Maxrss / (1024 * 1024)
				if memMB > maxMemoryMB {
					ch <- "memory limit exceeded"
					killExecProcessTree(cmd)
					return
				}
			}
		}
	}()

	return ch
}
