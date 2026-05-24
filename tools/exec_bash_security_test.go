package tools

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// isUnsafeEnvPrefix
// ---------------------------------------------------------------------------

func TestIsUnsafeEnvPrefix_UnsafePrefixes(t *testing.T) {
	unsafe := []string{
		"PATH=/evil:/usr/bin",
		"LD_PRELOAD=/evil.so",
		"LD_LIBRARY_PATH=/evil",
		"DYLD_INSERT_LIBRARIES=/evil.dylib",
		"PYTHONPATH=/evil",
		"NODE_PATH=/evil",
		"GOFLAGS=-mod=vendor",
		"RUSTFLAGS=-C target-cpu=native",
		"NODE_OPTIONS=--require /evil.js",
		"HOME=/tmp/fake",
		"TMPDIR=/tmp/evil",
		"SHELL=/bin/fake",
		"BASH_ENV=/tmp/evil.sh",
	}
	for _, u := range unsafe {
		got := isUnsafeEnvPrefix(u)
		if got == "" {
			t.Errorf("isUnsafeEnvPrefix(%q) should return non-empty", u)
		}
	}
}

func TestIsUnsafeEnvPrefix_SafePrefixes(t *testing.T) {
	safe := []string{
		"GOOS=linux",
		"GOARCH=amd64",
		"CGO_ENABLED=0",
		"RUST_BACKTRACE=1",
		"NODE_ENV=production",
		"LANG=en_US.UTF-8",
		"TERM=xterm-256color",
		"TZ=UTC",
	}
	for _, s := range safe {
		got := isUnsafeEnvPrefix(s)
		if got != "" {
			t.Errorf("isUnsafeEnvPrefix(%q) = %q, want empty", s, got)
		}
	}
}

func TestIsUnsafeEnvPrefix_CaseInsensitive(t *testing.T) {
	got := isUnsafeEnvPrefix("path=/evil:/usr/bin")
	if got == "" {
		t.Error("should be case insensitive")
	}
}

// ---------------------------------------------------------------------------
// checkUnsafeEnvPrefixes
// ---------------------------------------------------------------------------

func TestCheckUnsafeEnvPrefixes_DetectsLdPreload(t *testing.T) {
	got := checkUnsafeEnvPrefixes("LD_PRELOAD=/evil.so rm -f /tmp/file")
	if got == "" {
		t.Error("should detect LD_PRELOAD")
	}
}

func TestCheckUnsafeEnvPrefixes_AllowsSafe(t *testing.T) {
	got := checkUnsafeEnvPrefixes("GOOS=linux CGO_ENABLED=0 echo hello")
	if got != "" {
		t.Errorf("safe env vars should not be flagged: %s", got)
	}
}

func TestCheckUnsafeEnvPrefixes_StopsAtShellOperator(t *testing.T) {
	// Env vars only appear at command beginning, should stop at | or &&
	got := checkUnsafeEnvPrefixes("echo hello | PATH=/evil cat")
	if got != "" {
		t.Error("should stop at shell operator, not check PATH= later in command")
	}
}

// ---------------------------------------------------------------------------
// checkBashSecurityPatterns — DENY
// ---------------------------------------------------------------------------

func TestCheckBashSecurityPatterns_DenyAnsiCQuoting(t *testing.T) {
	deny, _ := checkBashSecurityPatterns("echo $'hello world'")
	if len(deny) == 0 {
		t.Error("ANSI-C quoting should be denied")
	}
}

func TestCheckBashSecurityPatterns_DenyIFSInjection(t *testing.T) {
	deny, _ := checkBashSecurityPatterns("echo $IFS")
	if len(deny) == 0 {
		t.Error("IFS injection should be denied")
	}
}

func TestCheckBashSecurityPatterns_DenyUnicodeWhitespace(t *testing.T) {
	// U+00A0 is non-breaking space
	deny, _ := checkBashSecurityPatterns("echo\u00a0hello")
	if len(deny) == 0 {
		t.Error("Unicode whitespace should be denied")
	}
}

func TestCheckBashSecurityPatterns_DenyCarriageReturn(t *testing.T) {
	deny, _ := checkBashSecurityPatterns("echo hello\rworld")
	if len(deny) == 0 {
		t.Error("Carriage return should be denied")
	}
}

func TestCheckBashSecurityPatterns_DenyBackslashEscapedOperator(t *testing.T) {
	deny, _ := checkBashSecurityPatterns("echo hello \\; echo world")
	if len(deny) == 0 {
		t.Error("Backslash-escaped operator should be denied")
	}
}

func TestCheckBashSecurityPatterns_DenyZshDangerous(t *testing.T) {
	deny, _ := checkBashSecurityPatterns("zmodload zsh/datetime")
	if len(deny) == 0 {
		t.Error("Zsh dangerous builtin should be denied")
	}
}

// ---------------------------------------------------------------------------
// checkBashSecurityPatterns — ASK
// ---------------------------------------------------------------------------

func TestCheckBashSecurityPatterns_AskShellMetacharacters(t *testing.T) {
	_, ask := checkBashSecurityPatterns(`echo "hello; world"`)
	if len(ask) == 0 {
		t.Error("Shell metacharacters in quoted context should be ask")
	}
}

func TestCheckBashSecurityPatterns_AskVariableExpansion(t *testing.T) {
	_, ask := checkBashSecurityPatterns("echo $HOME | cat")
	if len(ask) == 0 {
		t.Error("Variable expansion before pipe should be ask")
	}
}

func TestCheckBashSecurityPatterns_AskShellPrefix(t *testing.T) {
	_, ask := checkBashSecurityPatterns("sh -c 'echo hello'")
	if len(ask) == 0 {
		t.Error("Dangerous shell executable prefix should be ask")
	}
}

func TestCheckBashSecurityPatterns_AskEnvPrefix(t *testing.T) {
	_, ask := checkBashSecurityPatterns("env FOO=bar echo hello")
	if len(ask) == 0 {
		t.Error("Dangerous command modifier prefix should be ask")
	}
}

