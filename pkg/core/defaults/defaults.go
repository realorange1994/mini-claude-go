// Package defaults provides default configuration constants.
// Aligned to pi's defaults.ts.
package defaults

import "miniclaudecode-go/pkg/core/agent"

// DefaultThinkingLevel is the default thinking level for the agent.
const DefaultThinkingLevel = agent.ThinkingLevelMedium

// DefaultMaxTurns is the default max turns per message loop.
const DefaultMaxTurns = 100

// DefaultMaxTokens is the default max tokens per LLM response.
const DefaultMaxTokens = 8192

// DefaultCompactAfter is the default number of messages before compaction.
const DefaultCompactAfter = 50

// DefaultModelID is the default model ID when no model is specified.
const DefaultModelID = "claude-sonnet-4-20250514"

// DefaultRetryMaxRetries is the default number of retries for API calls.
const DefaultRetryMaxRetries = 3

// DefaultRetryBaseDelayMs is the default base delay for retry backoff in ms.
const DefaultRetryBaseDelayMs = 1000