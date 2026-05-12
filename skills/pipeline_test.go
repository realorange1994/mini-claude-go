package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Argument Substitution Tests ---

func TestExpandSkillContent(t *testing.T) {
	// Test $ARGUMENTS replacement
	content := "Run the command: $ARGUMENTS"
	args := map[string]string{"ARGUMENTS": "ls -la"}
	result := ExpandSkillContent(content, args)
	if result != "Run the command: ls -la" {
		t.Errorf("expected 'Run the command: ls -la', got: %q", result)
	}

	// Test $ARG_NAME replacement
	content = "File: $ARG_FILE, Line: $ARG_LINE"
	args = map[string]string{"ARG_FILE": "main.go", "ARG_LINE": "42"}
	result = ExpandSkillContent(content, args)
	if result != "File: main.go, Line: 42" {
		t.Errorf("expected 'File: main.go, Line: 42', got: %q", result)
	}

	// Test both together
	content = "Target: $ARG_TARGET, Args: $ARGUMENTS"
	args = map[string]string{"ARG_TARGET": "test", "ARGUMENTS": "--verbose"}
	result = ExpandSkillContent(content, args)
	if result != "Target: test, Args: --verbose" {
		t.Errorf("expected 'Target: test, Args: --verbose', got: %q", result)
	}

	// Test no replacement needed
	content = "No variables here"
	args = map[string]string{"ARG_X": "y"}
	result = ExpandSkillContent(content, args)
	if result != "No variables here" {
		t.Errorf("expected 'No variables here', got: %q", result)
	}
}

func TestExpandSkillContentEmptyArgs(t *testing.T) {
	content := "$ARGUMENTS and $ARG_FILE"
	result := ExpandSkillContent(content, nil)
	// Nothing should be replaced with nil args
	if result != content {
		t.Errorf("expected unchanged content, got: %q", result)
	}
}

// --- Variable Expansion Tests ---

func TestExpandSkillVariables(t *testing.T) {
	content := "Dir: ${CLAUDE_SKILL_DIR}, Session: ${CLAUDE_SESSION_ID}, Project: ${CLAUDE_PROJECT_DIR}, Config: ${CLAUDE_CONFIG_DIR}"
	result := ExpandSkillVariables(content, "/skill/dir", "sess-123")

	// Check that replacements happened
	if !strings.Contains(result, "/skill/dir") {
		t.Errorf("expected skill dir in result: %q", result)
	}
	if !strings.Contains(result, "sess-123") {
		t.Errorf("expected session id in result: %q", result)
	}
	// CLAUDE_PROJECT_DIR should be non-empty (current working dir)
	if !strings.Contains(result, "/") {
		t.Errorf("expected project dir in result: %q", result)
	}
	// CLAUDE_CONFIG_DIR should end with .claude
	if !strings.Contains(result, ".claude") {
		t.Errorf("expected config dir in result: %q", result)
	}
}

func TestExpandSkillVariablesNoReplacement(t *testing.T) {
	content := "No variables to replace here"
	result := ExpandSkillVariables(content, "/dir", "sess-1")
	if result != "No variables to replace here" {
		t.Errorf("expected no changes, got: %q", result)
	}
}

func TestExpandSkillVariablesPartial(t *testing.T) {
	content := "Has ${CLAUDE_SKILL_DIR} but also ${UNKNOWN_VAR}"
	result := ExpandSkillVariables(content, "/my/skill/dir", "sess-1")
	if !strings.Contains(result, "/my/skill/dir") {
		t.Errorf("expected skill dir replaced: %q", result)
	}
	if !strings.Contains(result, "${UNKNOWN_VAR}") {
		t.Errorf("expected ${UNKNOWN_VAR} to remain: %q", result)
	}
}

func TestGetProjectDir(t *testing.T) {
	dir := getProjectDir()
	if dir == "" {
		t.Error("expected non-empty project dir")
	}
}

func TestGetConfigDir(t *testing.T) {
	dir := getConfigDir()
	if dir == "" {
		t.Error("expected non-empty config dir")
	}
	if !strings.HasSuffix(dir, ".claude") {
		t.Errorf("expected config dir to end with .claude, got: %q", dir)
	}
}

// --- Conditional Paths Tests ---

