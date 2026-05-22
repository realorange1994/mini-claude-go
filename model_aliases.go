package main

import (
	"os"
	"regexp"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// [1m] context window support — upstream: src/utils/context.ts has1mContext()
// ---------------------------------------------------------------------------

// has1mContext checks if the model string has the [1m] suffix (case-insensitive).
// This suffix enables 1M context window on supported models.
// Upstream: has1mContext() in src/utils/context.ts:36
func has1mContext(model string) bool {
	return regexp.MustCompile(`(?i)\[1m\]$`).MatchString(model)
}

// modelSupports1M checks if a model ID (without [1m] suffix) supports the
// 1M context window feature. Matches upstream's modelSupports1M() in context.ts.
func modelSupports1M(modelID string) bool {
	// Opus 4+ family
	if strings.Contains(modelID, "claude-opus-4") {
		return true
	}
	// Sonnet 4+ family
	if strings.Contains(modelID, "claude-sonnet-4") {
		return true
	}
	// Claude 3.7 Sonnet
	if strings.Contains(modelID, "claude-3-7-sonnet") {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Parse user-specified model — upstream: parseUserSpecifiedModel() in model.ts:497
// ---------------------------------------------------------------------------

// ModelAliasMapping defines the alias-to-default mapping.
// The actual default is resolved at runtime via GetDefaultOpusModel/GetDefaultSonnetModel/GetDefaultHaikuModel.
type modelFamily string

const (
	familyOpus   modelFamily = "opus"
	familySonnet modelFamily = "sonnet"
	familyHaiku  modelFamily = "haiku"
)

var modelAliasFamily = map[string]modelFamily{
	"opus":     familyOpus,
	"sonnet":   familySonnet,
	"haiku":    familyHaiku,
	"best":     familyOpus, // best = current Opus
	"fast":     familySonnet,
}

// ResolveModelAlias resolves a user-specified model string to an actual model ID.
// Supports [1m] suffix on any alias (e.g., sonnet[1m], opus[1m]) to enable
// 1M context window without requiring each variant to be in the alias map.
//
// Upstream: parseUserSpecifiedModel() in src/utils/model/model.ts:497
//
// Resolution rules:
//  1. Strip [1m] suffix (if present) before alias resolution
//  2. Resolve alias to default model for that family
//  3. Re-append [1m] suffix if the model family supports it
//  4. If input is already a full model ID (not an alias), return unchanged
//  5. Legacy Opus remap: opus-4.0/opus-4.1 → current Opus default
func ResolveModelAlias(model string) (string, bool) {
	model = strings.TrimSpace(model)
	normalized := strings.ToLower(model)

	// Extract [1m] suffix before resolution
	has1m := has1mContext(normalized)
	modelStr := normalized
	if has1m {
		modelStr = strings.TrimSpace(regexp.MustCompile(`(?i)\[1m\]$`).ReplaceAllString(modelStr, ""))
	}

	// Check if it's a known alias
	if family, ok := modelAliasFamily[modelStr]; ok {
		var resolved string
		switch family {
		case familyOpus:
			resolved = getDefaultOpusModel()
		case familySonnet:
			resolved = getDefaultSonnetModel()
		case familyHaiku:
			resolved = getDefaultHaikuModel()
		}
		// Re-append [1m] only if the family supports it
		if has1m && modelSupports1M(resolved) {
			resolved += "[1m]"
		}
		return resolved, true
	}

	// Check legacy Opus model IDs that should be remapped to current default
	if isLegacyOpusID(modelStr) {
		resolved := getDefaultOpusModel()
		if has1m && modelSupports1M(resolved) {
			resolved += "[1m]"
		}
		return resolved, true
	}

	// Check backward-compatible legacy aliases (full model IDs)
	if resolved, ok := legacyModelRemap[modelStr]; ok {
		if has1m && modelSupports1M(resolved) {
			resolved += "[1m]"
		}
		return resolved, true
	}

	// Not an alias — return as-is, preserving [1m] suffix if present
	if has1m {
		return strings.TrimSpace(regexp.MustCompile(`(?i)\[1m\]$`).ReplaceAllString(model, "")) + "[1m]", false
	}
	return model, false
}

// isLegacyOpusID checks if the model string is a legacy Opus 4.0 or 4.1 ID
// that should be remapped to the current default Opus.
// Upstream: isLegacyOpusFirstParty() in model.ts:603
func isLegacyOpusID(modelStr string) bool {
	// Opus 4.0 and 4.1 are no longer available on the first-party API
	legacyOpusPatterns := []string{
		"claude-opus-4-0-",
		"claude-opus-4-1-",
		"claude-opus-4.0-",
		"claude-opus-4.1-",
	}
	for _, pattern := range legacyOpusPatterns {
		if strings.HasPrefix(modelStr, pattern) {
			return true
		}
	}
	return false
}

// legacyModelRemap provides backward-compatible remapping for older model IDs.
var legacyModelRemap = map[string]string{
	// Legacy remap (older model IDs that should be upgraded to current defaults)
	"claude-3-opus-20240229":     "claude-opus-4-5-20250610",
	"claude-3-5-sonnet-20240620": "claude-sonnet-4-20250514",
	"claude-3-5-sonnet-20241022": "claude-sonnet-4-20250514",
}

// ---------------------------------------------------------------------------
// Default model getters — upstream: getDefaultOpusModel(), etc. in model.ts
// ---------------------------------------------------------------------------

// These can be overridden by environment variables.
var (
	defaultOpusModel   = "claude-opus-4-5-20250610"
	defaultSonnetModel = "claude-sonnet-4-20250514"
	defaultHaikuModel  = "claude-haiku-4-5-20250610"
)

func init() {
	// Allow env overrides at startup (matches upstream's env pattern)
	if m := os.Getenv("CLAUDE_DEFAULT_OPUS_MODEL"); m != "" {
		defaultOpusModel = m
	}
	if m := os.Getenv("CLAUDE_DEFAULT_SONNET_MODEL"); m != "" {
		defaultSonnetModel = m
	}
	if m := os.Getenv("CLAUDE_DEFAULT_HAIKU_MODEL"); m != "" {
		defaultHaikuModel = m
	}
}

// getDefaultOpusModel returns the current default Opus model.
// Upstream: getDefaultOpusModel() in model.ts:116
func getDefaultOpusModel() string {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultOpusModel
}

// getDefaultSonnetModel returns the current default Sonnet model.
// Upstream: getDefaultSonnetModel() in model.ts:141
func getDefaultSonnetModel() string {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultSonnetModel
}

// getDefaultHaikuModel returns the current default Haiku model.
// Upstream: getDefaultHaikuModel() in model.ts:167
func getDefaultHaikuModel() string {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultHaikuModel
}

// GetDefaultModel returns the default model based on subscription type.
func GetDefaultModel(subscriptionType string) string {
	switch subscriptionType {
	case "enterprise":
		return getDefaultOpusModel()
	case "claude_ai", "api":
		return getDefaultSonnetModel()
	default:
		return getDefaultSonnetModel()
	}
}

// ---------------------------------------------------------------------------
// Canonical name extraction — upstream: firstPartyNameToCanonical() in model.ts:262
// ---------------------------------------------------------------------------

// ExtractCanonicalModelName extracts the canonical model family from a full model ID.
// E.g., "claude-opus-4-5-20250610" → "claude-opus-4-5"
func ExtractCanonicalModelName(modelID string) string {
	name := strings.ToLower(modelID)
	// Strip [1m] suffix if present
	name = regexp.MustCompile(`(?i)\[1m\]$`).ReplaceAllString(name, "")

	// Order matters: check more specific versions first
	canonicalMappings := []struct {
		pattern    string
		canonical  string
	}{
		{"claude-opus-4-7", "claude-opus-4-7"},
		{"claude-opus-4-6", "claude-opus-4-6"},
		{"claude-opus-4-5", "claude-opus-4-5"},
		{"claude-opus-4-1", "claude-opus-4-1"},
		{"claude-opus-4", "claude-opus-4"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"claude-sonnet-4-5", "claude-sonnet-4-5"},
		{"claude-sonnet-4", "claude-sonnet-4"},
		{"claude-haiku-4-5", "claude-haiku-4-5"},
		{"claude-3-7-sonnet", "claude-3-7-sonnet"},
		{"claude-3-5-sonnet", "claude-3-5-sonnet"},
		{"claude-3-5-haiku", "claude-3-5-haiku"},
		{"claude-3-opus", "claude-3-opus"},
	}

	for _, m := range canonicalMappings {
		if strings.Contains(name, m.pattern) {
			return m.canonical
		}
	}
	return name
}

// ---------------------------------------------------------------------------
// Context window from model string — upstream: getContextWindowForModel() in context.ts
// ---------------------------------------------------------------------------

// GetContextWindowForModel returns the context window size for a model string.
// Resolves [1m] suffix to 1,000,000 tokens for supported models.
// Upstream: getContextWindowForModel() in src/utils/context.ts:55
func GetContextWindowForModel(model string) int64 {
	normalized := strings.ToLower(strings.TrimSpace(model))

	// [1m] suffix — explicit client-side opt-in
	if has1mContext(normalized) {
		return 1_000_000
	}

	// Fall back to known capabilities
	modelID := normalized
	caps, ok := DefaultModelCapabilities[modelID]
	if ok {
		return caps.ContextWindow
	}

	// Default fallback
	return 200_000
}

// ---------------------------------------------------------------------------
// SetDefaultsForTesting — test helper to override defaults
// ---------------------------------------------------------------------------

var defaultMu sync.RWMutex

