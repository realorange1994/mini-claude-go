package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ─── Subagent Progress Checker (MiMo-Code 3) ───────────────────────────────
//
// Validates that subagents write structured progress journals before exiting.
// Ensures subagent work is always documented for checkpoint writer.
//
// MiMo-Code source: plugin/subagent-progress-checker.ts (147 lines)

// ProgressChecker validates subagent progress files.
type ProgressChecker struct {
	taskDir string
}

// NewProgressChecker creates a new progress checker.
func NewProgressChecker(taskDir string) *ProgressChecker {
	return &ProgressChecker{taskDir: taskDir}
}

// RequiredSections defines the required sections in a progress file.
var RequiredSections = []string{
	"Task identity",
	"Subagent intent",
	"Files and code sections",
	"Outcome and discoveries",
}

// ValidateProgress validates a progress file for completeness.
func (c *ProgressChecker) ValidateProgress(taskID string) (*ProgressValidation, error) {
	path := filepath.Join(c.taskDir, taskID, "progress.md")

	data, err := os.ReadFile(path)
	if err != nil {
		return &ProgressValidation{
			TaskID:  taskID,
			Valid:   false,
			Reason:  "Progress file not found",
			Missing: RequiredSections,
		}, nil
	}

	content := string(data)
	var missing []string

	for _, section := range RequiredSections {
		if !strings.Contains(content, section) {
			missing = append(missing, section)
		}
	}

	if len(missing) > 0 {
		return &ProgressValidation{
			TaskID:  taskID,
			Valid:   false,
			Reason:  "Missing required sections",
			Missing: missing,
		}, nil
	}

	return &ProgressValidation{
		TaskID: taskID,
		Valid:  true,
	}, nil
}

// ProgressValidation represents the result of progress validation.
type ProgressValidation struct {
	TaskID  string   `json:"task_id"`
	Valid   bool     `json:"valid"`
	Reason  string   `json:"reason,omitempty"`
	Missing []string `json:"missing,omitempty"`
}

// WriteProgressTemplate writes a progress template for a task.
func (c *ProgressChecker) WriteProgressTemplate(taskID string) error {
	dir := filepath.Join(c.taskDir, taskID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	template := fmt.Sprintf(`---
task_id: %s
written_at: %s
---

## Task identity
<!-- Task ID and subject -->

## Subagent intent
<!-- What the subagent was asked to do -->

## Files and code sections
<!-- Files read or modified -->

## Verbatim commands
<!-- Commands executed -->

## Outcome and discoveries
<!-- What was accomplished and learned -->
`, taskID, time.Now().Format(time.RFC3339))

	path := filepath.Join(dir, "progress.md")
	return os.WriteFile(path, []byte(template), 0644)
}

// FormatValidation formats a progress validation for display.
func FormatValidation(v *ProgressValidation) string {
	if v.Valid {
		return fmt.Sprintf("✓ Progress valid for task %s", v.TaskID)
	}

	var sb string
	sb += fmt.Sprintf("✗ Progress invalid for task %s: %s\n", v.TaskID, v.Reason)
	if len(v.Missing) > 0 {
		sb += "Missing sections:\n"
		for _, s := range v.Missing {
			sb += "  - " + s + "\n"
		}
	}
	return sb
}
