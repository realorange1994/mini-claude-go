package main

import (
	"os"
	"testing"
)

// ─── Text Loop Recovery Tests ──────────────────────────────────────────────

func TestTextLoopDetector_NoLoop(t *testing.T) {
	d := NewTextLoopDetector()

	d.CheckRecord("first response")
	d.CheckRecord("second response")
	d.CheckRecord("third response")

	if d.CheckRecord("fourth response") {
		t.Error("expected no loop with different responses")
	}
}

func TestTextLoopDetector_LoopDetected(t *testing.T) {
	d := NewTextLoopDetector()

	d.CheckRecord("same response")
	d.CheckRecord("same response")
	if !d.CheckRecord("same response") {
		t.Error("expected loop after 3 identical responses")
	}
}

func TestTextLoopDetector_RecoveryPrompt(t *testing.T) {
	d := NewTextLoopDetector()

	prompt := d.GetRecoveryPrompt()
	if prompt != RecoveryPromptMild {
		t.Errorf("expected mild prompt, got %q", prompt)
	}

	prompt = d.GetRecoveryPrompt()
	if prompt != RecoveryPromptStrong {
		t.Errorf("expected strong prompt, got %q", prompt)
	}
}

func TestTextLoopDetector_Reset(t *testing.T) {
	d := NewTextLoopDetector()

	d.CheckRecord("same")
	d.CheckRecord("same")
	d.Reset()

	if d.CheckRecord("same") {
		t.Error("expected no loop after reset")
	}
}

// ─── Repeated Step Detection Tests ────────────────────────────────────────

func TestRepeatedStepDetector_NoRepeat(t *testing.T) {
	d := NewRepeatedStepDetector()

	calls1 := []map[string]any{{"name": "bash", "input": map[string]any{"command": "ls"}}}
	calls2 := []map[string]any{{"name": "bash", "input": map[string]any{"command": "pwd"}}}

	d.CheckRecord(calls1)
	d.CheckRecord(calls2)

	if d.CheckRecord(calls1) {
		t.Error("expected no repeat with different calls")
	}
}

func TestRepeatedStepDetector_RepeatDetected(t *testing.T) {
	d := NewRepeatedStepDetector()

	calls := []map[string]any{{"name": "bash", "input": map[string]any{"command": "ls"}}}

	d.CheckRecord(calls)
	d.CheckRecord(calls)
	if !d.CheckRecord(calls) {
		t.Error("expected repeat after 3 identical calls")
	}
}

func TestRepeatedStepDetector_Reset(t *testing.T) {
	d := NewRepeatedStepDetector()

	calls := []map[string]any{{"name": "bash", "input": map[string]any{"command": "ls"}}}
	d.CheckRecord(calls)
	d.CheckRecord(calls)
	d.Reset()

	if d.CheckRecord(calls) {
		t.Error("expected no repeat after reset")
	}
}

func TestStableStringify(t *testing.T) {
	tests := []struct {
		input    map[string]any
		expected string
	}{
		{nil, "null"},
		{map[string]any{"b": 1, "a": 2}, `{"a":2,"b":1}`},
	}

	for _, tt := range tests {
		result := stableStringify(tt.input)
		if result != tt.expected {
			t.Errorf("stableStringify(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestStableStringify_EmptyMap(t *testing.T) {
	result := stableStringify(map[string]any{})
	if result != "{}" {
		t.Errorf("expected '{}', got %q", result)
	}
}

// ─── Session CWD Tests ────────────────────────────────────────────────────

func TestSessionCwd_Default(t *testing.T) {
	dir := t.TempDir()
	cwd := NewSessionCwd(dir)

	if cwd.Get("session-1") != dir {
		t.Errorf("expected %s, got %s", dir, cwd.Get("session-1"))
	}
}

func TestSessionCwd_Set(t *testing.T) {
	dir := t.TempDir()
	cwd := NewSessionCwd(dir)

	subdir := dir + "/subdir"
	mkdirAll(subdir)

	err := cwd.Set("session-1", subdir)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}

	if cwd.Get("session-1") != subdir {
		t.Errorf("expected %s, got %s", subdir, cwd.Get("session-1"))
	}
}

func TestSessionCwd_Clear(t *testing.T) {
	dir := t.TempDir()
	cwd := NewSessionCwd(dir)

	subdir := dir + "/subdir"
	mkdirAll(subdir)

	cwd.Set("session-1", subdir)
	cwd.Clear("session-1")

	if cwd.Get("session-1") != dir {
		t.Errorf("expected %s after clear, got %s", dir, cwd.Get("session-1"))
	}
}

func TestSessionCwd_ResolvePath(t *testing.T) {
	dir := t.TempDir()
	cwd := NewSessionCwd(dir)

	// Relative path
	relResult := cwd.ResolvePath("session-1", "relative/path")
	if relResult == "" {
		t.Error("expected non-empty result")
	}
}

func mkdirAll(path string) {
	os.MkdirAll(path, 0755)
}

// ─── Bash Interactive Tests ───────────────────────────────────────────────

func TestBashInteractiveService_List(t *testing.T) {
	s := NewBashInteractiveService()

	pending := s.List()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestBashInteractiveService_Cancel(t *testing.T) {
	s := NewBashInteractiveService()

	// Cancel non-existent request
	err := s.Cancel("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent request")
	}
}

// ─── Predict Service Tests ────────────────────────────────────────────────

func TestPredictService_Disabled(t *testing.T) {
	s := NewPredictService(false)

	result := s.Predict("test", "response")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestPredictService_Enabled(t *testing.T) {
	s := NewPredictService(true)

	result := s.Predict("create a file", "done")
	if result == "" {
		t.Error("expected non-empty prediction")
	}
}

func TestPredictService_GetLastPrediction(t *testing.T) {
	s := NewPredictService(true)

	s.Predict("what is Go?", "Go is a language")

	if s.GetLastPrediction() == "" {
		t.Error("expected non-empty last prediction")
	}
}

func TestPredictService_SetEnabled(t *testing.T) {
	s := NewPredictService(false)

	s.SetEnabled(true)
	if !s.IsEnabled() {
		t.Error("expected enabled")
	}
}
