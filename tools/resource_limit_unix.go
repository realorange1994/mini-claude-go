//go:build unix

package tools

import (
	"fmt"
	"os/exec"
	"strings"
)

// JobObject is a Windows-only type. Stub definition for Unix compilation.
type JobObject struct {
	handle any
}

// prepareResourceLimitsWindows is a no-op on Unix.
func prepareResourceLimitsWindows(rl ResourceLimits) (*JobObject, error) { return nil, nil }

// assignResourceLimitsWindows is a no-op on Unix.
func assignResourceLimitsWindows(cmd *exec.Cmd, job *JobObject) error { return nil }

// closeResourceLimitsWindows is a no-op on Unix.
func closeResourceLimitsWindows(job *JobObject) {}

// UlimitPrefix returns a bash-compatible prefix that sets resource limits
// via the ulimit builtin. Returns empty string if no limits are set.
// This prefix is inserted inside the bash -c string, so ulimit (a builtin)
// is always available — no external tool dependency.
func (rl ResourceLimits) UlimitPrefix() string {
	if rl.MaxMemoryBytes == 0 && rl.MaxCPUMillis == 0 {
		return ""
	}
	var parts []string
	if rl.MaxMemoryBytes > 0 {
		// ulimit -v: virtual memory limit in KB
		parts = append(parts, fmt.Sprintf("ulimit -v %d", rl.MaxMemoryBytes/1024))
	}
	if rl.MaxCPUMillis > 0 {
		// ulimit -t: CPU time limit in seconds (minimum 1)
		sec := rl.MaxCPUMillis / 1000
		if sec == 0 {
			sec = 1
		}
		parts = append(parts, fmt.Sprintf("ulimit -t %d", sec))
	}
	return strings.Join(parts, "; ") + "; "
}
