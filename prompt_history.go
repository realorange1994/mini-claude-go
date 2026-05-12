package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PromptHistory persists user prompts to a JSONL file for session continuity.
// Each entry records the prompt text, timestamp, and session ID.
type PromptHistory struct {
	filePath string
	mu       sync.Mutex
}

// PromptEntry is a single history record.
type PromptEntry struct {
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"session_id"`
}

// NewPromptHistory creates a history manager that writes to .claude/history.jsonl.
func NewPromptHistory(sessionID string) *PromptHistory {
	dir := ".claude"
	os.MkdirAll(dir, 0o755)
	fp := filepath.Join(dir, "history.jsonl")
	return &PromptHistory{filePath: fp}
}

// Record appends a prompt to the history file.
func (h *PromptHistory) Record(text, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry := PromptEntry{
		Text:      text,
		Timestamp: time.Now().Format(time.RFC3339),
		SessionID: sessionID,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	f, err := os.OpenFile(h.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.Write(data)
	f.Write([]byte{'\n'})
	f.Close()
}

// LoadRecent returns the most recent N prompts from history.
func (h *PromptHistory) LoadRecent(n int) []PromptEntry {
	h.mu.Lock()
	defer h.mu.Unlock()

	f, err := os.Open(h.filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []PromptEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry PromptEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	if len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	return entries
}