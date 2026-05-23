package main

// getTokenCountFromUsage calculates total context window tokens from usage data.
// Includes input_tokens + cache tokens + output_tokens.
// Ported from upstream tokens.ts getTokenCountFromUsage.
func getTokenCountFromUsage(usage *UsageInfo) int {
	if usage == nil {
		return 0
	}
	return usage.InputTokens + usage.CacheCreationInputTokens +
		usage.CacheReadInputTokens + usage.OutputTokens
}

// UsageInfo represents token usage data from an API response.
type UsageInfo struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

// tokenCountFromLastAPIResponse returns the total token count from the last
// assistant message with valid usage data. Returns 0 if no such message exists.
// Ported from upstream tokens.ts tokenCountFromLastAPIResponse.
func tokenCountFromLastAPIResponse(messages []Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		usage := getTokenUsage(messages[i])
		if usage != nil {
			return getTokenCountFromUsage(usage)
		}
	}
	return 0
}

// messageTokenCountFromLastAPIResponse returns only the output_tokens from
// the last assistant message with usage data.
// Ported from upstream tokens.ts messageTokenCountFromLastAPIResponse.
func messageTokenCountFromLastAPIResponse(messages []Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		usage := getTokenUsage(messages[i])
		if usage != nil {
			return usage.OutputTokens
		}
	}
	return 0
}

// getCurrentUsage returns the usage object from the last assistant message
// with valid (non-zero) usage data, or nil if none found.
// Ported from upstream tokens.ts getCurrentUsage.
func getCurrentUsage(messages []Message) *UsageInfo {
	for i := len(messages) - 1; i >= 0; i-- {
		usage := getTokenUsage(messages[i])
		if usage != nil {
			// Skip placeholder usage (all zeros) — third-party APIs may emit
			// message_start without real usage data
			inputTotal := usage.InputTokens + usage.CacheCreationInputTokens +
				usage.CacheReadInputTokens
			if inputTotal == 0 && usage.OutputTokens == 0 {
				continue
			}
			return &UsageInfo{
				InputTokens:              usage.InputTokens,
				OutputTokens:             usage.OutputTokens,
				CacheCreationInputTokens: usage.CacheCreationInputTokens,
				CacheReadInputTokens:     usage.CacheReadInputTokens,
			}
		}
	}
	return nil
}

// doesMostRecentAssistantMessageExceed200k returns true if the most recent
// assistant message's total token count exceeds 200,000.
// Ported from upstream tokens.ts doesMostRecentAssistantMessageExceed200k.
func doesMostRecentAssistantMessageExceed200k(messages []Message) bool {
	const threshold = 200000
	// Find last assistant message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Type == "assistant" {
			usage := getTokenUsage(messages[i])
			if usage != nil {
				return getTokenCountFromUsage(usage) > threshold
			}
			return false
		}
	}
	return false
}

// getTokenUsage extracts usage data from a message, returning nil for
// non-assistant messages or messages with synthetic model.
// Ported from upstream tokens.ts getTokenUsage.
func getTokenUsage(msg Message) *UsageInfo {
	if msg.Type != "assistant" {
		return nil
	}
	if msg.Model == "<synthetic>" {
		return nil
	}
	if msg.Usage == nil {
		return nil
	}
	return msg.Usage
}

// Message represents a conversation message for token counting.
type Message struct {
	Type  string     // "assistant" or "user"
	Model string     // model name, "<synthetic>" for synthetic messages
	Usage *UsageInfo // token usage data (nil for user messages)
}
