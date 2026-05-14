package main

import (
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
