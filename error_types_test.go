package main

import (
	"testing"
)

// ─── classifyError core classification ───────────────────────────────────────

func TestClassifyErrorAuth401(t *testing.T) {
	cr := classifyError("401 Unauthorized", 0, 0)
	if cr.Class != ECAuth {
		t.Errorf("expected ECAuth, got %s", cr.Class)
	}
	if cr.Retryable {
		t.Error("401 should not be retryable")
	}
	if !cr.RotateKey {
		t.Error("401 should suggest key rotation")
	}
}

func TestClassifyErrorAuth403(t *testing.T) {
	cr := classifyError("403 Forbidden", 0, 0)
	if cr.Class != ECAuth {
		t.Errorf("expected ECAuth, got %s", cr.Class)
	}
	if cr.Retryable {
		t.Error("403 should not be retryable")
	}
}

func TestClassifyErrorBilling403KeyLimit(t *testing.T) {
	cr := classifyError("403 key limit exceeded", 0, 0)
	if cr.Class != ECBilling {
		t.Errorf("expected ECBilling, got %s", cr.Class)
	}
	if cr.Retryable {
		t.Error("billing should not be retryable")
	}
	if !cr.RotateKey {
		t.Error("billing should suggest key rotation")
	}
}

func TestClassifyErrorBilling402(t *testing.T) {
	cr := classifyError("402 insufficient credits", 0, 0)
	if cr.Class != ECBilling {
		t.Errorf("expected ECBilling, got %s", cr.Class)
	}
	if cr.Retryable {
		t.Error("402 billing should not be retryable")
	}
}

func TestClassifyErrorRateLimit429(t *testing.T) {
	cr := classifyError("429 rate limit exceeded", 0, 0)
	if cr.Class != ECRateLimit {
		t.Errorf("expected ECRateLimit, got %s", cr.Class)
	}
	if !cr.Retryable {
		t.Error("429 should be retryable")
	}
}

func TestClassifyErrorContextOverflow(t *testing.T) {
	cr := classifyError("prompt is too long: 137500 tokens > 135000 maximum", 0, 0)
	if cr.Class != ECContextOverflow {
		t.Errorf("expected ECContextOverflow, got %s", cr.Class)
	}
	if cr.Retryable {
		t.Error("context overflow should not be retryable without compression")
	}
	if !cr.Compress {
		t.Error("context overflow should suggest compression")
	}
}

func TestClassifyErrorContextOverflow400(t *testing.T) {
	cr := classifyError("400 context_length exceeded", 0, 0)
	if cr.Class != ECContextOverflow {
		t.Errorf("expected ECContextOverflow, got %s", cr.Class)
	}
	if !cr.Compress {
		t.Error("should suggest compression")
	}
}

func TestClassifyErrorToolPairing2013(t *testing.T) {
	cr := classifyError("2013 tool call result does not follow tool call", 0, 0)
	if cr.Class != ECToolPairing {
		t.Errorf("expected ECToolPairing, got %s", cr.Class)
	}
	if cr.Retryable {
		t.Error("tool pairing should not be retryable")
	}
}

func TestClassifyErrorModelNotFound404(t *testing.T) {
	cr := classifyError("404 model not found", 0, 0)
	if cr.Class != ECModelNotFound {
		t.Errorf("expected ECModelNotFound, got %s", cr.Class)
	}
	if cr.Retryable {
		t.Error("model not found should not be retryable")
	}
	if !cr.Fallback {
		t.Error("model not found should suggest fallback")
	}
}

func TestClassifyErrorPayloadTooLarge413(t *testing.T) {
	cr := classifyError("413 payload too large", 0, 0)
	if cr.Class != ECPayloadTooLarge {
		t.Errorf("expected ECPayloadTooLarge, got %s", cr.Class)
	}
	if !cr.Compress {
		t.Error("413 should suggest compression")
	}
}

func TestClassifyErrorOverloaded503(t *testing.T) {
	cr := classifyError("503 service overloaded", 0, 0)
	if cr.Class != ECOverloaded {
		t.Errorf("expected ECOverloaded, got %s", cr.Class)
	}
	if !cr.Retryable {
		t.Error("503 should be retryable")
	}
}

