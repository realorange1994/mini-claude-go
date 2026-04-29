package main

import (
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
	prompt1 := csp.GetOrBuild(registry, "auto", "/tmp/test", nil, nil)
	if prompt1 == "" {
		t.Error("GetOrBuild should return non-empty prompt")
	}

	// Second call should return cached (same content)
	prompt2 := csp.GetOrBuild(registry, "auto", "/tmp/test", nil, nil)
	if prompt1 != prompt2 {
		t.Error("GetOrBuild should return cached prompt on second call")
	}
}

func TestCachedSystemPromptMarkDirty(t *testing.T) {
	csp := NewCachedSystemPrompt()
	registry := tools.NewRegistry()

	// Build initial prompt
	_ = csp.GetOrBuild(registry, "auto", "/tmp/test", nil, nil)

	// Mark dirty
	csp.MarkDirty()

	// Next call should rebuild (same content since same config)
	prompt2 := csp.GetOrBuild(registry, "auto", "/tmp/test", nil, nil)
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
			results[idx] = csp.GetOrBuild(registry, "auto", "/tmp/test", nil, nil)
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
	prompt := BuildSystemPrompt(registry, "auto", "/tmp/test", nil, nil)

	// Should contain key sections
	checks := []string{
		"You are",
		"tool",
	}

	for _, check := range checks {
		if !strings.Contains(strings.ToLower(prompt), strings.ToLower(check)) {
			t.Errorf("system prompt should contain %q", check)
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
		prompt := BuildSystemPrompt(registry, tt.mode, "/tmp/test", nil, nil)
		if !strings.Contains(prompt, tt.keyword) {
			t.Errorf("mode %s prompt should contain %q", tt.mode, tt.keyword)
		}
	}
}

func TestBuildSystemPromptWithSkills(t *testing.T) {
	registry := tools.NewRegistry()
	loader := skills.NewLoader("/tmp/skills")
	tracker := skills.NewSkillTracker()

	prompt := BuildSystemPrompt(registry, "auto", "/tmp/test", loader, tracker)
	if prompt == "" {
		t.Error("prompt with skills should be non-empty")
	}
}

// Suppress unused import warnings
var _ = skills.NewLoader
var _ = tools.NewRegistry