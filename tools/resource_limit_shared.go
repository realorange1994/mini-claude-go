package tools

import "fmt"

// ResourceLimits specifies resource constraints for an exec command.
type ResourceLimits struct {
	// MaxMemoryBytes sets the maximum memory the process can use.
	// 0 means no limit.
	MaxMemoryBytes int64

	// MaxCPUMillis sets the maximum CPU time in milliseconds.
	// 0 means no limit.
	MaxCPUMillis int64
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

// Format returns a human-readable description of the limits.
func (rl ResourceLimits) Format() string {
	if rl.MaxMemoryBytes == 0 && rl.MaxCPUMillis == 0 {
		return ""
	}
	var parts []string
	if rl.MaxMemoryBytes > 0 {
		parts = append(parts, fmt.Sprintf("max memory: %d MB", rl.MaxMemoryBytes/1024/1024))
	}
	if rl.MaxCPUMillis > 0 {
		parts = append(parts, fmt.Sprintf("max CPU: %d ms", rl.MaxCPUMillis))
	}
	return "(" + joinStr(parts, ", ") + ")"
}

func joinStr(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	r := ss[0]
	for i := 1; i < len(ss); i++ {
		r += sep + ss[i]
	}
	return r
}
