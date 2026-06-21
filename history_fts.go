package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── Cross-Session History FTS (MiMo-Code 2) ───────────────────────────────
//
// Full-text search index over all session message parts.
// Enables powerful cross-session recall.
//
// MiMo-Code source: history/service.ts (258 lines)

// HistoryEntry represents a searchable history entry.
type HistoryEntry struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	ToolName  string    `json:"tool_name,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Tokens    []string  `json:"-"`
}

// HistorySearchResultFTS represents a search result from history FTS.
type HistorySearchResultFTS struct {
	Entry HistoryEntry
	Score float64
}

// HistoryService provides cross-session history search.
type HistoryService struct {
	mu       sync.RWMutex
	entries  map[string]*HistoryEntry
	index    map[string][]string // token -> entry IDs
	total    int
}

// NewHistoryService creates a new history service.
func NewHistoryService() *HistoryService {
	return &HistoryService{
		entries: make(map[string]*HistoryEntry),
		index:   make(map[string][]string),
	}
}

// Index adds an entry to the history index.
func (h *HistoryService) Index(entry HistoryEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()

	tokens := tokenizeHistory(entry.Content)
	entry.Tokens = tokens

	h.entries[entry.ID] = &entry
	h.total++

	for _, token := range tokens {
		h.index[token] = append(h.index[token], entry.ID)
	}
}

// Search performs a full-text search over history.
func (h *HistoryService) Search(query string, limit int) []HistorySearchResultFTS {
	h.mu.RLock()
	defer h.mu.RUnlock()

	queryTokens := tokenizeHistory(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// Score entries
	scores := make(map[string]float64)
	for _, qt := range queryTokens {
		entryIDs := h.index[qt]
		idf := computeIDF(len(entryIDs), h.total)
		for _, id := range entryIDs {
			scores[id] += idf
		}
	}

	// Sort by score
	type scored struct {
		id    string
		score float64
	}
	var results []scored
	for id, score := range scores {
		results = append(results, scored{id, score})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Build results
	var out []HistorySearchResultFTS
	for i, r := range results {
		if limit > 0 && i >= limit {
			break
		}
		entry := h.entries[r.id]
		if entry != nil {
			out = append(out, HistorySearchResultFTS{
				Entry: *entry,
				Score: r.score,
			})
		}
	}

	return out
}

// Around returns context messages around a hit.
func (h *HistoryService) Around(entryID string, before, after int) []HistoryEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	entry, exists := h.entries[entryID]
	if !exists {
		return nil
	}

	// Find entries in the same session
	var sessionEntries []*HistoryEntry
	for _, e := range h.entries {
		if e.SessionID == entry.SessionID {
			sessionEntries = append(sessionEntries, e)
		}
	}

	// Sort by timestamp
	sort.Slice(sessionEntries, func(i, j int) bool {
		return sessionEntries[i].Timestamp.Before(sessionEntries[j].Timestamp)
	})

	// Find the target entry
	targetIdx := -1
	for i, e := range sessionEntries {
		if e.ID == entryID {
			targetIdx = i
			break
		}
	}

	if targetIdx < 0 {
		return nil
	}

	// Extract context
	start := targetIdx - before
	if start < 0 {
		start = 0
	}
	end := targetIdx + after + 1
	if end > len(sessionEntries) {
		end = len(sessionEntries)
	}

	var result []HistoryEntry
	for _, e := range sessionEntries[start:end] {
		result = append(result, *e)
	}
	return result
}

// Count returns the number of indexed entries.
func (h *HistoryService) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.total
}

// tokenizeHistory tokenizes text for history search.
func tokenizeHistory(text string) []string {
	lower := strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range lower {
		if isAlphanumeric(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				token := current.String()
				if len(token) > 2 && !isHistoryStopWord(token) {
					tokens = append(tokens, token)
				}
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		token := current.String()
		if len(token) > 2 && !isHistoryStopWord(token) {
			tokens = append(tokens, token)
		}
	}

	return tokens
}

// isAlphanumeric checks if a rune is alphanumeric.
func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// isHistoryStopWord checks if a token is a stop word.
func isHistoryStopWord(token string) bool {
	stopWords := map[string]bool{
		"the": true, "and": true, "for": true, "are": true, "but": true,
		"not": true, "you": true, "all": true, "can": true, "had": true,
		"her": true, "was": true, "one": true, "our": true, "out": true,
		"has": true, "have": true, "been": true, "some": true, "them": true,
	}
	return stopWords[token]
}

// computeIDF computes inverse document frequency.
func computeIDF(docCount, totalDocs int) float64 {
	if docCount == 0 {
		return 0
	}
	return float64(totalDocs) / float64(docCount+1)
}

// FormatHistoryResults formats history search results for display.
func FormatHistoryResults(results []HistorySearchResultFTS) string {
	if len(results) == 0 {
		return "No history results found."
	}

	var sb string
	sb += fmt.Sprintf("## History Search Results (%d found)\n\n", len(results))

	for i, r := range results {
		if i >= 10 {
			sb += fmt.Sprintf("\n... and %d more\n", len(results)-10)
			break
		}
		sb += fmt.Sprintf("- [%s] %s (score: %.2f)\n", r.Entry.Role, truncateHistoryStr(r.Entry.Content, 80), r.Score)
	}

	return sb
}

// truncateHistoryStr truncates a string to maxLen.
func truncateHistoryStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
