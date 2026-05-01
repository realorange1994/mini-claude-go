package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"miniclaudecode-go/skills"
	"miniclaudecode-go/tools"
)

const systemPromptTemplate = `You are miniClaudeCode (model: %s), a lightweight AI coding assistant that operates in the terminal.

## Environment
- OS: %s
- Working Directory: %s
- Current Date/Time: %s (%s)
- Shell: PowerShell on Windows, sh/bash on Unix

You have access to the following tools to help the user with software engineering tasks:
%s

## Operating Rules

### Behavioral Guidelines

1. **Think Before Coding** -- Don't assume. Don't hide confusion. State assumptions explicitly. If multiple interpretations exist, present them. If something is unclear, stop and ask.
2. **Simplicity First** -- Write the minimum code that solves the problem. No features beyond what was asked. No abstractions for single-use code. No error handling for impossible scenarios.
3. **Surgical Changes** -- Touch only what you must. Don't "improve" adjacent code, comments, or formatting. Don't refactor things that aren't broken. Match existing style. Remove only imports/variables/functions that YOUR changes made unused.
4. **Goal-Driven Execution** -- For multi-step tasks, state a brief plan with verification criteria: "1. [Step] -> verify: [check]". Define success criteria before starting.

### Tool Rules

5. Always read a file before editing it.
6. Use tools to accomplish tasks -- don't just describe what to do.
7. When running bash commands, prefer non-destructive read operations.
8. For file edits, provide enough context in old_string to uniquely match.
9. Be concise and direct in your responses.
10. On Windows, use PowerShell syntax and commands (e.g., Get-ChildItem, Test-Path, Copy-Item). On Unix, use bash commands.
11. Prefer built-in tools over exec commands. For git operations, use the git tool instead of exec. For file searches, use grep and glob instead of exec. Always choose the most appropriate built-in tool when available.
12. **Sub-Agent Dispatching** -- When the user requests dispatching, delegating, or assigning a task to a sub-agent (indicated by keywords like: 派遣, 安排, 让, 要, 使, dispatch, delegate, spawn, launch agent, sub-agent), you MUST use the "agent" tool. Do NOT use mcp_call_tool, coze_llm, minimax_llm, or any MCP tool for sub-agent dispatching. The "agent" tool creates autonomous sub-agents with their own context and tool access. MCP LLM tools are only for calling external LLM APIs (generation/embedding/search), NOT for creating sub-agents.

## Task Management Rules

13. **When to Create Tasks** -- Use task_create for complex multi-step tasks (3+ distinct steps or actions). When the user provides multiple tasks (numbered or comma-separated), immediately capture them as tasks. When starting work on a task, mark it as in_progress BEFORE beginning work. After completing a task, mark it as completed and add any new follow-up tasks discovered.
14. **Task Workflow** -- Create tasks with clear, specific subjects in imperative form (e.g., "Fix authentication bug"). Use task_update to set status: pending → in_progress → completed. ONLY mark as completed when FULLY accomplished — if tests fail, implementation is partial, or you encountered unresolved errors, keep the task in_progress. If blocked, create a new task describing what needs to be resolved. After completing a task, check task_list to find the next available task. Do not batch up multiple tasks before marking them as completed — mark each one done as soon as it is finished.
15. **Background Command Execution** -- For long-running commands, use run_in_background=true with the exec tool. You will receive a task ID and output file path immediately; you do not need to check the output right away. When the background task completes, you will be notified via a task-notification message. Use task_output to retrieve results. Use task_stop to stop a running background task if needed. Do NOT use sleep to poll for results — use run_in_background and wait for the notification.

## Tool Parameters

All tools accept an optional "timeout" parameter (integer, seconds, range 1-600, default 600) to override the execution timeout. Use a larger timeout for operations that may take longer, such as scanning large directories with grep or glob.

## Current Permission Mode: %s
%s
%s
%s
%s`

var modeDescriptions = map[string]string{
	"ask":  "In ASK mode, potentially dangerous operations will require user confirmation.",
	"auto": "In AUTO mode, all operations are auto-approved (use with caution).",
	"plan": "In PLAN mode, only read-only operations are allowed. Write operations are blocked.",
}

