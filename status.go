package main

import (
	"fmt"
	"time"
)

// handleStatus handles the /status slash command.
// Shows comprehensive session status: model, tokens, cache, cost, etc.
func handleStatus(agent *AgentLoop) {
	fmt.Println("\n=== Session Status ===")

	// Model & Mode
	fmt.Printf("Model: %s\n", agent.config.Model)
	fmt.Printf("Mode:  %s\n", agent.config.PermissionMode)

	// Context
	entries := agent.context.Entries()
	estTokens := agent.context.EstimatedTokens()
	remaining := agent.RemainingTokenBudget()
	fmt.Printf("Messages: %d (est. %s tokens)\n", len(entries), formatTokenCount(int64(estTokens)))
	fmt.Printf("Token Budget: %s remaining\n", formatTokenCount(int64(remaining)))

	// Token usage
	inputTokens := agent.totalInputTokens.Load()
	outputTokens := agent.totalOutputTokens.Load()
	cacheCreation := agent.totalCacheCreationTokens.Load()
	cacheRead := agent.totalCacheReadTokens.Load()
	cacheDeleted := agent.totalCacheEditsDeletions.Load()

	fmt.Printf("Input Tokens:    %s\n", formatTokenCount(inputTokens))
	fmt.Printf("Output Tokens:   %s\n", formatTokenCount(outputTokens))
	fmt.Printf("Cache Creation:  %s  Cache Read: %s  Cache Deleted: %s\n",
		formatTokenCount(cacheCreation),
		formatTokenCount(cacheRead),
		formatTokenCount(cacheDeleted))

	// Cache hit rate
	totalCacheTokens := cacheCreation + cacheRead
	if totalCacheTokens > 0 {
		hitRate := float64(cacheRead) / float64(totalCacheTokens) * 100
		fmt.Printf("Cache Hit Rate:  %.0f%%\n", hitRate)
	} else {
		fmt.Println("Cache Hit Rate:  N/A (no cache usage yet)")
	}

	// Cost tracking
	if agent.costTracker != nil {
		fmt.Printf("Token Usage:     %s\n", agent.costTracker.FormatCostDisplay())
	}

	// Turns
	turns := agent.budget.Consumed()
	fmt.Printf("Turns:           %d\n", turns)

	// Streaming status
	if agent.IsStreaming() {
		fmt.Println("Streaming:       enabled")
	} else {
		fmt.Println("Streaming:       disabled")
	}

	fmt.Println("======================")
}

// handleStatusWithUptime is called when start time is available.
func handleStatusWithUptime(agent *AgentLoop, startTime time.Time) {
	handleStatus(agent)
	// Could append uptime if needed
	_ = time.Since(startTime)
}
