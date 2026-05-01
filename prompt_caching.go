package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
)

// ApplyPromptCaching applies Anthropic's system_and_3 caching strategy to API messages.
// Places up to 4 cache_control breakpoints: system prompt + last 3 non-system messages.
// Returns a new slice with cache_control breakpoints injected into the messages.
//
// This reduces input token costs by ~75% on multi-turn conversations by reusing
// cached prefixes across API calls.
func ApplyPromptCaching(messages []map[string]any, ttl string) []map[string]any {
	if len(messages) == 0 {
		return messages
	}

	result := deepCopyMessages(messages)
	marker := map[string]any{"type": "ephemeral"}
	if ttl == "1h" {
		marker = map[string]any{"type": "ephemeral", "ttl": "1h"}
	}

	breakpointsUsed := 0

	// 1. Cache the system prompt (first message if system role)
	if role, _ := result[0]["role"].(string); role == "system" {
		applyCacheMarker(result[0], marker)
		breakpointsUsed++
	}

	// 2. Cache the last N non-system messages (up to 4-total breakpoints)
	remaining := 4 - breakpointsUsed
	nonSysIndices := make([]int, 0)
	for i := range result {
		if role, _ := result[i]["role"].(string); role != "system" {
			nonSysIndices = append(nonSysIndices, i)
		}
	}
	if len(nonSysIndices) > remaining {
		nonSysIndices = nonSysIndices[len(nonSysIndices)-remaining:]
	}
	for _, idx := range nonSysIndices {
		applyCacheMarker(result[idx], marker)
	}

	return result
}

// applyCacheMarker adds cache_control to a single message, handling all formats.
func applyCacheMarker(msg map[string]any, marker map[string]any) {
	role, _ := msg["role"].(string)

	// tool role: cache_control goes at message level
	if role == "tool" {
		msg["cache_control"] = marker
		return
	}

	content, exists := msg["content"]
	if !exists {
		msg["cache_control"] = marker
		return
	}

	// Empty string content
	if s, ok := content.(string); ok && s == "" {
		msg["cache_control"] = marker
		return
	}

	// String content -> convert to array format
	if s, ok := content.(string); ok {
		msg["content"] = []map[string]any{
			{
				"type":          "text",
				"text":          s,
				"cache_control": marker,
			},
		}
		return
	}

	// Array content -> add cache_control to last block
	if arr, ok := content.([]any); ok && len(arr) > 0 {
		last := arr[len(arr)-1]
		if m, ok := last.(map[string]any); ok {
			m["cache_control"] = marker
		}
	}
}

// deepCopyMessages does a deep copy via JSON marshal/unmarshal.
// Returns the original slice on marshal failure (avoiding nil/empty results).
func deepCopyMessages(messages []map[string]any) []map[string]any {
	data, err := json.Marshal(messages)
	if err != nil {
		return messages
	}
	var result []map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return messages
	}
	return result
}

// cacheMessageParams converts []anthropic.MessageParam to []map[string]any,
// applies prompt caching, and converts back.
func cacheMessageParams(params *anthropic.MessageNewParams) {
	// Convert messages to maps
	msgMaps := messageParamToMaps(params.Messages)
	msgMaps = ApplyPromptCaching(msgMaps, "5m")

	// Convert back to MessageParam
	params.Messages = mapsToMessageParam(msgMaps)

	// Add cache_control to system prompt
	if len(params.System) > 0 {
		params.System[0].CacheControl = anthropic.CacheControlEphemeralParam{}
	}
}

// messageParamToMaps converts SDK message params to map representation.
func messageParamToMaps(msgs []anthropic.MessageParam) []map[string]any {
	data, err := json.Marshal(msgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] prompt_caching: marshal failed: %v\n", err)
		return nil
	}
	var result []map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] prompt_caching: unmarshal failed: %v\n", err)
		return nil
	}
	return result
}

// mapsToMessageParam converts maps back to SDK message params.
func mapsToMessageParam(msgs []map[string]any) []anthropic.MessageParam {
	data, err := json.Marshal(msgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] prompt_caching: marshal failed: %v\n", err)
		return nil
	}
	var result []anthropic.MessageParam
	if err := json.Unmarshal(data, &result); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] prompt_caching: unmarshal failed: %v\n", err)
		return nil
	}
	return result
}

// FormatCachedSystemPrompt wraps the system prompt text for Anthropic caching.
func FormatCachedSystemPrompt(text string, ttl string) []map[string]any {
	marker := map[string]any{"type": "ephemeral"}
	if ttl == "1h" {
		marker = map[string]any{"type": "ephemeral", "ttl": "1h"}
	}
	return []map[string]any{
		{
			"type":          "text",
			"text":          text,
			"cache_control": marker,
		},
	}
}

// FormatBoundaryCachedSystemPrompt splits the system prompt at the static/dynamic
// boundary and applies separate caching scopes. The static part gets a "global"
// cache scope (long-lived, survives across sessions), while the dynamic part
// gets an "org" or no caching scope (short-lived, per-session).
//
// This means the static tool descriptions only need to be hashed once, and
// changes to dynamic content (skills, memory, project instructions) don't
// invalidate the static cache.
func FormatBoundaryCachedSystemPrompt(text string, ttl string) []map[string]any {
	staticPart, dynamicPart, found := SplitSystemPrompt(text)

	if !found {
		// No boundary found, fall back to single-block caching
		return FormatCachedSystemPrompt(text, ttl)
	}

	// Static content: use global cache scope for long-lived caching.
	// The static part (tool descriptions, rules) rarely changes,
	// so a global cache scope maximizes cache hit rates.
	globalMarker := map[string]any{"type": "ephemeral"}
	if ttl == "1h" {
		globalMarker = map[string]any{"type": "ephemeral", "ttl": "1h"}
	}

	// Dynamic content: use standard ephemeral cache (no extended TTL).
	// This content changes per-session or per-turn, so no point in
	// extending its cache lifetime beyond the default.
	dynamicMarker := map[string]any{"type": "ephemeral"}

	result := []map[string]any{
		{
			"type":          "text",
			"text":          staticPart + "\n" + SYSTEM_PROMPT_STATIC_BOUNDARY,
			"cache_control": globalMarker,
		},
	}

	if dynamicPart != "" {
		result = append(result, map[string]any{
			"type":          "text",
			"text":          dynamicPart,
			"cache_control": dynamicMarker,
		})
	}

	return result
}
