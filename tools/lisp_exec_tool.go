package tools

import (
	"context"
	"fmt"
	"strings"

	"miniclaudecode-go/microlisp"
)

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
	return "Execute commands without shell dependency (pure Go via os/exec). " +
		"Safer than exec: no shell injection, no shell syntax parsing. " +
		"Use for running programs directly (go test, node, python, etc.). " +
		"Returns structured plist with :stdout, :stderr, :exit-code. " +
		"Supports resource limits (memory, CPU), timeouts, and environment variables. " +
		"For shell features (pipes, redirects, glob expansion), use exec instead."
}

func (*LispExecTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Command to execute (program name or path). Required for exec and which operations.",
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

	// Build the Lisp expression
	var exprBuilder strings.Builder

	// Check if there's stdin input
	input, hasInput := params["input"].(string)

	if hasInput && input != "" {
		exprBuilder.WriteString(fmt.Sprintf(`(exec-with-input %s %s`, lispQuote(command), lispQuote(input)))
	} else {
		exprBuilder.WriteString(fmt.Sprintf(`(exec %s`, lispQuote(command)))
	}

	// Add args
	if args, ok := params["args"].([]any); ok && len(args) > 0 {
		exprBuilder.WriteString(" :args (list")
		for _, arg := range args {
			if s, ok := arg.(string); ok {
				exprBuilder.WriteString(" ")
				exprBuilder.WriteString(lispQuote(s))
			}
		}
		exprBuilder.WriteString(")")
	}

	// Add working-dir
	if wd, ok := params["working_dir"].(string); ok && wd != "" {
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
// Input: (:stdout "..." :stderr "..." :exit-code N)
// Output: stdout + stderr + "Exit code: N"
func formatExecResult(lispResult string) ToolResult {
	// Parse the plist to extract stdout, stderr, exit-code
	stdout := extractPlistValue(lispResult, ":stdout")
	stderr := extractPlistValue(lispResult, ":stderr")
	exitCode := extractPlistValue(lispResult, ":exit-code")

	var output strings.Builder
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
// Very simple parser — handles :key "value" and :key N patterns.
func extractPlistValue(plist, key string) string {
	idx := strings.Index(plist, key)
	if idx < 0 {
		return ""
	}
	// Move past the key
	rest := plist[idx+len(key):]
	// Skip whitespace
	rest = strings.TrimLeft(rest, " ")

	if len(rest) == 0 {
		return ""
	}

	// If starts with quote, extract quoted string
	if rest[0] == '"' {
		return extractQuotedString(rest)
	}

	// Otherwise extract until whitespace or closing paren
	var val strings.Builder
	for i := 0; i < len(rest); i++ {
		ch := rest[i]
		if ch == ' ' || ch == ')' || ch == '\n' {
			break
		}
		val.WriteByte(ch)
	}
	return val.String()
}

// extractQuotedString extracts a Lisp string from "content", handling escapes.
func extractQuotedString(s string) string {
	if len(s) < 2 || s[0] != '"' {
		return ""
	}
	var result strings.Builder
	escaped := false
	for i := 1; i < len(s); i++ {
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
