package main

import (
	"strings"
	"testing"
)

func TestResolveModelAliasSonnet(t *testing.T) {
	resolved, ok := ResolveModelAlias("sonnet")
	if !ok {
		t.Fatal("expected alias to resolve")
	}
	if resolved != "claude-sonnet-4-20250514" {
		t.Fatalf("expected claude-sonnet-4-20250514, got %s", resolved)
	}
}

func TestResolveModelAliasOpus(t *testing.T) {
	resolved, ok := ResolveModelAlias("opus")
	if !ok {
		t.Fatal("expected alias to resolve")
	}
	if resolved != "claude-opus-4-5-20250610" {
		t.Fatalf("expected claude-opus-4-5-20250610, got %s", resolved)
	}
}

func TestResolveModelAliasHaiku(t *testing.T) {
	resolved, ok := ResolveModelAlias("haiku")
	if !ok {
		t.Fatal("expected alias to resolve")
	}
	if resolved != "claude-haiku-4-5-20250610" {
		t.Fatalf("expected claude-haiku-4-5-20250610, got %s", resolved)
	}
}

func TestResolveModelAliasCaseInsensitive(t *testing.T) {
	resolved, ok := ResolveModelAlias("OPUS")
	if !ok {
		t.Fatal("expected alias to resolve case-insensitively")
	}
	if resolved != "claude-opus-4-5-20250610" {
		t.Fatalf("expected claude-opus-4-5-20250610, got %s", resolved)
	}

	resolved2, ok2 := ResolveModelAlias("Sonnet")
	if !ok2 {
		t.Fatal("expected Sonnet alias to resolve case-insensitively")
	}
	if resolved2 != "claude-sonnet-4-20250514" {
		t.Fatalf("expected claude-sonnet-4-20250514, got %s", resolved2)
	}
}

func TestResolveModelAliasFullID(t *testing.T) {
	resolved, ok := ResolveModelAlias("claude-opus-4-20250514")
	if ok {
		t.Fatal("expected full model ID not to be an alias")
	}
	if resolved != "claude-opus-4-20250514" {
		t.Fatalf("expected unchanged, got %s", resolved)
	}
}

func TestResolveModelAliasLegacyRemap(t *testing.T) {
	resolved, ok := ResolveModelAlias("claude-3-opus-20240229")
	if !ok {
		t.Fatal("expected legacy model ID to resolve")
	}
	if resolved != "claude-opus-4-5-20250610" {
		t.Fatalf("expected claude-opus-4-5-20250610, got %s", resolved)
	}

	resolved2, ok2 := ResolveModelAlias("claude-3-5-sonnet-20240620")
	if !ok2 {
		t.Fatal("expected legacy sonnet model ID to resolve")
	}
	if resolved2 != "claude-sonnet-4-20250514" {
		t.Fatalf("expected claude-sonnet-4-20250514, got %s", resolved2)
	}
}

func TestResolveModelAliasVariantForms(t *testing.T) {
	// These are full model ID variants that map to current defaults via the legacy remap.
	// Variant forms like "sonnet4", "opus-4.5" are not alias-mapped — they are treated
	// as-is model strings (ResolveModelAlias returns them with ok=false).
	tests := []struct {
		alias    string
		expected string
		ok       bool
	}{
		{"best", "claude-opus-4-5-20250610", true},
		{"fast", "claude-sonnet-4-20250514", true},
	}
	for _, tt := range tests {
		resolved, ok := ResolveModelAlias(tt.alias)
		if ok != tt.ok {
			t.Errorf("alias %q: expected ok=%v, got ok=%v", tt.alias, tt.ok, ok)
			continue
		}
		if resolved != tt.expected {
			t.Errorf("alias %q: expected %s, got %s", tt.alias, tt.expected, resolved)
		}
	}
}

func TestGetDefaultModel(t *testing.T) {
	tests := []struct {
		subscription string
		expected     string
	}{
		{"enterprise", "claude-opus-4-5-20250610"},
		{"claude_ai", "claude-sonnet-4-20250514"},
		{"api", "claude-sonnet-4-20250514"},
		{"unknown", "claude-sonnet-4-20250514"},
		{"", "claude-sonnet-4-20250514"},
	}
	for _, tt := range tests {
		got := GetDefaultModel(tt.subscription)
		if got != tt.expected {
			t.Errorf("GetDefaultModel(%q): expected %s, got %s", tt.subscription, tt.expected, got)
		}
	}
}

func TestConfigModelAliasResolution(t *testing.T) {
	// Verify that ResolveModelAlias works on a Config-like model string
	cfg := Config{Model: "sonnet"}
	if resolved, ok := ResolveModelAlias(cfg.Model); ok {
		cfg.Model = resolved
	}
	if cfg.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("expected Config.Model to be resolved to claude-sonnet-4-20250514, got %s", cfg.Model)
	}

	// Full model IDs should remain unchanged
	cfg2 := Config{Model: "claude-opus-4-5-20250610"}
	if resolved, ok := ResolveModelAlias(cfg2.Model); ok {
		cfg2.Model = resolved
	}
	if cfg2.Model != "claude-opus-4-5-20250610" {
		t.Fatalf("expected full model ID to remain unchanged, got %s", cfg2.Model)
	}
}

// ─── Upstream Quality: [1m] suffix roundtrip ──────────────────────────────────

