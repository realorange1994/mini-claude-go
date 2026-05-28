package agent

import "fmt"

// ThinkingLevel represents the level of reasoning/thinking effort the model should use.
// Aligned to TS ThinkingLevel type.
type ThinkingLevel string

const (
	ThinkingLevelOff      ThinkingLevel = "off"
	ThinkingLevelMinimal  ThinkingLevel = "minimal"
	ThinkingLevelLow      ThinkingLevel = "low"
	ThinkingLevelMedium   ThinkingLevel = "medium"
	ThinkingLevelHigh     ThinkingLevel = "high"
)

// ValidThinkingLevels returns all valid thinking levels in order.
func ValidThinkingLevels() []ThinkingLevel {
	return []ThinkingLevel{ThinkingLevelOff, ThinkingLevelMinimal, ThinkingLevelLow, ThinkingLevelMedium, ThinkingLevelHigh}
}

// IsValid returns true if the level is a valid thinking level.
func (l ThinkingLevel) IsValid() bool {
	for _, v := range ValidThinkingLevels() {
		if l == v {
			return true
		}
	}
	return false
}

// ThinkingBudget maps thinking level to budget_tokens for extended thinking API requests.
// Aligned to TS thinking level token budget mapping.
func (l ThinkingLevel) BudgetTokens() int {
	switch l {
	case ThinkingLevelMinimal:
		return 1024
	case ThinkingLevelLow:
		return 4096
	case ThinkingLevelMedium:
		return 16384
	case ThinkingLevelHigh:
		return 32768
	default:
		return 0
	}
}

// ThinkingConfig is the thinking configuration to pass in LLM API requests.
type ThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

// GetSupportedThinkingLevels returns the thinking levels supported by a model.
// If the model supports reasoning (extended thinking), all levels are available.
// Otherwise, only "off" is available.
func GetSupportedThinkingLevels(modelReasoning bool) []ThinkingLevel {
	if modelReasoning {
		return ValidThinkingLevels()
	}
	return []ThinkingLevel{ThinkingLevelOff}
}

// ClampThinkingLevel clamps the requested level to what the model supports.
func ClampThinkingLevel(requested ThinkingLevel, modelReasoning bool) ThinkingLevel {
	if !modelReasoning && requested != ThinkingLevelOff {
		return ThinkingLevelOff
	}
	if requested.IsValid() {
		return requested
	}
	return ThinkingLevelOff
}

// BuildThinkingConfig returns the thinking config for API requests, or nil if thinking is off.
func BuildThinkingConfig(level ThinkingLevel) *ThinkingConfig {
	if level == ThinkingLevelOff {
		return nil
	}
	budget := level.BudgetTokens()
	if budget <= 0 {
		return nil
	}
	return &ThinkingConfig{
		Type:         "enabled",
		BudgetTokens: budget,
	}
}

// ParseThinkingLevel parses a string into a ThinkingLevel, returning an error if invalid.
func ParseThinkingLevel(s string) (ThinkingLevel, error) {
	for _, l := range ValidThinkingLevels() {
		if string(l) == s {
			return l, nil
		}
	}
	return ThinkingLevelOff, fmt.Errorf("invalid thinking level: %q", s)
}
