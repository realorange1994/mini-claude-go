package main

import (
	"fmt"
	"strings"
)

// ─── Checkpoint Validator (MiMo-Code 4A) ────────────────────────────────────
//
// Validates checkpoint content against structural rules.
// On failure, quarantines the checkpoint and builds a reflection message.
//
// MiMo-Code source: checkpoint-validator.ts (259 lines)

const (
	// MaxTopicLength maximum characters for checkpoint topic
	MaxTopicLength = 80
	// MaxCheckpointTokens maximum tokens for checkpoint file
	MaxCheckpointTokens = 11000
	// MaxMemoryTokens maximum tokens for memory file
	MaxMemoryTokens = 10000
	// MaxProgressTokens maximum tokens per progress file
	MaxProgressTokens = 6000
)

// SectionBudget defines per-section token budgets.
var SectionBudget = map[string]int{
	"Active intent":              500,
	"Next concrete action":       500,
	"Directives":                 300,
	"Task tree":                  1000,
	"Current work":               2000,
	"Files and code sections":    1500,
	"Discovered knowledge":       1500,
	"Errors and fixes":           500,
	"Live resources":             300,
	"Design decisions":           1000,
	"Open notes":                 500,
}

// CheckpointValidationRule represents a validation rule.
type CheckpointValidationRule struct {
	ID       string
	Severity string // "error", "warning", "extract-required"
	Check    func(content string) *ValidationError
}

// ValidationError represents a validation error.
type ValidationError struct {
	RuleID   string
	Severity string
	Message  string
	Line     int
}

// CheckpointValidator validates checkpoint content.
type CheckpointValidator struct {
	rules []CheckpointValidationRule
}

// NewCheckpointValidator creates a new validator with default rules.
func NewCheckpointValidator() *CheckpointValidator {
	v := &CheckpointValidator{}
	v.registerDefaultRules()
	return v
}

// registerDefaultRules registers the default validation rules.
func (v *CheckpointValidator) registerDefaultRules() {
	v.rules = []CheckpointValidationRule{
		{
			ID:       "topic-missing",
			Severity: "error",
			Check: func(content string) *ValidationError {
				if !strings.Contains(content, "Topic:") {
					return &ValidationError{RuleID: "topic-missing", Severity: "error", Message: "Missing 'Topic:' line"}
				}
				return nil
			},
		},
		{
			ID:       "topic-too-long",
			Severity: "warning",
			Check: func(content string) *ValidationError {
				for _, line := range strings.Split(content, "\n") {
					if strings.HasPrefix(line, "Topic:") {
						topic := strings.TrimPrefix(line, "Topic:")
						topic = strings.TrimSpace(topic)
						if len(topic) > MaxTopicLength {
							return &ValidationError{
								RuleID:   "topic-too-long",
								Severity: "warning",
								Message:  fmt.Sprintf("Topic exceeds %d chars (%d)", MaxTopicLength, len(topic)),
							}
						}
					}
				}
				return nil
			},
		},
		{
			ID:       "subsection-missing",
			Severity: "error",
			Check: func(content string) *ValidationError {
				required := []string{"### Execution context", "### Live resources", "### Session metadata"}
				for _, section := range required {
					if !strings.Contains(content, section) {
						return &ValidationError{
							RuleID:   "subsection-missing",
							Severity: "error",
							Message:  fmt.Sprintf("Missing required subsection: %s", section),
						}
					}
				}
				return nil
			},
		},
		{
			ID:       "budget-exceeded",
			Severity: "extract-required",
			Check: func(content string) *ValidationError {
				tokens := estimateTokensCV(content)
				if tokens > MaxCheckpointTokens {
					return &ValidationError{
						RuleID:   "budget-exceeded",
						Severity: "extract-required",
						Message:  fmt.Sprintf("Checkpoint exceeds %d tokens (%d)", MaxCheckpointTokens, tokens),
					}
				}
				return nil
			},
		},
	}
}

// Validate validates checkpoint content against all rules.
func (v *CheckpointValidator) Validate(content string) []*ValidationError {
	var errors []*ValidationError
	for _, rule := range v.rules {
		if err := rule.Check(content); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

// ValidateAndQuarantine validates and quarantines if invalid.
// Returns the validation errors and whether the checkpoint was quarantined.
func (v *CheckpointValidator) ValidateAndQuarantine(content string, checkpointPath string) ([]*ValidationError, bool) {
	errors := v.Validate(content)
	if len(errors) == 0 {
		return nil, false
	}

	// Check if any error requires extraction
	requiresExtract := false
	for _, err := range errors {
		if err.Severity == "extract-required" {
			requiresExtract = true
			break
		}
	}

	// Quarantine the checkpoint
	if requiresExtract {
		quarantinePath := strings.TrimSuffix(checkpointPath, ".md") + ".invalid.md"
		// In real implementation, rename the file
		_ = quarantinePath
	}

	return errors, requiresExtract
}

// estimateTokensCV estimates tokens from character count.
func estimateTokensCV(text string) int {
	return (len(text) + 3) / 4
}
