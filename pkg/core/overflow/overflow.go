// Package overflow detects context window overflow errors from LLM API responses.
// Aligned to pi's overflow.ts.
package overflow

import (
	"regexp"
	"strings"
)

// Overflow patterns match error messages from various LLM providers.
var overflowPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)prompt is too long`),
	regexp.MustCompile(`(?i)request_too_large`),
	regexp.MustCompile(`(?i)input is too long for requested model`),
	regexp.MustCompile(`(?i)exceeds the context window`),
	regexp.MustCompile(`(?i)exceeds (?:the )?(?:model'?s )?maximum context length of [\d,]+ tokens?`),
	regexp.MustCompile(`(?i)input token count.*exceeds the maximum`),
	regexp.MustCompile(`(?i)maximum prompt length is \d+`),
	regexp.MustCompile(`(?i)reduce the length of the messages`),
	regexp.MustCompile(`(?i)maximum context length is \d+ tokens`),
	regexp.MustCompile(`(?i)exceeds (?:the )?maximum allowed input length of [\d,]+ tokens?`),
	regexp.MustCompile(`(?i)input \(\d+ tokens\) is longer than the model'?s context length \(\d+ tokens\)`),
	regexp.MustCompile(`(?i)exceeds the limit of \d+`),
	regexp.MustCompile(`(?i)exceeds the available context size`),
	regexp.MustCompile(`(?i)greater than the context length`),
	regexp.MustCompile(`(?i)context window exceeds limit`),
	regexp.MustCompile(`(?i)exceeded model token limit`),
	regexp.MustCompile(`(?i)too large for model with \d+ maximum context length`),
	regexp.MustCompile(`(?i)model_context_window_exceeded`),
	regexp.MustCompile(`(?i)prompt too long; exceeded (?:max )?context length`),
	regexp.MustCompile(`(?i)context[_ ]length[_ ]exceeded`),
	regexp.MustCompile(`(?i)too many tokens`),
	regexp.MustCompile(`(?i)token limit exceeded`),
	regexp.MustCompile(`(?i)^4(?:00|13)\s*(?:status code)?\s*\(no body\)`),
}

// Non-overflow patterns match errors that look like overflow but aren't
// (e.g., throttling, rate limiting).
var nonOverflowPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(Throttling error|Service unavailable):`),
	regexp.MustCompile(`(?i)rate limit`),
	regexp.MustCompile(`(?i)too many requests`),
}

// IsContextOverflow checks if an error message indicates context window overflow.
// Returns true if the message matches an overflow pattern and does NOT match
// any non-overflow pattern.
func IsContextOverflow(errorMessage string) bool {
	if errorMessage == "" {
		return false
	}

	// Check non-overflow patterns first
	for _, p := range nonOverflowPatterns {
		if p.MatchString(errorMessage) {
			return false
		}
	}

	// Check overflow patterns
	for _, p := range overflowPatterns {
		if p.MatchString(errorMessage) {
			return true
		}
	}

	return false
}

// IsContextOverflowWithUsage checks if context overflow occurred based on
// stop reason and actual token usage vs the context window.
// This handles "silent" overflow cases where the provider doesn't return an error
// but truncates the response.
func IsContextOverflowWithUsage(stopReason string, inputTokens, cacheRead, contextWindow int) bool {
	if contextWindow <= 0 {
		return false
	}

	totalInput := inputTokens + cacheRead

	switch strings.ToLower(stopReason) {
	case "error":
		// Handled by IsContextOverflow
		return false
	case "length":
		// Length stop with zero output typically means input was too large
		return totalInput >= int(float64(contextWindow)*0.99)
	case "stop":
		// Some providers (z.ai) silently truncate
		return totalInput > contextWindow
	}

	return false
}

// GetOverflowPatterns returns a copy of the overflow patterns for testing.
func GetOverflowPatterns() []*regexp.Regexp {
	result := make([]*regexp.Regexp, len(overflowPatterns))
	copy(result, overflowPatterns)
	return result
}