package main

import (
	"fmt"
	"sync"
)

// ─── Fork Context Caching (MiMo-Code 5) ────────────────────────────────────
//
// Captures a frozen snapshot of the LLM request prefix at fork-agent spawn time.
// Reuses this snapshot on every subsequent fork-agent turn for cache hits.
//
// MiMo-Code source: session/llm-request-prefix.ts (82 lines)

// ForkContext represents a frozen prefix snapshot for a fork agent.
type ForkContext struct {
	SystemPrompt string
	ToolSchemas  []string
	ModelMessages []string
	Hash         string
}

// ForkContextCache manages fork context snapshots.
type ForkContextCache struct {
	mu       sync.Mutex
	contexts map[string]*ForkContext // agentID -> context
}

// NewForkContextCache creates a new fork context cache.
func NewForkContextCache() *ForkContextCache {
	return &ForkContextCache{
		contexts: make(map[string]*ForkContext),
	}
}

// Capture captures a fork context snapshot.
func (c *ForkContextCache) Capture(agentID string, systemPrompt string, toolSchemas []string, modelMessages []string) *ForkContext {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx := &ForkContext{
		SystemPrompt:  systemPrompt,
		ToolSchemas:   toolSchemas,
		ModelMessages: modelMessages,
		Hash:          computePrefixHash(systemPrompt, toolSchemas, modelMessages),
	}

	c.contexts[agentID] = ctx
	return ctx
}

// Get returns the cached fork context for an agent.
func (c *ForkContextCache) Get(agentID string) *ForkContext {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.contexts[agentID]
}

// Clear removes the cached fork context for an agent.
func (c *ForkContextCache) Clear(agentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.contexts, agentID)
}

// ClearAll clears all cached fork contexts.
func (c *ForkContextCache) ClearAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contexts = make(map[string]*ForkContext)
}

// computePrefixHash computes a hash of the prefix components.
func computePrefixHash(systemPrompt string, toolSchemas []string, modelMessages []string) string {
	// Simple hash based on content lengths
	total := len(systemPrompt)
	for _, s := range toolSchemas {
		total += len(s)
	}
	for _, s := range modelMessages {
		total += len(s)
	}
	return fmt.Sprintf("%d", total)
}

// FormatForkContext formats a fork context for display.
func FormatForkContext(ctx *ForkContext) string {
	if ctx == nil {
		return "No fork context."
	}

	var sb string
	sb += "## Fork Context\n\n"
	sb += fmt.Sprintf("- System prompt: %d chars\n", len(ctx.SystemPrompt))
	sb += fmt.Sprintf("- Tool schemas: %d tools\n", len(ctx.ToolSchemas))
	sb += fmt.Sprintf("- Model messages: %d messages\n", len(ctx.ModelMessages))
	sb += "- Hash: " + ctx.Hash + "\n"
	return sb
}
