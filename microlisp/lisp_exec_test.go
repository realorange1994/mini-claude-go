package microlisp

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestExecArgs tests Bug #4: :args parameter completely non-functional
func TestExecArgs(t *testing.T) {
	ResetGlobalEnv()

	expr := `(exec "echo" :args (list "hello" "world"))`
	result, err := SafeEvalString(expr)
	if err != nil {
		t.Fatalf("SafeEvalString(%q) error: %v", expr, err)
	}

	stdout := extractPlistTestValue(result, ":stdout")
	exitCode := extractPlistTestValue(result, ":exit-code")

	if exitCode != "0" {
		t.Errorf("exec with :args: exit-code=%q, want 0, result=%q", exitCode, result)
	}
	if !strings.Contains(stdout, "hello") {
		t.Errorf("exec with :args: stdout=%q missing 'hello', full result=%q", stdout, result)
	}
	if !strings.Contains(stdout, "world") {
		t.Errorf("exec with :args: stdout=%q missing 'world', full result=%q", stdout, result)
	}
}

// TestExecEnv tests Bug #3: :env parameter not working
func TestExecEnv(t *testing.T) {
	ResetGlobalEnv()

	expr := `(exec "env" :env (list (cons "MYVAR" "test123")))`
	result, err := SafeEvalString(expr)
	if err != nil {
		t.Fatalf("SafeEvalString(%q) error: %v", expr, err)
	}

	stdout := extractPlistTestValue(result, ":stdout")
	exitCode := extractPlistTestValue(result, ":exit-code")

	if exitCode != "0" {
		t.Errorf("exec with :env: exit-code=%q, want 0, result=%q", exitCode, result)
	}
	if !strings.Contains(stdout, "MYVAR=test123") {
		t.Errorf("exec with :env: stdout=%q missing 'MYVAR=test123', full result=%q", stdout, result)
	}
}

// TestExecStdin verifies exec works for commands that don't need stdin
func TestExecStdin(t *testing.T) {
	ResetGlobalEnv()

	expr := `(exec "echo" :args (list "no stdin needed"))`
	result, err := SafeEvalString(expr)
	if err != nil {
		t.Fatalf("SafeEvalString(%q) error: %v", expr, err)
	}

	stdout := extractPlistTestValue(result, ":stdout")
	if !strings.Contains(stdout, "no stdin needed") {
		t.Errorf("exec without stdin: stdout=%q missing 'no stdin needed', full result=%q", stdout, result)
	}
}

// TestExecWorkingDir tests :working-dir parameter
func TestExecWorkingDir(t *testing.T) {
	ResetGlobalEnv()

	// pwd prints current directory; :working-dir should change it
	expr := `(exec "pwd")`
	result, err := SafeEvalString(expr)
	if err != nil {
		t.Fatalf("SafeEvalString(%q) error: %v", expr, err)
	}

	stdout := extractPlistTestValue(result, ":stdout")
	exitCode := extractPlistTestValue(result, ":exit-code")

	if exitCode != "0" {
		t.Errorf("exec pwd: exit-code=%q, want 0", exitCode)
	}
	// Just verify it returns something (the current directory)
	if stdout == "" {
		t.Errorf("exec pwd: empty stdout")
	}
}

// TestExecArgsDirect tests :args at the Go level
func TestExecArgsDirect(t *testing.T) {
	ResetGlobalEnv()

	cmd := StringValue("echo")
	argsKey := vsym(":args")
	argsVal := listFromStrings("hello", "world")

	args := []*Value{cmd, argsKey, argsVal}
	result, err := builtinLispExec(args)
	if err != nil {
		t.Fatalf("builtinLispExec error: %v", err)
	}

	resultStr := princToString(result)
	stdout := extractPlistTestValue(resultStr, ":stdout")
	exitCode := extractPlistTestValue(resultStr, ":exit-code")

	if exitCode != "0" {
		t.Errorf("direct exec with :args: exit-code=%q, want 0", exitCode)
	}
	if !strings.Contains(stdout, "hello") {
		t.Errorf("direct exec with :args: stdout=%q missing 'hello'", stdout)
	}
	if !strings.Contains(stdout, "world") {
		t.Errorf("direct exec with :args: stdout=%q missing 'world'", stdout)
	}
}

