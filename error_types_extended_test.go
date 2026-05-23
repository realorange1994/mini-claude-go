package main

import (
	"errors"
	"testing"
)

// ---------------------------------------------------------------------------
// Extended error classification tests — covering previously untested functions
// ---------------------------------------------------------------------------

func TestShouldRetry429(t *testing.T) {
	tests := []struct {
		subscription string
		overage      bool
		expected     bool
	}{
		{"claude_ai", false, false},
		{"claude_ai", true, false},
		{"enterprise", false, true},
		{"api", false, true},
		{"unknown", false, true},
		{"", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.subscription, func(t *testing.T) {
			result := shouldRetry429(tt.subscription, tt.overage)
			if result != tt.expected {
				t.Errorf("shouldRetry429(%q, %v) = %v, expected %v", tt.subscription, tt.overage, result, tt.expected)
			}
		})
	}
}

func TestIs529Error(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"HTTP 529 Overloaded", true},
		{"error 529 service overloaded", true},
		{"529 Overloaded", true},
		{"500 Internal Server Error", false},
		{"429 Too Many Requests", false},
		{"random error", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := is529Error(tt.input)
			if result != tt.expected {
				t.Errorf("is529Error(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestContainsOverageSignal(t *testing.T) {
	if !containsOverageSignal("x-anthropic-overage: true") {
		t.Error("should detect overage signal in header")
	}
	if !containsOverageSignal("overage true") {
		t.Error("should detect 'overage true' pattern")
	}
	if containsOverageSignal("random error message") {
		t.Error("should not detect overage in random message")
	}
	if containsOverageSignal("overage is false") {
		t.Error("should not detect overage when not 'true'")
	}
}

func TestFallbackTriggeredErrorStruct(t *testing.T) {
	err := &FallbackTriggeredError{
		OriginalModel:  "claude-opus-4-20250514",
		FallbackModel:  "claude-sonnet-4-20250514",
		Consecutive529: 3,
	}
	msg := err.Error()
	if msg == "" {
		t.Error("FallbackTriggeredError.Error() should return non-empty string")
	}
	if err.OriginalModel != "claude-opus-4-20250514" {
		t.Errorf("expected OriginalModel 'claude-opus-4-20250514', got %q", err.OriginalModel)
	}
	if err.Consecutive529 != 3 {
		t.Errorf("expected Consecutive529=3, got %d", err.Consecutive529)
	}
}

func TestParseMaxTokensExtended(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		overflow int
		found    bool
	}{
		{
			name:     "standard pattern",
			errMsg:   "prompt is too long: 137500 tokens > 135000 maximum",
			overflow: 2500,
			found:    true,
		},
		{
			name:     "request too large pattern",
			errMsg:   "request too large: 140000 tokens, max 135000",
			overflow: 5000,
			found:    true,
		},
		{
			name:     "nil error",
			errMsg:   "",
			overflow: 0,
			found:    false,
		},
		{
			name:     "no token numbers",
			errMsg:   "context length exceeded",
			overflow: 0,
			found:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.errMsg != "" {
				err = errors.New(tt.errMsg)
			}
			overflow, found := parseMaxTokensContextOverflowError(err)
			if overflow != tt.overflow || found != tt.found {
				t.Errorf("parseMaxTokensContextOverflowError() = (%d, %v), expected (%d, %v)",
					overflow, found, tt.overflow, tt.found)
			}
		})
	}
}

func TestClassifyError402TransientExtended(t *testing.T) {
	r := classifyError("402 usage limit exceeded, try again later", 0, 0)
	if r.Class != ECRateLimit {
		t.Errorf("expected ECRateLimit for transient 402, got %v", r.Class)
	}
	if !r.Retryable {
		t.Error("transient 402 should be retryable")
	}
}

func TestClassifyError404GenericExtended(t *testing.T) {
	r := classifyError("404 Not Found: some random resource", 0, 0)
	if r.Class != ECUnknown {
		t.Errorf("expected ECUnknown for generic 404, got %v", r.Class)
	}
}

