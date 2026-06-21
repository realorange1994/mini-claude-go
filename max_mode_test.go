package main

import (
	"testing"
)

func TestMaxModeConfig_Defaults(t *testing.T) {
	config := NewMaxModeConfig()

	if config.Enabled {
		t.Error("expected disabled by default")
	}
	if config.NumCandidates != DefaultCandidates {
		t.Errorf("expected %d candidates, got %d", DefaultCandidates, config.NumCandidates)
	}
}

func TestMaxModeService_IsEnabled(t *testing.T) {
	config := NewMaxModeConfig()
	s := NewMaxModeService(*config)

	if s.IsEnabled() {
		t.Error("expected disabled")
	}

	s.SetEnabled(true)
	if !s.IsEnabled() {
		t.Error("expected enabled")
	}
}

func TestMaxModeService_RunCandidates(t *testing.T) {
	config := NewMaxModeConfig()
	s := NewMaxModeService(*config)

	candidates := s.RunCandidates("test prompt", 3)
	if len(candidates) != 3 {
		t.Errorf("expected 3 candidates, got %d", len(candidates))
	}
}

func TestMaxModeService_RunCandidates_Clamp(t *testing.T) {
	config := NewMaxModeConfig()
	s := NewMaxModeService(*config)

	// Test min clamp
	candidates := s.RunCandidates("test prompt", 1)
	if len(candidates) < MinCandidates {
		t.Errorf("expected at least %d candidates, got %d", MinCandidates, len(candidates))
	}

	// Test max clamp
	candidates = s.RunCandidates("test prompt", 20)
	if len(candidates) > MaxCandidates {
		t.Errorf("expected at most %d candidates, got %d", MaxCandidates, len(candidates))
	}
}

func TestMaxModeService_SelectBest(t *testing.T) {
	config := NewMaxModeConfig()
	s := NewMaxModeService(*config)

	candidates := []Candidate{
		{ID: 0, Text: "short", Tokens: 10},
		{ID: 1, Text: "longer response with more content", Tokens: 30},
		{ID: 2, Text: "medium", Tokens: 20},
	}

	selected, verdict := s.SelectBest(candidates)
	if selected.ID != 1 {
		t.Errorf("expected candidate 1, got %d", selected.ID)
	}
	if verdict.SelectedID != 1 {
		t.Errorf("expected verdict ID 1, got %d", verdict.SelectedID)
	}
}

func TestMaxModeService_RunMaxMode_Disabled(t *testing.T) {
	config := NewMaxModeConfig()
	s := NewMaxModeService(*config)

	_, err := s.RunMaxMode("test prompt")
	if err == nil {
		t.Error("expected error when disabled")
	}
}

func TestMaxModeService_RunMaxMode_Enabled(t *testing.T) {
	config := NewMaxModeConfig()
	config.Enabled = true
	config.NumCandidates = 3
	s := NewMaxModeService(*config)

	result, err := s.RunMaxMode("test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Candidates) != 3 {
		t.Errorf("expected 3 candidates, got %d", len(result.Candidates))
	}
	if result.Overhead == 0 {
		t.Error("expected non-zero overhead")
	}
}

func TestFormatMaxModeResult(t *testing.T) {
	result := &MaxModeResult{
		Candidates: []Candidate{
			{ID: 0, Tokens: 10},
			{ID: 1, Tokens: 20},
		},
		Selected: Candidate{ID: 1},
		Verdict:  JudgeVerdict{SelectedID: 1, Reason: "best"},
		Overhead: 30,
	}

	output := FormatMaxModeResult(result)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatMaxModeResult_Nil(t *testing.T) {
	output := FormatMaxModeResult(nil)
	if output != "No max mode result." {
		t.Errorf("expected 'No max mode result.', got %q", output)
	}
}
