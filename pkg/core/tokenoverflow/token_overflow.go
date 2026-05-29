// Package tokenoverflow provides context overflow resolution strategies.
// Aligned to pi's token-overflow-resolver.ts.
package tokenoverflow

import (
	"fmt"
	"strings"

	"miniclaudecode-go/pkg/core/compaction"
	"miniclaudecode-go/pkg/core/overflow"
)

// OverflowResolutionType represents the type of resolution strategy.
type OverflowResolutionType string

const (
	// ResolutionCompact resolves overflow by compacting the conversation.
	ResolutionCompact OverflowResolutionType = "compact"

	// ResolutionRemoveImage removes images from the conversation.
	ResolutionRemoveImage OverflowResolutionType = "remove_image"

	// ResolutionRemoveOldMessages removes old messages from the conversation.
	ResolutionRemoveOldMessages OverflowResolutionType = "remove_old_messages"

	// ResolutionNoResolution means no resolution is possible.
	ResolutionNoResolution OverflowResolutionType = "none"
)

// OverflowResolutionResult holds the result of an overflow resolution attempt.
type OverflowResolutionResult struct {
	Resolved     bool
	Resolution   OverflowResolutionType
	RemovedTokens int
	Message      string
}

// ResolveOverflow determines the appropriate resolution strategy for a context overflow.
// It tries strategies in order of preference:
//  1. Compact the conversation
//  2. Remove images
//  3. Remove old messages
func ResolveOverflow(contextTokens int, contextWindow int, hasImages bool, compactionSettings compaction.CompactionSettings) OverflowResolutionResult {
	reserveTokens := compactionSettings.ReserveTokens
	if reserveTokens <= 0 {
		reserveTokens = 16384
	}

	overflowTokens := contextTokens - (contextWindow - reserveTokens)
	if overflowTokens <= 0 {
		return OverflowResolutionResult{Resolved: true, Resolution: ResolutionNoResolution}
	}

	// Strategy 1: Compact
	compactSavings := estimateCompactionSavings(contextTokens)
	if compactSavings >= overflowTokens {
		return OverflowResolutionResult{
			Resolved:     true,
			Resolution:   ResolutionCompact,
			RemovedTokens: compactSavings,
			Message:      "Compacting conversation to free context space",
		}
	}

	// Strategy 2: Remove images
	if hasImages {
		imageSavings := estimateImageSavings(contextTokens)
		if imageSavings+compactSavings >= overflowTokens {
			return OverflowResolutionResult{
				Resolved:     true,
				Resolution:   ResolutionRemoveImage,
				RemovedTokens: imageSavings,
				Message:      "Removing images from conversation to free context space",
			}
		}
	}

	// Strategy 3: Remove old messages
	remainingNeeded := overflowTokens - compactSavings
	if hasImages {
		remainingNeeded -= estimateImageSavings(contextTokens)
	}
	if remainingNeeded > 0 {
		return OverflowResolutionResult{
			Resolved:     true,
			Resolution:   ResolutionRemoveOldMessages,
			RemovedTokens: remainingNeeded,
			Message:      fmt.Sprintf("Need to remove ~%d more tokens from old messages", remainingNeeded),
		}
	}

	return OverflowResolutionResult{
		Resolved:   false,
		Resolution: ResolutionNoResolution,
		Message:    "Cannot resolve context overflow. Conversation is too large.",
	}
}

// estimateCompactionSavings estimates how many tokens compaction can save.
func estimateCompactionSavings(contextTokens int) int {
	// Heuristic: compaction typically saves 40-60% of context tokens
	return int(float64(contextTokens) * 0.5)
}

// estimateImageSavings estimates tokens saved by removing images.
func estimateImageSavings(contextTokens int) int {
	// Heuristic: images typically use 10-20% of context when present
	return int(float64(contextTokens) * 0.15)
}

// IsRetryableAfterOverflow checks if the error is an overflow that can be
// resolved by compaction (and thus the API call should be retried after compacting).
func IsRetryableAfterOverflow(errMsg string) bool {
	if !overflow.IsContextOverflow(errMsg) {
		return false
	}

	// All overflow errors are potentially retryable after compaction
	return true
}

// FormatOverflowMessage formats a user-facing message about the overflow.
func FormatOverflowMessage(result OverflowResolutionResult, contextTokens, contextWindow int) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Context window overflow: %d tokens used, %d available.\n", contextTokens, contextWindow))

	switch result.Resolution {
	case ResolutionCompact:
		b.WriteString("Compacting conversation to make room...")
	case ResolutionRemoveImage:
		b.WriteString("Removing images and compacting to make room...")
	case ResolutionRemoveOldMessages:
		b.WriteString("Compacting and trimming old messages to make room...")
	case ResolutionNoResolution:
		b.WriteString("Cannot fit the conversation within the context window. Please start a new session or reduce the conversation length.")
	}

	return b.String()
}
