package main

import (
	"fmt"
	"strings"
)

// ─── Task Gate Stop (MiMo-Code P5) ──────────────────────────────────────────
//
// Before an agent stops, the gate checks the task registry for non-terminal
// tasks (open/in_progress). If any exist, it injects a system-reminder listing
// incomplete tasks and forces the agent to re-enter its loop.
//
// MiMo-Code source: task/gate.ts (116 lines)

const (
	// MaxTaskGateSubagentReact cap for subagent re-entries.
	MaxTaskGateSubagentReact = 2
	// MaxTaskGateMainReact cap for main session re-entries.
	MaxTaskGateMainReact = 3
)

// GateMode determines the gate behavior.
type GateMode string

const (
	GateModeSubagent GateMode = "subagent"
	GateModeMain     GateMode = "main"
)

// TaskGateDecision represents the gate's decision.
type TaskGateDecision struct {
	NeedReentry      bool
	CapExceeded      bool
	ReentryText      string
	IncompleteTasks  []string
}

// TaskGateInput provides input to the gate decision function.
type TaskGateInput struct {
	TaskStore  *WorkTaskStore
	Owner      string    // subagent ID or empty for main
	ReactCount int       // current re-entry count
	MaxReact   int       // max re-entries allowed
	Mode       GateMode
}

// TaskGateDecide checks if the agent should be prevented from stopping.
// Returns a decision indicating whether to re-enter the loop.
func TaskGateDecide(input TaskGateInput) TaskGateDecision {
	if input.TaskStore == nil {
		return TaskGateDecision{NeedReentry: false, CapExceeded: false}
	}

	// Get active tasks (open/in_progress only, exclude blocked)
	tasks := input.TaskStore.ListActiveTasks()

	// Filter to actionable tasks only (open or in_progress)
	var actionable []WorkTask
	for _, t := range tasks {
		if t.Status == WorkTaskPending || t.Status == WorkTaskInProgress {
			// If owner specified, only count tasks owned by that agent
			if input.Owner == "" || t.Owner == input.Owner {
				actionable = append(actionable, *t)
			}
		}
	}

	if len(actionable) == 0 {
		return TaskGateDecision{NeedReentry: false, CapExceeded: false}
	}

	// Check if cap exceeded
	if input.ReactCount >= input.MaxReact {
		var ids []string
		for _, t := range actionable {
			ids = append(ids, t.ID)
		}
		return TaskGateDecision{
			NeedReentry:     false,
			CapExceeded:     true,
			IncompleteTasks: ids,
		}
	}

	// Build reentry text
	reentryText := buildTaskGateReentryText(actionable, input.Mode)
	var ids []string
	for _, t := range actionable {
		ids = append(ids, t.ID)
	}

	return TaskGateDecision{
		NeedReentry:     true,
		CapExceeded:     false,
		ReentryText:     reentryText,
		IncompleteTasks: ids,
	}
}

// buildTaskGateReentryText builds the system-reminder text for task gate.
func buildTaskGateReentryText(tasks []WorkTask, mode GateMode) string {
	var sb strings.Builder

	sb.WriteString("<system-reminder>\n")

	if mode == GateModeSubagent {
		sb.WriteString("You are about to finish, but these tasks you own are still unfinished:\n")
	} else {
		sb.WriteString("You are about to finish, but these tasks in this session are still unfinished:\n")
	}

	for _, t := range tasks {
		sb.WriteString(fmt.Sprintf("- %s (%s): %s\n", t.ID, t.Status, t.Subject))
	}

	sb.WriteString("For EACH: complete the work then `task done <id> <summary>`, or `task abandon <id> <reason>` if it is genuinely not needed.\n")

	if mode == GateModeSubagent {
		sb.WriteString("Then re-emit your final message starting with the **Status**/**Summary** header.\n")
	} else {
		sb.WriteString("Then continue or respond.\n")
	}

	sb.WriteString("</system-reminder>")

	return sb.String()
}

// GetMaxReact returns the max re-entry count for the given mode.
func GetMaxReact(mode GateMode) int {
	if mode == GateModeSubagent {
		return MaxTaskGateSubagentReact
	}
	return MaxTaskGateMainReact
}
