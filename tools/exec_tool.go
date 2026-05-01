package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// BashBgTaskCallback is called when a command should run in the background.
// Returns (taskID, outputFilePath, errorText).
type BashBgTaskCallback func(command, workingDir string) (taskID, outputFilePath, errText string)

// ExecTool executes shell commands with security guards.
type ExecTool struct {
	// BackgroundTaskCallback, when set, enables run_in_background support.
	// When nil, background requests fall through to foreground execution.
	BackgroundTaskCallback BashBgTaskCallback
}

func (*ExecTool) Name() string { return "exec" }
func (*ExecTool) Description() string {
	return "Execute a shell command. Use for package installs, test runners, build commands, git operations, and any shell task. " +
		"Do NOT use exec for file reading (use read_file), file searching (use grep or glob), or file editing (use edit_file). " +
		"Commands run in the current working directory. " +
		"On Windows, use PowerShell syntax. On Unix, use bash syntax. " +
		"Supports running commands in the background with run_in_background=true."
}

func (*ExecTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute.",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "Working directory for the command (default: current directory).",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 600, max 600).",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "Set to true to run this command in the background. Returns immediately with a task ID. Use task_output to read results later.",
			},
		},
		"required": []string{"command"},
	}
}

var denyRegexps = compileDenyPatterns()

func compileDenyPatterns() []*regexp.Regexp {
	patterns := []string{
		`\brm\s+-[rf]{1,2}\b`,                       // rm -r, rm -rf
		`\bdel\s+/[fq]\b`,                            // del /f, del /q
		`\brmdir\s+/s\b`,                             // rmdir /s
		`(?:^|[;&|]\s*)format\b`,                     // format (as standalone command only)
		`\b(mkfs|diskpart)\b`,                        // disk formatting
		`\bdd\s+.*\bof=`,                             // dd with output
		`>\s*/dev/sd`,                                // write to disk device
		`\b(shutdown|reboot|poweroff)\b`,             // power operations
		`:\(\)\s*\{.*\};\s*:`,                        // fork bomb
		`\w+\(\)\s*\{[^}]*\|\s*[^}]*&\s*\}\s*;\s*`,   // fork bomb variation
		`&\S*&\S*&`,                                  // chained background processes
	}
	result := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		result[i] = regexp.MustCompile("(?i)" + p)
	}
	return result
}

func (*ExecTool) CheckPermissions(params map[string]any) string {
	cmd, _ := params["command"].(string)
	cmd = strings.TrimSpace(cmd)
	lower := strings.ToLower(cmd)

	// First, check deny patterns against the FULL command (before splitting).
	// Some patterns like fork bombs span multiple segments and must be checked whole.
	for _, re := range denyRegexps {
		if re.MatchString(lower) {
			return "Dangerous command pattern detected: " + re.String()
		}
	}

	// Warn about commands accessing internal/private URLs (check full command)
	if containsInternalURL(cmd) {
		return "Internal/private URL detected"
	}

	// Check each subcommand of a compound command independently
	subcmds := splitCompoundCommand(cmd)
	for _, sub := range subcmds {
		sub = strings.TrimSpace(sub)
		if sub == "" {
			continue
		}

		// Strip safe wrappers to validate the actual command
		inner := stripSafeWrappers(sub)

		// Command substitution detection
		if reason := detectCommandSubstitution(inner); reason != "" {
			return reason
		}

		// Glob and brace expansion in destructive commands
		if reason := detectExpansion(inner); reason != "" {
			return reason
		}

		// Validate file paths in the command
		if pathViolation := validatePaths(inner); pathViolation != "" {
			return pathViolation
		}
	}

	return ""
}

func (et *ExecTool) Execute(params map[string]any) ToolResult {
	// Check for background execution request
	if bg, ok := params["run_in_background"].(bool); ok && bg {
		return et.execInBackground(params)
	}
	return execToolExecute(context.Background(), params)
}

// ExecuteContext runs the command with context support for cancellation.
func (et *ExecTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	// Check for background execution request
	if bg, ok := params["run_in_background"].(bool); ok && bg {
		return et.execInBackground(params)
	}
	return execToolExecute(ctx, params)
}

