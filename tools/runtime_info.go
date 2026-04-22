package tools

import (
	"fmt"
	"os"
	"runtime"
)

// RuntimeInfoTool provides Go runtime and system diagnostics.
type RuntimeInfoTool struct{}

func (*RuntimeInfoTool) Name() string        { return "runtime_info" }
func (*RuntimeInfoTool) Description() string { return "Show Go runtime and system information: version, OS, architecture, CPU count, working directory, and memory usage." }

func (*RuntimeInfoTool) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (*RuntimeInfoTool) CheckPermissions(params map[string]any) string { return "" }

func (*RuntimeInfoTool) Execute(params map[string]any) ToolResult {
	wd, _ := os.Getwd()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	info := fmt.Sprintf(`Go Version: %s
GOOS: %s
GOARCH: %s
NumCPU: %d
NumGoroutine: %d
Working Directory: %s
Memory Alloc: %.1f MB
Memory TotalAlloc: %.1f MB
Memory Sys: %.1f MB`,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
		runtime.NumCPU(),
		runtime.NumGoroutine(),
		wd,
		float64(mem.Alloc)/1024/1024,
		float64(mem.TotalAlloc)/1024/1024,
		float64(mem.Sys)/1024/1024,
	)

	return ToolResult{Output: info}
}