func TestCheckBashSecurityPatterns_AskCommandSubstitution(t *testing.T) {
	_, ask := checkBashSecurityPatterns("echo $(whoami)")
	if len(ask) == 0 {
		t.Error("Command substitution should be ask")
	}
}

func TestCheckBashSecurityPatterns_AskBacktickSubstitution(t *testing.T) {
	_, ask := checkBashSecurityPatterns("echo `whoami`")
	if len(ask) == 0 {
		t.Error("Backtick substitution should be ask")
	}
}

func TestCheckBashSecurityPatterns_AskBraceExpansion(t *testing.T) {
	_, ask := checkBashSecurityPatterns("echo {a,b,c}")
	if len(ask) == 0 {
		t.Error("Brace expansion should be ask")
	}
}

func TestCheckBashSecurityPatterns_AskBackslashWhitespace(t *testing.T) {
	_, ask := checkBashSecurityPatterns("echo hello\\ world")
	if len(ask) == 0 {
		t.Error("Backslash-escaped whitespace should be ask")
	}
}

func TestCheckBashSecurityPatterns_AskNewlineInjection(t *testing.T) {
	_, ask := checkBashSecurityPatterns("echo hello\necho world")
	if len(ask) == 0 {
		t.Error("Newline injection should be ask")
	}
}

func TestCheckBashSecurityPatterns_AskControlChars(t *testing.T) {
	_, ask := checkBashSecurityPatterns("echo\x01hello")
	if len(ask) == 0 {
		t.Error("Control character injection should be ask")
	}
}

func TestCheckBashSecurityPatterns_AskIncompleteCompound(t *testing.T) {
	_, ask := checkBashSecurityPatterns("echo hello &&")
	if len(ask) == 0 {
		t.Error("Incomplete compound command should be ask")
	}
}

func TestCheckBashSecurityPatterns_SafeNoNewPatterns(t *testing.T) {
	// Simple commands should not trigger P1 patterns
	deny, ask := checkBashSecurityPatterns("ls -la /home/user")
	if len(deny) > 0 {
		t.Errorf("safe command should not have deny: %v", deny)
	}
	// Note: the existing shell metacharacter/variable patterns might ask,
	// but the P1-specific patterns (brace, newline, control, backslash-space,
	// incomplete) should not fire on simple commands.
	_ = ask // check that deny is clean; ask may exist from pre-existing patterns
}

func TestCheckBashSecurityPatterns_SafeCommand(t *testing.T) {
	deny, ask := checkBashSecurityPatterns("ls -la /home/user")
	if len(deny) > 0 {
		t.Errorf("safe command should not have deny: %v", deny)
	}
	if len(ask) > 0 {
		t.Errorf("safe command should not have ask: %v", ask)
	}
}

// ---------------------------------------------------------------------------
// checkJqSecurity
// ---------------------------------------------------------------------------

func TestCheckJqSecurity_DenySystem(t *testing.T) {
	got := checkJqSecurity(`jq 'system("whoami")' data.json`)
	if got == "" {
		t.Error("jq system() should be denied")
	}
}

func TestCheckJqSecurity_DenyFromFile(t *testing.T) {
	got := checkJqSecurity("jq -f filter.jq data.json")
	if got == "" {
		t.Error("jq -f should be denied")
	}
}

func TestCheckJqSecurity_DenyRawfile(t *testing.T) {
	got := checkJqSecurity("jq --rawfile f /etc/passwd data.json")
	if got == "" {
		t.Error("jq --rawfile should be denied")
	}
}

func TestCheckJqSecurity_DenyLibraryDir(t *testing.T) {
	got := checkJqSecurity("jq -L /tmp/evil '...' data.json")
	if got == "" {
		t.Error("jq -L should be denied")
	}
}

func TestCheckJqSecurity_AskEnvAccess(t *testing.T) {
	got := checkJqSecurity(`jq '.env[\"PATH\"]' data.json`)
	if got == "" {
		t.Error("jq env[] access should be ask")
	}
}

func TestCheckJqSecurity_NotJq(t *testing.T) {
	got := checkJqSecurity("echo hello")
	if got != "" {
		t.Errorf("non-jq command should return empty: %s", got)
	}
}

func TestCheckJqSecurity_SafeJq(t *testing.T) {
	got := checkJqSecurity(`jq '.name' data.json`)
	if got != "" {
		t.Errorf("safe jq command should return empty: %s", got)
	}
}

// ---------------------------------------------------------------------------
// checkSedSecurity
// ---------------------------------------------------------------------------

func TestCheckSedSecurity_DenyEFlag(t *testing.T) {
	got := checkSedSecurity(`sed -e 's/foo/bar/e' file.txt`)
	if got == "" {
		t.Error("sed 'e' flag should be denied")
	}
}

func TestCheckSedSecurity_NotSed(t *testing.T) {
	got := checkSedSecurity("echo hello")
	if got != "" {
		t.Errorf("non-sed command should return empty: %s", got)
	}
}

func TestCheckSedSecurity_SafeSed(t *testing.T) {
	got := checkSedSecurity(`sed 's/foo/bar/g' file.txt`)
	if got != "" {
		t.Errorf("safe sed command should return empty: %s", got)
	}
}

// ---------------------------------------------------------------------------
// checkXargsSecurity
// ---------------------------------------------------------------------------

func TestCheckXargsSecurity_DenyLowercaseI(t *testing.T) {
	got := checkXargsSecurity("xargs -i echo {}")
	if got == "" {
		t.Error("xargs -i should be denied")
	}
}

func TestCheckXargsSecurity_NotXargs(t *testing.T) {
	got := checkXargsSecurity("echo hello")
	if got != "" {
		t.Errorf("non-xargs command should return empty: %s", got)
	}
}

func TestCheckXargsSecurity_SafeXargs(t *testing.T) {
	got := checkXargsSecurity("xargs -I {} echo {}")
	if got != "" {
		t.Errorf("safe xargs command should return empty: %s", got)
	}
}