// execInBackground handles the run_in_background=true case.
// It delegates to the BackgroundTaskCallback to spawn the process and track it.
func (et *ExecTool) execInBackground(params map[string]any) ToolResult {
	command, _ := params["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return ToolResult{Output: "Error: empty command", IsError: true}
	}

	if et.BackgroundTaskCallback == nil {
		// Fallback: run in foreground if callback not set
		return execToolExecute(context.Background(), params)
	}

	// Determine working directory
	wd, _ := params["working_dir"].(string)
	if wd == "" {
		wd, _ = os.Getwd()
	}

	taskID, outputFile, errText := et.BackgroundTaskCallback(command, wd)
	if errText != "" {
		return ToolResult{Output: errText, IsError: true}
	}

	return ToolResult{
		Output: fmt.Sprintf("Background task started.\nTask ID: %s\nOutput file: %s\nUse the task_output tool to check results when ready.", taskID, outputFile),
		IsError: false,
	}.WithMetadata(NewToolResultMetadata("exec", 0))
}

func execToolExecute(ctx context.Context, params map[string]any) ToolResult {
	command, _ := params["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return ToolResult{Output: "Error: empty command", IsError: true}
	}

	timeout := 600
	if t, ok := params["timeout"]; ok {
		switch v := t.(type) {
		case float64:
			timeout = int(v)
		case int:
			timeout = v
		}
	}
	if timeout <= 0 {
		timeout = 600
	}
	if timeout > 600 {
		timeout = 600
	}

	var shell, flag string
	if runtime.GOOS == "windows" {
		// Prefer PowerShell on Windows, then bash (Git Bash), then cmd
		if _, err := exec.LookPath("powershell"); err == nil {
			shell, flag = "powershell", "-Command"
		} else if _, err := exec.LookPath("bash"); err == nil {
			shell, flag = "bash", "-c"
		} else {
			shell, flag = "cmd", "/C"
		}
	} else {
		shell, flag = "bash", "-c"
	}

	// Determine working directory
	wd, _ := params["working_dir"].(string)
	if wd == "" {
		wd, _ = os.Getwd()
	} else {
		wd = expandPath(wd)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, flag, command)
	cmd.Dir = wd
	cmd.Stdin = nil // Isolate from REPL stdin to prevent interactive prompts

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	if err := cmd.Start(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	// Read outputs concurrently
	type readResult struct {
		data string
		isStderr bool
	}
	outputCh := make(chan readResult, 2)

	go func() {
		data := readLimited(stdout, 50000)
		outputCh <- readResult{data, false}
	}()
	go func() {
		data := readLimited(stderr, 25000)
		outputCh <- readResult{"STDERR:\n" + data, true}
	}()

	errCh := make(chan error, 1)
	go func() { errCh <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		<-errCh
		return ToolResult{Output: fmt.Sprintf("Error: command timed out after %ds. Consider using run_in_background: true for long-running commands.", timeout), IsError: true}
	case err := <-errCh:
		var stdoutOut, stderrOut string
		for i := 0; i < 2; i++ {
			r := <-outputCh
			if r.isStderr {
				stderrOut = r.data
			} else {
				stdoutOut = r.data
			}
		}

		var result strings.Builder
		if stdoutOut != "" {
			result.WriteString(stdoutOut)
		}
		if stderrOut != "" && stderrOut != "STDERR:\n" {
			if result.Len() > 0 {
				result.WriteString("\n")
			}
			result.WriteString(stderrOut)
		}

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}
		}
		result.WriteString(fmt.Sprintf("\nExit code: %d", exitCode))

		if result.Len() == 0 {
			result.WriteString("(no output)")
		}

		// Truncate if too large
		output := result.String()
		const maxOutput = 50000
		if len(output) > maxOutput {
			half := maxOutput / 2
			truncated := len(output) - maxOutput
			output = output[:half] + fmt.Sprintf("\n\n... (%d chars truncated) ...\n\n", truncated) + output[len(output)-half:]
		}

		return ToolResult{Output: output, IsError: err != nil && !isExitError(err)}
	}
}

func isExitError(err error) bool {
	_, ok := err.(*exec.ExitError)
	return ok
}

func readLimited(r interface{ Read([]byte) (int, error) }, limit int) string {
	buf := make([]byte, limit)
	off := 0
	for {
		n, err := r.Read(buf[off:])
		off += n
		if err != nil {
			break
		}
		if off >= limit {
			break
		}
	}
	return string(buf[:off])
}

var internalURLPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)https?://(localhost|127\.0\.0\.1|0\.0\.0\.0|192\.168\.\d+\.\d+|10\.\d+\.\d+\.\d+|172\.(1[6-9]|2\d|3[01])\.\d+\.\d+)[:/]`),
	regexp.MustCompile(`(?i)https?://[0-9]+(?:\.[0-9]+){3}:\d+`),
}

// containsInternalURL checks for internal/private URLs.
func containsInternalURL(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, re := range internalURLPatterns {
		if re.MatchString(lower) {
			return true
		}
	}
	return false
}

// safeVarPattern matches known-safe environment variable expansions in ${...} form.
// These are common environment variables that do not execute arbitrary code.
var safeVarNames = map[string]bool{
	// Common environment variables
	"HOME": true, "PATH": true, "USER": true, "PWD": true, "SHELL": true,
	"TERM": true, "EDITOR": true, "VISUAL": true, "LANG": true, "LC_ALL": true,
	"GOPATH": true, "GOROOT": true, "JAVA_HOME": true, "NODE_PATH": true,
	"PYTHONPATH": true, "VIRTUAL_ENV": true, "CONDA_PREFIX": true,
	"CARGO_HOME": true, "RUSTUP_HOME": true, "GOPROXY": true, "GOFLAGS": true,
	"GOOS": true, "GOARCH": true,
	// Git environment variables
	"GIT_DIR": true, "GIT_WORK_TREE": true,
	"GIT_AUTHOR_NAME": true, "GIT_AUTHOR_EMAIL": true,
	"GIT_COMMITTER_NAME": true, "GIT_COMMITTER_EMAIL": true,
	// CI environment variables
	"CI": true, "GITHUB_ACTIONS": true, "TRAVIS": true,
	"CIRCLECI": true, "JENKINS_URL": true,
}

// safeSpecialVars matches special shell variables (positional, special)
var safeSpecialVars = regexp.MustCompile(`^\d+$|^[\?#!@*0]$`)

// detectCommandSubstitution detects shell injection via command substitution patterns.
// Returns a descriptive reason string if dangerous patterns are found, empty string otherwise.
func detectCommandSubstitution(cmd string) string {
	// Always flag $(
	if strings.Contains(cmd, "$(") {
		return "Command substitution $(...) detected: may execute arbitrary commands"
	}

	// Always flag backticks
	if strings.Contains(cmd, "`") {
		return "Backtick command substitution detected: may execute arbitrary commands"
	}

	// Always flag process substitution
	if strings.Contains(cmd, "<(") || strings.Contains(cmd, ">(") {
		return "Process substitution detected: may execute arbitrary commands"
	}

	// Always flag $((  (arithmetic expansion can contain command substitution)
	if strings.Contains(cmd, "$((") || strings.Contains(cmd, "$(( ") {
		return "Arithmetic expansion detected: may contain command substitution"
	}

	// Flag ${...} UNLESS it matches a known-safe variable
	idx := strings.Index(cmd, "${")
	for idx >= 0 {
		// Find the closing }
		end := idx + 2
		depth := 1
		for end < len(cmd) && depth > 0 {
			if cmd[end] == '{' {
				depth++
			} else if cmd[end] == '}' {
				depth--
			}
			end++
		}
		if depth == 0 && end <= len(cmd) {
			varPart := cmd[idx+2 : end-1]
			// Check if this is a safe variable (simple name or special var)
			if !isSafeVariable(varPart) {
				return "Variable expansion ${...} detected: may execute arbitrary commands"
			}
		} else {
			// No closing brace found, skip
			end = idx + 2
		}
		// Find next ${
		idx = strings.Index(cmd[end:], "${")
		if idx >= 0 {
			idx = end + idx
		}
	}

	return ""
}

// isSafeVariable checks if a variable name inside ${...} is a known safe variable.
func isSafeVariable(name string) bool {
	// Check for simple safe env var names
	if safeVarNames[name] {
		return true
	}

	// Check for special shell variables: $?, $$, $!, $0, $#, $@, $*
	if safeSpecialVars.MatchString(name) {
		return true
	}

	// Check for safe prefix patterns like ${VAR:-default}, ${VAR:=default}, ${VAR:?error}
	if colonIdx := strings.Index(name, ":"); colonIdx >= 0 {
		return isSafeVariable(name[:colonIdx])
	}

	return false
}

// safeGlobPrefixCommands are commands after which glob expansion is considered dangerous.
var safeGlobPrefixCommands = map[string]bool{
	"rm": true, "mv": true, "cp": true, "chmod": true, "chown": true,
	"git rm": true, "git clean": true, "git add": true,
}

