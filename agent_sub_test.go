package main

import (
	"strings"
	"testing"

	"miniclaudecode-go/tools"
)

// testRegistryTool is a minimal Tool implementation for testing buildSubAgentRegistry.
type testRegistryTool struct {
	name string
}

func (m testRegistryTool) Name() string                              { return m.name }
func (m testRegistryTool) Description() string                       { return "" }
func (m testRegistryTool) InputSchema() map[string]any             { return nil }
func (m testRegistryTool) CheckPermissions(map[string]any) string  { return "" }
func (m testRegistryTool) Execute(map[string]any) tools.ToolResult {
	return tools.ToolResult{Output: ""}
}

// toolNames extracts tool names from a registry for assertion.
func toolNames(reg *tools.Registry) map[string]bool {
	names := make(map[string]bool)
	for _, t := range reg.AllTools() {
		names[t.Name()] = true
	}
	return names
}

func TestBuildSubAgentSystemPromptContainsSections(t *testing.T) {
	registry := tools.NewRegistry()
	cfg := Config{Model: "test-model", PermissionMode: "auto"}

	for _, agentType := range []AgentType{AgentTypeExplore, AgentTypePlan, AgentTypeVerify, AgentTypeGeneral} {
		prompt := buildSubAgentSystemPrompt(registry, cfg, agentType, "")

		// All prompts should contain key sections
		required := []string{
			"## Environment",
			"## Permission Mode",
			"## Available Tools",
			"Notes:",
		}
		for _, section := range required {
			if !strings.Contains(prompt, section) {
				t.Errorf("agentType=%q: prompt should contain %q", agentType, section)
			}
		}

		// Notes section should contain key behavioral rules
		if !strings.Contains(prompt, "absolute file paths") {
			t.Errorf("agentType=%q: prompt should contain absolute path rule in Notes", agentType)
		}
		if !strings.Contains(prompt, "MUST avoid using emojis") {
			t.Errorf("agentType=%q: prompt should contain no-emoji rule in Notes", agentType)
		}
		if !strings.Contains(prompt, "Do not use a colon before tool calls") {
			t.Errorf("agentType=%q: prompt should contain no-colon-before-tool-call rule in Notes", agentType)
		}
	}
}

func TestBuildSubAgentSystemPromptExploreReadOnly(t *testing.T) {
	registry := tools.NewRegistry()
	cfg := Config{Model: "test-model", PermissionMode: "auto"}

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeExplore, "")

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

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypePlan, "")

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

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeVerify, "")

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

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeGeneral, "")

	// General agent should have basic sections
	if !strings.Contains(prompt, "## Environment") {
		t.Error("General prompt should contain Environment section")
	}
	if !strings.Contains(prompt, "Notes:") {
		t.Error("General prompt should contain Notes section")
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

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeExplore, "")

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
		prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeGeneral, "")

		if !strings.Contains(prompt, strings.ToUpper(string(mode))) {
			t.Errorf("prompt for mode=%q should contain %q", mode, strings.ToUpper(string(mode)))
		}
	}
}

func TestBuildSubAgentSystemPromptToolList(t *testing.T) {
	registry := tools.NewRegistry()
	cfg := Config{Model: "test-model", PermissionMode: "auto"}

	prompt := buildSubAgentSystemPrompt(registry, cfg, AgentTypeGeneral, "")

	// Should contain Available Tools section
	if !strings.Contains(prompt, "## Available Tools") {
		t.Error("prompt should contain Available Tools section")
	}
}

// Suppress unused import warnings
var _ = tools.NewRegistry

// TestBuildSubAgentRegistryDisallowedOverridesAllowed verifies that when a tool
// appears in both allowed_tools and disallowed_tools, disallowed always wins.
// This is the core test for the "disallowed_tools 对 exec 未完全阻止" issue.
func TestBuildSubAgentRegistryDisallowedOverridesAllowed(t *testing.T) {
	// Create a parent registry with test tools
	parentRegistry := tools.NewRegistry()
	parentRegistry.Register(testRegistryTool{"read_file"})
	parentRegistry.Register(testRegistryTool{"exec"})
	parentRegistry.Register(testRegistryTool{"grep"})
	parentRegistry.Register(testRegistryTool{"write_file"})

	// Create AgentLoop with the parent registry
	loop := &AgentLoop{registry: parentRegistry}

	// allowed=["read_file", "exec"], disallowed=["exec"]
	// exec should NOT appear in the result because disallowed wins
	child := loop.buildSubAgentRegistry(AgentTypeGeneral, []string{"read_file", "exec"}, []string{"exec"}, false)

	names := toolNames(child)

	if names["exec"] {
		t.Error("exec should be excluded: it is in disallowed_tools, even though it is also in allowed_tools")
	}
	if !names["read_file"] {
		t.Error("read_file should be included: it is in allowed_tools and not disallowed")
	}

	// grep and write_file should be excluded because they are not in the allowed list
	if names["grep"] {
		t.Error("grep should be excluded: not in allowed_tools")
	}
	if names["write_file"] {
		t.Error("write_file should be excluded: not in allowed_tools")
	}
}

