package tools

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// ProcessTool provides process management (list, kill, pkill, pgrep, top, pstree).
type ProcessTool struct{}

func (*ProcessTool) Name() string { return "process" }
func (*ProcessTool) Description() string {
	return "Process management and monitoring. Supports list (ps), kill, pkill, pgrep, top, and pstree operations. On Windows, uses PowerShell cmdlets."
}

func (*ProcessTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"description": "Operation: list, kill, pkill, pgrep, top, pstree",
				"enum":        []any{"list", "kill", "pkill", "pgrep", "top", "pstree"},
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
				"description": "Signal to send (e.g., SIGTERM, SIGKILL, SIGINT, SIGHUP, or numeric 9/15). Unix only. Ignored if force=true.",
			},
			"force": map[string]any{
				"type":        "boolean",
				"description": "Force kill. Unix: sends SIGKILL (-9). Windows: uses -Force flag. Overrides signal parameter.",
			},
			"user": map[string]any{
				"type":        "string",
				"description": "Filter by user (for list, pgrep).",
			},
			"filter_name": map[string]any{
				"type":        "string",
				"description": "Filter processes by name/command keyword (for list).",
			},
			"filter_status": map[string]any{
				"type":        "string",
				"description": "Filter by process status: running, idle, stopped (for list).",
			},
			"max_cpu": map[string]any{
				"type":        "number",
				"description": "Maximum CPU usage percent to include (for list). Processes above this are excluded.",
			},
			"max_memory": map[string]any{
				"type":        "number",
				"description": "Maximum memory usage in MB to include (for list). Processes above this are excluded.",
			},
			"lines": map[string]any{
				"type":        "integer",
				"description": "Number of lines to show for top (default: 10).",
			},
		},
		"required": []string{"operation"},
	}
}

func (*ProcessTool) CheckPermissions(params map[string]any) PermissionResult {
	return PermissionResultPassthrough()
}

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
		return ToolResult{Output: fmt.Sprintf("Error: unknown operation: %s. Available operations: list, kill, pkill, pgrep, top, pstree", operation), IsError: true}
	}
}

func processList(params map[string]any, isWindows bool) ToolResult {
	filterName, _ := params["filter_name"].(string)
	filterStatus, _ := params["filter_status"].(string)
	filterUser, _ := params["user"].(string)
	var maxCPU, maxMemory float64
	if v, ok := params["max_cpu"].(float64); ok {
		maxCPU = v
	}
	if v, ok := params["max_memory"].(float64); ok {
		maxMemory = v
	}

	hasFilters := filterName != "" || filterStatus != "" || filterUser != "" || maxCPU > 0 || maxMemory > 0

	if isWindows {
		// Use ConvertTo-Json for structured output so we can filter in Go
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			`Get-Process -IncludeUserName | Select-Object Id, ProcessName, CPU, @{N='MemMB';E={[math]::Round($_.WorkingSet64/1MB,1)}}, UserName | ConvertTo-Json`)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, strings.TrimSpace(string(out))), IsError: true}
		}
		result := filterWindowsProcesses(string(out), filterName, filterStatus, filterUser, maxCPU, maxMemory, hasFilters)
		return ToolResult{Output: result}
	}

	// Unix: parse ps aux output
	cmd := exec.Command("ps", "aux")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, strings.TrimSpace(string(out))), IsError: true}
	}

	if !hasFilters {
		return ToolResult{Output: strings.TrimSpace(string(out))}
	}

	return ToolResult{Output: filterUnixProcesses(string(out), filterName, filterStatus, filterUser, maxCPU, maxMemory)}
}

// procInfo holds parsed process fields for filtering.
type procInfo struct {
	user    string
	pid     string
	cpu     float64
	memPct  float64
	vsz     string
	rss     string
	tty     string
	stat    string
	start   string
	time    string
	command string
}

