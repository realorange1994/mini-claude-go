// Package modelregistry provides model resolution and pattern matching.
// This file implements the resolver aligned to pi's model-resolver.ts.
package modelregistry

import (
	"fmt"
	"strings"
	"sync"
)

// modelPatterns maps short aliases to concrete model IDs.
// Protected by patternsMu for concurrent access via RegisterAlias.
var (
	modelPatterns = map[string]string{
		"sonnet":        "claude-sonnet-4-20250514",
		"sonnet4":       "claude-sonnet-4-20250514",
		"claude-sonnet":  "claude-sonnet-4-20250514",
		"opus":          "claude-opus-4-20250514",
		"opus4":         "claude-opus-4-20250514",
		"claude-opus":    "claude-opus-4-20250514",
		"haiku":         "claude-haiku-3-5-20241022",
		"haiku3.5":      "claude-haiku-3-5-20241022",
		"claude-haiku":   "claude-haiku-3-5-20241022",
	}
	patternsMu sync.RWMutex
)

// Resolve resolves a model identifier to a concrete ModelInfo.
// It handles:
//   - Exact model IDs (e.g., "claude-sonnet-4-20250514")
//   - Short aliases (e.g., "sonnet", "opus", "haiku")
//   - Provider-prefixed IDs (e.g., "anthropic:sonnet")
//   - Custom models from the registry
func (r *Registry) Resolve(id string) (ModelInfo, error) {
	if id == "" {
		return ModelInfo{}, fmt.Errorf("empty model id")
	}

	// 1. Check provider prefix: "anthropic:sonnet"
	if parts := strings.SplitN(id, ":", 2); len(parts) == 2 {
		provider, modelID := parts[0], parts[1]
		// Resolve the model part (may be an alias)
		resolved := r.resolveAlias(modelID)
		if m, ok := r.Find(provider, resolved); ok {
			return m, nil
		}
		if m, ok := r.FindByID(resolved); ok {
			return m, nil
		}
		return ModelInfo{}, fmt.Errorf("model %q not found for provider %q", modelID, provider)
	}

	// 2. Try exact match in registry
	if m, ok := r.FindByID(id); ok {
		return m, nil
	}

	// 3. Try alias resolution
	resolved := r.resolveAlias(id)
	if resolved != id {
		if m, ok := r.FindByID(resolved); ok {
			return m, nil
		}
	}

	// 4. Try case-insensitive match
	lowerID := strings.ToLower(id)
	for _, m := range r.All() {
		if strings.ToLower(m.ID) == lowerID {
			return m, nil
		}
	}

	// 5. Partial match: if id is a prefix of a model ID
	for _, m := range r.All() {
		if strings.HasPrefix(strings.ToLower(m.ID), lowerID) {
			return m, nil
		}
	}

	return ModelInfo{}, fmt.Errorf("model %q not found (tried exact, alias, case-insensitive, prefix)", id)
}

// resolveAlias maps a short name to a concrete model ID.
func (r *Registry) resolveAlias(id string) string {
	patternsMu.RLock()
	defer patternsMu.RUnlock()
	if resolved, ok := modelPatterns[strings.ToLower(id)]; ok {
		return resolved
	}
	return id
}

// RegisterAlias adds a custom alias for model resolution.
func (r *Registry) RegisterAlias(alias, modelID string) {
	patternsMu.Lock()
	defer patternsMu.Unlock()
	modelPatterns[strings.ToLower(alias)] = modelID
}

// Aliases returns all registered aliases.
func Aliases() map[string]string {
	patternsMu.RLock()
	defer patternsMu.RUnlock()
	out := make(map[string]string, len(modelPatterns))
	for k, v := range modelPatterns {
		out[k] = v
	}
	return out
}