func TestResolveModelAlias1mSuffixRoundtrip(t *testing.T) {
	// ResolveModelAlias should handle [1m] suffix consistently:
	// alias[1m] → resolved_model[1m] (if family supports 1M)
	// Resolve→strip→Resolve should produce same result
	tests := []struct {
		input string
	}{
		{"sonnet[1m]"},
		{"opus[1m]"},
		{"haiku[1m]"},
		{"best[1m]"},
		{"fast[1m]"},
	}
	for _, tt := range tests {
		resolved1, ok1 := ResolveModelAlias(tt.input)
		if !ok1 {
			t.Errorf("ResolveModelAlias(%q) should resolve alias, got ok=false", tt.input)
			continue
		}
		// Strip [1m] and resolve again
		clean := strings.ReplaceAll(resolved1, "[1m]", "")
		resolved2, ok2 := ResolveModelAlias(clean)
		// The second resolution should produce the same base model
		if resolved2 != clean {
			t.Errorf("roundtrip mismatch: %q → %q → %q (expected base=%q)",
				tt.input, resolved1, resolved2, clean)
		}
		if ok2 {
			t.Errorf("full model ID %q should not be an alias (ok should be false)", clean)
		}
	}
}

// ─── Upstream Quality: All legacy remaps resolve ──────────────────────────────

func TestAllLegacyRemapsResolve(t *testing.T) {
	// Every entry in legacyModelRemap should resolve via ResolveModelAlias
	for oldID, newID := range legacyModelRemap {
		resolved, ok := ResolveModelAlias(oldID)
		if !ok {
			t.Errorf("legacyModelRemap entry %q should resolve, got ok=false", oldID)
			continue
		}
		if resolved != newID {
			t.Errorf("legacyModelRemap[%q] = %q but ResolveModelAlias returned %q",
				oldID, newID, resolved)
		}
	}
}

// ─── Upstream Quality: ExtractCanonicalModelName ──────────────────────────────

func TestExtractCanonicalModelName(t *testing.T) {
	tests := []struct {
		modelID string
		want    string
	}{
		{"claude-opus-4-5-20250610", "claude-opus-4-5"},
		{"claude-sonnet-4-20250514", "claude-sonnet-4"},
		{"claude-haiku-4-5-20250610", "claude-haiku-4-5"},
		{"claude-3-5-sonnet-20241022", "claude-3-5-sonnet"},
		{"claude-3-opus-20240229", "claude-3-opus"},
		{"unknown-model", "unknown-model"},
	}
	for _, tt := range tests {
		got := ExtractCanonicalModelName(tt.modelID)
		if got != tt.want {
			t.Errorf("ExtractCanonicalModelName(%q) = %q, want %q", tt.modelID, got, tt.want)
		}
	}
}

func TestExtractCanonicalModelName1mSuffix(t *testing.T) {
	// [1m] suffix should be stripped before canonical extraction
	got := ExtractCanonicalModelName("claude-opus-4-5-20250610[1m]")
	if got != "claude-opus-4-5" {
		t.Errorf("expected 'claude-opus-4-5', got %q", got)
	}
}

// ─── Upstream Quality: GetContextWindowForModel ───────────────────────────────

func TestGetContextWindowForModel1m(t *testing.T) {
	// [1m] suffix should return 1M context window
	window := GetContextWindowForModel("claude-sonnet-4-20250514[1m]")
	if window != 1_000_000 {
		t.Errorf("expected 1M context window, got %d", window)
	}
}

func TestGetContextWindowForModelDefault(t *testing.T) {
	// Known model without [1m] should return its default context window
	window := GetContextWindowForModel("claude-sonnet-4-20250514")
	if window <= 0 {
		t.Errorf("expected positive context window, got %d", window)
	}
}

func TestGetContextWindowForModelUnknown(t *testing.T) {
	// Unknown model should return 200K fallback
	window := GetContextWindowForModel("unknown-model")
	if window != 200_000 {
		t.Errorf("expected 200K fallback, got %d", window)
	}
}

// ─── Upstream Quality: modelSupports1M ────────────────────────────────────────

func TestModelSupports1M(t *testing.T) {
	supported := []string{
		"claude-opus-4-20250514",
		"claude-opus-4-5-20250610",
		"claude-sonnet-4-20250514",
		"claude-3-7-sonnet-20250219",
	}
	for _, model := range supported {
		if !modelSupports1M(model) {
			t.Errorf("modelSupports1M(%q) should be true", model)
		}
	}

	unsupported := []string{
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-haiku-4-5-20250610",
	}
	for _, model := range unsupported {
		if modelSupports1M(model) {
			t.Errorf("modelSupports1M(%q) should be false", model)
		}
	}
}

// ─── Upstream Quality: has1mContext ───────────────────────────────────────────

func TestHas1mContext(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-sonnet-4-20250514[1m]", true},
		{"claude-sonnet-4-20250514[1M]", true}, // case insensitive
		{"claude-sonnet-4-20250514", false},
		{"sonnet[1m]", true},
		{"[1m]", true},
		{"claude[1m]extra", false}, // [1m] must be at end
	}
	for _, tt := range tests {
		got := has1mContext(tt.model)
		if got != tt.want {
			t.Errorf("has1mContext(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

// ─── Upstream Quality: isLegacyOpusID ─────────────────────────────────────────

func TestIsLegacyOpusID(t *testing.T) {
	legacy := []string{
		"claude-opus-4-0-20250514",
		"claude-opus-4-1-20250514",
		"claude-opus-4.0-20250514",
		"claude-opus-4.1-20250514",
	}
	for _, id := range legacy {
		if !isLegacyOpusID(id) {
			t.Errorf("isLegacyOpusID(%q) should be true", id)
		}
	}

	current := []string{
		"claude-opus-4-5-20250610",
		"claude-opus-4-20250514",
		"claude-sonnet-4-20250514",
	}
	for _, id := range current {
		if isLegacyOpusID(id) {
			t.Errorf("isLegacyOpusID(%q) should be false for current model", id)
		}
	}
}
