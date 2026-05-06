package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// SystemTool provides system information and monitoring.
type SystemTool struct{}

func (*SystemTool) Name() string    { return "system" }
func (*SystemTool) Description() string {
	return "Get system information. Supports info (full system overview), uname, df (disk), free (memory), top (processes), uptime, who, w, hostname, and arch. On Windows, uses PowerShell cmdlets."
}

func (*SystemTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"description": "System operation: info (full overview), uname, df, free, top, uptime, who, w, hostname, arch",
				"enum":        []string{"info", "uname", "df", "free", "top", "uptime", "who", "w", "hostname", "arch"},
			},
			"flags": map[string]any{
				"type":        "string",
				"description": "Additional flags for the command (Unix only)",
			},
			"lines": map[string]any{
				"type":        "integer",
				"description": "Number of lines to show for top (default: 10)",
			},
		},
		"required": []string{"operation"},
	}
}

func (*SystemTool) CheckPermissions(params map[string]any) string { return "" }

func (*SystemTool) Execute(params map[string]any) ToolResult {
	return systemExecute(context.Background(), params)
}

func (*SystemTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	return systemExecute(ctx, params)
}

func systemExecute(ctx context.Context, params map[string]any) ToolResult {
	operation, _ := params["operation"].(string)
	if operation == "" {
		return ToolResult{Output: "Error: operation is required", IsError: true}
	}

	var result ToolResult
	switch operation {
	case "info":
		result = systemInfo()
	case "uname":
		result = systemUname(params)
	case "df":
		result = systemDF(params)
	case "free":
		result = systemFree(params)
	case "top":
		result = systemTop(params)
	case "uptime":
		result = systemUptime()
	case "who":
		result = systemWho()
	case "w":
		result = systemW()
	case "hostname":
		result = systemHostname()
	case "arch":
		result = systemArch()
	default:
		return ToolResult{Output: fmt.Sprintf("Error: unknown operation: %s", operation), IsError: true}
	}
	return result
}

// systemInfo returns a comprehensive system overview.
func systemInfo() ToolResult {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			`$os = Get-CimInstance Win32_OperatingSystem; $cpu = Get-CimInstance Win32_Processor; $mem = [math]::Round($os.TotalVisibleMemorySize/1MB,2); $free = [math]::Round($os.FreePhysicalMemory/1MB,2); $used = [math]::Round($mem - $free, 2); $bootTime = $os.LastBootUpTime; $now = Get-Date; $diff = New-TimeSpan -Start $bootTime -End $now; $d = [math]::Floor($diff.TotalDays); $h = $diff.Hours; $m = $diff.Minutes; $uptimeStr = if ($d -gt 0) { "{0} days, {1}:{2}" -f $d, $h.ToString("00"), $m.ToString("00") } else { "{0}:{1}" -f $h.ToString("00"), $m.ToString("00") }; Write-Output "OS:       $($os.Caption) $($os.Version)"; Write-Output "Host:     $env:COMPUTERNAME"; Write-Output "Arch:     $($cpu.Name)"; Write-Output "CPU:      $($cpu.NumberOfCores) cores / $($cpu.NumberOfLogicalProcessors) threads"; Write-Output "Memory:   ${used}GB used / ${mem}GB total ($(${free})GB free)"; Write-Output "Uptime:   $uptimeStr"`)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
		}
		return ToolResult{Output: strings.TrimSpace(stdout.String())}
	}

	// Unix: combine uname, uptime, memory, cpu info
	var parts []string
	// uname -a
	if out, err := exec.Command("uname", "-a").CombinedOutput(); err == nil {
		parts = append(parts, "OS:       "+strings.TrimSpace(string(out)))
	}
	// uptime
	if out, err := exec.Command("uptime").CombinedOutput(); err == nil {
		parts = append(parts, "Uptime:   "+strings.TrimSpace(string(out)))
	}
	// cpu info (nproc)
	if out, err := exec.Command("nproc").CombinedOutput(); err == nil {
		parts = append(parts, "CPU:      "+strings.TrimSpace(string(out))+" cores")
	}
	// memory (free -h, first 2 lines)
	if out, err := exec.Command("free", "-h").CombinedOutput(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) >= 2 {
			parts = append(parts, "Memory:   "+strings.TrimSpace(lines[1]))
		}
	}
	return ToolResult{Output: strings.Join(parts, "\n")}
}

