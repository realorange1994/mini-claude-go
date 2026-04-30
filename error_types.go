package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrorClass categorizes API errors for retry decision-making.
// Expanded 15-category taxonomy matching Hermes-agent error_classifier.py.
type ErrorClass int

const (
	ECRetryable       ErrorClass = iota // generic retryable (network, 5xx)
	ECNonRetryable                      // generic non-retryable (4xx)
	ECContextOverflow                   // context too large -- compress
	ECToolPairing                       // 2013 tool pairing broken
	ECRateLimit                         // 429 -- backoff + retry
	ECBilling                           // 402/credit exhausted -- rotate key or fallback
	ECModelNotFound                     // model doesn't exist -- fallback
	ECPayloadTooLarge                   // 413 -- compress prompt
	ECOverloaded                        // 503/529 -- provider overloaded
	ECTimeout                           // connection/read timeout -- retry
	ECFormatError                       // 400 bad request -- abort or fix
	ECAuth                              // 401/403 -- rotate credential
	ECThinkingSig                       // thinking block signature invalid
	ECLongContextTier                   // 429 + "extra usage" + "long context"
	ECUnknown                           // unclassifiable -- retry with backoff
)

// ClassifyResult is the output of classifyError with recovery hints.
type ClassifyResult struct {
	Class       ErrorClass
	Retryable   bool
	Compress    bool  // should_compress: compress context before retry
	RotateKey   bool  // should_rotate_credential: try a different API key
	Fallback    bool  // should_fallback: switch to a different provider/model
	Message     string
	RetryAfter  time.Duration // for rate limits with Retry-After header
	StatusCode  int
}

func (c ErrorClass) String() string {
	names := []string{
		"retryable", "non_retryable", "context_overflow", "tool_pairing",
		"rate_limit", "billing", "model_not_found", "payload_too_large",
		"overloaded", "timeout", "format_error", "auth", "thinking_signature",
		"long_context_tier", "unknown",
	}
	if int(c) < len(names) {
		return names[c]
	}
	return fmt.Sprintf("error_class(%d)", c)
}

// Error patterns matching Hermes-agent error_classifier.py
var (
	billingPatterns = []string{
		"insufficient credits", "insufficient_quota", "credit balance",
		"credits have been exhausted", "top up your credits",
		"payment required", "billing hard limit",
		"exceeded your current quota", "account is deactivated",
		"plan does not include",
	}

	rateLimitPatterns = []string{
		"rate limit", "rate_limit", "too many requests", "throttled",
		"requests per minute", "tokens per minute", "requests per day",
		"try again in", "please retry after", "resource_exhausted",
		"rate increased too quickly",
		"throttlingexception", "too many concurrent requests",
		"servicequotaexceededexception",
	}

	usageLimitPatterns = []string{
		"usage limit", "quota", "limit exceeded", "key limit exceeded",
	}

	usageLimitTransientSignals = []string{
		"try again", "retry", "resets at", "reset in", "wait",
		"requests remaining", "periodic", "window",
	}

	contextOverflowPatterns = []string{
		"context_length", "maximum context", "too many tokens",
		"prompt_too_long", "token limit", "context_exceeded",
		"max_tokens_exceeded", "context window", "context limit",
		"prompt exceeds max length", "prompt is too long",
		"exceeds the limit", "reduce the length", "context size",
		"exceeds the max_model_len", "max_model_len",
		"engine prompt length", "input is too long",
		"maximum model length", "context length exceeded",
		"truncating input", "slot context", "n_ctx_slot",
		"超过最大长度", "上下文长度", "max input token",
		"input token", "exceeds the maximum number of input tokens",
	}

	modelNotFoundPatterns = []string{
		"is not a valid model", "invalid model", "model not found",
		"model_not_found", "does not exist", "no such model",
		"unknown model", "unsupported model",
	}

	authPatterns = []string{
		"invalid api key", "invalid_api_key", "authentication",
		"unauthorized", "forbidden", "invalid token", "token expired",
		"token revoked", "access denied",
	}

	serverDisconnectPatterns = []string{
		"server disconnected", "peer closed connection",
		"connection reset by peer", "connection was closed",
		"network connection lost", "unexpected eof",
		"incomplete chunked read",
	}

	networkErrorPatterns = []string{
		"connection refused", "connection reset", "connection timed out",
		"connection error", "connection lost", "no such host",
		"temporary failure", "dns error", "network error",
		"network is unreachable", "host unreachable",
		"socket error", "tcp error",
	}

	serverErrorPatterns = []string{
		"internal server error", "bad gateway",
		"service unavailable", "gateway timeout",
	}

	transportErrorTypes = []string{
		"readtimeout", "connecttimeout", "pooltimeout",
		"connecterror", "remoteprotocolerror",
		"connectionerror", "connectionreseterror",
		"connectionabortederror", "brokenpipeerror",
		"timeouterror", "readerror",
		"serverdisconnectederror",
	}

	// status code regex for extracting HTTP status from error messages
	statusCodeRegex = regexp.MustCompile(`\b(\d{3})\b`)
)

