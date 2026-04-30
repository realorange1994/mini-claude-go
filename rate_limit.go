package main

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimitBucket represents one rate-limit window (e.g. requests per minute).
type RateLimitBucket struct {
	Limit        int
	Remaining    int
	ResetSeconds float64
	CapturedAt   time.Time
}

// Used returns tokens/requests consumed.
func (b *RateLimitBucket) Used() int {
	used := b.Limit - b.Remaining
	if used < 0 {
		return 0
	}
	return used
}

// UsagePct returns usage as a percentage.
func (b *RateLimitBucket) UsagePct() float64 {
	if b.Limit <= 0 {
		return 0
	}
	return float64(b.Used()) / float64(b.Limit) * 100
}

// RemainingSecondsNow returns estimated seconds until reset, adjusted for elapsed time.
func (b *RateLimitBucket) RemainingSecondsNow() float64 {
	elapsed := time.Since(b.CapturedAt).Seconds()
	remaining := b.ResetSeconds - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// RateLimitState holds the full rate-limit state parsed from response headers.
type RateLimitState struct {
	mu           sync.RWMutex
	RequestsMin  RateLimitBucket
	RequestsHour RateLimitBucket
	TokensMin    RateLimitBucket
	TokensHour   RateLimitBucket
	CapturedAt   time.Time
	Provider     string
}

// HasData returns true if rate limit data has been captured.
func (s *RateLimitState) HasData() bool {
	return !s.CapturedAt.IsZero()
}

// Age returns how long ago the data was captured.
func (s *RateLimitState) Age() time.Duration {
	if !s.HasData() {
		return time.Duration(math.MaxInt64)
	}
	return time.Since(s.CapturedAt)
}

// MostConstrainedBucket returns the bucket with the highest usage percentage,
// which is the one most likely to cause a 429 next.
func (s *RateLimitState) MostConstrainedBucket() (label string, bucket RateLimitBucket) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	buckets := []struct {
		label  string
		bucket RateLimitBucket
	}{
		{"requests/min", s.RequestsMin},
		{"requests/hr", s.RequestsHour},
		{"tokens/min", s.TokensMin},
		{"tokens/hr", s.TokensHour},
	}

	var maxPct float64
	for _, b := range buckets {
		if b.bucket.Limit > 0 {
			pct := b.bucket.UsagePct()
			if pct > maxPct {
				maxPct = pct
				label = b.label
				bucket = b.bucket
			}
		}
	}
	return
}

// RetryDelay estimates when it's safe to retry based on rate limit state.
// Returns 0 if no rate limit data or if retry should be safe now.
func (s *RateLimitState) RetryDelay() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	buckets := []RateLimitBucket{
		s.RequestsMin, s.RequestsHour, s.TokensMin, s.TokensHour,
	}

	var maxDelay float64
	for _, b := range buckets {
		if b.Remaining <= 0 && b.Limit > 0 {
			// This bucket is exhausted -- wait for reset
			delay := b.RemainingSecondsNow()
			if delay > maxDelay {
				maxDelay = delay
			}
		}
	}

	if maxDelay <= 0 {
		return 0
	}
	// Add 10% safety margin
	return time.Duration(maxDelay*1.1) * time.Second
}

// Update merges new rate limit data into the state.
func (s *RateLimitState) Update(newState *RateLimitState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if newState.RequestsMin.Limit > 0 {
		s.RequestsMin = newState.RequestsMin
	}
	if newState.RequestsHour.Limit > 0 {
		s.RequestsHour = newState.RequestsHour
	}
	if newState.TokensMin.Limit > 0 {
		s.TokensMin = newState.TokensMin
	}
	if newState.TokensHour.Limit > 0 {
		s.TokensHour = newState.TokensHour
	}
	if !newState.CapturedAt.IsZero() {
		s.CapturedAt = newState.CapturedAt
		s.Provider = newState.Provider
	}
}

