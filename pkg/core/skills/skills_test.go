package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "commit", false},
		{"valid with hyphens", "review-pr", false},
		{"valid with numbers", "test-123", false},
		{"too long", "a-very-long-name-that-exceeds-the-maximum-limit-of-sixty-four-characters-x", true},
		{"uppercase", "Commit", true},
		{"spaces", "my skill", true},
		{"starts with hyphen", "-skill", true},
		{"ends with hyphen", "skill-", true},
		{"double hyphens", "my--skill", true},
		{"underscores", "my_skill", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateName(tt.input)
			if tt.wantErr && len(errors) == 0 {
				t.Errorf("ValidateName(%q) should have errors", tt.input)
			}
			if !tt.wantErr && len(errors) > 0 {
				t.Errorf("ValidateName(%q) should not have errors, got: %v", tt.input, errors)
			}
		})
	}
}

func TestValidateDescription(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "Create a commit", false},
		{"empty", "", true},
		{"whitespace only", "   ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateDescription(tt.input)
			if tt.wantErr && len(errors) == 0 {
				t.Errorf("ValidateDescription(%q) should have errors", tt.input)
			}
			if !tt.wantErr && len(errors) > 0 {
				t.Errorf("ValidateDescription(%q) should not have errors, got: %v", tt.input, errors)
			}
		})
	}
}

func TestLoadSkillsFromDir_SKILLMD(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "commit")
	os.MkdirAll(skillDir, 0755)

	skillContent := `---
name: commit
description: Create a git commit
---
When asked to commit, follow these steps:
1. Run git status
2. Draft a commit message`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644)

	result := loadSkillsFromDir(tmpDir, "user")
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "commit" {
		t.Errorf("skill name = %q, want commit", result.Skills[0].Name)
	}
	if result.Skills[0].Description != "Create a git commit" {
		t.Errorf("skill description = %q, want 'Create a git commit'", result.Skills[0].Description)
	}
}

func TestLoadSkillsFromDir_RootMD(t *testing.T) {
	tmpDir := t.TempDir()
	skillContent := `---
name: my-skill
description: A root-level skill
---
Do something custom.`
	os.WriteFile(filepath.Join(tmpDir, "my-skill.md"), []byte(skillContent), 0644)
	result := loadSkillsFromDir(tmpDir, "project")
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "my-skill" {
		t.Errorf("skill name = %q, want my-skill", result.Skills[0].Name)
	}
}

func TestLoadSkillsFromDir_NoDescription(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "bad-skill")
	os.MkdirAll(skillDir, 0755)
	skillContent := `---
name: bad-skill
---
Some content without description.`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644)
	result := loadSkillsFromDir(tmpDir, "user")
	if len(result.Skills) != 0 {
		t.Errorf("skill without description should not be loaded, got %d", len(result.Skills))
	}
	hasDescWarning := false
	for _, d := range result.Diagnostics {
		if d.Message == "description is required" {
			hasDescWarning = true
		}
	}
	if !hasDescWarning {
		t.Error("should have description-is-required diagnostic")
	}
}

func TestLoadSkillsFromDir_DisableModelInvocation(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "hidden-skill")
	os.MkdirAll(skillDir, 0755)
	skillContent := `---
name: hidden-skill
description: A hidden skill
disable-model-invocation: true
---
Hidden content.`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644)
	result := loadSkillsFromDir(tmpDir, "user")
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if !result.Skills[0].DisableModelInvocation {
		t.Error("skill should have DisableModelInvocation=true")
	}
}

func TestLoadSkillsFromDir_NameFallback(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "fallback-name")
	os.MkdirAll(skillDir, 0755)
	skillContent := `---
description: Skill without explicit name
---
Some content.`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644)
	result := loadSkillsFromDir(tmpDir, "user")
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "fallback-name" {
		t.Errorf("skill name = %q, want fallback-name (parent dir)", result.Skills[0].Name)
	}
}

func TestLoadSkillsFromDir_NestedSKILLMD(t *testing.T) {
	tmpDir := t.TempDir()
	parentDir := filepath.Join(tmpDir, "skills")
	os.MkdirAll(filepath.Join(parentDir, "nested"), 0755)
	skillContent := `---
name: nested-skill
description: A nested skill
---
Nested content.`
	os.WriteFile(filepath.Join(parentDir, "nested", "SKILL.md"), []byte(skillContent), 0644)
	result := loadSkillsFromDir(parentDir, "user")
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(result.Skills))
	}
	if result.Skills[0].Name != "nested-skill" {
		t.Errorf("skill name = %q, want nested-skill", result.Skills[0].Name)
	}
}

func TestLoadSkills_CollisionDetection(t *testing.T) {
	tmpDir := t.TempDir()
	dir1 := filepath.Join(tmpDir, "dir1")
	os.MkdirAll(dir1, 0755)
	os.WriteFile(filepath.Join(dir1, "SKILL.md"), []byte(`---
name: commit
description: First commit skill
---
Content 1.`), 0644)
	dir2 := filepath.Join(tmpDir, "dir2")
	os.MkdirAll(dir2, 0755)
	os.WriteFile(filepath.Join(dir2, "SKILL.md"), []byte(`---
name: commit
description: Second commit skill
---
Content 2.`), 0644)
	result := LoadSkills(LoadSkillsOptions{
		SkillPaths:      []string{dir1, dir2},
		IncludeDefaults: false,
	})
	if len(result.Skills) != 1 {
		t.Errorf("should have 1 skill (winner), got %d", len(result.Skills))
	}
	hasCollision := false
	for _, d := range result.Diagnostics {
		if string(d.Type) == "collision" {
			hasCollision = true
		}
	}
	if !hasCollision {
		t.Error("should have collision diagnostic")
	}
}

func TestFormatSkillsForPrompt(t *testing.T) {
	skills := []Skill{
		{Name: "commit", Description: "Create a git commit", FilePath: "/path/to/commit/SKILL.md"},
		{Name: "review-pr", Description: "Review a pull request", FilePath: "/path/to/review-pr/SKILL.md"},
	}
	output := FormatSkillsForPrompt(skills)
	if !containsAll(output, []string{"<available_skills>", "<skill>", "<name>commit</name>", "<description>Create a git commit</description>", "<location>", "</available_skills>", "Use the read tool to load"}) {
		t.Errorf("FormatSkillsForPrompt output missing expected XML tags")
	}
}

func TestFormatSkillsForPrompt_DisableModelInvocation(t *testing.T) {
	skills := []Skill{
		{Name: "commit", Description: "Create a commit", FilePath: "/path/commit"},
		{Name: "hidden", Description: "Hidden skill", FilePath: "/path/hidden", DisableModelInvocation: true},
	}
	output := FormatSkillsForPrompt(skills)
	if contains(output, "<name>hidden</name>") {
		t.Error("skills with DisableModelInvocation should not appear in prompt")
	}
	if !contains(output, "<name>commit</name>") {
		t.Error("visible skills should appear in prompt")
	}
}

func TestFormatSkillsForPrompt_Empty(t *testing.T) {
	output := FormatSkillsForPrompt(nil)
	if output != "" {
		t.Errorf("empty skills should produce empty output, got: %s", output)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

func containsAll(s string, substrs []string) bool {
	for _, substr := range substrs {
		if !strings.Contains(s, substr) {
			return false
		}
	}
	return true
}