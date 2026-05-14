package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"miniclaudecode-go/skills"
	"miniclaudecode-go/tools"
)

func TestCachedSystemPromptGetOrBuild(t *testing.T) {
	csp := NewCachedSystemPrompt()
	registry := tools.NewRegistry()

	// First call should build
	prompt1 := csp.GetOrBuild(registry, "auto", "/tmp/test", "test-model", nil, nil, nil)
	if prompt1 == "" {
		t.Error("GetOrBuild should return non-empty prompt")
	}

	// Second call should return cached (same content)
	prompt2 := csp.GetOrBuild(registry, "auto", "/tmp/test", "test-model", nil, nil, nil)
	if prompt1 != prompt2 {
		t.Error("GetOrBuild should return cached prompt on second call")
	}
}

func TestCachedSystemPromptMarkDirty(t *testing.T) {
	csp := NewCachedSystemPrompt()
	registry := tools.NewRegistry()

	// Build initial prompt
	_ = csp.GetOrBuild(registry, "auto", "/tmp/test", "test-model", nil, nil, nil)

	// Mark dirty
	csp.MarkDirty()

	// Next call should rebuild (same content since same config)
	prompt2 := csp.GetOrBuild(registry, "auto", "/tmp/test", "test-model", nil, nil, nil)
	if prompt2 == "" {
		t.Error("GetOrBuild after MarkDirty should return non-empty prompt")
	}
}

func TestCachedSystemPromptConcurrent(t *testing.T) {
	csp := NewCachedSystemPrompt()
	registry := tools.NewRegistry()

	var wg sync.WaitGroup
	results := make([]string, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = csp.GetOrBuild(registry, "auto", "/tmp/test", "test-model", nil, nil, nil)
		}(i)
	}
	wg.Wait()

	// All results should be non-empty
	for i, r := range results {
		if r == "" {
			t.Errorf("goroutine %d got empty prompt", i)
		}
	}
}

func TestBuildSystemPromptContainsSections(t *testing.T) {
	registry := tools.NewRegistry()
	prompt := BuildSystemPrompt(registry, "auto", "/tmp/test", "test-model", nil, nil, nil)

	// Should contain key sections
	checks := []string{
		"You are",
		"tool",
		"Environment",
	}

	for _, check := range checks {
		if !strings.Contains(strings.ToLower(prompt), strings.ToLower(check)) {
			t.Errorf("system prompt should contain %q", check)
		}
	}
}

func TestBuildSystemPromptGitContext(t *testing.T) {
	registry := tools.NewRegistry()
	prompt := BuildSystemPrompt(registry, "auto", "/tmp/test", "test-model", nil, nil, nil)

	// If we're in a git repo (likely during development), the prompt should contain
	// git context in the Environment section. If not in a git repo, it should still
	// have the Environment section.
	if !strings.Contains(prompt, "Environment") {
		t.Error("system prompt should contain Environment section")
	}

	// When in a git repo, verify git context injection
	wd, _ := os.Getwd()
	if _, err := tools.FindGitRoot(wd); err == nil {
		// We're in a git repo, so the prompt should contain Git context
		if !strings.Contains(prompt, "Git Branch:") && !strings.Contains(prompt, "Git Dirty:") {
			t.Error("system prompt should contain git context when in a git repo")
		}
	}
}

func TestBuildSystemPromptModeSpecific(t *testing.T) {
	registry := tools.NewRegistry()

	tests := []struct {
		mode    string
		keyword string
	}{
		{"auto", "AUTO"},
		{"ask", "ASK"},
		{"plan", "PLAN"},
	}

	for _, tt := range tests {
		prompt := BuildSystemPrompt(registry, tt.mode, "/tmp/test", "test-model", nil, nil, nil)
		if !strings.Contains(prompt, tt.keyword) {
			t.Errorf("mode %s prompt should contain %q", tt.mode, tt.keyword)
		}
	}
}

func TestBuildSystemPromptWithSkills(t *testing.T) {
	registry := tools.NewRegistry()
	loader := skills.NewLoader("/tmp/skills")
	tracker := skills.NewSkillTracker()

	prompt := BuildSystemPrompt(registry, "auto", "/tmp/test", "test-model", loader, tracker, nil)
	if prompt == "" {
		t.Error("prompt with skills should be non-empty")
	}
}

// Suppress unused import warnings
var _ = skills.NewLoader
var _ = tools.NewRegistry

// ---------------------------------------------------------------------------
// fnvHash — system_prompt.go:609
// ---------------------------------------------------------------------------

func TestFnvHashDeterminism(t *testing.T) {
	// Same input always produces same hash
	input := "test content for hashing"
	h1 := fnvHash(input)
	h2 := fnvHash(input)
	if h1 != h2 {
		t.Errorf("fnvHash not deterministic: %d != %d", h1, h2)
	}
}

func TestFnvHashDifferentInputs(t *testing.T) {
	// Different inputs should produce different hashes (collision resistance)
	inputs := []string{
		"hello",
		"world",
		"hello world",
		"Hello",        // case difference
		"hello ",       // trailing space
		"hello\n",      // trailing newline
		"",             // empty string
		"こんにちは",     // unicode
		"a very long string that tests the hash function with more characters to ensure good distribution",
	}

	hashes := make(map[uint64]string)
	for _, input := range inputs {
		h := fnvHash(input)
		if existing, ok := hashes[h]; ok {
			t.Errorf("collision: %q and %q both hash to %d", existing, input, h)
		}
		hashes[h] = input
	}
}

func TestFnvHashEmptyString(t *testing.T) {
	// Empty string should hash to the FNV-1a 64-bit initial value
	h := fnvHash("")
	// FNV-1a 64-bit offset base: 14695981039346656037 (0xcbf29ce484222325)
	if h != 14695981039346656037 {
		t.Errorf("fnvHash(\"\") = %d, want 14695981039346656037", h)
	}
}

func TestFnvHashKnownValues(t *testing.T) {
	// Known FNV-1a 64-bit values for verification
	// These are the actual values produced by Go's FNV-1a 64-bit implementation
	tests := []struct {
		input string
		want  uint64
	}{
		{"", 14695981039346656037},  // offset base
		{"a", 12638187200555641996},
	}
	for _, tt := range tests {
		got := fnvHash(tt.input)
		if got != tt.want {
			t.Errorf("fnvHash(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestFnvHash64BitRange(t *testing.T) {
	// Hash values should span the full 64-bit range
	var minHash, maxHash uint64 = ^uint64(0), 0
	for i := 0; i < 1000; i++ {
		h := fnvHash(fmt.Sprintf("input-%d", i))
		if h < minHash {
			minHash = h
		}
		if h > maxHash {
			maxHash = h
		}
	}
	// Verify reasonable spread (not all values clustered)
	if maxHash-minHash < 1000000 {
		t.Errorf("hash values too clustered: min=%d, max=%d, range=%d", minHash, maxHash, maxHash-minHash)
	}
}