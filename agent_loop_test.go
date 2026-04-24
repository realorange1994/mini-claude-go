package main

import (
	"testing"
)

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		err   string
		trans bool
	}{
		// Transient errors
		{"connection refused", true},
		{"Connection reset", true},
		{"connection timed out", true},
		{"no such host", true},
		{"temporary failure", true},
		{"dns error", true},
		{"Internal server error", true},
		{"500 internal server error", true},
		{"502 bad gateway", true},
		{"503 service unavailable", true},
		{"504 gateway timeout", true},
		{"rate limit exceeded", true},
		{"429 too many requests", true},
		{"request timeout", true},
		{"deadline exceeded", true},
		// Non-transient errors
		{"model confused", false},
		{"stream stalled", false},
		{"context_length exceeded", false},
		{"authentication failed", false},
		{"invalid request", false},
		{"unauthorized", false},
		{"not found", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			got := isTransientError(tt.err)
			if got != tt.trans {
				t.Errorf("isTransientError(%q) = %v, want %v", tt.err, got, tt.trans)
			}
		})
	}
}

func TestIsContextLengthError(t *testing.T) {
	tests := []struct {
		err   string
		isCtx bool
	}{
		{"context_length exceeded", true},
		{"maximum context", true},
		{"too many tokens", true},
		{"prompt_too_long", true},
		{"token limit", true},
		{"context_exceeded", true},
		{"max_tokens_exceeded", true},
		{"context window", true},
		{"context limit", true},
		{"normal error", false},
		{"", false},
		{"model confused", false},
		{"stream stalled", false},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			got := isContextLengthError(tt.err)
			if got != tt.isCtx {
				t.Errorf("isContextLengthError(%q) = %v, want %v", tt.err, got, tt.isCtx)
			}
		})
	}
}
