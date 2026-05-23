package tools

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

// ─── needsShell ─────────────────────────────────────────────────────────────

func TestNeedsShell(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Simple commands — no shell needed
		{"echo hello", "echo hello", false},
		{"ls -la", "ls -la", false},
		{"go version", "go version", false},
		{"mkdir -p dir", "mkdir -p dir", false},
		{"cat file.txt", "cat file.txt", false},
		{"file name<with>brackets", "file name<with>brackets", false},

		// Pipe — shell needed
		{"pipe", "cat /etc/passwd | grep root", true},
		{"double pipe", "echo hello || echo world", true},

		// Command chaining — shell needed
		{"and chain", "mkdir /tmp && echo done", true},
		{"semicolon", "echo a; echo b", true},

		// Redirects — shell needed
		{"redirect out", "echo hello > /tmp/out.txt", true},
		{"redirect append", "echo hello >> /tmp/out.txt", true},
		{"redirect stderr", "cmd 2> /dev/null", true},
		{"redirect both", "cmd 2>&1", true},
		{"redirect and", "cmd &> /dev/null", true},
		{"redirect append both", "cmd &>> /dev/null", true},

		// Input redirects — shell needed
		{"redirect in", "cat < /tmp/in.txt", true},
		{"heredoc", "cat << EOF", true},
		{"here-string", "cat <<< hello", true},

		// Command substitution — shell needed
		{"dollar paren", "echo $(whoami)", true},
		{"backtick", "echo `whoami`", true},

		// Brace expansion — shell needed
		{"brace expand", "echo {a,b,c}", true},
		{"single brace open", "echo {a", false},
		{"single brace close", "echo a}", false},

		// Filename edge cases — NOT shell syntax
		{"filename with less", "cat a<b.txt", false},
		{"filename with greater", "ls x>y.log", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := needsShell(tc.input)
			if got != tc.want {
				t.Errorf("needsShell(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// ─── wrapWithShell ──────────────────────────────────────────────────────────

func TestWrapWithShell(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		args      []string
		input     string
		wantProg  string
		wantArgs  []string
		wantInput bool
	}{
		{"simple command no args",
			"echo hello", nil, "",
			shellProg(), []string{shellFlag(), "echo hello"}, false},
		{"command with args",
			"mkdir", []string{"-p", "/tmp/test"}, "",
			shellProg(), []string{shellFlag(), "mkdir -p /tmp/test"}, false},
		{"command with input",
			"cat", nil, "hello",
			shellProg(), []string{shellFlag(), "cat"}, true},
		{"empty command",
			"", nil, "",
			shellProg(), []string{shellFlag(), ""}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prog, args, hasInput := wrapWithShell(tc.command, tc.args, tc.input)
			if prog != tc.wantProg {
				t.Errorf("program = %q, want %q", prog, tc.wantProg)
			}
			if len(args) != len(tc.wantArgs) {
				t.Errorf("args = %v, want %v", args, tc.wantArgs)
				return
			}
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tc.wantArgs[i])
				}
			}
			if hasInput != tc.wantInput {
				t.Errorf("hasInput = %v, want %v", hasInput, tc.wantInput)
			}
		})
	}
}

// ─── splitCommand / shellSplit ──────────────────────────────────────────────

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantProgram string
		wantArgs    []string
	}{
		{"single command", "echo", "echo", nil},
		{"command with one arg", "echo hello", "echo", []string{"hello"}},
		{"command with multiple args", "ls -la /tmp", "ls", []string{"-la", "/tmp"}},
		{"empty string", "", "", nil},
		{"only spaces", "   ", "", nil},
		{"double quoted arg", `echo "hello world"`, "echo", []string{"hello world"}},
		{"single quoted arg", `echo 'hello world'`, "echo", []string{"hello world"}},
		{"nested quotes", `bash -c "echo 'Hello'"`, "bash", []string{"-c", "echo 'Hello'"}},
		{"backslash escape", `echo hello\ world`, "echo", []string{"hello world"}},
		{"mixed whitespace", "  ls   -la  ", "ls", []string{"-la"}},
		{"tab separated", "ls\t-la", "ls", []string{"-la"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prog, args := splitCommand(tc.input)
			if prog != tc.wantProgram {
				t.Errorf("program = %q, want %q", prog, tc.wantProgram)
			}
			if len(args) != len(tc.wantArgs) {
				t.Errorf("args = %v (len=%d), want %v (len=%d)", args, len(args), tc.wantArgs, len(tc.wantArgs))
				return
			}
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tc.wantArgs[i])
				}
			}
		})
	}
}

