package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TelemetryEvent represents a single telemetry event.
type TelemetryEvent struct {
	Name      string                 `json:"name"`
	Timestamp string                 `json:"timestamp"`
	Duration  int64                  `json:"duration_ms,omitempty"`
	Tags      map[string]string      `json:"tags,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// TelemetryManager captures and persists telemetry events.
// Events are written to .claude/telemetry/YYYY-MM-DD.jsonl.
// Telemetry can be disabled via CLAUDE_CODE_TELEMETRY_DISABLED env var.
type TelemetryManager struct {
	mu      sync.Mutex
	dir     string
	enabled bool
	events  []TelemetryEvent
}

// NewTelemetryManager creates a telemetry manager.
// disabled=true prevents all telemetry recording.
// Env var CLAUDE_CODE_TELEMETRY_DISABLED is a fallback for backward compatibility.
func NewTelemetryManager(disabled bool) *TelemetryManager {
	dir := filepath.Join(".claude", "telemetry")
	os.MkdirAll(dir, 0o755)

	enabled := !disabled
	// Additional env var fallback (for backward compatibility)
	if !enabled {
		// already disabled by caller
	} else if v := os.Getenv("CLAUDE_CODE_TELEMETRY_DISABLED"); v == "1" || v == "true" {
		enabled = false
	}

	return &TelemetryManager{
		dir:     dir,
		enabled: enabled,
	}
}

// Record writes a telemetry event.
func (t *TelemetryManager) Record(name string, durationMs int64, tags map[string]string, fields map[string]interface{}) {
	if !t.enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	event := TelemetryEvent{
		Name:      name,
		Timestamp: time.Now().Format(time.RFC3339),
		Duration:  durationMs,
		Tags:      tags,
		Fields:    fields,
	}
	t.events = append(t.events, event)

	// Write to daily log
	day := time.Now().Format("2006-01-02")
	filePath := filepath.Join(t.dir, day+".jsonl")
	data, _ := json.Marshal(event)
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.Write(data)
	f.Write([]byte{'\n'})
	f.Close()
}

// RecordAPICall records an API call telemetry event.
func (t *TelemetryManager) RecordAPICall(model string, stream bool, durationMs int64, inputTokens int64, outputTokens int64, err error) {
	fields := map[string]interface{}{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	t.Record("api_call", durationMs, map[string]string{
		"model":  model,
		"stream": fmt.Sprintf("%v", stream),
	}, fields)
}

// RecordToolCall records a tool call telemetry event.
func (t *TelemetryManager) RecordToolCall(toolName string, durationMs int64, isError bool) {
	t.Record("tool_call", durationMs, map[string]string{
		"tool": toolName,
	}, map[string]interface{}{
		"is_error": isError,
	})
}

// RecordCompaction records a compaction event.
func (t *TelemetryManager) RecordCompaction(method string, tokensBefore int64, tokensAfter int64) {
	t.Record("compaction", 0, map[string]string{
		"method": method,
	}, map[string]interface{}{
		"tokens_before": tokensBefore,
		"tokens_after":  tokensAfter,
	})
}

// LoadFromFile reads today's JSONL log and populates the in-memory event list.
func (t *TelemetryManager) LoadFromFile() {
	t.mu.Lock()
	defer t.mu.Unlock()

	day := time.Now().Format("2006-01-02")
	filePath := filepath.Join(t.dir, day+".jsonl")
	f, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var event TelemetryEvent
		if err := json.Unmarshal([]byte(line), &event); err == nil {
			t.events = append(t.events, event)
		}
	}
}

// GetRecent returns the last N events.
func (t *TelemetryManager) GetRecent(n int) []TelemetryEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n > len(t.events) {
		n = len(t.events)
	}
	result := make([]TelemetryEvent, n)
	copy(result, t.events[len(t.events)-n:])
	return result
}

// Summary returns event counts by name.
func (t *TelemetryManager) Summary() map[string]int {
	t.mu.Lock()
	defer t.mu.Unlock()
	counts := make(map[string]int)
	for _, e := range t.events {
		counts[e.Name]++
	}
	return counts
}

// SetEnabled enables or disables telemetry.
func (t *TelemetryManager) SetEnabled(enabled bool) {
	t.mu.Lock()
	t.enabled = enabled
	t.mu.Unlock()
}

// handleTelemetry handles the /telemetry slash command.
func handleTelemetry(args []string) {
	tm := NewTelemetryManager(false)
	tm.LoadFromFile()

	if len(args) == 0 {
		summary := tm.Summary()
		if len(summary) == 0 {
			fmt.Println("No telemetry events recorded.")
			return
		}
		fmt.Println("Telemetry Summary:")
		for name, count := range summary {
			fmt.Printf("  %s: %d events\n", name, count)
		}
		status := "enabled"
		if !tm.enabled {
			status = "disabled"
		}
		fmt.Printf("Status: %s\n", status)
		return
	}

	switch args[0] {
	case "recent":
		n := 10
		if len(args) > 1 {
			fmt.Sscanf(args[1], "%d", &n)
		}
		events := tm.GetRecent(n)
		for _, e := range events {
			fmt.Printf("[%s] %s (%dms)\n", e.Timestamp, e.Name, e.Duration)
		}
	case "enable":
		tm.SetEnabled(true)
		fmt.Println("Telemetry enabled.")
	case "disable":
		tm.SetEnabled(false)
		fmt.Println("Telemetry disabled.")
	case "clear":
		dir := filepath.Join(".claude", "telemetry")
		os.RemoveAll(dir)
		os.MkdirAll(dir, 0o755)
		fmt.Println("Telemetry logs cleared.")
	default:
		fmt.Printf("Unknown telemetry command: %s\n", args[0])
		fmt.Println("Usage: /telemetry [recent [N]|enable|disable|clear]")
	}
}