package main

import (
	"testing"
)

// Ported from upstream: src/utils/__tests__/configConstants.test.ts
// Tests for NOTIFICATION_CHANNELS, EDITOR_MODES, TEAMMATE_MODES

func TestNotificationChannelsContains(t *testing.T) {
	required := []string{
		"auto",
		"iterm2",
		"iterm2_with_bell",
		"terminal_bell",
		"kitty",
		"ghostty",
		"notifications_disabled",
	}
	for _, r := range required {
		t.Run(r, func(t *testing.T) {
			if !sliceContains(NotificationChannels, r) {
				t.Errorf("NotificationChannels missing %q", r)
			}
		})
	}
}

func TestEditorModesContains(t *testing.T) {
	required := []string{
		"normal",
		"vim",
	}
	for _, r := range required {
		t.Run(r, func(t *testing.T) {
			if !sliceContains(EditorModes, r) {
				t.Errorf("EditorModes missing %q", r)
			}
		})
	}
}

func TestTeammateModesContains(t *testing.T) {
	required := []string{
		"auto",
		"tmux",
		"in-process",
	}
	for _, r := range required {
		t.Run(r, func(t *testing.T) {
			if !sliceContains(TeammateModes, r) {
				t.Errorf("TeammateModes missing %q", r)
			}
		})
	}
}

func TestNotificationChannelsNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, ch := range NotificationChannels {
		if seen[ch] {
			t.Errorf("duplicate entry in NotificationChannels: %q", ch)
		}
		seen[ch] = true
	}
}

func TestEditorModesNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, m := range EditorModes {
		if seen[m] {
			t.Errorf("duplicate entry in EditorModes: %q", m)
		}
		seen[m] = true
	}
}

func TestTeammateModesNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, m := range TeammateModes {
		if seen[m] {
			t.Errorf("duplicate entry in TeammateModes: %q", m)
		}
		seen[m] = true
	}
}

func sliceContains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// ─── Upstream Quality: Exact-value and ordering tests ─────────────────────────
// Ported from upstream: configConstants.test.ts exact-value and ordering tests

func TestEditorModesExactLength(t *testing.T) {
	// From upstream: "has exactly 2 entries"
	if len(EditorModes) != 2 {
		t.Fatalf("expected EditorModes to have exactly 2 entries, got %d", len(EditorModes))
	}
}

func TestEditorModesOrdering(t *testing.T) {
	// From upstream: "is ordered: normal, vim"
	if len(EditorModes) >= 2 {
		if EditorModes[0] != "normal" {
			t.Errorf("EditorModes[0]: expected 'normal', got %q", EditorModes[0])
		}
		if EditorModes[1] != "vim" {
			t.Errorf("EditorModes[1]: expected 'vim', got %q", EditorModes[1])
		}
	}
}

func TestTeammateModesExactLength(t *testing.T) {
	// From upstream: "has exactly 3 entries"
	if len(TeammateModes) != 3 {
		t.Fatalf("expected TeammateModes to have exactly 3 entries, got %d", len(TeammateModes))
	}
}

func TestTeammateModesOrdering(t *testing.T) {
	// From upstream: "is ordered: auto, tmux, in-process"
	if len(TeammateModes) >= 3 {
		if TeammateModes[0] != "auto" {
			t.Errorf("TeammateModes[0]: expected 'auto', got %q", TeammateModes[0])
		}
		if TeammateModes[1] != "tmux" {
			t.Errorf("TeammateModes[1]: expected 'tmux', got %q", TeammateModes[1])
		}
		if TeammateModes[2] != "in-process" {
			t.Errorf("TeammateModes[2]: expected 'in-process', got %q", TeammateModes[2])
		}
	}
}

func TestNotificationChannelsExactLength(t *testing.T) {
	// From upstream: length assertion pattern
	expected := []string{
		"auto",
		"iterm2",
		"iterm2_with_bell",
		"terminal_bell",
		"kitty",
		"ghostty",
		"notifications_disabled",
	}
	if len(NotificationChannels) != len(expected) {
		t.Fatalf("expected NotificationChannels to have %d entries, got %d", len(expected), len(NotificationChannels))
	}
}

func TestNotificationChannelsExactValues(t *testing.T) {
	// From upstream: exact value array match
	expected := []string{
		"auto",
		"iterm2",
		"iterm2_with_bell",
		"terminal_bell",
		"kitty",
		"ghostty",
		"notifications_disabled",
	}
	for i, ch := range NotificationChannels {
		if ch != expected[i] {
			t.Errorf("NotificationChannels[%d]: expected %q, got %q", i, expected[i], ch)
		}
	}
}
