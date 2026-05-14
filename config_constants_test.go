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
