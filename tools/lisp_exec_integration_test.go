package tools

import (
	"strings"
	"testing"
)

func TestLispExecToolArgs(t *testing.T) {
	tool := &LispExecTool{}

	tests := []struct {
		name    string
		params  map[string]any
		want    string
	}{
		// Bug 1 regression: args parameter not passed to command
		{"seq 5", map[string]any{"command": "seq", "args": []any{5.0}}, "1"},
		{"seq 1 5", map[string]any{"command": "seq", "args": []any{1.0, 5.0}}, "1"},
		{"factor 42", map[string]any{"command": "factor", "args": []any{42.0}}, "42"},
		{"factor 3", map[string]any{"command": "factor", "args": []any{3.0}}, "3"},
		{"date +%Y", map[string]any{"command": "date", "args": []any{"+%Y"}}, "2026"},
		// Bug 2 regression: stdin + args combination
		{"grep hello + input", map[string]any{
			"command": "grep",
			"args":    []any{"hello"},
			"input":   "hello world\nfoo bar\nhello again",
		}, "hello"},
		{"grep -e hello + input", map[string]any{
			"command": "grep",
			"args":    []any{"-e", "hello"},
			"input":   "hello world\nfoo bar\nhello again",
		}, "hello"},
		{"tr a-z A-Z", map[string]any{
			"command": "tr",
			"args":    []any{"a-z", "A-Z"},
			"input":   "hello",
		}, "HELLO"},
		// sed with stdin
		{"sed s/line/LINE/", map[string]any{
			"command": "sed",
			"args":    []any{"s/line/LINE/"},
			"input":   "first line\nsecond line",
		}, "LINE"},
		// awk with stdin
		{"awk print", map[string]any{
			"command": "awk",
			"args":    []any{"{print $1, $3}"},
			"input":   "a b c\nd e f",
		}, "a c"},
		// args as []string (JSON decode variant)
		{"seq args []string", map[string]any{"command": "seq", "args": []string{"1", "3"}}, "1"},
		// numeric args converted to strings
		{"seq numeric args", map[string]any{"command": "seq", "args": []any{1.0, 3.0}}, "1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tool.Execute(tc.params)
			if result.IsError {
				t.Errorf("exec failed: %s", result.Output)
			}
			if !strings.Contains(result.Output, tc.want) {
				t.Errorf("output=%q missing %q", result.Output, tc.want)
			}
		})
	}
}

func TestLispExecToolWhich(t *testing.T) {
	tool := &LispExecTool{}

	result := tool.Execute(map[string]any{
		"command":   "go",
		"operation": "which",
	})
	if result.IsError {
		t.Errorf("which go failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "go") {
		t.Errorf("which go output=%q missing 'go'", result.Output)
	}
}

func TestLispExecToolWorkingDir(t *testing.T) {
	tool := &LispExecTool{}

	result := tool.Execute(map[string]any{
		"command":     "pwd",
		"working_dir": "/tmp",
	})
	if result.IsError {
		t.Errorf("pwd with working_dir failed: %s", result.Output)
	}
}

func TestLispExecToolEnv(t *testing.T) {
	tool := &LispExecTool{}

	result := tool.Execute(map[string]any{
		"command": "env",
		"env":     map[string]any{"MYVAR": "test123"},
	})
	if result.IsError {
		t.Errorf("env with custom env failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "MYVAR=test123") {
		t.Errorf("env output=%q missing MYVAR=test123", result.Output)
	}
}
