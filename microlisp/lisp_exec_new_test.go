package microlisp

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// ─── needShellWrap ──────────────────────────────────────────────────────────

func TestNeedShellWrap(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		// Shell builtins — need wrap
		{"cd", "cd", true},
		{"export", "export", true},
		{"source", "source", true},
		{"alias", "alias", true},
		{"set", "set", true},
		{"ulimit", "ulimit", true},
		{"eval", "eval", true},
		{"true", "true", true},
		{"false", "false", true},
		{"read", "read", true},

		// Regular commands — no wrap
		{"ls", "ls", false},
		{"echo", "echo", false},
		{"mkdir", "mkdir", false},
		{"go", "go", false},
		{"python", "python", false},
		{"git", "git", false},
		{"docker", "docker", false},
		{"cat", "cat", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := needShellWrap(tc.cmd)
			if got != tc.want {
				t.Errorf("needShellWrap(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

// ─── containsShellSyntax ────────────────────────────────────────────────────

func TestContainsShellSyntax(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"pipe", []string{"|", "grep"}, true},
		{"redirect out", []string{">", "file.txt"}, true},
		{"redirect in", []string{"<", "input.txt"}, true},
		{"and chain", []string{"&&", "echo"}, true},
		{"or chain", []string{"||", "echo"}, true},
		{"semicolon", []string{";", "cmd"}, true},
		{"dollar paren", []string{"$(whoami)"}, true},
		{"backtick", []string{"`date`"}, true},
		{"heredoc", []string{"<<", "EOF"}, true},
		{"append redirect", []string{">>", "log.txt"}, true},
		{"redirect both", []string{"2>&1"}, true},
		{"here-string", []string{"<<<", "data"}, true},
		{"safe args", []string{"-la", "/tmp"}, false},
		{"no args", []string{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := containsShellSyntax(tc.args)
			if got != tc.want {
				t.Errorf("containsShellSyntax(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

// ─── wrapShellBuiltin ───────────────────────────────────────────────────────

func TestWrapShellBuiltin(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		args     []string
		wantProg string
		wantArg1 string // -c argument first part
	}{
		{"cd /tmp", "cd", []string{"/tmp"}, "bash", "cd /tmp"},
		{"export FOO=bar", "export", []string{"FOO=bar"}, "bash", "export FOO=bar"},
		{"source script", "source", []string{"script.sh"}, "bash", "source script.sh"},
		{"no args cd", "cd", []string{}, "bash", "cd"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prog, args := wrapShellBuiltin(tc.command, tc.args)
			if prog != tc.wantProg {
				t.Errorf("program = %q, want %q", prog, tc.wantProg)
			}
			if len(args) < 2 || args[0] != "-c" {
				t.Fatalf("expected args[0]='-c', got %v", args)
			}
			if !strings.HasPrefix(args[1], tc.wantArg1) {
				t.Errorf("args[1] = %q, expected prefix %q", args[1], tc.wantArg1)
			}
		})
	}
}

// ─── extractCwdFromStdout ──────────────────────────────────────────────────

func TestExtractCwdFromStdout(t *testing.T) {
	tests := []struct {
		name      string
		stdout    string
		wantClean string
		wantCwd   string
	}{
		{"no sentinel", "hello world", "hello world", ""},
		{"sentinel at end", "output\n__SHELL_CWD__:/tmp\n", "output", "/tmp"},
		{"sentinel with Windows CRLF", "output\r\n__SHELL_CWD__:/tmp\r\n", "output\r", "/tmp"},
		{"sentinel no trailing newline", "output\n__SHELL_CWD__:/home/user", "output", "/home/user"},
		{"empty output with sentinel", "__SHELL_CWD__:/opt\n", "", "/opt"},
		{"sentinel in middle (last wins)", "a\n__SHELL_CWD__:/first\nb\n__SHELL_CWD__:/last\n", "a\n__SHELL_CWD__:/first\nb", "/last"},
		{"Windows path in CWD", "ok\r\n__SHELL_CWD__:C:\\Users\\foo\r\n", "ok\r", `C:\Users\foo`},
		{"empty stdout", "", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clean, cwd := extractCwdFromStdout(tc.stdout)
			if clean != tc.wantClean {
				t.Errorf("clean = %q, want %q", clean, tc.wantClean)
			}
			if cwd != tc.wantCwd {
				t.Errorf("cwd = %q, want %q", cwd, tc.wantCwd)
			}
		})
	}
}

// ─── exec-simple via SafeEvalString ─────────────────────────────────────────

func TestExecSimple(t *testing.T) {
	ResetGlobalEnv()

	result, err := SafeEvalString(`(exec-simple "echo" "hello simple")`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}
	// exec-simple returns (output . exit-code) dotted pair
	resultStr := result
	if !strings.Contains(resultStr, "hello simple") {
		t.Errorf("exec-simple echo: result=%q missing 'hello simple'", resultStr)
	}
}

func TestExecSimpleExitCode(t *testing.T) {
	ResetGlobalEnv()

	result, err := SafeEvalString(`(exec-simple "false")`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}
	resultStr := result
	// "false" exits with code 1 — should be in the dotted pair
	if !strings.Contains(resultStr, "1") {
		t.Errorf("exec-simple false: result=%q should contain exit code 1", resultStr)
	}
}

func TestExecSimpleWithArgs(t *testing.T) {
	ResetGlobalEnv()

	result, err := SafeEvalString(`(exec-simple "seq" "1" "5")`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}
	resultStr := result
	for i := 1; i <= 5; i++ {
		if !strings.Contains(resultStr, string(rune('0'+i))) {
			t.Errorf("exec-simple seq 1 5: result=%q missing %d", resultStr, i)
		}
	}
}

func TestExecSimpleCommandNotFound(t *testing.T) {
	ResetGlobalEnv()

	result, err := SafeEvalString(`(exec-simple "nonexistent_cmd_xyz")`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}
	resultStr := result
	// Command not found should result in exit code 1
	if !strings.Contains(resultStr, "1") {
		t.Errorf("exec-simple nonexistent: result=%q should contain exit code 1", resultStr)
	}
}

// ─── which builtin ─────────────────────────────────────────────────────────

func TestWhichBuiltin(t *testing.T) {
	ResetGlobalEnv()

	result, err := SafeEvalString(`(which "echo")`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}
	resultStr := result
	if resultStr == "nil" || resultStr == "NIL" {
		t.Errorf("which echo: should find a path, got nil")
	}
}

func TestWhichNotFound(t *testing.T) {
	ResetGlobalEnv()

	result, err := SafeEvalString(`(which "nonexistent_xyz")`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}
	resultStr := result
	if resultStr != "nil" && resultStr != "NIL" && resultStr != "()" {
		t.Errorf("which nonexistent: should return nil, got %q", resultStr)
	}
}

// ─── exec-pipe streaming ────────────────────────────────────────────────────

func TestExecPipeBasic(t *testing.T) {
	ResetGlobalEnv()

	// exec-pipe returns a GoVal that can't easily be used through SafeEvalString
	// (which returns a string). Test it directly via builtinLispExecPipe.
	pipeVal, err := builtinLispExecPipe([]*Value{vstr("seq"), vsym(":args"), listFromStrings("1", "5")})
	if err != nil {
		t.Fatalf("exec-pipe error: %v", err)
	}
	if pipeVal.typ != VGoVal {
		t.Fatalf("exec-pipe should return VGoVal, got %v", pipeVal.typ)
	}

	// Read from the pipe
	pipe, ok := pipeVal.goVal.(*execPipeState)
	if !ok {
		t.Fatalf("exec-pipe returned wrong type: %T", pipeVal.goVal)
	}

	buf := make([]byte, 4096)
	n, err := pipe.stdout.Read(buf)
	if err != nil {
		t.Fatalf("pipe read error: %v", err)
	}
	if n == 0 {
		t.Error("expected data from pipe but got none")
	}

	// Wait for the process to finish
	pipe.cmd.Wait()
	pipe.stdout.Close()
	pipe.stderr.Close()
}

// ─── exec timeout ───────────────────────────────────────────────────────────

func TestExecTimeout(t *testing.T) {
	ResetGlobalEnv()

	// A command with a very short timeout that should time out
	result, err := SafeEvalString(`(exec "sleep" :args (list "10") :timeout 1000)`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}

	resultStr := result
	background := extractPlistTestValue(resultStr, ":background")
	// sleep 10 with 1s timeout should be moved to background
	if background == "" {
		t.Errorf("sleep 10 with 1s timeout should be moved to background, result=%q", resultStr)
	}
}

// ─── exec with :stdin ────────────────────────────────────────────────────────

func TestExecStdinParamViaLisp(t *testing.T) {
	ResetGlobalEnv()

	result, err := SafeEvalString(`(exec "cat" :stdin "hello from stdin test")`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}

	stdout := extractPlistTestValue(result, ":stdout")
	if !strings.Contains(stdout, "hello from stdin test") {
		t.Errorf("exec cat :stdin: stdout=%q missing expected content", stdout)
	}
}

// ─── exec-with-input ────────────────────────────────────────────────────────

func TestExecWithInputViaLisp(t *testing.T) {
	ResetGlobalEnv()

	result, err := SafeEvalString(`(exec-with-input "cat" "piped content here")`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}

	stdout := extractPlistTestValue(result, ":stdout")
	if !strings.Contains(stdout, "piped content here") {
		t.Errorf("exec-with-input cat: stdout=%q missing expected content", stdout)
	}
}

// ─── exec with :env via Lisp ────────────────────────────────────────────────

func TestExecEnvViaLisp(t *testing.T) {
	ResetGlobalEnv()

	result, err := SafeEvalString(`(exec "env" :env (list (cons "TESTEXEC_ENV_VAR" "envvalue456")))`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}

	stdout := extractPlistTestValue(result, ":stdout")
	if !strings.Contains(stdout, "TESTEXEC_ENV_VAR=envvalue456") {
		t.Errorf("exec env :env: stdout=%q missing env var", stdout)
	}
}

// ─── Stall detection: proportional timeout ──────────────────────────────────

func TestExecStallProportionalTimeout(t *testing.T) {
	ResetGlobalEnv()

	// With a 30s timeout, stallTimeout = 30s/3 = 10s (clamped from 10s min)
	// sleep 2 should finish well before 10s stall timeout
	start := time.Now()
	result, err := SafeEvalString(`(exec "sleep" :args (list "2") :timeout 30000)`)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}

	resultStr := result
	background := extractPlistTestValue(resultStr, ":background")
	exitCode := extractPlistTestValue(resultStr, ":exit-code")

	if background != "" {
		t.Errorf("sleep 2 with 30s timeout should NOT be moved to background, got :background=%q, elapsed=%v", background, elapsed)
	}
	if exitCode != "0" {
		t.Errorf("sleep 2: exit-code=%q, want 0", exitCode)
	}
	if elapsed > 8*time.Second {
		t.Errorf("sleep 2 took too long (%v), stall detection may have triggered incorrectly", elapsed)
	}
}

// ─── cd auto-detection in executeCommand ─────────────────────────────────────

func TestExecCdCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cd command test requires Unix shell")
	}
	ResetGlobalEnv()

	// cd /tmp should work — executeCommand handles it specially
	result, err := SafeEvalString(`(exec "cd" :args (list "/tmp"))`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}

	resultStr := result
	exitCode := extractPlistTestValue(resultStr, ":exit-code")
	cwd := extractPlistTestValue(resultStr, ":cwd")

	if exitCode != "0" {
		t.Errorf("cd /tmp: exit-code=%q, want 0, result=%q", exitCode, resultStr)
	}
	if cwd != "/tmp" {
		t.Errorf("cd /tmp: cwd=%q, want /tmp", cwd)
	}
}

// ─── shell builtin auto-wrap ────────────────────────────────────────────────

func TestExecShellBuiltinExport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("export test requires Unix shell")
	}
	ResetGlobalEnv()

	result, err := SafeEvalString(`(exec "export" :args (list "MY_EXPORT_VAR=hello"))`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}

	resultStr := result
	exitCode := extractPlistTestValue(resultStr, ":exit-code")
	if exitCode != "0" {
		t.Errorf("export: exit-code=%q, want 0, result=%q", exitCode, resultStr)
	}
}