func TestClassifyErrorOverloaded529(t *testing.T) {
	cr := classifyError("529 overloaded", 0, 0)
	if cr.Class != ECOverloaded {
		t.Errorf("expected ECOverloaded, got %s", cr.Class)
	}
}

func TestClassifyErrorServer500(t *testing.T) {
	cr := classifyError("500 internal server error", 0, 0)
	if cr.Class != ECRetryable {
		t.Errorf("expected ECRetryable, got %s", cr.Class)
	}
	if !cr.Retryable {
		t.Error("500 should be retryable")
	}
}

func TestClassifyErrorServer502(t *testing.T) {
	cr := classifyError("502 bad gateway", 0, 0)
	if cr.Class != ECRetryable {
		t.Errorf("expected ECRetryable, got %s", cr.Class)
	}
}

func TestClassifyErrorTimeout(t *testing.T) {
	cr := classifyError("connection timed out", 0, 0)
	// "connection timed out" matches networkErrorPatterns which runs before
	// the deadline-exceeded timeout heuristic, so it classifies as retryable.
	if cr.Class != ECRetryable {
		t.Errorf("expected ECRetryable, got %s", cr.Class)
	}
	if !cr.Retryable {
		t.Error("timeout should be retryable")
	}
}

func TestClassifyErrorNetworkError(t *testing.T) {
	cr := classifyError("connection refused", 0, 0)
	if cr.Class != ECRetryable {
		t.Errorf("expected ECRetryable, got %s", cr.Class)
	}
}

func TestClassifyErrorFormatError400(t *testing.T) {
	cr := classifyError("400 bad request", 0, 0)
	if cr.Class != ECFormatError {
		t.Errorf("expected ECFormatError, got %s", cr.Class)
	}
	if cr.Retryable {
		t.Error("generic 400 should not be retryable")
	}
}

func TestClassifyErrorUnknown(t *testing.T) {
	cr := classifyError("something completely unexpected", 0, 0)
	if cr.Class != ECUnknown {
		t.Errorf("expected ECUnknown, got %s", cr.Class)
	}
	if cr.Retryable {
		t.Error("unknown should not be retryable by default")
	}
}

// ─── classify400 sub-classification ──────────────────────────────────────────

func TestClassify400ContextOverflow(t *testing.T) {
	cr := classifyError("400 prompt too long: context_length exceeded", 0, 0)
	if cr.Class != ECContextOverflow {
		t.Errorf("expected ECContextOverflow, got %s", cr.Class)
	}
	if !cr.Compress {
		t.Error("should suggest compression")
	}
}

func TestClassify400ModelNotFound(t *testing.T) {
	cr := classifyError("400 is not a valid model", 0, 0)
	if cr.Class != ECModelNotFound {
		t.Errorf("expected ECModelNotFound, got %s", cr.Class)
	}
}

func TestClassify400RateLimit(t *testing.T) {
	cr := classifyError("400 rate limit exceeded", 0, 0)
	if cr.Class != ECRateLimit {
		t.Errorf("expected ECRateLimit, got %s", cr.Class)
	}
}

func TestClassify400Billing(t *testing.T) {
	cr := classifyError("400 insufficient credits", 0, 0)
	if cr.Class != ECBilling {
		t.Errorf("expected ECBilling, got %s", cr.Class)
	}
}

func TestClassify400LargeSessionHeuristic(t *testing.T) {
	// 100K tokens with 200K context = 50% → should trigger context overflow heuristic
	cr := classifyError("400 bad request", 100000, 200000)
	if cr.Class != ECContextOverflow {
		t.Errorf("expected ECContextOverflow for large session, got %s", cr.Class)
	}
	if !cr.Compress {
		t.Error("should suggest compression")
	}
}

func TestClassify400SmallSessionNotOverflow(t *testing.T) {
	// 10K tokens with 200K context = 5% → should NOT trigger context overflow heuristic
	cr := classifyError("400 bad request", 10000, 200000)
	if cr.Class != ECFormatError {
		t.Errorf("expected ECFormatError for small session, got %s", cr.Class)
	}
}

// ─── classify402 sub-classification ──────────────────────────────────────────

func TestClassify402Billing(t *testing.T) {
	cr := classifyError("402 payment required", 0, 0)
	if cr.Class != ECBilling {
		t.Errorf("expected ECBilling, got %s", cr.Class)
	}
}

