package main

import (
	"fmt"
	"sync"
)

// ─── Plan Exit Tool (MiMo-Code 7) ──────────────────────────────────────────
//
// A tool available only to the plan agent. When invoked, it asks the user
// whether to switch to the build agent and start implementing.
//
// MiMo-Code source: tool/plan.ts (20-90 lines)

// PlanExitResult represents the result of a plan exit.
type PlanExitResult struct {
	Approved bool   `json:"approved"`
	Message  string `json:"message"`
	PlanPath string `json:"plan_path,omitempty"`
}

// PlanExitTool provides the plan exit functionality.
type PlanExitTool struct {
	mu        sync.Mutex
	planPath  string
	onApprove func() // callback when plan is approved
}

// NewPlanExitTool creates a new plan exit tool.
func NewPlanExitTool() *PlanExitTool {
	return &PlanExitTool{}
}

// SetPlanPath sets the plan file path.
func (t *PlanExitTool) SetPlanPath(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.planPath = path
}

// SetOnApprove sets the callback for when plan is approved.
func (t *PlanExitTool) SetOnApprove(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onApprove = fn
}

// Execute executes the plan exit tool.
func (t *PlanExitTool) Execute(approved bool) PlanExitResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	if approved {
		if t.onApprove != nil {
			t.onApprove()
		}
		return PlanExitResult{
			Approved: true,
			Message:  fmt.Sprintf("Plan approved. Switching to build agent. Execute the plan at %s.", t.planPath),
			PlanPath: t.planPath,
		}
	}

	return PlanExitResult{
		Approved: false,
		Message:  "Plan not approved. Continue refining.",
		PlanPath: t.planPath,
	}
}

// BuildPlanExitPrompt builds a prompt for the plan exit decision.
func BuildPlanExitPrompt(planPath string) string {
	return fmt.Sprintf("The plan at %s has been reviewed. Would you like to switch to the build agent and start implementing?", planPath)
}
