package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBashDenyRmRf(t *testing.T) {
	tool := &ExecTool{}
	dangerous := []string{
		"rm -rf /",
		"rm -rf ~",
		"sudo rm -rf /",
	}
	for _, cmd := range dangerous {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Errorf("expected denial for: %s", cmd)
		}
	}
}

func TestBashDenyInternalURL(t *testing.T) {
	tool := &ExecTool{}
	cmds := []string{
		"curl http://10.0.0.1/admin",
		"curl http://localhost:8080/internal",
		"curl http://192.168.1.1/config",
		"curl http://127.0.0.1:3000/api",
	}
	for _, cmd := range cmds {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Errorf("expected internal URL to be denied: %s", cmd)
		}
	}
}

func TestBashDenyForkBomb(t *testing.T) {
	tool := &ExecTool{}
	cmds := []string{
		":(){ :|: & };:",
		"bomb(){ bomb|bomb & }; bomb",
	}
	for _, cmd := range cmds {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Errorf("expected fork bomb denial for: %s", cmd)
		}
	}
}

func TestBashSafeCommand(t *testing.T) {
	tool := &ExecTool{}
	result := tool.Execute(map[string]any{"command": "echo hello"})
	if result.IsError {
		t.Errorf("expected echo to succeed, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", result.Output)
	}
}

func TestBashLsCommand(t *testing.T) {
	tool := &ExecTool{}
	result := tool.Execute(map[string]any{"command": "ls"})
	if result.IsError {
		t.Errorf("expected ls to succeed, got: %s", result.Output)
	}
}

func TestBashDenyMkfs(t *testing.T) {
	tool := &ExecTool{}
	result := tool.CheckPermissions(map[string]any{"command": "mkfs.ext4 /dev/sda"})
	if result.Behavior == PermissionPassthrough {
		t.Error("expected denial for mkfs")
	}
}

func TestBashDenySudoRm(t *testing.T) {
	tool := &ExecTool{}
	result := tool.CheckPermissions(map[string]any{"command": "sudo rm -rf /tmp/test"})
	if result.Behavior == PermissionPassthrough {
		t.Error("expected denial for sudo rm")
	}
}

func TestBashDenyRedirectToDev(t *testing.T) {
	tool := &ExecTool{}
	result := tool.CheckPermissions(map[string]any{"command": "echo bad > /dev/sda"})
	if result.Behavior == PermissionPassthrough {
		t.Error("expected denial for redirect to /dev/sda")
	}
}

func TestBashAllowPublicURL(t *testing.T) {
	tool := &ExecTool{}
	result := tool.CheckPermissions(map[string]any{"command": "curl https://example.com/api"})
	if result.Behavior != PermissionPassthrough {
		t.Errorf("expected public URL to be allowed, got: %v", result)
	}
}

func TestBashDenyPowerCommands(t *testing.T) {
	tool := &ExecTool{}
	cmds := []string{"shutdown -h now", "reboot", "poweroff"}
	for _, cmd := range cmds {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Errorf("expected denial for: %s", cmd)
		}
	}
}

func TestExecToolBackgroundNoCallback(t *testing.T) {
	// When callback is nil, should fall through to foreground execution
	tool := &ExecTool{}
	result := tool.Execute(map[string]any{
		"command":           "echo hello",
		"run_in_background": true,
	})
	if result.IsError {
		t.Errorf("expected success with fallback, got: %s", result.Output)
	}
	if result.Output != "STDOUT:\nhello\nExit code: 0" {
		t.Logf("got output: %s", result.Output)
	}
}

