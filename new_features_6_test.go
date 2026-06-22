package main

import (
	"strings"
	"testing"
)

// ─── MCP OAuth Tests ────────────────────────────────────────────────────────

func TestMCPOAuthProvider_New(t *testing.T) {
	dir := t.TempDir()
	p := NewMCPOAuthProvider("test-server", dir)
	if p == nil {
		t.Error("expected non-nil provider")
	}
}

func TestMCPOAuthProvider_SetConfig(t *testing.T) {
	dir := t.TempDir()
	p := NewMCPOAuthProvider("test-server", dir)

	config := OAuthConfig{
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
		AuthURL:     "https://example.com/auth",
		TokenURL:    "https://example.com/token",
	}
	p.SetConfig(config)

	// Config should be set (no error)
}

func TestMCPOAuthProvider_GetAuthURL(t *testing.T) {
	dir := t.TempDir()
	p := NewMCPOAuthProvider("test-server", dir)

	config := OAuthConfig{
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
		AuthURL:     "https://example.com/auth",
	}
	p.SetConfig(config)

	url := p.GetAuthURL("test-state")
	if url == "" {
		t.Error("expected non-empty URL")
	}
	if !strings.Contains(url, "test-client") {
		t.Error("expected URL to contain client ID")
	}
}

func TestMCPOAuthProvider_GetToken(t *testing.T) {
	dir := t.TempDir()
	p := NewMCPOAuthProvider("test-server", dir)

	token := p.GetToken()
	if token != nil {
		t.Error("expected nil token initially")
	}
}

func TestMCPOAuthProvider_IsExpired(t *testing.T) {
	dir := t.TempDir()
	p := NewMCPOAuthProvider("test-server", dir)

	if !p.IsExpired() {
		t.Error("expected expired when no token")
	}
}

func TestMCPOAuthProvider_Invalidate(t *testing.T) {
	dir := t.TempDir()
	p := NewMCPOAuthProvider("test-server", dir)

	p.Invalidate() // Should not panic
}

func TestFormatOAuthStatus(t *testing.T) {
	dir := t.TempDir()
	p := NewMCPOAuthProvider("test-server", dir)

	status := FormatOAuthStatus(p)
	if status != "Not authenticated." {
		t.Errorf("expected 'Not authenticated.', got %q", status)
	}
}

// ─── Checkpoint Child Session Tests ─────────────────────────────────────────

func TestCheckpointChildSession_New(t *testing.T) {
	writer := NewCheckpointWriter(t.TempDir())
	s := NewCheckpointChildSession("parent-1", writer)
	if s == nil {
		t.Error("expected non-nil session")
	}
}

func TestCheckpointChildSession_IsEnabled(t *testing.T) {
	writer := NewCheckpointWriter(t.TempDir())
	s := NewCheckpointChildSession("parent-1", writer)

	if !s.IsEnabled() {
		t.Error("expected enabled by default")
	}
}

func TestCheckpointChildSession_SetEnabled(t *testing.T) {
	writer := NewCheckpointWriter(t.TempDir())
	s := NewCheckpointChildSession("parent-1", writer)

	s.SetEnabled(false)
	if s.IsEnabled() {
		t.Error("expected disabled")
	}
}

func TestCheckpointChildSession_GetLastCheckpointID(t *testing.T) {
	writer := NewCheckpointWriter(t.TempDir())
	s := NewCheckpointChildSession("parent-1", writer)

	if s.GetLastCheckpointID() != "" {
		t.Error("expected empty checkpoint ID initially")
	}
}

func TestCheckpointChildSession_Submit_Disabled(t *testing.T) {
	writer := NewCheckpointWriter(t.TempDir())
	s := NewCheckpointChildSession("parent-1", writer)
	s.SetEnabled(false)

	s.Submit(CheckpointRequest{}) // Should not panic
}

func TestFormatCheckpointChildStatus(t *testing.T) {
	writer := NewCheckpointWriter(t.TempDir())
	s := NewCheckpointChildSession("parent-1", writer)

	status := FormatCheckpointChildStatus(s)
	if status == "" {
		t.Error("expected non-empty status")
	}
}

func TestFormatCheckpointChildStatus_Nil(t *testing.T) {
	status := FormatCheckpointChildStatus(nil)
	if status != "No checkpoint child session." {
		t.Errorf("expected 'No checkpoint child session.', got %q", status)
	}
}
