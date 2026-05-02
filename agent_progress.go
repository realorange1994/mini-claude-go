package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
)

// subAgentProgressWriter is an io.Writer that intercepts a sub-agent's output
// and writes condensed progress updates to the parent's output.
//
// It:
// - Suppresses [THINK] lines entirely
// - Extracts tool names from [+] / [ERR] / [TIMEOUT] lines
// - Writes a single condensed progress line to the parent REPL
// - Tracks tool call count and token usage for display
type subAgentProgressWriter struct {
	parentOut    func(string) // callback to write to parent REPL
	description  string
	mu           sync.Mutex
	toolCount    int
	totalTokens  int
	lastToolName string
	buf          bytes.Buffer
}

// NewSubAgentProgressWriter creates a new progress writer that writes condensed
// progress to the given callback. The description is used as the agent label
// in the progress line (e.g. "[agent: description] Running exec · 3 tool uses").
func NewSubAgentProgressWriter(description string, parentOut func(string)) *subAgentProgressWriter {
	return &subAgentProgressWriter{
		parentOut:   parentOut,
		description: description,
	}
}

func (w *subAgentProgressWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write all bytes to buffer for line detection
	w.buf.Write(p)

	// Process complete lines
	for {
		data := w.buf.Bytes()
		nl := bytes.IndexByte(data, '\n')
		if nl < 0 {
			break
		}
		line := string(data[:nl])
		w.buf.Next(nl + 1) // consume the line including newline
		w.processLine(line)
	}
	return len(p), nil
}

// Flush drains any remaining buffered content and emits a final progress line.
func (w *subAgentProgressWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf.Len() > 0 {
		w.processLine(w.buf.String())
		w.buf.Reset()
	}
	// Write final progress line
	w.writeProgressUnsafe()
	// Print newline to move past the single-line progress
	if w.parentOut != nil {
		w.parentOut("\n")
	}
}

func (w *subAgentProgressWriter) processLine(line string) {
	// Trim ANSI codes for matching
	clean := stripAnsi(line)

	// [THINK] lines: suppress entirely
	if strings.HasPrefix(clean, "[THINK]") {
		return
	}

	// [+], [ERR], [TIMEOUT] lines: extract tool name
	toolName := ""
	if idx := strings.Index(clean, "[+]"); idx >= 0 {
		toolName = extractToolName(clean[idx+3:])
	} else if idx := strings.Index(clean, "[ERR]"); idx >= 0 {
		toolName = extractToolName(clean[idx+5:])
	} else if strings.HasPrefix(clean, "[TIMEOUT]") {
		// TIMEOUT doesn't include a tool name, skip
		toolName = ""
	}

	if toolName != "" {
		w.toolCount++
		w.lastToolName = toolName
	}

	// Called from Write() which already holds the lock
	w.writeProgressUnsafe()
}

func (w *subAgentProgressWriter) writeProgress() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writeProgressUnsafe()
}

// writeProgressUnsafe writes the progress line without acquiring the lock.
// Caller must hold w.mu.
func (w *subAgentProgressWriter) writeProgressUnsafe() {
	if w.parentOut == nil {
		return
	}

	var sb strings.Builder

	// Start with carriage return and clear-to-end-line for single-line updates
	sb.WriteString("\r\x1b[K")
	sb.WriteString("[agent: ")
	sb.WriteString(w.description)
	sb.WriteString("]")

	if w.lastToolName != "" {
		sb.WriteString(" Running ")
		sb.WriteString(w.lastToolName)
	}

	sb.WriteString(" · ")
	sb.WriteString(fmt.Sprintf("%d tool uses", w.toolCount))

	if w.totalTokens > 0 {
		sb.WriteString(fmt.Sprintf(" · %d tokens", w.totalTokens))
	}

	w.parentOut(sb.String())
}

// GetStats returns the current tool count and token count.
func (w *subAgentProgressWriter) GetStats() (toolCount, totalTokens int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.toolCount, w.totalTokens
}

// UpdateTokens updates the token count and writes a progress update.
// Called periodically from the sub-agent goroutine to reflect live token usage.
func (w *subAgentProgressWriter) UpdateTokens(totalTokens int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.totalTokens = totalTokens
	w.writeProgressUnsafe()
}

// SetToolCount sets the tool count directly (used to sync stats at completion).
func (w *subAgentProgressWriter) SetToolCount(count int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.toolCount = count
}

// extractToolName parses the tool name from a tool result line like
// " exec: reading config" or " file_read"
func extractToolName(s string) string {
	s = strings.TrimSpace(s)
	// Format: "tool_name: preview" or "tool_name"
	if idx := strings.Index(s, ":"); idx >= 0 {
		name := strings.TrimSpace(s[:idx])
		// Take only the first word (tool name)
		if sp := strings.Index(name, " "); sp >= 0 {
			name = name[:sp]
		}
		return name
	}
	// Just the tool name
	if sp := strings.Index(s, " "); sp >= 0 {
		return s[:sp]
	}
	return s
}

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	var sb strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' || r == 'K' || r == 'J' || r == 'H' || r == 'L' {
				inEscape = false
			}
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// parentOutputAdapter creates an io.Writer that writes to the parent agent's
// output. Used to give the subAgentProgressWriter access to parent output.
type parentOutputAdapter struct {
	parentWriter io.Writer
}

func (a *parentOutputAdapter) Write(p []byte) (n int, err error) {
	return a.parentWriter.Write(p)
}

// Ensure subAgentProgressWriter implements io.Writer.
var _ io.Writer = (*subAgentProgressWriter)(nil)
