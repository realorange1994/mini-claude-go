package tools

import (
	"strings"
	"testing"

	"miniclaudecode-go/skills"
)

// ─── ReadSkillTool ──────────────────────────────────────────────────────────

func TestReadSkillToolName(t *testing.T) {
	tool := &ReadSkillTool{}
	if tool.Name() != "read_skill" {
		t.Errorf("expected 'read_skill', got %q", tool.Name())
	}
}

func TestReadSkillToolSchema(t *testing.T) {
	tool := &ReadSkillTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "name" {
		t.Errorf("expected required=[name], got %v", required)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["name"]; !ok {
		t.Error("schema should have name property")
	}
}

func TestReadSkillToolPermissions(t *testing.T) {
	tool := &ReadSkillTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestReadSkillToolExecuteNoName(t *testing.T) {
	tool := &ReadSkillTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing name should return error")
	}
}

func TestReadSkillToolExecuteNilLoader(t *testing.T) {
	tool := &ReadSkillTool{}
	result := tool.Execute(map[string]any{"name": "test"})
	if !result.IsError {
		t.Error("nil loader should return error")
	}
}

// ─── ListSkillsTool ─────────────────────────────────────────────────────────

func TestListSkillsToolName(t *testing.T) {
	tool := &ListSkillsTool{}
	if tool.Name() != "list_skills" {
		t.Errorf("expected 'list_skills', got %q", tool.Name())
	}
}

func TestListSkillsToolSchema(t *testing.T) {
	tool := &ListSkillsTool{}
	schema := tool.InputSchema()
	props, _ := schema["properties"].(map[string]any)
	if len(props) != 0 {
		t.Logf("list_skills props: %v", props)
	}
}

func TestListSkillsToolPermissions(t *testing.T) {
	tool := &ListSkillsTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestListSkillsToolExecuteNilLoader(t *testing.T) {
	tool := &ListSkillsTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("nil loader should return error")
	}
}

// ─── SearchSkillsTool ───────────────────────────────────────────────────────

func TestSearchSkillsToolName(t *testing.T) {
	tool := &SearchSkillsTool{}
	if tool.Name() != "search_skills" {
		t.Errorf("expected 'search_skills', got %q", tool.Name())
	}
}

func TestSearchSkillsToolSchema(t *testing.T) {
	tool := &SearchSkillsTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "query" {
		t.Errorf("expected required=[query], got %v", required)
	}
}

func TestSearchSkillsToolPermissions(t *testing.T) {
	tool := &SearchSkillsTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestSearchSkillsToolExecuteNoQuery(t *testing.T) {
	tool := &SearchSkillsTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing query should return error")
	}
}

func TestSearchSkillsToolExecuteNilLoader(t *testing.T) {
	tool := &SearchSkillsTool{}
	result := tool.Execute(map[string]any{"query": "test"})
	if !result.IsError {
		t.Error("nil loader should return error")
	}
}

// ─── scoreSkill ─────────────────────────────────────────────────────────────

func TestScoreSkillExactNameMatch(t *testing.T) {
	info := skills.SkillInfo{Name: "commit", Description: "Create a commit"}
	score := scoreSkill(info, []string{"commit"})
	if score < 90 {
		t.Errorf("exact name match should score high, got %f", score)
	}
}

func TestScoreSkillNameContains(t *testing.T) {
	info := skills.SkillInfo{Name: "commit_push", Description: "Push commits"}
	score := scoreSkill(info, []string{"commit"})
	if score <= 0 {
		t.Errorf("name contains should score > 0, got %f", score)
	}
}

func TestScoreSkillDescriptionMatch(t *testing.T) {
	info := skills.SkillInfo{Name: "git", Description: "Create a git commit"}
	score := scoreSkill(info, []string{"commit"})
	if score <= 0 {
		t.Errorf("description match should score > 0, got %f", score)
	}
}

func TestScoreSkillTagMatch(t *testing.T) {
	info := skills.SkillInfo{Name: "git", Tags: []string{"commit", "vcs"}}
	score := scoreSkill(info, []string{"commit"})
	if score <= 0 {
		t.Errorf("tag match should score > 0, got %f", score)
	}
}

func TestScoreSkillWhenToUseMatch(t *testing.T) {
	info := skills.SkillInfo{Name: "git", WhenToUse: "Use when you need to commit changes"}
	score := scoreSkill(info, []string{"commit"})
	if score <= 0 {
		t.Errorf("when_to_use match should score > 0, got %f", score)
	}
}

func TestScoreSkillNoMatch(t *testing.T) {
	info := skills.SkillInfo{Name: "git", Description: "Git operations"}
	score := scoreSkill(info, []string{"docker"})
	if score > 0 {
		t.Errorf("no match should score 0, got %f", score)
	}
}

func TestScoreSkillMultipleTerms(t *testing.T) {
	info := skills.SkillInfo{Name: "commit", Description: "Create a git commit", Tags: []string{"vcs"}}
	score := scoreSkill(info, []string{"commit", "git"})
	if score <= 100 {
		t.Errorf("multiple matching terms should score > 100, got %f", score)
	}
}

// ─── formatSearchResults ────────────────────────────────────────────────────

func TestFormatSearchResultsEmpty(t *testing.T) {
	result := formatSearchResults(nil, "test")
	if result == "" {
		t.Error("empty results should still return a message")
	}
}

func TestFormatSearchResultsWithMatches(t *testing.T) {
	scored := []scoredSkill{
		{info: skills.SkillInfo{Name: "commit", Description: "Create commit", Available: true}, score: 100},
		{info: skills.SkillInfo{Name: "push", Description: "Push to remote", Available: true}, score: 50},
	}
	result := formatSearchResults(scored, "commit")
	if result == "" {
		t.Error("should format results")
	}
	if !strings.Contains(result, "commit") {
		t.Error("result should contain skill name")
	}
}

func TestFormatSearchResultsUnavailable(t *testing.T) {
	scored := []scoredSkill{
		{info: skills.SkillInfo{Name: "commit", Description: "Create commit", Available: false}, score: 50},
	}
	result := formatSearchResults(scored, "commit")
	if !strings.Contains(result, "unavailable") {
		t.Error("unavailable skill should be marked")
	}
}

func TestFormatSearchResultsWithTags(t *testing.T) {
	scored := []scoredSkill{
		{info: skills.SkillInfo{Name: "commit", Description: "Create commit", Tags: []string{"git", "vcs"}}, score: 100},
	}
	result := formatSearchResults(scored, "commit")
	if !strings.Contains(result, "git") {
		t.Error("result should contain tags")
	}
}

// ─── searchSkillCharBudget ──────────────────────────────────────────────────

func TestSearchSkillCharBudget(t *testing.T) {
	if searchSkillCharBudget != 4000 {
		t.Errorf("expected budget 4000, got %d", searchSkillCharBudget)
	}
}
