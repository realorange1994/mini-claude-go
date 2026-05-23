package main

import (
	"testing"
)

// ─── getTokenCountFromUsage ──────────────────────────────────────────────
// Ported from upstream tokens.test.ts

func TestGetTokenCountFromUsageAllFields(t *testing.T) {
	usage := &UsageInfo{
		InputTokens:              100,
		OutputTokens:             50,
		CacheCreationInputTokens: 20,
		CacheReadInputTokens:     10,
	}
	result := getTokenCountFromUsage(usage)
	if result != 180 {
		t.Errorf("expected 180, got %d", result)
	}
}

func TestGetTokenCountFromUsageMissingCache(t *testing.T) {
	usage := &UsageInfo{
		InputTokens:  100,
		OutputTokens: 50,
	}
	result := getTokenCountFromUsage(usage)
	if result != 150 {
		t.Errorf("expected 150, got %d", result)
	}
}

func TestGetTokenCountFromUsageAllZeros(t *testing.T) {
	usage := &UsageInfo{}
	result := getTokenCountFromUsage(usage)
	if result != 0 {
		t.Errorf("expected 0, got %d", result)
	}
}

func TestGetTokenCountFromUsageNil(t *testing.T) {
	result := getTokenCountFromUsage(nil)
	if result != 0 {
		t.Errorf("expected 0 for nil, got %d", result)
	}
}

// ─── getTokenUsage ───────────────────────────────────────────────────────
// Ported from upstream tokens.test.ts

func TestGetTokenUsageAssistantMessage(t *testing.T) {
	msg := Message{
		Type:  "assistant",
		Model: "claude-sonnet-4-20250514",
		Usage: &UsageInfo{InputTokens: 100, OutputTokens: 50},
	}
	usage := getTokenUsage(msg)
	if usage == nil {
		t.Error("expected usage for assistant message")
	}
	if usage.InputTokens != 100 {
		t.Errorf("expected InputTokens=100, got %d", usage.InputTokens)
	}
}

func TestGetTokenUsageUserMessage(t *testing.T) {
	msg := Message{Type: "user"}
	usage := getTokenUsage(msg)
	if usage != nil {
		t.Errorf("expected nil for user message, got %v", usage)
	}
}

func TestGetTokenUsageSyntheticModel(t *testing.T) {
	msg := Message{
		Type:  "assistant",
		Model: "<synthetic>",
		Usage: &UsageInfo{InputTokens: 10, OutputTokens: 5},
	}
	usage := getTokenUsage(msg)
	if usage != nil {
		t.Errorf("expected nil for synthetic model, got %v", usage)
	}
}

func TestGetTokenUsageNoUsage(t *testing.T) {
	msg := Message{Type: "assistant", Model: "claude"}
	usage := getTokenUsage(msg)
	if usage != nil {
		t.Errorf("expected nil for message without usage, got %v", usage)
	}
}

// ─── tokenCountFromLastAPIResponse ───────────────────────────────────────
// Ported from upstream tokens.test.ts

func TestTokenCountFromLastAPIResponseWithMessages(t *testing.T) {
	msgs := []Message{
		{
			Type:  "assistant",
			Model: "claude",
			Usage: &UsageInfo{
				InputTokens:              200,
				OutputTokens:             100,
				CacheCreationInputTokens: 50,
				CacheReadInputTokens:     25,
			},
		},
	}
	result := tokenCountFromLastAPIResponse(msgs)
	if result != 375 {
		t.Errorf("expected 375, got %d", result)
	}
}

func TestTokenCountFromLastAPIResponseEmpty(t *testing.T) {
	result := tokenCountFromLastAPIResponse(nil)
	if result != 0 {
		t.Errorf("expected 0 for empty messages, got %d", result)
	}
}

func TestTokenCountFromLastAPIResponseSkipsUser(t *testing.T) {
	msgs := []Message{
		{
			Type:  "assistant",
			Model: "claude",
			Usage: &UsageInfo{InputTokens: 100, OutputTokens: 50},
		},
		{Type: "user"},
	}
	result := tokenCountFromLastAPIResponse(msgs)
	if result != 150 {
		t.Errorf("expected 150, got %d", result)
	}
}

func TestTokenCountFromLastAPIResponseFindsLastAssistant(t *testing.T) {
	msgs := []Message{
		{
			Type:  "assistant",
			Model: "claude",
			Usage: &UsageInfo{InputTokens: 50, OutputTokens: 20},
		},
		{Type: "user"},
		{
			Type:  "assistant",
			Model: "claude",
			Usage: &UsageInfo{InputTokens: 100, OutputTokens: 50},
		},
	}
	result := tokenCountFromLastAPIResponse(msgs)
	if result != 150 {
		t.Errorf("expected 150 (last assistant), got %d", result)
	}
}

// ─── messageTokenCountFromLastAPIResponse ────────────────────────────────
// Ported from upstream tokens.test.ts

func TestMessageTokenCountFromLastAPIResponseWithMessages(t *testing.T) {
	msgs := []Message{
		{
			Type:  "assistant",
			Model: "claude",
			Usage: &UsageInfo{InputTokens: 200, OutputTokens: 75},
		},
	}
	result := messageTokenCountFromLastAPIResponse(msgs)
	if result != 75 {
		t.Errorf("expected 75 output tokens, got %d", result)
	}
}