// ParseRateLimitHeaders extracts x-ratelimit-* headers from an HTTP response.
// Returns nil if no rate limit headers are present.
// Supports Nous Portal / OpenRouter / OpenAI-compatible header format:
//
//	x-ratelimit-limit-requests          RPM cap
//	x-ratelimit-limit-requests-1h       RPH cap
//	x-ratelimit-limit-tokens            TPM cap
//	x-ratelimit-limit-tokens-1h         TPH cap
//	x-ratelimit-remaining-requests      requests left in minute window
//	x-ratelimit-remaining-requests-1h   requests left in hour window
//	x-ratelimit-remaining-tokens        tokens left in minute window
//	x-ratelimit-remaining-tokens-1h     tokens left in hour window
//	x-ratelimit-reset-requests          seconds until minute request window resets
//	x-ratelimit-reset-requests-1h       seconds until hour request window resets
//	x-ratelimit-reset-tokens            seconds until minute token window resets
//	x-ratelimit-reset-tokens-1h         seconds until hour token window resets
//
// Also supports standard Retry-After header.
func ParseRateLimitHeaders(resp *http.Response, provider string) *RateLimitState {
	if resp == nil {
		return nil
	}

	// Normalize headers to lowercase
	lowered := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			lowered[strings.ToLower(k)] = v[0]
		}
	}

	// Quick check: at least one rate limit header must exist
	hasAny := false
	for k := range lowered {
		if strings.HasPrefix(k, "x-ratelimit-") {
			hasAny = true
			break
		}
	}
	if !hasAny {
		// Check for Retry-After as a fallback
		if ra, ok := lowered["retry-after"]; ok {
			secs := parseSeconds(ra)
			if secs > 0 {
				now := time.Now()
				return &RateLimitState{
					RequestsMin: RateLimitBucket{CapturedAt: now},
					CapturedAt:  now,
					Provider:    provider,
				}
			}
		}
		return nil
	}

	now := time.Now()

	bucket := func(resource, suffix string) RateLimitBucket {
		tag := resource + suffix
		return RateLimitBucket{
			Limit:        parseInt(lowered["x-ratelimit-limit-"+tag]),
			Remaining:    parseInt(lowered["x-ratelimit-remaining-"+tag]),
			ResetSeconds: parseFloat(lowered["x-ratelimit-reset-"+tag]),
			CapturedAt:   now,
		}
	}

	return &RateLimitState{
		RequestsMin:  bucket("requests", ""),
		RequestsHour: bucket("requests", "-1h"),
		TokensMin:    bucket("tokens", ""),
		TokensHour:   bucket("tokens", "-1h"),
		CapturedAt:   now,
		Provider:     provider,
	}
}

