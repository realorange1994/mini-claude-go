package main

import (
	"testing"
)

func TestMemoryFTSIndex_BasicSearch(t *testing.T) {
	idx := NewMemoryFTSIndex()

	idx.Index(ScopeSession, "state", "Working on authentication module")
	idx.Index(ScopeSession, "decision", "Use Go 1.25 for all projects")
	idx.Index(ScopeSession, "preference", "Dark theme preferred")

	results := idx.Search("authentication", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if len(results) > 0 && results[0].Content != "Working on authentication module" {
		t.Errorf("expected 'Working on authentication module', got '%s'", results[0].Content)
	}
}

func TestMemoryFTSIndex_MultipleMatches(t *testing.T) {
	idx := NewMemoryFTSIndex()

	idx.Index(ScopeSession, "state", "Working on authentication module")
	idx.Index(ScopeSession, "decision", "Authentication approach decided")

	results := idx.Search("authentication", 10)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestMemoryFTSIndex_NoMatch(t *testing.T) {
	idx := NewMemoryFTSIndex()

	idx.Index(ScopeSession, "state", "Working on authentication")

	results := idx.Search("quantum physics", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestMemoryFTSIndex_Update(t *testing.T) {
	idx := NewMemoryFTSIndex()

	idx.Index(ScopeSession, "state", "Working on auth")
	idx.Index(ScopeSession, "state", "Working on authentication module")

	results := idx.Search("authentication", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result after update, got %d", len(results))
	}
	if len(results) > 0 && results[0].Content != "Working on authentication module" {
		t.Errorf("expected updated content, got '%s'", results[0].Content)
	}
}

func TestMemoryFTSIndex_Remove(t *testing.T) {
	idx := NewMemoryFTSIndex()

	idx.Index(ScopeSession, "state", "Working on auth")
	idx.Remove(ScopeSession, "state", "Working on auth")

	results := idx.Search("auth", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results after remove, got %d", len(results))
	}
}

func TestMemoryFTSIndex_Limit(t *testing.T) {
	idx := NewMemoryFTSIndex()

	idx.Index(ScopeSession, "state", "Task 1")
	idx.Index(ScopeSession, "state", "Task 2")
	idx.Index(ScopeSession, "state", "Task 3")

	results := idx.Search("task", 2)
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestMemoryFTSIndex_RebuildIndex(t *testing.T) {
	idx := NewMemoryFTSIndex()

	idx.Index(ScopeSession, "state", "Working on auth")
	idx.RebuildIndex()

	results := idx.Search("auth", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result after rebuild, got %d", len(results))
	}
}

func TestMemoryFTSIndex_Stats(t *testing.T) {
	idx := NewMemoryFTSIndex()

	idx.Index(ScopeSession, "state", "Working on auth")
	idx.Index(ScopeSession, "decision", "Use Go")

	stats := idx.Stats()
	if stats.TotalEntries != 2 {
		t.Errorf("expected 2 entries, got %d", stats.TotalEntries)
	}
	if stats.TotalTokens == 0 {
		t.Error("expected non-zero tokens")
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		minTokens int
	}{
		{"hello world", 2},
		{"the quick brown fox", 3}, // "the" is stop word
		{"", 0},
		{"a", 0}, // single char, stop word
	}

	for _, tt := range tests {
		tokens := tokenize(tt.input)
		if len(tokens) < tt.minTokens {
			t.Errorf("tokenize(%q) = %d tokens, want >= %d", tt.input, len(tokens), tt.minTokens)
		}
	}
}

func TestMemoryFTSIndex_CaseInsensitive(t *testing.T) {
	idx := NewMemoryFTSIndex()

	idx.Index(ScopeSession, "state", "Working on AUTHENTICATION")

	results := idx.Search("authentication", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result (case insensitive), got %d", len(results))
	}
}
