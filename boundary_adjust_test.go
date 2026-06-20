package main

import (
	"testing"
)

func TestAdjustBoundary_NoSplit(t *testing.T) {
	messages := []BoundaryMessage{
		{Role: "user", Content: []BoundaryContent{{Type: "text"}}},
		{Role: "assistant", Content: []BoundaryContent{{Type: "text"}}},
		{Role: "user", Content: []BoundaryContent{{Type: "text"}}},
	}

	idx := AdjustBoundaryForApiInvariants(messages, 2)
	if idx != 2 {
		t.Errorf("expected 2, got %d", idx)
	}
}

func TestAdjustBoundary_ToolPairing(t *testing.T) {
	messages := []BoundaryMessage{
		{Role: "user", Content: []BoundaryContent{{Type: "text"}}},
		{Role: "assistant", Content: []BoundaryContent{
			{Type: "tool_use", ID: "call-1"},
		}},
		{Role: "user", Content: []BoundaryContent{
			{Type: "tool_result", ToolUseID: "call-1"},
		}},
		{Role: "assistant", Content: []BoundaryContent{{Type: "text"}}},
	}

	// Boundary at 2 would orphan tool_result at index 2
	idx := AdjustBoundaryForApiInvariants(messages, 2)
	if idx != 1 {
		t.Errorf("expected 1 (include tool_use), got %d", idx)
	}
}

func TestAdjustBoundary_ThinkingBlocks(t *testing.T) {
	messages := []BoundaryMessage{
		{Role: "user", Content: []BoundaryContent{{Type: "text"}}},
		{Role: "assistant", ID: "msg-1", Content: []BoundaryContent{
			{Type: "thinking"},
		}},
		{Role: "assistant", ID: "msg-1", Content: []BoundaryContent{
			{Type: "text"},
		}},
	}

	// Boundary at 2 should walk back to include thinking at 1
	idx := AdjustBoundaryForApiInvariants(messages, 2)
	if idx != 1 {
		t.Errorf("expected 1 (include thinking), got %d", idx)
	}
}

func TestAdjustBoundary_ZeroBoundary(t *testing.T) {
	messages := []BoundaryMessage{
		{Role: "user", Content: []BoundaryContent{{Type: "text"}}},
	}

	idx := AdjustBoundaryForApiInvariants(messages, 0)
	if idx != 0 {
		t.Errorf("expected 0, got %d", idx)
	}
}

func TestAlignToNonToolResultUser(t *testing.T) {
	messages := []BoundaryMessage{
		{Role: "user", Content: []BoundaryContent{{Type: "text"}}},
		{Role: "assistant", Content: []BoundaryContent{{Type: "text"}}},
		{Role: "user", Content: []BoundaryContent{
			{Type: "tool_result", ToolUseID: "call-1"},
		}},
		{Role: "assistant", Content: []BoundaryContent{{Type: "text"}}},
	}

	idx := AlignToNonToolResultUser(messages, 3)
	if idx != 0 {
		t.Errorf("expected 0 (first user message), got %d", idx)
	}
}

func TestAlignToNonToolResultUser_AllToolResult(t *testing.T) {
	messages := []BoundaryMessage{
		{Role: "user", Content: []BoundaryContent{
			{Type: "tool_result", ToolUseID: "call-1"},
		}},
		{Role: "assistant", Content: []BoundaryContent{{Type: "text"}}},
	}

	idx := AlignToNonToolResultUser(messages, 1)
	if idx != 0 {
		t.Errorf("expected 0 (fallback), got %d", idx)
	}
}
