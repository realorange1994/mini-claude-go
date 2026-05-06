package tools

import (
	"fmt"
)

// ExitPlanModeTool switches the agent out of plan mode back to its previous mode.
type ExitPlanModeTool struct {
	GetMode        func() string
	SetMode        func(mode string)
	GetPrePlanMode func() string
}

func (t *ExitPlanModeTool) Name() string { return "ExitPlanMode" }

func (t *ExitPlanModeTool) Description() string {
	return "Exit plan mode and return to normal execution. This allows all tools to be used again. Call this after the user has approved your plan."
}

func (t *ExitPlanModeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"approved": map[string]any{
				"type":        "boolean",
				"description": "Whether the user has approved the plan. If false, remain in plan mode.",
				"default":     true,
			},
			"summary": map[string]any{
				"type":        "string",
				"description": "Brief summary of what was approved and what will be implemented.",
			},
		},
		"required": []string{},
	}
}

func (t *ExitPlanModeTool) CheckPermissions(params map[string]any) string {
	return "" // auto-approved
}

func (t *ExitPlanModeTool) Execute(params map[string]any) ToolResult {
	currentMode := t.GetMode()
	if currentMode != "plan" {
		return ToolResultOK("Not in plan mode. Nothing to exit.")
	}

	approved, hasApproved := params["approved"].(bool)
	if hasApproved && !approved {
		return ToolResultOK("Plan not yet approved. Stay in plan mode and continue refining the plan.")
	}

	// Restore to the mode that was active before entering plan mode
	prePlan := t.GetPrePlanMode()
	if prePlan == "plan" || prePlan == "" {
		prePlan = "auto" // Default fallback if no pre-plan mode was stored
	}
	t.SetMode(prePlan)

	summary, _ := params["summary"].(string)
	msg := fmt.Sprintf("Exited plan mode and restored to %s mode. Ready to execute.", prePlan)
	if summary != "" {
		msg = fmt.Sprintf("Exited plan mode. Plan approved: %s\n\n%s", summary, msg)
	}

	return ToolResultOK(msg)
}
