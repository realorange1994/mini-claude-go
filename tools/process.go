package tools

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ProcessTool provides process management (list, kill, pkill, pgrep, top, pstree).
type ProcessTool struct{}

func (*ProcessTool) Name() string        { return "process" }
func (*ProcessTool) Description() string { return "Process management and monitoring. Supports list (ps), kill, pkill, pgrep, top, and pstree operations. On Windows, uses PowerShell cmdlets." }

func (*ProcessTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"description": "Operation: list, kill, pkill, pgrep, top, pstree",
				"enum":        []string{"list", "kill", "pkill", "pgrep", "top", "pstree"},
			},
			"pid": map[string]any{
				"type":        "integer",
				"description": "Process ID (for kill).",
			},
			"pattern": map[string]any{
				"type":        "string",
				"description": "Process name pattern (for pkill, pgrep).",
			},
			"signal": map[string]any{
				"type":        "string",
				"description": "Signal to send (e.g., SIGTERM, SIGKILL, 9). Unix only (default: SIGTERM).",
			},
			"user": map[string]any{
				"type":        "string",
				"description": "Filter by user (for list, pgrep).",
			},
			"lines": map[string]any{
				"type":        "integer",
				"description": "Number of lines to show for top (default: 10).",
			},
		},
		"required": []string{"operation"},
	}
}

func (*ProcessTool) CheckPermissions(params map[string]any) string { return "" }

func (*ProcessTool) Execute(params map[string]any) ToolResult {
	operation, _ := params["operation"].(string)
	if operation == "" {
		return ToolResult{Output: "Error: operation is required", IsError: true}
	}

	isWindows := runtime.GOOS == "windows"

	switch operation {
	case "list":
		return processList(params, isWindows)
	case "kill":
		return processKill(params, isWindows)
	case "pkill":
		return processKillByName(params, isWindows)
	case "pgrep":
		return processGrep(params, isWindows)
	case "top":
		return processTop(params, isWindows)
	case "pstree":
		return processPstree(isWindows)
	default:
		return ToolResult{Output: fmt.Sprintf("Error: unknown operation: %s", operation), IsError: true}
	}
}

func processList(params map[string]any, isWindows bool) ToolResult {
	var cmd *exec.Cmd
	if isWindows {
		user, hasUser := params["user"].(string)
		if hasUser && user != "" {
			safeUser := sanitizePSInput(user)
			cmd = exec.Command("powershell", "-NoProfile", "-Command",
				fmt.Sprintf("Get-Process -IncludeUserName | Where-Object {$_.UserName -like '*%s*'} | Format-Table -AutoSize", safeUser))
		} else {
			cmd = exec.Command("powershell", "-NoProfile", "-Command",
				"Get-Process | Format-Table -AutoSize")
		}
	} else {
		user, hasUser := params["user"].(string)
		if hasUser && user != "" {
			cmd = exec.Command("ps", "-u", user)
		} else {
			cmd = exec.Command("ps", "aux")
		}
	}
	return runCmd(cmd)
}

func processKill(params map[string]any, isWindows bool) ToolResult {
	pidVal, ok := params["pid"]
	if !ok {
		return ToolResult{Output: "Error: pid is required for kill", IsError: true}
	}
	var pid int
	switch v := pidVal.(type) {
	case float64:
		pid = int(v)
	case int:
		pid = v
	}
	if pid == 0 {
		return ToolResult{Output: "Error: pid must be a non-zero integer", IsError: true}
	}

	if isWindows {
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf("Stop-Process -Id %d -Force", pid))
		return runCmd(cmd)
	}

	signal := "SIGTERM"
	if s, ok := params["signal"].(string); ok && s != "" {
		signal = s
	}
	sig := signal
	if _, err := fmt.Sscanf(signal, "%d", new(int)); err != nil {
		sig = "-" + strings.TrimPrefix(strings.TrimPrefix(signal, "SIG"), "")
	}
	cmd := exec.Command("kill", sig, fmt.Sprintf("%d", pid))
	return runCmd(cmd)
}

