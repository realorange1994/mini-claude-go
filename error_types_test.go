package main

import (
	"testing"
	"time"
)

func TestClassifyErrorTransient(t *testing.T) {
	tests := []struct {
		err   string
		trans bool
	}{
		// Network errors — retryable
		{"connection refused", true},
		{"Connection reset", true},
		{"connection timed out", true},
		{"no such host", true},
		{"temporary failure", true},
		{"dns error", true},
		// Server errors — retryable
		{"Internal server error", true},
		{"500 internal server error", true},
		{"502 bad gateway", true},
		{"503 service unavailable", true},
		{"504 gateway timeout", true},
		// Rate limit — retryable
		{"rate limit exceeded", true},
		{"429 too many requests", true},
		// Timeout — retryable
		{"request timeout", true},
		{"deadline exceeded", true},
		// Non-retryable errors
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
			r := classifyError(tt.err, 0, 0)
			if r.Retryable != tt.trans {
				t.Errorf("classifyError(%q).Retryable = %v, want %v (class=%s)", tt.err, r.Retryable, tt.trans, r.Class)
			}
		})
	}
}

func TestClassifyErrorCategories(t *testing.T) {
	tests := []struct {
		err    string
		class  ErrorClass
		retry  bool
		compress bool
	}{
		// Context overflow
		{"context_length exceeded", ECContextOverflow, false, true},
		{"too many tokens", ECContextOverflow, false, true},
		// Rate limit
		{"rate limit exceeded", ECRateLimit, true, false},
		{"429 too many requests", ECRateLimit, true, false},
		// Auth
		{"authentication failed", ECAuth, false, false},
		{"unauthorized", ECAuth, false, false},
		{"invalid api key", ECAuth, false, false},
		// Billing
		{"insufficient credits", ECBilling, false, false},
		// Model not found
		{"model not found", ECModelNotFound, false, false},
		// Tool pairing
		{"2013 tool call result does not follow tool call", ECToolPairing, false, false},
		// Timeout
		{"request timeout", ECTimeout, true, false},
		{"deadline exceeded", ECTimeout, true, false},
		// Network
		{"connection refused", ECRetryable, true, false},
		// Server errors
		{"500 internal server error", ECRetryable, true, false},
		{"503 service unavailable", ECOverloaded, true, false},
		// Format error
		{"400 bad request", ECFormatError, false, false},
		// Unknown
		{"model confused", ECUnknown, false, false},
		{"", ECUnknown, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			r := classifyError(tt.err, 0, 0)
			if r.Class != tt.class {
				t.Errorf("classifyError(%q).Class = %v, want %v", tt.err, r.Class, tt.class)
			}
			if r.Retryable != tt.retry {
				t.Errorf("classifyError(%q).Retryable = %v, want %v", tt.err, r.Retryable, tt.retry)
			}
			if r.Compress != tt.compress {
				t.Errorf("classifyError(%q).Compress = %v, want %v", tt.err, r.Compress, tt.compress)
			}
		})
	}
}

func TestClassifyErrorStatusCodeExtraction(t *testing.T) {
	tests := []struct {
		err        string
		statusCode int
	}{
		{"500 internal server error", 500},
		{"429 too many requests", 429},
		{"401 unauthorized", 401},
		{"403 forbidden", 403},
		{"connection refused", 0},
		{"rate limit exceeded", 0},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			r := classifyError(tt.err, 0, 0)
			if r.StatusCode != tt.statusCode {
				t.Errorf("classifyError(%q).StatusCode = %v, want %v", tt.err, r.StatusCode, tt.statusCode)
			}
		})
	}
}

func TestClassifyErrorRecoveryHints(t *testing.T) {
	tests := []struct {
		err        string
		rotateKey  bool
		fallback   bool
		compress   bool
	}{
		{"401 unauthorized", true, true, false},
		{"rate limit exceeded", true, true, false},
		{"insufficient credits", true, true, false},
		{"model not found", false, true, false},
		{"context_length exceeded", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			r := classifyError(tt.err, 0, 0)
			if r.RotateKey != tt.rotateKey {
				t.Errorf("classifyError(%q).RotateKey = %v, want %v", tt.err, r.RotateKey, tt.rotateKey)
			}
			if r.Fallback != tt.fallback {
				t.Errorf("classifyError(%q).Fallback = %v, want %v", tt.err, r.Fallback, tt.fallback)
			}
			if r.Compress != tt.compress {
				t.Errorf("classifyError(%q).Compress = %v, want %v", tt.err, r.Compress, tt.compress)
			}
		})
	}
}

