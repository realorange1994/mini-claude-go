package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"miniclaudecode-go/skills"
	"miniclaudecode-go/tools"
)

const systemPromptTemplate = `You are miniClaudeCode, a lightweight AI coding assistant that operates in the terminal.

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
11. Use git directly for git operations -- it is available in the PATH.

## Tool Parameters

All tools accept an optional "timeout" parameter (integer, seconds, range 1-300, default 30) to override the execution timeout. Use a larger timeout for operations that may take longer, such as scanning large directories with grep or glob.

## Current Permission Mode: %s
%s
%s
%s`

var modeDescriptions = map[string]string{
	"ask":  "In ASK mode, potentially dangerous operations will require user confirmation.",
	"auto": "In AUTO mode, all operations are auto-approved (use with caution).",
	"plan": "In PLAN mode, only read-only operations are allowed. Write operations are blocked.",
}

// BuildSystemPrompt constructs the system prompt from tool list, mode, project instructions, and skills.
func BuildSystemPrompt(registry *tools.Registry, permissionMode, projectDir string, skillLoader *skills.Loader, skillTracker *skills.SkillTracker) string {
	toolList := buildToolList(registry)

	modeDesc := modeDescriptions[permissionMode]

	wd, _ := os.Getwd()
	envInfo := fmt.Sprintf("%s / %s / %s", runtime.GOOS, runtime.Version(), runtime.GOARCH)

	projectInstructions := LoadProjectInstructions(projectDir)
	var projectSection string
	if projectInstructions != "" {
		projectSection = "## Project Instructions (from CLAUDE.md)\n\n" + projectInstructions
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
	return fmt.Sprintf(systemPromptTemplate, envInfo, wd, currentTime, timezone, toolList, strings.ToUpper(permissionMode), modeDesc, projectSection, skillsSection)
}

func buildToolList(registry *tools.Registry) string {
	var sb strings.Builder
	for _, t := range registry.AllTools() {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", t.Name(), t.Description()))
	}
	return strings.TrimRight(sb.String(), "\n")
}
