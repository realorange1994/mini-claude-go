package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryEntry represents a single memory note.
type MemoryEntry struct {
	Category  string    // "preference" | "decision" | "state" | "reference"
	Content   string    // the actual note text
	Timestamp time.Time // when it was created
	Source    string    // "user" | "assistant" | "auto"
}

// SessionMemory manages structured notes that persist across the session.
// It runs as a background goroutine that periodically flushes notes to disk.
type SessionMemory struct {
	mu         sync.RWMutex
	entries    []MemoryEntry
	projectDir string
	filePath   string
	dirty      bool
	stopCh     chan struct{}
	maxEntries int
	// onAdd is an optional callback invoked when a note is added.
	// Used to mark the system prompt dirty so memory appears in the next turn.
	onAdd func()
}

// NewSessionMemory creates a new SessionMemory for the given project.
func NewSessionMemory(projectDir string) *SessionMemory {
	sm := &SessionMemory{
		entries:    make([]MemoryEntry, 0),
		projectDir: projectDir,
		filePath:   filepath.Join(projectDir, ".claude", "session_memory.md"),
		stopCh:     make(chan struct{}),
		maxEntries: 100,
	}
	sm.loadFromDisk()
	return sm
}

// SetOnAdd sets the callback invoked when a note is added.
func (sm *SessionMemory) SetOnAdd(fn func()) {
	sm.mu.Lock()
	sm.onAdd = fn
	sm.mu.Unlock()
}

// AddNote adds a new memory entry and marks the memory as dirty.
func (sm *SessionMemory) AddNote(category, content, source string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Deduplicate: if same category+content exists, update timestamp
	for i, e := range sm.entries {
		if e.Category == category && e.Content == content {
			sm.entries[i].Timestamp = time.Now()
			sm.dirty = true
			if sm.onAdd != nil {
				sm.onAdd()
			}
			return
		}
	}

	sm.entries = append(sm.entries, MemoryEntry{
		Category:  category,
		Content:   content,
		Timestamp: time.Now(),
		Source:    source,
	})

	// Enforce max entries (keep newest)
	if len(sm.entries) > sm.maxEntries {
		sm.entries = sm.entries[len(sm.entries)-sm.maxEntries:]
	}

	sm.dirty = true
	if sm.onAdd != nil {
		sm.onAdd()
	}
}

// GetNotes returns all memory entries, sorted by category then timestamp.
func (sm *SessionMemory) GetNotes() []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]MemoryEntry, len(sm.entries))
	copy(result, sm.entries)

	sort.Slice(result, func(i, j int) bool {
		if result[i].Category != result[j].Category {
			return result[i].Category < result[j].Category
		}
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	return result
}

// SearchNotes returns memory entries whose content contains the query (case-insensitive).
func (sm *SessionMemory) SearchNotes(query string) []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	lower := strings.ToLower(query)
	var result []MemoryEntry
	for _, e := range sm.entries {
		if strings.Contains(strings.ToLower(e.Content), lower) ||
			strings.Contains(strings.ToLower(e.Category), lower) {
			result = append(result, e)
		}
	}
	return result
}

// FormatForPrompt formats memory entries for injection into the system prompt.
func (sm *SessionMemory) FormatForPrompt() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.entries) == 0 {
		return ""
	}

	// Group by category
	groups := make(map[string][]MemoryEntry)
	var categories []string
	for _, e := range sm.entries {
		if _, ok := groups[e.Category]; !ok {
			categories = append(categories, e.Category)
		}
		groups[e.Category] = append(groups[e.Category], e)
	}
	sort.Strings(categories)

	var sb strings.Builder
	sb.WriteString("## Session Memory\n\n")
	sb.WriteString("The following notes were recorded during this or previous sessions. Use them as context.\n\n")

	for _, cat := range categories {
		entries := groups[cat]
		sb.WriteString(fmt.Sprintf("### %s\n", cat))
		for _, e := range entries {
			sb.WriteString(fmt.Sprintf("- %s\n", e.Content))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// loadFromDisk reads memory entries from the session memory file.
func (sm *SessionMemory) loadFromDisk() {
	data, err := os.ReadFile(sm.filePath)
	if err != nil {
		return // no file yet
	}

	// Parse markdown format: ### Category\n<!-- timestamp -->\n- content\n
	lines := strings.Split(string(data), "\n")
	var currentCategory string
	var lastTimestamp time.Time
	for _, line := range lines {
		if strings.HasPrefix(line, "### ") {
			currentCategory = strings.TrimSpace(strings.TrimPrefix(line, "### "))
		} else if strings.HasPrefix(line, "<!-- ") && strings.HasSuffix(line, " -->") {
			ts := strings.TrimPrefix(line, "<!-- ")
			ts = strings.TrimSuffix(ts, " -->")
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				lastTimestamp = t
			}
		} else if strings.HasPrefix(line, "- ") && currentCategory != "" {
			content := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			sm.entries = append(sm.entries, MemoryEntry{
				Category:  currentCategory,
				Content:   content,
				Timestamp: lastTimestamp,
				Source:    "disk",
			})
		}
	}
}

// flushToDisk writes memory entries to disk if dirty.
func (sm *SessionMemory) flushToDisk() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.dirty {
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(sm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}

	// Group by category
	groups := make(map[string][]MemoryEntry)
	var categories []string
	for _, e := range sm.entries {
		if _, ok := groups[e.Category]; !ok {
			categories = append(categories, e.Category)
		}
		groups[e.Category] = append(groups[e.Category], e)
	}
	sort.Strings(categories)

	var sb strings.Builder
	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("### %s\n", cat))
		for _, e := range groups[cat] {
			sb.WriteString(fmt.Sprintf("<!-- %s -->\n", e.Timestamp.Format(time.RFC3339)))
			sb.WriteString(fmt.Sprintf("- %s\n", e.Content))
		}
		sb.WriteString("\n")
	}

	if err := os.WriteFile(sm.filePath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("write memory file: %w", err)
	}

	sm.dirty = false
	return nil
}

// StartFlushLoop starts a background goroutine that periodically flushes
// memory to disk. Call Stop() to terminate.
func (sm *SessionMemory) StartFlushLoop() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := sm.flushToDisk(); err != nil {
					fmt.Fprintf(os.Stderr, "[memory] flush error: %v\n", err)
				}
			case <-sm.stopCh:
				// Final flush on stop
				sm.flushToDisk()
				return
			}
		}
	}()
}

// Stop signals the background flush goroutine to stop and does a final flush.
func (sm *SessionMemory) Stop() {
	close(sm.stopCh)
}
