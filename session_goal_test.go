package main

import (
	"strings"
	"testing"
)

func TestGoalManager_SetAndGet(t *testing.T) {
	m := NewGoalManager()

	m.Set("session-1", "Create a working auth module")

	goal := m.Get("session-1")
	if goal == nil {
		t.Fatal("expected goal to be set")
	}
	if goal.Condition != "Create a working auth module" {
		t.Errorf("expected condition 'Create a working auth module', got %q", goal.Condition)
	}
	if goal.React != 0 {
		t.Errorf("expected react=0, got %d", goal.React)
	}
}

func TestGoalManager_Clear(t *testing.T) {
	m := NewGoalManager()

	m.Set("session-1", "test goal")
	m.Clear("session-1")

	if m.Get("session-1") != nil {
		t.Error("expected goal to be cleared")
	}
}

func TestGoalManager_BumpReact(t *testing.T) {
	m := NewGoalManager()

	m.Set("session-1", "test goal")

	if m.BumpReact("session-1") != 1 {
		t.Error("expected react=1")
	}
	if m.BumpReact("session-1") != 2 {
		t.Error("expected react=2")
	}
}

func TestGoalManager_HasGoal(t *testing.T) {
	m := NewGoalManager()

	if m.HasGoal("session-1") {
		t.Error("expected no goal initially")
	}

	m.Set("session-1", "test goal")

	if !m.HasGoal("session-1") {
		t.Error("expected goal to exist")
	}
}

func TestGoalManager_BumpReact_NoGoal(t *testing.T) {
	m := NewGoalManager()

	if m.BumpReact("session-1") != 0 {
		t.Error("expected 0 for non-existent goal")
	}
}

func TestBuildJudgePrompt(t *testing.T) {
	prompt := BuildJudgePrompt("Create auth module", "User: implement auth\nAssistant: done")

	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "Create auth module") {
		t.Error("expected prompt to contain condition")
	}
	if !strings.Contains(prompt, "implement auth") {
		t.Error("expected prompt to contain transcript")
	}
}

func TestParseJudgeResponse_OK(t *testing.T) {
	response := `{"ok": true, "reason": "Auth module was created successfully"}`
	verdict, err := ParseJudgeResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !verdict.OK {
		t.Error("expected ok=true")
	}
	if verdict.Reason != "Auth module was created successfully" {
		t.Errorf("expected reason, got %q", verdict.Reason)
	}
}

func TestParseJudgeResponse_NotOK(t *testing.T) {
	response := `{"ok": false, "reason": "Auth module not yet created"}`
	verdict, err := ParseJudgeResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.OK {
		t.Error("expected ok=false")
	}
}

func TestParseJudgeResponse_Impossible(t *testing.T) {
	response := `{"ok": false, "impossible": true, "reason": "Cannot access external API"}`
	verdict, err := ParseJudgeResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.OK {
		t.Error("expected ok=false")
	}
	if !verdict.Impossible {
		t.Error("expected impossible=true")
	}
}

func TestParseJudgeResponse_InvalidJSON(t *testing.T) {
	_, err := ParseJudgeResponse("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseJudgeResponse_WithPrefix(t *testing.T) {
	response := `Here is my evaluation: {"ok": true, "reason": "done"}`
	verdict, err := ParseJudgeResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !verdict.OK {
		t.Error("expected ok=true")
	}
}

func TestBuildGoalReentryMessage(t *testing.T) {
	goal := &Goal{Condition: "Create auth", React: 2}
	verdict := GoalVerdict{OK: false, Reason: "Auth not created yet"}

	msg := BuildGoalReentryMessage(goal, verdict)

	if !strings.Contains(msg, "Create auth") {
		t.Error("expected message to contain goal condition")
	}
	if !strings.Contains(msg, "Auth not created yet") {
		t.Error("expected message to contain verdict reason")
	}
	if !strings.Contains(msg, "2/5") {
		t.Error("expected message to contain attempt count")
	}
}

func TestBuildGoalReentryMessage_Impossible(t *testing.T) {
	goal := &Goal{Condition: "Test goal", React: 1}
	verdict := GoalVerdict{OK: false, Impossible: true, Reason: "Cannot be done"}

	msg := BuildGoalReentryMessage(goal, verdict)

	if !strings.Contains(msg, "impossible") {
		t.Error("expected message to indicate impossible")
	}
}

func TestShouldReentry(t *testing.T) {
	tests := []struct {
		name     string
		goal     *Goal
		verdict  GoalVerdict
		expected bool
	}{
		{"nil goal", nil, GoalVerdict{}, false},
		{"ok", &Goal{React: 0}, GoalVerdict{OK: true}, false},
		{"impossible", &Goal{React: 0}, GoalVerdict{OK: false, Impossible: true}, false},
		{"cap exceeded", &Goal{React: 5}, GoalVerdict{OK: false}, false},
		{"should reentry", &Goal{React: 2}, GoalVerdict{OK: false}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldReentry(tt.goal, tt.verdict)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExtractJSONFromResponse(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"ok": true}`, `{"ok": true}`},
		{`prefix {"ok": true} suffix`, `{"ok": true}`},
		{`no json here`, ""},
		{`{invalid}`, `{invalid}`},
	}

	for _, tt := range tests {
		result := extractJSONFromResponse(tt.input)
		if result != tt.expected {
			t.Errorf("extractJSONFromResponse(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