func TestMessageTokenCountFromLastAPIResponseEmpty(t *testing.T) {
	result := messageTokenCountFromLastAPIResponse(nil)
	if result != 0 {
		t.Errorf("expected 0 for empty messages, got %d", result)
	}
}

// ─── getCurrentUsage ─────────────────────────────────────────────────────
// Ported from upstream tokens.test.ts

func TestGetCurrentUsageWithMessages(t *testing.T) {
	msgs := []Message{
		{
			Type:  "assistant",
			Model: "claude",
			Usage: &UsageInfo{
				InputTokens:              100,
				OutputTokens:             50,
				CacheCreationInputTokens: 10,
				CacheReadInputTokens:     5,
			},
		},
	}
	result := getCurrentUsage(msgs)
	if result == nil {
		t.Fatal("expected non-nil usage")
	}
	if result.InputTokens != 100 {
		t.Errorf("expected InputTokens=100, got %d", result.InputTokens)
	}
	if result.OutputTokens != 50 {
		t.Errorf("expected OutputTokens=50, got %d", result.OutputTokens)
	}
	if result.CacheCreationInputTokens != 10 {
		t.Errorf("expected CacheCreationInputTokens=10, got %d", result.CacheCreationInputTokens)
	}
	if result.CacheReadInputTokens != 5 {
		t.Errorf("expected CacheReadInputTokens=5, got %d", result.CacheReadInputTokens)
	}
}

func TestGetCurrentUsageEmpty(t *testing.T) {
	result := getCurrentUsage(nil)
	if result != nil {
		t.Errorf("expected nil for empty messages, got %v", result)
	}
}

func TestGetCurrentUsageDefaultsCacheToZero(t *testing.T) {
	msgs := []Message{
		{
			Type:  "assistant",
			Model: "claude",
			Usage: &UsageInfo{InputTokens: 100, OutputTokens: 50},
		},
	}
	result := getCurrentUsage(msgs)
	if result == nil {
		t.Fatal("expected non-nil usage")
	}
	if result.CacheCreationInputTokens != 0 {
		t.Errorf("expected default 0, got %d", result.CacheCreationInputTokens)
	}
	if result.CacheReadInputTokens != 0 {
		t.Errorf("expected default 0, got %d", result.CacheReadInputTokens)
	}
}

func TestGetCurrentUsageSkipsAllZeros(t *testing.T) {
	msgs := []Message{
		{
			Type:  "assistant",
			Model: "claude",
			Usage: &UsageInfo{}, // all zeros - placeholder
		},
		{
			Type:  "assistant",
			Model: "claude",
			Usage: &UsageInfo{InputTokens: 100, OutputTokens: 50},
		},
	}
	result := getCurrentUsage(msgs)
	if result == nil {
		t.Fatal("expected non-nil usage")
	}
	if result.InputTokens != 100 {
		t.Errorf("expected 100 (skipped placeholder), got %d", result.InputTokens)
	}
}

// ─── doesMostRecentAssistantMessageExceed200k ────────────────────────────
// Ported from upstream tokens.test.ts

func TestDoesMostRecentAssistantExceed200kUnder(t *testing.T) {
	msgs := []Message{
		{
			Type:  "assistant",
			Model: "claude",
			Usage: &UsageInfo{InputTokens: 1000, OutputTokens: 500},
		},
	}
	if doesMostRecentAssistantMessageExceed200k(msgs) {
		t.Error("expected false when under 200k")
	}
}

func TestDoesMostRecentAssistantExceed200kOver(t *testing.T) {
	msgs := []Message{
		{
			Type:  "assistant",
			Model: "claude",
			Usage: &UsageInfo{InputTokens: 190000, OutputTokens: 15000},
		},
	}
	if !doesMostRecentAssistantMessageExceed200k(msgs) {
		t.Error("expected true when over 200k")
	}
}

func TestDoesMostRecentAssistantExceed200kEmpty(t *testing.T) {
	if doesMostRecentAssistantMessageExceed200k(nil) {
		t.Error("expected false for empty messages")
	}
}

// ─── Invariants ──────────────────────────────────────────────────────────

func TestGetTokenCountFromUsageAlwaysNonNegative(t *testing.T) {
	// All-zero usage should be 0, not negative
	usage := &UsageInfo{}
	if getTokenCountFromUsage(usage) < 0 {
		t.Error("token count should never be negative")
	}
}

func TestGetTokenCountFromUsageAdditive(t *testing.T) {
	// getTokenCountFromUsage should be additive: sum of all fields
	usage := &UsageInfo{
		InputTokens:              100,
		OutputTokens:             50,
		CacheCreationInputTokens: 30,
		CacheReadInputTokens:     20,
	}
	expected := 100 + 50 + 30 + 20
	if getTokenCountFromUsage(usage) != expected {
		t.Errorf("additive invariant broken: expected %d, got %d", expected, getTokenCountFromUsage(usage))
	}
}
