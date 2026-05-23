package tools

import (
	"strings"
	"testing"
)

func TestLispExecToolBashC(t *testing.T) {
	tool := &LispExecTool{}

	tests := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{"bash -c simple", map[string]any{
			"command": "bash",
			"args":    []any{"-c", "echo Hello"},
		}, "Hello"},
		{"bash -c single-quoted", map[string]any{
			"command": "bash",
			"args":    []any{"-c", "echo 'Hello'"},
		}, "Hello"},
		{"bash -c double-quoted", map[string]any{
			"command": "bash",
			"args":    []any{"-c", `echo "Hello"`},
		}, "Hello"},
		// command string with bash -c
		{"command string bash -c", map[string]any{
			"command": `bash -c "echo 'Hello'"`,
		}, "Hello"},
		// command string echo 'Hello' (needs shell wrapping)
		{"command string echo quoted", map[string]any{
			"command": `echo 'Hello'`,
		}, "Hello"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tool.Execute(tc.params)
			if result.IsError {
				t.Errorf("exec error: %s", result.Output)
			}
			if !strings.Contains(result.Output, tc.want) {
				t.Errorf("output=%q missing %q", result.Output, tc.want)
			}
		})
	}
}
