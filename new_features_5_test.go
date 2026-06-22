package main

import (
	"testing"
	"time"
)

// ─── Reasoning Variants Tests ───────────────────────────────────────────────

func TestReasoningVariantService_New(t *testing.T) {
	s := NewReasoningVariantService()
	if s == nil {
		t.Error("expected non-nil service")
	}
}

func TestReasoningVariantService_GetVariant_Anthropic(t *testing.T) {
	s := NewReasoningVariantService()

	v := s.GetVariant("anthropic", "claude-sonnet-4-20250514", EffortMedium)
	if v == nil {
		t.Fatal("expected variant for Anthropic medium")
	}
	if v.Provider != "anthropic" {
		t.Errorf("expected anthropic, got %s", v.Provider)
	}
}

func TestReasoningVariantService_GetVariant_OpenAI(t *testing.T) {
	s := NewReasoningVariantService()

	v := s.GetVariant("openai", "o1", EffortHigh)
	if v == nil {
		t.Fatal("expected variant for OpenAI high")
	}
	if v.Provider != "openai" {
		t.Errorf("expected openai, got %s", v.Provider)
	}
}

func TestReasoningVariantService_GetVariant_Google(t *testing.T) {
	s := NewReasoningVariantService()

	v := s.GetVariant("google", "gemini-pro", EffortLow)
	if v == nil {
		t.Fatal("expected variant for Google low")
	}
}

func TestReasoningVariantService_GetVariant_NotFound(t *testing.T) {
	s := NewReasoningVariantService()

	v := s.GetVariant("unknown", "model", EffortMedium)
	if v != nil {
		t.Error("expected nil for unknown provider")
	}
}

func TestReasoningVariantService_GetParams(t *testing.T) {
	s := NewReasoningVariantService()

	params := s.GetParams("anthropic", "claude", EffortHigh)
	if params == nil {
		t.Fatal("expected params")
	}
}

func TestReasoningVariantService_ListVariants(t *testing.T) {
	s := NewReasoningVariantService()

	variants := s.ListVariants()
	if len(variants) == 0 {
		t.Error("expected non-empty variants")
	}
}

// ─── Actor Registry Tests ───────────────────────────────────────────────────

func TestActorRegistry_New(t *testing.T) {
	r := NewActorRegistry()
	if r == nil {
		t.Error("expected non-nil registry")
	}
}

func TestActorRegistry_Register(t *testing.T) {
	r := NewActorRegistry()

	actor := r.Register("actor-1", "session-1", ActorModeSubagent, false)
	if actor == nil {
		t.Fatal("expected non-nil actor")
	}
	if actor.ID != "actor-1" {
		t.Errorf("expected actor-1, got %s", actor.ID)
	}
	if actor.Status != ActorStatusRunning {
		t.Errorf("expected running, got %s", actor.Status)
	}
}

func TestActorRegistry_UpdateStatus(t *testing.T) {
	r := NewActorRegistry()

	r.Register("actor-1", "session-1", ActorModeSubagent, false)
	err := r.UpdateStatus("actor-1", ActorStatusCompleted)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	actor := r.GetActor("actor-1")
	if actor.Status != ActorStatusCompleted {
		t.Errorf("expected completed, got %s", actor.Status)
	}
}

func TestActorRegistry_RecordActivity(t *testing.T) {
	r := NewActorRegistry()

	r.Register("actor-1", "session-1", ActorModeSubagent, false)
	r.RecordActivity("actor-1")

	actor := r.GetActor("actor-1")
	if actor.LastActivity.IsZero() {
		t.Error("expected non-zero last activity")
	}
}

func TestActorRegistry_ListRunning(t *testing.T) {
	r := NewActorRegistry()

	r.Register("actor-1", "session-1", ActorModeSubagent, false)
	r.Register("actor-2", "session-2", ActorModeSubagent, false)
	r.UpdateStatus("actor-1", ActorStatusCompleted)

	running := r.ListRunning()
	if len(running) != 1 {
		t.Errorf("expected 1 running, got %d", len(running))
	}
}

func TestActorRegistry_IsSystemSpawned(t *testing.T) {
	r := NewActorRegistry()

	r.Register("actor-1", "session-1", ActorModeSubagent, true)
	r.Register("actor-2", "session-2", ActorModeSubagent, false)

	if !r.IsSystemSpawned("actor-1") {
		t.Error("expected actor-1 to be system spawned")
	}
	if r.IsSystemSpawned("actor-2") {
		t.Error("expected actor-2 to NOT be system spawned")
	}
}

func TestActorRegistry_DetectStuck(t *testing.T) {
	r := NewActorRegistry()

	actor := r.Register("actor-1", "session-1", ActorModeSubagent, false)
	// Set last activity to past
	actor.LastActivity = actor.LastActivity.Add(-10 * time.Minute)

	stuck := r.DetectStuck()
	if len(stuck) != 1 {
		t.Errorf("expected 1 stuck, got %d", len(stuck))
	}
}

func TestActorRegistry_Remove(t *testing.T) {
	r := NewActorRegistry()

	r.Register("actor-1", "session-1", ActorModeSubagent, false)
	r.Remove("actor-1")

	if r.Count() != 0 {
		t.Errorf("expected 0 actors, got %d", r.Count())
	}
}

func TestFormatActor(t *testing.T) {
	actor := &Actor{
		ID:      "actor-1",
		Mode:    ActorModeSubagent,
		Status:  ActorStatusRunning,
		TaskID:  "task-1",
	}

	output := FormatActor(actor)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatActor_Nil(t *testing.T) {
	output := FormatActor(nil)
	if output != "No actor." {
		t.Errorf("expected 'No actor.', got %q", output)
	}
}