// classifyError categorizes an error into a ClassifyResult with recovery hints.
// Priority-ordered pipeline matching Hermes-agent:
//  1. Status code classification
//  2. Error code classification (from body)
//  3. Message pattern matching
//  4. Server disconnect + large session heuristic
//  5. Transport error heuristics
//  6. Fallback: unknown
func classifyError(errMsg string, approxTokens int, contextLength int) ClassifyResult {
	lower := strings.ToLower(errMsg)
	statusCode := extractStatusCode(errMsg)

	result := func(class ErrorClass, opts ...func(*ClassifyResult)) ClassifyResult {
		r := ClassifyResult{
			Class:      class,
			Retryable:  true,
			StatusCode: statusCode,
			Message:    truncateStr(errMsg, 500),
		}
		for _, o := range opts {
			o(&r)
		}
		return r
	}

	notRetryable := func(class ErrorClass, opts ...func(*ClassifyResult)) ClassifyResult {
		opts = append([]func(*ClassifyResult){func(r *ClassifyResult) { r.Retryable = false }}, opts...)
		return result(class, opts...)
	}

	// ── Status code classification ──────────────────────────────────

	if statusCode == 401 {
		return notRetryable(ECAuth, func(r *ClassifyResult) {
			r.RotateKey = true
			r.Fallback = true
		})
	}

	if statusCode == 403 {
		// OpenRouter 403 "key limit exceeded" is actually billing
		if strings.Contains(lower, "key limit exceeded") || strings.Contains(lower, "spending limit") {
			return notRetryable(ECBilling, func(r *ClassifyResult) {
				r.RotateKey = true
				r.Fallback = true
			})
		}
		return notRetryable(ECAuth, func(r *ClassifyResult) { r.Fallback = true })
	}

	if statusCode == 402 {
		return classify402(lower, result, notRetryable)
	}

	if statusCode == 404 {
		if matchesAny(lower, modelNotFoundPatterns) {
			return notRetryable(ECModelNotFound, func(r *ClassifyResult) { r.Fallback = true })
		}
		// Generic 404 -- unknown (don't assume model not found)
		return result(ECUnknown)
	}

	if statusCode == 413 {
		return result(ECPayloadTooLarge, func(r *ClassifyResult) { r.Compress = true })
	}

	if statusCode == 429 {
		return result(ECRateLimit, func(r *ClassifyResult) {
			r.RotateKey = true
			r.Fallback = true
		})
	}

	if statusCode == 400 {
		return classify400(lower, approxTokens, contextLength, result, notRetryable)
	}

	if statusCode == 500 || statusCode == 502 {
		return result(ECRetryable)
	}

	if statusCode == 503 || statusCode == 529 {
		return result(ECOverloaded)
	}

	if statusCode >= 400 && statusCode < 500 {
		return notRetryable(ECFormatError, func(r *ClassifyResult) { r.Fallback = true })
	}

	if statusCode >= 500 && statusCode < 600 {
		return result(ECRetryable)
	}

	// ── Message pattern matching (no status code) ───────────────────

	// Context overflow -- not retryable without compression
	if matchesAny(lower, contextOverflowPatterns) {
		return notRetryable(ECContextOverflow, func(r *ClassifyResult) { r.Compress = true })
	}

	// Tool pairing -- not retryable without fixing context
	if strings.Contains(lower, "2013") || strings.Contains(lower, "tool call result does not follow tool call") {
		return notRetryable(ECToolPairing)
	}

	// Billing patterns
	if matchesAny(lower, billingPatterns) {
		return notRetryable(ECBilling, func(r *ClassifyResult) {
			r.RotateKey = true
			r.Fallback = true
		})
	}

	// Rate limit (check before usageLimitPatterns to avoid misclassification)
	if matchesAny(lower, rateLimitPatterns) {
		return result(ECRateLimit, func(r *ClassifyResult) {
			r.RotateKey = true
			r.Fallback = true
		})
	}

	// Usage limit with disambiguation (only if not already classified as rate limit)
	if matchesAny(lower, usageLimitPatterns) {
		if matchesAny(lower, usageLimitTransientSignals) {
			return result(ECRateLimit, func(r *ClassifyResult) {
				r.RotateKey = true
				r.Fallback = true
			})
		}
		return notRetryable(ECBilling, func(r *ClassifyResult) {
			r.RotateKey = true
			r.Fallback = true
		})
	}

	// Model not found
	if matchesAny(lower, modelNotFoundPatterns) {
		return notRetryable(ECModelNotFound, func(r *ClassifyResult) { r.Fallback = true })
	}

	// Auth patterns
	if matchesAny(lower, authPatterns) {
		return notRetryable(ECAuth, func(r *ClassifyResult) {
			r.RotateKey = true
			r.Fallback = true
		})
	}

	// Server disconnect + large session → context overflow heuristic
	if matchesAny(lower, serverDisconnectPatterns) && statusCode == 0 {
		isLarge := approxTokens > contextLength*6/10 || approxTokens > 120000
		if isLarge {
			return notRetryable(ECContextOverflow, func(r *ClassifyResult) { r.Compress = true })
		}
		return result(ECTimeout)
	}

	// Network errors -- retryable (connection refused, DNS, etc.)
	if matchesAny(lower, networkErrorPatterns) {
		return result(ECRetryable)
	}

	// Server errors without status code -- retryable
	if matchesAny(lower, serverErrorPatterns) {
		return result(ECRetryable)
	}

	// Transport error heuristics
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded") {
		return result(ECTimeout)
	}
	if matchesAny(lower, transportErrorTypes) {
		return result(ECTimeout)
	}

	// ── Fallback: unknown -- not retryable by default ──────────────────
	return notRetryable(ECUnknown)
}

