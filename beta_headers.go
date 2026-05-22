package main

import (
	"os"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Beta headers — upstream: src/constants/betas.ts, src/utils/betas.ts
// ---------------------------------------------------------------------------

// Memoized beta header cache. Upstream: lodash-es/memoize on getAllModelBetas().
// Cleared on model change, auth change, or compaction.
var (
	betaHeaderCache   map[string][]string // model → memoized beta headers
	betaHeaderCacheMu sync.RWMutex
)

const (
	// BetaHeaderPromptCaching enables prompt caching on the API.
	// Upstream: 'prompt-caching-2024-07-31'
	BetaHeaderPromptCaching = "prompt-caching-2024-07-31"

	// BetaHeaderContext1M enables 1M context window on supported models.
	// Triggered by the [1m] suffix in the model string.
	// Upstream: 'context-1m-2025-08-07'
	BetaHeaderContext1M = "context-1m-2025-08-07"

	// BetaHeaderInterleavedThinking enables interleaved thinking blocks.
	// Upstream: 'interleaved-thinking-2025-05-14'
	BetaHeaderInterleavedThinking = "interleaved-thinking-2025-05-14"

	// BetaHeaderContextManagement enables context management features.
	// Upstream: 'context-management-2025-06-27'
	BetaHeaderContextManagement = "context-management-2025-06-27"

	// BetaHeaderMaxTokens enables extended max tokens output.
	// Upstream: 'max-tokens-3-5-sonnet-2024-07-15'
	BetaHeaderMaxTokens = "max-tokens-3-5-sonnet-2024-07-15"

	// BetaHeaderTokenCounting enables token counting features.
	// Upstream: 'token-counting-2024-11-01'
	BetaHeaderTokenCounting = "token-counting-2024-11-01"

	// BetaHeaderComputerUse enables computer use features.
	// Upstream: 'computer-use-2024-10-22'
	BetaHeaderComputerUse = "computer-use-2024-10-22"

	// BetaHeaderClaudeCode identifies this as a Claude Code client.
	// Upstream: 'claude-code-20250219'
	BetaHeaderClaudeCode = "claude-code-20250219"
)

// BuildBetaHeaders constructs the beta headers list based on model and configuration.
// Results are memoized per model string to ensure cache stability across API calls.
// Upstream: getBetaHeaders() in src/utils/betas.ts, memoized via lodash-es/memoize.
func BuildBetaHeaders(model string) []string {
	betaHeaderCacheMu.RLock()
	if cached, ok := betaHeaderCache[model]; ok {
		betaHeaderCacheMu.RUnlock()
		return append([]string(nil), cached...)
	}
	betaHeaderCacheMu.RUnlock()

	betaHeaderCacheMu.Lock()
	defer betaHeaderCacheMu.Unlock()

	// Double-check after acquiring write lock
	if cached, ok := betaHeaderCache[model]; ok {
		return append([]string(nil), cached...)
	}

	betas := buildBetaHeadersUncached(model)
	if betaHeaderCache == nil {
		betaHeaderCache = make(map[string][]string)
	}
	betaHeaderCache[model] = betas
	return append([]string(nil), betas...)
}

// buildBetaHeadersUncached is the inner computation — same logic as current BuildBetaHeaders.
func buildBetaHeadersUncached(model string) []string {
	var betas []string

	// Always include Claude Code identification
	betas = append(betas, BetaHeaderClaudeCode)

	// Prompt caching
	betas = append(betas, BetaHeaderPromptCaching)

	// [1m] context — triggered by [1m] suffix on model string
	if has1mContext(model) {
		betas = append(betas, BetaHeaderContext1M)
	}

	// Interleaved thinking — enabled by default unless explicitly disabled
	if !isEnvTruthy("CLAUDE_CODE_DISABLE_INTERLEAVED_THINKING") {
		betas = append(betas, BetaHeaderInterleavedThinking)
	}

	// Context management
	betas = append(betas, BetaHeaderContextManagement)

	// Max tokens (for extended output)
	betas = append(betas, BetaHeaderMaxTokens)

	// Token counting
	if !isEnvTruthy("CLAUDE_CODE_DISABLE_TOKEN_COUNTING") {
		betas = append(betas, BetaHeaderTokenCounting)
	}

	return betas
}

// ClearBetaHeaderCache clears the memoized beta header cache.
// Call on model change, auth change, or compaction.
func ClearBetaHeaderCache() {
	betaHeaderCacheMu.Lock()
	defer betaHeaderCacheMu.Unlock()
	betaHeaderCache = make(map[string][]string)
}

// FormatBetaHeader formats the beta headers as a comma-separated string
// for the anthropic-beta HTTP header.
func FormatBetaHeader(betas []string) string {
	return strings.Join(betas, ",")
}

// GetModelForAPI strips the [1m] suffix from the model string for API calls.
// The API doesn't accept [1m] in the model field — it's only used client-side
// to determine context window and beta headers.
// Preserves original case for direct model names (not aliases) so proxies
// that are case-sensitive still work.
// Upstream: model is stripped of [1m] before being sent to the API
func GetModelForAPI(model string) string {
	clean := strings.TrimSpace(strings.Replace(model, "[1m]", "", 1))
	// Only lowercase known aliases; keep direct model names (like "M2.7") as-is
	lower := strings.ToLower(clean)
	if _, ok := modelAliasFamily[lower]; ok {
		return lower
	}
	return clean
}

// isEnvTruthy checks if an environment variable is set to a truthy value.
// Matches upstream's isEnvTruthy() in envUtils.ts:32-36.
func isEnvTruthy(key string) bool {
	v := strings.ToLower(os.Getenv(key))
	v = strings.TrimSpace(v)
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
