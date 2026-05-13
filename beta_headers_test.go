package main

import (
	"os"
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