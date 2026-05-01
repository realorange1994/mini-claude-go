package main

import (
	"strings"
	"testing"

	"miniclaudecode-go/tools"
)

func TestBuildSubAgentSystemPromptContainsSections(t *testing.T) {
	registry := tools.NewRegistry()
	cfg := Config{Model: "test-model", PermissionMode: "auto"}

	for _, agentType := range []AgentType{AgentTypeExplore, AgentTypePlan, AgentTypeVerify, AgentTypeGeneral} {
		prompt := buildSubAgentSystemPrompt(registry, cfg, agentType)

		// All prompts should contain key sections
		required := []string{
			"## Environment",
			"## Permission Mode",
			"## Available Tools",
			"## Output Format",
			"## Security",
		}
		for _, section := range required {
			if !strings.Contains(prompt, section) {
				t.Errorf("agentType=%q: prompt should contain %q", agentType, section)
			}
		}

		// All prompts should contain behavioral rules
		if !strings.Contains(prompt, "Do NOT ask the user questions") {
			t.Errorf("agentType=%q: prompt should contain autonomous completion rule", agentType)
		}
		if !strings.Contains(prompt, "provide your final answer concisely") {
			t.Errorf("agentType=%q: prompt should contain concise answer rule", agentType)
		}
		if !strings.Contains(prompt, "absolute file paths") || !strings.Contains(prompt, "absolute paths") {
			t.Errorf("agentType=%q: prompt should contain absolute path rule", agentType)
		}
	}
}

func TestBuildSubAgentSystemPromptExploreReadOnly(t *testing.T) {
	registry := tools.NewRegistry()
	cfg := Config{Model: "test-model", PermissionMode: "auto"}

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeExplore)

	// Explore should have READ-ONLY constraint
	if !strings.Contains(prompt, "READ-ONLY MODE") {
		t.Error("Explore prompt should contain READ-ONLY MODE constraint")
	}
	if !strings.Contains(prompt, "STRICTLY PROHIBITED") {
		t.Error("Explore prompt should contain STRICTLY PROHIBITED")
	}
	if !strings.Contains(prompt, "file search specialist") {
		t.Error("Explore prompt should contain 'file search specialist' identity")
	}
	// Should have efficiency optimization note
	if !strings.Contains(prompt, "CLAUDE.md and gitStatus are omitted") {
		t.Error("Explore prompt should mention omitted CLAUDE.md/gitStatus")
	}
	// Should have tool guidance
	if !strings.Contains(prompt, "glob for broad file pattern matching") {
		t.Error("Explore prompt should contain glob guidance")
	}
	if !strings.Contains(prompt, "grep for searching file contents") {
		t.Error("Explore prompt should contain grep guidance")
	}
}

func TestBuildSubAgentSystemPromptPlanReadOnly(t *testing.T) {
	registry := tools.NewRegistry()
	cfg := Config{Model: "test-model", PermissionMode: "auto"}

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypePlan)

	// Plan should have READ-ONLY constraint
	if !strings.Contains(prompt, "READ-ONLY MODE") {
		t.Error("Plan prompt should contain READ-ONLY MODE constraint")
	}
	if !strings.Contains(prompt, "software architect and planning specialist") {
		t.Error("Plan prompt should contain 'software architect and planning specialist' identity")
	}
	// Should have process steps
	if !strings.Contains(prompt, "Understand Requirements") {
		t.Error("Plan prompt should contain 'Understand Requirements' step")
	}
	if !strings.Contains(prompt, "Explore Thoroughly") {
		t.Error("Plan prompt should contain 'Explore Thoroughly' step")
	}
	if !strings.Contains(prompt, "Design Solution") {
		t.Error("Plan prompt should contain 'Design Solution' step")
	}
	if !strings.Contains(prompt, "Detail the Plan") {
		t.Error("Plan prompt should contain 'Detail the Plan' step")
	}
	// Should have critical files section
	if !strings.Contains(prompt, "Critical Files for Implementation") {
		t.Error("Plan prompt should contain 'Critical Files for Implementation'")
	}
	// Should have efficiency optimization note
	if !strings.Contains(prompt, "CLAUDE.md and gitStatus are omitted") {
		t.Error("Plan prompt should mention omitted CLAUDE.md/gitStatus")
	}
}

