package main

import (
	"testing"
)

func TestShareService_New(t *testing.T) {
	config := ShareConfig{Enabled: true}
	s := NewShareService(config)
	if s == nil {
		t.Error("expected non-nil service")
	}
}

func TestShareService_Share_Disabled(t *testing.T) {
	config := ShareConfig{Enabled: false}
	s := NewShareService(config)

	_, err := s.Share("session-1", nil)
	if err == nil {
		t.Error("expected error when disabled")
	}
}

func TestShareService_IsShared(t *testing.T) {
	config := ShareConfig{Enabled: true}
	s := NewShareService(config)

	if s.IsShared("session-1") {
		t.Error("expected not shared initially")
	}
}

func TestShareService_GetShareState(t *testing.T) {
	config := ShareConfig{Enabled: true}
	s := NewShareService(config)

	state := s.GetShareState("session-1")
	if state != nil {
		t.Error("expected nil state initially")
	}
}

func TestShareService_GetShareURL(t *testing.T) {
	config := ShareConfig{Enabled: true}
	s := NewShareService(config)

	url := s.GetShareURL("session-1")
	if url != "" {
		t.Error("expected empty URL initially")
	}
}

func TestShareService_Unshare_NotShared(t *testing.T) {
	config := ShareConfig{Enabled: true}
	s := NewShareService(config)

	err := s.Unshare("session-1")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestShareService_Sync_NotShared(t *testing.T) {
	config := ShareConfig{Enabled: true}
	s := NewShareService(config)

	err := s.Sync("session-1", map[string]any{"key": "value"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestFormatShareState(t *testing.T) {
	state := &ShareState{
		SessionID: "session-1",
		ShareID:   "share-1",
		ShareURL:  "https://example.com/share/share-1",
	}

	output := FormatShareState(state)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatShareState_Nil(t *testing.T) {
	output := FormatShareState(nil)
	if output != "Session not shared." {
		t.Errorf("expected 'Session not shared.', got %q", output)
	}
}
