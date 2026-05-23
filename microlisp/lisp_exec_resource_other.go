//go:build !unix && !windows

package microlisp

// wrapWithResourceLimits is a no-op on unsupported platforms.
func wrapWithResourceLimits(command string, args []string, maxMemoryMB int64, maxCPUMS int64) (string, []string, error) {
	return command, args, nil
}