func TestSkillInfoIsApplicable(t *testing.T) {
	// No paths = always applicable
	skill := SkillInfo{Name: "test"}
	if !skill.IsApplicable("/any/project") {
		t.Error("skill without paths should be applicable everywhere")
	}

	// Exact match
	skill = SkillInfo{Name: "test", Paths: []string{"/specific/project"}}
	if !skill.IsApplicable("/specific/project") {
		t.Error("skill should apply to exact path match")
	}
	if skill.IsApplicable("/different/project") {
		t.Error("skill should not apply to non-matching path")
	}

	// Wildcard pattern matching base name
	skill = SkillInfo{Name: "test", Paths: []string{"my-project"}}
	if !skill.IsApplicable("/some/path/my-project") {
		t.Error("skill should apply when base name matches pattern")
	}
	if skill.IsApplicable("/some/path/other-project") {
		t.Error("skill should not apply when base name does not match")
	}

	// Wildcard with glob pattern
	skill = SkillInfo{Name: "test", Paths: []string{"*-project"}}
	if !skill.IsApplicable("/some/path/my-project") {
		t.Error("skill should apply with wildcard pattern match on base")
	}

	// Empty paths list
	skill = SkillInfo{Name: "test", Paths: []string{}}
	if !skill.IsApplicable("/any/project") {
		t.Error("skill with empty paths should be applicable everywhere")
	}

	// Multiple patterns, second matches
	skill = SkillInfo{Name: "test", Paths: []string{"project-a", "project-b"}}
	if !skill.IsApplicable("/path/project-b") {
		t.Error("skill should match any pattern in paths list")
	}
}

