package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// runMemoryUpdate runs after a qualifying task to persist important knowledge
// into session memory. This mirrors openclacky's run_memory_update_subagent pattern.
//
// Trigger condition:
//   - Task iterations >= MemoryUpdateMinTurns (default: 10)
//   - Task was not interrupted
//   - Not a subagent
//
// The forked subagent analyzes the conversation and decides what knowledge
// is worth persisting, following a whitelist/blacklist decision model.
func (a *AgentLoop) runMemoryUpdate(taskTurns int) {
	if !a.config.MemoryUpdateEnabled {
		return
	}
	if taskTurns < a.config.MemoryUpdateMinTurns {
		return
	}
	if a.IsInterrupted() {
		return
	}
	sm := a.config.SessionMemory
	if sm == nil {
		return
	}

	a.out("[memory-update] Task completed (%d turns), analyzing for knowledge persistence...\n", taskTurns)

	// Capture cache-safe params from current state for the forked agent
	messages := a.context.BuildMessages()
	messages = NormalizeAPIMessages(messages)
	cacheParams := CaptureCacheSafeParams(
		a.context.SystemPrompt(),
		a.config.Model,
		a.registry,
		messages,
	)

	// Read current session memory content
	memoryPath := filepath.Join(a.config.ProjectDir, ".claude", "session_memory.md")
	currentContent, _ := os.ReadFile(memoryPath)
	if len(currentContent) == 0 {
		currentContent = []byte(defaultSessionMemoryTemplate)
	}

	// Build the memory update prompt
	prompt := buildMemoryUpdatePrompt(string(currentContent), sm.FormatForPrompt())

	forkMessages := []anthropic.MessageParam{
		{
			Role:    anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{{OfText: &anthropic.TextBlockParam{Text: prompt}}},
		},
	}

	// Restrict tools for the forked agent: only memory_add, memory_search, and read_file
	canUseTool := createMemoryUpdateCanUseTool()

	cfg := ForkedAgentConfig{
		CacheSafeParams: cacheParams,
		ForkMessages:    forkMessages,
		CanUseTool:      canUseTool,
		MaxTokens:       8192,
		QuerySource:     "memory_update",
		MaxTurns:        5,
		Registry:        a.registry,
		ProjectDir:      a.config.ProjectDir,
		Client:          a.client,
	}

	_, err := RunForkedAgent(cfg)
	if err != nil {
		a.logDebug("[memory-update] forked agent error: %v\n", err)
		return
	}

	a.out("[memory-update] Knowledge persistence complete.\n")
}

// buildMemoryUpdatePrompt constructs the memory update prompt with whitelist/blacklist.
func buildMemoryUpdatePrompt(currentContent, formattedMemory string) string {
	return fmt.Sprintf(`═══════════════════════════════════════════════════════════════
MEMORY UPDATE MODE
═══════════════════════════════════════════════════════════════
The conversation above has ended. You are now in MEMORY UPDATE MODE.

## Default: Do NOT write anything.

Memory writes are expensive. Only write if the session contains at least one of the
following high-value signals. If NONE apply, respond immediately with:
"No memory updates needed." and STOP — do not use any tools.

## Whitelist: Write ONLY if at least one condition is met

1. **Explicit decision** — The user made a clear technical, product, or process decision
   that will affect future work (e.g. "we'll use X instead of Y going forward").
2. **New persistent context** — The user introduced project background, constraints, or
   goals that are not already obvious from the code (e.g. a new feature direction,
   a deployment target, a team convention).
3. **Correction of prior knowledge** — The user corrected a previous misunderstanding
   or the agent discovered that an existing memory is wrong or outdated.
4. **Stated preference** — The user expressed a clear personal or team preference about
   how they want the agent to behave, communicate, or write code.

## What does NOT qualify (skip these entirely)

- Running tests, fixing lint, formatting code
- Committing, deploying, or releasing
- Answering a one-off question or explaining a concept
- Any task that produced no lasting decisions or preferences
- Repeating or slightly rephrasing what is already in memory

## Current Session Memory

%s

## Current Formatted Memory (for reference)

%s

## Action

Use the memory_add tool to persist any knowledge that qualifies under the whitelist.
Use categories: 'preference', 'decision', 'state', or 'reference'.
Be concise — each memory entry will be read on future conversations.

If nothing qualifies, respond: "No memory updates needed."`,
		truncateForPrompt(currentContent, 5000),
		truncateForPrompt(formattedMemory, 5000),
	)
}

// createMemoryUpdateCanUseTool creates a CanUseToolFn for the memory update forked agent.
// Only memory_add, memory_search, and read_file are allowed.
func createMemoryUpdateCanUseTool() CanUseToolFn {
	allowed := map[string]bool{
		"memory_add":    true,
		"memory_search": true,
		"read_file":     true,
		"glob":          true,
	}

	return func(toolName string, args map[string]any) (bool, string) {
		if allowed[toolName] {
			return true, ""
		}
		return false, fmt.Sprintf("Tool %q is not available during memory update — only memory_add, memory_search, read_file, and glob are allowed", toolName)
	}
}

