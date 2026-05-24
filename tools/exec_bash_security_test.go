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