func TestListSkillsForProject(t *testing.T) {
	dir := t.TempDir()

	// Create an unrestricted skill
	unrestrictedContent := `---
name: universal-skill
description: Available everywhere
---

Universal skill body.
`
	skillDir1 := filepath.Join(dir, "skills", "universal-skill")
	os.MkdirAll(skillDir1, 0755)
	os.WriteFile(filepath.Join(skillDir1, "SKILL.md"), []byte(unrestrictedContent), 0644)

	// Create a restricted skill (only for my-project)
	restrictedContent := `---
name: project-skill
description: Only for my-project
paths: [my-project]
---

Project-specific skill body.
`
	skillDir2 := filepath.Join(dir, "skills", "project-skill")
	os.MkdirAll(skillDir2, 0755)
	os.WriteFile(filepath.Join(skillDir2, "SKILL.md"), []byte(restrictedContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	// In a project matching the restricted skill
	matchingSkills := loader.ListSkillsForProject("/some/path/my-project", false)
	if len(matchingSkills) != 2 {
		t.Fatalf("expected 2 skills in my-project, got: %d", len(matchingSkills))
	}

	// In a different project (restricted skill should not appear)
	nonMatchingSkills := loader.ListSkillsForProject("/some/path/other-project", false)
	if len(nonMatchingSkills) != 1 {
		t.Fatalf("expected 1 skill in other-project, got: %d", len(nonMatchingSkills))
	}
	if nonMatchingSkills[0].Name != "universal-skill" {
		t.Errorf("expected universal-skill, got: %q", nonMatchingSkills[0].Name)
	}
}

// --- MCP Skill Discovery Tests ---

func TestMCPSkillDiscoveryNilClient(t *testing.T) {
	// Test with nil client
	d := NewMCPSkillDiscovery(nil)
	skills := d.DiscoverSkills(context.Background())
	if len(skills) != 0 {
		t.Errorf("expected 0 skills with nil client, got: %d", len(skills))
	}

	// Test LoadSkillContent with nil client
	content := d.LoadSkillContent("test")
	if content != "" {
		t.Errorf("expected empty content with nil client, got: %q", content)
	}
}

func TestMCPSkillDiscovery(t *testing.T) {
	// Test with mock client
	mock := &mockMCPClient{
		resources: []MCPSkillResource{
			{
				Server:      "test-server",
				URI:         "skill://code-review",
				Name:        "code-review",
				Description: "Review code changes",
				Content:     "Review the code carefully.",
			},
			{
				Server:      "test-server",
				URI:         "tool://something",
				Name:        "something",
				Description: "Not a skill",
				Content:     "This is not a skill.",
			},
			{
				Server:      "other-server",
				URI:         "skill://formatting",
				Name:        "", // No name, should use URI path
				Description: "Format code",
				Content:     "Format the code properly.",
			},
		},
	}

	d := NewMCPSkillDiscovery(mock)
	skills := d.DiscoverSkills(context.Background())
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got: %d", len(skills))
	}

	// Check first skill
	if skills[0].Name != "code-review" {
		t.Errorf("expected name 'code-review', got: %q", skills[0].Name)
	}
	if skills[0].Source != "mcp:test-server" {
		t.Errorf("expected source 'mcp:test-server', got: %q", skills[0].Source)
	}

	// Check LoadSkillContent
	content := d.LoadSkillContent("code-review")
	if content != "Review the code carefully." {
		t.Errorf("expected skill content, got: %q", content)
	}

	content = d.LoadSkillContent("nonexistent")
	if content != "" {
		t.Errorf("expected empty content for nonexistent skill, got: %q", content)
	}
}

// mockMCPClient implements MCPResourceClient for testing.
type mockMCPClient struct {
	resources []MCPSkillResource
}

func (m *mockMCPClient) DiscoverSkillResources(ctx context.Context) []MCPSkillResource {
	_ = ctx
	return m.resources
}

// --- Frontmatter Paths Parsing Tests ---

func TestParseFrontmatterPaths(t *testing.T) {
	// Test inline paths
	content := `---
name: path-skill
description: A skill with paths
paths: [my-project, another-project]
---

Path skill body.
`
	meta := parseFrontmatter(content)
	if meta.Name != "path-skill" {
		t.Errorf("expected name 'path-skill', got: %q", meta.Name)
	}
	if len(meta.Paths) != 2 {
		t.Fatalf("expected 2 paths, got: %d", len(meta.Paths))
	}
	if meta.Paths[0] != "my-project" {
		t.Errorf("expected first path 'my-project', got: %q", meta.Paths[0])
	}
	if meta.Paths[1] != "another-project" {
		t.Errorf("expected second path 'another-project', got: %q", meta.Paths[1])
	}
}

func TestParseFrontmatterPathsMultiLine(t *testing.T) {
	content := `---
name: multiline-paths
description: Paths in multiline
paths:
  - project-a
  - project-b
  - project-c
---

Multiline path skill body.
`
	meta := parseFrontmatter(content)
	if len(meta.Paths) != 3 {
		t.Fatalf("expected 3 paths, got: %d", len(meta.Paths))
	}
	if meta.Paths[0] != "project-a" {
		t.Errorf("expected 'project-a', got: %q", meta.Paths[0])
	}
	if meta.Paths[1] != "project-b" {
		t.Errorf("expected 'project-b', got: %q", meta.Paths[1])
	}
	if meta.Paths[2] != "project-c" {
		t.Errorf("expected 'project-c', got: %q", meta.Paths[2])
	}
}

func TestParseFrontmatterPathsEmpty(t *testing.T) {
	content := `---
name: no-paths
description: No paths defined
---

Body.
`
	meta := parseFrontmatter(content)
	if meta.Paths != nil {
		t.Errorf("expected nil paths, got: %v", meta.Paths)
	}
}

func TestMCPManagerAdapter(t *testing.T) {
	// Test nil adapter
	var a *MCPManagerAdapter
	resources := a.DiscoverSkillResources(context.Background())
	if len(resources) != 0 {
		t.Errorf("expected 0 from nil adapter, got: %d", len(resources))
	}

	// Test adapter with nil discoverFn
	a = NewMCPManagerAdapter(nil, nil)
	resources = a.DiscoverSkillResources(context.Background())
	if len(resources) != 0 {
		t.Errorf("expected 0 from adapter with nil fn, got: %d", len(resources))
	}

	// Test adapter with working discoverFn
	expected := []MCPSkillResource{
		{Server: "test", URI: "skill://foo", Name: "foo", Description: "test", Content: "content"},
	}
	a = NewMCPManagerAdapter(nil, func(ctx context.Context) []MCPSkillResource {
		return expected
	})
	resources = a.DiscoverSkillResources(context.Background())
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got: %d", len(resources))
	}
	if resources[0].Name != "foo" {
		t.Errorf("expected name 'foo', got: %q", resources[0].Name)
	}
}
