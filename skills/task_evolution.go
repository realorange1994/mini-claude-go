package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// CreateSkillFile writes a new SKILL.md file in the skills directory.
// This is the execution endpoint called by the auto-creation forked agent
// when it determines a new skill should be created.
func CreateSkillFile(skillsDir, skillName, content string) error {
	if skillsDir == "" {
		return fmt.Errorf("skills directory not configured")
	}
	if skillName == "" {
		return fmt.Errorf("skill name is required")
	}
	// Validate skill name (lowercase, hyphens, underscores, alphanumeric only)
	for _, c := range skillName {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return fmt.Errorf("invalid skill name %q: only lowercase letters, numbers, hyphens, and underscores allowed", skillName)
		}
	}

	skillDir := filepath.Join(skillsDir, skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write SKILL.md: %w", err)
	}

	return nil
}

// UpdateSkillFile updates an existing skill's SKILL.md file with new content.
// Used by the reflection system to improve existing skills.
func UpdateSkillFile(loader *Loader, skillName, content string) error {
	if loader == nil {
		return fmt.Errorf("skill loader is nil")
	}
	if skillName == "" {
		return fmt.Errorf("skill name is required")
	}

	// Try workspace skills directory first
	if wsDir := loader.WorkspaceDir(); wsDir != "" {
		skillDir := filepath.Join(wsDir, skillName)
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillPath); err == nil {
			return os.WriteFile(skillPath, []byte(content), 0o644)
		}
	}

	// Try builtin directory
	if builtinDir := loader.BuiltinDir(); builtinDir != "" {
		skillDir := filepath.Join(builtinDir, skillName)
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillPath); err == nil {
			return os.WriteFile(skillPath, []byte(content), 0o644)
		}
	}

	return fmt.Errorf("skill %q not found in any directory", skillName)
}