func filterUnixProcesses(raw, filterName, filterStatus, filterUser string, maxCPU, maxMemory float64) string {
	lines := strings.Split(raw, "\n")
	if len(lines) < 2 {
		return strings.TrimSpace(raw)
	}

	var result []string
	result = append(result, lines[0]) // header

	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}

		p := procInfo{
			user:    fields[0],
			pid:     fields[1],
			tty:     fields[5],
			stat:    fields[6],
			start:   fields[7],
			time:    fields[8],
			command: strings.Join(fields[10:], " "),
		}
		p.cpu, _ = strconv.ParseFloat(fields[2], 64)
		p.memPct, _ = strconv.ParseFloat(fields[3], 64)
		p.vsz = fields[4]
		p.rss = fields[5]

		if filterUser != "" && !strings.Contains(strings.ToLower(p.user), strings.ToLower(filterUser)) {
			continue
		}
		if filterName != "" && !strings.Contains(strings.ToLower(p.command), strings.ToLower(filterName)) {
			continue
		}
		if filterStatus != "" && !statusMatches(p.stat, filterStatus) {
			continue
		}
		if maxCPU > 0 && p.cpu > maxCPU {
			continue
		}
		if maxMemory > 0 {
			rssKB, _ := strconv.ParseFloat(p.rss, 64)
			rssMB := rssKB / 1024.0
			if rssMB > maxMemory {
				continue
			}
		}

		result = append(result, line)
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

// statusMatches maps Unix stat codes to human-readable status.
func statusMatches(stat, filter string) bool {
	f := strings.ToLower(filter)
	s := strings.ToUpper(stat)
	switch f {
	case "running":
		return strings.Contains(s, "R")
	case "idle":
		return strings.Contains(s, "S")
	case "stopped":
		return strings.Contains(s, "T")
	}
	return false
}

func filterWindowsProcesses(jsonStr, filterName, filterStatus, filterUser string, maxCPU, maxMemory float64, hasFilters bool) string {
	if !hasFilters {
		// Just pretty-print the JSON as a table
		return formatWinProcJSON(jsonStr)
	}

	// Parse JSON array of process objects
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || jsonStr == "null" {
		return "No processes found."
	}

	// Simple JSON array parsing (single object case too)
	var procs []map[string]any
	if strings.HasPrefix(jsonStr, "[") {
		// Array case - manual parse
		procs = parseSimpleWinProcs(jsonStr)
	} else if strings.HasPrefix(jsonStr, "{") {
		// Single object
		p := parseSingleWinProc(jsonStr)
		if p != nil {
			procs = append(procs, p)
		}
	}

	var filtered []map[string]any
	for _, p := range procs {
		name, _ := p["ProcessName"].(string)
		user, _ := p["UserName"].(string)
		cpu, _ := p["CPU"].(float64)
		memMB, _ := p["MemMB"].(float64)

		if filterName != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(filterName)) {
			continue
		}
		if filterUser != "" && !strings.Contains(strings.ToLower(user), strings.ToLower(filterUser)) {
			continue
		}
		if filterStatus != "" {
			// Windows doesn't have Unix-style stat codes; skip filter
		}
		if maxCPU > 0 && cpu > maxCPU {
			continue
		}
		if maxMemory > 0 && memMB > maxMemory {
			continue
		}
		filtered = append(filtered, p)
	}

	if len(filtered) == 0 {
		return "No processes match the filter criteria."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-8s %-25s %-10s %-10s %s\n", "Id", "ProcessName", "CPU", "MemMB", "UserName"))
	for _, p := range filtered {
		id := fmt.Sprintf("%v", p["Id"])
		name, _ := p["ProcessName"].(string)
		cpu := fmt.Sprintf("%.1f", p["CPU"])
		mem := fmt.Sprintf("%.1f", p["MemMB"])
		user, _ := p["UserName"].(string)
		sb.WriteString(fmt.Sprintf("%-8s %-25s %-10s %-10s %s\n", id, name, cpu, mem, user))
	}
	return strings.TrimSpace(sb.String())
}

func formatWinProcJSON(jsonStr string) string {
	procs := parseSimpleWinProcs(jsonStr)
	if len(procs) == 0 {
		return strings.TrimSpace(jsonStr)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-8s %-25s %-10s %-10s %s\n", "Id", "ProcessName", "CPU", "MemMB", "UserName"))
	for _, p := range procs {
		id := fmt.Sprintf("%v", p["Id"])
		name, _ := p["ProcessName"].(string)
		cpu := fmt.Sprintf("%.1f", p["CPU"])
		mem := fmt.Sprintf("%.1f", p["MemMB"])
		user, _ := p["UserName"].(string)
		sb.WriteString(fmt.Sprintf("%-8s %-25s %-10s %-10s %s\n", id, name, cpu, mem, user))
	}
	return strings.TrimSpace(sb.String())
}

// parseSingleWinProc parses a single JSON object into a map.
func parseSingleWinProc(s string) map[string]any {
	return parseSimpleWinProcs("[" + s + "]")[0]
}

// parseSimpleWinProcs does minimal JSON parsing for Windows process output.
func parseSimpleWinProcs(jsonStr string) []map[string]any {
	var result []map[string]any
	// Tokenize objects between { and }
	depth := 0
	start := -1
	for i := 0; i < len(jsonStr); i++ {
		switch jsonStr[i] {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				obj := parseFlatJSONObject(jsonStr[start+1 : i])
				if obj != nil {
					result = append(result, obj)
				}
				start = -1
			}
		}
	}
	return result
}

