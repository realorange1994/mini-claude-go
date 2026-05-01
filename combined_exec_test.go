package main

import (
	"fmt"
	"testing"
)

func TestCombinedExecCommands(t *testing.T) {
	commands := []struct {
		cmd      string
		expected bool // true = auto-allowed, false = not auto-allowed (LLM or blocked)
	}{
		{"go build && go test", true},
		{"cargo build && cargo test", true},
		{"ls -la && cat main.go", true},
		{"echo hello && rm -rf /", false},
		{"cat /etc/passwd || echo failed", true},
		{"ls && sudo rm -rf /", false},
		{"git status && git log", false},
		{`find . -name "*.go" | xargs wc -l`, true},
		{"cat main.go | grep func", true},
		{"echo test > /dev/null", true},        // writing to /dev/null is harmless
		{"echo hello 2>&1", true},              // stderr-to-stdout redirect is safe
		{"echo hacked > /etc/passwd", false},   // redirect to /etc is dangerous
		{`ls || echo "no files"`, true},
	}

	for _, tc := range commands {
		input := map[string]any{"command": tc.cmd}
		got := IsAutoAllowlisted("exec", input)
		safeStr := isSafeExecCommand(tc.cmd)
		dangerous := hasDangerousPatterns(tc.cmd)

		var detail string
		if safeStr {
			detail = "SafePrefix=YES"
		} else {
			detail = "SafePrefix=NO"
		}
		if dangerous {
			detail += ", Dangerous=YES"
		} else {
			detail += ", Dangerous=NO"
		}
		if got != tc.expected {
			t.Errorf("FAIL: %q\n  Expected allowlisted=%v, got %v (%s)",
				tc.cmd, tc.expected, got, detail)
		} else {
			fmt.Printf("PASS: %q -> allowlisted=%v (%s)\n", tc.cmd, got, detail)
		}
	}
}