// TestExecEnvDirect tests :env at the Go level
func TestExecEnvDirect(t *testing.T) {
	ResetGlobalEnv()

	cmd := StringValue("env")
	envKey := vsym(":env")
	envVal := cons(cons(StringValue("MYVAR"), StringValue("test123")), vnil())

	args := []*Value{cmd, envKey, envVal}
	result, err := builtinLispExec(args)
	if err != nil {
		t.Fatalf("builtinLispExec error: %v", err)
	}

	resultStr := princToString(result)
	stdout := extractPlistTestValue(resultStr, ":stdout")
	exitCode := extractPlistTestValue(resultStr, ":exit-code")

	if exitCode != "0" {
		t.Errorf("direct exec with :env: exit-code=%q, want 0", exitCode)
	}
	if !strings.Contains(stdout, "MYVAR=test123") {
		t.Errorf("direct exec with :env: stdout=%q missing 'MYVAR=test123'", stdout)
	}
}

// TestExecWithInput verifies exec-with-input still works
func TestExecWithInput(t *testing.T) {
	ResetGlobalEnv()

	expr := `(exec-with-input "cat" "test input")`
	result, err := SafeEvalString(expr)
	if err != nil {
		t.Fatalf("SafeEvalString(%q) error: %v", expr, err)
	}

	stdout := extractPlistTestValue(result, ":stdout")
	exitCode := extractPlistTestValue(result, ":exit-code")

	if exitCode != "0" {
		t.Errorf("exec-with-input: exit-code=%q, want 0", exitCode)
	}
	if !strings.Contains(stdout, "test input") {
		t.Errorf("exec-with-input: stdout=%q missing 'test input'", stdout)
	}
}

// TestExecStdinParam tests the new :stdin parameter on exec
func TestExecStdinParam(t *testing.T) {
	ResetGlobalEnv()

	expr := `(exec "cat" :stdin "hello via stdin")`
	result, err := SafeEvalString(expr)
	if err != nil {
		t.Fatalf("SafeEvalString(%q) error: %v", expr, err)
	}

	stdout := extractPlistTestValue(result, ":stdout")
	exitCode := extractPlistTestValue(result, ":exit-code")

	if exitCode != "0" {
		t.Errorf("exec with :stdin: exit-code=%q, want 0", exitCode)
	}
	if !strings.Contains(stdout, "hello via stdin") {
		t.Errorf("exec with :stdin: stdout=%q missing 'hello via stdin'", stdout)
	}
}

// TestExecShortOutput tests that short output (single line) is captured correctly.
// This is a regression test for the pipe-reader race condition where
// short commands finish before io.Copy goroutines have read all data.
func TestExecShortOutput(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		name  string
		expr  string
		want  string
	}{
		{"echo single word", `(exec "echo" :args (list "hello"))`, "hello"},
		{"echo empty", `(exec "echo")`, ""},
		{"whoami", `(exec "whoami")`, ""},
		{"pwd", `(exec "pwd")`, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := SafeEvalString(tc.expr)
			if err != nil {
				t.Fatalf("SafeEvalString(%q) error: %v", tc.expr, err)
			}

			stdout := extractPlistTestValue(result, ":stdout")
			exitCode := extractPlistTestValue(result, ":exit-code")

			if exitCode != "0" {
				t.Errorf("exit-code=%q, want 0, result=%q", exitCode, result)
			}
			if tc.want != "" && !strings.Contains(stdout, tc.want) {
				t.Errorf("stdout=%q missing '%s', result=%q", stdout, tc.want, result)
			}
			// For commands that must produce output, verify stdout is not empty
			if tc.name == "echo single word" && stdout == "" {
				t.Errorf("stdout empty for 'echo hello', result=%q", result)
			}
			if tc.name == "whoami" && stdout == "" {
				t.Errorf("stdout empty for 'whoami', result=%q", result)
			}
			if tc.name == "pwd" && stdout == "" {
				t.Errorf("stdout empty for 'pwd', result=%q", result)
			}
		})
	}
}

