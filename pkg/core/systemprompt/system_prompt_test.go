package systemprompt

import (
	"strings"
	"testing"
)

func TestBuildSystemPrompt_CustomPrompt(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		CustomPrompt:       "You are a helpful assistant.",
		AppendSystemPrompt: "Always be concise.",
	})
	if !strings.Contains(prompt, "You are a helpful assistant.") {
		t.Error("missing custom prompt")
	}
	if !strings.Contains(prompt, "Always be concise.") {
		t.Error("missing append section")
	}
}

func TestBuildSystemPrompt_DefaultPrompt(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools: []string{"Read", "Bash"},
		ToolSnippets: map[string]string{
			"Read": "Read file contents",
			"Bash": "Run shell commands",
		},
		Cwd: "/home/user",
	})
	if !strings.Contains(prompt, "Available tools:") {
		t.Error("missing tools section")
	}
	if !strings.Contains(prompt, "Read: Read file contents") {
		t.Error("missing Read tool")
	}
	if !strings.Contains(prompt, "Bash: Run shell commands") {
		t.Error("missing Bash tool")
	}
	if !strings.Contains(prompt, "Guidelines:") {
		t.Error("missing guidelines")
	}
	if !strings.Contains(prompt, "/home/user") {
		t.Error("missing cwd")
	}
}

func TestBuildSystemPrompt_VisibleToolsOnly(t *testing.T) {
	// Tools without snippets should not appear
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools: []string{"Read", "Bash", "Missing"},
		ToolSnippets: map[string]string{
			"Read": "Read files",
		},
	})
	if strings.Contains(prompt, "Bash") {
		t.Error("Bash should not appear (no snippet)")
	}
	if !strings.Contains(prompt, "Read: Read files") {
		t.Error("missing Read tool")
	}
}

func TestBuildSystemPrompt_ContextFiles(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools: []string{"Read"},
		ToolSnippets:  map[string]string{"Read": "Read files"},
		ContextFiles: []ContextFile{
			{Path: "CLAUDE.md", Content: "Use Go 1.22+"},
		},
	})
	if !strings.Contains(prompt, "<project_context>") {
		t.Error("missing project context")
	}
	if !strings.Contains(prompt, `path="CLAUDE.md"`) {
		t.Error("missing context file path")
	}
	if !strings.Contains(prompt, "Use Go 1.22+") {
		t.Error("missing context file content")
	}
}

func TestBuildSystemPrompt_GuidelineDedup(t *testing.T) {
	prompt := BuildSystemPrompt(BuildSystemPromptOptions{
		SelectedTools:   []string{"Bash"},
		ToolSnippets:    map[string]string{"Bash": "Run commands"},
		PromptGuidelines: []string{"My custom dedup guideline", "My custom dedup guideline"},
	})
	// Should appear exactly once
	count := strings.Count(prompt, "- My custom dedup guideline")
	if count != 1 {
		t.Errorf("expected 1 occurrence, got %d", count)
	}
}