// detectExpansion detects glob patterns and brace expansion that could be dangerous.
// Only flags these patterns when used with potentially destructive commands.
func detectExpansion(cmd string) string {
	// Extract the base command (first word, stripping leading env vars and wrappers)
	base := extractBaseCommand(cmd)

	// Check if this is a potentially destructive command
	destructive := safeGlobPrefixCommands[base]
	if !destructive {
		return ""
	}

	// Now check for dangerous glob/brace patterns
	quoted := extractQuotedRegions(cmd)

	// Check for unquoted glob characters
	for _, ch := range []string{"*", "?"} {
		idx := 0
		for idx < len(cmd) {
			pos := strings.Index(cmd[idx:], ch)
			if pos < 0 {
				break
			}
			absPos := idx + pos
			if !quoted[absPos] {
				return fmt.Sprintf("Unquoted glob pattern %q detected in destructive command %q: may expand to unintended arguments", ch, base)
			}
			idx = absPos + 1
		}
	}

	// Check for unquoted [...] character classes
	idx := 0
	for idx < len(cmd) {
		pos := strings.Index(cmd[idx:], "[")
		if pos < 0 {
			break
		}
		absPos := idx + pos
		if !quoted[absPos] {
			// Check it's actually a glob bracket, not a shell negation
			endPos := absPos + 1
			if endPos < len(cmd) && cmd[endPos] == '!' {
				endPos++
			}
			if endPos < len(cmd) && cmd[endPos] != ']' {
				// Looks like a glob bracket pattern
				closePos := strings.Index(cmd[endPos:], "]")
				if closePos > 0 {
					return fmt.Sprintf("Unquoted glob bracket pattern detected in destructive command %q: may expand to unintended arguments", base)
				}
			}
		}
		idx = absPos + 1
	}

	// Check for unquoted brace expansion {a,b} or {1..10}
	idx = 0
	for idx < len(cmd) {
		pos := strings.Index(cmd[idx:], "{")
		if pos < 0 {
			break
		}
		absPos := idx + pos
		if !quoted[absPos] {
			// Look for closing }
			end := absPos + 1
			for end < len(cmd) && cmd[end] != '}' {
				end++
			}
			if end < len(cmd) && end > absPos+1 {
				inner := cmd[absPos+1 : end]
				// Brace expansion: contains comma or .. with digits
				if strings.Contains(inner, ",") || regexp.MustCompile(`^\d+\.\.\d+$`).MatchString(inner) {
					return fmt.Sprintf("Unquoted brace expansion {%s} detected in destructive command %q: may expand to many arguments", inner, base)
				}
			}
		}
		idx = absPos + 1
	}

	return ""
}

// extractBaseCommand extracts the base command name from a command string,
// stripping env variable assignments and common wrappers.
// For git subcommands, returns "git <subcmd>" (e.g., "git rm").
func extractBaseCommand(cmd string) string {
	// First strip env prefix (e.g., FOO=bar cmd)
	trimmed := cmd
	if idx := strings.Index(trimmed, "="); idx > 0 {
		beforeEq := strings.TrimSpace(trimmed[:idx])
		afterEq := strings.TrimSpace(trimmed[idx+1:])
		// If the part before = has no spaces, it's likely VAR=value
		if !strings.Contains(beforeEq, " ") && afterEq != "" && !strings.Contains(string(afterEq[0]), "=") {
			trimmed = afterEq
			// Handle chained assignments like FOO=bar BAR=baz cmd
			for {
				nextEq := strings.Index(trimmed, "=")
				if nextEq < 0 {
					break
				}
				beforeEq := strings.TrimSpace(trimmed[:nextEq])
				afterEq := strings.TrimSpace(trimmed[nextEq+1:])
				if !strings.Contains(beforeEq, " ") && afterEq != "" && !strings.Contains(string(afterEq[0]), "=") {
					trimmed = afterEq
				} else {
					break
				}
			}
		}
	}

	// Now strip common wrappers
	wrappers := []string{"timeout", "nice", "nohup", "time", "stdbuf", "ionice", "env", "command", "builtin", "unbuffer"}
	fields := strings.Fields(trimmed)
	result := []string{}
	skipNext := false
	for i, f := range fields {
		if skipNext {
			skipNext = false
			continue
		}
		if i == 0 {
			// Extract just the base name (strip path)
			base := filepath.Base(f)
			result = append(result, base)
		} else {
			// Check if this field is a wrapper with optional numeric arg
			wrapperMatched := false
			for _, w := range wrappers {
				if f == w {
					wrapperMatched = true
					break
				}
				// Check for wrapper -n, -i, etc.
				if strings.HasPrefix(f, "-") && (f == "-n" || f == "-i" || f == "-c" || f == "-o" || f == "-e") {
					// These take an argument, skip next field too
					skipNext = true
					wrapperMatched = true
					break
				}
			}
			if !wrapperMatched {
				result = append(result, f)
			}
		}
	}

	if len(result) == 0 {
		return trimmed
	}

	// For git, include the subcommand
	base := result[0]
	if base == "git" && len(result) > 1 {
		return base + " " + result[1]
	}
	return base
}

