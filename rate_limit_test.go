package main

import (
	"math"
	"net/http"
	"testing"
	"time"
)

func TestRateLimitBucketUsed(t *testing.T) {
	tests := []struct {
		limit     int
		remaining int
		want      int
	}{
		{100, 80, 20},
		{100, 100, 0},
		{100, 0, 100},
		{100, 150, 0}, // remaining > limit -> clamp to 0
	}

	for _, tt := range tests {
		b := RateLimitBucket{Limit: tt.limit, Remaining: tt.remaining}
		got := b.Used()
		if got != tt.want {
			t.Errorf("Used() = %d, want %d (limit=%d, remaining=%d)", got, tt.want, tt.limit, tt.remaining)
		}
	}
}

func TestRateLimitBucketUsagePct(t *testing.T) {
	tests := []struct {
		limit     int
		remaining int
		want      float64
	}{
		{100, 80, 20.0},
		{100, 0, 100.0},
		{100, 100, 0.0},
		{0, 0, 0.0}, // limit=0 -> avoid divide by zero
	}

	for _, tt := range tests {
		b := RateLimitBucket{Limit: tt.limit, Remaining: tt.remaining}
		got := b.UsagePct()
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("UsagePct() = %f, want %f", got, tt.want)
		}
	}
}

func TestRateLimitBucketRemainingSecondsNow(t *testing.T) {
	// Just captured, full reset time
	b := RateLimitBucket{ResetSeconds: 60, CapturedAt: time.Now()}
	got := b.RemainingSecondsNow()
	if got < 59 || got > 60 {
		t.Errorf("RemainingSecondsNow() = %f, expected ~60", got)
	}

	// Captured long ago, reset time elapsed
	b = RateLimitBucket{ResetSeconds: 1, CapturedAt: time.Now().Add(-10 * time.Second)}
	got = b.RemainingSecondsNow()
	if got != 0 {
		t.Errorf("RemainingSecondsNow() = %f, expected 0 for elapsed reset", got)
	}
}

func TestRateLimitStateHasData(t *testing.T) {
	s := &RateLimitState{}
	if s.HasData() {
		t.Error("new state should not have data")
	}

	s.CapturedAt = time.Now()
	if !s.HasData() {
		t.Error("state with CapturedAt should have data")
	}
}

func TestRateLimitStateRetryDelay(t *testing.T) {
	// No exhausted buckets -> 0 delay
	s := &RateLimitState{
		RequestsMin:  RateLimitBucket{Limit: 100, Remaining: 50, ResetSeconds: 60, CapturedAt: time.Now()},
		CapturedAt:   time.Now(),
	}
	if d := s.RetryDelay(); d != 0 {
		t.Errorf("expected 0 delay when not exhausted, got %v", d)
	}

	// Exhausted requests/min bucket
	s = &RateLimitState{
		RequestsMin:  RateLimitBucket{Limit: 100, Remaining: 0, ResetSeconds: 30, CapturedAt: time.Now()},
		CapturedAt:   time.Now(),
	}
	d := s.RetryDelay()
	if d == 0 {
		t.Error("expected non-zero delay when exhausted")
	}
	// Should be ~30s + 10% safety margin = ~33s
	if d < 30*time.Second || d > 35*time.Second {
		t.Errorf("expected ~33s delay, got %v", d)
	}
}

func TestRateLimitStateMostConstrainedBucket(t *testing.T) {
	s := &RateLimitState{
		RequestsMin:  RateLimitBucket{Limit: 100, Remaining: 10, CapturedAt: time.Now()},  // 90% used
		RequestsHour: RateLimitBucket{Limit: 1000, Remaining: 500, CapturedAt: time.Now()}, // 50% used
		TokensMin:    RateLimitBucket{Limit: 0},                                            // no data
		CapturedAt:   time.Now(),
	}

	label, bucket := s.MostConstrainedBucket()
	if label != "requests/min" {
		t.Errorf("expected 'requests/min' as most constrained, got %q", label)
	}
	if bucket.UsagePct() < 89 || bucket.UsagePct() > 91 {
		t.Errorf("expected ~90%% usage, got %f", bucket.UsagePct())
	}
}