// BuildSystemPrompt constructs the system prompt from tool list, mode, project instructions, and skills.
func BuildSystemPrompt(registry *tools.Registry, permissionMode, projectDir, modelName string, skillLoader *skills.Loader, skillTracker *skills.SkillTracker, sessionMemory *SessionMemory) string {
	toolList := buildToolList(registry)

	modeDesc := modeDescriptions[permissionMode]

	wd, _ := os.Getwd()
	envInfo := fmt.Sprintf("%s / %s / %s", runtime.GOOS, runtime.Version(), runtime.GOARCH)

	projectInstructions := LoadProjectInstructions(projectDir)
	var projectSection string
	if projectInstructions != "" {
		projectSection = "## Project Instructions (from CLAUDE.md)\n\n" + projectInstructions
	}

	// Inject session memory
	var memorySection string
	if sessionMemory != nil {
		if mem := sessionMemory.FormatForPrompt(); mem != "" {
			memorySection = mem
		}
	}

	// Build skills section
	var skillsSection string
	if skillLoader != nil {
		var skillGuidance string
		if skillTracker != nil {
			skillGuidance = "\n## Skill System Guidance\n\n" +
				"BLOCKING REQUIREMENT: When a skill matches the user's request, you MUST invoke the relevant skill tool BEFORE generating any other response.\n" +
				"Your visible tool list is partial by design -- many skills are hidden until discovered.\n" +
				"Discovery steps:\n" +
				"1. Use **search_skills** to find skills by topic (e.g., search_skills 'testing')\n" +
				"2. Use **read_skill** to load a skill's full instructions\n" +
				"3. Follow the skill's instructions precisely\n\n"
		}

		// Get unsent skills (not yet shown in system prompt)
		allSkills := skillLoader.ListSkills(false)
		var unsentSkills []skills.SkillInfo
		if skillTracker != nil {
			unsentSkills = skillTracker.GetUnsentSkills(allSkills)
			// Mark unsent skills as shown
			for _, s := range unsentSkills {
				if !s.Always {
					skillTracker.MarkShown(s.Name)
				}
			}
		} else {
			// No tracker -- treat all non-always skills as unsent (first run)
			for _, s := range allSkills {
				if !s.Always {
					unsentSkills = append(unsentSkills, s)
				}
			}
		}

		// Always-on skills section
		alwaysSkills := skillLoader.GetAlwaysSkills()
		if len(alwaysSkills) > 0 {
			var skillNames []string
			for _, s := range alwaysSkills {
				skillNames = append(skillNames, s.Name)
			}
			skillsSection = skillLoader.BuildSystemPrompt(skillNames)
		}

		// "New This Turn" section for unsent non-always skills
		var newSkills []skills.SkillInfo
		for _, s := range unsentSkills {
			if !s.Always {
				newSkills = append(newSkills, s)
			}
		}
		if len(newSkills) > 0 {
			var sb strings.Builder
			sb.WriteString("\n## Available Skills (New This Turn)\n\n")
			sb.WriteString("The following skills are newly available. Use read_skill to load full instructions.\n\n")
			budget := 4000
			used := 0
			for _, s := range newSkills {
				entry := fmt.Sprintf("- **%s**: %s", s.Name, s.Description)
				if s.WhenToUse != "" {
					entry += fmt.Sprintf(" (%s)", s.WhenToUse)
				}
				if !s.Available {
					entry += " (unavailable)"
				}
				entry += "\n"
				if used+len(entry) > budget {
					break
				}
				sb.WriteString(entry)
				used += len(entry)
			}
			if skillsSection != "" {
				skillsSection += "\n"
			}
			skillsSection += sb.String()
		}

		// Skills summary for already-shown skills
		skillsSummary := skillLoader.BuildSkillsSummary()
		if skillsSummary != "" {
			if skillsSection != "" {
				skillsSection += "\n"
			}
			skillsSection += "## Available Skills\n\n" + skillsSummary
		}

		// Prepend skill guidance
		if skillGuidance != "" && skillsSection != "" {
			skillsSection = skillGuidance + skillsSection
		}
	}

	currentTime := time.Now().Format("2006-01-02 15:04:05")
	_, offset := time.Now().Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	timezone := fmt.Sprintf("UTC%s%02d:%02d", sign, hours, minutes)
	return fmt.Sprintf(systemPromptTemplate, modelName, envInfo, wd, currentTime, timezone, toolList, strings.ToUpper(permissionMode), modeDesc, projectSection, memorySection, skillsSection)
}

func buildToolList(registry *tools.Registry) string {
	var sb strings.Builder
	for _, t := range registry.AllTools() {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", t.Name(), t.Description()))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// CachedSystemPrompt caches the system prompt and only rebuilds when marked dirty.
type CachedSystemPrompt struct {
	cached string
	dirty  atomic.Bool
	mu     sync.RWMutex
}

// NewCachedSystemPrompt creates a new CachedSystemPrompt initialized as dirty.
func NewCachedSystemPrompt() *CachedSystemPrompt {
	cp := &CachedSystemPrompt{}
	cp.dirty.Store(true)
	return cp
}

// GetOrBuild returns the cached system prompt, rebuilding only if dirty.
func (cp *CachedSystemPrompt) GetOrBuild(registry *tools.Registry, permissionMode, projectDir, modelName string, skillLoader *skills.Loader, skillTracker *skills.SkillTracker, sessionMemory *SessionMemory) string {
	if !cp.dirty.Load() {
		cp.mu.RLock()
		cached := cp.cached
		cp.mu.RUnlock()
		if cached != "" {
			return cached
		}
	}

	prompt := BuildSystemPrompt(registry, permissionMode, projectDir, modelName, skillLoader, skillTracker, sessionMemory)
	cp.mu.Lock()
	cp.cached = prompt
	cp.mu.Unlock()
	cp.dirty.Store(false)
	return prompt
}

// MarkDirty marks the cached system prompt as needing rebuild on next access.
func (cp *CachedSystemPrompt) MarkDirty() {
	cp.dirty.Store(true)
}
