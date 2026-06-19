package main

import (
	"fmt"
	"testing"
	"time"
)

func TestIsRetryableStreamError_RateLimit(t *testing.T) {
	tests := []struct {
		err      string
		retryable bool
	}{
		{"HTTP 429 Too Many Requests", true},
		{"HTTP 500 Internal Server Error", true},
		{"HTTP 502 Bad Gateway", true},
		{"HTTP 503 Service Unavailable", true},
		{"HTTP 504 Gateway Timeout", true},
		{"HTTP 529", true},
		{"rate_limit exceeded", true},
		{"too many requests", true},
		{"overloaded", true},
		{"capacity exceeded", true},
		{"ECONNRESET", true},
		{"connection reset by peer", true},
		{"connection timed out", true},
		{"server disconnected", true},
		{"unexpected eof", true},
		{"SSE read timed out", true},
		{"stream stalled", true},
		// Non-retryable
		{"context_length_exceeded", false},
		{"prompt is too long", false},
		{"invalid api key", false},
		{"authentication failed", false},
		{"bad request", false},
	}

	for _, tt := range tests {
		err := fmt.Errorf("%s", tt.err)
		got := isRetryableStreamError(err)
		if got != tt.retryable {
			t.Errorf("isRetryableStreamError(%q) = %v, want %v", tt.err, got, tt.retryable)
		}
	}
}

func TestIsRetryableStreamError_Nil(t *testing.T) {
	if isRetryableStreamError(nil) {
		t.Error("expected false for nil error")
	}
}

func TestGetRetryDelay_ExponentialBackoff(t *testing.T) {
	err := fmt.Errorf("HTTP 500")

	tests := []struct {
		attempt int
		min     time.Duration
		max     time.Duration
	}{
		{0, 1 * time.Second, 3 * time.Second},
		{1, 3 * time.Second, 5 * time.Second},
		{2, 7 * time.Second, 9 * time.Second},
		{3, 15 * time.Second, 17 * time.Second},
		{10, 29 * time.Second, 31 * time.Second}, // capped at 30s
	}

	for _, tt := range tests {
		delay := getRetryDelay(err, tt.attempt)
		if delay < tt.min || delay > tt.max {
			t.Errorf("attempt %d: expected %v-%v, got %v", tt.attempt, tt.min, tt.max, delay)
		}
	}
}

func TestGetRetryDelay_RetryAfterMs(t *testing.T) {
	err := fmt.Errorf("HTTP 429: retry-after-ms: 5000")
	delay := getRetryDelay(err, 0)
	if delay != 5000*time.Millisecond {
		t.Errorf("expected 5000ms, got %v", delay)
	}
}

func TestGetRetryDelay_RetryAfterSeconds(t *testing.T) {
	err := fmt.Errorf("HTTP 429: retry-after: 10")
	delay := getRetryDelay(err, 0)
	if delay != 10*time.Second {
		t.Errorf("expected 10s, got %v", delay)
	}
}

func TestParseInt64(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"123", 123, false},
		{"0", 0, false},
		{"-1", -1, false},
		{"abc", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		got, err := parseInt64(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseInt64(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("parseInt64(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