// ─── makeExecResult / makeBackgroundExecResult ──────────────────────────────

func TestMakeExecResult(t *testing.T) {
	result := makeExecResult("out", "err", 42, "/tmp")
	str := princToString(result)

	if !strings.Contains(str, ":stdout") {
		t.Errorf("result missing :stdout, got: %q", str)
	}
	if !strings.Contains(str, ":stderr") {
		t.Errorf("result missing :stderr, got: %q", str)
	}
	if !strings.Contains(str, ":exit-code") {
		t.Errorf("result missing :exit-code, got: %q", str)
	}
	if !strings.Contains(str, ":cwd") {
		t.Errorf("result missing :cwd, got: %q", str)
	}
}

func TestMakeExecResultNoCwd(t *testing.T) {
	result := makeExecResult("out", "err", 0, "")
	str := princToString(result)

	if strings.Contains(str, ":cwd") {
		t.Errorf("result should not have :cwd when empty, got: %q", str)
	}
}

func TestMakeBackgroundExecResult(t *testing.T) {
	result := makeBackgroundExecResult("out", "err", 1234, "timed out", "/tmp")
	str := princToString(result)

	if !strings.Contains(str, ":background") {
		t.Errorf("result missing :background, got: %q", str)
	}
	if !strings.Contains(str, ":stall-reason") {
		t.Errorf("result missing :stall-reason, got: %q", str)
	}
	if !strings.Contains(str, ":exit-code") {
		t.Errorf("result missing :exit-code, got: %q", str)
	}
	if !strings.Contains(str, "timed out") {
		t.Errorf("result missing stall reason text, got: %q", str)
	}
}

