//go:build !windows && !unix

package tools

// wrapCommandForResourceLimits is a no-op on unsupported platforms.
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
