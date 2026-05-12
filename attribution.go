package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// Attribution tracks which model contributed to each file edit.
// This is useful for organizations that need to track AI contributions.
type Attribution struct {
	Model string
}

// NewAttribution creates an attribution tracker for the given model.
func NewAttribution(model string) *Attribution {
	return &Attribution{Model: sanitizeModelName(model)}
}

// sanitizeModelName cleans a model name for use in git notes.
// Removes version hashes and normalizes format.
func sanitizeModelName(model string) string {
	// Remove version hash suffix (e.g., claude-sonnet-4-20250514 -> claude-sonnet-4)
	parts := strings.Split(model, "-")
	// Check if last part looks like a date (8+ digits)
	if len(parts) > 0 && len(parts[len(parts)-1]) >= 8 {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, "-")
}

// SetGitNote attaches an attribution note to the most recent commit.
// Uses git notes to store model attribution without modifying the commit.
func (a *Attribution) SetGitNote() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not available for attribution")
	}

	note := fmt.Sprintf("AI-attribution: model=%s", a.Model)

	// Try to add git note (requires git to be initialized)
	cmd := exec.Command("git", "notes", "add", "-m", note, "HEAD")
	if err := cmd.Run(); err != nil {
		// Notes ref may not exist yet, try append
		cmd = exec.Command("git", "notes", "append", "-m", note, "HEAD")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git notes failed: %w", err)
		}
	}
	return nil
}

// GetAttribution retrieves the attribution note for a commit.
func GetAttribution(commitRef string) string {
	cmd := exec.Command("git", "notes", "show", commitRef)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// FormatAttribution formats an attribution string for display.
func FormatAttribution(model string, files []string) string {
	if len(files) == 0 {
		return fmt.Sprintf("Model: %s", sanitizeModelName(model))
	}
	return fmt.Sprintf("Model: %s | Files: %s", sanitizeModelName(model), strings.Join(files, ", "))
}