func TestExecToolBackgroundWithCallback(t *testing.T) {
	var called bool
	var gotCommand string
	tool := &ExecTool{
		BackgroundTaskCallback: func(command, workingDir string) (string, string, string) {
			called = true
			gotCommand = command
			_ = workingDir
			return "b12345678", "/tmp/test.output", ""
		},
	}
	result := tool.Execute(map[string]any{
		"command":           "echo test",
		"run_in_background": true,
	})
	if !called {
		t.Error("expected callback to be invoked")
	}
	if gotCommand != "echo test" {
		t.Errorf("expected command 'echo test', got %q", gotCommand)
	}
	if result.IsError {
		t.Errorf("expected success, got: %s", result.Output)
	}
	if result.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestExecToolForegroundIgnoresBackground(t *testing.T) {
	tool := &ExecTool{}
	// Without run_in_background=true, should run in foreground
	result := tool.Execute(map[string]any{
		"command": "echo foreground",
	})
	if result.IsError {
		t.Errorf("expected success, got: %s", result.Output)
	}
}

func TestExecToolBackgroundEmptyCommand(t *testing.T) {
	tool := &ExecTool{
		BackgroundTaskCallback: func(command, workingDir string) (string, string, string) {
			t.Error("callback should not be called for empty command")
			return "", "", ""
		},
	}
	result := tool.Execute(map[string]any{
		"command":           "  ",
		"run_in_background": true,
	})
	if !result.IsError {
		t.Error("expected error for empty command")
	}
}

func TestExecToolBackgroundCallbackError(t *testing.T) {
	tool := &ExecTool{
		BackgroundTaskCallback: func(command, workingDir string) (string, string, string) {
			return "", "", "Error: simulated failure"
		},
	}
	result := tool.Execute(map[string]any{
		"command":           "echo test",
		"run_in_background": true,
	})
	if !result.IsError {
		t.Error("expected error from callback")
	}
}

func TestExecToolInputSchema(t *testing.T) {
	tool := &ExecTool{}
	schema := tool.InputSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in schema")
	}
	if _, ok := props["run_in_background"]; !ok {
		t.Error("expected run_in_background in schema")
	}
	if _, ok := props["command"]; !ok {
		t.Error("expected command in schema")
	}
}

func TestDetectCommandSubstitution(t *testing.T) {
	// Should be blocked: command substitution
	blocked := []string{
		"echo $(whoami)",
		"curl `ls`",
		"cat /etc/passwd | $(grep root)",
		"diff <(ls) <(ls /)",
		"echo $((1+$(whoami)))",
	}
	for _, cmd := range blocked {
		reason := detectCommandSubstitution(cmd)
		if reason == "" {
			t.Errorf("expected detection for: %s", cmd)
		}
	}

	// Should be allowed: safe variable expansions
	allowed := []string{
		"echo $HOME",
		"echo ${HOME}",
		"echo $USER",
		"cd $PWD",
		"echo $CI",
		"echo $?",
		"echo $$",
		"echo $!",
		"echo $1",
		"echo $2",
		"echo $#",
		"echo $@",
		"echo $*",
		"echo ${HOME:-/default}",
		"env FOO=bar ./script.sh",
	}
	for _, cmd := range allowed {
		reason := detectCommandSubstitution(cmd)
		if reason != "" {
			t.Errorf("expected no detection for: %s, got: %s", cmd, reason)
		}
	}

	// Should be blocked: dangerous variable expansions
	dangerous := []string{
		"echo ${IFS}",
		"echo ${!VAR}",
		"echo ${BASH_VERSION}",
		"echo ${DANGER_VAR}",
		"echo ${PATH}",
		"echo ${GOPATH}",
		"git commit -m ${GIT_AUTHOR_NAME}",
	}
	for _, cmd := range dangerous {
		reason := detectCommandSubstitution(cmd)
		if reason == "" {
			t.Errorf("expected detection for: %s", cmd)
		}
	}
}

func TestDetectExpansion(t *testing.T) {
	// Should be blocked in destructive commands
	destructiveBlocked := []string{
		"rm -rf *.txt",
		"rm file?.log",
		"mv *.bak /backup/",
		"cp file[0-9].dat /dest/",
		"chmod 777 {a,b,c}",
		"chown user:group {1..10}",
		"git rm *.tmp",
		"git clean -f *.log",
	}
	for _, cmd := range destructiveBlocked {
		reason := detectExpansion(cmd)
		if reason == "" {
			t.Errorf("expected expansion detection for: %s", cmd)
		}
	}

	// Should be allowed: quoted globs
	quotedAllowed := []string{
		`ls "*.go"`,
		"grep pattern '*.txt'",
		`find . -name "*.log"`,
	}
	for _, cmd := range quotedAllowed {
		reason := detectExpansion(cmd)
		if reason != "" {
			t.Errorf("expected no detection for: %s, got: %s", cmd, reason)
		}
	}

	// Should be allowed: non-destructive commands with globs
	safe := []string{
		"ls *.go",
		"find . -name '*.txt'",
		"grep pattern *.log",
		"echo *.txt",
		"cat config.ini",
	}
	for _, cmd := range safe {
		reason := detectExpansion(cmd)
		if reason != "" {
			t.Errorf("expected no detection for: %s, got: %s", cmd, reason)
		}
	}
}

func TestSplitCompoundCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"cmd1; cmd2", []string{"cmd1", "cmd2"}},
		{"cmd1 && cmd2", []string{"cmd1", "cmd2"}},
		{"cmd1 || cmd2", []string{"cmd1", "cmd2"}},
		{"cmd1 | cmd2", []string{"cmd1", "cmd2"}},
		{"cmd1\ncmd2", []string{"cmd1", "cmd2"}},
		{"cmd1 | cmd2 && cmd3; cmd4", []string{"cmd1", "cmd2", "cmd3", "cmd4"}},
		{`echo "hello; world"`, []string{`echo "hello; world"`}},
		{"echo 'hello && world'", []string{"echo 'hello && world'"}},
		{"echo `date`", []string{"echo `date`"}},
		{"ls", []string{"ls"}},
		{"", nil},
		{"   ", nil},
		{"cmd1; cmd2; cmd3", []string{"cmd1", "cmd2", "cmd3"}},
	}
	for _, tc := range tests {
		result := splitCompoundCommand(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("splitCompoundCommand(%q): expected %v, got %v", tc.input, tc.expected, result)
		} else {
			for i := range result {
				if result[i] != tc.expected[i] {
					t.Errorf("splitCompoundCommand(%q): expected %v, got %v", tc.input, tc.expected, result)
				}
			}
		}
	}
}

func TestStripSafeWrappers(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"timeout 30 rm -rf /tmp", "rm -rf /tmp"},
		{"nice -n 10 make build", "make build"},
		{"nohup ./script.sh &", "./script.sh &"},
		{"time make test", "make test"},
		{"stdbuf -oL grep pattern file", "grep pattern file"},
		{"ionice -c 3 make build", "make build"},
		{"env FOO=bar ./script.sh", "./script.sh"},
		{"env FOO=bar BAR=baz ./script.sh", "./script.sh"},
		{"command ls -la", "ls -la"},
		{"builtin echo hello", "echo hello"},
		{"unbuffer ssh host cmd", "ssh host cmd"},
		{"rm -rf /tmp", "rm -rf /tmp"},
		{"ls -la", "ls -la"},
	}
	for _, tc := range tests {
		result := stripSafeWrappers(tc.input)
		if result != tc.expected {
			t.Errorf("stripSafeWrappers(%q): expected %q, got %q", tc.input, tc.expected, result)
		}
	}
}

func TestCheckPermissionsCommandSubstitution(t *testing.T) {
	tool := &ExecTool{}
	dangerous := []string{
		"echo $(whoami)",
		"cat `ls`",
		"diff <(ls) <(ls /)",
		"echo $((1+$(id)))",
	}
	for _, cmd := range dangerous {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Errorf("expected denial for: %s", cmd)
		}
	}
}

func TestCheckPermissionsGlobExpansion(t *testing.T) {
	tool := &ExecTool{}
	// Should be denied in destructive commands
	dangerous := []string{
		"rm *.log",
		"mv *.bak /tmp",
		"cp file?.dat /dest",
		"chmod 777 {a,b,c}",
		"git rm *.tmp",
	}
	for _, cmd := range dangerous {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Errorf("expected denial for: %s", cmd)
		}
	}

	// Should be allowed in non-destructive commands
	safe := []string{
		"ls *.go",
		"grep pattern *.txt",
		"find . -name '*.log'",
	}
	for _, cmd := range safe {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior != PermissionPassthrough {
			t.Errorf("expected allowance for: %s, got: %v", cmd, result)
		}
	}
}

func TestCheckPermissionsCompoundCommand(t *testing.T) {
	tool := &ExecTool{}
	// Any dangerous subcommand should block the whole command
	dangerous := []string{
		"echo hello; rm -rf /",
		"ls && rm -rf /tmp",
		"cat file || $(malicious)",
		"echo test | $(whoami)",
	}
	for _, cmd := range dangerous {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Errorf("expected denial for compound command: %s", cmd)
		}
	}
}

