package main

import (
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// ─── Memory FTS Index (MiMo-Code pattern) ──────────────────────────────────

// MemoryFTSIndex provides full-text search over memory entries.
// Uses an in-memory inverted index with BM25-style ranking.
// Inspired by MiMo-Code's FTS5-based memory search.
type MemoryFTSIndex struct {
	mu      sync.RWMutex
	entries map[string]*IndexedEntry // key: "scope:category:content_hash"
	index   map[string][]Posting     // token -> list of postings
	idf     map[string]float64       // token -> inverse document frequency
	total   int                      // total number of indexed entries
	dirty   bool
}

// IndexedEntry represents an entry in the FTS index.
type IndexedEntry struct {
	Key      string
	Scope    MemoryScope
	Category string
	Content  string
	Tokens   []string
	Score    float64 // pre-computed static score
}

// Posting represents a token occurrence in a document.
type Posting struct {
	EntryKey string
	TF       float64 // term frequency
}

// NewMemoryFTSIndex creates a new FTS index.
func NewMemoryFTSIndex() *MemoryFTSIndex {
	return &MemoryFTSIndex{
		entries: make(map[string]*IndexedEntry),
		index:   make(map[string][]Posting),
		idf:     make(map[string]float64),
	}
}

// Index adds or updates an entry in the index.
func (idx *MemoryFTSIndex) Index(scope MemoryScope, category, content string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	key := makeKey(scope, category, content)
	tokens := tokenize(content)

	entry := &IndexedEntry{
		Key:      key,
		Scope:    scope,
		Category: category,
		Content:  content,
		Tokens:   tokens,
	}

	// Remove old entry if exists
	if old, exists := idx.entries[key]; exists {
		idx.removeEntryFromIndex(old)
		idx.total--
	}

	idx.entries[key] = entry
	idx.total++
	idx.addEntryToIndex(entry)
	idx.dirty = true
}

// Remove removes an entry from the index.
func (idx *MemoryFTSIndex) Remove(scope MemoryScope, category, content string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	key := makeKey(scope, category, content)
	if entry, exists := idx.entries[key]; exists {
		idx.removeEntryFromIndex(entry)
		delete(idx.entries, key)
		idx.total--
		idx.dirty = true
	}
}

// Search performs a full-text search and returns ranked results.
func (idx *MemoryFTSIndex) Search(query string, limit int) []FTSSearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// Score all entries
	scores := make(map[string]float64)
	for _, qt := range queryTokens {
		postings, ok := idx.index[qt]
		if !ok {
			continue
		}
		idf := idx.idf[qt]
		for _, p := range postings {
			// BM25-style scoring
			score := idf * p.TF / (p.TF + 1.0)
			scores[p.EntryKey] += score
		}
	}

	// Sort by score
	type scored struct {
		key   string
		score float64
	}
	var results []scored
	for key, score := range scores {
		if score > 0 {
			results = append(results, scored{key, score})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Build results
	var out []FTSSearchResult
	for i, r := range results {
		if limit > 0 && i >= limit {
			break
		}
		entry := idx.entries[r.key]
		if entry == nil {
			continue
		}
		out = append(out, FTSSearchResult{
			Scope:    entry.Scope,
			Category: entry.Category,
			Content:  entry.Content,
			Score:    r.score,
		})
	}

	return out
}

// FTSSearchResult represents an FTS search result.
type FTSSearchResult struct {
	Scope    MemoryScope
	Category string
	Content  string
	Score    float64
}

// addEntryToIndex adds an entry's tokens to the inverted index.
func (idx *MemoryFTSIndex) addEntryToIndex(entry *IndexedEntry) {
	tokenCounts := make(map[string]int)
	for _, token := range entry.Tokens {
		tokenCounts[token]++
	}

	for token, count := range tokenCounts {
		tf := float64(count) / float64(len(entry.Tokens))
		idx.index[token] = append(idx.index[token], Posting{
			EntryKey: entry.Key,
			TF:       tf,
		})
	}

	// Update IDF
	for token := range tokenCounts {
		docCount := len(idx.index[token])
		idx.idf[token] = math.Log(float64(idx.total)/float64(docCount+1)) + 1
	}
}

// removeEntryFromIndex removes an entry's tokens from the inverted index.
func (idx *MemoryFTSIndex) removeEntryFromIndex(entry *IndexedEntry) {
	for _, token := range entry.Tokens {
		postings := idx.index[token]
		for i, p := range postings {
			if p.EntryKey == entry.Key {
				idx.index[token] = append(postings[:i], postings[i+1:]...)
				break
			}
		}
		// Update IDF
		docCount := len(idx.index[token])
		if docCount > 0 {
			idx.idf[token] = math.Log(float64(idx.total)/float64(docCount+1)) + 1
		}
	}
}

// RebuildIndex rebuilds the entire index from current entries.
func (idx *MemoryFTSIndex) RebuildIndex() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.index = make(map[string][]Posting)
	idx.idf = make(map[string]float64)
	idx.total = len(idx.entries)

	for _, entry := range idx.entries {
		idx.addEntryToIndex(entry)
	}
	idx.dirty = false
}

// Stats returns index statistics.
func (idx *MemoryFTSIndex) Stats() FTSIndexStats {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return FTSIndexStats{
		TotalEntries: len(idx.entries),
		TotalTokens:  len(idx.index),
		Dirty:        idx.dirty,
	}
}

// FTSIndexStats holds index statistics.
type FTSIndexStats struct {
	TotalEntries int
	TotalTokens  int
	Dirty        bool
}

// ─── Tokenization ──────────────────────────────────────────────────────────

// tokenize splits text into searchable tokens.
func tokenize(text string) []string {
	lower := strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				token := current.String()
				if !isStopWord(token) && len(token) > 1 {
					tokens = append(tokens, token)
				}
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		token := current.String()
		if !isStopWord(token) && len(token) > 1 {
			tokens = append(tokens, token)
		}
	}

	return tokens
}

// makeKey creates a unique key for an entry.
func makeKey(scope MemoryScope, category, content string) string {
	// Simple hash: scope + category + first 50 chars
	prefix := content
	if len(prefix) > 50 {
		prefix = prefix[:50]
	}
	return string(scope) + ":" + category + ":" + prefix
}