// systemUname runs `uname` on Unix or `systeminfo` equivalent on Windows.
func systemUname(params map[string]any) ToolResult {
	flags, _ := params["flags"].(string)

	if runtime.GOOS == "windows" {
		var args []string
		if flags != "" {
			args = strings.Fields(flags)
		}
		if len(args) == 0 || (len(args) == 1 && args[0] == "-a") {
			// Return OS name, hostname, kernel version (like uname -a on Unix)
			cmd := exec.Command("powershell", "-NoProfile", "-Command",
				"$h = $env:COMPUTERNAME; "+
					"$os = (Get-CimInstance Win32_OperatingSystem).Caption -replace 'Microsoft ', ''; "+
					"$v = (Get-CimInstance Win32_OperatingSystem).Version; "+
					"$arch = (Get-CimInstance Win32_Processor)[0].Name; "+
					"Write-Output \"Windows $h $os $v $arch\"")
			out, err := cmd.CombinedOutput()
			if err != nil {
				return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, string(out)), IsError: true}
			}
			return ToolResult{Output: strings.TrimSpace(string(out))}
		}
		cmd := exec.Command("hostname")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
		}
		return ToolResult{Output: strings.TrimSpace(string(out))}
	}

	cmdArgs := []string{"uname"}
	if flags != "" {
		cmdArgs = append(cmdArgs, strings.Fields(flags)...)
	} else {
		cmdArgs = append(cmdArgs, "-a")
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
	}
	return ToolResult{Output: strings.TrimSpace(stdout.String())}
}

// systemDF runs `df` on Unix or PowerShell Get-PSDrive on Windows.
func systemDF(params map[string]any) ToolResult {
	flags, _ := params["flags"].(string)

	if runtime.GOOS == "windows" {
		psScript := `Get-PSDrive -PSProvider FileSystem | Format-Table Name, @{N='Used GB';E={[math]::Round($_.Used/1GB,2)}}, @{N='Free GB';E={[math]::Round($_.Free/1GB,2)}}, @{N='Total GB';E={[math]::Round(($_.Used+$_.Free)/1GB,2)}} -AutoSize`
		cmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
		}
		return ToolResult{Output: strings.TrimSpace(stdout.String())}
	}

	cmdArgs := []string{"df"}
	if flags != "" {
		cmdArgs = append(cmdArgs, strings.Fields(flags)...)
	} else {
		cmdArgs = append(cmdArgs, "-h")
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
	}
	return ToolResult{Output: strings.TrimSpace(stdout.String())}
}

// systemFree runs `free` on Unix or parses memory via PowerShell on Windows.
func systemFree(params map[string]any) ToolResult {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			`$os = Get-CimInstance Win32_OperatingSystem; $total = [math]::Round($os.TotalVisibleMemorySize/1MB, 2); $free = [math]::Round($os.FreePhysicalMemory/1MB, 2); $used = [math]::Round($total - $free, 2); Write-Output "              total        used        free      shared  buff/cache   available"; Write-Output ("Mem:      {0,8}GB    {1,8}GB    {2,8}GB    {3,8}    {4,8}      {5,8}GB" -f $total, $used, $free, 0, 0, $free)`)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
		}
		return ToolResult{Output: strings.TrimSpace(stdout.String())}
	}

	if runtime.GOOS == "darwin" {
		return systemFreeDarwin()
	}

	flags, _ := params["flags"].(string)
	cmdArgs := []string{"free"}
	if flags != "" {
		cmdArgs = append(cmdArgs, strings.Fields(flags)...)
	} else {
		cmdArgs = append(cmdArgs, "-h")
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
	}
	return ToolResult{Output: strings.TrimSpace(stdout.String())}
}

// systemFreeDarwin provides memory info for macOS using vm_stat.
func systemFreeDarwin() ToolResult {
	pageSizeCmd := exec.Command("sysctl", "-n", "hw.pagesize")
	var psOut bytes.Buffer
	pageSizeCmd.Stdout = &psOut
	pageSize := int64(4096)
	if err := pageSizeCmd.Run(); err == nil {
		if ps, err := strconv.ParseInt(strings.TrimSpace(psOut.String()), 10, 64); err == nil {
			pageSize = ps
		}
	}

	memCmd := exec.Command("sysctl", "-n", "hw.memsize")
	var memOut bytes.Buffer
	memCmd.Stdout = &memOut
	totalMem := int64(0)
	if err := memCmd.Run(); err == nil {
		if tm, err := strconv.ParseInt(strings.TrimSpace(memOut.String()), 10, 64); err == nil {
			totalMem = tm
		}
	}

	cmd := exec.Command("vm_stat")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
	}

	var freePages, activePages, inactivePages, speculativePages, wiredPages, compressedPages int64
	lines := strings.Split(stdout.String(), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			value := strings.TrimSuffix(parts[2], ".")
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				switch {
				case strings.Contains(line, "Pages free:"):
					freePages = v
				case strings.Contains(line, "Pages active:"):
					activePages = v
				case strings.Contains(line, "Pages inactive:"):
					inactivePages = v
				case strings.Contains(line, "Pages speculative:"):
					speculativePages = v
				case strings.Contains(line, "Pages wired down:"):
					wiredPages = v
				case strings.Contains(line, "Pages stored in compressor:"):
					compressedPages = v
				}
			}
		}
	}

	freeMem := (freePages + speculativePages) * pageSize
	usedMem := (activePages + wiredPages + compressedPages) * pageSize
	cacheMem := inactivePages * pageSize
	availableMem := freeMem + cacheMem

	formatBytes := func(b int64) string {
		const unit = int64(1024)
		if b < unit {
			return fmt.Sprintf("%dB", b)
		}
		div, exp := unit, 0
		for n := b / unit; n >= unit; n /= unit {
			div *= unit
			exp++
		}
		return fmt.Sprintf("%.1f%c", float64(b)/float64(div), "KMGTPE"[exp])
	}

	output := fmt.Sprintf(`              total        used        free      shared  buff/cache   available
Mem:      %8s    %8s    %8s    %8s    %8s    %8s
Swap:     %8s    %8s    %8s
`,
		formatBytes(totalMem),
		formatBytes(usedMem),
		formatBytes(freeMem),
		formatBytes(0),
		formatBytes(cacheMem),
		formatBytes(availableMem),
		"?", "?", "?")
	return ToolResult{Output: output}
}

