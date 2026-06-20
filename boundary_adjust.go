package main

// ─── Boundary Adjustment for API Invariants (MiMo-Code 4C) ─────────────────
//
// Ensures tool_use/tool_result pairs are not split across summary/tail boundary,
// and that thinking blocks sharing a message.id with kept assistants are included.
//
// MiMo-Code source: session/boundary.ts (77 lines)

// BoundaryMessage represents a message for boundary adjustment.
type BoundaryMessage struct {
	Role    string
	ID      string
	Content []BoundaryContent
}

// BoundaryContent represents a content block in a message.
type BoundaryContent struct {
	Type      string // "tool_use", "tool_result", "text", "thinking"
	ID        string // for tool_use
	ToolUseID string // for tool_result
}

// AdjustBoundaryForApiInvariants walks the boundary backward to ensure
// tool_use/tool_result pairs are not split across the summary/tail divide.
// Returns the adjusted boundary index.
func AdjustBoundaryForApiInvariants(messages []BoundaryMessage, candidateBoundary int) int {
	if candidateBoundary <= 0 || candidateBoundary >= len(messages) {
		return candidateBoundary
	}

	idx := candidateBoundary

	// Step 1: tool_use/tool_result pairing
	tailToolResults := make(map[string]bool)
	tailToolUses := make(map[string]bool)

	for i := idx; i < len(messages); i++ {
		for _, block := range messages[i].Content {
			if block.Type == "tool_result" && block.ToolUseID != "" {
				tailToolResults[block.ToolUseID] = true
			}
			if block.Type == "tool_use" && block.ID != "" {
				tailToolUses[block.ID] = true
			}
		}
	}

	// Find orphaned tool_results (results without matching use in tail)
	orphans := make(map[string]bool)
	for id := range tailToolResults {
		if !tailToolUses[id] {
			orphans[id] = true
		}
	}

	// Walk backward to find matching tool_use blocks
	for i := idx - 1; i >= 0 && len(orphans) > 0; i-- {
		if messages[i].Role != "assistant" {
			continue
		}
		for _, block := range messages[i].Content {
			if block.Type == "tool_use" && block.ID != "" && orphans[block.ID] {
				idx = i
				delete(orphans, block.ID)
			}
		}
	}

	// Step 2: same message.id walk-back (thinking blocks share id with sibling)
	if idx > 0 && idx < len(messages) {
		boundaryMsgID := messages[idx].ID
		if boundaryMsgID != "" {
			for i := idx - 1; i >= 0; i-- {
				if messages[i].ID == boundaryMsgID {
					idx = i
				} else {
					break
				}
			}
		}
	}

	return idx
}

// AlignToNonToolResultUser walks backward to find a user message that is NOT
// entirely tool_result parts. Prevents checkpoint writer from receiving
// malformed message slices.
func AlignToNonToolResultUser(messages []BoundaryMessage, startIdx int) int {
	for i := startIdx; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		// Check if all content is tool_result
		allToolResult := true
		for _, block := range messages[i].Content {
			if block.Type != "tool_result" {
				allToolResult = false
				break
			}
		}
		if !allToolResult {
			return i
		}
	}
	return 0
}
