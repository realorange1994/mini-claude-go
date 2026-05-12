// Package transcript provides JSONL-based transcript recording for audit and replay.
package transcript

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// uuidV4 generates a version 4 UUID using crypto/rand.
func uuidV4() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fallback: should essentially never happen
		return hex.EncodeToString(buf[:])
	}
	buf[6] = (buf[6] & 0x0f) | 0x40 // version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// Entry represents a single transcript entry.
type Entry struct {
	UUID       string         `json:"uuid"`                  // unique identifier per entry
	ParentUUID string         `json:"parent_uuid,omitempty"` // parent entry for DAG branching
	Type       string         `json:"type"`                  // user, assistant, tool_use, tool_result, error, system, metadata
	Content    string         `json:"content,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	ToolArgs   map[string]any `json:"tool_args,omitempty"`
	ToolID     string         `json:"tool_id,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
	Model      string         `json:"model,omitempty"`
	Error      string         `json:"error,omitempty"`
	Subtype    string         `json:"subtype,omitempty"`  // e.g. "compact_boundary", "interrupt", "custom-title", "tag"
	Metadata   map[string]any `json:"metadata,omitempty"` // extensible metadata
}

// Writer writes transcript entries to a JSONL file.
// Each Write call flushes immediately to disk for crash safety.
type Writer struct {
	sessionID string
	filePath  string
	mu        sync.Mutex
	closed    bool
	lastUUID  string // UUID of the last written entry for DAG chain
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
// It auto-generates a UUID if one is not set, and links to the parent entry
// via ParentUUID for DAG chain tracking.
func (w *Writer) Write(entry Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("transcript writer closed")
	}
	if entry.UUID == "" {
		entry.UUID = uuidV4()
	}
	if entry.ParentUUID == "" && w.lastUUID != "" {
		entry.ParentUUID = w.lastUUID
	}
	w.lastUUID = entry.UUID
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

// WriteTitle writes a custom-title metadata entry.
func (w *Writer) WriteTitle(title string) error {
	return w.Write(Entry{Type: "metadata", Subtype: "custom-title", Metadata: map[string]any{"title": title}})
}

// WriteTag writes a tag metadata entry.
func (w *Writer) WriteTag(tag string) error {
	return w.Write(Entry{Type: "metadata", Subtype: "tag", Metadata: map[string]any{"tag": tag}})
}

// WriteCompactBoundary writes a compact boundary with rich metadata.
func (w *Writer) WriteCompactBoundary(trigger string, preCompactTokens int, messagesSummarized int, discoveredTools []string) error {
	return w.Write(Entry{
		Type:    "system",
		Subtype: "compact_boundary",
		Metadata: map[string]any{
			"trigger":             trigger,
			"pre_compact_tokens":  preCompactTokens,
			"messages_summarized": messagesSummarized,
			"discovered_tools":    discoveredTools,
		},
	})
}

// WriteInterrupt writes an interrupt detection entry.
func (w *Writer) WriteInterrupt(interruptType string) error {
	return w.Write(Entry{
		Type:    "system",
		Subtype: "interrupt",
		Metadata: map[string]any{
			"interrupt_type": interruptType,
		},
	})
}

// Flush is a no-op kept for API compatibility; writes are already immediate.
func (w *Writer) Flush() error {
	return nil
}

// FilePath returns the path to the transcript file.
func (w *Writer) FilePath() string {
	return w.filePath
}

// LastUUID returns the UUID of the most recently written entry.
func (w *Writer) LastUUID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastUUID
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
// Deduplicates entries by UUID and skips pre-boundary entries in large files.
func (r *Reader) ReadAll() ([]Entry, error) {
	f, err := os.Open(r.filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Check file size for pre-boundary skip optimization
	var skipPreBoundary bool
	if info, statErr := f.Stat(); statErr == nil {
		skipPreBoundary = info.Size() > 5*1024*1024 // > 5MB
	}

	var entries []Entry
	var lastBadLine string
	seenUUIDs := make(map[string]bool)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// First pass: collect all entries
	var allEntries []Entry
	for scanner.Scan() {
		line := scanner.Bytes()
		var entry Entry
		if err := json.Unmarshal(line, &entry); err == nil {
			allEntries = append(allEntries, entry)
			lastBadLine = ""
		} else {
			// Keep the bad line in case it's the last one (truncated write)
			lastBadLine = string(line)
		}
	}
	// If the last line was corrupt (truncated JSON from crash/Ctrl+C),
	// it's safe to discard -- it was an incomplete write.
	_ = lastBadLine

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// For large files, skip entries before the last compact boundary
	startIdx := 0
	if skipPreBoundary {
		for i := len(allEntries) - 1; i >= 0; i-- {
			e := allEntries[i]
			if (e.Type == "system" && e.Subtype == "compact_boundary") || e.Type == "compact" {
				startIdx = i
				break
			}
		}
	}

	// Second pass: dedup and collect
	for i := startIdx; i < len(allEntries); i++ {
		entry := allEntries[i]
		if entry.UUID != "" {
			if seenUUIDs[entry.UUID] {
				continue // dedup
			}
			seenUUIDs[entry.UUID] = true
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// ============================================================================
// Interrupt Detection
// ============================================================================

// DetectInterruptType examines the last few entries to determine
// if and how the conversation was interrupted.
func DetectInterruptType(entries []Entry) string {
	if len(entries) == 0 {
		return "none"
	}
	last := entries[len(entries)-1]

	// If last entry is an explicit interrupt marker
	if last.Type == "system" && last.Subtype == "interrupt" {
		if md, ok := last.Metadata["interrupt_type"].(string); ok {
			return md
		}
	}

	// Heuristic: if last entry is a user message without a following assistant response
	if last.Type == "user" {
		return "interrupted_prompt"
	}

	// If last entry is tool_use without tool_result
	if last.Type == "tool_use" {
		return "interrupted_turn"
	}

	return "none"
}
