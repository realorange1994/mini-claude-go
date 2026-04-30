package skills

import (
	"sort"
	"testing"
)

func TestGetReadSkillNames(t *testing.T) {
	tracker := NewSkillTracker()

	// No skills read yet
	names := tracker.GetReadSkillNames()
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}

	// Mark some skills as read
	tracker.MarkRead("commit")
	tracker.MarkRead("review")
	tracker.MarkRead("test")

	names = tracker.GetReadSkillNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}

	// Sort for deterministic comparison
	sort.Strings(names)
	expected := []string{"commit", "review", "test"}
	for i, n := range expected {
		if names[i] != n {
			t.Errorf("expected names[%d] = %q, got %q", i, n, names[i])
		}
	}

	// ReadCount should match
	if tracker.ReadCount() != 3 {
		t.Errorf("expected ReadCount 3, got %d", tracker.ReadCount())
	}
}