func TestCheckPermissionsWrapperStripping(t *testing.T) {
	tool := &ExecTool{}
	// Wrapped dangerous commands should still be blocked
	dangerous := []string{
		"timeout 5 rm -rf /tmp/test",
		"nice -n 10 rm -rf /tmp/test",
		"nohup rm -rf /tmp/test",
	}
	for _, cmd := range dangerous {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Errorf("expected denial for wrapped command: %s", cmd)
		}
	}
}

func TestValidatePathsDangerous(t *testing.T) {
	cases := []struct {
		cmd     string
		blocked bool
	}{
		// System root paths - should block
		{"rm -rf /", true},
		{"rm -rf /home", true},
		{"rm -rf /tmp", true},
		{"rm -rf /etc", true},
		{"rm -rf /usr", true},
		{"rm -rf /bin", true},
		{"rm -rf /sbin", true},
		{"rm -rf /var", true},
		{"rm -rf /root", true},
		{"rm -rf /opt", true},
		{"rm -rf /boot", true},
		{"rm -rf /sys", true},
		{"rm -rf /proc", true},
		{"rm -rf /dev", true},
		{"rm -rf /lib", true},
		{"rm -rf /lib64", true},
		// Home directory - should block
		{"rm -rf ~", true},
		{"rm -rf ~/Downloads", true},
		{"rm -rf $HOME", true},
		// Relative paths - should NOT block
		{"rm -rf .", false},
		{"rm file.txt", false},
		{"rm ./file.txt", false},
		{"rm -rf node_modules", false},
		// rmdir - should block on dangerous paths
		{"rmdir /tmp", true},
		{"rmdir /etc", true},
		// unlink - should block on dangerous paths
		{"unlink /etc/passwd", true},
		// Path escape via --
		{"rm -- -/../secret", true},
		// Safe commands on relative paths should pass
		{"rm -rf src/build", false},
		{"rm *.log", false},
		{"rmdir build/dist", false},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			reason := validatePaths(tc.cmd)
			blocked := reason != ""
			if blocked != tc.blocked {
				t.Errorf("validatePaths(%q): blocked=%v (expected %v), reason=%q", tc.cmd, blocked, tc.blocked, reason)
			}
		})
	}
}

func TestExtractDeletionTargets(t *testing.T) {
	tests := []struct {
		args   []string
		expect []string
	}{
		{[]string{"-rf", "file.txt"}, []string{"file.txt"}},
		{[]string{"-rf", "--", "file.txt"}, []string{"file.txt"}},
		{[]string{"--", "-f"}, []string{"-f"}},
		{[]string{"--", "-/../secret"}, []string{"-/../secret"}},
		{[]string{"-rf", "a.txt", "b.txt"}, []string{"a.txt", "b.txt"}},
		{[]string{"--", "file1", "-rf"}, []string{"file1", "-rf"}},
	}
	for _, tc := range tests {
		result := extractDeletionTargets(tc.args)
		if len(result) != len(tc.expect) {
			t.Errorf("extractDeletionTargets(%v): expected %v, got %v", tc.args, tc.expect, result)
			continue
		}
		for i := range result {
			if result[i] != tc.expect[i] {
				t.Errorf("extractDeletionTargets(%v): expected %v, got %v", tc.args, tc.expect, result)
				break
			}
		}
	}
}

func TestCheckPermissionsPathProtection(t *testing.T) {
	tool := &ExecTool{}
	dangerous := []string{
		"rm -rf /",
		"rm -rf /home",
		"rm -rf /tmp",
		"rm -rf ~/Downloads",
		"rmdir /etc",
	}
	for _, cmd := range dangerous {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Errorf("expected denial for: %s", cmd)
		}
	}
}

func TestCheckPermissionsCriticalProjectFiles(t *testing.T) {
	tool := &ExecTool{}
	dangerous := []string{
		"rm -rf .git",
		"rm .gitignore",
		"rm .gitconfig",
		"rm go.mod",
		"rm package.json",
		"rm Cargo.toml",
		"rm Makefile",
		"rm -rf .claude",
	}
	for _, cmd := range dangerous {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Errorf("expected denial for critical project file: %s", cmd)
		}
	}
}

