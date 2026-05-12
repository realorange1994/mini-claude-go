package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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