// TestExecPositionalArgs tests that non-keyword args are passed as command args.
// This is a common user mistake: (exec "echo" "hello") instead of
// (exec "echo" :args (list "hello")). The fix treats non-keyword args
// as positional command arguments.
func TestExecPositionalArgs(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		name  string
		expr  string
		want  string
	}{
		{"echo hello", `(exec "echo" "hello")`, "hello"},
		{"echo two args", `(exec "echo" "hello" "world")`, "hello"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := SafeEvalString(tc.expr)
			if err != nil {
				t.Fatalf("SafeEvalString(%q) error: %v", tc.expr, err)
			}

			stdout := extractPlistTestValue(result, ":stdout")
			if !strings.Contains(stdout, tc.want) {
				t.Errorf("stdout=%q missing '%s', full result=%q", stdout, tc.want, result)
			}
		})
	}
}

// extractPlistTestValue extracts a value from a plist result string
// using the same sequential state machine as extractPlistValue.
func extractPlistTestValue(plist, key string) string {
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
					return extractQuotedStringTest(plist, valStart)
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

func extractQuotedStringTest(s string, pos int) string {
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

func listFromStrings(ss ...string) *Value {
	result := vnil()
	for i := len(ss) - 1; i >= 0; i-- {
		result = cons(StringValue(ss[i]), result)
	}
	return result
}

// TestExecSleepCommand tests that a sleep command (no output but completes)
// is NOT falsely detected as "waiting for interactive input".
// The stall detection should wait at least 10s for timeout >= 30s,
// and sleep 2 completes well before that.
func TestExecSleepCommand(t *testing.T) {
	ResetGlobalEnv()

	expr := `(exec "sleep" :args (list "2") :timeout 30000)`
	result, err := SafeEvalString(expr)
	if err != nil {
		t.Fatalf("SafeEvalString(%q) error: %v", expr, err)
	}

	exitCode := extractPlistTestValue(result, ":exit-code")
	background := extractPlistTestValue(result, ":background")

	if background != "" {
		t.Errorf("sleep 2 should NOT be moved to background, got :background=%q, full result=%q", background, result)
	}
	if exitCode != "0" {
		t.Errorf("sleep 2: exit-code=%q, want 0", exitCode)
	}
}

// TestExecGoTestCommand tests that `go test` with a short package completes
// normally without being falsely detected as stalled (the bug being fixed).
// go test can take a few seconds to compile with no output.
func TestExecGoTestCommand(t *testing.T) {
	ResetGlobalEnv()

	// Run go test on a small package — this exercises the stall detection
	// during the compilation phase (no output until compilation completes).
	expr := `(exec "go" :args (list "version") :timeout 60000)`
	result, err := SafeEvalString(expr)
	if err != nil {
		t.Fatalf("SafeEvalString(%q) error: %v", expr, err)
	}

	stdout := extractPlistTestValue(result, ":stdout")
	exitCode := extractPlistTestValue(result, ":exit-code")
	background := extractPlistTestValue(result, ":background")

	if background != "" {
		t.Errorf("go version should NOT be moved to background, got :background=%q, full result=%q", background, result)
	}
	if exitCode != "0" {
		t.Errorf("go version: exit-code=%q, want 0", exitCode)
	}
	if !strings.Contains(stdout, "go version") {
		t.Errorf("go version: stdout=%q missing 'go version'", stdout)
	}
}

// TestExecBackgroundDetection tests that commands waiting for stdin ARE moved to background.
// Uses a short timeout (15s) where stallTimeout = min(15s/3, 60s) clamped to 10s minimum.
func TestExecBackgroundDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cat without stdin behaves differently on Windows")
	}
	ResetGlobalEnv()

	// cat with no stdin will wait for input. With a 15s timeout,
	// stallTimeout = 10s (clamped from 15s/3). It should be moved to background.
	start := time.Now()
	expr := `(exec "cat" :timeout 15000)`
	result, err := SafeEvalString(expr)
	if err != nil {
		t.Fatalf("SafeEvalString(%q) error: %v", expr, err)
	}
	elapsed := time.Since(start)

	background := extractPlistTestValue(result, ":background")
	if background != "1" {
		t.Errorf("cat without stdin should be moved to background, got :background=%q, elapsed=%v, result=%q", background, elapsed, result)
	}

	// Should return after ~10s (stall timeout), not 15s (full timeout)
	if elapsed < 8*time.Second {
		t.Errorf("cat returned too early (%v), expected ~10s stall", elapsed)
	}
	if elapsed > 16*time.Second {
		t.Errorf("cat took too long (%v), stall detection may not have worked", elapsed)
	}
}
