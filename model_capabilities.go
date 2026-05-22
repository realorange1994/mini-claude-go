package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// ModelCapabilities holds the capabilities of a specific model.
type ModelCapabilities struct {
	ContextWindow     int64 `json:"context_window"`
	MaxOutputTokens   int64 `json:"max_output_tokens"`
	MaxThinkingTokens int64 `json:"max_thinking_tokens"`
	SupportsVision    bool  `json:"supports_vision"`
	SupportsThinking  bool  `json:"supports_thinking"`
}

// ModelCapabilitiesCache caches model capabilities to disk.
type ModelCapabilitiesCache struct {
	mu       sync.RWMutex
	cache    map[string]ModelCapabilities
	cacheDir string
	loaded   bool
}

// DefaultModelCapabilities provides known model capabilities as fallback
// when the API is unavailable or a model is not in the API response.
// Only latest models are kept here — older versions are removed.
var DefaultModelCapabilities = map[string]ModelCapabilities{
	"claude-sonnet-4-6-20260125":    {ContextWindow: 1_000_000, MaxOutputTokens: 64000, MaxThinkingTokens: 32000, SupportsVision: true, SupportsThinking: true},
	"claude-opus-4-6-20260302":      {ContextWindow: 1_000_000, MaxOutputTokens: 64000, MaxThinkingTokens: 32000, SupportsVision: true, SupportsThinking: true},
	"claude-haiku-4-5-20250610":     {ContextWindow: 200_000, MaxOutputTokens: 8192, MaxThinkingTokens: 4096, SupportsVision: true, SupportsThinking: true},
	"claude-sonnet-4-5-20250929":    {ContextWindow: 1_000_000, MaxOutputTokens: 64000, MaxThinkingTokens: 32000, SupportsVision: true, SupportsThinking: true},
	"claude-sonnet-4-20250514":      {ContextWindow: 1_000_000, MaxOutputTokens: 64000, MaxThinkingTokens: 32000, SupportsVision: true, SupportsThinking: true},
	"claude-opus-4-5-20250610":      {ContextWindow: 1_000_000, MaxOutputTokens: 64000, MaxThinkingTokens: 32000, SupportsVision: true, SupportsThinking: true},
	"claude-opus-4-20250514":        {ContextWindow: 1_000_000, MaxOutputTokens: 32000, MaxThinkingTokens: 32000, SupportsVision: true, SupportsThinking: true},
	// Legacy model IDs (kept for test compatibility and older API responses)
	"claude-3-5-sonnet-20241022":    {ContextWindow: 200_000, MaxOutputTokens: 8192, MaxThinkingTokens: 4096, SupportsVision: true, SupportsThinking: false},
	"claude-3-5-haiku-20241022":     {ContextWindow: 200_000, MaxOutputTokens: 8192, MaxThinkingTokens: 4096, SupportsVision: true, SupportsThinking: false},
	"claude-3-opus-20240229":        {ContextWindow: 200_000, MaxOutputTokens: 4096, MaxThinkingTokens: 0, SupportsVision: true, SupportsThinking: false},
	"claude-3-sonnet-20240229":      {ContextWindow: 200_000, MaxOutputTokens: 4096, MaxThinkingTokens: 0, SupportsVision: true, SupportsThinking: false},
	"claude-3-haiku-20240307":       {ContextWindow: 200_000, MaxOutputTokens: 4096, MaxThinkingTokens: 0, SupportsVision: true, SupportsThinking: false},
}

// NewModelCapabilitiesCache creates a new cache with the given directory.
func NewModelCapabilitiesCache(cacheDir string) *ModelCapabilitiesCache {
	mc := &ModelCapabilitiesCache{
		cache:    make(map[string]ModelCapabilities),
		cacheDir: cacheDir,
	}
	// Load from disk if available
	_ = mc.LoadFromDisk()
	return mc
}

// NewModelCapabilitiesCacheDefault creates a cache using the default Claude cache directory.
func NewModelCapabilitiesCacheDefault() *ModelCapabilitiesCache {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.Getenv("USERPROFILE")
	}
	if homeDir == "" {
		homeDir = "."
	}
	cacheDir := filepath.Join(homeDir, ".claude", "cache")
	return NewModelCapabilitiesCache(cacheDir)
}

