package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExternalImportService_New(t *testing.T) {
	config := ExternalImportConfig{Enabled: true}
	s := NewExternalImportService(config)
	if s == nil {
		t.Error("expected non-nil service")
	}
}

func TestExternalImportService_ScanClaudeCode_NoDir(t *testing.T) {
	config := ExternalImportConfig{
		Enabled:       true,
		ClaudeCodeDir: filepath.Join(t.TempDir(), "nonexistent"),
	}
	s := NewExternalImportService(config)

	sessions, err := s.ScanClaudeCode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil sessions, got %d", len(sessions))
	}
}

func TestExternalImportService_ScanCodex_NoDir(t *testing.T) {
	config := ExternalImportConfig{
		Enabled:  true,
		CodexDir: filepath.Join(t.TempDir(), "nonexistent"),
	}
	s := NewExternalImportService(config)

	sessions, err := s.ScanCodex()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil sessions, got %d", len(sessions))
	}
}

func TestExternalImportService_ScanClaudeCode_WithFiles(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project1")
	os.MkdirAll(projectDir, 0755)

	// Create a mock JSONL session file
	sessionFile := filepath.Join(projectDir, "session1.jsonl")
	content := `{"role": "user", "content": "hello"}
{"role": "assistant", "content": "hi there"}
`
	os.WriteFile(sessionFile, []byte(content), 0644)

	config := ExternalImportConfig{
		Enabled:       true,
		ClaudeCodeDir: dir,
	}
	s := NewExternalImportService(config)

	sessions, err := s.ScanClaudeCode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
	if len(sessions) > 0 && len(sessions[0].Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(sessions[0].Messages))
	}
}

func TestExternalImportService_ImportSession(t *testing.T) {
	config := ExternalImportConfig{Enabled: true}
	s := NewExternalImportService(config)

	session := ExternalSession{
		ID:       "test-session",
		Source:   SourceClaudeCode,
		Messages: []ExternalMessage{{Role: "user", Content: "hello"}},
	}

	err := s.ImportSession(session)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if !s.IsImported("test-session") {
		t.Error("expected session to be imported")
	}
	if s.GetImportedCount() != 1 {
		t.Errorf("expected 1 imported, got %d", s.GetImportedCount())
	}
}

func TestExternalImportService_ImportSession_SkipDuplicate(t *testing.T) {
	config := ExternalImportConfig{Enabled: true}
	s := NewExternalImportService(config)

	session := ExternalSession{
		ID:       "test-session",
		Source:   SourceClaudeCode,
		Messages: []ExternalMessage{{Role: "user", Content: "hello"}},
	}

	s.ImportSession(session)
	s.ImportSession(session) // Second import should be skipped

	if s.GetImportedCount() != 1 {
		t.Errorf("expected 1 imported, got %d", s.GetImportedCount())
	}
}

func TestExternalImportService_ScanAll(t *testing.T) {
	config := ExternalImportConfig{
		Enabled:       true,
		ClaudeCodeDir: filepath.Join(t.TempDir(), "nonexistent"),
		CodexDir:      filepath.Join(t.TempDir(), "nonexistent"),
	}
	s := NewExternalImportService(config)

	sessions, err := s.ScanAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil sessions, got %d", len(sessions))
	}
}

func TestFormatImportSummary(t *testing.T) {
	sessions := []ExternalSession{
		{Source: SourceClaudeCode, Title: "Test Session", Messages: []ExternalMessage{{Role: "user", Content: "hello"}}},
	}

	output := FormatImportSummary(sessions)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatImportSummary_Empty(t *testing.T) {
	output := FormatImportSummary(nil)
	if output != "No external sessions found." {
		t.Errorf("expected 'No external sessions found.', got %q", output)
	}
}
