package main

import (
	"testing"
)

// Ported from upstream: src/utils/__tests__/collapseHookSummaries.test.ts

func makeHookSummary(hookLabel string, hookCount int, preventedContinuation bool, totalDurationMs int) *HookSummary {
	return &HookSummary{
		Type:                 "system",
		Subtype:              "stop_hook_summary",
		HookLabel:            hookLabel,
		HookCount:            hookCount,
		HookInfos:            []any{},
		HookErrors:           []any{},
		PreventedContinuation: preventedContinuation,
		HasOutput:            false,
		TotalDurationMs:      totalDurationMs,
	}
}

func makeNonHookMessage() string {
	return "user-message"
}

func TestCollapseHookSummariesNoHookSummaries(t *testing.T) {
	messages := []any{makeNonHookMessage(), makeNonHookMessage()}
	result := CollapseHookSummaries(messages)
	if len(result) != len(messages) {
		t.Errorf("expected %d messages, got %d", len(messages), len(result))
	}
}

func TestCollapseHookSummariesConsecutiveSameLabel(t *testing.T) {
	messages := []any{
		makeHookSummary("PostToolUse", 1, false, 10),
		makeHookSummary("PostToolUse", 2, false, 10),
	}
	result := CollapseHookSummaries(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	summary := result[0].(*HookSummary)
	if summary.HookCount != 3 {
		t.Errorf("expected hookCount=3, got %d", summary.HookCount)
	}
}

func TestCollapseHookSummariesDifferentLabels(t *testing.T) {
	messages := []any{
		makeHookSummary("PostToolUse", 1, false, 10),
		makeHookSummary("PreToolUse", 1, false, 10),
	}
	result := CollapseHookSummaries(messages)
	if len(result) != 2 {
		t.Errorf("expected 2 messages for different labels, got %d", len(result))
	}
}

func TestCollapseHookSummariesAggregatesHookCount(t *testing.T) {
	messages := []any{
		makeHookSummary("A", 3, false, 10),
		makeHookSummary("A", 5, false, 10),
	}
	result := CollapseHookSummaries(messages)
	summary := result[0].(*HookSummary)
	if summary.HookCount != 8 {
		t.Errorf("expected hookCount=8, got %d", summary.HookCount)
	}
}

func TestCollapseHookSummariesMergesHookInfos(t *testing.T) {
	info1 := map[string]string{"tool": "Read"}
	info2 := map[string]string{"tool": "Write"}
	s1 := makeHookSummary("A", 1, false, 10)
	s1.HookInfos = []any{info1}
	s2 := makeHookSummary("A", 1, false, 10)
	s2.HookInfos = []any{info2}

	messages := []any{s1, s2}
	result := CollapseHookSummaries(messages)
	summary := result[0].(*HookSummary)
	if len(summary.HookInfos) != 2 {
		t.Errorf("expected 2 hookInfos, got %d", len(summary.HookInfos))
	}
}

func TestCollapseHookSummariesMaxDuration(t *testing.T) {
	messages := []any{
		makeHookSummary("A", 1, false, 50),
		makeHookSummary("A", 1, false, 100),
		makeHookSummary("A", 1, false, 75),
	}
	result := CollapseHookSummaries(messages)
	summary := result[0].(*HookSummary)
	if summary.TotalDurationMs != 100 {
		t.Errorf("expected max totalDurationMs=100, got %d", summary.TotalDurationMs)
	}
}

func TestCollapseHookSummariesPreventedContinuation(t *testing.T) {
	messages := []any{
		makeHookSummary("A", 1, false, 10),
		makeHookSummary("A", 1, true, 10),
	}
	result := CollapseHookSummaries(messages)
	summary := result[0].(*HookSummary)
	if summary.PreventedContinuation != true {
		t.Error("expected preventedContinuation=true when any is true")
	}
}

func TestCollapseHookSummariesSingleUnchanged(t *testing.T) {
	msg := makeHookSummary("PostToolUse", 5, false, 10)
	result := CollapseHookSummaries([]any{msg})
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	summary := result[0].(*HookSummary)
	if summary.HookCount != 5 {
		t.Errorf("expected hookCount=5, got %d", summary.HookCount)
	}
}

func TestCollapseHookSummariesThreeConsecutive(t *testing.T) {
	messages := []any{
		makeHookSummary("X", 1, false, 10),
		makeHookSummary("X", 1, false, 10),
		makeHookSummary("X", 1, false, 10),
	}
	result := CollapseHookSummaries(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	summary := result[0].(*HookSummary)
	if summary.HookCount != 3 {
		t.Errorf("expected hookCount=3, got %d", summary.HookCount)
	}
}

func TestCollapseHookSummariesPreservesNonHookBetween(t *testing.T) {
	messages := []any{
		makeHookSummary("A", 1, false, 10),
		makeNonHookMessage(),
		makeHookSummary("A", 1, false, 10),
	}
	result := CollapseHookSummaries(messages)
	if len(result) != 3 {
		t.Errorf("expected 3 messages (non-hook separates groups), got %d", len(result))
	}
}

func TestCollapseHookSummariesEmpty(t *testing.T) {
	result := CollapseHookSummaries([]any{})
	if len(result) != 0 {
		t.Errorf("expected empty array for empty input, got %d", len(result))
	}
}