// GetModelCapabilities returns capabilities for the given model.
// Checks in order: API cache > disk cache > built-in defaults > 200K fallback.
func (mc *ModelCapabilitiesCache) GetModelCapabilities(model string) ModelCapabilities {
	mc.mu.RLock()
	if caps, ok := mc.cache[model]; ok {
		mc.mu.RUnlock()
		return caps
	}
	mc.mu.RUnlock()

	// Check built-in defaults
	if caps, ok := DefaultModelCapabilities[model]; ok {
		return caps
	}

	// Check for partial matches using known model prefixes
	knownPrefixes := []string{
		"claude-sonnet-4",
		"claude-opus-4",
		"claude-haiku-4",
		"claude-3-5-sonnet",
		"claude-3-5-haiku",
		"claude-3-7-sonnet",
	}
	for _, prefix := range knownPrefixes {
		if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
			// Return a reasonable default for models matching known families
			return ModelCapabilities{
				ContextWindow:     200_000,
				MaxOutputTokens:   8192,
				MaxThinkingTokens: 4096,
				SupportsVision:    true,
				SupportsThinking:  true,
			}
		}
	}

	// Final fallback: generic 200K Anthropic model default
	return ModelCapabilities{
		ContextWindow:     200_000,
		MaxOutputTokens:   8192,
		MaxThinkingTokens: 4096,
		SupportsVision:    false,
		SupportsThinking:  false,
	}
}

// GetContextWindow returns the context window for the given model.
// Respects CLAUDE_CODE_MAX_CONTEXT_TOKENS env override.
func (mc *ModelCapabilitiesCache) GetContextWindow(model string) int64 {
	// Environment variable override
	if override := os.Getenv("CLAUDE_CODE_MAX_CONTEXT_TOKENS"); override != "" {
		if val, err := strconv.ParseInt(override, 10, 64); err == nil && val > 0 {
			return val
		}
	}

	caps := mc.GetModelCapabilities(model)
	return caps.ContextWindow
}

// GetMaxOutputTokens returns the max output tokens for the given model.
// Respects CLAUDE_CODE_MAX_OUTPUT_TOKENS env override.
func (mc *ModelCapabilitiesCache) GetMaxOutputTokens(model string) int64 {
	// Environment variable override
	if override := os.Getenv("CLAUDE_CODE_MAX_OUTPUT_TOKENS"); override != "" {
		if val, err := strconv.ParseInt(override, 10, 64); err == nil && val > 0 {
			return val
		}
	}

	caps := mc.GetModelCapabilities(model)
	return caps.MaxOutputTokens
}

// RefreshFromAPI fetches model capabilities from the Anthropic /v1/models endpoint
// and updates the in-memory cache and disk cache.
func (mc *ModelCapabilitiesCache) RefreshFromAPI(apiKey, baseURL string) error {
	if apiKey == "" {
		return fmt.Errorf("no API key provided for model capabilities refresh")
	}
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	url := baseURL + "/v1/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("model capabilities request: %w", err)
	}
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("model capabilities request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("model capabilities API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return fmt.Errorf("reading model capabilities response: %w", err)
	}

	var apiResp struct {
		Data []struct {
			ID         string `json:"id"`
			Created    int64  `json:"created"`
			Capabilities struct {
				ContextWindow int64 `json:"context_window"`
			} `json:"capabilities"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("parsing model capabilities response: %w", err)
	}

	mc.mu.Lock()
	for _, m := range apiResp.Data {
		// Only update cache with API values, preserving known defaults for
		// fields not provided by the API (max_output_tokens, thinking, etc.)
		if existing, ok := mc.cache[m.ID]; ok {
			existing.ContextWindow = m.Capabilities.ContextWindow
			mc.cache[m.ID] = existing
		} else if defaults, ok := DefaultModelCapabilities[m.ID]; ok {
			defaults.ContextWindow = m.Capabilities.ContextWindow
			mc.cache[m.ID] = defaults
		} else {
			mc.cache[m.ID] = ModelCapabilities{
				ContextWindow:     m.Capabilities.ContextWindow,
				MaxOutputTokens:   8192,
				MaxThinkingTokens: 4096,
				SupportsVision:    false,
				SupportsThinking:  false,
			}
		}
	}
	mc.loaded = true
	mc.mu.Unlock()

	// Persist to disk
	return mc.SaveToDisk()
}

// SaveToDisk persists the in-memory cache to model-capabilities.json.
func (mc *ModelCapabilitiesCache) SaveToDisk() error {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if err := os.MkdirAll(mc.cacheDir, 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	data, err := json.Marshal(mc.cache)
	if err != nil {
		return fmt.Errorf("marshaling cache: %w", err)
	}

	path := filepath.Join(mc.cacheDir, "model-capabilities.json")
	return os.WriteFile(path, data, 0o644)
}

// LoadFromDisk loads the cache from model-capabilities.json.
func (mc *ModelCapabilitiesCache) LoadFromDisk() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	path := filepath.Join(mc.cacheDir, "model-capabilities.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No cache file yet, not an error
		}
		return fmt.Errorf("reading cache file: %w", err)
	}

	// Only consider the cache fresh if it was written recently (< 24h)
	info, statErr := os.Stat(path)
	if statErr == nil && time.Since(info.ModTime()) > 24*time.Hour {
		// Stale cache — don't load it, let defaults take over until a refresh
		return nil
	}

	var loaded map[string]ModelCapabilities
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("parsing cache file: %w", err)
	}

	for k, v := range loaded {
		mc.cache[k] = v
	}
	mc.loaded = true
	return nil
}
