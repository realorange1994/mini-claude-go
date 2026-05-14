package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultModelCapabilities(t *testing.T) {
	requiredModels := []string{
		"claude-sonnet-4-20250514",
		"claude-opus-4-20250514",
		"claude-opus-4-5-20250610",
		"claude-haiku-4-5-20250610",
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
	}
	for _, model := range requiredModels {
		if _, ok := DefaultModelCapabilities[model]; !ok {
			t.Errorf("DefaultModelCapabilities missing entry for %s", model)
		}
	}
}

func TestGetContextWindow(t *testing.T) {
	mc := NewModelCapabilitiesCacheDefault()

	// Known models should return their specific context window
	tests := []struct {
		model string
		want  int64
	}{
		{"claude-sonnet-4-20250514", 1_000_000},
		{"claude-opus-4-20250514", 1_000_000},
		{"claude-3-5-haiku-20241022", 200_000},
	}
	for _, tc := range tests {
		got := mc.GetContextWindow(tc.model)
		if got != tc.want {
			t.Errorf("GetContextWindow(%s) = %d, want %d", tc.model, got, tc.want)
		}
	}
}

func TestGetContextWindowUnknown(t *testing.T) {
	mc := NewModelCapabilitiesCacheDefault()

	// Unknown models should return 200K default
	got := mc.GetContextWindow("claude-unknown-12345")
	if got != 200_000 {
		t.Errorf("GetContextWindow(unknown) = %d, want 200000", got)
	}
}

func TestGetMaxOutputTokens(t *testing.T) {
	mc := NewModelCapabilitiesCacheDefault()

	tests := []struct {
		model string
		want  int64
	}{
		{"claude-sonnet-4-20250514", 64000},
		{"claude-opus-4-20250514", 32000},
		{"claude-3-5-sonnet-20241022", 8192},
	}
	for _, tc := range tests {
		got := mc.GetMaxOutputTokens(tc.model)
		if got != tc.want {
			t.Errorf("GetMaxOutputTokens(%s) = %d, want %d", tc.model, got, tc.want)
		}
	}
}

func TestEnvOverride(t *testing.T) {
	mc := NewModelCapabilitiesCacheDefault()

	// Set env override
	os.Setenv("CLAUDE_CODE_MAX_CONTEXT_TOKENS", "500000")
	defer os.Unsetenv("CLAUDE_CODE_MAX_CONTEXT_TOKENS")

	got := mc.GetContextWindow("claude-sonnet-4-20250514")
	if got != 500_000 {
		t.Errorf("GetContextWindow with env override = %d, want 500000", got)
	}

	// Invalid env value should fall back to model defaults
	os.Setenv("CLAUDE_CODE_MAX_CONTEXT_TOKENS", "not-a-number")
	got = mc.GetContextWindow("claude-sonnet-4-20250514")
	if got != 1_000_000 {
		t.Errorf("GetContextWindow with invalid env override = %d, want 1000000", got)
	}

	// Negative env value should fall back
	os.Setenv("CLAUDE_CODE_MAX_CONTEXT_TOKENS", "-100")
	got = mc.GetContextWindow("claude-sonnet-4-20250514")
	if got != 1_000_000 {
		t.Errorf("GetContextWindow with negative env override = %d, want 1000000", got)
	}

	// Test max output tokens override
	os.Setenv("CLAUDE_CODE_MAX_OUTPUT_TOKENS", "32000")
	defer os.Unsetenv("CLAUDE_CODE_MAX_OUTPUT_TOKENS")

	got = mc.GetMaxOutputTokens("claude-sonnet-4-20250514")
	if got != 32000 {
		t.Errorf("GetMaxOutputTokens with env override = %d, want 32000", got)
	}
}