// parseRateLimitHeadersFromMap parses from a string map (for non-HTTP contexts).
func parseRateLimitHeadersFromMap(headers map[string]string, provider string) *RateLimitState {
	lowered := make(map[string]string)
	for k, v := range headers {
		lowered[strings.ToLower(k)] = v
	}

	hasAny := false
	for k := range lowered {
		if strings.HasPrefix(k, "x-ratelimit-") {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return nil
	}

	now := time.Now()

	bucket := func(resource, suffix string) RateLimitBucket {
		tag := resource + suffix
		return RateLimitBucket{
			Limit:        parseInt(lowered["x-ratelimit-limit-"+tag]),
			Remaining:    parseInt(lowered["x-ratelimit-remaining-"+tag]),
			ResetSeconds: parseFloat(lowered["x-ratelimit-reset-"+tag]),
			CapturedAt:   now,
		}
	}

	return &RateLimitState{
		RequestsMin:  bucket("requests", ""),
		RequestsHour: bucket("requests", "-1h"),
		TokensMin:    bucket("tokens", ""),
		TokensHour:   bucket("tokens", "-1h"),
		CapturedAt:   now,
		Provider:     provider,
	}
}

// parseSeconds parses a seconds value from a string (int or float).
func parseSeconds(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// parseInt parses an integer from a string.
func parseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

// parseFloat parses a float from a string.
func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// ── Formatting ──

// fmtCount returns a human-friendly number: 7999856 -> "8.0M", 33599 -> "33.6K".
func fmtCount(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return strconv.Itoa(n)
}

// fmtSeconds formats seconds into human-friendly duration.
func fmtSeconds(seconds float64) string {
	s := int(math.Max(0, seconds))
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	if s < 3600 {
		m := s / 60
		sec := s % 60
		if sec > 0 {
			return fmt.Sprintf("%dm %ds", m, sec)
		}
		return fmt.Sprintf("%dm", m)
	}
	h := s / 3600
	m := (s % 3600) / 60
	if m > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dh", h)
}

// bar returns an ASCII progress bar.
func bar(pct float64, width int) string {
	filled := int(pct / 100.0 * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	empty := width - filled
	return fmt.Sprintf("[%s%s]", strings.Repeat("#", filled), strings.Repeat("-", empty))
}

// bucketLine formats one bucket as a single line.
func bucketLine(label string, bucket RateLimitBucket, labelWidth int) string {
	if bucket.Limit <= 0 {
		return fmt.Sprintf("  %-14s  (no data)", label)
	}

	pct := bucket.UsagePct()
	used := fmtCount(bucket.Used())
	limit := fmtCount(bucket.Limit)
	remaining := fmtCount(bucket.Remaining)
	reset := fmtSeconds(bucket.RemainingSecondsNow())

	b := bar(pct, 20)
	return fmt.Sprintf("  %-14s %s %5.1f%%  %s/%s used  (%s left, resets in %s)",
		label, b, pct, used, limit, remaining, reset)
}

// FormatRateLimitDisplay formats rate limit state for terminal display.
func FormatRateLimitDisplay(state *RateLimitState) string {
	state.mu.RLock()
	defer state.mu.RUnlock()

	if !state.HasData() {
		return "No rate limit data yet -- make an API request first."
	}

	age := state.Age()
	var freshness string
	if age < 5*time.Second {
		freshness = "just now"
	} else if age < time.Minute {
		freshness = fmt.Sprintf("%.0fs ago", age.Seconds())
	} else {
		freshness = fmt.Sprintf("%s ago", fmtSeconds(age.Seconds()))
	}

	providerLabel := "Provider"
	if state.Provider != "" {
		providerLabel = strings.ToTitle(state.Provider[:1]) + strings.ToLower(state.Provider[1:])
	}

	lines := []string{
		fmt.Sprintf("%s Rate Limits (captured %s):", providerLabel, freshness),
		"",
		bucketLine("Requests/min", state.RequestsMin, 14),
		bucketLine("Requests/hr", state.RequestsHour, 14),
		"",
		bucketLine("Tokens/min", state.TokensMin, 14),
		bucketLine("Tokens/hr", state.TokensHour, 14),
	}

	// Warnings if any bucket is getting hot
	var warnings []string
	for _, entry := range []struct {
		label  string
		bucket RateLimitBucket
	}{
		{"requests/min", state.RequestsMin},
		{"requests/hr", state.RequestsHour},
		{"tokens/min", state.TokensMin},
		{"tokens/hr", state.TokensHour},
	} {
		if entry.bucket.Limit > 0 && entry.bucket.UsagePct() >= 80 {
			reset := fmtSeconds(entry.bucket.RemainingSecondsNow())
			warnings = append(warnings, fmt.Sprintf("  [!] %s at %.0f%% -- resets in %s",
				entry.label, entry.bucket.UsagePct(), reset))
		}
	}

	if len(warnings) > 0 {
		lines = append(lines, "")
		lines = append(lines, warnings...)
	}

	return strings.Join(lines, "\n")
}

// FormatRateLimitCompact returns a one-line summary for status bars.
func FormatRateLimitCompact(state *RateLimitState) string {
	state.mu.RLock()
	defer state.mu.RUnlock()

	if !state.HasData() {
		return "No rate limit data."
	}

	var parts []string
	rm := state.RequestsMin
	if rm.Limit > 0 {
		parts = append(parts, fmt.Sprintf("RPM: %d/%d", rm.Remaining, rm.Limit))
	}
	rh := state.RequestsHour
	if rh.Limit > 0 {
		parts = append(parts, fmt.Sprintf("RPH: %s/%s", fmtCount(rh.Remaining), fmtCount(rh.Limit)))
	}
	tm := state.TokensMin
	if tm.Limit > 0 {
		parts = append(parts, fmt.Sprintf("TPM: %s/%s", fmtCount(tm.Remaining), fmtCount(tm.Limit)))
	}
	th := state.TokensHour
	if th.Limit > 0 {
		parts = append(parts, fmt.Sprintf("TPH: %s/%s", fmtCount(th.Remaining), fmtCount(th.Limit)))
	}

	return strings.Join(parts, " | ")
}