func TestClassifyErrorLargeSessionHeuristic(t *testing.T) {
	// Server disconnect + large session → context overflow
	r := classifyError("connection reset by peer", 150000, 200000)
	if r.Class != ECContextOverflow {
		t.Errorf("large session disconnect: class = %v, want ECContextOverflow", r.Class)
	}
	if !r.Compress {
		t.Errorf("large session disconnect: Compress = false, want true")
	}

	// Server disconnect + small session → timeout
	r = classifyError("connection reset by peer", 1000, 200000)
	if r.Class != ECTimeout {
		t.Errorf("small session disconnect: class = %v, want ECTimeout", r.Class)
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

func TestErrorClassString(t *testing.T) {
	tests := []struct {
		class ErrorClass
		name  string
	}{
		{ECRetryable, "retryable"},
		{ECNonRetryable, "non_retryable"},
		{ECContextOverflow, "context_overflow"},
		{ECRateLimit, "rate_limit"},
		{ECAuth, "auth"},
		{ECUnknown, "unknown"},
	}

	for _, tt := range tests {
		if got := tt.class.String(); got != tt.name {
			t.Errorf("ErrorClass(%d).String() = %q, want %q", tt.class, got, tt.name)
		}
	}
}

func TestExtractStatusCode(t *testing.T) {
	tests := []struct {
		err  string
		code int
	}{
		{"500 internal server error", 500},
		{"status 429", 429},
		{"error 401: unauthorized", 401},
		{"connection refused", 0},
		{"no status here", 0},
		{"99 too small", 0},
		{"600 too large", 0},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			got := extractStatusCode(tt.err)
			if got != tt.code {
				t.Errorf("extractStatusCode(%q) = %d, want %d", tt.err, got, tt.code)
			}
		})
	}
}

func TestMatchesAny(t *testing.T) {
	patterns := []string{"rate limit", "too many requests", "throttled"}
	if !matchesAny("rate limit exceeded", patterns) {
		t.Error("should match 'rate limit'")
	}
	if !matchesAny("429 too many requests", patterns) {
		t.Error("should match 'too many requests'")
	}
	if matchesAny("connection refused", patterns) {
		t.Error("should not match")
	}
}

func TestClassifyErrorRetryAfter(t *testing.T) {
	// Rate limit with Retry-After hint
	r := classifyError("rate limit exceeded, retry after 30s", 0, 0)
	if r.Class != ECRateLimit {
		t.Errorf("class = %v, want ECRateLimit", r.Class)
	}
	// RetryAfter is only set when explicitly provided via header, not from message parsing
	// So we just verify the class is correct
}

func TestClassifyErrorUsageLimitDisambiguation(t *testing.T) {
	// Usage limit with transient signal → rate limit (retryable)
	r := classifyError("usage limit exceeded, please try again later", 0, 0)
	if r.Class != ECRateLimit {
		t.Errorf("transient usage limit: class = %v, want ECRateLimit", r.Class)
	}
	if !r.Retryable {
		t.Errorf("transient usage limit: should be retryable")
	}

	// Usage limit without transient signal → billing (non-retryable)
	r = classifyError("usage limit exceeded", 0, 0)
	if r.Class != ECBilling {
		t.Errorf("permanent usage limit: class = %v, want ECBilling", r.Class)
	}
	if r.Retryable {
		t.Errorf("permanent usage limit: should not be retryable")
	}
}

func TestClassifyErrorBillingPatterns(t *testing.T) {
	tests := []string{
		"insufficient credits",
		"insufficient_quota",
		"credits have been exhausted",
		"billing hard limit",
	}

	for _, err := range tests {
		t.Run(err, func(t *testing.T) {
			r := classifyError(err, 0, 0)
			if r.Class != ECBilling {
				t.Errorf("class = %v, want ECBilling", r.Class)
			}
			if r.Retryable {
				t.Errorf("billing errors should not be retryable")
			}
			if !r.RotateKey {
				t.Errorf("billing errors should suggest key rotation")
			}
		})
	}
}

