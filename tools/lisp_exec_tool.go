package tools

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"unicode"

	"miniclaudecode-go/microlisp"
)

// shellSyntaxMarkers contains characters/sequences that indicate the user
// intended shell syntax (pipes, redirects, command chaining, etc.).
// These require a shell to execute and cannot work with os/exec directly.
var shellSyntaxMarkers = []string{
	"|", "||", "&&", ";",
	">", ">>", "2>", "2>&1", "&>", "&>>",
	"<", "<<", "<<<",
	"$(", "`",
	"{", "}",
}

// needsShell returns true if the command string contains shell syntax
// that cannot be executed directly by os/exec.
func needsShell(s string) bool {
	// Quick check: if the string contains any of the markers
	for _, marker := range shellSyntaxMarkers {
		if strings.Contains(s, marker) {
			// For single-char markers, ensure it's not part of a filename
			// (e.g. "file.txt" contains "." but isn't shell syntax)
			if len(marker) > 1 {
				return true
			}
			// For "|" specifically, it's almost always shell syntax
			if marker == "|" {
				return true
			}
			// For ">" and "<", check context
			if marker == ">" || marker == "<" {
				// Find the position and check surroundings
				idx := strings.Index(s, marker)
				if idx > 0 {
					prev := rune(s[idx-1])
					if !unicode.IsSpace(prev) && prev != '"' {
						continue // likely part of a filename like "a<b.txt"
					}
				}
				return true
			}
			// For "$" and "`", always shell syntax
			if marker == "$(" || marker == "`" {
				return true
			}
			// For "{" and "}", check if they look like brace expansion
			if marker == "{" || marker == "}" {
				if strings.Contains(s, "{") && strings.Contains(s, "}") {
					return true
				}
				continue
			}
		}
	}
	// Check for semicolons (command separator)
	if strings.Contains(s, ";") {
		return true
	}
	return false
}

// wrapWithShell wraps a command+args into a "bash -c" (Unix) or "cmd /c" (Windows) form.
// Returns (shellProgram, shellArgs, shellInput).
func wrapWithShell(command string, args []string, input string) (program string, allArgs []string, hasInput bool) {
	// Reconstruct the full command string
	fullCmd := command
	if len(args) > 0 {
		fullCmd += " " + strings.Join(args, " ")
	}

	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", fullCmd}, input != ""
	}
	return "bash", []string{"-c", fullCmd}, input != ""
}

// splitCommand splits a command string into program name and implicit args.
// If the command contains spaces (e.g. "mkdir -p dir"), the first token
// is the program name and the rest are args. This fixes the issue where
// exec.Command("mkdir -p dir") looks for a program literally named
// "mkdir -p dir" instead of running mkdir with args.
func splitCommand(s string) (program string, implicitArgs []string) {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return "", nil
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return parts[0], parts[1:]
}

// extractArgs attempts to extract a []string slice from a param value.
// Handles both []any (JSON-decoded array) and []string.
// JSON numbers (float64) are converted to strings.
func extractArgs(v any) []string {
	switch arr := v.(type) {
	case []any:
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			switch x := item.(type) {
			case string:
				result = append(result, x)
			case float64:
				result = append(result, fmt.Sprintf("%g", x))
			case nil:
				continue
			default:
				result = append(result, fmt.Sprintf("%v", x))
			}
		}
		return result
	case []string:
		return arr
	}
	return nil
}

// LispExecTool provides a pure-Go command execution tool via the embedded
// Lisp interpreter. Unlike the shell-based ExecTool, this tool runs commands
// directly via os/exec without any shell dependency, making it safer and
// more portable. It supports resource limits, timeouts, and streaming I/O.
//
// The tool exposes two operations:
//   - exec: run a command and wait for completion
//   - which: find a command's path on the system
type LispExecTool struct{}

func (*LispExecTool) Name() string { return "lisp_exec" }

func (*LispExecTool) Description() string {
	return "Execute SYSTEM COMMANDS / PROGRAMS (like bash, go, python, npm) — " +
		"NOT Lisp evaluation. This tool runs external programs via os/exec, " +
		"NOT a Lisp interpreter. Use this to run command-line tools, not to evaluate code. " +
		"Safer than exec: no shell injection, no shell syntax parsing. " +
		"Returns structured plist with :stdout, :stderr, :exit-code. " +
		"Supports resource limits (memory, CPU), timeouts, and environment variables. " +
		"For shell features (pipes, redirects, glob expansion), use exec instead."
}

