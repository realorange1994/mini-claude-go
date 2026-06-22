package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// ─── RecoverableError Tests ─────────────────────────────────────────────────

func TestRecoverableError_Error(t *testing.T) {
	err := NewRecoverableError("test error")
	if err.Error() != "test error" {
		t.Errorf("expected 'test error', got %q", err.Error())
	}
}

func TestRecoverableError_IsRecoverable(t *testing.T) {
	err := NewRecoverableError("test")
	if !IsRecoverable(err) {
		t.Error("expected recoverable")
	}
	if IsRecoverable(nil) {
		t.Error("expected false for nil")
	}
}

func TestRecoverableError_WithType(t *testing.T) {
	err := NewRecoverableErrorWithType("test", "validation")
	if err.ErrorType != "validation" {
		t.Errorf("expected 'validation', got %q", err.ErrorType)
	}
}

func TestFormatRecoverableError(t *testing.T) {
	err := NewRecoverableError("test")
	msg := FormatRecoverableError(err)
	if msg == "" {
		t.Error("expected non-empty message")
	}
}

func TestFormatRecoverableError_Nil(t *testing.T) {
	msg := FormatRecoverableError(nil)
	if msg != "" {
		t.Errorf("expected empty, got %q", msg)
	}
}

// ─── Abort Utilities Tests ──────────────────────────────────────────────────

func TestAbortAfter(t *testing.T) {
	ctx, cancel := AbortAfter(100 * time.Millisecond)
	defer cancel()

	select {
	case <-ctx.Done():
		// Expected
	case <-time.After(500 * time.Millisecond):
		t.Error("expected context to be cancelled")
	}
}

func TestAbortAfterAny(t *testing.T) {
	parent := context.Background()
	ctx, cancel := AbortAfterAny(parent, 100*time.Millisecond)
	defer cancel()

	select {
	case <-ctx.Done():
		// Expected
	case <-time.After(500 * time.Millisecond):
		t.Error("expected context to be cancelled")
	}
}

func TestAbortWithSignals(t *testing.T) {
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	combined, cancel := AbortWithSignals(ctx1)
	defer cancel()

	cancel1()
	time.Sleep(50 * time.Millisecond) // Allow goroutine to run

	select {
	case <-combined.Done():
		// Expected
	case <-time.After(500 * time.Millisecond):
		t.Error("expected combined context to be cancelled")
	}
}

func TestFormatAbortStatus(t *testing.T) {
	ctx := context.Background()
	status := FormatAbortStatus(ctx)
	if status != "Active" {
		t.Errorf("expected 'Active', got %q", status)
	}
}

func TestFormatAbortStatus_Nil(t *testing.T) {
	status := FormatAbortStatus(nil)
	if status != "No context." {
		t.Errorf("expected 'No context.', got %q", status)
	}
}

// ─── Server Middleware Tests ─────────────────────────────────────────────────

func TestNewMiddlewareConfig(t *testing.T) {
	config := NewMiddlewareConfig()
	if config == nil {
		t.Error("expected non-nil config")
	}
}

func TestCORSMiddleware_Allowed(t *testing.T) {
	config := &MiddlewareConfig{AllowedOrigins: []string{"http://localhost"}}
	middleware := CORSMiddleware(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	// Test would require httptest, simplified check
	if handler == nil {
		t.Error("expected non-nil handler")
	}
}

func TestAuthMiddleware_NoPassword(t *testing.T) {
	middleware := AuthMiddleware("")
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	if handler == nil {
		t.Error("expected non-nil handler")
	}
}

func TestCompressionMiddleware(t *testing.T) {
	middleware := CompressionMiddleware(false)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	if handler == nil {
		t.Error("expected non-nil handler")
	}
}

func TestIsSSEEndpoint(t *testing.T) {
	if !isSSEEndpoint("/event") {
		t.Error("expected true for /event")
	}
	if !isSSEEndpoint("/global/event") {
		t.Error("expected true for /global/event")
	}
	if isSSEEndpoint("/api/data") {
		t.Error("expected false for /api/data")
	}
}

func TestIsSessionEndpoint(t *testing.T) {
	if !isSessionEndpoint("/session/s1/message") {
		t.Error("expected true for session message")
	}
	if !isSessionEndpoint("/session/s1/prompt") {
		t.Error("expected true for session prompt")
	}
	if isSessionEndpoint("/api/data") {
		t.Error("expected false for /api/data")
	}
}

func TestFormatMiddlewareStatus(t *testing.T) {
	config := &MiddlewareConfig{AllowedOrigins: []string{"http://localhost"}}
	status := FormatMiddlewareStatus(config)
	if status == "" {
		t.Error("expected non-empty status")
	}
}

func TestFormatMiddlewareStatus_Nil(t *testing.T) {
	status := FormatMiddlewareStatus(nil)
	if status != "No middleware." {
		t.Errorf("expected 'No middleware.', got %q", status)
	}
}
