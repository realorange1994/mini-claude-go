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

// [STUB] wrapCommandForResourceLimits is a no-op on Windows (limits are via Job Objects).
func wrapCommandForResourceLimits(command string, rl ResourceLimits) string {
	return command
}
