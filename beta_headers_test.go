package main

import (
	"os"
	"sort"
	"strings"
	"testing"
)

func TestBuildBetaHeadersDefault(t *testing.T) {
	betas := BuildBetaHeaders("claude-sonnet-4-20250514")

	// Must always include Claude Code identification
	if !containsStr(betas, BetaHeaderClaudeCode) {
		t.Error("BuildBetaHeaders must always include claude-code beta")
	}
	// Must always include prompt caching
	if !containsStr(betas, BetaHeaderPromptCaching) {
		t.Error("BuildBetaHeaders must always include prompt-caching beta")
	}
	// Must always include context management
	if !containsStr(betas, BetaHeaderContextManagement) {
		t.Error("BuildBetaHeaders must always include context-management beta")
	}
	// Must always include max tokens
	if !containsStr(betas, BetaHeaderMaxTokens) {
		t.Error("BuildBetaHeaders must always include max-tokens beta")
	}
	// Should NOT include [1m] context beta without [1m] suffix
	if containsStr(betas, BetaHeaderContext1M) {
		t.Error("BuildBetaHeaders should NOT include context-1m without [1m] suffix")
	}
}

func TestBuildBetaHeaders1mSuffix(t *testing.T) {
	betas := BuildBetaHeaders("claude-sonnet-4-20250514[1m]")
	if !containsStr(betas, BetaHeaderContext1M) {
		t.Error("BuildBetaHeaders must include context-1m when model has [1m] suffix")
	}
}

func TestBuildBetaHeadersInterleavedThinkingEnvVar(t *testing.T) {
	// Save original
	orig := os.Getenv("CLAUDE_CODE_DISABLE_INTERLEAVED_THINKING")
	defer os.Setenv("CLAUDE_CODE_DISABLE_INTERLEAVED_THINKING", orig)

	// Default: interleaved thinking included
	betas := BuildBetaHeaders("claude-sonnet-4-20250514")
	if !containsStr(betas, BetaHeaderInterleavedThinking) {
		t.Error("BuildBetaHeaders should include interleaved-thinking by default")
	}

	// Set env var to disable
	os.Setenv("CLAUDE_CODE_DISABLE_INTERLEAVED_THINKING", "1")
	betas = BuildBetaHeaders("claude-sonnet-4-20250514")
	if containsStr(betas, BetaHeaderInterleavedThinking) {
		t.Error("BuildBetaHeaders should NOT include interleaved-thinking when env var is set")
	}
}

func TestBuildBetaHeadersTokenCountingEnvVar(t *testing.T) {
	orig := os.Getenv("CLAUDE_CODE_DISABLE_TOKEN_COUNTING")
	defer os.Setenv("CLAUDE_CODE_DISABLE_TOKEN_COUNTING", orig)

	// Default: token counting included
	betas := BuildBetaHeaders("claude-sonnet-4-20250514")
	if !containsStr(betas, BetaHeaderTokenCounting) {
		t.Error("BuildBetaHeaders should include token-counting by default")
	}

	// Set env var to disable
	os.Setenv("CLAUDE_CODE_DISABLE_TOKEN_COUNTING", "true")
	betas = BuildBetaHeaders("claude-sonnet-4-20250514")
	if containsStr(betas, BetaHeaderTokenCounting) {
		t.Error("BuildBetaHeaders should NOT include token-counting when env var is set")
	}
}

func TestFormatBetaHeader(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{[]string{"a", "b", "c"}, "a,b,c"},
		{[]string{}, ""},
		{[]string{"only"}, "only"},
	}
	for _, tt := range tests {
		got := FormatBetaHeader(tt.input)
		if got != tt.expected {
			t.Errorf("FormatBetaHeader(%v) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestGetModelForAPIStrips1mSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-sonnet-4-20250514[1m]", "claude-sonnet-4-20250514"},
		{"claude-3-5-sonnet-20241022[1m]", "claude-3-5-sonnet-20241022"},
		{"claude-sonnet-4-20250514", "claude-sonnet-4-20250514"},
		{"M2.7", "M2.7"},
		{"MiniMax-M2.7", "MiniMax-M2.7"},
		// Known aliases are lowercased (resolution happens elsewhere)
		{"sonnet", "sonnet"},
		{"opus", "opus"},
		{"haiku", "haiku"},
		// [1m] suffix on alias — strip it, then lowercase
		{"sonnet[1m]", "sonnet"},
		{"opus[1m]", "opus"},
	}
	for _, tt := range tests {
		got := GetModelForAPI(tt.input)
		if got != tt.expected {
			t.Errorf("GetModelForAPI(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestIsEnvTruthy(t *testing.T) {
	tests := []struct {
		value  string
		expect bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"1", true},
		{"true", true},
		{"yes", true},
		{"True", true},
		{"YES", true},
	}
	for _, tt := range tests {
		os.Setenv("TEST_TRUTHY", tt.value)
		got := isEnvTruthy("TEST_TRUTHY")
		if got != tt.expect {
			t.Errorf("isEnvTruthy(%q) = %v, expected %v", tt.value, got, tt.expect)
		}
	}
	os.Unsetenv("TEST_TRUTHY")
}

func TestBetaHeaderConstants(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{"BetaHeaderPromptCaching", BetaHeaderPromptCaching, "prompt-caching-2024-07-31"},
		{"BetaHeaderContext1M", BetaHeaderContext1M, "context-1m-2025-08-07"},
		{"BetaHeaderInterleavedThinking", BetaHeaderInterleavedThinking, "interleaved-thinking-2025-05-14"},
		{"BetaHeaderContextManagement", BetaHeaderContextManagement, "context-management-2025-06-27"},
		{"BetaHeaderMaxTokens", BetaHeaderMaxTokens, "max-tokens-3-5-sonnet-2024-07-15"},
		{"BetaHeaderTokenCounting", BetaHeaderTokenCounting, "token-counting-2024-11-01"},
		{"BetaHeaderComputerUse", BetaHeaderComputerUse, "computer-use-2024-10-22"},
		{"BetaHeaderClaudeCode", BetaHeaderClaudeCode, "claude-code-20250219"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("%s = %q, expected %q", tt.name, tt.value, tt.expected)
			}
		})
	}
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ─── Upstream Quality: No-duplicate beta headers invariant ───────────────────

