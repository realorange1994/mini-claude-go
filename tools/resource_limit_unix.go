//go:build unix

package tools

import (
	"fmt"
)

// ResourceLimits specifies resource constraints for an exec command.
type ResourceLimits struct {
	// MaxMemoryBytes sets the maximum memory (address space) the process can use.
	// 0 means no limit.
	MaxMemoryBytes int64

	// MaxCPUMillis sets the maximum CPU time in milliseconds.
	// 0 means no limit.
	MaxCPUMillis int64
}

// WrapCommand prepends resource limit commands to the shell command.
// On Unix, uses prlimit for memory and CPU limits.
// Returns the wrapped command string, or the original if no limits are set.
func (rl ResourceLimits) WrapCommand(command string) string {
	if rl.MaxMemoryBytes == 0 && rl.MaxCPUMillis == 0 {
		return command
	}

	var args []string
	if rl.MaxMemoryBytes > 0 {
		args = append(args, "--as", fmt.Sprintf("%d", rl.MaxMemoryBytes))
	}
	if rl.MaxCPUMillis > 0 {
		cpuSec := rl.MaxCPUMillis / 1000
		if cpuSec == 0 {
			cpuSec = 1
		}
		args = append(args, "--cpu", fmt.Sprintf("%d", cpuSec))
	}

	wrapped := "prlimit "
	for _, a := range args {
		wrapped += a + " "
	}
	wrapped += "-- " + command
	return wrapped
}

// Format returns a human-readable description of the limits.
func (rl ResourceLimits) Format() string {
	if rl.MaxMemoryBytes == 0 && rl.MaxCPUMillis == 0 {
		return ""
	}
	var parts []string
	if rl.MaxMemoryBytes > 0 {
		parts = append(parts, fmt.Sprintf("max memory: %d MB", rl.MaxMemoryBytes/1024/1024))
	}
	if rl.MaxCPUMillis > 0 {
		parts = append(parts, fmt.Sprintf("max CPU: %d ms", rl.MaxCPUMillis))
	}
	return "(" + joinStr(parts, ", ") + ")"
}

func joinStr(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	r := ss[0]
	for i := 1; i < len(ss); i++ {
		r += sep + ss[i]
	}
	return r
}
