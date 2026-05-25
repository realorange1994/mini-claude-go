package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// TaskEvolutionConfig holds configuration for skill evolution triggers.
type TaskEvolutionConfig struct {
	ReflectMinTurns    int    // min task iterations to trigger skill reflection (default: 5)
	AutoCreateMinTurns int    // min task iterations to trigger auto-creation (default: 12)
	SkillsDir          string // workspace skills directory for writing new skills
}

// RunSkillEvolution is the post-run entry point for the skill evolution system.
// Called from AgentLoop.Run() after the main loop exits (normal completion).
// Two mutually exclusive scenarios:
//
// A. Skill was used: Reflect on the executed skill and suggest improvements.
//    Triggered when: skillTracker.ReadCount() > taskStartReadCount && taskTurns >= ReflectMinTurns
//
// B. No skill was used + complex task: Auto-create a new skill from the workflow.
//    Triggered when: skillTracker.ReadCount() == taskStartReadCount && taskTurns >= AutoCreateMinTurns
//
// This mirrors openclacky's run_skill_evolution_hooks pattern: after Run() completes,
// check whether a skill was invoked. If yes, reflect; if no and the task was complex,
// consider auto-extracting a new skill.
func RunSkillEvolution(
	cfg TaskEvolutionConfig,
	skillTracker *SkillTracker,
	taskStartReadSkillCount int,
	taskTurns int,
	out func(format string, args ...interface{}),
) {
	if skillTracker == nil {
		return
	}

	skillsUsed := skillTracker.ReadCount() - taskStartReadSkillCount
	if skillsUsed > 0 {
		// Scenario A: Skill was used — reflect on improvements
		if taskTurns < cfg.ReflectMinTurns {
			return
		}
		runSkillReflection(cfg, skillTracker, out)
	} else {
		// Scenario B: No skill was used — consider auto-creating one
		if taskTurns < cfg.AutoCreateMinTurns {
			return
		}
		runSkillAutoCreation(cfg, taskTurns, out)
	}
}

// runSkillReflection analyzes skills that were used during the task and
// determines if improvements should be made. The full implementation would
// fork a subagent with restricted tools to perform the analysis and edit
// SKILL.md files. This version logs the reflection intent.
func runSkillReflection(
	cfg TaskEvolutionConfig,
	skillTracker *SkillTracker,
	out func(format string, args ...interface{}),
) {
	readSkillNames := skillTracker.GetReadSkillNames()
	if len(readSkillNames) == 0 {
		return
	}

	// Log skill names for now — the full implementation would fork a subagent
	_ = readSkillNames
	out("[skill-evolution] Reflecting on used skills\n")
}

// runSkillAutoCreation analyzes the complex task that just completed and
// determines if it should be captured as a new reusable skill. The full
// implementation would fork a subagent to analyze and create SKILL.md files.
// This version logs the analysis intent.
func runSkillAutoCreation(
	cfg TaskEvolutionConfig,
	taskTurns int,
	out func(format string, args ...interface{}),
) {
	out("[skill-evolution] Analyzing complex task (%d turns) for skill creation...\n", taskTurns)
}

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