// systemTop runs `top` on Unix or Get-Process on Windows.
func systemTop(params map[string]any) ToolResult {
	lines := 10
	if l, ok := params["lines"].(float64); ok {
		lines = int(l)
	}
	if lines <= 0 {
		lines = 10
	}

	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf(`Get-Process | Sort-Object CPU -Descending | Select-Object -First %d Id, ProcessName, CPU, @{N='Memory MB';E={[math]::Round($_.WorkingSet64/1MB,1)}} | Format-Table -AutoSize`, lines))
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
		}
		return ToolResult{Output: strings.TrimSpace(stdout.String())}
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

// systemUptime runs `uptime` on Unix or computes from LastBootUpTime on Windows.
func systemUptime() ToolResult {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			`(Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime | ForEach-Object { $d = [math]::Floor($_.TotalDays); $h = [math]::Floor($_.Hours); $m = $_.Minutes; if ($d -gt 0) { Write-Output "$d days, $h hours, $m minutes" } else { Write-Output "$h hours, $m minutes" } }`)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
		}
		return ToolResult{Output: strings.TrimSpace(stdout.String())}
	}
	cmd := exec.Command("uptime")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
	}
	return ToolResult{Output: strings.TrimSpace(stdout.String())}
}

// systemWho shows logged-in users.
func systemWho() ToolResult {
	if runtime.GOOS == "windows" {
		// Primary: use Get-CimInstance to find currently logged-in users
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			`$sessions = Get-CimInstance Win32_LoggedOnUser | Where-Object { $_.StartTime -ne $null } | Group-Object Antecedent | ForEach-Object { $user = $_.Group[0].Antecedent.Name; $session = $_.Group[0].Dependent.StartTime; if ($session) { "$user  pts/0  $(Get-Date $session -Format 'yyyy-MM-dd HH:mm')" } }; if ($sessions) { $sessions } else { "No users logged in." }`)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err == nil {
			out := strings.TrimSpace(stdout.String())
			if out != "" && !strings.Contains(out, "error") {
				return ToolResult{Output: out}
			}
		}
		// Fallback: try query user (may not exist on Windows Home)
		cmd2 := exec.Command("powershell", "-NoProfile", "-Command",
			`$u = Get-CimInstance Win32_ComputerSystem | Select-Object -ExpandProperty UserName; if ($u) { "$u  pts/0  $(Get-Date -Format 'yyyy-MM-dd HH:mm')" } else { "No users logged in." }`)
		var out2 bytes.Buffer
		cmd2.Stdout = &out2
		cmd2.Run()
		users := strings.TrimSpace(out2.String())
		if users != "" {
			return ToolResult{Output: users}
		}
		return ToolResult{Output: "No users logged in."}
	}
	cmd := exec.Command("who")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
	}
	return ToolResult{Output: strings.TrimSpace(stdout.String())}
}

// systemW shows who is logged in and what they are doing.
func systemW() ToolResult {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			`Get-Process | Where-Object { $_.MainWindowTitle -ne "" } | Select-Object -First 10 Id, ProcessName, @{N='CPU(s)';E={$_.CPU}}, @{N='Window';E={$_.MainWindowTitle}} | Format-Table -AutoSize`)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
		}
		return ToolResult{Output: strings.TrimSpace(stdout.String())}
	}
	cmd := exec.Command("w")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
	}
	return ToolResult{Output: strings.TrimSpace(stdout.String())}
}

// systemHostname returns the hostname.
func systemHostname() ToolResult {
	cmd := exec.Command("hostname")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
	}
	return ToolResult{Output: strings.TrimSpace(stdout.String())}
}

// systemArch returns the system architecture.
func systemArch() ToolResult {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			`(Get-CimInstance Win32_Processor).Architecture | ForEach-Object { switch($_){0{'x86'}4{'x64'}5{'ARM'}9{'x64'}12{'ARM64'}default{$_} } }`)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
		}
		return ToolResult{Output: strings.TrimSpace(stdout.String())}
	}
	cmd := exec.Command("arch")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		cmd2 := exec.Command("uname", "-m")
		var out2 bytes.Buffer
		cmd2.Stdout = &out2
		if err2 := cmd2.Run(); err2 == nil {
			return ToolResult{Output: strings.TrimSpace(out2.String())}
		}
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, stderr.String()), IsError: true}
	}
	return ToolResult{Output: strings.TrimSpace(stdout.String())}
}
