package main

import (
	"fmt"
	"testing"
)

// Ported from upstream: src/utils/__tests__/groupToolUses.test.ts

func TestApplyGroupingSingleToolUse(t *testing.T) {
	entries := []ToolUseEntry{
		{ID: "tu1", Name: "Grep", Input: "{}", Output: "ok", Status: "success"},
	}
	result := ApplyGrouping(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result))
	}
	if result[0].IsGrouped {
		t.Error("single tool use should not be grouped")
	}
}

func TestApplyGroupingTwoSameTool(t *testing.T) {
	entries := []ToolUseEntry{
		{ID: "tu1", Name: "Grep", Input: "{}", Output: "ok", Status: "success"},
		{ID: "tu2", Name: "Grep", Input: "{}", Output: "ok", Status: "success"},
	}
	result := ApplyGrouping(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result))
	}
	if !result[0].IsGrouped {
		t.Error("two same tool uses should be grouped")
	}
	if len(result[0].ToolUses) != 2 {
		t.Errorf("expected 2 tool uses in group, got %d", len(result[0].ToolUses))
	}
}

func TestApplyGroupingDifferentToolsNotGrouped(t *testing.T) {
	entries := []ToolUseEntry{
		{ID: "tu1", Name: "Grep", Input: "{}", Output: "ok", Status: "success"},
		{ID: "tu2", Name: "Bash", Input: "{}", Output: "ok", Status: "success"},
	}
	result := ApplyGrouping(entries)
	if len(result) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result))
	}
	if result[0].IsGrouped {
		t.Error("different tool uses should not be grouped")
	}
}

func TestApplyGroupingThreeSameTool(t *testing.T) {
	entries := []ToolUseEntry{
		{ID: "tu1", Name: "Grep", Input: "{}", Output: "ok", Status: "success"},
		{ID: "tu2", Name: "Grep", Input: "{}", Output: "ok", Status: "success"},
		{ID: "tu3", Name: "Grep", Input: "{}", Output: "ok", Status: "success"},
	}
	result := ApplyGrouping(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result))
	}
	if !result[0].IsGrouped {
		t.Error("three same tool uses should be grouped")
	}
	if len(result[0].ToolUses) != 3 {
		t.Errorf("expected 3 tool uses in group, got %d", len(result[0].ToolUses))
	}
}

func TestApplyGroupingEmpty(t *testing.T) {
	result := ApplyGrouping([]ToolUseEntry{})
	if len(result) != 0 {
		t.Errorf("expected 0 groups for empty input, got %d", len(result))
	}
}

func TestRenderGroupedToolUseSingle(t *testing.T) {
	group := ToolUseGroup{
		ID:        "tu1",
		Name:      "Grep",
		IsGrouped: false,
		ToolUses:  []ToolUseEntry{{ID: "tu1", Name: "Grep"}},
	}
	result := RenderGroupedToolUse(group)
	if result != "Grep(tu1)" {
		t.Errorf("expected Grep(tu1), got %s", result)
	}
}

func TestRenderGroupedToolUseMultiple(t *testing.T) {
	group := ToolUseGroup{
		ID:        "tu1",
		Name:      "Grep",
		IsGrouped: true,
		ToolUses: []ToolUseEntry{
			{ID: "tu1", Name: "Grep"},
			{ID: "tu2", Name: "Grep"},
			{ID: "tu3", Name: "Grep"},
		},
	}
	result := RenderGroupedToolUse(group)
	if result != "Grep(tu1) [x3]" {
		t.Errorf("expected Grep(tu1) [x3], got %s", result)
	}
}

// ─── Additional upstream patterns from groupToolUses.test.ts ─────────────────

func TestApplyGroupingMixedToolSequence(t *testing.T) {
	// Test: text + grep + grep + text should produce 3 groups
	// (the two consecutive grep should be grouped together)
	entries := []ToolUseEntry{
		{ID: "t1", Name: "Read", Output: "file1"},
		{ID: "t2", Name: "Grep", Output: "grep1"},
		{ID: "t3", Name: "Grep", Output: "grep2"},
		{ID: "t4", Name: "Write", Output: "wrote"},
	}
	result := ApplyGrouping(entries)
	// Should be: Read (ungrouped) + Grep (grouped) + Write (ungrouped)
	if len(result) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(result))
	}
	// First should be ungrouped Read
	if result[0].Name != "Read" || result[0].IsGrouped {
		t.Errorf("first group should be ungrouped Read, got %s grouped=%v", result[0].Name, result[0].IsGrouped)
	}
	// Second should be grouped Grep
	if result[1].Name != "Grep" || !result[1].IsGrouped {
		t.Errorf("second group should be grouped Grep, got %s grouped=%v", result[1].Name, result[1].IsGrouped)
	}
	if len(result[1].ToolUses) != 2 {
		t.Errorf("grouped Grep should have 2 tool uses, got %d", len(result[1].ToolUses))
	}
	// Third should be ungrouped Write
	if result[2].Name != "Write" || result[2].IsGrouped {
		t.Errorf("third group should be ungrouped Write, got %s grouped=%v", result[2].Name, result[2].IsGrouped)
	}
}

