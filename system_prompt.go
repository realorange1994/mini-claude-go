package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"miniclaudecode-go/skills"
	"miniclaudecode-go/tools"
)

const systemPromptTemplate = `You are miniClaudeCode, a lightweight AI coding assistant that operates in the terminal.

## Environment
- OS: %s
- Working Directory: %s
- Shell: PowerShell on Windows, sh/bash on Unix

You have access to the following tools to help the user with software engineering tasks:
%s

## Operating Rules

1. Always read a file before editing it.
2. Use tools to accomplish tasks -- don't just describe what to do.
3. When running bash commands, prefer non-destructive read operations.
4. For file edits, provide enough context in old_string to uniquely match.
5. Be concise and direct in your responses.
6. On Windows, use PowerShell syntax and commands (e.g., Get-ChildItem, Test-Path, Copy-Item). On Unix, use bash commands.
7. Use git directly for git operations — it is available in the PATH.

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
func BuildSystemPrompt(registry *tools.Registry, permissionMode, projectDir string, skillLoader *skills.Loader) string {
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
		// Add always-on skills to system prompt
		alwaysSkills := skillLoader.GetAlwaysSkills()
		if len(alwaysSkills) > 0 {
			var skillNames []string
			for _, s := range alwaysSkills {
				skillNames = append(skillNames, s.Name)
			}
			skillsSection = skillLoader.BuildSystemPrompt(skillNames)
		}

		// Add skills summary for discovery
		skillsSummary := skillLoader.BuildSkillsSummary()
		if skillsSummary != "" {
			if skillsSection != "" {
				skillsSection += "\n\n"
			}
			skillsSection += "## Available Skills\n\n" + skillsSummary
		}
	}

	return fmt.Sprintf(systemPromptTemplate, envInfo, wd, toolList, strings.ToUpper(permissionMode), modeDesc, projectSection, skillsSection)
}

func buildToolList(registry *tools.Registry) string {
	var sb strings.Builder
	for _, t := range registry.AllTools() {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", t.Name(), t.Description()))
	}
	return strings.TrimRight(sb.String(), "\n")
}
