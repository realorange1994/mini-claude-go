package tools

import (
	"fmt"
)

// PermissionModeSetter is a callback that sets the permission mode.
type PermissionModeSetter func(mode string)

// EnterPlanModeTool switches the agent into plan mode.
type EnterPlanModeTool struct {
	GetMode func() string
	SetMode PermissionModeSetter
}

func (t *EnterPlanModeTool) Name() string { return "EnterPlanMode" }

func (t *EnterPlanModeTool) Description() string {
	return "Use this tool to enter plan mode. In plan mode, you will explore the codebase and design a plan before making any changes. Only read-only operations are allowed. Write your plan to the plan file, then use ExitPlanMode when ready to implement."
}

func (t *EnterPlanModeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reason": map[string]any{
				"type":        "string",
				"description": "Brief reason for entering plan mode (e.g., 'Implement new feature', 'Fix complex bug')",
			},
		},
		"required": []string{},
	}
}

func (t *EnterPlanModeTool) CheckPermissions(params map[string]any) PermissionResult {
	return PermissionResultPassthrough() // auto-approved
}

func (t *EnterPlanModeTool) Execute(params map[string]any) ToolResult {
	currentMode := t.GetMode()
	if currentMode == "plan" {
		return ToolResultOK("Already in plan mode. Continue planning — use ExitPlanMode when ready to implement.")
	}

	t.SetMode("plan")

	reason, _ := params["reason"].(string)
	msg := "Entered plan mode. Only read-only operations are allowed.\n\n"
	msg += "Follow the 5-phase plan mode workflow:\n"
	msg += "1. **Initial Understanding** — Explore the codebase using read-only tools\n"
	msg += "2. **Design** — Evaluate approaches and trade-offs\n"
	msg += "3. **Review** — Read critical files and clarify requirements\n"
	msg += "4. **Final Plan** — Write the plan to the plan file\n"
	msg += "5. **ExitPlanMode** — Call ExitPlanMode when ready to implement\n"

	if reason != "" {
		msg = fmt.Sprintf("Entered plan mode for: %s\n\n%s", reason, msg)
	}

	return ToolResultOK(msg)
}