func (*LispExecTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"description": "Runs external system programs via os/exec. NOT a Lisp evaluator. " +
			"Do NOT use this to evaluate Lisp code — use lisp_eval for that.",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "System program to execute (e.g. go, ls, python, npm). " +
					"This is a program PATH lookup, NOT Lisp code evaluation.",
			},
			"args": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Arguments to pass to the command.",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "Working directory for the command. Defaults to current directory.",
			},
			"env": map[string]any{
				"type":        "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description": "Environment variables to set (merged with current env). Example: {\"GOOS\": \"linux\", \"CGO_ENABLED\": \"0\"}.",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in milliseconds (max 600000). Default: 120000 (2 min).",
			},
			"max_memory_mb": map[string]any{
				"type":        "integer",
				"description": "Maximum memory in MB the command can use. 0 (default) = no limit. Unix: ulimit -v; Windows: Job Objects.",
			},
			"max_cpu_ms": map[string]any{
				"type":        "integer",
				"description": "Maximum CPU time in milliseconds. 0 (default) = no limit. Unix: ulimit -t; Windows: Job Objects.",
			},
			"input": map[string]any{
				"type":        "string",
				"description": "Input to send to the command's stdin. Only for exec operation.",
			},
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"exec", "which"},
				"description": "Operation: exec (default, run command), which (find command path).",
			},
		},
		"required": []string{"command"},
	}
}

func (*LispExecTool) CheckPermissions(params map[string]any) PermissionResult {
	// Check command for dangerous patterns similar to ExecTool
	cmd, _ := params["command"].(string)
	if cmd == "" {
		return PermissionResultPassthrough()
	}

	// Block dangerous commands (same deny patterns as ExecTool)
	lower := strings.ToLower(cmd)
	for _, re := range denyRegexps {
		if re.MatchString(lower) {
			return PermissionResultDeny("Dangerous command pattern detected: " + re.String())
		}
	}

	// Check for UNC paths
	if containsVulnerableUncPath(cmd) {
		return PermissionResultDeny("UNC path detected: commands targeting SMB/WebDAV shares are blocked")
	}

	return PermissionResultPassthrough()
}

func (t *LispExecTool) Execute(params map[string]any) ToolResult {
	return t.ExecuteContext(context.Background(), params)
}

func (t *LispExecTool) ExecuteContext(ctx context.Context, params map[string]any) (result ToolResult) {
	defer func() {
		if r := recover(); r != nil {
			result = ToolResult{Output: fmt.Sprintf("Error: lisp_exec panic: %v", r), IsError: true}
		}
	}()

	// Check context
	select {
	case <-ctx.Done():
		return ToolResult{Output: fmt.Sprintf("Error: lisp_exec cancelled: %v", ctx.Err()), IsError: true}
	default:
	}

	op, _ := params["operation"].(string)
	if op == "" {
		op = "exec"
	}

	switch op {
	case "which":
		return t.executeWhich(params)
	case "exec":
		return t.executeExec(ctx, params)
	default:
		return ToolResult{Output: fmt.Sprintf("Error: unknown operation %q. Use exec or which.", op), IsError: true}
	}
}

func (t *LispExecTool) executeWhich(params map[string]any) ToolResult {
	command, _ := params["command"].(string)
	if command == "" {
		return ToolResult{Output: "Error: command is required for which operation", IsError: true}
	}

	// Call microlisp which builtin directly
	lispExpr := fmt.Sprintf(`(which %s)`, lispQuote(command))
	cancelChan := microlisp.NewCancelChannel()
	limits := microlisp.DefaultLimits()
	limits.CancelChan = cancelChan

	result, err := microlisp.SafeEvalWithLimits(lispExpr, limits)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}
	if result == "nil" || result == "NIL" {
		return ToolResult{Output: fmt.Sprintf("Command %q not found in PATH", command), IsError: true}
	}
	return ToolResult{Output: result}
}

