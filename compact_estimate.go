package main

import (
	"math"
	"strings"
)

// ─── Token Estimation (extracted from compact.go) ───────────────────────────

// estimateTokens returns an approximate token count for the given text.
// Uses ceiling division to avoid 0 for short strings like "hi" (2 chars).
// CJK characters count as ~1 token each; non-CJK ~4 chars per token.
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	// Count CJK characters (roughly 1 token per character)
	cjkCount := 0
	for _, r := range text {
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) { // Katakana
			cjkCount++
		}
	}
	// Non-CJK chars: ~4 chars per token (ceiling division to avoid 0 for short strings)
	nonCJK := len(text) - cjkCount
	return ((nonCJK + 3) / 4) + cjkCount
}

// EstimateTokens returns an approximate token count for the given text.
// Kept for backwards compatibility; new code should use estimateTokens().
func EstimateTokens(text string) int {
	return estimateTokens(text)
}

// EstimateContentTokens estimates tokens based on content type.
// Different content types have different chars/token ratios:
//   - Code: 3.5 chars/token (denser, more special tokens)
//   - Natural language: 4 chars/token (default)
//   - JSON/structured: 3 chars/token (lots of delimiters)
//   - tool_use blocks: 3 chars/token + 10 overhead
//   - tool_result blocks: 3 chars/token + 5 overhead
func EstimateContentTokens(text string, contentType string) int {
	charsPerToken := 4.0 // default: natural language
	switch contentType {
	case "code":
		charsPerToken = 3.5
	case "json":
		charsPerToken = 3.0
	case "tool_use":
		charsPerToken = 3.0
	case "tool_result":
		charsPerToken = 3.0
	}
	if text == "" {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / charsPerToken))
}

// DetectContentType heuristically detects content type for token estimation.
func DetectContentType(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		// Likely JSON
		return "json"
	}
	// Check for code indicators
	codeIndicators := []string{"func ", "func(", "var ", "const ", "type ", "struct ", "impl ", "fn ", "class ", "def ", "import ", "package "}
	for _, ind := range codeIndicators {
		if strings.Contains(text, ind) {
			return "code"
		}
	}
	return "natural"
}
