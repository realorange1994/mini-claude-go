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

// Writer writes transcript entries to a JSONL file (buffered).
type Writer struct {
	sessionID string
	filePath  string
	mu        sync.Mutex
	pending   []Entry
	closed    bool
}

// NewWriter creates a new transcript writer.
func NewWriter(sessionID, filePath string) *Writer {
	return &Writer{
		sessionID: sessionID,
		filePath:  filePath,
		pending:   make([]Entry, 0, 100),
	}
}

// Write adds an entry to the transcript (buffered, flushes at 100 entries).
func (w *Writer) Write(entry Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("transcript writer closed")
	}
	entry.Timestamp = time.Now()
	w.pending = append(w.pending, entry)
	if len(w.pending) >= 100 {
		return w.flush()
	}
	return nil
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

// Flush forces pending entries to disk.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flush()
}

func (w *Writer) flush() error {
	if len(w.pending) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(w.filePath), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(w.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, entry := range w.pending {
		line, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		f.Write(append(line, '\n'))
	}
	f.Sync()
	w.pending = w.pending[:0]
	return nil
}

// Close flushes and closes the writer.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return w.flush()
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
func (r *Reader) ReadAll() ([]Entry, error) {
	f, err := os.Open(r.filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			entries = append(entries, entry)
		}
	}
	return entries, scanner.Err()
}
