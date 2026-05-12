package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrorReporter captures and stores error events for analysis.
// This provides a lightweight local error reporting system that
// can be extended to integrate with Sentry or other services.
type ErrorReporter struct {
	mu      sync.Mutex
	dir     string
	enabled bool
	events  []ErrorEvent
}

// ErrorEvent represents a captured error event.
type ErrorEvent struct {
	Timestamp   string                 `json:"timestamp"`
	Message     string                 `json:"message"`
	Type        string                 `json:"type"`
	Stack       string                 `json:"stack,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	SessionID   string                 `json:"session_id,omitempty"`
	Model       string                 `json:"model,omitempty"`
	Severity    string                 `json:"severity"` // "error", "warning", "info"
}

// NewErrorReporter creates an error reporter that writes events to .claude/errors/
func NewErrorReporter() *ErrorReporter {
	dir := filepath.Join(".claude", "errors")
	os.MkdirAll(dir, 0o755)
	return &ErrorReporter{
		dir:     dir,
		enabled: true,
	}
}

// Capture records an error event.
func (r *ErrorReporter) Capture(msg string, severity string, context map[string]interface{}) {
	if !r.enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	event := ErrorEvent{
		Timestamp: time.Now().Format(time.RFC3339),
		Message:   msg,
		Type:      classifyErrorType(msg),
		Severity:  severity,
		Context:   context,
	}
	r.events = append(r.events, event)

	// Write to daily log file
	day := time.Now().Format("2006-01-02")
	filePath := filepath.Join(r.dir, day+".jsonl")
	data, _ := json.Marshal(event)
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.Write(data)
	f.Write([]byte{'\n'})
	f.Close()
}

// CaptureError records an error-level event.
func (r *ErrorReporter) CaptureError(msg string, context map[string]interface{}) {
	r.Capture(msg, "error", context)
}

// CaptureWarning records a warning-level event.
func (r *ErrorReporter) CaptureWarning(msg string, context map[string]interface{}) {
	r.Capture(msg, "warning", context)
}

// GetRecent returns the last N captured events.
func (r *ErrorReporter) GetRecent(n int) []ErrorEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n > len(r.events) {
		n = len(r.events)
	}
	result := make([]ErrorEvent, n)
	copy(result, r.events[len(r.events)-n:])
	return result
}

// SetEnabled enables or disables error reporting.
func (r *ErrorReporter) SetEnabled(enabled bool) {
	r.mu.Lock()
	r.enabled = enabled
	r.mu.Unlock()
}

// Summary returns a summary of captured events by type.
func (r *ErrorReporter) Summary() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	counts := make(map[string]int)
	for _, e := range r.events {
		counts[e.Type]++
	}
	return counts
}

// classifyErrorType categorizes an error message into a type.
func classifyErrorType(msg string) string {
	if containsAny(msg, "context_length_exceeded", "context overflow", "max context") {
		return "context_overflow"
	}
	if containsAny(msg, "529", "overloaded") {
		return "overloaded"
	}
	if containsAny(msg, "429", "rate limit") {
		return "rate_limit"
	}
	if containsAny(msg, "stream stalled", "stream error", "stream interrupted") {
		return "stream_error"
	}
	if containsAny(msg, "2013", "tool pairing") {
		return "tool_pairing"
	}
	if containsAny(msg, "permission", "denied", "blocked") {
		return "permission"
	}
	if containsAny(msg, "timeout", "deadline exceeded") {
		return "timeout"
	}
	if containsAny(msg, "network", "connection", "DNS") {
		return "network"
	}
	return "unknown"
}

func containsAny(s string, patterns ...string) bool {
	for _, p := range patterns {
		if len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
		if len(s) > len(p) {
			for i := 0; i <= len(s)-len(p); i++ {
				if s[i:i+len(p)] == p {
					return true
				}
			}
		}
	}
	return false
}

// handleErrors handles the /errors slash command.
func handleErrors(args []string) {
	reporter := NewErrorReporter()

	if len(args) == 0 {
		// Show summary
		summary := reporter.Summary()
		if len(summary) == 0 {
			fmt.Println("No errors recorded.")
			return
		}
		fmt.Println("Error Summary:")
		for typ, count := range summary {
			fmt.Printf("  %s: %d\n", typ, count)
		}
		return
	}

	switch args[0] {
	case "recent":
		n := 10
		if len(args) > 1 {
			fmt.Sscanf(args[1], "%d", &n)
		}
		events := reporter.GetRecent(n)
		for _, e := range events {
			fmt.Printf("[%s] %s: %s\n", e.Timestamp, e.Severity, e.Message)
		}
	case "clear":
		dir := filepath.Join(".claude", "errors")
		os.RemoveAll(dir)
		os.MkdirAll(dir, 0o755)
		fmt.Println("Error logs cleared.")
	default:
		fmt.Printf("Unknown errors command: %s\n", args[0])
		fmt.Println("Usage: /errors [recent [N]] [clear]")
	}
}