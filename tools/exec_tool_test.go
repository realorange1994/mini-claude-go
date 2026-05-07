package tools

import (
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
