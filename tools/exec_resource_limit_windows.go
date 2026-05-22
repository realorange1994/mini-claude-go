//go:build windows

package tools

import (
	"fmt"
	"os/exec"
)

// prepareResourceLimitsWindows creates a Job Object before cmd.Start().
// Returns the job handle (caller should defer Close()) or nil if no limits.
func prepareResourceLimitsWindows(rl ResourceLimits) (*JobObject, error) {
	if rl.MaxMemoryBytes == 0 && rl.MaxCPUMillis == 0 {
		return nil, nil
	}
	job, err := rl.CreateJob()
	if err != nil {
		return nil, fmt.Errorf("resource limit: %w", err)
	}
	return job, nil
}

// assignResourceLimitsWindows assigns the started process to the Job Object.
func assignResourceLimitsWindows(cmd *exec.Cmd, job *JobObject) error {
	if job == nil {
		return nil
	}
	if err := job.AssignProcess(cmd); err != nil {
		job.Close()
		return fmt.Errorf("resource limit assign: %w", err)
	}
	return nil
}

// closeResourceLimitsWindows closes the Job Object handle.
func closeResourceLimitsWindows(job *JobObject) {
	if job != nil {
		job.Close()
	}
}

// wrapCommandForResourceLimits is a no-op on Windows (limits are via Job Objects).
func wrapCommandForResourceLimits(command string, rl ResourceLimits) string {
	return command
}

// parseResourceLimits extracts ResourceLimits from the tool params.
func parseResourceLimits(params map[string]any) ResourceLimits {
	var rl ResourceLimits
	if m, ok := params["max_memory_mb"]; ok {
		switch v := m.(type) {
		case float64:
			rl.MaxMemoryBytes = int64(v) * 1024 * 1024
		case int:
			rl.MaxMemoryBytes = int64(v) * 1024 * 1024
		}
	}
	if c, ok := params["max_cpu_ms"]; ok {
		switch v := c.(type) {
		case float64:
			rl.MaxCPUMillis = int64(v)
		case int:
			rl.MaxCPUMillis = int64(v)
		}
	}
	return rl
}