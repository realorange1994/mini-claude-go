package permissions

import (
	"strings"
	"testing"
)

// ─── IsDangerousAllowRule ───────────────────────────────────────────────────

func TestIsDangerousRuleNonAllow(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "python", Behavior: "deny"}
	if IsDangerousAllowRule(rule) {
		t.Error("deny rule should not be dangerous allow")
	}
}

func TestIsDangerousRuleNonBash(t *testing.T) {
	rule := &ParsedRule{ToolName: "Read", Content: "python", Behavior: "allow"}
	if IsDangerousAllowRule(rule) {
		t.Error("non-Bash tool should not be dangerous")
	}
}

func TestIsDangerousRuleToolLevelBash(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("tool-level Bash allow should be dangerous")
	}
}

func TestIsDangerousRuleToolLevelExec(t *testing.T) {
	rule := &ParsedRule{ToolName: "Exec", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("tool-level Exec allow should be dangerous")
	}
}

func TestIsDangerousRulePython(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "python", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("python should be dangerous")
	}
}

func TestIsDangerousRuleNode(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "node", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("node should be dangerous")
	}
}

func TestIsDangerousRuleSudo(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "sudo", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("sudo should be dangerous")
	}
}

func TestIsDangerousRuleBash(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "bash", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("bash should be dangerous")
	}
}

func TestIsDangerousRuleEval(t *testing.T) {
	rule := &ParsedRule{ToolName: "Exec", Content: "eval", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("eval should be dangerous")
	}
}

func TestIsDangerousRulePythonScript(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "python script.py", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("python script.py should be dangerous")
	}
}

func TestIsDangerousRulePythonColon(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "python:script", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("python:script should be dangerous")
	}
}

func TestIsDangerousRuleNpmRun(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "npm run", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("npm run should be dangerous")
	}
}

func TestIsDangerousRuleSafe(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "ls -la", Behavior: "allow"}
	if IsDangerousAllowRule(rule) {
		t.Error("ls -la should not be dangerous")
	}
}

func TestIsDangerousRuleCaseInsensitive(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "PYTHON", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("PYTHON should be dangerous (case insensitive)")
	}
}

func TestIsDangerousRuleZsh(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "zsh", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("zsh should be dangerous")
	}
}

func TestIsDangerousRuleFish(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "fish", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("fish should be dangerous")
	}
}

func TestIsDangerousRuleXargs(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "xargs", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("xargs should be dangerous")
	}
}

func TestIsDangerousRuleSSH(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "ssh", Behavior: "allow"}
	if !IsDangerousAllowRule(rule) {
		t.Error("ssh should be dangerous")
	}
}

// ─── matchesDangerousPattern ────────────────────────────────────────────────

func TestMatchesDangerousExact(t *testing.T) {
	if !matchesDangerousPattern("python", "python") {
		t.Error("exact match should succeed")
	}
}

func TestMatchesDangerousPrefixColon(t *testing.T) {
	if !matchesDangerousPattern("python:*", "python:*") {
		t.Error("python:* should match")
	}
}

func TestMatchesDangerousPrefixStar(t *testing.T) {
	if !matchesDangerousPattern("python*", "python*") {
		t.Error("python* should match")
	}
}

func TestMatchesDangerousSpaceStar(t *testing.T) {
	if !matchesDangerousPattern("python *", "python *") {
		t.Error("python * should match")
	}
}

func TestMatchesDangerousFlagStar(t *testing.T) {
	if !matchesDangerousPattern("python -*", "python -*") {
		t.Error("python -* should match")
	}
}

func TestMatchesDangerousContentPrefix(t *testing.T) {
	if !matchesDangerousPattern("python script.py", "python") {
		t.Error("python script.py should match python prefix")
	}
}

func TestMatchesDangerousContentColon(t *testing.T) {
	if !matchesDangerousPattern("python:run", "python") {
		t.Error("python:run should match python colon")
	}
}

func TestNoMatchDifferentCommand(t *testing.T) {
	if matchesDangerousPattern("ls", "python") {
		t.Error("ls should not match python")
	}
}

// ─── StripDangerousAllowRules ───────────────────────────────────────────────

func TestStripDangerousAllowRulesEmpty(t *testing.T) {
	store := NewRuleStore()
	stash := store.StripDangerousAllowRules()
	if len(stash) != 0 {
		t.Errorf("expected empty stash, got %d entries", len(stash))
	}
}

func TestStripDangerousAllowRulesWithDangerous(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash(python)")
	rule.Behavior = "allow"
	store.AddRules("session", "allow", []*ParsedRule{rule})

	stash := store.StripDangerousAllowRules()
	if len(stash) == 0 {
		t.Error("dangerous rule should be stripped")
	}
}

func TestStripDangerousAllowRulesNonDangerous(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash(ls)")
	rule.Behavior = "allow"
	store.AddRules("session", "allow", []*ParsedRule{rule})

	stash := store.StripDangerousAllowRules()
	if len(stash) != 0 {
		t.Error("non-dangerous rule should not be stripped")
	}
}

func TestStripDangerousAllowRulesIgnoreDeny(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash(python)")
	rule.Behavior = "deny"
	store.AddRules("session", "deny", []*ParsedRule{rule})

	stash := store.StripDangerousAllowRules()
	if len(stash) != 0 {
		t.Error("deny rule should not be stripped")
	}
}

// ─── RestoreStrippedRules ──────────────────────────────────────────────────

