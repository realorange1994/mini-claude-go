//go:build unix

package tools

// wrapCommandForResourceLimits prepends ulimit settings inside the bash -c string.
// Since the command runs inside "bash -c <cmd>", ulimit (a shell builtin) is
// always available — no dependency on external tools like prlimit.
func wrapCommandForResourceLimits(command string, rl ResourceLimits) string {
	prefix := rl.UlimitPrefix()
	if prefix == "" {
		return command
	}
	return prefix + command
}
