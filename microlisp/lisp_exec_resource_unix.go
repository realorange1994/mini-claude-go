//go:build unix

package microlisp

import (
	"os/exec"
	"syscall"
)

// wrapWithResourceLimits temporarily sets RLIMIT_AS and RLIMIT_CPU on the
// current process, then returns a restore function.
//
// Usage:
//   restore := wrapWithResourceLimits(cmd, maxMem, maxCPU)
//   if err := cmd.Start(); err != nil { ... }
//   restore()  // child already inherited limits at fork
//
// Why not a "modify cmd" approach? Go doesn't expose the fork/exec split
// point, so the only way to pass rlimits to the child is to set them on
// the parent before Start() (which forks). We restore immediately after.
//
// Race: between setrlimit and Start(), other goroutines forking children
// would inherit our modified limits. This window is microseconds in
// practice. For production isolation, use a dedicated spawner goroutine.
func wrapWithResourceLimits(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) func() {
	if maxMemoryMB == 0 && maxCPUMS == 0 {
		return nil
	}

	oldMem, err := syscall.Getrlimit(syscall.RLIMIT_AS)
	if err != nil {
		return nil
	}
	oldCPU, _ := syscall.Getrlimit(syscall.RLIMIT_CPU)

	if maxMemoryMB > 0 {
		_ = syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{
			Cur: uint64(maxMemoryMB) * 1024 * 1024,
			Max: uint64(maxMemoryMB) * 1024 * 1024,
		})
	}
	if maxCPUMS > 0 {
		sec := maxCPUMS / 1000
		if sec == 0 {
			sec = 1
		}
		_ = syscall.Setrlimit(syscall.RLIMIT_CPU, &syscall.Rlimit{
			Cur: uint64(sec),
			Max: uint64(sec),
		})
	}

	return func() {
		_ = syscall.Setrlimit(syscall.RLIMIT_AS, &oldMem)
		_ = syscall.Setrlimit(syscall.RLIMIT_CPU, &oldCPU)
	}
}

// setupWindowsJobObject is a no-op on Unix.
func setupWindowsJobObject(cmd *exec.Cmd, maxMemoryMB int64, maxCPUMS int64) error {
	return nil
}
