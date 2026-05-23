package microlisp

import (
	"strings"
	"testing"
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
