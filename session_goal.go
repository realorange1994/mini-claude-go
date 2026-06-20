package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// ─── Session Goal / Judge Stop-Condition (MiMo-Code P14) ───────────────────
//
// A /goal command that sets an objective for the session. Once set, the main
// run loop refuses to stop until an independent judge model evaluates the
// conversation transcript and determines the condition is satisfied (or
// genuinely impossible).
//
// MiMo-Code source: session/goal.ts (232 lines)

const (
	// MaxGoalReact maximum judge-driven re-entries
	MaxGoalReact = 5
)

// Goal represents a session-level stop condition.
type Goal struct {
	Condition string `json:"condition"`
	React     int    `json:"react"` // re-entry counter
}

// GoalVerdict represents the judge's evaluation result.
type GoalVerdict struct {
	OK         bool   `json:"ok"`
	Impossible bool   `json:"impossible,omitempty"`
	Reason     string `json:"reason"`
}

// GoalManager manages session goals and judge evaluations.
type GoalManager struct {
	mu    sync.Mutex
	goals map[string]*Goal // sessionID -> goal
}

// NewGoalManager creates a new goal manager.
func NewGoalManager() *GoalManager {
	return &GoalManager{
		goals: make(map[string]*Goal),
	}
}

// Set sets a goal for a session.
func (m *GoalManager) Set(sessionID, condition string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.goals[sessionID] = &Goal{
		Condition: condition,
		React:     0,
	}
}

// Get returns the current goal for a session.
func (m *GoalManager) Get(sessionID string) *Goal {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.goals[sessionID]
}

// Clear removes the goal for a session.
func (m *GoalManager) Clear(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.goals, sessionID)
}

// BumpReact increments the re-entry counter and returns the new count.
func (m *GoalManager) BumpReact(sessionID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	goal := m.goals[sessionID]
	if goal == nil {
		return 0
	}
	goal.React++
	return goal.React
}

// HasGoal returns true if a session has an active goal.
func (m *GoalManager) HasGoal(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.goals[sessionID] != nil
}

// BuildJudgePrompt builds the judge prompt for evaluating a goal condition.
func BuildJudgePrompt(condition string, transcript string) string {
	var sb strings.Builder

	sb.WriteString(JudgeSystemPrompt)
	sb.WriteString("\n\n")
	sb.WriteString(transcript)
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("Based on the conversation transcript above, has the following stopping condition been satisfied? Answer based on transcript evidence only.\n\nCondition: %s", condition))

	return sb.String()
}

// ParseJudgeResponse parses the judge's JSON response into a GoalVerdict.
func ParseJudgeResponse(response string) (GoalVerdict, error) {
	// Try to extract JSON from the response
	jsonStr := extractJSONFromResponse(response)
	if jsonStr == "" {
		return GoalVerdict{}, fmt.Errorf("no JSON found in judge response")
	}

	var verdict GoalVerdict
	if err := json.Unmarshal([]byte(jsonStr), &verdict); err != nil {
		return GoalVerdict{}, fmt.Errorf("parse judge response: %w", err)
	}

	return verdict, nil
}

// BuildGoalReentryMessage builds a re-entry message when the judge says the goal is not met.
func BuildGoalReentryMessage(goal *Goal, verdict GoalVerdict) string {
	var sb strings.Builder

	sb.WriteString("<system-reminder>\n")
	sb.WriteString("The judge evaluated your progress against the session goal and determined it is NOT yet satisfied.\n\n")
	sb.WriteString(fmt.Sprintf("**Goal**: %s\n", goal.Condition))
	sb.WriteString(fmt.Sprintf("**Verdict**: not satisfied\n"))
	sb.WriteString(fmt.Sprintf("**Reason**: %s\n", verdict.Reason))
	sb.WriteString(fmt.Sprintf("**Attempt**: %d/%d\n\n", goal.React, MaxGoalReact))

	if verdict.Impossible {
		sb.WriteString("The judge determined this goal is **impossible** to achieve in this session. You may stop working on it.\n")
	} else {
		sb.WriteString("Continue working toward the goal. When you believe it is satisfied, provide evidence in your response.\n")
	}

	sb.WriteString("</system-reminder>")

	return sb.String()
}

// ShouldReentry returns true if the agent should continue working on the goal.
func ShouldReentry(goal *Goal, verdict GoalVerdict) bool {
	if goal == nil {
		return false
	}
	if verdict.OK {
		return false // goal satisfied
	}
	if verdict.Impossible {
		return false // goal impossible
	}
	if goal.React >= MaxGoalReact {
		return false // cap exceeded
	}
	return true
}

// JudgeSystemPrompt is the system prompt for the judge model.
const JudgeSystemPrompt = `You are evaluating a stop-condition hook in MiMo Code. Read the conversation transcript carefully, then judge whether the user-provided condition is satisfied.

Your response must be a JSON object with one of these shapes:
- {"ok": true, "reason": "<quote evidence from the transcript that satisfies the condition>"}
- {"ok": false, "reason": "<quote what is missing or what blocks the condition>"}
- {"ok": false, "impossible": true, "reason": "<explain why the condition can never be satisfied>"}

Always include a "reason" field, quoting specific text from the transcript whenever possible. If the transcript does not contain clear evidence that the condition is satisfied, return {"ok": false, "reason": "insufficient evidence in transcript"}.

Only use {"ok": false, "impossible": true} when the condition is genuinely unachievable in this session — for example: the condition is self-contradictory, it depends on a resource or capability that is unavailable, or the assistant has explicitly tried, exhausted reasonable approaches, and stated it cannot be done. Apply your own judgment when deciding this — the assistant claiming the goal is impossible is evidence, not proof; independently confirm the condition is genuinely unachievable rather than deferring to the assistant's self-assessment. Do not use it just because the goal has not been reached yet or because progress is slow. When in doubt, return {"ok": false} without "impossible".`

// extractJSONFromResponse extracts JSON from a response string.
func extractJSONFromResponse(s string) string {
	// Find first { and last }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}