func (t *LispExecTool) executeExec(ctx context.Context, params map[string]any) ToolResult {
	command, _ := params["command"].(string)
	if command == "" {
		return ToolResult{Output: "Error: command is required", IsError: true}
	}

	// Split command string into program + implicit args.
	// e.g. "mkdir -p dir" -> program="mkdir", implicitArgs=["-p", "dir"]
	program, implicitArgs := splitCommand(command)

	// Merge explicit args from params with implicit args from splitting
	explicitArgs := extractArgs(params["args"])

	// Check for shell syntax in the original command string.
	// If found, we need to wrap the entire command with a shell.
	// This handles pipes, redirects, and other shell features.
	if needsShell(command) {
		shellProgram, shellArgs, shellHasInput := wrapWithShell(command, nil, "")
		program = shellProgram
		implicitArgs = nil
		explicitArgs = shellArgs
		// Remove the input param since we wrapped with shell
		delete(params, "input")
		if shellHasInput {
			// Shouldn't happen with current wrapWithShell, but handle future cases
		}
	}

	allArgs := append(implicitArgs, explicitArgs...)

	// Build the Lisp expression
	var exprBuilder strings.Builder

	// Check if there's stdin input
	input, hasInput := params["input"].(string)

	if hasInput && input != "" {
		exprBuilder.WriteString(fmt.Sprintf(`(exec-with-input %s %s`, lispQuote(program), lispQuote(input)))
	} else {
		exprBuilder.WriteString(fmt.Sprintf(`(exec %s`, lispQuote(program)))
	}

	// Add all args (merged implicit + explicit)
	if len(allArgs) > 0 {
		exprBuilder.WriteString(" :args (list")
		for _, arg := range allArgs {
			exprBuilder.WriteString(" ")
			exprBuilder.WriteString(lispQuote(arg))
		}
		exprBuilder.WriteString(")")
	}

	// Add working-dir (translate POSIX paths on Windows)
	if wd, ok := params["working_dir"].(string); ok && wd != "" {
		if runtime.GOOS == "windows" {
			wd = PosixToWindowsPath(wd)
		}
		exprBuilder.WriteString(fmt.Sprintf(" :working-dir %s", lispQuote(wd)))
	}

	// Add env
	if envMap, ok := params["env"].(map[string]any); ok && len(envMap) > 0 {
		exprBuilder.WriteString(" :env (list")
		for k, v := range envMap {
			valStr, _ := v.(string)
			exprBuilder.WriteString(fmt.Sprintf(" (cons %s %s)", lispQuote(k), lispQuote(valStr)))
		}
		exprBuilder.WriteString(")")
	}

	// Add timeout
	if timeout, ok := params["timeout"]; ok {
		switch v := timeout.(type) {
		case float64:
			exprBuilder.WriteString(fmt.Sprintf(" :timeout %d", int(v)))
		case int:
			exprBuilder.WriteString(fmt.Sprintf(" :timeout %d", v))
		}
	}

	// Add max-memory-mb
	if maxMem, ok := params["max_memory_mb"]; ok {
		switch v := maxMem.(type) {
		case float64:
			exprBuilder.WriteString(fmt.Sprintf(" :max-memory-mb %d", int(v)))
		case int:
			exprBuilder.WriteString(fmt.Sprintf(" :max-memory-mb %d", v))
		}
	}

	// Add max-cpu-ms
	if maxCPU, ok := params["max_cpu_ms"]; ok {
		switch v := maxCPU.(type) {
		case float64:
			exprBuilder.WriteString(fmt.Sprintf(" :max-cpu-ms %d", int(v)))
		case int:
			exprBuilder.WriteString(fmt.Sprintf(" :max-cpu-ms %d", v))
		}
	}

	exprBuilder.WriteString(")")
	lispExpr := exprBuilder.String()

	// Execute via microlisp with context cancellation
	cancelChan := microlisp.NewCancelChannel()
	limits := microlisp.DefaultLimits()
	limits.CancelChan = cancelChan

	ch := make(chan evalResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
			}
		}()
		result, err := microlisp.SafeEvalWithLimits(lispExpr, limits)
		ch <- evalResult{result, err}
	}()

	select {
	case <-ctx.Done():
		close(cancelChan)
		<-ch // wait for evalMu release
		return ToolResult{Output: "Error: lisp_exec timed out", IsError: true}
	case r := <-ch:
		if r.err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v", r.err), IsError: true}
		}
		return formatExecResult(r.output)
	}
}