func TestApplyGroupingPreservesToolUseData(t *testing.T) {
	// Upstream invariant: grouped entries should preserve original data
	entries := []ToolUseEntry{
		{ID: "tu1", Name: "Grep", Input: `{"pattern": "foo"}`, Output: "result1", Status: "success"},
		{ID: "tu2", Name: "Grep", Input: `{"pattern": "bar"}`, Output: "result2", Status: "success"},
	}
	result := ApplyGrouping(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result))
	}
	// The group should contain both original entries
	if len(result[0].ToolUses) != 2 {
		t.Errorf("expected 2 tool uses in group, got %d", len(result[0].ToolUses))
	}
	// First entry should preserve original data
	if result[0].ToolUses[0].ID != "tu1" {
		t.Errorf("expected first entry ID tu1, got %s", result[0].ToolUses[0].ID)
	}
	if result[0].ToolUses[1].ID != "tu2" {
		t.Errorf("expected second entry ID tu2, got %s", result[0].ToolUses[1].ID)
	}
}

func TestApplyGroupingLongChainSameTool(t *testing.T) {
	// Upstream: grouping works for long chains of same tool
	var entries []ToolUseEntry
	for i := 0; i < 10; i++ {
		entries = append(entries, ToolUseEntry{
			ID: fmt.Sprintf("tu%d", i), Name: "Grep", Output: fmt.Sprintf("result%d", i),
		})
	}
	result := ApplyGrouping(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 group for 10 same tool uses, got %d", len(result))
	}
	if !result[0].IsGrouped {
		t.Error("long chain should be grouped")
	}
	if len(result[0].ToolUses) != 10 {
		t.Errorf("expected 10 tool uses, got %d", len(result[0].ToolUses))
	}
}

func TestApplyGroupingAlternatingTools(t *testing.T) {
	// Upstream: alternating tools should not be grouped
	entries := []ToolUseEntry{
		{ID: "t1", Name: "Grep", Output: "g1"},
		{ID: "t2", Name: "Read", Output: "r1"},
		{ID: "t3", Name: "Grep", Output: "g2"},
		{ID: "t4", Name: "Read", Output: "r2"},
	}
	result := ApplyGrouping(entries)
	if len(result) != 4 {
		t.Fatalf("expected 4 groups for alternating tools, got %d", len(result))
	}
	for i, g := range result {
		if g.IsGrouped {
			t.Errorf("group %d should not be grouped", i)
		}
	}
}

func TestApplyGroupingMixedChains(t *testing.T) {
	// Upstream: mixed consecutive and alternating
	entries := []ToolUseEntry{
		{ID: "t1", Name: "Grep", Output: "g1"},
		{ID: "t2", Name: "Grep", Output: "g2"}, // consecutive grep
		{ID: "t3", Name: "Read", Output: "r1"},
		{ID: "t4", Name: "Read", Output: "r2"}, // consecutive read
		{ID: "t5", Name: "Write", Output: "w1"},
	}
	result := ApplyGrouping(entries)
	if len(result) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(result))
	}
	// First: grouped Grep (2 entries)
	if !result[0].IsGrouped || len(result[0].ToolUses) != 2 {
		t.Error("first group should be grouped Grep with 2 entries")
	}
	// Second: grouped Read (2 entries)
	if !result[1].IsGrouped || len(result[1].ToolUses) != 2 {
		t.Error("second group should be grouped Read with 2 entries")
	}
	// Third: ungrouped Write
	if result[2].IsGrouped {
		t.Error("third group should be ungrouped Write")
	}
}

func TestRenderGroupedToolUseEmptyGroup(t *testing.T) {
	group := ToolUseGroup{
		ID:        "empty",
		Name:      "Grep",
		IsGrouped: false,
		ToolUses:  []ToolUseEntry{},
	}
	result := RenderGroupedToolUse(group)
	// Should render as ungrouped (just name + ID)
	if result != "Grep(empty)" {
		t.Errorf("expected Grep(empty), got %s", result)
	}
}