func processKillByName(params map[string]any, isWindows bool) ToolResult {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return ToolResult{Output: "Error: pattern is required for pkill", IsError: true}
	}

	if isWindows {
		safePattern := sanitizePSInput(pattern)
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf("Get-Process -Name '*%s*' -ErrorAction SilentlyContinue | Stop-Process -Force", safePattern))
		out, err := cmd.CombinedOutput()
		output := strings.TrimSpace(string(out))
		if output == "" {
			output = fmt.Sprintf("No processes matching '%s' found", pattern)
		}
		if err != nil {
			return ToolResult{Output: output, IsError: true}
		}
		return ToolResult{Output: output}
	}

	signal := "SIGTERM"
	if s, ok := params["signal"].(string); ok && s != "" {
		signal = s
	}
	sig := signal
	if _, err := fmt.Sscanf(signal, "%d", new(int)); err != nil {
		sig = "-" + strings.TrimPrefix(strings.TrimPrefix(signal, "SIG"), "")
	}
	cmd := exec.Command("pkill", sig, pattern)
	return runCmd(cmd)
}

func processGrep(params map[string]any, isWindows bool) ToolResult {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return ToolResult{Output: "Error: pattern is required for pgrep", IsError: true}
	}

	if isWindows {
		safePattern := sanitizePSInput(pattern)
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf("Get-Process -Name '*%s*' -ErrorAction SilentlyContinue | Format-Table Id, ProcessName, CPU, WorkingSet -AutoSize", safePattern))
		out, err := cmd.CombinedOutput()
		output := strings.TrimSpace(string(out))
		if output == "" {
			return ToolResult{Output: fmt.Sprintf("No processes matching '%s' found", pattern)}
		}
		if err != nil {
			return ToolResult{Output: output, IsError: true}
		}
		return ToolResult{Output: output}
	}

	cmd := exec.Command("pgrep", "-a", pattern)
	return runCmd(cmd)
}

func runCmd(cmd *exec.Cmd) ToolResult {
	if cmd == nil {
		return ToolResult{Output: "Error: nil command", IsError: true}
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if output == "" {
		output = "No output."
	}
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, output), IsError: true}
	}
	return ToolResult{Output: output}
}

// processTop shows top processes by CPU usage.
func processTop(params map[string]any, isWindows bool) ToolResult {
	lines := 10
	if l, ok := params["lines"].(float64); ok {
		lines = int(l)
	}
	if lines <= 0 {
		lines = 10
	}

	if isWindows {
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf(`Get-Process | Sort-Object CPU -Descending | Select-Object -First %d Id, ProcessName, CPU, @{N='Memory MB';E={[math]::Round($_.WorkingSet64/1MB,1)}} | Format-Table -AutoSize`, lines))
		return runCmd(cmd)
	}
	cmd := exec.Command("top", "-b", "-n", "1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
	}
	outputLines := strings.Split(stdout.String(), "\n")
	if len(outputLines) > lines+6 {
		outputLines = outputLines[:lines+6]
	}
	return ToolResult{Output: strings.TrimSpace(strings.Join(outputLines, "\n"))}
}

// processPstree shows process tree.
func processPstree(isWindows bool) ToolResult {
	if isWindows {
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			`Get-Process | Select-Object -First 30 Id, ProcessName, @{N='Parent';E={(Get-CimInstance Win32_Process -Filter "ProcessId=$($_.Id)").ParentProcessId}} | Format-Table -AutoSize`)
		return runCmd(cmd)
	}
	cmd := exec.Command("pstree")
	return runCmd(cmd)
}

// sanitizePSInput strips PowerShell metacharacters to prevent command injection.
func sanitizePSInput(s string) string {
	replacer := strings.NewReplacer(
		"'", "", `"`, "", "`", "", "$", "",
		";", "", "&", "", "|", "",
		"(", "", ")", "", "{", "", "}", "",
		"<", "", ">", "", "\n", "", "\r", "",
	)
	return replacer.Replace(s)
}