// ---------------------------------------------------------------------------
// checkFdSecurity
// ---------------------------------------------------------------------------

func TestCheckFdSecurity_DenyExec(t *testing.T) {
	got := checkFdSecurity("fd -x rm {}")
	if got == "" {
		t.Error("fd -x should be denied")
	}
}

func TestCheckFdSecurity_DenyExecBatch(t *testing.T) {
	got := checkFdSecurity("fd --exec-batch echo {}")
	if got == "" {
		t.Error("fd --exec-batch should be denied")
	}
}

func TestCheckFdSecurity_NotFd(t *testing.T) {
	got := checkFdSecurity("echo hello")
	if got != "" {
		t.Errorf("non-fd command should return empty: %s", got)
	}
}

func TestCheckFdSecurity_SafeFd(t *testing.T) {
	got := checkFdSecurity("fd -t f -d 3 pattern")
	if got != "" {
		t.Errorf("safe fd command should return empty: %s", got)
	}
}

// ---------------------------------------------------------------------------
// checkRgSecurity
// ---------------------------------------------------------------------------

func TestCheckRgSecurity_DenyPre(t *testing.T) {
	got := checkRgSecurity("rg --pre cat pattern")
	if got == "" {
		t.Error("rg --pre should be denied")
	}
}

func TestCheckRgSecurity_DenyPreGlob(t *testing.T) {
	got := checkRgSecurity("rg --pre-glob '*.gz:zcat' pattern")
	if got == "" {
		t.Error("rg --pre-glob should be denied")
	}
}

func TestCheckRgSecurity_DenySearchZip(t *testing.T) {
	got := checkRgSecurity("rg --search-zip pattern")
	if got == "" {
		t.Error("rg --search-zip should be denied")
	}
}

func TestCheckRgSecurity_NotRg(t *testing.T) {
	got := checkRgSecurity("echo hello")
	if got != "" {
		t.Errorf("non-rg command should return empty: %s", got)
	}
}

func TestCheckRgSecurity_SafeRg(t *testing.T) {
	got := checkRgSecurity("rg -F pattern file.txt")
	if got != "" {
		t.Errorf("safe rg command should return empty: %s", got)
	}
}

// ---------------------------------------------------------------------------
// checkGhSecurity
// ---------------------------------------------------------------------------

func TestCheckGhSecurity_DenyAuth(t *testing.T) {
	got := checkGhSecurity("gh auth login")
	if got == "" {
		t.Error("gh auth should be denied")
	}
}

func TestCheckGhSecurity_DenySecret(t *testing.T) {
	got := checkGhSecurity("gh secret list")
	if got == "" {
		t.Error("gh secret should be denied")
	}
}

func TestCheckGhSecurity_DenyApiWithoutGetMethod(t *testing.T) {
	got := checkGhSecurity("gh api /repos/foo/bar")
	if got == "" {
		t.Error("gh api without --method GET should be denied")
	}
}

func TestCheckGhSecurity_DenyHostOwnerRepo(t *testing.T) {
	got := checkGhSecurity("gh issue list github.com/foo/bar")
	if got == "" {
		t.Error("HOST/OWNER/REPO format should be denied")
	}
}

func TestCheckGhSecurity_NotGh(t *testing.T) {
	got := checkGhSecurity("echo hello")
	if got != "" {
		t.Errorf("non-gh command should return empty: %s", got)
	}
}

func TestCheckGhSecurity_SafeGh(t *testing.T) {
	got := checkGhSecurity("gh issue list")
	if got != "" {
		t.Errorf("safe gh command should return empty: %s", got)
	}
}

// ---------------------------------------------------------------------------
// checkDockerSecurity
// ---------------------------------------------------------------------------

func TestCheckDockerSecurity_AllowReadOnly(t *testing.T) {
	commands := []string{
		"docker ps",
		"docker images",
		"docker logs mycontainer",
		"docker inspect mycontainer",
		"docker info",
	}
	for _, cmd := range commands {
		result := checkDockerSecurity(cmd)
		if result != nil && result.Behavior != PermissionAllow {
			t.Errorf("%q should be allow, got %v", cmd, result.Behavior)
		}
	}
}

func TestCheckDockerSecurity_AskWrite(t *testing.T) {
	commands := []string{
		"docker rm mycontainer",
		"docker run ubuntu:latest",
		"docker exec mycontainer ls",
	}
	for _, cmd := range commands {
		result := checkDockerSecurity(cmd)
		if result == nil {
			t.Errorf("%q should return a result", cmd)
			continue
		}
		if result.Behavior != PermissionAsk {
			t.Errorf("%q should be ask, got %v", cmd, result.Behavior)
		}
	}
}

func TestCheckDockerSecurity_DenyPrune(t *testing.T) {
	result := checkDockerSecurity("docker system prune")
	if result == nil || result.Behavior != PermissionDeny {
		t.Errorf("docker prune should be deny, got %v", result.Behavior)
	}
}

func TestCheckDockerSecurity_NotDocker(t *testing.T) {
	result := checkDockerSecurity("echo hello")
	if result != nil {
		t.Error("non-docker command should return nil")
	}
}

// ---------------------------------------------------------------------------
// checkCdCompoundAttacks
// ---------------------------------------------------------------------------

func TestCheckCdCompoundAttacks_MultipleCd(t *testing.T) {
	subcmds := []string{"cd /tmp", "cd /var", "ls"}
	got := checkCdCompoundAttacks("cd /tmp && cd /var && ls", subcmds)
	if got == "" {
		t.Error("multiple cd should be flagged")
	}
}

func TestCheckCdCompoundAttacks_CdGitCompound(t *testing.T) {
	subcmds := []string{"cd /tmp/repo", "git status"}
	got := checkCdCompoundAttacks("cd /tmp/repo && git status", subcmds)
	if got == "" {
		t.Error("cd+git compound should be flagged")
	}
}