// TestBuildSubAgentRegistryAllowedOnly verifies that allowed_tools works as a
// whitelist when no disallowed_tools are specified.
func TestBuildSubAgentRegistryAllowedOnly(t *testing.T) {
	parentRegistry := tools.NewRegistry()
	parentRegistry.Register(testRegistryTool{"read_file"})
	parentRegistry.Register(testRegistryTool{"exec"})
	parentRegistry.Register(testRegistryTool{"grep"})

	loop := &AgentLoop{registry: parentRegistry}
	child := loop.buildSubAgentRegistry(AgentTypeGeneral, []string{"read_file", "exec"}, nil, false)

	names := toolNames(child)

	if !names["read_file"] {
		t.Error("read_file should be included")
	}
	if !names["exec"] {
		t.Error("exec should be included")
	}
	if names["grep"] {
		t.Error("grep should be excluded: not in allowed list")
	}
}

// TestBuildSubAgentRegistryDisallowedOnly verifies that disallowed_tools works
// as a deny list when no allowed_tools are specified.
func TestBuildSubAgentRegistryDisallowedOnly(t *testing.T) {
	parentRegistry := tools.NewRegistry()
	parentRegistry.Register(testRegistryTool{"read_file"})
	parentRegistry.Register(testRegistryTool{"exec"})
	parentRegistry.Register(testRegistryTool{"grep"})

	loop := &AgentLoop{registry: parentRegistry}
	child := loop.buildSubAgentRegistry(AgentTypeGeneral, nil, []string{"exec"}, false)

	names := toolNames(child)

	if names["exec"] {
		t.Error("exec should be excluded: in disallowed list")
	}
	if !names["read_file"] {
		t.Error("read_file should be included: not disallowed")
	}
	if !names["grep"] {
		t.Error("grep should be included: not disallowed")
	}
}

// TestBuildSubAgentRegistryWildcardAllowed verifies that allowed=["*"] means
// "all non-disallowed tools".
func TestBuildSubAgentRegistryWildcardAllowed(t *testing.T) {
	parentRegistry := tools.NewRegistry()
	parentRegistry.Register(testRegistryTool{"read_file"})
	parentRegistry.Register(testRegistryTool{"exec"})
	parentRegistry.Register(testRegistryTool{"grep"})

	loop := &AgentLoop{registry: parentRegistry}

	// Wildcard allows everything, but disallowed still applies
	child := loop.buildSubAgentRegistry(AgentTypeGeneral, []string{"*"}, []string{"exec"}, false)

	names := toolNames(child)

	if names["exec"] {
		t.Error("exec should be excluded even with wildcard allowed")
	}
	if !names["read_file"] || !names["grep"] {
		t.Error("read_file and grep should be included with wildcard")
	}
}

// TestBuildSubAgentRegistryMultipleDisallowedOverridesAllowed tests that multiple
// tools in the intersection are all excluded.
func TestBuildSubAgentRegistryMultipleDisallowedOverridesAllowed(t *testing.T) {
	parentRegistry := tools.NewRegistry()
	parentRegistry.Register(testRegistryTool{"read_file"})
	parentRegistry.Register(testRegistryTool{"exec"})
	parentRegistry.Register(testRegistryTool{"grep"})
	parentRegistry.Register(testRegistryTool{"write_file"})

	loop := &AgentLoop{registry: parentRegistry}

	// allowed=[read_file, exec, grep], disallowed=[exec, grep]
	child := loop.buildSubAgentRegistry(AgentTypeGeneral, []string{"read_file", "exec", "grep"}, []string{"exec", "grep"}, false)

	names := toolNames(child)

	if names["exec"] {
		t.Error("exec should be excluded")
	}
	if names["grep"] {
		t.Error("grep should be excluded")
	}
	if !names["read_file"] {
		t.Error("read_file should be included")
	}
	if names["write_file"] {
		t.Error("write_file should be excluded: not in allowed list")
	}
}
