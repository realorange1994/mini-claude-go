package main

import (
	"strings"
	"testing"
)

func TestLSPManager_New(t *testing.T) {
	m := NewLSPManager(true)
	if m == nil {
		t.Error("expected non-nil manager")
	}
	if !m.IsEnabled() {
		t.Error("expected enabled")
	}
}

func TestLSPManager_Disabled(t *testing.T) {
	m := NewLSPManager(false)
	if m.IsEnabled() {
		t.Error("expected disabled")
	}
}

func TestLSPManager_SetEnabled(t *testing.T) {
	m := NewLSPManager(false)
	m.SetEnabled(true)
	if !m.IsEnabled() {
		t.Error("expected enabled")
	}
}

func TestLSPManager_GetRunningServers(t *testing.T) {
	m := NewLSPManager(true)
	running := m.GetRunningServers()
	if len(running) != 0 {
		t.Errorf("expected 0 running servers, got %d", len(running))
	}
}

func TestLSPManager_GetDiagnostics(t *testing.T) {
	m := NewLSPManager(true)
	diags := m.GetDiagnostics("test.go")
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestLSPManager_ClearDiagnostics(t *testing.T) {
	m := NewLSPManager(true)
	m.diagnostics = []LSPDiagnostic{
		{File: "test.go", Line: 1, Severity: "error", Message: "test"},
	}
	m.ClearDiagnostics()
	if len(m.GetAllDiagnostics()) != 0 {
		t.Error("expected 0 diagnostics after clear")
	}
}

func TestLSPManager_ClearFileDiagnostics(t *testing.T) {
	m := NewLSPManager(true)
	m.diagnostics = []LSPDiagnostic{
		{File: "test1.go", Line: 1, Severity: "error", Message: "test1"},
		{File: "test2.go", Line: 2, Severity: "warning", Message: "test2"},
	}
	m.ClearFileDiagnostics("test1.go")
	diags := m.GetAllDiagnostics()
	if len(diags) != 1 {
		t.Errorf("expected 1 diagnostic, got %d", len(diags))
	}
	if len(diags) > 0 && diags[0].File != "test2.go" {
		t.Errorf("expected test2.go, got %s", diags[0].File)
	}
}

func TestLSPManager_MatchesExtension(t *testing.T) {
	m := NewLSPManager(true)

	tests := []struct {
		ext        string
		extensions []string
		expected   bool
	}{
		{".go", []string{".go"}, true},
		{".ts", []string{".ts", ".tsx"}, true},
		{".py", []string{".go"}, false},
	}

	for _, tt := range tests {
		result := m.matchesExtension(tt.ext, tt.extensions)
		if result != tt.expected {
			t.Errorf("matchesExtension(%s, %v) = %v, want %v", tt.ext, tt.extensions, result, tt.expected)
		}
	}
}

func TestLSPManager_StopAll(t *testing.T) {
	m := NewLSPManager(true)
	m.StopAll() // Should not panic
}

func TestFormatDiagnostics(t *testing.T) {
	diags := []LSPDiagnostic{
		{File: "test.go", Line: 1, Column: 5, Severity: "error", Message: "undefined variable"},
		{File: "test.go", Line: 10, Column: 1, Severity: "warning", Message: "unused import"},
	}

	output := FormatDiagnostics(diags)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatDiagnostics_Empty(t *testing.T) {
	output := FormatDiagnostics(nil)
	if output != "No diagnostics." {
		t.Errorf("expected 'No diagnostics.', got %q", output)
	}
}

func TestDiagnosticToHint(t *testing.T) {
	tests := []struct {
		diags    []LSPDiagnostic
		contains string
	}{
		{nil, ""},
		{[]LSPDiagnostic{{Severity: "error"}}, "errors"},
		{[]LSPDiagnostic{{Severity: "warning"}}, "warnings"},
		{[]LSPDiagnostic{{Severity: "info"}}, ""},
	}

	for _, tt := range tests {
		result := DiagnosticToHint(tt.diags)
		if tt.contains != "" && !strings.Contains(result, tt.contains) {
			t.Errorf("expected to contain %q, got %q", tt.contains, result)
		}
	}
}
