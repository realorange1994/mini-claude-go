package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSkillContent = `---
name: test-skill
description: A test skill for unit testing
always: false
available: true
commands: ["/test"]
tags: [test]
version: 1.0.0
requires:
  - GIT
---

This is the body of the test skill.
It contains instructions for the AI.
`

func TestParseFrontmatter(t *testing.T) {
	meta := parseFrontmatter(testSkillContent)

	if meta.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got: %q", meta.Name)
	}
	if meta.Description != "A test skill for unit testing" {
		t.Errorf("unexpected description: %q", meta.Description)
	}
	if meta.Always {
		t.Error("expected always=false")
	}
	if !meta.Available {
		t.Error("expected available=true")
	}
	if len(meta.Commands) != 1 || meta.Commands[0] != "/test" {
		t.Errorf("unexpected commands: %v", meta.Commands)
	}
	if len(meta.Tags) != 1 || meta.Tags[0] != "test" {
		t.Errorf("unexpected tags: %v", meta.Tags)
	}
	if meta.Version != "1.0.0" {
		t.Errorf("unexpected version: %q", meta.Version)
	}
	if len(meta.Requires) != 1 || meta.Requires[0] != "GIT" {
		t.Errorf("unexpected requires: %v", meta.Requires)
	}
}

func TestParseFrontmatterNoFrontmatter(t *testing.T) {
	meta := parseFrontmatter("Just plain markdown")
	if meta.Name != "" {
		t.Error("expected empty meta for content without frontmatter")
	}
}

func TestParseFrontmatterMissingEnd(t *testing.T) {
	meta := parseFrontmatter("---\nname: test\nno closing delimiter")
	if meta.Name != "" {
		t.Error("expected empty meta when end delimiter is missing")
	}
}

func TestParseInlineList(t *testing.T) {
	tests := []struct {
		input  string
		expect []string
	}{
		{"[a, b, c]", []string{"a", "b", "c"}},
		{"[\"x\", \"y\"]", []string{"x", "y"}},
		{"[]", nil},
		{"not a list", nil},
	}

	for _, tc := range tests {
		result := parseInlineList(tc.input)
		if len(result) != len(tc.expect) {
			t.Errorf("parseInlineList(%q) = %v, want %v", tc.input, result, tc.expect)
			continue
		}
		for i := range result {
			if result[i] != tc.expect[i] {
				t.Errorf("parseInlineList(%q)[%d] = %q, want %q", tc.input, i, result[i], tc.expect[i])
			}
		}
	}
}

func TestUnquote(t *testing.T) {
	if unquote(`"hello"`) != "hello" {
		t.Error("unquote failed for double-quoted string")
	}
	if unquote(`'hello'`) != "hello" {
		t.Error("unquote failed for single-quoted string")
	}
	if unquote("hello") != "hello" {
		t.Error("unquote failed for unquoted string")
	}
}

func TestStripFrontmatter(t *testing.T) {
	result := stripFrontmatter(testSkillContent)
	if !strings.Contains(result, "This is the body") {
		t.Errorf("expected body content in stripped result: %s", result)
	}
	if strings.HasPrefix(result, "---") {
		t.Error("expected frontmatter to be stripped")
	}
}

func TestLoaderLoadSkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testSkillContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	content := loader.LoadSkill("test-skill")
	if content == "" {
		t.Fatal("expected skill content, got empty")
	}
	if !strings.Contains(content, "A test skill for unit testing") {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestLoaderLoadSkillNotFound(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(dir)
	_ = loader.Refresh()

	content := loader.LoadSkill("nonexistent")
	if content != "" {
		t.Errorf("expected empty content, got: %s", content)
	}
}

func TestLoaderListSkills(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testSkillContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	skills := loader.ListSkills(false)
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got: %d", len(skills))
	}

	if skills[0].Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got: %q", skills[0].Name)
	}
}

func TestLoaderBuildSkillsSummary(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testSkillContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	summary := loader.BuildSkillsSummary()
	if !strings.Contains(summary, "<skills>") {
		t.Errorf("expected skills xml tag: %s", summary)
	}
	if !strings.Contains(summary, "test-skill") {
		t.Errorf("expected skill name in summary: %s", summary)
	}
}

func TestLoaderNoSkillsDir(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(dir)
	err := loader.Refresh()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	skills := loader.ListSkills(false)
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got: %d", len(skills))
	}
}

func TestLoaderLoadSkillsForContext(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testSkillContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	content := loader.LoadSkillsForContext([]string{"test-skill"})
	if !strings.Contains(content, "### Skill: test-skill") {
		t.Errorf("expected skill header: %s", content)
	}
	if !strings.Contains(content, "body of the test skill") {
		t.Errorf("expected skill body: %s", content)
	}
}

func TestParseBool(t *testing.T) {
	if !parseBool("true") {
		t.Error("parseBool(true) should be true")
	}
	if !parseBool("yes") {
		t.Error("parseBool(yes) should be true")
	}
	if !parseBool("True") {
		t.Error("parseBool(True) should be true")
	}
	if parseBool("false") {
		t.Error("parseBool(false) should be false")
	}
	if parseBool("no") {
		t.Error("parseBool(no) should be false")
	}
}

func TestLoaderGetAlwaysSkills(t *testing.T) {
	dir := t.TempDir()

	alwaysContent := `---
name: always-skill
description: Always on skill
always: true
available: true
---

Always skill body.
`

	skillDir := filepath.Join(dir, "skills", "always-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(alwaysContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	alwaysSkills := loader.GetAlwaysSkills()
	if len(alwaysSkills) != 1 {
		t.Fatalf("expected 1 always skill, got: %d", len(alwaysSkills))
	}
	if alwaysSkills[0].Name != "always-skill" {
		t.Errorf("expected 'always-skill', got: %q", alwaysSkills[0].Name)
	}
}
