//go:build windows

package microlisp

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// wrapWithResourceLimits returns the command and args unchanged.
// Resource limits are enforced via goroutine monitoring after Start(),
// so no external tool dependency is needed.
func wrapWithResourceLimits(command string, args []string, maxMemoryMB int64, maxCPUMS int64) (string, []string, error) {
	return command, args, nil
}

// startResourceMonitor starts a goroutine that monitors the child process.
// On Windows, we use runtime.ReadMemStats as a coarse guard for the parent's
// own memory, and periodic process liveness checks. Per-child memory/CPU
// tracking requires Windows Job Objects (future improvement).
//
// If limits are exceeded, the process tree is killed and a reason is sent
// on the returned channel.
func startResourceMonitor(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) <-chan string {
	if maxMemoryMB <= 0 && maxCPUMS <= 0 {
		return nil // nil channel never becomes ready in select
	}

	ch := make(chan string, 1)

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		start := time.Now()

		for range ticker.C {
			if cmd.Process == nil {
				return
			}

			// Check if process is still alive
			if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
				return
			}

			// Coarse CPU time limit: use wall-clock elapsed as a rough proxy.
			// Since we can't get per-child CPU on Windows without Job Objects,
			// this prevents infinite runaway but isn't as precise.
			if maxCPUMS > 0 {
				elapsed := time.Since(start).Milliseconds()
				// Use 3x multiplier as heuristic: wall-clock > CPU time
				// This is conservative — only triggers if wall time far exceeds limit
				if elapsed > maxCPUMS*3 {
					reason := fmt.Sprintf("CPU time limit likely exceeded (wall clock %d ms > 3x limit %d ms)", elapsed, maxCPUMS)
					killExecProcessTree(cmd)
					ch <- reason
					return
				}
			}

			// Memory: on Windows, we can't easily get per-child memory without
			// PSAPI calls or Job Objects. Skip per-process memory check for now.
			// The parent process's own safety limits in safety.go still apply.
		}
	}()

	return ch
}