func TestBuildBetaHeadersNoDuplicates(t *testing.T) {
	// BuildBetaHeaders must never return duplicate entries — invariant from upstream
	models := []string{
		"claude-sonnet-4-20250514",
		"claude-sonnet-4-20250514[1m]",
		"claude-opus-4-5-20250610",
		"claude-opus-4-5-20250610[1m]",
		"claude-haiku-4-5-20250610",
		"M2.7",
	}
	for _, m := range models {
		betas := BuildBetaHeaders(m)
		seen := make(map[string]bool)
		for _, b := range betas {
			if seen[b] {
				t.Errorf("BuildBetaHeaders(%q) returned duplicate beta header %q", m, b)
			}
			seen[b] = true
		}
	}
}

// ─── Upstream Quality: Beta header count invariant ────────────────────────────

func TestBuildBetaHeadersCountInvariant(t *testing.T) {
	// Default: 6 betas (claude-code, caching, interleaved-thinking, context-mgmt, max-tokens, token-counting)
	// [1m] adds one more (context-1m)
	// computer-use is NOT added unconditionally in BuildBetaHeaders
	betas := BuildBetaHeaders("claude-sonnet-4-20250514")
	if len(betas) != 6 {
		t.Errorf("expected 6 default beta headers, got %d", len(betas))
	}

	// With [1m]: 7 betas (adds context-1m)
	betas1m := BuildBetaHeaders("claude-sonnet-4-20250514[1m]")
	if len(betas1m) != 7 {
		t.Errorf("expected 7 beta headers with [1m], got %d", len(betas1m))
	}

	// With both env vars disabled and [1m]: 5 betas
	origThinking := os.Getenv("CLAUDE_CODE_DISABLE_INTERLEAVED_THINKING")
	origToken := os.Getenv("CLAUDE_CODE_DISABLE_TOKEN_COUNTING")
	defer func() {
		os.Setenv("CLAUDE_CODE_DISABLE_INTERLEAVED_THINKING", origThinking)
		os.Setenv("CLAUDE_CODE_DISABLE_TOKEN_COUNTING", origToken)
	}()
	os.Setenv("CLAUDE_CODE_DISABLE_INTERLEAVED_THINKING", "1")
	os.Setenv("CLAUDE_CODE_DISABLE_TOKEN_COUNTING", "1")
	betas = BuildBetaHeaders("claude-sonnet-4-20250514[1m]")
	if len(betas) != 5 {
		t.Errorf("expected 5 beta headers with [1m] + 2 disabled, got %d", len(betas))
	}
}

// ─── Upstream Quality: All model families coverage ────────────────────────────

func TestBuildBetaHeadersAllFamilies(t *testing.T) {
	// Every model family should get the same base set of beta headers
	families := []string{
		"claude-sonnet-4-20250514",
		"claude-opus-4-5-20250610",
		"claude-haiku-4-5-20250610",
		"claude-3-5-sonnet-20241022",
	}
	for _, m := range families {
		betas := BuildBetaHeaders(m)
		// All should contain the core betas
		for _, required := range []string{BetaHeaderClaudeCode, BetaHeaderPromptCaching, BetaHeaderMaxTokens} {
			if !containsStr(betas, required) {
				t.Errorf("BuildBetaHeaders(%q) missing required beta: %q", m, required)
			}
		}
		// None should have [1m] without suffix
		if containsStr(betas, BetaHeaderContext1M) {
			t.Errorf("BuildBetaHeaders(%q) should NOT include context-1m without [1m] suffix", m)
		}
	}
}

// ─── Upstream Quality: BuildBetaHeaders sorted output ─────────────────────────

func TestBuildBetaHeadersOrder(t *testing.T) {
	// Beta headers should be returned in a deterministic order
	betas1 := BuildBetaHeaders("claude-sonnet-4-20250514")
	betas2 := BuildBetaHeaders("claude-sonnet-4-20250514")
	if len(betas1) != len(betas2) {
		t.Fatalf("same model should produce same number of betas")
	}
	for i := range betas1 {
		if betas1[i] != betas2[i] {
			t.Errorf("beta header order mismatch at index %d: %q vs %q", i, betas1[i], betas2[i])
		}
	}
}

// ─── Upstream Quality: FormatBetaHeader roundtrip ─────────────────────────────

func TestFormatBetaHeaderRoundtrip(t *testing.T) {
	// BuildBetaHeaders → FormatBetaHeader → split should recover all betas
	model := "claude-sonnet-4-20250514[1m]"
	betas := BuildBetaHeaders(model)
	formatted := FormatBetaHeader(betas)
	parts := strings.Split(formatted, ",")
	sort.Strings(parts)
	sort.Strings(betas)
	if len(parts) != len(betas) {
		t.Fatalf("roundtrip length mismatch: original=%d, formatted=%d", len(betas), len(parts))
	}
	for i := range betas {
		if parts[i] != betas[i] {
			t.Errorf("roundtrip mismatch at %d: expected %q, got %q", i, betas[i], parts[i])
		}
	}
}