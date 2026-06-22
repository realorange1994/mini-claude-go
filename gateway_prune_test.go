package main

import (
	"testing"
)

// ─── Gateway Error Tests ────────────────────────────────────────────────────

func TestGatewayError_Error(t *testing.T) {
	err := NewGatewayError(421, "blocked", "reason: content")
	if err.Error() != "blocked: reason: content" {
		t.Errorf("expected 'blocked: reason: content', got %q", err.Error())
	}
}

func TestGatewayError_Error_NoParam(t *testing.T) {
	err := NewGatewayError(421, "blocked", "")
	if err.Error() != "blocked" {
		t.Errorf("expected 'blocked', got %q", err.Error())
	}
}

func TestIsGatewayError(t *testing.T) {
	err := NewGatewayError(421, "blocked", "")
	if !IsGatewayError(err) {
		t.Error("expected gateway error")
	}
	if IsGatewayError(nil) {
		t.Error("expected false for nil")
	}
}

func TestFormatGatewayError_Moderation(t *testing.T) {
	err := NewGatewayError(GatewayCodeContentModeration, "blocked", "")
	msg := FormatGatewayError(err)
	if msg == "" {
		t.Error("expected non-empty message")
	}
}

func TestFormatGatewayError_RiskControl(t *testing.T) {
	err := NewGatewayError(GatewayCodeRiskControl, "blocked", "")
	msg := FormatGatewayError(err)
	if msg == "" {
		t.Error("expected non-empty message")
	}
}

func TestFormatGatewayError_Nil(t *testing.T) {
	msg := FormatGatewayError(nil)
	if msg != "" {
		t.Errorf("expected empty, got %q", msg)
	}
}

// ─── Session Prune Service Tests ────────────────────────────────────────────

func TestSessionPruneService_New(t *testing.T) {
	s := NewSessionPruneService(PruneConfig{})
	if s == nil {
		t.Error("expected non-nil service")
	}
}

func TestSessionPruneService_ShouldPrune_Disabled(t *testing.T) {
	s := NewSessionPruneService(PruneConfig{Enabled: false})
	if s.ShouldPrune("s1", 100000) {
		t.Error("expected no prune when disabled")
	}
}

func TestSessionPruneService_ShouldPrune_Threshold(t *testing.T) {
	s := NewSessionPruneService(PruneConfig{
		Enabled:    true,
		Thresholds: []int{1000, 5000, 10000},
	})

	if s.ShouldPrune("s1", 500) {
		t.Error("expected no prune below threshold")
	}
	if !s.ShouldPrune("s1", 1500) {
		t.Error("expected prune above threshold")
	}
}

func TestSessionPruneService_IsCacheCold(t *testing.T) {
	s := NewSessionPruneService(PruneConfig{CacheTTL: 1000})

	if !s.IsCacheCold("s1") {
		t.Error("expected cold initially")
	}

	s.RecordActivity("s1")
	if s.IsCacheCold("s1") {
		t.Error("expected warm after activity")
	}
}

func TestSessionPruneService_GetPruneLevel(t *testing.T) {
	s := NewSessionPruneService(PruneConfig{
		Enabled:    true,
		Thresholds: []int{1000, 5000, 10000},
	})

	if s.GetPruneLevel("s1", 500) != 0 {
		t.Error("expected level 0")
	}
	if s.GetPruneLevel("s1", 2000) != 1 {
		t.Error("expected level 1")
	}
}

func TestSessionPruneService_Reset(t *testing.T) {
	s := NewSessionPruneService(PruneConfig{
		Enabled:    true,
		Thresholds: []int{1000},
	})

	s.ShouldPrune("s1", 1500)
	s.Reset("s1")

	// Should prune again after reset
	if !s.ShouldPrune("s1", 1500) {
		t.Error("expected prune after reset")
	}
}

func TestFormatPruneStatus(t *testing.T) {
	s := NewSessionPruneService(PruneConfig{Enabled: true})
	status := FormatPruneStatus(s, "s1")
	if status == "" {
		t.Error("expected non-empty status")
	}
}

func TestFormatPruneStatus_Nil(t *testing.T) {
	status := FormatPruneStatus(nil, "s1")
	if status != "No prune service." {
		t.Errorf("expected 'No prune service.', got %q", status)
	}
}