// extractQuotedRegions returns a map of positions that are inside quotes.
// true = inside quotes, false = outside quotes.
func extractQuotedRegions(cmd string) map[int]bool {
	result := make(map[int]bool)
	inSingle := false
	inDouble := false
	inBacktick := false

	for i := 0; i < len(cmd); i++ {
		ch := cmd[i]
		switch ch {
		case '\'':
			if !inDouble && !inBacktick {
				inSingle = !inSingle
			}
		case '`':
			if !inSingle && !inDouble {
				inBacktick = !inBacktick
			}
		case '"':
			if !inSingle && !inBacktick {
				inDouble = !inDouble
			}
		}
		if inSingle || inDouble || inBacktick {
			result[i] = true
		}
	}
	return result
}

// validatePaths checks for potentially dangerous path references in commands.
// Returns an empty string if safe, or a descriptive reason if dangerous.
func validatePaths(cmd string) string {
	// Stub implementation - expand with actual path validation logic
	_ = cmd
	return ""
}

// splitCompoundCommand splits a command string on shell separators while respecting quoting.
// Handles ;, &&, ||, |, and newlines (including backslash continuation).
func splitCompoundCommand(cmd string) []string {
	// Normalize: join lines broken by backslash
	normalized := regexp.MustCompile(`\\\s*\n\s*`).ReplaceAllString(cmd, " ")

	var result []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	inBacktick := false

	for i := 0; i < len(normalized); i++ {
		ch := normalized[i]

		// Handle quote states
		if ch == '\'' && !inDouble && !inBacktick {
			inSingle = !inSingle
		} else if ch == '`' && !inSingle && !inDouble {
			inBacktick = !inBacktick
		} else if ch == '"' && !inSingle && !inBacktick {
			inDouble = !inDouble
		}

		// Only split on separators when outside all quotes
		if !inSingle && !inDouble && !inBacktick {
			if ch == ';' || ch == '&' || ch == '|' || ch == '\n' {
				// Handle && and ||
				if ch == '&' && i+1 < len(normalized) && normalized[i+1] == '&' {
					trimmed := strings.TrimSpace(current.String())
					if trimmed != "" {
						result = append(result, trimmed)
					}
					current.Reset()
					i++
					continue
				}
				if ch == '|' && i+1 < len(normalized) && normalized[i+1] == '|' {
					trimmed := strings.TrimSpace(current.String())
					if trimmed != "" {
						result = append(result, trimmed)
					}
					current.Reset()
					i++
					continue
				}
				// Standalone ; | or newline
				trimmed := strings.TrimSpace(current.String())
				if trimmed != "" {
					result = append(result, trimmed)
				}
				current.Reset()
				continue
			}
		}

		current.WriteByte(ch)
	}

	// Don't forget the last segment
	trimmed := strings.TrimSpace(current.String())
	if trimmed != "" {
		result = append(result, trimmed)
	}

	return result
}

// safeWrapperPrefixes matches common command wrappers that should be stripped.
// skipArgs: number of positional arguments the wrapper takes (after any flags).
// hasFlags: whether the wrapper supports -flag style options.
var safeWrapperPrefixes = []struct {
	prefix   string
	skipArgs int // positional args to skip (e.g., timeout 30 -> skipArgs=1)
	hasFlags bool // whether wrapper takes -flag options
}{
	{"timeout", 1, true},  // timeout [flags] 30 cmd -> skip duration and any flags
	{"nice", 0, true},     // nice [-n 10] cmd -> skip flags and their values
	{"nohup", 0, false},
	{"time", 0, false},
	{"stdbuf", 0, true},   // stdbuf -oL cmd -> skip flags
	{"ionice", 0, true},   // ionice [-c 3] cmd -> skip flags
	{"env", -1, false},    // env VAR=val cmd -> strip all VAR=val assignments
	{"command", 0, false},
	{"builtin", 0, false},
	{"unbuffer", 0, false},
}

