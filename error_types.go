package main

import (
	"fmt"
	"strings"
	"time"
)

type AgentErrorType int

const (
	ErrorTransient      AgentErrorType = iota
	ErrorContextOverflow
	ErrorToolPairing
	ErrorMaxOutputTokens
	ErrorModelConfusion
	ErrorAuth
	ErrorRateLimit
	ErrorFatal
)

type AgentError struct {
	Type       AgentErrorType
	Message    string
	RetryAfter time.Duration
}

func (e AgentError) Error() string { return e.Message }

func ClassifyError(err error) AgentError {
	if err == nil {
		return AgentError{Type: ErrorFatal, Message: "nil error"}
	}
	msg := err.Error()
	lower := strings.ToLower(msg)

	// Auth errors
	authPatterns := []string{"401", "403", "authentication", "invalid api key", "invalid x-api-key", "permission denied", "unauthorized"}
	for _, p := range authPatterns {
		if strings.Contains(lower, p) {
			return AgentError{Type: ErrorAuth, Message: msg}
		}
	}

	// Rate limit
	rateLimitPatterns := []string{"429", "rate limit", "too many requests"}
	for _, p := range rateLimitPatterns {
		if strings.Contains(lower, p) {
			return AgentError{Type: ErrorRateLimit, Message: msg, RetryAfter: 5 * time.Second}
		}
	}

	// Context overflow
	contextPatterns := []string{"context_length", "maximum context", "too many tokens", "prompt_too_long", "token limit", "context_exceeded", "max_tokens_exceeded", "context window", "context limit"}
	for _, p := range contextPatterns {
		if strings.Contains(lower, p) {
			return AgentError{Type: ErrorContextOverflow, Message: msg}
		}
	}

	// Tool pairing
	if strings.Contains(lower, "2013") || strings.Contains(lower, "tool call result does not follow tool call") {
		return AgentError{Type: ErrorToolPairing, Message: msg}
	}

	// Model confusion
	if strings.Contains(lower, "model confused") {
		return AgentError{Type: ErrorModelConfusion, Message: msg}
	}

	// Max output tokens
	if strings.Contains(lower, "max_output_tokens") || strings.Contains(lower, "output length") {
		return AgentError{Type: ErrorMaxOutputTokens, Message: msg}
	}

	// Transient errors
	transientPatterns := []string{
		"connection refused", "connection reset", "connection timed out",
		"no such host", "temporary failure", "dns",
		"internal server error", "500", "502", "503", "504",
		"service unavailable", "bad gateway", "gateway timeout",
		"timeout", "deadline exceeded",
	}
	for _, p := range transientPatterns {
		if strings.Contains(lower, p) {
			return AgentError{Type: ErrorTransient, Message: msg}
		}
	}

	return AgentError{Type: ErrorFatal, Message: msg}
}

func (t AgentErrorType) String() string {
	switch t {
	case ErrorTransient:
		return "transient"
	case ErrorContextOverflow:
		return "context_overflow"
	case ErrorToolPairing:
		return "tool_pairing"
	case ErrorMaxOutputTokens:
		return "max_output_tokens"
	case ErrorModelConfusion:
		return "model_confusion"
	case ErrorAuth:
		return "auth"
	case ErrorRateLimit:
		return "rate_limit"
	case ErrorFatal:
		return "fatal"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}
