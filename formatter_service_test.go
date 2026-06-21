package main

import (
	"testing"
)

func TestFormatterService_New(t *testing.T) {
	s := NewFormatterService(true)
	if s == nil {
		t.Error("expected non-nil service")
	}
}

func TestFormatterService_IsEnabled(t *testing.T) {
	s := NewFormatterService(true)
	if !s.IsEnabled() {
		t.Error("expected enabled")
	}

	s.SetEnabled(false)
	if s.IsEnabled() {
		t.Error("expected disabled")
	}
}

func TestFormatterService_GetAvailable(t *testing.T) {
	s := NewFormatterService(true)
	available := s.GetAvailable()
	// gofmt should be available on most systems
	found := false
	for _, name := range available {
		if name == "gofmt" {
			found = true
			break
		}
	}
	if !found {
		t.Log("gofmt not found (may not be installed)")
	}
}

func TestFormatterService_FormatFile_Disabled(t *testing.T) {
	s := NewFormatterService(false)
	err := s.FormatFile("test.go")
	if err != nil {
		t.Errorf("expected no error when disabled, got %v", err)
	}
}

func TestFormatterService_MatchesExtension(t *testing.T) {
	s := NewFormatterService(true)

	tests := []struct {
		ext        string
		extensions []string
		expected   bool
	}{
		{".go", []string{".go"}, true},
		{".js", []string{".js", ".ts"}, true},
		{".py", []string{".go"}, false},
	}

	for _, tt := range tests {
		result := s.matchesExtension(tt.ext, tt.extensions)
		if result != tt.expected {
			t.Errorf("matchesExtension(%s, %v) = %v, want %v", tt.ext, tt.extensions, result, tt.expected)
		}
	}
}

func TestFormatterService_AddProfile(t *testing.T) {
	s := NewFormatterService(true)
	s.AddProfile(FormatterProfile{
		Name:       "custom",
		Command:    "custom-fmt",
		Extensions: []string{".custom"},
	})

	available := s.GetAvailable()
	found := false
	for _, name := range available {
		if name == "custom" {
			found = true
			break
		}
	}
	if found {
		t.Error("expected custom formatter to not be available (not installed)")
	}
}

func TestFormatterService_FormatStatus(t *testing.T) {
	s := NewFormatterService(true)
	status := s.FormatStatus()
	if status == "" {
		t.Error("expected non-empty status")
	}
}
