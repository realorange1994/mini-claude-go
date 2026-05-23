//go:build !windows && !unix

package tools

import "os/exec"

// JobObject is a Windows-only type. Stub definition for other platforms.
type JobObject struct {
	handle any
}

// prepareResourceLimitsWindows is a no-op on unsupported platforms.
func prepareResourceLimitsWindows(rl ResourceLimits) (*JobObject, error) { return nil, nil }

// assignResourceLimitsWindows is a no-op on unsupported platforms.
func assignResourceLimitsWindows(cmd *exec.Cmd, job *JobObject) error { return nil }

// closeResourceLimitsWindows is a no-op on unsupported platforms.
func closeResourceLimitsWindows(job *JobObject) {}

// WrapCommand is a no-op on unsupported platforms.
func (rl ResourceLimits) WrapCommand(command string) string {
	return command
}
