package main

import "strings"

// ModelAliases maps common aliases to actual model IDs.
var ModelAliases = map[string]string{
	// Sonnet family
	"sonnet":       "claude-sonnet-4-20250514",
	"sonnet4":      "claude-sonnet-4-20250514",
	"sonnet-4":     "claude-sonnet-4-20250514",
	"sonnet3.5":    "claude-3-5-sonnet-20241022",
	"sonnet-3.5":   "claude-3-5-sonnet-20241022",
	// Opus family
	"opus":         "claude-opus-4-20250514",
	"opus4":        "claude-opus-4-20250514",
	"opus-4":       "claude-opus-4-20250514",
	"opus4.5":      "claude-opus-4-5-20250610",
	"opus-4.5":     "claude-opus-4-5-20250610",
	// Haiku family
	"haiku":        "claude-haiku-4-5-20250610",
	"haiku4.5":     "claude-haiku-4-5-20250610",
	"haiku-4.5":    "claude-haiku-4-5-20250610",
	// Family wildcards (maps generic references)
	"best":         "claude-opus-4-20250514",
	"fast":         "claude-sonnet-4-20250514",
	// Legacy remap (older model IDs that should be upgraded)
	"claude-3-opus-20240229":     "claude-opus-4-20250514",
	"claude-3-5-sonnet-20240620": "claude-sonnet-4-20250514",
}

// ResolveModelAlias resolves a user-specified model string to an actual model ID.
// If the input matches an alias, returns the resolved model ID.
// If the input is already a full model ID, returns it unchanged.
func ResolveModelAlias(model string) (string, bool) {
	// Check exact match
	if resolved, ok := ModelAliases[model]; ok {
		return resolved, true
	}
	// Check case-insensitive match
	lower := strings.ToLower(model)
	if resolved, ok := ModelAliases[lower]; ok {
		return resolved, true
	}
	// Not an alias — assume it's a full model ID
	return model, false
}

// GetDefaultModel returns the default model based on subscription type.
func GetDefaultModel(subscriptionType string) string {
	switch subscriptionType {
	case "enterprise":
		return "claude-opus-4-20250514"
	case "claude_ai", "api":
		return "claude-sonnet-4-20250514"
	default:
		return "claude-sonnet-4-20250514"
	}
}
