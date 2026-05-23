package tools

import (
	"testing"
)

func TestExtractPlistValue(t *testing.T) {
	tests := []struct {
		name   string
		plist  string
		key    string
		expect string
	}{
		// Basic extraction
		{"basic stdout", `(:stdout "hello" :stderr "" :exit-code 0)`, ":stdout", "hello"},
		{"basic stderr", `(:stdout "hello" :stderr "err" :exit-code 0)`, ":stderr", "err"},
		{"basic exit-code", `(:stdout "hello" :stderr "" :exit-code 42)`, ":exit-code", "42"},
		{"zero exit-code", `(:stdout "" :stderr "" :exit-code 0)`, ":exit-code", "0"},

		// Key embedded in string values (should NOT match)
		{"stdout contains :stdout", `(:stdout "hello :stdout world" :stderr "" :exit-code 0)`, ":stdout", "hello :stdout world"},
		{"stdout contains :stderr", `(:stdout "no :stderr here" :stderr "actual" :exit-code 0)`, ":stderr", "actual"},
		{"stdout contains :exit-code", `(:stdout "exit :exit-code 99" :stderr "" :exit-code 0)`, ":exit-code", "0"},
		{"multiple embedded keys", `(:stdout ":stderr :exit-code" :stderr "clean" :exit-code 0)`, ":stderr", "clean"},
		{"value IS the key name", `(:stdout ":stdout" :stderr "" :exit-code 0)`, ":stdout", ":stdout"},
		{"many repeats in value", `(:stdout ":stderr:stderr:stderr" :stderr "final" :exit-code 0)`, ":stderr", "final"},
		{"all three keys in value", `(:stdout ":stdout :stderr :exit-code" :stderr "ok" :exit-code 1)`, ":exit-code", "1"},

		// Escaped quotes in values
		{"escaped quote in stdout", `(:stdout "he said \"hello\"" :stderr "" :exit-code 0)`, ":stdout", "he said \"hello\""},
		{"escaped quote then key in string", `(:stdout "a\"b :stderr c" :stderr "real" :exit-code 0)`, ":stderr", "real"},
		{"multiple escaped quotes", `(:stdout "\"x\" :stderr" :stderr "actual" :exit-code 0)`, ":stderr", "actual"},

		// Background result fields
		{"background flag", `(:stdout "" :stderr "" :exit-code -1 :background 1 :stall-reason "waiting")`, ":background", "1"},
		{"stall-reason", `(:stdout "" :stderr "" :exit-code -1 :background 1 :stall-reason "waiting for input")`, ":stall-reason", "waiting for input"},

		// Missing key
		{"missing key", `(:stdout "ok" :stderr "" :exit-code 0)`, ":background", ""},
		{"empty plist", `()`, ":stdout", ""},

		// Value with newlines (escaped)
		{"multiline stdout", "(:stdout \"line1\\nline2\" :stderr \"\" :exit-code 0)", ":stdout", "line1\nline2"},

		// Real-world: command output that looks like plist keys
		{"go test output", `(:stdout "ok :stderr 0.001s" :stderr "" :exit-code 0)`, ":stderr", ""},
		{"docker output", `(:stdout "STATUS :exit-code 0" :stderr "" :exit-code 0)`, ":exit-code", "0"},
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
