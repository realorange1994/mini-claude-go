package main

// reasoningPadEnabled returns true if the conversation history contains
// evidence that a thinking-mode provider is in use (any assistant message
// has a Thinking or RedactedThinking block in its content).
// This uses structural inference from history content, not model-name
// matching, matching openclacky's ensure_reasoning_content_consistency.
func (c *ConversationContext) reasoningPadEnabled() bool {
	entries := c.entries
	for i := range entries {
		if entries[i].role != "assistant" {
			continue
		}
		switch content := entries[i].content.(type) {
		case ToolUseContent:
			for _, block := range content {
				// If any block has thinking data, we're on a thinking provider
				if block.OfThinking != nil {
					return true
				}
				if block.OfRedactedThinking != nil {
					return true
				}
			}
		}
	}
	return false
}