func TestSaveLoadDisk(t *testing.T) {
	tmpDir := t.TempDir()
	mc := NewModelCapabilitiesCache(tmpDir)

	// Add a custom model to the cache
	mc.mu.Lock()
	mc.cache["claude-test-model"] = ModelCapabilities{
		ContextWindow:     300_000,
		MaxOutputTokens:   16000,
		MaxThinkingTokens: 8000,
		SupportsVision:    true,
		SupportsThinking:  true,
	}
	mc.mu.Unlock()

	// Save to disk
	if err := mc.SaveToDisk(); err != nil {
		t.Fatalf("SaveToDisk failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(tmpDir, "model-capabilities.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file not found after SaveToDisk: %v", err)
	}

	// Load into a fresh cache
	mc2 := NewModelCapabilitiesCache(tmpDir)
	if err := mc2.LoadFromDisk(); err != nil {
		t.Fatalf("LoadFromDisk failed: %v", err)
	}

	// Verify the custom model was loaded
	got := mc2.GetContextWindow("claude-test-model")
	if got != 300_000 {
		t.Errorf("loaded context window = %d, want 300000", got)
	}

	got = mc2.GetMaxOutputTokens("claude-test-model")
	if got != 16000 {
		t.Errorf("loaded max output tokens = %d, want 16000", got)
	}
}

func TestModelCapabilitiesJSONRoundTrip(t *testing.T) {
	caps := ModelCapabilities{
		ContextWindow:     1_000_000,
		MaxOutputTokens:   64000,
		MaxThinkingTokens: 32000,
		SupportsVision:    true,
		SupportsThinking:  true,
	}

	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded ModelCapabilities
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ContextWindow != caps.ContextWindow {
		t.Errorf("ContextWindow: got %d, want %d", decoded.ContextWindow, caps.ContextWindow)
	}
	if decoded.MaxOutputTokens != caps.MaxOutputTokens {
		t.Errorf("MaxOutputTokens: got %d, want %d", decoded.MaxOutputTokens, caps.MaxOutputTokens)
	}
	if decoded.MaxThinkingTokens != caps.MaxThinkingTokens {
		t.Errorf("MaxThinkingTokens: got %d, want %d", decoded.MaxThinkingTokens, caps.MaxThinkingTokens)
	}
	if decoded.SupportsVision != caps.SupportsVision {
		t.Errorf("SupportsVision: got %v, want %v", decoded.SupportsVision, caps.SupportsVision)
	}
	if decoded.SupportsThinking != caps.SupportsThinking {
		t.Errorf("SupportsThinking: got %v, want %v", decoded.SupportsThinking, caps.SupportsThinking)
	}
}

func TestPartialModelMatch(t *testing.T) {
	mc := NewModelCapabilitiesCacheDefault()

	// A model like "claude-sonnet-4-20250310" that isn't in defaults
	// but matches a known prefix should get a reasonable default
	got := mc.GetContextWindow("claude-sonnet-4-20250310")
	// It won't match the exact defaults but should return something reasonable
	if got < 200_000 {
		t.Errorf("GetContextWindow for known prefix model = %d, want >= 200000", got)
	}

	// Fully unknown model (not matching any prefix)
	got = mc.GetContextWindow("some-random-model")
	if got != 200_000 {
		t.Errorf("GetContextWindow for unknown model = %d, want 200000", got)
	}
}

// ─── Upstream Quality: Model version prefix matching (semver equivalent) ─────
// Mirrors upstream: modelSupports1M() and canonical name extraction patterns.
// The Go code doesn't have a general semver library, but model version
// comparison uses prefix matching for family-based resolution.

func TestModelPrefixKnownFamilies(t *testing.T) {
	// Every known family prefix should resolve to a non-zero context window
	families := []string{
		"claude-sonnet-4",
		"claude-opus-4",
		"claude-haiku-4",
		"claude-3-5-sonnet",
		"claude-3-5-haiku",
		"claude-3-7-sonnet",
	}
	mc := NewModelCapabilitiesCacheDefault()
	for _, family := range families {
		model := family + "-20990101" // future date to ensure it's not a built-in
		got := mc.GetContextWindow(model)
		if got <= 0 {
			t.Errorf("GetContextWindow(%q) = %d, want > 0", model, got)
		}
	}
}

func TestModelPrefixExactVsFamily(t *testing.T) {
	// Exact model match should use DefaultModelCapabilities
	// Prefix match should use family default (200K for unknown variants)
	mc := NewModelCapabilitiesCacheDefault()

	// Exact: claude-sonnet-4-20250514 has 1M context window
	exact := mc.GetContextWindow("claude-sonnet-4-20250514")
	if exact != 1_000_000 {
		t.Errorf("exact sonnet match: got %d, want 1000000", exact)
	}

	// Prefix match: a variant not in defaults should get family fallback
	variant := mc.GetContextWindow("claude-sonnet-4-20990101")
	if variant != 200_000 {
		t.Errorf("variant sonnet match: got %d, want 200000", variant)
	}
}

func TestModelContextWindowOrdering(t *testing.T) {
	// Mirrors upstream model capability ordering:
	// Newer generation models (opus-4-5, sonnet-4) have larger output limits
	// than older generation models (3-5-haiku).
	mc := NewModelCapabilitiesCacheDefault()

	opus := mc.GetMaxOutputTokens("claude-opus-4-5-20250610")
	sonnet := mc.GetMaxOutputTokens("claude-sonnet-4-20250514")
	haiku := mc.GetMaxOutputTokens("claude-3-5-haiku-20241022")

	// Newer generation models exceed older haiku's 8K limit
	if sonnet <= haiku {
		t.Errorf("sonnet max output tokens (%d) should exceed haiku (%d)", sonnet, haiku)
	}
	if opus <= haiku {
		t.Errorf("opus max output tokens (%d) should exceed haiku (%d)", opus, haiku)
	}
	// opus-4-5 should be >= sonnet-4 (they happen to be equal at 64000)
	if opus < sonnet {
		t.Errorf("opus max output tokens (%d) should be >= sonnet (%d)", opus, sonnet)
	}
}

func TestModelCapabilitiesAllDefaultsHaveRequiredFields(t *testing.T) {
	// Every entry in DefaultModelCapabilities should have valid fields
	for modelID, caps := range DefaultModelCapabilities {
		if caps.ContextWindow <= 0 {
			t.Errorf("%s: ContextWindow = %d, want > 0", modelID, caps.ContextWindow)
		}
		if caps.MaxOutputTokens <= 0 {
			t.Errorf("%s: MaxOutputTokens = %d, want > 0", modelID, caps.MaxOutputTokens)
		}
		if caps.MaxThinkingTokens < 0 {
			t.Errorf("%s: MaxThinkingTokens = %d, want >= 0", modelID, caps.MaxThinkingTokens)
		}
	}
}

func TestModelCapabilitiesCacheFreshInstance(t *testing.T) {
	// A fresh cache instance should not have any custom entries
	mc := NewModelCapabilitiesCache(t.TempDir())
	got := mc.GetContextWindow("claude-test-not-present")
	if got != 200_000 {
		t.Errorf("fresh cache for unknown model = %d, want 200000", got)
	}
}

func TestModelCapabilitiesCacheSaveToDiskCreatesDir(t *testing.T) {
	// SaveToDisk should create the cache directory if it doesn't exist
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "new", "cache", "dir")
	mc := NewModelCapabilitiesCache(cacheDir)

	mc.mu.Lock()
	mc.cache["test-model"] = ModelCapabilities{ContextWindow: 100_000}
	mc.mu.Unlock()

	err := mc.SaveToDisk()
	if err != nil {
		t.Fatalf("SaveToDisk failed: %v", err)
	}

	if _, err := os.Stat(cacheDir); err != nil {
		t.Errorf("cache directory not created: %v", err)
	}
}

