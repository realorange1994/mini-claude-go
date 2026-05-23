//go:build !windows && !unix

package tools

// WrapCommand is a no-op on unsupported platforms.
func (rl ResourceLimits) WrapCommand(command string) string {
	return command
}