// classify402 disambiguates 402: billing exhaustion vs transient usage limit.
func classify402(lower string, resultFn func(ErrorClass, ...func(*ClassifyResult)) ClassifyResult, notRetryable func(ErrorClass, ...func(*ClassifyResult)) ClassifyResult) ClassifyResult {
	if matchesAny(lower, usageLimitPatterns) && matchesAny(lower, usageLimitTransientSignals) {
		return resultFn(ECRateLimit, func(r *ClassifyResult) {
			r.RotateKey = true
			r.Fallback = true
		})
	}
	return notRetryable(ECBilling, func(r *ClassifyResult) {
		r.RotateKey = true
		r.Fallback = true
	})
}

// classify400 classifies 400 Bad Request -- context overflow, model not found, rate limit, billing, or format error.
func classify400(lower string, approxTokens int, contextLength int, resultFn func(ErrorClass, ...func(*ClassifyResult)) ClassifyResult, notRetryable func(ErrorClass, ...func(*ClassifyResult)) ClassifyResult) ClassifyResult {
	// Context overflow from 400 -- not retryable without compression
	if matchesAny(lower, contextOverflowPatterns) {
		return notRetryable(ECContextOverflow, func(r *ClassifyResult) { r.Compress = true })
	}

	// Model not found as 400
	if matchesAny(lower, modelNotFoundPatterns) {
		return notRetryable(ECModelNotFound, func(r *ClassifyResult) { r.Fallback = true })
	}

	// Rate limit / billing as 400
	if matchesAny(lower, rateLimitPatterns) {
		return resultFn(ECRateLimit, func(r *ClassifyResult) {
			r.RotateKey = true
			r.Fallback = true
		})
	}
	if matchesAny(lower, billingPatterns) {
		return notRetryable(ECBilling, func(r *ClassifyResult) {
			r.RotateKey = true
			r.Fallback = true
		})
	}

	// Generic 400 + large session → probable context overflow
	isLarge := approxTokens > contextLength*4/10 || approxTokens > 80000
	if isLarge {
		return resultFn(ECContextOverflow, func(r *ClassifyResult) { r.Compress = true })
	}

	return notRetryable(ECFormatError, func(r *ClassifyResult) { r.Fallback = true })
}

// extractStatusCode attempts to extract an HTTP status code from the error message.
// Looks for patterns like " 401 ", "status_code=401", "401 Unauthorized", etc.
func extractStatusCode(errMsg string) int {
	matches := statusCodeRegex.FindAllStringSubmatch(errMsg, -1)
	for _, m := range matches {
		if len(m) > 1 {
			var code int
			fmt.Sscanf(m[1], "%d", &code)
			if code >= 100 && code < 600 {
				return code
			}
		}
	}
	return 0
}

// matchesAny checks if text contains any of the patterns.
func matchesAny(text string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}

// truncateStr truncates a string to maxLen bytes, respecting UTF-8 boundaries.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	for maxLen > 0 && !utf8.RuneStart(s[maxLen]) {
		maxLen--
	}
	return s[:maxLen]
}

// parseErrorBody attempts to extract structured error info from the error message.
func parseErrorBody(errMsg string) map[string]any {
	// Try to find JSON object in error message
	start := strings.Index(errMsg, "{")
	if start == -1 {
		return nil
	}
	// Find matching closing brace
	depth := 0
	end := -1
	for i := start; i < len(errMsg); i++ {
		switch errMsg[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if end == -1 {
		return nil
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(errMsg[start:end]), &body); err == nil {
		return body
	}
	return nil
}

// ---- Backward compatibility wrappers ----

// isTransientError returns true for errors that may resolve on retry.
func isTransientError(errMsg string) bool {
	r := classifyError(errMsg, 0, 0)
	return r.Retryable
}

// isContextLengthError checks if the error is a context window overflow.
func isContextLengthError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	return matchesAny(lower, contextOverflowPatterns)
}

// isErrorNonRetryable returns true for errors that should not be retried.
func isErrorNonRetryable(errMsg string) bool {
	return !classifyError(errMsg, 0, 0).Retryable
}