func TestModelCapabilitiesCacheLoadFromDiskMissingFile(t *testing.T) {
	// LoadFromDisk on a directory with no cache file should not error
	mc := NewModelCapabilitiesCache(t.TempDir())
	err := mc.LoadFromDisk()
	if err != nil {
		t.Errorf("LoadFromDisk with no file should not error: %v", err)
	}
}

func TestModelCapabilitiesCacheStaleCache(t *testing.T) {
	// A cache file older than 24h should NOT be loaded
	tmpDir := t.TempDir()
	mc := NewModelCapabilitiesCache(tmpDir)
	mc.mu.Lock()
	mc.cache["test-model"] = ModelCapabilities{ContextWindow: 500_000}
	mc.mu.Unlock()

	if err := mc.SaveToDisk(); err != nil {
		t.Fatalf("SaveToDisk failed: %v", err)
	}

	// Make the file appear old
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(filepath.Join(tmpDir, "model-capabilities.json"), oldTime, oldTime)

	// Load into a fresh cache
	mc2 := NewModelCapabilitiesCache(tmpDir)
	if err := mc2.LoadFromDisk(); err != nil {
		t.Fatalf("LoadFromDisk failed: %v", err)
	}

	// Stale cache should NOT be loaded, so test-model should get 200K fallback
	got := mc2.GetContextWindow("test-model")
	if got != 200_000 {
		t.Errorf("stale cache should not load: got %d, want 200000", got)
	}
}

func TestGetContextWindowEnvOverrideZero(t *testing.T) {
	mc := NewModelCapabilitiesCacheDefault()
	os.Setenv("CLAUDE_CODE_MAX_CONTEXT_TOKENS", "0")
	defer os.Unsetenv("CLAUDE_CODE_MAX_CONTEXT_TOKENS")

	// Zero should fall back (not a valid positive override)
	got := mc.GetContextWindow("claude-sonnet-4-20250514")
	if got != 1_000_000 {
		t.Errorf("zero env override: got %d, want 1000000", got)
	}
}

func TestGetMaxOutputTokensEnvOverrideZero(t *testing.T) {
	mc := NewModelCapabilitiesCacheDefault()
	os.Setenv("CLAUDE_CODE_MAX_OUTPUT_TOKENS", "0")
	defer os.Unsetenv("CLAUDE_CODE_MAX_OUTPUT_TOKENS")

	got := mc.GetMaxOutputTokens("claude-sonnet-4-20250514")
	if got != 64000 {
		t.Errorf("zero env override: got %d, want 64000", got)
	}
}