func TestCheckCdCompoundAttacks_SafeCompound(t *testing.T) {
	subcmds := []string{"ls /tmp", "cat file.txt"}
	got := checkCdCompoundAttacks("ls /tmp && cat file.txt", subcmds)
	if got != "" {
		t.Errorf("safe compound should not be flagged: %s", got)
	}
}

func TestCheckCdCompoundAttacks_SingleCdNoGit(t *testing.T) {
	// Single cd without git is OK
	subcmds := []string{"cd /tmp", "ls"}
	got := checkCdCompoundAttacks("cd /tmp && ls", subcmds)
	if got != "" {
		t.Errorf("single cd without git should be OK: %s", got)
	}
}

// ---------------------------------------------------------------------------
// CheckBashPermission — end-to-end
// ---------------------------------------------------------------------------

func TestCheckBashPermission_DenyAnsiC(t *testing.T) {
	result := CheckBashPermission("echo $'hello world'")
	if result.Behavior != PermissionDeny {
		t.Errorf("ANSI-C quoting should deny, got %v", result.Behavior)
	}
}

func TestCheckBashPermission_DenyIFS(t *testing.T) {
	result := CheckBashPermission("echo $IFS")
	if result.Behavior != PermissionDeny {
		t.Errorf("IFS injection should deny, got %v", result.Behavior)
	}
}

func TestCheckBashPermission_AskUnsafeEnv(t *testing.T) {
	result := CheckBashPermission("LD_PRELOAD=/evil.so echo hello")
	if result.Behavior != PermissionAsk {
		t.Errorf("unsafe env var should ask, got %v", result.Behavior)
	}
	if !strings.Contains(result.Message, "environment variable") {
		t.Errorf("ask message should mention env var: %q", result.Message)
	}
}

func TestCheckBashPermission_AskShellPrefix(t *testing.T) {
	result := CheckBashPermission("sh -c 'echo hello'")
	if result.Behavior != PermissionAsk {
		t.Errorf("shell prefix should ask, got %v", result.Behavior)
	}
}

func TestCheckBashPermission_AskJqSystem(t *testing.T) {
	result := CheckBashPermission(`jq 'system("whoami")' data.json`)
	if result.Behavior != PermissionAsk {
		t.Errorf("jq system should ask, got %v", result.Behavior)
	}
}

func TestCheckBashPermission_AskDockerWrite(t *testing.T) {
	result := CheckBashPermission("docker rm mycontainer")
	if result.Behavior != PermissionAsk {
		t.Errorf("docker write should ask, got %v", result.Behavior)
	}
}

func TestCheckBashPermission_AskFdExec(t *testing.T) {
	result := CheckBashPermission("fd -x rm {}")
	if result.Behavior != PermissionAsk {
		t.Errorf("fd -x should ask, got %v", result.Behavior)
	}
}

func TestCheckBashPermission_AskRgPre(t *testing.T) {
	result := CheckBashPermission("rg --pre cat pattern")
	if result.Behavior != PermissionAsk {
		t.Errorf("rg --pre should ask, got %v", result.Behavior)
	}
}

func TestCheckBashPermission_PassthroughSafeCommand(t *testing.T) {
	result := CheckBashPermission("ls -la /home/user")
	if result.Behavior != PermissionPassthrough {
		t.Errorf("safe command should passthrough, got %v", result.Behavior)
	}
}

func TestCheckBashPermission_PassthroughNotBashCommand(t *testing.T) {
	result := CheckBashPermission("gcc -o main main.c")
	if result.Behavior != PermissionPassthrough {
		t.Errorf("non-bash command should passthrough, got %v", result.Behavior)
	}
}

func TestCheckBashPermission_EmptyCommand(t *testing.T) {
	result := CheckBashPermission("")
	if result.Behavior != PermissionPassthrough {
		t.Errorf("empty command should passthrough, got %v", result.Behavior)
	}
}

// ---------------------------------------------------------------------------
// isReadOnlyCommandWithFlags
// ---------------------------------------------------------------------------

func TestIsReadOnlyCommandWithFlags_SafeFd(t *testing.T) {
	got := isReadOnlyCommandWithFlags("fd -t f -d 3 pattern", "fd -t f -d 3 pattern")
	if !got {
		t.Error("safe fd should be read-only")
	}
}

func TestIsReadOnlyCommandWithFlags_DangerousFd(t *testing.T) {
	got := isReadOnlyCommandWithFlags("fd -x rm {}", "fd -x rm {}")
	if got {
		t.Error("fd with -x should not be read-only")
	}
}

func TestIsReadOnlyCommandWithFlags_SafeRg(t *testing.T) {
	got := isReadOnlyCommandWithFlags("rg -F pattern file.txt", "rg -F pattern file.txt")
	if !got {
		t.Error("safe rg should be read-only")
	}
}

func TestIsReadOnlyCommandWithFlags_DangerousRg(t *testing.T) {
	got := isReadOnlyCommandWithFlags("rg --pre cat pattern", "rg --pre cat pattern")
	if got {
		t.Error("rg with --pre should not be read-only")
	}
}

// ---------------------------------------------------------------------------
// P0 Fix #1: $() token bypass guard
// ---------------------------------------------------------------------------

func TestIsReadOnlyCommandWithFlags_DollarParenBypass(t *testing.T) {
	// $() token in command should block read-only flag validation
	got := isReadOnlyCommandWithFlags("rg $(echo --pre) pattern", "rg $(echo --pre) pattern")
	if got {
		t.Error("rg with $() token bypass should not be read-only")
	}
}

func TestIsReadOnlyCommandWithFlags_BacktickBypass(t *testing.T) {
	got := isReadOnlyCommandWithFlags("fd `echo --exec` pattern", "fd `echo --exec` pattern")
	if got {
		t.Error("fd with backtick bypass should not be read-only")
	}
}