// ─── lispListToStringSlice ──────────────────────────────────────────────────

func TestLispListToStringSlice(t *testing.T) {
	ResetGlobalEnv()

	tests := []struct {
		name string
		expr string
		want []string
	}{
		{"empty list", `(list)`, nil},
		{"single string", `(list "hello")`, []string{"hello"}},
		{"multiple strings", `(list "a" "b" "c")`, []string{"a", "b", "c"}},
		{"with spaces", `(list "-la" "/tmp")`, []string{"-la", "/tmp"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			val, err := EvalString(tc.expr, globalEnv)
			if err != nil {
				t.Fatalf("EvalString(%q) error: %v", tc.expr, err)
			}
			got := lispListToStringSlice(val)
			if len(got) != len(tc.want) {
				t.Errorf("lispListToStringSlice result = %v (len=%d), want %v (len=%d)", got, len(got), tc.want, len(tc.want))
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("lispListToStringSlice[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ─── lispEnvToGoSlice ────────────────────────────────────────────────────────

func TestLispEnvToGoSlice(t *testing.T) {
	ResetGlobalEnv()

	val, err := EvalString(`(list (cons "KEY" "VAL"))`, globalEnv)
	if err != nil {
		t.Fatalf("EvalString error: %v", err)
	}

	got := lispEnvToGoSlice(val)
	if len(got) != 1 || got[0] != "KEY=VAL" {
		t.Errorf("lispEnvToGoSlice = %v, want [KEY=VAL]", got)
	}
}

func TestLispEnvToGoSliceEmpty(t *testing.T) {
	ResetGlobalEnv()

	val, err := EvalString(`(list)`, globalEnv)
	if err != nil {
		t.Fatalf("EvalString error: %v", err)
	}

	got := lispEnvToGoSlice(val)
	if len(got) != 0 {
		t.Errorf("lispEnvToGoSlice empty list = %v, want []", got)
	}
}

// ─── exec-pipe kill ─────────────────────────────────────────────────────────

func TestExecPipeKill(t *testing.T) {
	ResetGlobalEnv()

	// Start a long-running pipe via direct builtin call
	pipeVal, err := builtinLispExecPipe([]*Value{vstr("sleep"), vsym(":args"), listFromStrings("30")})
	if err != nil {
		t.Fatalf("exec-pipe error: %v", err)
	}

	pipe, ok := pipeVal.goVal.(*execPipeState)
	if !ok {
		t.Fatalf("exec-pipe returned wrong type")
	}

	// Kill it
	killExecProcessTree(pipe.cmd)
}

// ─── positional args via exec ────────────────────────────────────────────────

func TestExecPositionalArgsViaLisp(t *testing.T) {
	ResetGlobalEnv()

	// (exec "echo" "hello" "world") — positional args
	result, err := SafeEvalString(`(exec "echo" "hello" "world")`)
	if err != nil {
		t.Fatalf("SafeEvalString error: %v", err)
	}

	stdout := extractPlistTestValue(result, ":stdout")
	if !strings.Contains(stdout, "hello") {
		t.Errorf("exec echo positional: stdout=%q missing 'hello'", stdout)
	}
	if !strings.Contains(stdout, "world") {
		t.Errorf("exec echo positional: stdout=%q missing 'world'", stdout)
	}
}