func TestShellSplit(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"simple", "echo hello world", []string{"echo", "hello", "world"}},
		{"double quoted", `echo "hello world"`, []string{"echo", "hello world"}},
		{"single quoted", "echo 'hello world'", []string{"echo", "hello world"}},
		{"escape in double quote", `echo "he said \"hi\""`, []string{"echo", `he said "hi"`}},
		{"backslash escape", `echo hello\ world`, []string{"echo", "hello world"}},
		{"empty string", "", nil},
		{"only whitespace", "   \t  ", nil},
		{"nested quotes", `bash -c "echo 'inner'"`, []string{"bash", "-c", "echo 'inner'"}},
		{"double quote with escape", `echo "line1\nline2"`, []string{"echo", "line1nline2"}},
		{"single quote no escape", `echo 'not\nescaped'`, []string{"echo", `not\nescaped`}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := shellSplit(tc.input)
			if err != nil {
				t.Fatalf("shellSplit error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Errorf("shellSplit(%q) = %v (len=%d), want %v (len=%d)", tc.input, got, len(got), tc.want, len(tc.want))
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("shellSplit(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ─── extractArgs ─────────────────────────────────────────────────────────────

func TestExtractArgs(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{"[]any strings", []any{"-la", "/tmp"}, []string{"-la", "/tmp"}},
		{"[]any with float64", []any{1.0, 2.0, "3"}, []string{"1", "2", "3"}},
		{"[]any with nil", []any{"a", nil, "b"}, []string{"a", "b"}},
		{"[]any with int", []any{"a", 42, "b"}, []string{"a", "42", "b"}},
		{"[]string", []string{"-la", "/tmp"}, []string{"-la", "/tmp"}},
		{"empty []any", []any{}, []string{}},
		{"empty []string", []string{}, []string{}},
		{"non-array string", "not an array", nil},
		{"non-array int", 42, nil},
		{"nil", nil, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractArgs(tc.in)
			if len(got) != len(tc.want) {
				t.Errorf("extractArgs(%v) = %v (len=%d), want %v (len=%d)", tc.in, got, len(got), tc.want, len(tc.want))
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("extractArgs[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ─── formatExecResult ───────────────────────────────────────────────────────

func TestFormatExecResult(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOut string
		wantErr bool
	}{
		{"success stdout only",
			`(:stdout "hello world" :stderr "" :exit-code 0)`,
			"hello world", false},
		{"success with stderr",
			`(:stdout "out" :stderr "err msg" :exit-code 0)`,
			"out\nSTDERR:\nerr msg", false},
		{"failure",
			`(:stdout "" :stderr "error" :exit-code 1)`,
			"STDERR:\nerror\nExit code: 1", true},
		{"failure with stdout",
			`(:stdout "partial" :stderr "failed" :exit-code 2)`,
			"partial\nSTDERR:\nfailed\nExit code: 2", true},
		{"no output but exit-code 0",
			`(:stdout "" :stderr "" :exit-code 0)`,
			"Exit code: 0", false},
		{"no output at all truly empty",
			`()`,
			"(no output)", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatExecResult(tc.input)
			if result.IsError != tc.wantErr {
				t.Errorf("IsError = %v, want %v, output:\n%s", result.IsError, tc.wantErr, result.Output)
			}
			if !strings.Contains(result.Output, tc.wantOut) {
				t.Errorf("output = %q, expected to contain %q", result.Output, tc.wantOut)
			}
		})
	}
}

func TestFormatExecResultBackground(t *testing.T) {
	input := `(:stdout "" :stderr "" :exit-code -1 :background 1 :stall-reason "waiting for input")`
	result := formatExecResult(input)
	if result.IsError {
		t.Errorf("background result should not be error, IsError = true")
	}
	if !strings.Contains(result.Output, "[Command still running in background]") {
		t.Errorf("output missing background header, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "waiting for input") {
		t.Errorf("output missing stall reason, got: %s", result.Output)
	}
}

func TestFormatExecResultBackgroundWithOutput(t *testing.T) {
	input := `(:stdout "some output" :stderr "some err" :exit-code -1 :background 1 :stall-reason "timed out")`
	result := formatExecResult(input)
	if !strings.Contains(result.Output, "[Command still running in background]") {
		t.Errorf("output missing background header, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "some output") {
		t.Errorf("output missing stdout, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "some err") {
		t.Errorf("output missing stderr, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "timed out") {
		t.Errorf("output missing stall reason, got: %s", result.Output)
	}
}

// ─── extractQuotedStringFrom ────────────────────────────────────────────────

func TestExtractQuotedStringFrom(t *testing.T) {
	tests := []struct {
		name  string
		input string
		pos   int
		want  string
	}{
		{"simple", `"hello"`, 0, "hello"},
		{"with spaces", `"hello world"`, 0, "hello world"},
		{"escaped quote", `"he said \"hi\""`, 0, `he said "hi"`},
		{"escaped newline", `"line1\nline2"`, 0, "line1\nline2"},
		{"escaped tab", `"col1\tcol2"`, 0, "col1\tcol2"},
		{"escaped backslash", `"path\\file"`, 0, `path\file`},
		{"pos not at start", `xxx "hello"`, 4, "hello"},
		{"too short", `"`, 0, ""},
		{"empty string", `""`, 0, ""},
		{"unknown escape", `"a\b"`, 0, "ab"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractQuotedStringFrom(tc.input, tc.pos)
			if got != tc.want {
				t.Errorf("extractQuotedStringFrom(%q, %d) = %q, want %q", tc.input, tc.pos, got, tc.want)
			}
		})
	}
}

// ─── lispQuote ───────────────────────────────────────────────────────────────

func TestLispQuote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "hello", `"hello"`},
		{"with spaces", "hello world", `"hello world"`},
		{"with double quote", `say "hi"`, `"say \"hi\""`},
		{"with backslash", `path\file`, `"path\\file"`},
		{"with newline", "line1\nline2", `"line1\nline2"`},
		{"with tab", "col1\tcol2", `"col1\tcol2"`},
		{"empty", "", `""`},
		{"special chars", "a<b>c", `"a<b>c"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := lispQuote(tc.input)
			if got != tc.want {
				t.Errorf("lispQuote(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ─── LispExecTool interface ──────────────────────────────────────────────────

func TestLispExecToolName(t *testing.T) {
	tool := &LispExecTool{}
	if tool.Name() != "lisp_exec" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "lisp_exec")
	}
}

func TestLispExecToolDescription(t *testing.T) {
	tool := &LispExecTool{}
	desc := tool.Description()
	if !strings.Contains(desc, "os/exec") {
		t.Error("description should mention os/exec")
	}
	if !strings.Contains(desc, "NOT") || !strings.Contains(desc, "Lisp") {
		t.Error("description should clarify it's not a Lisp tool")
	}
	if !strings.Contains(desc, "resource limits") {
		t.Error("description should mention resource limits")
	}
}

func TestLispExecToolInputSchema(t *testing.T) {
	tool := &LispExecTool{}
	schema := tool.InputSchema()

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in schema")
	}

	requiredFields := []string{"command"}
	for _, field := range requiredFields {
		if _, ok := props[field]; !ok {
			t.Errorf("schema missing %q property", field)
		}
	}

	expectedProps := []string{
		"command", "args", "working_dir", "env", "timeout",
		"max_memory_mb", "max_cpu_ms", "input", "operation",
	}
	for _, prop := range expectedProps {
		if _, ok := props[prop]; !ok {
			t.Errorf("schema should have %q property", prop)
		}
	}

	// Verify operation enum includes exec and which
	opProp, ok := props["operation"].(map[string]any)
	if !ok {
		t.Fatal("operation property should be a map")
	}
	enums, ok := opProp["enum"].([]string)
	if !ok {
		t.Fatal("operation enum should be a string slice")
	}
	expectedOps := map[string]bool{"exec": true, "which": true}
	for _, e := range enums {
		delete(expectedOps, e)
	}
	if len(expectedOps) > 0 {
		t.Errorf("operation enum missing: %v", expectedOps)
	}
}

// ─── LispExecTool CheckPermissions ──────────────────────────────────────────

func TestLispExecToolCheckPermissionsSafe(t *testing.T) {
	tool := &LispExecTool{}
	safe := []string{
		"echo hello",
		"ls -la",
		"go version",
		"pwd",
	}
	for _, cmd := range safe {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior != PermissionPassthrough {
			t.Errorf("safe command %q should passthrough, got: %v", cmd, result)
		}
	}
}

func TestLispExecToolCheckPermissionsDangerous(t *testing.T) {
	tool := &LispExecTool{}
	dangerous := []string{
		"rm -rf /",
		"sudo rm -rf /",
		"mkfs.ext4 /dev/sda",
	}
	for _, cmd := range dangerous {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Errorf("dangerous command %q should be denied", cmd)
		}
	}
}

func TestLispExecToolCheckPermissionsEmptyCommand(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.CheckPermissions(map[string]any{"command": ""})
	// Empty command should passthrough (it's validated later in Execute)
	if result.Behavior != PermissionPassthrough {
		t.Errorf("empty command should passthrough, got: %v", result)
	}
}

// ─── LispExecTool executeWhich ──────────────────────────────────────────────

func TestLispExecToolWhichFound(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{
		"command":   "echo",
		"operation": "which",
	})
	if result.IsError {
		t.Errorf("which echo failed: %s", result.Output)
	}
	if result.Output == "" {
		t.Error("which echo should return a path")
	}
}

func TestLispExecToolWhichNotFound(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{
		"command":   "nonexistent_command_xyz_123",
		"operation": "which",
	})
	// which returns "()" or empty for not-found, not necessarily an error
	if result.IsError && !strings.Contains(result.Output, "()") && !strings.Contains(result.Output, "not found") {
		t.Errorf("which nonexistent_command unexpected: %s", result.Output)
	}
}

func TestLispExecToolWhichEmptyCommand(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{
		"command":   "",
		"operation": "which",
	})
	if !result.IsError {
		t.Errorf("which empty command should error, got: %s", result.Output)
	}
}

func TestLispExecToolWhichUnknownOperation(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{
		"command":   "echo",
		"operation": "unknown_op",
	})
	if !result.IsError {
		t.Errorf("unknown operation should error, got: %s", result.Output)
	}
}

// ─── LispExecTool executeExec edge cases ─────────────────────────────────────

func TestLispExecToolExecEmptyCommand(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{"command": ""})
	if !result.IsError {
		t.Errorf("empty command should error, got: %s", result.Output)
	}
}

func TestLispExecToolExecCommandNotFound(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{"command": "nonexistent_cmd_xyz"})
	// Should either error or return exit-code != 0
	// On some systems, exec.Command returns "not found" error immediately
	if !result.IsError {
		t.Logf("command not found returned non-error: %s", result.Output)
	}
}

func TestLispExecToolExecContextCancellation(t *testing.T) {
	tool := &LispExecTool{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result := tool.ExecuteContext(ctx, map[string]any{"command": "sleep 10"})
	if !result.IsError {
		t.Errorf("cancelled context should error, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "cancelled") && !strings.Contains(result.Output, "timed out") {
		t.Errorf("expected cancellation error, got: %s", result.Output)
	}
}

func TestLispExecToolExecSleep(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{
		"command": "sleep",
		"args":    []any{"0.1"},
	})
	if result.IsError {
		t.Errorf("sleep 0.1 should succeed, got: %s", result.Output)
	}
}

func TestLispExecToolExecInput(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{
		"command": "cat",
		"input":   "hello stdin",
	})
	if result.IsError {
		t.Errorf("cat with input failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "hello stdin") {
		t.Errorf("output missing 'hello stdin', got: %s", result.Output)
	}
}

func TestLispExecToolExecWorkingDir(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{
		"command":     "pwd",
		"working_dir": "/tmp",
	})
	if result.IsError {
		t.Errorf("pwd with working_dir failed: %s", result.Output)
	}
	// On Unix, pwd should return /tmp
	if !strings.Contains(result.Output, "/tmp") {
		t.Logf("pwd output doesn't contain /tmp: %s", result.Output)
	}
}

func TestLispExecToolExecEnv(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{
		"command": "env",
		"env":     map[string]any{"TEST_LISP_EXEC_VAR": "value123"},
	})
	if result.IsError {
		t.Errorf("env with custom env failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "TEST_LISP_EXEC_VAR=value123") {
		t.Errorf("output missing env var, got: %s", result.Output)
	}
}

func TestLispExecToolExecMultipleArgs(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{
		"command": "echo",
		"args":    []any{"one", "two", "three"},
	})
	if result.IsError {
		t.Errorf("echo with multiple args failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "one") || !strings.Contains(result.Output, "two") || !strings.Contains(result.Output, "three") {
		t.Errorf("output missing args, got: %s", result.Output)
	}
}

func TestLispExecToolExecStringArgs(t *testing.T) {
	tool := &LispExecTool{}
	result := tool.Execute(map[string]any{
		"command": "echo",
		"args":    []string{"hello", "world"},
	})
	if result.IsError {
		t.Errorf("echo with []string args failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "hello") || !strings.Contains(result.Output, "world") {
		t.Errorf("output missing args, got: %s", result.Output)
	}
}

// ─── splitCommand + needsShell integration ──────────────────────────────────

func TestSplitCommandNeedsShellIntegration(t *testing.T) {
	// Commands that need shell should NOT be split and passed to os/exec directly
	// Instead, they should be wrapped with bash -c
	shellCommands := []string{
		"cat /etc/passwd | grep root",
		"echo hello > /tmp/out.txt",
		"mkdir /tmp && echo done",
	}
	tool := &LispExecTool{}
	for _, cmd := range shellCommands {
		if !needsShell(cmd) {
			t.Errorf("expected needsShell(%q) = true", cmd)
			continue
		}
		// The tool should handle this — either wrap with shell or handle appropriately
		result := tool.Execute(map[string]any{"command": cmd})
		// We just verify it doesn't panic or crash
		_ = result
	}
}

// ─── extractPlistValue edge cases ───────────────────────────────────────────

func TestExtractPlistValueEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		plist  string
		key    string
		expect string
	}{
		// Negative exit-code (background process)
		{"negative exit-code", `(:stdout "" :stderr "" :exit-code -1 :background 1)`, ":exit-code", "-1"},

		// Colons in values
		{"value with colons", `(:stdout "a:b:c" :stderr "" :exit-code 0)`, ":stdout", "a:b:c"},

		// Newlines in values (escaped in Lisp format)
		{"value with escaped newline", "(:stdout \"a\\nb\" :stderr \"\" :exit-code 0)", ":stdout", "a\nb"},

		// Very long values
		{"long value", `(:stdout "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" :stderr "" :exit-code 0)`, ":stdout", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},

		// Keys in different orders
		{"reordered keys", `(:exit-code 0 :stdout "hello" :stderr "")`, ":stdout", "hello"},

		// Extra whitespace
		{"extra whitespace", `(:stdout  "hello"  :stderr  ""  :exit-code  0)`, ":stdout", "hello"},

		// Plist with extra fields
		{"extra field", `(:stdout "ok" :stderr "" :exit-code 0 :cwd "/tmp")`, ":cwd", "/tmp"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPlistValue(tc.plist, tc.key)
			if got != tc.expect {
				t.Errorf("extractPlistValue(%q, %q) = %q, want %q", tc.plist, tc.key, got, tc.expect)
			}
		})
	}
}

func shellProg() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "bash"
}

func shellFlag() string {
	if runtime.GOOS == "windows" {
		return "/c"
	}
	return "-c"
}