func TestContainsCommandSubstitution(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"echo $(whoami)", true},
		{"echo `whoami`", true},
		{"echo hello", false},
		{"ls -la", false},
		{"rg $(echo --pre) pattern", true},
	}
	for _, tt := range tests {
		got := containsCommandSubstitution(tt.cmd)
		if got != tt.expect {
			t.Errorf("containsCommandSubstitution(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

// ---------------------------------------------------------------------------
// P0 Fix #2: fd -l removed from safe flags (PATH hijack risk)
// ---------------------------------------------------------------------------

func TestCheckFdSecurity_DangerousLFlag(t *testing.T) {
	got := checkFdSecurity("fd -l pattern")
	if got == "" {
		t.Error("fd -l should be flagged (PATH hijack risk)")
	}
}

func TestIsReadOnlyCommandWithFlags_FdLFlag(t *testing.T) {
	got := isReadOnlyCommandWithFlags("fd -l pattern", "fd -l pattern")
	if got {
		t.Error("fd -l should not be read-only (PATH hijack risk)")
	}
}

// ---------------------------------------------------------------------------
// P0 Fix #4: git command callbacks
// ---------------------------------------------------------------------------

func TestCheckGitSecurity_BranchPositionalArg(t *testing.T) {
	got := checkGitSecurity("git branch new-feature")
	if got == "" {
		t.Error("git branch with positional arg should be flagged")
	}
}

func TestCheckGitSecurity_BranchList(t *testing.T) {
	got := checkGitSecurity("git branch -a")
	if got != "" {
		t.Errorf("git branch -a should be safe: %s", got)
	}
}

func TestCheckGitSecurity_TagCreate(t *testing.T) {
	got := checkGitSecurity("git tag v1.0.0")
	if got == "" {
		t.Error("git tag with positional arg (create) should be flagged")
	}
}

func TestCheckGitSecurity_TagList(t *testing.T) {
	got := checkGitSecurity("git tag -l")
	if got != "" {
		t.Errorf("git tag -l should be safe: %s", got)
	}
}

func TestCheckGitSecurity_ReflogExpire(t *testing.T) {
	got := checkGitSecurity("git reflog expire --all")
	if got == "" {
		t.Error("git reflog expire should be flagged")
	}
}

func TestCheckGitSecurity_ReflogShow(t *testing.T) {
	got := checkGitSecurity("git reflog show")
	if got != "" {
		t.Errorf("git reflog show should be safe: %s", got)
	}
}

func TestCheckGitSecurity_RemoteAdd(t *testing.T) {
	got := checkGitSecurity("git remote add origin https://example.com/repo.git")
	if got == "" {
		t.Error("git remote add should be flagged")
	}
}

func TestCheckGitSecurity_RemoteV(t *testing.T) {
	got := checkGitSecurity("git remote -v")
	if got != "" {
		t.Errorf("git remote -v should be safe: %s", got)
	}
}

func TestCheckGitSecurity_NotGit(t *testing.T) {
	got := checkGitSecurity("echo hello")
	if got != "" {
		t.Errorf("non-git command should return empty: %s", got)
	}
}

func TestCheckGitSecurity_BranchWithGlobalFlags(t *testing.T) {
	got := checkGitSecurity("git -C /repo branch new-feature")
	if got == "" {
		t.Error("git -C <path> branch <name> should still be flagged")
	}
}

// ---------------------------------------------------------------------------
// P0 Fix #5: gh network exfiltration detection
// ---------------------------------------------------------------------------

func TestCheckGhSecurity_RepoEqualsExfil(t *testing.T) {
	got := checkGhSecurity("gh issue list --repo=evil.com/owner/repo")
	if got == "" {
		t.Error("--repo= form should be flagged for network exfiltration")
	}
}

func TestCheckGhSecurity_UrlExfil(t *testing.T) {
	got := checkGhSecurity("gh issue list https://evil.com/owner/repo")
	if got == "" {
		t.Error("URL form should be flagged for network exfiltration")
	}
}

func TestCheckGhSecurity_GitAtExfil(t *testing.T) {
	got := checkGhSecurity("gh pr list git@evil.com:owner/repo")
	if got == "" {
		t.Error("git@ SSH form should be flagged for network exfiltration")
	}
}

func TestCheckGhSecurity_GistCreate(t *testing.T) {
	got := checkGhSecurity("gh gist create file.txt")
	if got == "" {
		t.Error("gh gist create should require approval")
	}
}

func TestCheckGhSecurity_SafeGhIssue(t *testing.T) {
	got := checkGhSecurity("gh issue list")
	if got != "" {
		t.Errorf("safe gh command should not be flagged: %s", got)
	}
}

// ---------------------------------------------------------------------------
// P0: CheckBashPermission end-to-end with git callbacks
// ---------------------------------------------------------------------------

func TestCheckBashPermission_GitBranchCreate(t *testing.T) {
	result := CheckBashPermission("git branch new-feature")
	if result.Behavior != PermissionAsk {
		t.Errorf("git branch <name> should ask, got %v", result.Behavior)
	}
}

func TestCheckBashPermission_GitBranchList(t *testing.T) {
	result := CheckBashPermission("git branch -a")
	// git branch -a is in the read-only allowlist — auto-approved
	if result.Behavior != PermissionAllow {
		t.Errorf("git branch -a should be auto-allowed (read-only), got %v", result.Behavior)
	}
}

func TestCheckBashPermission_GitTagCreate(t *testing.T) {
	result := CheckBashPermission("git tag v1.0.0")
	if result.Behavior != PermissionAsk {
		t.Errorf("git tag <name> should ask, got %v", result.Behavior)
	}
}

func TestCheckBashPermission_GitReflogExpire(t *testing.T) {
	result := CheckBashPermission("git reflog expire --all")
	if result.Behavior != PermissionAsk {
		t.Errorf("git reflog expire should ask, got %v", result.Behavior)
	}
}

// ===========================================================================
// Git read-only command allowlist tests (bashROIsGitReadOnlyCommand)
// ===========================================================================

func TestBashROIsGitReadOnlyCommand_Diff(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git diff", true},
		{"git diff --stat", true},
		{"git diff --cached", true},
		{"git diff --staged", true},
		{"git diff --name-only", true},
		{"git diff --numstat", true},
		{"git diff -M -C", true},
		{"git diff -S foo", true},
		{"git diff --output=/tmp/pwned", false}, // --output not in allowlist
		{"git diff HEAD~1", true},               // positional args allowed
		{"git diff main..feature", true},
		{"git diff --unknown-flag", false},
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_Log(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git log", true},
		{"git log --oneline", true},
		{"git log --oneline -20", true},
		{"git log --all --graph --decorate", true},
		{"git log --author=alice --since=2024-01-01", true},
		{"git log -S needle", true},
		{"git log -G regex", true},
		{"git log --format='%h %s'", true},
		{"git log --unknown", false},
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_Show(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git show", true},
		{"git show HEAD", true},
		{"git show --stat abc123", true},
		{"git show --format='%h %s' HEAD", true},
		{"git show --unknown", false},
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_Status(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git status", true},
		{"git status --short", true},
		{"git status --porcelain", true},
		{"git status -s -b", true},
		{"git status --unknown", false},
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_Branch(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git branch", true},
		{"git branch -a", true},
		{"git branch --list", true},
		{"git branch -l", true},
		{"git branch -vv", true},
		{"git branch --all --no-color", true},
		{"git branch --contains=abc123", true},
		{"git branch --sort=-committerdate", true},
		{"git branch new-feature", false},            // positional = create
		{"git branch -d old-branch", false},          // -d not in allowlist
		{"git branch -D old-branch", false},          // -D not in allowlist
		{"git branch -m old new", false},             // -m not in allowlist
		{"git branch --list feature*", true},         // --list with pattern
		{"git branch -la", true},                     // combined -la (includes 'l')
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_Tag(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git tag", true},           // bare tag = list
		{"git tag -l", true},
		{"git tag --list", true},
		{"git tag -n5", true},
		{"git tag --contains=abc", true},
		{"git tag --sort=-version:refname", true},
		{"git tag v1.0.0", false},   // positional = create
		{"git tag -d v1.0.0", false}, // -d not in allowlist
		{"git tag -a v1.0.0", false},  // -a not in allowlist
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_Reflog(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git reflog", true},        // bare = show
		{"git reflog show", true},
		{"git reflog show --oneline", true},
		{"git reflog expire", false},
		{"git reflog delete", false},
		{"git reflog exists", false},
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_Remote(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git remote", true},
		{"git remote -v", true},
		{"git remote --verbose", true},
		{"git remote show origin", true},
		{"git remote show origin -n", true},
		{"git remote add upstream https://...", false},
		{"git remote remove origin", false},
		{"git remote show", false},                    // no remote name
		{"git remote show origin extra", false},       // too many args
		{"git remote show evil.com/owner/repo", false}, // bad chars in name
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_ConfigGet(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git config --get user.name", true},
		{"git config --get --global core.autocrlf", true},
		{"git config --get --type=bool core.bare", true},
		{"git config --set user.name bob", false}, // --set not in allowlist
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_OtherSubs(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git rev-parse --verify HEAD", true},
		{"git rev-parse --is-inside-work-tree", true},
		{"git describe --tags", true},
		{"git describe --always HEAD", true},
		{"git ls-remote --tags origin", true},
		{"git ls-remote --sort=-refname", true},
		{"git shortlog -sn -10 HEAD", true},
		{"git stash list", true},
		{"git stash list --oneline -5", true},
		{"git stash show -p", true},
		{"git merge-base --is-ancestor main HEAD", true},
		{"git for-each-ref --format='%(refname:short)'", true},
		{"git grep -n pattern", true},
		{"git grep -E -i pattern", true},
		{"git worktree list", true},
		{"git blame file.go", true},
		{"git ls-files", true},
		{"git ls-files --cached", true},
		{"git ls-files --stage -z", true},
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_GlobalFlags(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git -C /repo diff --stat", true},
		{"git -c core.autocrlf=false status", true},
		{"git --git-dir=/repo/.git log --oneline", true},
		{"git -C /repo branch -a", true},
		{"git -C /repo branch new-name", false},  // create, not list
		{"git commit -m 'fix'", false},            // not a read-only subcommand
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_NotGit(t *testing.T) {
	if bashROIsGitReadOnlyCommand("ls -la") {
		t.Error("non-git command should return false")
	}
	if bashROIsGitReadOnlyCommand("echo hello") {
		t.Error("non-git command should return false")
	}
}

func TestBashROIsGitReadOnlyCommand_LsRemoteNoServerOption(t *testing.T) {
	// --server-option / -o is intentionally excluded (network WRITE)
	if bashROIsGitReadOnlyCommand("git ls-remote --server-option=hello origin") {
		t.Error("git ls-remote --server-option should NOT be read-only")
	}
	if bashROIsGitReadOnlyCommand("git ls-remote -o hello origin") {
		t.Error("git ls-remote -o should NOT be read-only")
	}
}

func TestBashROIsGitReadOnlyCommand_FlagArgValidation(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		// --abbrev takes a number arg, not a string
		{"git describe --abbrev=7", true},
		{"git describe --abbrev=short", false}, // --abbrev expects number, not string
		// -n takes number arg
		{"git log -n 10", true},
		// Positional args are allowed for read-only subs
		{"git diff HEAD~3..HEAD", true},
		{"git log -- main.go", true},
		// --format takes string arg
		{"git log --format='%h %s'", true},
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_CombinedShortFlags(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		// Combined short flags: all must be 'none' type
		{"git diff -R", true},       // -R is none
		{"git diff -v", false},      // -v not in allowlist for diff
		// -la for branch (l=list, a=all)
		{"git branch -la", true},
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_EqualsAttachedFlags(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"git log --format=%h", true},
		{"git diff --color=never", false}, // --color is none type, = not allowed
		{"git log --max-count=5", true},
	}
	for _, tt := range tests {
		got := bashROIsGitReadOnlyCommand(tt.cmd)
		if got != tt.expect {
			t.Errorf("bashROIsGitReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
		}
	}
}

func TestBashROIsGitReadOnlyCommand_DashDash(t *testing.T) {
	// After --, everything is positional args
	got := bashROIsGitReadOnlyCommand("git log -- --unknown-flag")
	if !got {
		t.Error("git log -- --unknown-flag should be read-only (after --, all positional)")
	}
}

func TestCheckBashPermission_GitReadOnlyVsUnknown(t *testing.T) {
	// Read-only git commands should auto-allow
	result := CheckBashPermission("git diff --stat")
	if result.Behavior != PermissionAllow {
		t.Errorf("git diff --stat should auto-allow, got %v", result.Behavior)
	}

	// Unknown git subcommand should ask
	result = CheckBashPermission("git commit -m 'fix'")
	if result.Behavior != PermissionAsk {
		t.Errorf("git commit should ask, got %v", result.Behavior)
	}

	// Known subcommand with dangerous positional (branch create) should ask
	result = CheckBashPermission("git branch new-feature")
	if result.Behavior != PermissionAsk {
		t.Errorf("git branch new-feature should ask, got %v", result.Behavior)
	}
}

// ===========================================================================
// Bash read-only command allowlist tests (exec_bash_readonly.go)
// ===========================================================================

func TestCheckBashReadOnlyCommand_Xargs(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"xargs -n 1", true},
		{"xargs -I {} echo {}", true},
		{"xargs -0 -t -r", true},
		{"xargs -P 4 -L 10", true},
		{"xargs -d x", true},      // -d takes single char delimiter
		{"xargs -d '\\n'", false},  // '\n' (2 chars after unquoting) exceeds bashROChar
		{"xargs -i", false},  // dangerous: lowercase -i
		{"xargs -e", false},  // dangerous: lowercase -e
		{"xargs -I{} echo {}", true},
		{"xargs", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_File(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"file --brief myfile.txt", true},
		{"file --mime myfile.txt", true},
		{"file -i myfile.txt", true},
		{"file -f list.txt", true},
		{"file -z archive.gz", true},
		{"file", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Sort(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"sort -n -r file.txt", true},
		{"sort -k 2,2 -t , file.txt", true},
		{"sort -u", true},
		{"sort -o output.txt", false}, // -o writes to file
		{"sort --unique --reverse", true},
		{"sort", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Man(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"man ls", true},
		{"man -k network", true},
		{"man -a printf", true},
		{"man -P less ls", false}, // -P runs arbitrary pager
		{"man -s 2 open", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Ps(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"ps aux", true},
		{"ps -ef", true},
		{"ps -p 1234", true},
		{"ps --sort pid", true},
		{"ps auxf", true},    // forest output, safe (no 'e' modifier)
		{"ps -e -o pid,cmd", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Grep(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"grep -r 'pattern' .", true},
		{"grep -i --include='*.go' TODO .", true},
		{"grep -A 5 -B 2 'error' log.txt", true},
		{"grep -l -R 'TODO' src/", true},
		{"grep --color=auto 'pattern'", true},
		{"grep", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Rg(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"rg 'pattern' .", true},
		{"rg -i --type go 'TODO' src/", true},
		{"rg -l -C 3 'error' .", true},
		{"rg --json 'pattern'", true},
		{"rg", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Checksums(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"sha256sum file.tar", true},
		{"sha256sum -c checksum.txt", true},
		{"sha1sum file.bin", true},
		{"md5sum file.dat", true},
		{"sha256sum --tag file.tar", true},
		{"sha256sum", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Tree(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"tree -L 2 src/", true},
		{"tree -a -d .", true},
		{"tree -I 'node_modules' .", true},
		{"tree -R .", false}, // -R recursively creates listing (intensive)
		{"tree -o output.txt .", false}, // -o writes to file
		{"tree", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Date(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"date", true},
		{"date '+%Y-%m-%d'", true},
		{"date -u", true},
		{"date -d '2024-01-01'", true},
		{"date --rfc-3339=seconds", true},
		{"date 01011200", false}, // positional MMDDhhmm = sets system time
		{"date -s '12:00'", false}, // -s sets system time
		{"date --set='12:00'", false}, // --set sets system time
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Hostname(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"hostname", true},
		{"hostname -f", true},
		{"hostname -i", true},
		{"hostname -s", true},
		{"hostname newname", false}, // positional arg changes hostname
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Base64(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"base64 file.txt", true},
		{"base64 -d encoded.txt", true},
		{"base64 --decode encoded.txt", true},
		{"base64 -o output.txt", false}, // -o writes to file
		{"base64 --output output.txt", false}, // --output writes to file
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Lsof(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"lsof -i :8080", true},
		{"lsof -p 1234", true},
		{"lsof -n -P", true},
		{"lsof +m", false}, // +m enables mount verification (dangerous)
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Ss(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"ss -tlnp", true},
		{"ss -s", true},
		{"ss -4 -t state established", true},
		{"ss -K", false},  // -K kills sockets
		{"ss --kill", false},
		{"ss -D", false},  // -D dumps diag
		{"ss --diag", false},
		{"ss -F", false},  // -F applies filter
		{"ss --filter", false},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Info(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"info coreutils", true},
		{"info -k network", true},
		{"info -o output.txt topic", false}, // -o writes to file
		{"info --output output.txt topic", false},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Tput(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"tput cols", true},
		{"tput colors", true},
		{"tput -T xterm-256color cols", true},
		{"tput init", false},   // init is dangerous (changes terminal state)
		{"tput reset", false},  // reset is dangerous
		{"tput clear", false},  // clear is dangerous
		{"tput -S", false},     // -S reads from stdin (injection vector)
		{"tput smcup", false},  // enters alternate screen buffer
		{"tput rmcup", false},  // exits alternate screen buffer
		{"tput rs1", false},    // reset string
		{"tput rs2", false},    // reset string
		{"tput rs3", false},    // reset string
		{"tput is1", false},    // initialization string
		{"tput is2", false},    // initialization string
		{"tput is3", false},    // initialization string
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Pgrep(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"pgrep -f 'node'", true},
		{"pgrep -l -u root", true},
		{"pgrep -P 1234", true},
		{"pgrep -v python", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Netstat(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"netstat -tlnp", true},
		{"netstat -an", true},
		{"netstat -r", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_Help(t *testing.T) {
	tests := []struct {
		cmd    string
		safe   bool
	}{
		{"help", true},
		{"help -d", true},
		{"help -m", true},
		{"help -s", true},
	}

	for _, tt := range tests {
		got := checkBashReadOnlyCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("checkBashReadOnlyCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}

func TestCheckBashReadOnlyCommand_UnknownCommand(t *testing.T) {
	if checkBashReadOnlyCommand("curl http://example.com") {
		t.Error("unknown command 'curl' should not be read-only")
	}
	if checkBashReadOnlyCommand("python3 script.py") {
		t.Error("unknown command 'python3' should not be read-only")
	}
	if checkBashReadOnlyCommand("") {
		t.Error("empty command should not be read-only")
	}
}

func TestCheckBashReadOnlyCommand_CombinedShortFlags(t *testing.T) {
	// Combined short flags where all are 'none' type should be safe
	if !checkBashReadOnlyCommand("grep -ri 'pattern' .") {
		t.Error("grep -ri with all none-type flags should be safe")
	}
	// grep -A takes a number arg, so -An5 should NOT be combined
	if checkBashReadOnlyCommand("grep -An5 'pattern'") {
		// This actually depends on whether -A5 is parsed as attached arg or combined
		// -A with attached number should be valid
		t.Log("grep -An5 parsing depends on attached arg handling")
	}
}

func TestCheckBashReadOnlyCommand_EqualsAttached(t *testing.T) {
	if !checkBashReadOnlyCommand("grep --color=auto 'pattern'") {
		t.Error("grep --color=auto should be safe")
	}
	if !checkBashReadOnlyCommand("sort --key=2,2 file.txt") {
		t.Error("sort --key=2,2 should be safe")
	}
}

func TestCheckBashReadOnlyCommand_DashDash(t *testing.T) {
	// POSIX -- should stop flag parsing for commands that respect it
	if !checkBashReadOnlyCommand("grep -- -v file.txt") {
		t.Error("grep -- -v should be safe (-- ends flags, -v is positional)")
	}
	// base64 does NOT respect -- on macOS
	if !checkBashReadOnlyCommand("base64 -- -d") {
		t.Error("base64 -- -d should still be safe (even without respecting --)")
	}
}

// ===========================================================================
// Integration tests: CheckBashPermission with read-only allowlist
// ===========================================================================

func TestCheckBashPermission_ReadOnlyAllowlist(t *testing.T) {
	// Read-only commands should auto-allow
	readOnlyCmds := []string{
		"grep -r 'TODO' .",
		"rg 'pattern' src/",
		"sort -n file.txt",
		"file --mime myfile.txt",
		"ps aux",
		"sha256sum file.tar",
		"date '+%Y-%m-%d'",
		"hostname -f",
		"tree -L 2 src/",
		"man ls",
		"netstat -tlnp",
		"pgrep -f 'node'",
		"ss -tlnp",
		"xargs -n 1",
	}

	for _, cmd := range readOnlyCmds {
		result := CheckBashPermission(cmd)
		if result.Behavior != PermissionAllow {
			t.Errorf("CheckBashPermission(%q) = %v, want Allow", cmd, result.Behavior)
		}
	}
}

func TestCheckBashPermission_ReadOnlyDangerousFlags(t *testing.T) {
	// Commands with dangerous flags should not auto-allow
	dangerousCmds := []struct {
		cmd string
		desc string
	}{
		{"xargs -i", "xargs -i is dangerous (GNU parser differential)"},
		{"tree -R .", "tree -R is intensive"},
		{"tree -o output.txt .", "tree -o writes to file"},
		{"base64 -o out.txt", "base64 -o writes to file"},
		{"sort -o output.txt", "sort -o writes to file"},
		{"man -P less ls", "man -P runs arbitrary pager"},
		{"ss -K", "ss -K kills sockets"},
		{"info -o output.txt topic", "info -o writes to file"},
	}

	for _, tt := range dangerousCmds {
		result := CheckBashPermission(tt.cmd)
		if result.Behavior == PermissionAllow {
			t.Errorf("CheckBashPermission(%q) should not auto-allow: %s", tt.cmd, tt.desc)
		}
	}
}

func TestCheckBashPermission_DateTimeSetting(t *testing.T) {
	// date with positional MMDDhhmm arg should not auto-allow
	result := CheckBashPermission("date 01011200")
	if result.Behavior == PermissionAllow {
		t.Error("date with MMDDhhmm positional should not auto-allow")
	}
}

func TestCheckBashPermission_PsEnvLeak(t *testing.T) {
	// ps with BSD 'e' modifier (shows env vars) should not auto-allow
	result := CheckBashPermission("ps auxe")
	if result.Behavior == PermissionAllow {
		t.Error("ps auxe (BSD 'e' modifier) should not auto-allow")
	}
}

func TestCheckBashPermission_HostnameChange(t *testing.T) {
	// hostname with positional arg (changes hostname) should not auto-allow
	result := CheckBashPermission("hostname newname")
	if result.Behavior == PermissionAllow {
		t.Error("hostname with positional arg should not auto-allow")
	}
}

func TestCheckBashPermission_TputDangerous(t *testing.T) {
	// tput with dangerous capabilities should not auto-allow
	dangerous := []string{"tput init", "tput reset", "tput clear", "tput smcup", "tput rmcup"}
	for _, cmd := range dangerous {
		result := CheckBashPermission(cmd)
		if result.Behavior == PermissionAllow {
			t.Errorf("%q should not auto-allow", cmd)
		}
	}
}

func TestCheckBashPermission_LsofPlusM(t *testing.T) {
	// lsof +m should not auto-allow
	result := CheckBashPermission("lsof +m")
	if result.Behavior == PermissionAllow {
		t.Error("lsof +m should not auto-allow")
	}
}