func TestRestoreStrippedRules(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash(python)")
	rule.Behavior = "allow"
	store.AddRules("session", "allow", []*ParsedRule{rule})

	stash := store.StripDangerousAllowRules()
	store.RestoreStrippedRules(stash)

	// Rule should be back
	found := false
	for _, rules := range store.rules {
		for _, r := range rules {
			if r.ToolName == "Bash" && r.Content == "python" {
				found = true
			}
		}
	}
	if !found {
		t.Error("restored rule should be found")
	}
}

// ─── StrippedRulesSummary ──────────────────────────────────────────────────

func TestStrippedRulesSummaryEmpty(t *testing.T) {
	if StrippedRulesSummary(nil) != "" {
		t.Error("empty stash should have empty summary")
	}
}

// ============================================================================
// Upstream Quality: Port from dangerousPatterns.test.ts
// Tests for DANGEROUS_SHELL_PATTERNS (equivalent to CROSS_PLATFORM_CODE_EXEC +
// DANGEROUS_BASH_PATTERNS in upstream).
// ============================================================================

// ─── DANGEROUS_SHELL_PATTERNS: content invariants ───────────────────────────

func TestDangerousShellPatternsNonEmpty(t *testing.T) {
	// Invariant from upstream: CROSS_PLATFORM_CODE_EXEC.length > 0
	if len(DANGEROUS_SHELL_PATTERNS) == 0 {
		t.Error("DANGEROUS_SHELL_PATTERNS should not be empty")
	}
}

func TestDangerousShellPatternsAllStrings(t *testing.T) {
	// Invariant from upstream: all elements are strings
	for i, p := range DANGEROUS_SHELL_PATTERNS {
		if p == "" {
			t.Errorf("DANGEROUS_SHELL_PATTERNS[%d] should not be empty string", i)
		}
	}
}

func TestDangerousShellPatternsIncludesCoreInterpreters(t *testing.T) {
	// Upstream: includes core interpreters
	set := make(map[string]bool, len(DANGEROUS_SHELL_PATTERNS))
	for _, p := range DANGEROUS_SHELL_PATTERNS {
		set[p] = true
	}
	for _, expected := range []string{"python", "node", "ruby", "perl"} {
		if !set[expected] {
			t.Errorf("DANGEROUS_SHELL_PATTERNS should include %q", expected)
		}
	}
}

func TestDangerousShellPatternsIncludesPackageRunners(t *testing.T) {
	// Upstream: includes package runners
	set := make(map[string]bool, len(DANGEROUS_SHELL_PATTERNS))
	for _, p := range DANGEROUS_SHELL_PATTERNS {
		set[p] = true
	}
	for _, expected := range []string{"npx", "bunx"} {
		if !set[expected] {
			t.Errorf("DANGEROUS_SHELL_PATTERNS should include %q", expected)
		}
	}
}

func TestDangerousShellPatternsIncludesShells(t *testing.T) {
	// Upstream: includes shells
	set := make(map[string]bool, len(DANGEROUS_SHELL_PATTERNS))
	for _, p := range DANGEROUS_SHELL_PATTERNS {
		set[p] = true
	}
	for _, expected := range []string{"bash", "sh"} {
		if !set[expected] {
			t.Errorf("DANGEROUS_SHELL_PATTERNS should include %q", expected)
		}
	}
}

func TestDangerousShellPatternsIncludesUnixSpecific(t *testing.T) {
	// Upstream: includes unix-specific patterns
	set := make(map[string]bool, len(DANGEROUS_SHELL_PATTERNS))
	for _, p := range DANGEROUS_SHELL_PATTERNS {
		set[p] = true
	}
	for _, expected := range []string{"zsh", "fish", "eval", "exec", "sudo", "xargs", "env"} {
		if !set[expected] {
			t.Errorf("DANGEROUS_SHELL_PATTERNS should include %q", expected)
		}
	}
}

func TestDangerousShellPatternsNoDuplicates(t *testing.T) {
	// Invariant from upstream: new Set(DANGEROUS_BASH_PATTERNS).size === DANGEROUS_BASH_PATTERNS.length
	seen := make(map[string]bool)
	for _, p := range DANGEROUS_SHELL_PATTERNS {
		if seen[p] {
			t.Errorf("DANGEROUS_SHELL_PATTERNS has duplicate entry: %q", p)
		}
		seen[p] = true
	}
}

func TestDangerousShellPatternsEmptyStringNoMatch(t *testing.T) {
	// Upstream: "" does not start with any dangerous pattern
	for _, pattern := range DANGEROUS_SHELL_PATTERNS {
		if strings.HasPrefix("", strings.ToLower(pattern)) {
			t.Errorf("empty string should not match pattern %q", pattern)
		}
	}
}

func TestDangerousShellPatternsContainsExpectedInterpreters(t *testing.T) {
	// Upstream: contains expected interpreters
	expected := []string{
		"node", "python", "python3", "ruby", "perl", "php", "lua",
		"deno", "npx", "bunx", "tsx",
	}
	set := make(map[string]bool, len(DANGEROUS_SHELL_PATTERNS))
	for _, p := range DANGEROUS_SHELL_PATTERNS {
		set[p] = true
	}
	for _, entry := range expected {
		if !set[entry] {
			t.Errorf("DANGEROUS_SHELL_PATTERNS should include %q", entry)
		}
	}
}