func TestClassifyErrorBillingPatternsExtended(t *testing.T) {
	patterns := []string{
		"insufficient credits",
		"insufficient_quota",
		"credit balance is too low",
		"credits have been exhausted",
		"top up your credits",
		"payment required",
		"billing hard limit",
		"exceeded your current quota",
		"account is deactivated",
		"plan does not include",
	}
	for _, p := range patterns {
		r := classifyError(p, 0, 0)
		if r.Class != ECBilling {
			t.Errorf("pattern %q should be ECBilling, got %v", p, r.Class)
		}
	}
}

func TestClassifyErrorRateLimitPatternsExtended(t *testing.T) {
	patterns := []string{
		"rate limit exceeded",
		"rate_limit_error",
		"too many requests",
		"throttled",
		"requests per minute exceeded",
		"tokens per minute exceeded",
		"try again in 60 seconds",
		"please retry after 30s",
		"resource_exhausted",
	}
	for _, p := range patterns {
		r := classifyError(p, 0, 0)
		if r.Class != ECRateLimit {
			t.Errorf("pattern %q should be ECRateLimit, got %v", p, r.Class)
		}
		if !r.Retryable {
			t.Errorf("pattern %q should be retryable", p)
		}
	}
}

func TestClassifyErrorContextOverflowPatternsExtended(t *testing.T) {
	patterns := []string{
		"context_length exceeded",
		"maximum context window exceeded",
		"too many tokens in prompt",
		"prompt_too_long",
		"token limit exceeded",
		"context_exceeded",
		"max_tokens_exceeded",
		"context window too large",
		"prompt exceeds max length",
		"exceeds the limit",
		"reduce the length",
		"context size exceeded",
	}
	for _, p := range patterns {
		r := classifyError(p, 0, 0)
		if r.Class != ECContextOverflow {
			t.Errorf("pattern %q should be ECContextOverflow, got %v", p, r.Class)
		}
		if !r.Compress {
			t.Errorf("pattern %q should trigger compression", p)
		}
	}
}

func TestClassifyErrorChineseContextOverflowExtended(t *testing.T) {
	patterns := []string{
		"超过最大长度137500，最大135000",
		"上下文长度超出限制",
	}
	for _, p := range patterns {
		r := classifyError(p, 0, 0)
		if r.Class != ECContextOverflow {
			t.Errorf("Chinese pattern %q should be ECContextOverflow, got %v", p, r.Class)
		}
	}
}

func TestClassifyErrorModelNotFoundPatternsExtended(t *testing.T) {
	patterns := []string{
		"is not a valid model",
		"invalid model identifier",
		"model not found",
		"model_not_found",
		"does not exist as a model",
		"no such model available",
		"unknown model specified",
		"unsupported model requested",
	}
	for _, p := range patterns {
		r := classifyError(p, 0, 0)
		if r.Class != ECModelNotFound {
			t.Errorf("pattern %q should be ECModelNotFound, got %v", p, r.Class)
		}
		if r.Retryable {
			t.Errorf("pattern %q should NOT be retryable", p)
		}
		if !r.Fallback {
			t.Errorf("pattern %q should trigger fallback", p)
		}
	}
}

func TestClassifyErrorAuthPatternsExtended(t *testing.T) {
	patterns := []string{
		"invalid api key provided",
		"invalid_api_key",
		"authentication failed",
		"unauthorized access",
		"forbidden: access denied",
		"invalid token",
		"token expired",
		"token revoked",
		"access denied",
	}
	for _, p := range patterns {
		r := classifyError(p, 0, 0)
		if r.Class != ECAuth {
			t.Errorf("pattern %q should be ECAuth, got %v", p, r.Class)
		}
		if !r.RotateKey {
			t.Errorf("pattern %q should trigger key rotation", p)
		}
	}
}
