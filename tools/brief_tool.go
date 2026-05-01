package tools

import (
	"fmt"
)

// BriefTool provides proactive communication guidance to the agent.
// When called, it returns a set of concise communication principles
// to help the agent be more effective in its responses.
type BriefTool struct{}

func (t *BriefTool) Name() string { return "brief" }

func (t *BriefTool) Description() string {
	return `Provides communication principles for effective agent interactions.
Call this tool to receive concise guidelines on how to communicate clearly, directly, and efficiently with the user.
Always call this at the start of a task to align with best practices.`
}

func (t *BriefTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"task"},
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "Description of the task you are about to work on. Used to tailor communication guidance.",
			},
		},
	}
}

func (t *BriefTool) CheckPermissions(params map[string]any) string { return "" }

func (t *BriefTool) Execute(params map[string]any) ToolResult {
	task, _ := params["task"].(string)
	if task == "" {
		return ToolResult{Output: "Error: task parameter is required", IsError: true}
	}

	guidelines := `# Communication Principles

## Core Rules
- **Be concise and direct** — lead with the answer, not the reasoning
- **Skip filler words, preamble, and unnecessary transitions** — no "I think", "Let me", "Here is"
- **Don't restate what the user said** — just do it
- **Include only what's necessary** — when explaining, give the minimum needed for understanding
- **One sentence beats three** — if you can say it briefly, do
- **Focus output on**: decisions needing user input, high-level status updates, errors/blockers

## Task Context
Task: ` + task

	return ToolResult{Output: fmt.Sprintf("## Brief: Communication Guidance\n\n%s", guidelines)}
}
