//go:build unix

package microlisp

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// wrapWithResourceLimits returns the command and args unchanged.
// Resource limits are enforced via goroutine monitoring after Start(),
// so no external tool dependency (bash/ulimit) is needed.
func wrapWithResourceLimits(command string, args []string, maxMemoryMB int64, maxCPUMS int64) (string, []string, error) {
	return command, args, nil
}

// startResourceMonitor starts a goroutine that periodically checks
// the child process's memory and CPU usage. If limits are exceeded,
// the process tree is killed and the reason is sent on the returned channel.
//
// Returns a channel that receives the kill reason (or empty string if
// the process exited before any limit was hit). The caller should
// select on this channel alongside cmd.Wait() to detect limit breaches.
func startResourceMonitor(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) <-chan string {
	if maxMemoryMB <= 0 && maxCPUMS <= 0 {
		return nil // nil channel never becomes ready in select
	}

	ch := make(chan string, 1)

	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			if cmd.Process == nil {
				return
			}

			// Check if process is still alive
			if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
				// Process already exited
				return
			}

			pid := cmd.Process.Pid

			// Memory check: read /proc/[pid]/statm (Linux) or use getrusage (other Unix)
			if maxMemoryMB > 0 {
				memKB := getProcessMemoryKB(pid)
				if memKB > 0 {
					limitKB := maxMemoryMB * 1024
					if memKB > limitKB {
						reason := fmt.Sprintf("memory limit exceeded (%d MB > %d MB)", memKB/1024, maxMemoryMB)
						killExecProcessTree(cmd)
						ch <- reason
						return
					}
				}
			}

			// CPU check: use getrusage for the child process
			if maxCPUMS > 0 {
				cpuMS := getProcessCPUMS(pid)
				if cpuMS > maxCPUMS {
					reason := fmt.Sprintf("CPU limit exceeded (%d ms > %d ms)", cpuMS, maxCPUMS)
					killExecProcessTree(cmd)
					ch <- reason
					return
				}
			}
		}
	}()

	return ch
}

// getProcessMemoryKB reads the resident set size of a process in kilobytes.
// On Linux, reads /proc/[pid]/statm. Returns 0 on failure.
func getProcessMemoryKB(pid int) int64 {
	// Try /proc/[pid]/statm (Linux)
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err == nil {
		// Format: size resident shared text lib data dt
		// resident is in pages; second field
		fields := strings.Fields(string(data))
		if len(fields) >= 2 {
			if residentPages, e := strconv.ParseInt(fields[1], 10, 64); e == nil {
				pageSize := int64(os.Getpagesize())
				return (residentPages * pageSize) / 1024
			}
		}
	}

	// Fallback: getrusage (works on macOS/BSDs, but only for own children
	// with RUSAGE_CHILDREN — not per-PID). On Linux /proc is the way.
	// For macOS, we try RUSAGE_CHILDREN as a best-effort.
	var ru syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_CHILDREN, &ru) == nil {
		// Maxrss is in kilobytes on Linux, bytes on macOS/BSD
		maxrss := ru.Maxrss
		if maxrss > 0 {
			// On macOS, Maxrss is in bytes — convert
			if maxrss > 10*1024*1024 { // heuristic: > 10GB means it's in bytes
				maxrss /= 1024
			}
			return maxrss
		}
	}

	return 0
}

// getProcessCPUMS returns the total CPU time (user + system) in milliseconds
// for the child process. Uses getrusage(RUSAGE_CHILDREN).
func getProcessCPUMS(pid int) int64 {
	var ru syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_CHILDREN, &ru) == nil {
		userMS := ru.Utime.Sec*1000 + int64(ru.Utime.Usec/1000)
		sysMS := ru.Stime.Sec*1000 + int64(ru.Stime.Usec/1000)
		return userMS + sysMS
	}
	return 0
}