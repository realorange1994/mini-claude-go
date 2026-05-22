//go:build !windows && !unix

package tools

// ResourceLimits specifies resource constraints for an exec command.
// On unsupported platforms, this is a no-op.
type ResourceLimits struct {
	MaxMemoryBytes int64
	MaxCPUMillis   int64
}

// WrapCommand is a no-op on unsupported platforms.
func (rl ResourceLimits) WrapCommand(command string) string {
	return command
}

// Format returns empty string on unsupported platforms.
func (rl ResourceLimits) Format() string {
	return ""
}