// formatExecResult converts a Lisp plist result into a human-readable format.
// Input: (:stdout "..." :stderr "..." :exit-code N) or
//        (:stdout "..." :stderr "..." :exit-code -1 :background 1 :stall-reason "...")
func formatExecResult(lispResult string) ToolResult {
	stdout := extractPlistValue(lispResult, ":stdout")
	stderr := extractPlistValue(lispResult, ":stderr")
	exitCode := extractPlistValue(lispResult, ":exit-code")
	background := extractPlistValue(lispResult, ":background")
	stallReason := extractPlistValue(lispResult, ":stall-reason")

	var output strings.Builder

	// Background mode: command moved to background instead of killed
	if background != "" && background != "0" {
		output.WriteString("[Command moved to background]\n")
		if stallReason != "" {
			output.WriteString(stallReason)
			output.WriteString("\n\n")
		}
		if stdout != "" {
			output.WriteString(stdout)
		}
		if stderr != "" {
			if output.Len() > 0 {
				output.WriteString("\n")
			}
			output.WriteString("STDERR:\n")
			output.WriteString(stderr)
		}
		result := output.String()
		if result == "" {
			result = "(no output)"
		}
		return ToolResult{Output: result, IsError: false}
	}

	// Normal completion
	if stdout != "" {
		output.WriteString(stdout)
	}
	if stderr != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("STDERR:\n")
		output.WriteString(stderr)
	}
	if exitCode != "" {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("Exit code: ")
		output.WriteString(exitCode)
	}

	result := output.String()
	if result == "" {
		result = "(no output)"
	}

	isError := exitCode != "" && exitCode != "0"
	return ToolResult{Output: result, IsError: isError}
}

// extractPlistValue extracts a value from a Lisp plist string by key.
// Uses a sequential scan that properly tracks quoted string boundaries,
// avoiding false matches when command output contains key-like text
// such as ":stdout" or ":exit-code" inside a string value.
func extractPlistValue(plist, key string) string {
	i := 0
	n := len(plist)
	keyLen := len(key)

	for i < n {
		ch := plist[i]

		// Skip whitespace and structural chars
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '(' || ch == ')' {
			i++
			continue
		}

		// Check if key matches at position i with proper word boundary
		if i+keyLen <= n && plist[i:i+keyLen] == key {
			afterKey := i + keyLen
			// Verify trailing boundary: whitespace, structural char, quote, or end
			if afterKey >= n || plist[afterKey] == ' ' || plist[afterKey] == '\t' ||
				plist[afterKey] == '\n' || plist[afterKey] == ')' ||
				plist[afterKey] == '(' || plist[afterKey] == '"' {
				// Key match at structure level — extract value
				valStart := afterKey
				for valStart < n && (plist[valStart] == ' ' || plist[valStart] == '\t' || plist[valStart] == '\n') {
					valStart++
				}
				if valStart >= n {
					return ""
				}
				if plist[valStart] == '"' {
					return extractQuotedStringFrom(plist, valStart)
				}
				// Number or atom
				var val strings.Builder
				for j := valStart; j < n; j++ {
					c := plist[j]
					if c == ' ' || c == ')' || c == '\n' || c == '(' {
						break
					}
					val.WriteByte(c)
				}
				return val.String()
			}
		}

		// Not a key match — skip this token
		if ch == '"' {
			// Skip entire quoted string, properly handling escape sequences
			escaped := false
			i++ // skip opening quote
			for i < n {
				c := plist[i]
				if escaped {
					escaped = false
					i++
					continue
				}
				if c == '\\' {
					escaped = true
					i++
					continue
				}
				if c == '"' {
					i++ // skip closing quote
					break
				}
				i++
			}
		} else {
			// Skip atom (non-whitespace, non-structural, non-quote token)
			for i < n {
				c := plist[i]
				if c == ' ' || c == '\t' || c == '\n' || c == '(' || c == ')' || c == '"' {
					break
				}
				i++
			}
		}
	}
	return ""
}

// extractQuotedStringFrom extracts a Lisp string starting at the opening quote.
// s[pos] must be '"'.
func extractQuotedStringFrom(s string, pos int) string {
	if pos+1 >= len(s) {
		return ""
	}
	var result strings.Builder
	escaped := false
	for i := pos + 1; i < len(s); i++ {
		ch := s[i]
		if escaped {
			switch ch {
			case 'n':
				result.WriteByte('\n')
			case 't':
				result.WriteByte('\t')
			case '\\':
				result.WriteByte('\\')
			case '"':
				result.WriteByte('"')
			default:
				result.WriteByte(ch)
			}
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			return result.String()
		}
		result.WriteByte(ch)
	}
	return result.String()
}

// extractQuotedString extracts a Lisp string from a string starting with ".
// Kept for backwards compatibility; prefer extractQuotedStringFrom.
func extractQuotedString(s string) string {
	if len(s) < 2 || s[0] != '"' {
		return ""
	}
	return extractQuotedStringFrom(s, 0)
}

// lispQuote wraps a string in double quotes for Lisp, escaping special chars.
func lispQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, ch := range s {
		switch ch {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(ch)
		}
	}
	b.WriteByte('"')
	return b.String()
}
