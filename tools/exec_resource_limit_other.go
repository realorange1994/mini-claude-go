//go:build !windows && !unix

package tools

// [STUB] wrapCommandForResourceLimits is a no-op on unsupported platforms.
func wrapCommandForResourceLimits(command string, rl ResourceLimits) string {
	return command
}