func TestClassify402TransientUsageLimit(t *testing.T) {
	cr := classifyError("402 usage limit exceeded, try again later", 0, 0)
	if cr.Class != ECRateLimit {
		t.Errorf("expected ECRateLimit for transient 402, got %s", cr.Class)
	}
}

// ─── extractStatusCode ───────────────────────────────────────────────────────

func TestExtractStatusCode(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"status_code=429", 429},
		{" 403 Forbidden", 403},
		{"error 500 internal", 500},
		{"no status code here", 0},
		{"code 99 too low", 0},
		{"code 600 too high", 0},
	}
	for _, tt := range tests {
		got := extractStatusCode(tt.input)
		if got != tt.want {
			t.Errorf("extractStatusCode(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// ─── parsePromptTooLongTokenGap ──────────────────────────────────────────────

func TestParsePromptTooLongTokenGap(t *testing.T) {
	actual, max, found := parsePromptTooLongTokenGap("prompt is too long: 137500 tokens > 135000 maximum")
	if !found {
		t.Error("expected to find token gap")
	}
	if actual != 137500 {
		t.Errorf("actual = %d, want 137500", actual)
	}
	if max != 135000 {
		t.Errorf("max = %d, want 135000", max)
	}
}

func TestParsePromptTooLongTokenGapNotFound(t *testing.T) {
	_, _, found := parsePromptTooLongTokenGap("some other error")
	if found {
		t.Error("should not find token gap in unrelated error")
	}
}

// ─── Backward compatibility wrappers ─────────────────────────────────────────

func TestIsTransientError(t *testing.T) {
	if isTransientError("500 internal server error") != true {
		t.Error("500 should be transient")
	}
	if isTransientError("401 unauthorized") != false {
		t.Error("401 should not be transient")
	}
}

func TestIsContextLengthError(t *testing.T) {
	if !isContextLengthError("prompt is too long: context_length exceeded") {
		t.Error("should detect context length error")
	}
	if isContextLengthError("500 internal server error") {
		t.Error("should not detect context length error for 500")
	}
}

func TestIsErrorNonRetryable(t *testing.T) {
	if !isErrorNonRetryable("401 unauthorized") {
		t.Error("401 should be non-retryable")
	}
	if isErrorNonRetryable("500 internal server error") {
		t.Error("500 should be retryable (not non-retryable)")
	}
}

// ─── matchesAny ──────────────────────────────────────────────────────────────

func TestMatchesAny(t *testing.T) {
	patterns := []string{"rate limit", "throttled", "too many"}
	if !matchesAny("request rate limit exceeded", patterns) {
		t.Error("should match rate limit pattern")
	}
	if matchesAny("all good", patterns) {
		t.Error("should not match any pattern")
	}
}

// ─── truncateStr ─────────────────────────────────────────────────────────────

func TestTruncateStr(t *testing.T) {
	if truncateStr("short", 100) != "short" {
		t.Error("short string should not be truncated")
	}
	result := truncateStr("hello world", 5)
	if len(result) > 5 {
		t.Errorf("truncated string should be <= 5 bytes, got %d", len(result))
	}
}

// ─── ErrorClass String ───────────────────────────────────────────────────────

func TestErrorClassString(t *testing.T) {
	tests := []struct {
		class ErrorClass
		want  string
	}{
		{ECRetryable, "retryable"},
		{ECNonRetryable, "non_retryable"},
		{ECContextOverflow, "context_overflow"},
		{ECToolPairing, "tool_pairing"},
		{ECRateLimit, "rate_limit"},
		{ECBilling, "billing"},
		{ECModelNotFound, "model_not_found"},
		{ECPayloadTooLarge, "payload_too_large"},
		{ECOverloaded, "overloaded"},
		{ECTimeout, "timeout"},
		{ECFormatError, "format_error"},
		{ECAuth, "auth"},
		{ECThinkingSig, "thinking_signature"},
		{ECLongContextTier, "long_context_tier"},
		{ECUnknown, "unknown"},
	}
	for _, tt := range tests {
		if tt.class.String() != tt.want {
			t.Errorf("ErrorClass(%d).String() = %q, want %q", tt.class, tt.class.String(), tt.want)
		}
	}
}

func TestErrorClassStringOutOfRange(t *testing.T) {
	class := ErrorClass(99)
	result := class.String()
	if result != "error_class(99)" {
		t.Errorf("out-of-range class should be error_class(99), got %s", result)
	}
}

// ─── parseErrorBody ──────────────────────────────────────────────────────────

func TestParseErrorBody(t *testing.T) {
	body := parseErrorBody(`error: {"type":"error","message":"rate limited"}`)
	if body == nil {
		t.Fatal("should parse JSON body")
	}
	if body["type"] != "error" {
		t.Errorf("expected type=error, got %v", body["type"])
	}
}

func TestParseErrorBodyNoJSON(t *testing.T) {
	body := parseErrorBody("no json here")
	if body != nil {
		t.Error("should return nil for non-JSON error")
	}
}

func TestParseErrorBodyMalformedJSON(t *testing.T) {
	body := parseErrorBody("error: {broken json}")
	if body != nil {
		t.Error("should return nil for malformed JSON")
	}
}

// ─── Server disconnect heuristic ─────────────────────────────────────────────

func TestClassifyErrorServerDisconnectLargeSession(t *testing.T) {
	// No status code, server disconnect, large session → context overflow
	cr := classifyError("server disconnected", 130000, 200000)
	if cr.Class != ECContextOverflow {
		t.Errorf("expected ECContextOverflow, got %s", cr.Class)
	}
	if !cr.Compress {
		t.Error("should suggest compression")
	}
}

func TestClassifyErrorServerDisconnectSmallSession(t *testing.T) {
	// No status code, server disconnect, small session → timeout
	cr := classifyError("server disconnected", 10000, 200000)
	if cr.Class != ECTimeout {
		t.Errorf("expected ECTimeout, got %s", cr.Class)
	}
}

// ─── Transport error heuristics ──────────────────────────────────────────────

func TestClassifyErrorTransportTimeout(t *testing.T) {
	cr := classifyError("readtimeout error", 0, 0)
	if cr.Class != ECTimeout {
		t.Errorf("expected ECTimeout, got %s", cr.Class)
	}
}

func TestClassifyErrorDeadlineExceeded(t *testing.T) {
	cr := classifyError("context deadline exceeded", 0, 0)
	if cr.Class != ECTimeout {
		t.Errorf("expected ECTimeout, got %s", cr.Class)
	}
}

// ─── Chinese context overflow patterns ───────────────────────────────────────

func TestClassifyErrorChineseContextOverflow(t *testing.T) {
	cr := classifyError("超过最大长度", 0, 0)
	if cr.Class != ECContextOverflow {
		t.Errorf("expected ECContextOverflow for Chinese pattern, got %s", cr.Class)
	}
}

// ─── 403 spending limit ──────────────────────────────────────────────────────

func TestClassifyError403SpendingLimit(t *testing.T) {
	cr := classifyError("403 spending limit exceeded", 0, 0)
	if cr.Class != ECBilling {
		t.Errorf("expected ECBilling for 403 spending limit, got %s", cr.Class)
	}
}

// ─── Billing patterns ────────────────────────────────────────────────────────

func TestClassifyErrorBillingPatterns(t *testing.T) {
	patterns := []string{
		"insufficient credits",
		"insufficient_quota",
		"credit balance is low",
		"credits have been exhausted",
		"top up your credits",
		"payment required",
		"billing hard limit reached",
		"exceeded your current quota",
		"account is deactivated",
		"plan does not include this feature",
	}
	for _, p := range patterns {
		cr := classifyError(p, 0, 0)
		if cr.Class != ECBilling {
			t.Errorf("expected ECBilling for %q, got %s", p, cr.Class)
		}
	}
}

// ─── Rate limit patterns ─────────────────────────────────────────────────────

func TestClassifyErrorRateLimitPatterns(t *testing.T) {
	patterns := []string{
		"rate limit exceeded",
		"rate_limit hit",
		"too many requests",
		"throttled by API",
		"requests per minute exceeded",
		"tokens per minute exceeded",
		"try again in 60 seconds",
		"please retry after 30s",
		"resource_exhausted",
	}
	for _, p := range patterns {
		cr := classifyError(p, 0, 0)
		if cr.Class != ECRateLimit {
			t.Errorf("expected ECRateLimit for %q, got %s", p, cr.Class)
		}
	}
}
