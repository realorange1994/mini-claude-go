// Package transcript provides JSONL-based transcript recording for audit and replay.
package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry represents a single transcript entry.
type Entry struct {
	Type      string         `json:"type"`                 // user, assistant, tool_use, tool_result, error, system
	Content   string         `json:"content,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
	ToolArgs  map[string]any `json:"tool_args,omitempty"`
	ToolID    string         `json:"tool_id,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Model     string         `json:"model,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// Writer writes transcript entries to a JSONL file.
// Each Write call flushes immediately to disk for crash safety.
type Writer struct {
	sessionID string
	filePath  string
	mu        sync.Mutex
	closed    bool
}

// NewWriter creates a new transcript writer.
func NewWriter(sessionID, filePath string) *Writer {
	return &Writer{
		sessionID: sessionID,
		filePath:  filePath,
	}
}

// NewWriterFromExisting creates a transcript writer that continues writing
// to an existing transcript file, preserving the original session ID.
func NewWriterFromExisting(filePath string) *Writer {
	// Extract session ID from filename (e.g. "20260428-235831.jsonl" -> "20260428-235831")
	base := filepath.Base(filePath)
	sessionID := base[:len(base)-len(".jsonl")]
	return &Writer{
		sessionID: sessionID,
		filePath:  filePath,
	}
}

// Write adds an entry to the transcript and flushes immediately to disk.
func (w *Writer) Write(entry Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("transcript writer closed")
	}
	entry.Timestamp = time.Now()
	return w.writeOne(entry)
}

// WriteUser writes a user message entry.
func (w *Writer) WriteUser(content string) error {
	return w.Write(Entry{Type: "user", Content: content})
}

// WriteAssistant writes an assistant response entry.
func (w *Writer) WriteAssistant(content, model string) error {
	return w.Write(Entry{Type: "assistant", Content: content, Model: model})
}

// WriteToolUse writes a tool use entry.
func (w *Writer) WriteToolUse(toolID, toolName string, args map[string]any) error {
	return w.Write(Entry{Type: "tool_use", ToolID: toolID, ToolName: toolName, ToolArgs: args})
}

// WriteToolResult writes a tool result entry.
func (w *Writer) WriteToolResult(toolID, toolName, result string) error {
	return w.Write(Entry{Type: "tool_result", ToolID: toolID, ToolName: toolName, Content: result})
}

// WriteError writes an error entry.
func (w *Writer) WriteError(err string) error {
	return w.Write(Entry{Type: "error", Error: err})
}

// WriteSystem writes a system entry.
func (w *Writer) WriteSystem(content string) error {
	return w.Write(Entry{Type: "system", Content: content})
}

// WriteCompact writes a compact boundary entry.
func (w *Writer) WriteCompact(trigger string, preCompactTokens int) error {
	content := fmt.Sprintf("Compacted conversation (trigger: %s, %d tokens compressed)", trigger, preCompactTokens)
	return w.Write(Entry{
		Type:    "compact",
		Content: content,
		ToolArgs: map[string]any{
			"pre_compact_tokens": preCompactTokens,
		},
	})
}

// WriteSummary writes a summary entry (from LLM-driven compaction).
func (w *Writer) WriteSummary(content string) error {
	return w.Write(Entry{Type: "summary", Content: content})
}

// Flush is a no-op kept for API compatibility; writes are already immediate.
func (w *Writer) Flush() error {
	return nil
}

// FilePath returns the path to the transcript file.
func (w *Writer) FilePath() string {
	return w.filePath
}

func (w *Writer) writeOne(entry Entry) error {
	if err := os.MkdirAll(filepath.Dir(w.filePath), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(w.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// Close closes the writer.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return nil
}

// ============================================================================
// Reader
// ============================================================================

// Reader reads transcript entries from a JSONL file.
type Reader struct {
	filePath string
}

// NewReader creates a new transcript reader.
func NewReader(filePath string) *Reader { return &Reader{filePath: filePath} }

// ReadAll reads all entries from the transcript.
// Handles truncated last lines (from Ctrl+C / crash) by discarding them.
func (r *Reader) ReadAll() ([]Entry, error) {
	f, err := os.Open(r.filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	var lastBadLine string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var entry Entry
		if err := json.Unmarshal(line, &entry); err == nil {
			entries = append(entries, entry)
			lastBadLine = ""
		} else {
			// Keep the bad line in case it's the last one (truncated write)
			lastBadLine = string(line)
		}
	}
	// If the last line was corrupt (truncated JSON from crash/Ctrl+C),
	// it's safe to discard -- it was an incomplete write.
	_ = lastBadLine

	return entries, scanner.Err()
}