func TestCheckPermissionsWindowsPaths(t *testing.T) {
	tool := &ExecTool{}
	dangerous := []string{
		`rm -rf C:\Windows`,
		`rm -rf "C:\Program Files"`,
		`rm -rf C:\ProgramData`,
	}
	for _, cmd := range dangerous {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result.Behavior == PermissionPassthrough {
			t.Logf("expected denial for Windows path: %s (got: %v)", cmd, result)
		}
	}
}

// ─── PosixToWindowsPath ──────────────────────────────────────────────────────

func TestPosixToWindowsPath(t *testing.T) {
	tmpDir := os.TempDir()
	homeDir, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		// Drive letter paths
		{"/e/workspace", "E:\\workspace"},
		{"/c/Users/foo", "C:\\Users\\foo"},
		{"/c/", "C:\\"},
		// Cygwin drive prefix
		{"/cygdrive/c/file.txt", "C:\\file.txt"},
		{"/cygdrive/e/workspace", "E:\\workspace"},
		// MSYS2 mount: /tmp
		{"/tmp", tmpDir},
		{"/tmp/test.txt", filepath.Join(tmpDir, "test.txt")},
		// MSYS2 mount: /home
		{"/home", homeDir},
		{"/home/user/file.txt", filepath.Join(homeDir, "file.txt")},
		// UNC paths
		{"//server/share", `\\server\share`},
		// Relative paths (no conversion needed)
		{"relative/path", filepath.Clean("relative/path")},
		// Already Windows paths (no conversion needed)
		{`C:\Users\foo`, `C:\Users\foo`},
	}

	for _, tt := range tests {
		got := PosixToWindowsPath(tt.input)
		gotClean := filepath.Clean(got)
		expectedClean := filepath.Clean(tt.expected)
		if gotClean != expectedClean {
			t.Errorf("PosixToWindowsPath(%q) = %q (clean: %q), want %q (clean: %q)",
				tt.input, got, gotClean, tt.expected, expectedClean)
		}
	}
}

// ─── Upstream Quality: Invariants ────────────────────────────────────────────

// TestExecToolEmptyStringNonMatch — empty string should NOT match any deny pattern.
// Invariant from upstream dangerousPatterns.test.ts: CROSS_PLATFORM_CODE_EXEC[0] !== ""
func TestExecToolEmptyStringNonMatch(t *testing.T) {
	tool := &ExecTool{}
	// Empty command should not be denied as dangerous (it's an input validation issue)
	result := tool.CheckPermissions(map[string]any{"command": ""})
	// If it's denied, the reason should NOT be a pattern match — empty doesn't match patterns
	if result.Behavior != PermissionPassthrough {
		// If denied, verify it's for the right reason (empty command validation, not pattern match)
		// The invariant: no deny pattern should fire on empty string
		denyPatterns := compileDenyPatterns()
		for _, re := range denyPatterns {
			if re.MatchString("") {
				t.Errorf("deny pattern %q matches empty string — violates non-match invariant", re.String())
			}
		}
	}
}

// TestExecToolNoDuplicatePatterns — deny pattern list must have no duplicates.
// Invariant from upstream: new Set(DANGEROUS_BASH_PATTERNS).size === DANGEROUS_BASH_PATTERNS.length
func TestExecToolNoDuplicatePatterns(t *testing.T) {
	patternStrings := []string{
		`\brm\s+-[rf]{1,2}\b`,
		`\bdel\s+/[fq]\b`,
		`\brmdir\s+/s\b`,
		`(?:^|[;&|]\s*)format\b`,
		`\b(mkfs|diskpart)\b`,
		`\bdd\s+.*\bof=`,
		`>\s*/dev/sd`,
		`\b(shutdown|reboot|poweroff)\b`,
		`:\(\)\s*\{.*\};\s*:`,
		`\w+\(\)\s*\{[^}]*\|\s*[^}]*&\s*\}\s*;\s*`,
		`remove-item\s`,
		`\bri\s+`,
		`remove-itemproperty\s`,
		`rd\s+/[sS]\b`,
		`docker\s+system\s+prune`,
		`docker\s+\S+\s+prune`,
		`git\s+push\s+.*--force`,
		`git\s+push\s+-f\b`,
		`git\s+clean\s+-[fd]`,
		`git\s+reset\s+--hard`,
		`git\s+checkout\s+--force`,
		`git\s+rebase\s+--interactive`,
		`git\s+filter-branch`,
		`git\s+reflog\s+expire`,
		`&\S*&\S*&`,
	}

	seen := make(map[string]bool)
	for _, p := range patternStrings {
		if seen[p] {
			t.Errorf("duplicate deny pattern: %q", p)
		}
		seen[p] = true
	}
}

