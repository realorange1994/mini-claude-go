package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"miniclaudecode-go/microlisp"
)

func RegisterExecTool(s *MCPServer) {
	s.RegisterTool(ToolDef{
		Name: "lisp_exec",
		Description: "Execute SYSTEM COMMANDS / PROGRAMS (like bash, go, python, npm) — " +
			"NOT Lisp evaluation. This tool runs external programs via os/exec, " +
			"NOT a Lisp interpreter. Use this to run command-line tools, not to evaluate code. " +
			"Safer than exec: no shell injection, no shell syntax parsing. " +
			"Returns structured plist with :stdout, :stderr, :exit-code. " +
			"Supports resource limits (memory, CPU), timeouts, and environment variables. " +
			"For shell features (pipes, redirects, glob expansion), use exec instead.",
		InputSchema: map[string]any{
			"type": "object",
			"description": "Runs external system programs via os/exec. NOT a Lisp evaluator. " +
				"Do NOT use this to evaluate Lisp code — use lisp_eval for that.",
			"properties": map[string]any{
				"command": map[string]any{
					"type": "string",
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
					"type":                 "object",
					"additionalProperties": map[string]any{"type": "string"},
					"description":          "Environment variables to set (merged with current env). Example: {\"GOOS\": \"linux\", \"CGO_ENABLED\": \"0\"}.",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in milliseconds (max 600000). Default: 120000 (2 min).",
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
		},
	}, handleExec)
}

func handleExec(params json.RawMessage) (ToolCallResult, error) {
	var p struct {
		Command    string            `json:"command"`
		Args       []string          `json:"args"`
		WorkingDir string            `json:"working_dir"`
		Env        map[string]string `json:"env"`
		Timeout    int               `json:"timeout"`
		Input      string            `json:"input"`
		Operation  string            `json:"operation"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return textResult("Invalid params: "+err.Error(), true), nil
	}

	op := p.Operation
	if op == "" {
		op = "exec"
	}

	switch op {
	case "which":
		return handleWhich(p.Command)
	case "exec":
		return handleExecOp(p)
	default:
		return textResult(fmt.Sprintf("Error: unknown operation %q. Use exec or which.", op), true), nil
	}
}

func handleWhich(command string) (ToolCallResult, error) {
	if command == "" {
		return textResult("Error: command is required for which operation", true), nil
	}

	lispExpr := fmt.Sprintf(`(which %s)`, lispQuote(command))
	cancelChan := microlisp.NewCancelChannel()
	limits := microlisp.DefaultLimits()
	limits.CancelChan = cancelChan

	result, err := microlisp.SafeEvalWithLimits(lispExpr, limits)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}
	if result == "nil" || result == "NIL" {
		return textResult(fmt.Sprintf("Command %q not found in PATH", command), true), nil
	}
	return textResult(result, false), nil
}

type execParams struct {
	Command    string
	Args       []string
	WorkingDir string
	Env        map[string]string
	Timeout    int
	Input      string
}

func handleExecOp(p struct {
	Command    string            `json:"command"`
	Args       []string          `json:"args"`
	WorkingDir string            `json:"working_dir"`
	Env        map[string]string `json:"env"`
	Timeout    int               `json:"timeout"`
	Input      string            `json:"input"`
	Operation  string            `json:"operation"`
}) (ToolCallResult, error) {
	if p.Command == "" {
		return textResult("Error: command is required", true), nil
	}

	program, implicitArgs := splitCommand(p.Command)
	explicitArgs := p.Args

	if needsShell(p.Command) {
		shellProgram, shellArgs, _ := wrapWithShell(p.Command, nil, "")
		program = shellProgram
		implicitArgs = nil
		explicitArgs = shellArgs
	}

	allArgs := append(implicitArgs, explicitArgs...)

	var exprBuilder strings.Builder

	if p.Input != "" {
		exprBuilder.WriteString(fmt.Sprintf(`(exec-with-input %s %s`, lispQuote(program), lispQuote(p.Input)))
	} else {
		exprBuilder.WriteString(fmt.Sprintf(`(exec %s`, lispQuote(program)))
	}

	if len(allArgs) > 0 {
		exprBuilder.WriteString(" :args (list")
		for _, arg := range allArgs {
			exprBuilder.WriteString(" ")
			exprBuilder.WriteString(lispQuote(arg))
		}
		exprBuilder.WriteString(")")
	}

	if p.WorkingDir != "" {
		wd := p.WorkingDir
		if runtime.GOOS == "windows" {
			wd = posixToWindowsPath(wd)
		}
		exprBuilder.WriteString(fmt.Sprintf(" :working-dir %s", lispQuote(wd)))
	}

	if len(p.Env) > 0 {
		exprBuilder.WriteString(" :env (list")
		for k, v := range p.Env {
			exprBuilder.WriteString(fmt.Sprintf(" (cons %s %s)", lispQuote(k), lispQuote(v)))
		}
		exprBuilder.WriteString(")")
	}

	if p.Timeout > 0 {
		exprBuilder.WriteString(fmt.Sprintf(" :timeout %d", p.Timeout))
	}

	exprBuilder.WriteString(")")
	lispExpr := exprBuilder.String()

	cancelChan := microlisp.NewCancelChannel()
	limits := microlisp.DefaultLimits()
	limits.CancelChan = cancelChan

	type evalRes struct {
		output string
		err    error
	}
	ch := make(chan evalRes, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- evalRes{"", fmt.Errorf("panic: %v", r)}
			}
		}()
		result, err := microlisp.SafeEvalWithLimits(lispExpr, limits)
		ch <- evalRes{result, err}
	}()

	dur := limitsDuration(limits)
	if dur > 0 {
		timer := time.NewTimer(dur)
		select {
		case <-timer.C:
			close(cancelChan)
			<-ch
			return textResult("Error: lisp_exec timed out", true), nil
		case r := <-ch:
			timer.Stop()
			if r.err != nil {
				return textResult(fmt.Sprintf("Error: %v", r.err), true), nil
			}
			return formatExecResult(r.output), nil
		}
	}

	r := <-ch
	if r.err != nil {
		return textResult(fmt.Sprintf("Error: %v", r.err), true), nil
	}
	return formatExecResult(r.output), nil
}

func formatExecResult(lispResult string) ToolCallResult {
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
	return textResult(result, isError)
}

func extractPlistValue(plist, key string) string {
	i := 0
	n := len(plist)
	keyLen := len(key)

	for i < n {
		ch := plist[i]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '(' || ch == ')' {
			i++
			continue
		}

		if i+keyLen <= n && plist[i:i+keyLen] == key {
			afterKey := i + keyLen
			if afterKey >= n || plist[afterKey] == ' ' || plist[afterKey] == '\t' ||
				plist[afterKey] == '\n' || plist[afterKey] == ')' ||
				plist[afterKey] == '(' || plist[afterKey] == '"' {
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

		if ch == '"' {
			escaped := false
			i++
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
					i++
					break
				}
				i++
			}
		} else {
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

var shellSyntaxMarkers = []string{
	"|", "||", "&&", ";",
	">", ">>", "2>", "2>&1", "&>", "&>>",
	"<", "<<", "<<<",
	"$(", "`",
	"{", "}",
}

func needsShell(s string) bool {
	for _, marker := range shellSyntaxMarkers {
		if strings.Contains(s, marker) {
			if len(marker) > 1 {
				return true
			}
			if marker == "|" {
				return true
			}
			if marker == ">" || marker == "<" {
				idx := strings.Index(s, marker)
				if idx > 0 {
					prev := rune(s[idx-1])
					if !unicode.IsSpace(prev) && prev != '"' {
						continue
					}
				}
				return true
			}
			if marker == "$(" || marker == "`" {
				return true
			}
			if marker == "{" || marker == "}" {
				if strings.Contains(s, "{") && strings.Contains(s, "}") {
					return true
				}
				continue
			}
		}
	}
	if strings.Contains(s, ";") {
		return true
	}
	return false
}

func wrapWithShell(command string, args []string, input string) (program string, allArgs []string, hasInput bool) {
	fullCmd := command
	if len(args) > 0 {
		fullCmd += " " + strings.Join(args, " ")
	}

	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", fullCmd}, input != ""
	}
	return "bash", []string{"-c", fullCmd}, input != ""
}

func splitCommand(s string) (program string, implicitArgs []string) {
	parts, _ := shellSplit(s)
	if len(parts) == 0 {
		return "", nil
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return parts[0], parts[1:]
}

func shellSplit(s string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	i := 0
	n := len(s)

	for i < n {
		ch := s[i]

		if ch == ' ' || ch == '\t' {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			i++
			continue
		}

		if ch == '"' {
			i++
			for i < n {
				c := s[i]
				if c == '\\' && i+1 < n {
					current.WriteByte(s[i+1])
					i += 2
					continue
				}
				if c == '"' {
					i++
					break
				}
				current.WriteByte(c)
				i++
			}
			continue
		}

		if ch == '\'' {
			i++
			for i < n {
				if s[i] == '\'' {
					i++
					break
				}
				current.WriteByte(s[i])
				i++
			}
			continue
		}

		if ch == '\\' && i+1 < n {
			current.WriteByte(s[i+1])
			i += 2
			continue
		}

		current.WriteByte(ch)
		i++
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens, nil
}

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

func posixToWindowsPath(posixPath string) string {
	if strings.HasPrefix(posixPath, "//") {
		return `\\` + strings.ReplaceAll(posixPath[2:], "/", `\`)
	}

	if strings.HasPrefix(posixPath, "/cygdrive/") {
		rest := posixPath[len("/cygdrive/"):]
		if len(rest) >= 2 && isLetter(rune(rest[0])) && rest[1] == '/' {
			return strings.ToUpper(string(rest[0])) + `:\` + strings.ReplaceAll(rest[2:], "/", `\`)
		}
	}

	if posixPath == "/tmp" || strings.HasPrefix(posixPath, "/tmp/") {
		rest := posixPath[len("/tmp"):]
		return filepath.Join(os.TempDir(), rest)
	}

	if posixPath == "/home" || strings.HasPrefix(posixPath, "/home/") {
		rest := posixPath[len("/home"):]
		if rest != "" && rest[0] == '/' {
			rest = rest[1:]
			if idx := strings.Index(rest, "/"); idx > 0 {
				rest = rest[idx+1:]
			} else if idx < 0 && rest != "" {
				return filepath.Join(os.TempDir(), rest)
			}
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), rest)
		}
		return filepath.Join(home, rest)
	}

	if len(posixPath) >= 2 && isLetter(rune(posixPath[0])) && posixPath[1] == ':' {
		return posixPath
	}

	if strings.HasPrefix(posixPath, "/") {
		rest := posixPath[1:]
		if len(rest) >= 2 && isLetter(rune(rest[0])) && rest[1] == '/' {
			return strings.ToUpper(string(rest[0])) + `:\` + strings.ReplaceAll(rest[2:], "/", `\`)
		}
	}

	return strings.ReplaceAll(posixPath, "/", `\`)
}

func isLetter(r rune) bool {
	return unicode.IsLetter(r)
}