func TestParseErrorBody(t *testing.T) {
	// Valid JSON body
	body := parseErrorBody(`error: {"type": "error", "message": "rate limited"}`)
	if body == nil {
		t.Error("should parse valid JSON body")
	}
	if body["type"] != "error" {
		t.Errorf("type = %v, want 'error'", body["type"])
	}

	// No JSON
	body = parseErrorBody("connection refused")
	if body != nil {
		t.Error("should return nil for non-JSON error")
	}

	// Malformed JSON
	body = parseErrorBody(`error: {"broken json`)
	if body != nil {
		t.Error("should return nil for malformed JSON")
	}
}

func TestTruncateStr(t *testing.T) {
	if got := truncateStr("hello", 10); got != "hello" {
		t.Errorf("short string: got %q, want %q", got, "hello")
	}
	if got := truncateStr("hello world", 5); got != "hello" {
		t.Errorf("long string: got %q, want %q", got, "hello")
	}
}

func TestIsErrorNonRetryable(t *testing.T) {
	if isErrorNonRetryable("connection refused") {
		t.Error("connection refused should be retryable")
	}
	if !isErrorNonRetryable("authentication failed") {
		t.Error("auth error should be non-retryable")
	}
	if !isErrorNonRetryable("context_length exceeded") {
		t.Error("context overflow should be non-retryable")
	}
}

func TestClassifyError403Disambiguation(t *testing.T) {
	// 403 with key limit → billing
	r := classifyError("403 key limit exceeded", 0, 0)
	if r.Class != ECBilling {
		t.Errorf("403 key limit: class = %v, want ECBilling", r.Class)
	}

	// 403 generic → auth
	r = classifyError("403 forbidden", 0, 0)
	if r.Class != ECAuth {
		t.Errorf("403 forbidden: class = %v, want ECAuth", r.Class)
	}
}

func TestClassifyError404Disambiguation(t *testing.T) {
	// 404 with model not found pattern
	r := classifyError("404 model not found", 0, 0)
	if r.Class != ECModelNotFound {
		t.Errorf("404 model not found: class = %v, want ECModelNotFound", r.Class)
	}

	// 404 generic → unknown
	r = classifyError("404 not found", 0, 0)
	if r.Class != ECUnknown {
		t.Errorf("404 generic: class = %v, want ECUnknown", r.Class)
	}
}

func TestClassifyError413(t *testing.T) {
	r := classifyError("413 payload too large", 0, 0)
	if r.Class != ECPayloadTooLarge {
		t.Errorf("class = %v, want ECPayloadTooLarge", r.Class)
	}
	if !r.Compress {
		t.Error("413 should suggest compression")
	}
}

func TestClassifyErrorServerDisconnectPatterns(t *testing.T) {
	patterns := []string{
		"server disconnected",
		"peer closed connection",
		"unexpected eof",
	}

	for _, err := range patterns {
		t.Run(err, func(t *testing.T) {
			r := classifyError(err, 0, 200000)
			if r.Class != ECTimeout {
				t.Errorf("class = %v, want ECTimeout", r.Class)
			}
		})
	}
}

func TestClassifyErrorTransportTypes(t *testing.T) {
	// These use lowercase no-space format
	r := classifyError("ConnectionError: connection refused", 0, 0)
	if !r.Retryable {
		t.Errorf("should be retryable, got class=%v", r.Class)
	}

	r = classifyError("ReadTimeout error", 0, 0)
	if r.Class != ECTimeout {
		t.Errorf("class = %v, want ECTimeout", r.Class)
	}

	r = classifyError("BrokenPipeError: broken pipe", 0, 0)
	if !r.Retryable {
		t.Errorf("should be retryable, got class=%v", r.Class)
	}
}

// Benchmark classifyError to ensure pattern matching is fast
func BenchmarkClassifyError(b *testing.B) {
	errors := []string{
		"connection refused",
		"500 internal server error",
		"rate limit exceeded",
		"context_length exceeded",
		"authentication failed",
		"model confused",
		"429 too many requests",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifyError(errors[i%len(errors)], 0, 0)
	}
}

// Verify backward compatibility wrappers
func TestBackwardCompatibilityWrappers(t *testing.T) {
	// isTransientError should match classifyError's Retryable
	tests := []string{
		"connection refused",
		"rate limit exceeded",
		"context_length exceeded",
		"authentication failed",
		"model confused",
		"",
	}
	for _, err := range tests {
		r := classifyError(err, 0, 0)
		got := isTransientError(err)
		if got != r.Retryable {
			t.Errorf("isTransientError(%q) = %v, classifyError().Retryable = %v", err, got, r.Retryable)
		}
	}
}

// Suppress unused import warning for time
var _ time.Duration