// stripSafeWrappers removes common command wrappers to expose the actual command.
// This prevents bypassing security checks by wrapping dangerous commands.
func stripSafeWrappers(cmd string) string {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return trimmed
	}

	// First, handle env prefix: FOO=bar BAR=baz cmd
	// Keep stripping VAR=value pairs until we hit something that isn't
	for {
		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			break
		}
		beforeEq := strings.TrimSpace(trimmed[:eqIdx])
		afterEq := strings.TrimSpace(trimmed[eqIdx+1:])
		// Check if it looks like a VAR=val assignment (no spaces in var name, not empty value)
		if strings.Contains(beforeEq, " ") || afterEq == "" || strings.Contains(afterEq, " ") {
			break
		}
		// Looks like VAR=val, strip it
		trimmed = afterEq
		trimmed = strings.TrimSpace(trimmed)
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return cmd
	}

	// Try to match wrapper prefixes
	remaining := fields
	for {
		if len(remaining) == 0 {
			break
		}
		matched := false
		for _, w := range safeWrapperPrefixes {
			base := filepath.Base(remaining[0])
			if base != w.prefix {
				continue
			}

			// Matched a wrapper
			skip := 1 // skip the wrapper command itself

			if w.skipArgs == -1 {
				// env: skip until we find a field that doesn't contain =
				for skip < len(remaining) && strings.Contains(remaining[skip], "=") {
					skip++
				}
			} else {
				// Skip any -flag style options and their values
				if w.hasFlags {
					for skip < len(remaining) {
						field := remaining[skip]
						if !strings.HasPrefix(field, "-") || field == "--" {
							break
						}
						skip++ // skip the flag
						// Short flags (exactly 2 chars like -n, -c) take a separate value arg.
						// Longer flags like -oL have the value embedded.
						if len(field) == 2 && skip < len(remaining) && !strings.HasPrefix(remaining[skip], "-") {
							skip++ // skip the separate value
						}
					}
				}
				// Skip positional arguments (e.g., timeout's duration)
				for i := 0; i < w.skipArgs && skip < len(remaining); i++ {
					skip++
				}
			}

			if skip < len(remaining) {
				remaining = remaining[skip:]
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}

	return strings.Join(remaining, " ")
}

// IsReadOnly reports whether the given command is read-only (safe to auto-approve).
func (et *ExecTool) IsReadOnly(params map[string]any) bool {
	cmd, _ := params["command"].(string)
	return isReadOnlyCommand(cmd)
}

// CheckDestructiveWarning returns a warning message if the command is known to be
// destructive, or empty string if not. This is informational only, not blocking.
func CheckDestructiveWarning(cmd string) string {
	_, warning := isDestructiveCommand(cmd)
	return warning
}

// isReadOnlyCommand reports whether a command is read-only (no side effects).
func isReadOnlyCommand(cmd string) bool {
	stripped := stripSafeWrappers(cmd)

	// Check for redirects — any write operator means NOT read-only
	if strings.Contains(stripped, ">") || strings.Contains(stripped, ">>") {
		return false
	}

	parts := strings.Fields(stripped)
	if len(parts) == 0 {
		return false
	}

	bin := filepath.Base(parts[0])

	// Commands that are always read-only regardless of flags
	alwaysReadOnly := map[string]bool{
		"ls": true, "cat": true, "head": true, "tail": true, "less": true,
		"more": true, "wc": true, "file": true, "stat": true, "du": true,
		"df": true, "find": true, "which": true, "whereis": true, "locate": true,
		"grep": true, "rg": true, "ag": true, "ack": true,
		"pwd": true, "whoami": true, "id": true, "hostname": true,
		"uname": true, "date": true, "env": true, "printenv": true,
		"realpath": true, "basename": true, "dirname": true,
		"md5sum": true, "sha256sum": true, "sha1sum": true,
		"ps": true, "uptime": true, "free": true,
		"lscpu": true, "lsblk": true, "lsusb": true, "lspci": true,
		"echo": true, "printf": true, "type": true, "tree": true, "jq": true,
	}

	if alwaysReadOnly[bin] {
		return true
	}

	// Git read-only subcommands
	if bin == "git" && len(parts) >= 2 {
		gitRO := map[string]bool{
			"status": true, "diff": true, "log": true, "show": true,
			"blame": true, "ls-files": true, "ls-tree": true,
			"shortlog": true, "describe": true, "rev-parse": true,
			"rev-list": true, "reflog": true, "count-objects": true,
		}
		if gitRO[parts[1]] {
			return true
		}
		// Commands with read-only and mutation variants
		switch parts[1] {
		case "branch":
			for _, p := range parts[2:] {
				if strings.HasPrefix(p, "-") && (strings.Contains(p, "d") || strings.Contains(p, "D") || strings.Contains(p, "m")) {
					return false
				}
			}
			return true
		case "remote":
			if len(parts) > 2 && parts[2] != "-v" {
				return false
			}
			return true
		case "stash":
			if len(parts) <= 2 || parts[2] == "list" || parts[2] == "show" || parts[2] == "--help" {
				return true
			}
			return false
		case "tag":
			for _, p := range parts[2:] {
				if strings.HasPrefix(p, "-") && (strings.Contains(p, "d") || strings.Contains(p, "a") || strings.Contains(p, "v")) {
					return false
				}
			}
			return true
		case "worktree":
			if len(parts) > 2 && parts[2] != "list" {
				return false
			}
			return true
		case "config":
			for _, p := range parts[2:] {
				if p == "--set" || p == "--unset" || p == "--add" {
					return false
				}
			}
			return true
		}
	}

	// npm/pip read-only commands
	if bin == "npm" && len(parts) >= 2 {
		if parts[1] == "list" || parts[1] == "ls" || parts[1] == "view" ||
			parts[1] == "info" || parts[1] == "outdated" || parts[1] == "audit" {
			return true
		}
	}
	if bin == "pip" && len(parts) >= 2 {
		if parts[1] == "list" || parts[1] == "show" || parts[1] == "freeze" {
			return true
		}
	}

	// --help or --version flags are read-only
	for _, flag := range []string{"--help", "-h", "--version", "-V"} {
		if len(parts) >= 2 && parts[len(parts)-1] == flag {
			return true
		}
	}

	return false
}

// isDestructiveCommand checks if a command is known to be destructive.
func isDestructiveCommand(cmd string) (bool, string) {
	stripped := stripSafeWrappers(cmd)
	parts := strings.Fields(stripped)
	if len(parts) == 0 {
		return false, ""
	}
	bin := filepath.Base(parts[0])

	destructive := []struct {
		bin      string
		subCheck string
		msg      string
	}{
		{"rm", "", "File deletion command"},
		{"rmdir", "", "Directory deletion command"},
		{"unlink", "", "File deletion command"},
		{"git", "push --force", "Force push detected"},
		{"git", "push -f", "Force push detected"},
		{"git", "reset --hard", "Hard reset discards local changes"},
		{"git", "clean", "Git clean removes untracked files"},
		{"git", "checkout .", "Discards all local changes"},
		{"git", "stash drop", "Drops a stash entry"},
		{"git", "branch -d", "Deletes a branch"},
		{"git", "branch -D", "Force-deletes a branch"},
		{"kubectl", "delete", "Kubernetes resource deletion"},
		{"docker", "rm", "Container removal"},
		{"docker", "rmi", "Image removal"},
		{"terraform", "destroy", "Infrastructure destruction"},
		{"mysql", "", "Database client command"},
		{"psql", "", "Database client command"},
	}

	for _, d := range destructive {
		if bin != d.bin {
			continue
		}
		if d.subCheck == "" {
			return true, d.msg
		}
		if strings.Contains(strings.ToLower(stripped), d.subCheck) {
			return true, d.msg
		}
	}

	// Package manager install/remove
	installBins := map[string]bool{
		"apt-get": true, "apt": true, "yum": true, "dnf": true,
		"brew": true, "choco": true, "scoop": true, "winget": true,
		"pacman": true, "zypper": true, "apk": true,
	}
	if installBins[bin] && len(parts) >= 2 {
		installSubs := map[string]bool{
			"install": true, "remove": true, "purge": true,
			"uninstall": true, "erase": true, "upgrade": true,
			"dist-upgrade": true,
		}
		if installSubs[parts[1]] {
			return true, "System package manager command"
		}
	}

	return false, ""
}