// parseFlatJSONObject parses a flat JSON object (no nested objects/arrays).
func parseFlatJSONObject(s string) map[string]any {
	m := make(map[string]any)
	i := 0
	for i < len(s) {
		// Find key
		kq := strings.Index(s[i:], "\"")
		if kq < 0 {
			break
		}
		i += kq + 1
		end := strings.Index(s[i:], "\"")
		if end < 0 {
			break
		}
		key := s[i : i+end]
		i += end + 1
		// Skip :
		colon := strings.Index(s[i:], ":")
		if colon < 0 {
			break
		}
		i += colon + 1
		// Skip whitespace
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
			i++
		}
		if i >= len(s) {
			break
		}
		// Parse value
		if s[i] == '"' {
			i++
			end = strings.Index(s[i:], "\"")
			if end < 0 {
				break
			}
			val := s[i : i+end]
			i += end + 1
			m[key] = val
		} else if s[i] == 'n' && i+3 < len(s) && s[i:i+4] == "null" {
			m[key] = nil
			i += 4
		} else {
			// Number or boolean
			end := i
			for end < len(s) && s[end] != ',' && s[end] != '}' && s[end] != ' ' && s[end] != '\r' && s[end] != '\n' {
				end++
			}
			val := strings.TrimSpace(s[i:end])
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				m[key] = f
			} else if val == "true" {
				m[key] = true
			} else if val == "false" {
				m[key] = false
			} else {
				m[key] = val
			}
			i = end
		}
		// Skip comma
		comma := strings.Index(s[i:], ",")
		if comma < 0 {
			break
		}
		i += comma + 1
	}
	return m
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

	force, _ := params["force"].(bool)

	if isWindows {
		if force {
			cmd := exec.Command("powershell", "-NoProfile", "-Command",
				fmt.Sprintf("Stop-Process -Id %d -Force", pid))
			return runCmd(cmd)
		}
		// Without force, use taskkill without /F (allows graceful shutdown)
		cmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid))
		return runCmd(cmd)
	}

	sig := resolveSignal(params, force)
	cmd := exec.Command("kill", sig, fmt.Sprintf("%d", pid))
	return runCmd(cmd)
}

func processKillByName(params map[string]any, isWindows bool) ToolResult {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return ToolResult{Output: "Error: pattern is required for pkill", IsError: true}
	}

	force, _ := params["force"].(bool)

	if isWindows {
		safePattern := sanitizePSInput(pattern)
		var cmd *exec.Cmd
		if force {
			cmd = exec.Command("powershell", "-NoProfile", "-Command",
				fmt.Sprintf("Get-Process -Name '*%s*' -ErrorAction SilentlyContinue | Stop-Process -Force", safePattern))
		} else {
			cmd = exec.Command("powershell", "-NoProfile", "-Command",
				fmt.Sprintf("Get-Process -Name '*%s*' -ErrorAction SilentlyContinue | Stop-Process", safePattern))
		}
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

	sig := resolveSignal(params, force)
	cmd := exec.Command("pkill", sig, pattern)
	return runCmd(cmd)
}

// resolveSignal determines the correct signal based on force and signal parameters.
// Priority: force=true → SIGKILL; otherwise use signal param or default SIGTERM.
func resolveSignal(params map[string]any, force bool) string {
	if force {
		return "-9"
	}
	if s, ok := params["signal"].(string); ok && s != "" {
		// Numeric signal like "9" or "15"
		if _, err := strconv.Atoi(s); err == nil {
			return "-" + s
		}
		// Named signal: strip "SIG" prefix for kill command
		sigName := strings.TrimPrefix(s, "SIG")
		return "-" + sigName
	}
	return "-SIGTERM"
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
