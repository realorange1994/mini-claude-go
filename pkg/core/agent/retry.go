package agent

import (
	"math"
	"regexp"
	"strings"
	"time"
)

// RetryConfig holds auto-retry configuration.
type RetryConfig struct {
	MaxRetries  int
	BaseDelay   time.Duration
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  2 * time.Second,
	}
}

// retryablePatterns are error message patterns that indicate retryable errors.
// Aligned to TS _isRetryableError() regex patterns.
var retryablePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)overloaded`),
	regexp.MustCompile(`(?i)rate.?limit`),
	regexp.MustCompile(`(?i)\b429\b`),
	regexp.MustCompile(`(?i)\b500\b`),
	regexp.MustCompile(`(?i)\b502\b`),
	regexp.MustCompile(`(?i)\b503\b`),
	regexp.MustCompile(`(?i)\b504\b`),
	regexp.MustCompile(`(?i)connection.?error`),
	regexp.MustCompile(`(?i)websocket`),
	regexp.MustCompile(`(?i)fetch.?failed`),
	regexp.MustCompile(`(?i)premature.?stream`),
	regexp.MustCompile(`(?i)request.?timeout`),
}

// nonRetryablePatterns are error patterns that should NOT be retried.
var nonRetryablePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)prompt.?too.?long`),
	regexp.MustCompile(`(?i)context.?window`),
	regexp.MustCompile(`(?i)token.?limit`),
	regexp.MustCompile(`(?i)billing`),
	regexp.MustCompile(`(?i)quota`),
	regexp.MustCompile(`(?i)invalid.?api.?key`),
	regexp.MustCompile(`(?i)authentication`),
	regexp.MustCompile(`(?i)not.?found`),
}

// IsRetryableError determines if an error can be automatically retried.
// Returns false for context overflow and billing errors.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for APIError type first
	if apiErr, ok := err.(*APIError); ok {
		if apiErr.IsRetryable() {
			return true
		}
	}

	errStr := err.Error()

	// Non-retryable patterns (check first — these take precedence)
	for _, pat := range nonRetryablePatterns {
		if pat.MatchString(errStr) {
			return false
		}
	}

	// Retryable patterns
	for _, pat := range retryablePatterns {
		if pat.MatchString(errStr) {
			return true
		}
	}

	// Also check for APIError's IsRetryable via the error text
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") || strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") {
		return true
	}

	return false
}

// IsContextOverflow detects if the error is due to context window overflow.
// Context overflow should NOT be retried — it should trigger compaction instead.
func IsContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.IsContextOverflow()
	}
	errStr := err.Error()
	return strings.Contains(strings.ToLower(errStr), "prompt is too long") ||
		strings.Contains(strings.ToLower(errStr), "context window") ||
		strings.Contains(strings.ToLower(errStr), "token limit")
}

// ShouldRetry calculates whether to retry and the delay before the next attempt.
// Uses exponential backoff: baseDelay * 2^(attempt-1).
func ShouldRetry(attempt int, config RetryConfig) (shouldRetry bool, delay time.Duration) {
	if attempt >= config.MaxRetries {
		return false, 0
	}

	// Exponential backoff
	exp := math.Pow(2, float64(attempt))
	delay = time.Duration(float64(config.BaseDelay) * exp)

	// Cap delay at 60 seconds
	if delay > 60*time.Second {
		delay = 60 * time.Second
	}

	return true, delay
}

// RetryState tracks the current retry attempt count.
type RetryState struct {
	Attempts int
}

// Reset resets the retry counter.
func (rs *RetryState) Reset() {
	rs.Attempts = 0
}

// Next increments the attempt counter.
func (rs *RetryState) Next() {
	rs.Attempts++
}