func TestRateLimitStateUpdate(t *testing.T) {
	s := &RateLimitState{
		RequestsMin: RateLimitBucket{Limit: 100, Remaining: 50},
	}

	newState := &RateLimitState{
		RequestsMin:  RateLimitBucket{Limit: 100, Remaining: 30},
		TokensMin:    RateLimitBucket{Limit: 10000, Remaining: 5000},
		CapturedAt:   time.Now(),
	}

	s.Update(newState)

	if s.RequestsMin.Remaining != 30 {
		t.Errorf("expected RequestsMin.Remaining=30, got %d", s.RequestsMin.Remaining)
	}
	if s.TokensMin.Limit != 10000 {
		t.Errorf("expected TokensMin.Limit=10000, got %d", s.TokensMin.Limit)
	}
}

func TestParseRateLimitHeadersNil(t *testing.T) {
	result := ParseRateLimitHeaders(nil, "test")
	if result != nil {
		t.Error("expected nil for nil response")
	}
}

func TestParseRateLimitHeadersNoHeaders(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	result := ParseRateLimitHeaders(resp, "test")
	if result != nil {
		t.Error("expected nil for response with no rate limit headers")
	}
}

func TestParseRateLimitHeadersValid(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"X-Ratelimit-Limit-Requests":     {"100"},
			"X-Ratelimit-Remaining-Requests": {"80"},
			"X-Ratelimit-Reset-Requests":     {"30"},
			"X-Ratelimit-Limit-Tokens":       {"10000"},
			"X-Ratelimit-Remaining-Tokens":   {"5000"},
			"X-Ratelimit-Reset-Tokens":       {"60"},
		},
	}

	result := ParseRateLimitHeaders(resp, "anthropic")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.RequestsMin.Limit != 100 {
		t.Errorf("expected RequestsMin.Limit=100, got %d", result.RequestsMin.Limit)
	}
	if result.RequestsMin.Remaining != 80 {
		t.Errorf("expected RequestsMin.Remaining=80, got %d", result.RequestsMin.Remaining)
	}
	if result.TokensMin.Limit != 10000 {
		t.Errorf("expected TokensMin.Limit=10000, got %d", result.TokensMin.Limit)
	}
	if result.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", result.Provider)
	}
}

func TestParseSeconds(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"30", 30},
		{"30.5", 30.5},
		{"", 0},
		{"abc", 0},
	}

	for _, tt := range tests {
		got := parseSeconds(tt.input)
		if got != tt.want {
			t.Errorf("parseSeconds(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestFmtCount(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0K"},
		{33599, "33.6K"},
		{999999, "1000.0K"},
		{1000000, "1.0M"},
		{7999856, "8.0M"},
	}

	for _, tt := range tests {
		got := fmtCount(tt.input)
		if got != tt.want {
			t.Errorf("fmtCount(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFmtSeconds(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0, "0s"},
		{30, "30s"},
		{59, "59s"},
		{60, "1m"},
		{61, "1m 1s"},
		{90, "1m 30s"},
		{3599, "59m 59s"},
		{3600, "1h"},
		{3660, "1h 1m"},
		{-5, "0s"},
	}

	for _, tt := range tests {
		got := fmtSeconds(tt.input)
		if got != tt.want {
			t.Errorf("fmtSeconds(%f) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBar(t *testing.T) {
	tests := []struct {
		pct   float64
		width int
		want  string
	}{
		{0, 10, "[----------]"},
		{50, 10, "[#####-----]"},
		{100, 10, "[##########]"},
		{25, 8, "[##------]"},
		{-10, 10, "[----------]"},  // negative -> 0 filled
		{150, 10, "[##########]"},   // >100 -> all filled
	}

	for _, tt := range tests {
		got := bar(tt.pct, tt.width)
		if got != tt.want {
			t.Errorf("bar(%f, %d) = %q, want %q", tt.pct, tt.width, got, tt.want)
		}
	}
}

func TestFormatRateLimitDisplayNoData(t *testing.T) {
	state := &RateLimitState{}
	got := FormatRateLimitDisplay(state)
	if got == "" {
		t.Error("expected non-empty display for no data")
	}
}

func TestFormatRateLimitCompactNoData(t *testing.T) {
	state := &RateLimitState{}
	got := FormatRateLimitCompact(state)
	if got != "No rate limit data." {
		t.Errorf("expected 'No rate limit data.', got %q", got)
	}
}

func TestFormatRateLimitCompactWithData(t *testing.T) {
	state := &RateLimitState{
		RequestsMin:  RateLimitBucket{Limit: 100, Remaining: 80},
		TokensMin:    RateLimitBucket{Limit: 10000, Remaining: 5000},
		CapturedAt:   time.Now(),
	}
	got := FormatRateLimitCompact(state)
	if got == "" || got == "No rate limit data." {
		t.Errorf("expected compact display with data, got %q", got)
	}
}

// Suppress unused import warning
var _ = math.Abs