// truncateForPrompt truncates content to maxChars for inclusion in prompts.
func truncateForPrompt(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars] + "\n... [truncated]"
}

// BuildSkillsReflectionPrompt creates a prompt for reflecting on skills used in the task.
func BuildSkillsReflectionPrompt(skillNames []string, taskTurns int) string {
	if len(skillNames) == 0 {
		return ""
	}

	skillList := strings.Join(skillNames, ", ")
	return fmt.Sprintf(`═══════════════════════════════════════════════════════════════
SKILL REFLECTION MODE
═══════════════════════════════════════════════════════════════
You just executed the following skill(s) over %d turns: %s

## Quick Analysis

Reflect on whether the skill(s) could be improved:
- Were the instructions clear enough?
- Did you encounter any edge cases not covered?
- Were there any steps that could be streamlined?
- Is there missing context that would make it easier next time?
- Did the skill(s) produce the expected results?

## Decision

If you identified **concrete, actionable improvements** to any skill:
→ Edit the corresponding SKILL.md file to incorporate the improvements.

If everything worked well as-is:
→ Respond briefly: "Skills worked well, no improvements needed."

## Constraints

- Be specific and actionable in your improvement suggestions
- Only suggest improvements that would make a meaningful difference
- If you're unsure, err on the side of "no improvements needed"`, taskTurns, skillList)
}

// BuildSkillAutoCreatePrompt creates a prompt for analyzing whether a complex
// task should be captured as a new skill.
func BuildSkillAutoCreatePrompt(taskTurns int) string {
	return fmt.Sprintf(`═══════════════════════════════════════════════════════════════
SKILL AUTO-CREATION MODE
═══════════════════════════════════════════════════════════════
You just completed a complex task (%d turns) without using any existing skill.

## Analysis

Review the conversation history and determine:
- Is this workflow likely to be reused in similar future tasks?
- Does it have a clear input → process → output pattern?
- Would it save significant time if automated as a skill?

## Decision Criteria (ALL must be true)

1. **Reusable**: The workflow could apply to similar tasks in the future
   (not a one-off, project-specific task)
2. **Well-defined**: Clear steps with consistent logic, not just exploratory conversation
3. **Valuable**: Would save more than 5 minutes of work if reused
4. **Generalizable**: Can be parameterized for different inputs/contexts

## Action

If **ALL** criteria are met:
→ Create a new SKILL.md file in the workspace skills directory.

If **NOT all** criteria are met:
→ Respond briefly: "This task doesn't warrant a new skill." (no file writes)

## Constraints

- Be selective: Don't create skills for one-off tasks
- Be specific: Clearly describe the workflow steps
- Keep it simple: Focus on the core happy path
- Prefer generalization: The skill should work across different contexts

## SKILL.md Format

---
name: skill-name
description: Brief description
tags: [tag1, tag2]
when_to_use: When this skill should be used
---

# Instructions
...step-by-step instructions...`, taskTurns)
}

// runSkillEvolutionIntegration runs the skill evolution system after a task completes.
// This is the main integration point called from AgentLoop.Run().
func (a *AgentLoop) runSkillEvolutionIntegration(taskTurns int) {
	if !a.config.SkillEvolutionEnabled {
		return
	}
	if a.skillTracker == nil {
		return
	}

	skillsUsed := a.skillTracker.ReadCount() - a.taskStartReadSkillCount
	if skillsUsed > 0 {
		// Scenario A: Skill was used — reflect on improvements
		if taskTurns < 5 { // ReflectMinTurns hardcoded to 5 (matches openclacky MIN_SKILL_ITERATIONS)
			return
		}
		a.runSkillReflection()
	} else {
		// Scenario B: No skill was used — consider auto-creating one
		autoCreateThreshold := max(a.config.SkillEvolutionMinTurns, 12)
		if taskTurns < autoCreateThreshold {
			return
		}
		a.runSkillAutoCreation(taskTurns)
	}
}