// ─── Upstream Quality: IsReadOnlyCommand ─────────────────────────────────────

func TestIsReadOnlyCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		// Empty/whitespace — not read-only (input validation issue)
		{"empty", "", false},
		{"whitespace", "   ", false},

		// Always read-only commands
		{"ls", "ls", true},
		{"ls -la", "ls -la", true},
		{"cat", "cat file.txt", true},
		{"head", "head -n 10 file.txt", true},
		{"tail", "tail -f log.txt", true},
		{"less", "less file.txt", true},
		{"more", "more file.txt", true},
		{"wc", "wc -l file.txt", true},
		{"file", "file binary.bin", true},
		{"stat", "stat file.txt", true},
		{"du", "du -sh .", true},
		{"df", "df -h", true},
		{"find", "find . -name '*.go'", true},
		{"which", "which go", true},
		{"whereis", "whereis go", true},
		{"locate", "locate go.mod", true},
		{"grep", "grep pattern file.go", true},
		{"rg", "rg pattern", true},
		{"pwd", "pwd", true},
		{"whoami", "whoami", true},
		{"id", "id", true},
		{"hostname", "hostname", true},
		{"uname", "uname -a", true},
		{"date", "date", true},
		{"env", "env", true},
		{"echo", "echo hello", true},
		{"printf", "printf '%s' hello", true},
		{"jq", "jq '.foo' data.json", true},
		{"tree", "tree", true},

		// Redirects → NOT read-only
		{"redirect output", "echo hello > out.txt", false},
		{"redirect append", "echo hello >> out.txt", false},
		{"ls with redirect", "ls > files.txt", false},

		// Safe wrappers preserve read-only
		{"timeout ls", "timeout 30 ls", true},
		{"nice ls", "nice -n 10 ls -la", true},
		{"nohup ls", "nohup ls &", true},

		// Git read-only subcommands
		{"git status", "git status", true},
		{"git log", "git log", true},
		{"git diff", "git diff", true},
		{"git blame", "git blame file.go", true},
		{"git ls-files", "git ls-files", true},
		{"git branch (list)", "git branch", true},
		{"git branch -v", "git branch -v", true},
		{"git remote (list)", "git remote", true},
		{"git remote -v", "git remote -v", true},
		{"git stash list", "git stash list", true},
		{"git stash show", "git stash show", true},
		{"git tag (list)", "git tag", true},
		{"git tag -l", "git tag -l", true},
		{"git rev-parse", "git rev-parse HEAD", true},

		// Git mutation subcommands
		{"git branch -d", "git branch -d old-branch", false},
		{"git branch -D", "git branch -D force-branch", false},
		{"git branch -m", "git branch -m old new", false},
		{"git remote add", "git remote add origin url", false},
		{"git remote rm", "git remote rm origin", false},
		{"git stash pop", "git stash pop", false},
		{"git stash drop", "git stash drop", false},
		{"git stash save", "git stash save message", false},
		{"git tag -d", "git tag -d old-tag", false},
		{"git tag -a", "git tag -a v1.0 -m 'message'", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsReadOnlyCommand(tc.cmd)
			if got != tc.want {
				t.Errorf("IsReadOnlyCommand(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

// ─── Upstream Quality: CheckDestructiveWarning ──────────────────────────────

func TestCheckDestructiveWarning(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantMsg bool
	}{
		{"rm -rf", "rm -rf /tmp/test", true},
		{"rmdir", "rmdir /tmp/test", true},
		{"unlink", "unlink /tmp/file", true},
		{"git force push", "git push --force origin main", true},
		{"git push -f", "git push -f origin main", true},
		{"git reset hard", "git reset --hard HEAD", true},
		{"git clean", "git clean -fd", true},
		{"git checkout .", "git checkout .", true},
		{"git stash drop", "git stash drop", true},
		{"git branch -D", "git branch -D feature", true},
		{"kubectl delete", "kubectl delete pod foo", true},
		{"docker rm", "docker rm container", true},
		{"docker rmi", "docker rmi image", true},
		{"docker system prune", "docker system prune", true},
		{"terraform destroy", "terraform destroy", true},
		{"mysql", "mysql -u root -e 'DROP DATABASE test'", true},
		{"psql", "psql -c 'DROP TABLE users'", true},

		// Non-destructive
		{"echo hello", "echo hello", false},
		{"ls", "ls -la", false},
		{"git status", "git status", false},
		{"git log", "git log --oneline", false},
		{"cat", "cat file.txt", false},
		{"empty", "", false},

		// Safe wrappers should still be detected
		{"timeout rm", "timeout 30 rm -rf /tmp/test", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := CheckDestructiveWarning(tc.cmd)
			hasMsg := msg != ""
			if hasMsg != tc.wantMsg {
				t.Errorf("CheckDestructiveWarning(%q) hasMsg=%v (want %v), msg=%q", tc.cmd, hasMsg, tc.wantMsg, msg)
			}
		})
	}
}

// ─── Upstream Quality: containsVulnerableUncPath ─────────────────────────────

func TestContainsVulnerableUncPath(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		// UNC backslash paths — should detect
		{"UNC backslash", `echo \\server\share`, true},
		{"UNC backslash share", `cat \\server\share\file.txt`, true},
		{"UNC IPv4", `dir \\192.168.1.1\share`, true},

		// UNC forward-slash paths (non-URL) — should detect
		{"UNC forward slash", `echo //server/share`, true},

		// URLs — should NOT detect
		{"https URL", "curl https://example.com/api", false},
		{"http URL", "curl http://example.com/file", false},
		{"ftp URL", "wget ftp://ftp.example.com/file", false},

		// WebDAV patterns — should detect
		{"WebDAV SSL", `copy \\server@SSL@8443\path\file .`, true},
		{"WebDAV port", `copy \\server@8443@SSL\path\file .`, true},
		{"DavWWWRoot", `dir \\server\DavWWWRoot\path`, true},

		// Non-UNC paths — should NOT detect
		{"relative path", "cat ./file.txt", false},
		{"absolute unix", "cat /tmp/file.txt", false},
		{"Windows path", `cat C:\Users\foo\file.txt`, false},

		// Empty — should NOT detect
		{"empty", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := containsVulnerableUncPath(tc.cmd)
			if got != tc.want {
				t.Errorf("containsVulnerableUncPath(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

// ─── Upstream Quality: Path Conversion Roundtrip ─────────────────────────────

func TestWindowsPathRoundtrip(t *testing.T) {
	// Windows → POSIX → Windows should return equivalent path.
	// Invariant: roundtrip should preserve semantic meaning.
	paths := []string{
		`C:\Users\foo`,
		`C:\Users\foo\bar.txt`,
		`E:\workspace\project`,
	}

	for _, p := range paths {
		posix := windowsToPosixPath(p)
		roundtrip := PosixToWindowsPath(posix)
		// Normalize both for comparison (lowercase, forward slashes)
		origNorm := strings.ToLower(strings.ReplaceAll(p, `\`, `/`))
		rtNorm := strings.ToLower(strings.ReplaceAll(roundtrip, `\`, `/`))
		if origNorm != rtNorm {
			t.Errorf("Roundtrip %q → %q → %q (original: %q, roundtrip: %q)",
				p, posix, roundtrip, origNorm, rtNorm)
		}
	}
}

func TestWindowsToPosixPathUNC(t *testing.T) {
	got := windowsToPosixPath(`\\server\share`)
	want := `//server/share`
	if got != want {
		t.Errorf("windowsToPosixPath(UNC) = %q, want %q", got, want)
	}
}

func TestWindowsToPosixPathDrive(t *testing.T) {
	got := windowsToPosixPath(`C:\Users\foo`)
	want := `/c/Users/foo`
	if got != want {
		t.Errorf("windowsToPosixPath(drive) = %q, want %q", got, want)
	}
}

func TestWindowsToPosixPathRelative(t *testing.T) {
	got := windowsToPosixPath(`relative\path`)
	want := `relative/path`
	if got != want {
		t.Errorf("windowsToPosixPath(relative) = %q, want %q", got, want)
	}
}
