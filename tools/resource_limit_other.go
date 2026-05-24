//go:build !windows && !unix

package tools

import "os/exec"

// [STUB] JobObject is a Windows-only type. Stub definition for other platforms.
type JobObject struct {
	handle any
}

// [STUB] prepareResourceLimitsWindows is a no-op on unsupported platforms.
func prepareResourceLimitsWindows(rl ResourceLimits) (*JobObject, error) { return nil, nil }

// [STUB] assignResourceLimitsWindows is a no-op on unsupported platforms.
func assignResourceLimitsWindows(cmd *exec.Cmd, job *JobObject) error { return nil }

// [STUB] closeResourceLimitsWindows is a no-op on unsupported platforms.
func closeResourceLimitsWindows(job *JobObject) {}

// [STUB] WrapCommand is a no-op on unsupported platforms.
func (rl ResourceLimits) WrapCommand(command string) string {
	return command
}