func TestBuildSubAgentSystemPromptVerifyAdversarial(t *testing.T) {
	registry := tools.NewRegistry()
	cfg := Config{Model: "test-model", PermissionMode: "auto"}

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeVerify)

	// Verify should have adversarial mindset
	if !strings.Contains(prompt, "verification specialist") {
		t.Error("Verify prompt should contain 'verification specialist' identity")
	}
	if !strings.Contains(prompt, "try to break it") {
		t.Error("Verify prompt should contain adversarial instruction")
	}
	if !strings.Contains(prompt, "DO NOT MODIFY THE PROJECT") {
		t.Error("Verify prompt should contain project modification constraint")
	}
	// Should have verification strategies
	if !strings.Contains(prompt, "Verification Strategy") {
		t.Error("Verify prompt should contain 'Verification Strategy'")
	}
	if !strings.Contains(prompt, "Required Steps") {
		t.Error("Verify prompt should contain 'Required Steps'")
	}
	if !strings.Contains(prompt, "Adversarial Probes") {
		t.Error("Verify prompt should contain 'Adversarial Probes'")
	}
	// Should have VERDICT format
	if !strings.Contains(prompt, "VERDICT: PASS") {
		t.Error("Verify prompt should contain 'VERDICT: PASS'")
	}
	if !strings.Contains(prompt, "VERDICT: FAIL") {
		t.Error("Verify prompt should contain 'VERDICT: FAIL'")
	}
	if !strings.Contains(prompt, "VERDICT: PARTIAL") {
		t.Error("Verify prompt should contain 'VERDICT: PARTIAL'")
	}
	// Should have rationalization recognition
	if !strings.Contains(prompt, "Recognize Your Own Rationalizations") {
		t.Error("Verify prompt should contain rationalization awareness")
	}
}

func TestBuildSubAgentSystemPromptGeneral(t *testing.T) {
	registry := tools.NewRegistry()
	cfg := Config{Model: "test-model", PermissionMode: "auto"}

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeGeneral)

	// General agent should have basic sections but no type-specific modifier
	if !strings.Contains(prompt, "## Environment") {
		t.Error("General prompt should contain Environment section")
	}
	if !strings.Contains(prompt, "## Output Format") {
		t.Error("General prompt should contain Output Format section")
	}
	// Should NOT have READ-ONLY constraint
	if strings.Contains(prompt, "READ-ONLY MODE") {
		t.Error("General prompt should NOT contain READ-ONLY MODE constraint")
	}
	// Should NOT have efficiency optimization note (only for Explore/Plan)
	if strings.Contains(prompt, "CLAUDE.md and gitStatus are omitted") {
		t.Error("General prompt should NOT mention omitted CLAUDE.md/gitStatus")
	}
}

func TestBuildSubAgentSystemPromptPriorityOrder(t *testing.T) {
	registry := tools.NewRegistry()
	cfg := Config{Model: "test-model", PermissionMode: "auto"}

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeExplore)

	// Agent type modifier should come FIRST (before Environment)
	exploreIdx := strings.Index(prompt, "file search specialist")
	envIdx := strings.Index(prompt, "## Environment")
	if exploreIdx == -1 || envIdx == -1 {
		t.Fatal("prompt missing expected sections")
	}
	if exploreIdx > envIdx {
		t.Error("agent type identity should appear before Environment section")
	}
}

func TestBuildSubAgentSystemPromptPermissionMode(t *testing.T) {
	registry := tools.NewRegistry()

	for _, mode := range []PermissionMode{"auto", "ask", "plan"} {
		cfg := Config{Model: "test-model", PermissionMode: mode}
		prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeGeneral)

		if !strings.Contains(prompt, strings.ToUpper(string(mode))) {
			t.Errorf("prompt for mode=%q should contain %q", mode, strings.ToUpper(string(mode)))
		}
	}
}

func TestBuildSubAgentSystemPromptToolList(t *testing.T) {
	registry := tools.NewRegistry()
	cfg := Config{Model: "test-model", PermissionMode: "auto"}

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeGeneral)

	// Should contain Available Tools section
	if !strings.Contains(prompt, "## Available Tools") {
		t.Error("prompt should contain Available Tools section")
	}
}

// Suppress unused import warnings
var _ = tools.NewRegistry