// runSkillReflection forks a sub-agent to analyze skills used during the task
// and determine if improvements should be made to their SKILL.md files.
func (a *AgentLoop) runSkillReflection() {
	readSkillNames := a.skillTracker.GetReadSkillNames()
	if len(readSkillNames) == 0 {
		return
	}

	skillsDir := ""
	if a.config.SkillLoader != nil {
		skillsDir = a.config.SkillLoader.WorkspaceDir()
	}
	if skillsDir == "" {
		return
	}

	a.out("[skill-evolution] Reflecting on used skills: %s\n", strings.Join(readSkillNames, ", "))

	messages := a.context.BuildMessages()
	messages = NormalizeAPIMessages(messages)
	cacheParams := CaptureCacheSafeParams(
		a.context.SystemPrompt(),
		a.config.Model,
		a.registry,
		messages,
	)

	prompt := BuildSkillsReflectionPrompt(readSkillNames, a.budget.Consumed()-a.taskStartTurns)
	if prompt == "" {
		return
	}

	forkMessages := []anthropic.MessageParam{
		{
			Role:    anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{{OfText: &anthropic.TextBlockParam{Text: prompt}}},
		},
	}

	canUseTool := createSkillEvolutionCanUseTool(skillsDir)

	cfg := ForkedAgentConfig{
		CacheSafeParams: cacheParams,
		ForkMessages:    forkMessages,
		CanUseTool:      canUseTool,
		MaxTokens:       8192,
		QuerySource:     "skill_reflection",
		MaxTurns:        5,
		Registry:        a.registry,
		ProjectDir:      a.config.ProjectDir,
		Client:          a.client,
	}

	_, err := RunForkedAgent(cfg)
	if err != nil {
		a.logDebug("[skill-evolution] reflection forked agent error: %v\n", err)
		return
	}

	a.out("[skill-evolution] Skill reflection complete.\n")
}

// runSkillAutoCreation forks a sub-agent to analyze the complex task that just
// completed and determine if it should be captured as a new reusable skill.
func (a *AgentLoop) runSkillAutoCreation(taskTurns int) {
	skillsDir := ""
	if a.config.SkillLoader != nil {
		skillsDir = a.config.SkillLoader.WorkspaceDir()
	}
	if skillsDir == "" {
		return
	}

	a.out("[skill-evolution] Analyzing complex task (%d turns) for skill creation...\n", taskTurns)

	messages := a.context.BuildMessages()
	messages = NormalizeAPIMessages(messages)
	cacheParams := CaptureCacheSafeParams(
		a.context.SystemPrompt(),
		a.config.Model,
		a.registry,
		messages,
	)

	prompt := BuildSkillAutoCreatePrompt(taskTurns)

	forkMessages := []anthropic.MessageParam{
		{
			Role:    anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{{OfText: &anthropic.TextBlockParam{Text: prompt}}},
		},
	}

	canUseTool := createSkillAutoCreateCanUseTool(skillsDir)

	cfg := ForkedAgentConfig{
		CacheSafeParams: cacheParams,
		ForkMessages:    forkMessages,
		CanUseTool:      canUseTool,
		MaxTokens:       8192,
		QuerySource:     "skill_auto_creation",
		MaxTurns:        5,
		Registry:        a.registry,
		ProjectDir:      a.config.ProjectDir,
		Client:          a.client,
	}

	_, err := RunForkedAgent(cfg)
	if err != nil {
		a.logDebug("[skill-evolution] auto-creation forked agent error: %v\n", err)
		return
	}

	a.out("[skill-evolution] Skill auto-creation analysis complete.\n")
}

// createSkillEvolutionCanUseTool creates a CanUseToolFn for the skill reflection forked agent.
// Only edit_file (skills dir only), read_file, and glob are allowed.
func createSkillEvolutionCanUseTool(skillsDir string) CanUseToolFn {
	allowed := map[string]bool{
		"edit_file": true,
		"read_file": true,
		"glob":      true,
	}

	return func(toolName string, args map[string]any) (bool, string) {
		if !allowed[toolName] {
			return false, fmt.Sprintf("Tool %q is not available during skill reflection — only edit_file, read_file, and glob are allowed", toolName)
		}
		if toolName == "edit_file" {
			if path, ok := args["file_path"].(string); ok {
				cleanPath := filepath.Clean(path)
				cleanDir := filepath.Clean(skillsDir)
				if !strings.HasPrefix(cleanPath, cleanDir+string(filepath.Separator)) && cleanPath != cleanDir {
					return false, "edit_file is only allowed in the skills directory"
				}
			}
		}
		return true, ""
	}
}

// createSkillAutoCreateCanUseTool creates a CanUseToolFn for the skill auto-creation forked agent.
// Only write_file (skills dir only), read_file, and glob are allowed.
func createSkillAutoCreateCanUseTool(skillsDir string) CanUseToolFn {
	allowed := map[string]bool{
		"write_file": true,
		"read_file":  true,
		"glob":       true,
	}

	return func(toolName string, args map[string]any) (bool, string) {
		if !allowed[toolName] {
			return false, fmt.Sprintf("Tool %q is not available during skill auto-creation — only write_file, read_file, and glob are allowed", toolName)
		}
		if toolName == "write_file" {
			if path, ok := args["file_path"].(string); ok {
				cleanPath := filepath.Clean(path)
				cleanDir := filepath.Clean(skillsDir)
				if !strings.HasPrefix(cleanPath, cleanDir+string(filepath.Separator)) {
					return false, "write_file is only allowed in the skills directory"
				}
			}
		}
		return true, ""
	}
}

