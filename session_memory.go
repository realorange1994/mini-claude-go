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

// ─── Constants ────────────────────────────────────────────────────────────────

const (
	// Default session memory template matching upstream's structured format.
	// Each section has a header and italic description (template instruction).
	// The LLM updates only the content, preserving the structure.
	defaultSessionMemoryTemplate = `# Session Title
_A short and distinctive 5-10 word descriptive title. Super info dense, no filler_

# Current State
_What is actively being worked on right now? Pending tasks not yet completed. Immediate next steps._

# Task Specification
_What did the user ask to build? Any design decisions or explanatory context._

# Files and Functions
_What are the important files? What do they contain and why are they relevant?_

# Workflow
_What bash commands are usually run and in what order? How to interpret their output?_

# Errors & Corrections
_Errors encountered and how they were fixed. What did the user correct? What approaches failed?_

# Codebase and System Documentation
_What are the important system components? How do they work/fit together?_

# Learnings
_What has worked well? What has not? What to avoid? Do not duplicate items from other sections._

# Key Results
_If the user asked for a specific output (answer, table, document), repeat the exact result here._

# Worklog
_Step by step, what was attempted and done? Very terse summary for each step._
`

	// Token budget constants — reduced from 20000/60000 to improve cache hit rates.
	// For a coding agent, most sections (Learnings, Key Results, Worklog) are
	// redundant with git/file state. Keeping Current State and Errors is sufficient.
	maxTokensPerSection         = 2500
	maxTotalSessionMemoryTokens = 10000

	// Three-level memory token budgets (MiMo-Code pattern).
	// Global memory: cross-project user preferences (small, stable).
	// Project memory: project-level rules and architecture decisions (medium).
	// Session memory: per-session state and work context (large).
	maxGlobalMemoryTokens  = 6000
	maxProjectMemoryTokens = 10000

	// Entry expiration: state entries expire after 7 days,
	// other categories expire after 30 days.
	entryExpirationState = 7 * 24 * time.Hour
	entryExpirationOther = 30 * 24 * time.Hour

	// Max entries per category (to prevent unbounded growth)
	maxStateEntries      = 20
	maxDecisionEntries   = 30
	maxPreferenceEntries = 20
	maxReferenceEntries  = 50
	maxTestEntries       = 20
	maxWorklogEntries    = 30
	maxErrorEntries      = 20
	maxResultEntries     = 15
)

// ─── MemoryEntry ─────────────────────────────────────────────────────────────

// MemoryEntry represents a single memory note with version tracking.
type MemoryEntry struct {
	Category  string    // "preference" | "decision" | "state" | "reference" | "test"
	Content   string    // the actual note text
	Timestamp time.Time // when it was created
	Source    string    // "user" | "assistant" | "auto" | "disk"

	// Version control fields
	Version   int       `json:"version,omitempty"`   // version number (1 = original)
	PrevHash  string    `json:"prev_hash,omitempty"` // hash of previous version content
	UpdatedAt time.Time `json:"updated_at,omitempty"` // when last updated
}

// ContentHash returns a simple hash of the content for change detection.
func (e *MemoryEntry) ContentHash() string {
	// Simple hash: length + first/last 8 chars
	if len(e.Content) == 0 {
		return "empty"
	}
	prefix := e.Content
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	suffix := e.Content
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	return fmt.Sprintf("%d:%s:%s", len(e.Content), prefix, suffix)
}

// maxEntriesForCategory returns the max entries limit for a given category.
func maxEntriesForCategory(category string) int {
	switch category {
	case "state":
		return maxStateEntries
	case "decision":
		return maxDecisionEntries
	case "preference":
		return maxPreferenceEntries
	case "reference":
		return maxReferenceEntries
	case "test":
		return maxTestEntries
	case "worklog":
		return maxWorklogEntries
	case "error":
		return maxErrorEntries
	case "result":
		return maxResultEntries
	default:
		return 20
	}
}

// isWorkflowItem checks if a reference entry describes a workflow/command pattern.
func isWorkflowItem(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "command") ||
		strings.Contains(lower, "run ") ||
		strings.Contains(lower, "workflow") ||
		strings.Contains(lower, "pipeline") ||
		strings.Contains(lower, "build") ||
		strings.Contains(lower, "test ") ||
		strings.Contains(lower, "deploy")
}

// isErrorRelated checks if a decision entry mentions errors or failures.
func isErrorRelated(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "fail") ||
		strings.Contains(lower, "bug") ||
		strings.Contains(lower, "fix") ||
		strings.Contains(lower, "wrong") ||
		strings.Contains(lower, "issue") ||
		strings.Contains(lower, "correction") ||
		strings.Contains(lower, "doesn't work") ||
		strings.Contains(lower, "does not work") ||
		strings.Contains(lower, "broken")
}

// isArchitectureItem checks if a reference entry describes system architecture.
func isArchitectureItem(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "architecture") ||
		strings.Contains(lower, "component") ||
		strings.Contains(lower, "module") ||
		strings.Contains(lower, "struct") ||
		strings.Contains(lower, "interface") ||
		strings.Contains(lower, "package") ||
		strings.Contains(lower, "system") ||
		strings.Contains(lower, "layer") ||
		strings.Contains(lower, "service") ||
		strings.Contains(lower, "flow")
}

// expirationForCategory returns the TTL for entries in a given category.
func expirationForCategory(category string) time.Duration {
	switch category {
	case "state":
		return entryExpirationState
	default:
		return entryExpirationOther
	}
}

// isExpired returns true if the entry is older than the category TTL.
func (e MemoryEntry) isExpired() bool {
	return time.Since(e.Timestamp) > expirationForCategory(e.Category)
}

// ─── MemoryScope ─────────────────────────────────────────────────────────────

// MemoryScope identifies the persistence scope of a memory entry.
// Inspired by MiMo-Code's three-level memory hierarchy.
type MemoryScope string

const (
	ScopeGlobal  MemoryScope = "global"  // Cross-project user preferences
	ScopeProject MemoryScope = "project" // Project-level rules, architecture decisions
	ScopeSession MemoryScope = "session" // Per-session state and work context
)

// ─── SessionMemory ───────────────────────────────────────────────────────────

// SessionMemory manages structured notes that persist across the session.
// It uses file locking to safely handle concurrent writes from multiple
// instances, and expires old entries on load to prevent unbounded growth.
//
// Three-level memory hierarchy (MiMo-Code pattern):
//   - Global: ~/.claude/memory/global.md — cross-project preferences
//   - Project: {projectDir}/.claude/memory/project.md — project rules
//   - Session: {projectDir}/.claude/session_memory.md — session state
type SessionMemory struct {
	mu         sync.RWMutex
	entries    []MemoryEntry // session-scoped entries
	projectDir string
	filePath   string // session memory file path
	dirty      bool
	stopCh     chan struct{}
	stopOnce   sync.Once // guards against double-close of stopCh
	wg         sync.WaitGroup
	maxEntries int
	onAdd      func() // optional callback invoked when a note is added
	// LastSummarizedMessageUUID tracks the UUID of the most recent message that
	// has been summarized by session memory extraction. This enables incremental
	// SM-compact: subsequent compactions only compact forward from this point,
	// avoiding redundant re-summarization of already-summarized content.
	// Mirrors upstream's lastSummarizedMessageId in sessionMemoryUtils.ts.
	LastSummarizedMessageUUID string

	// Three-level memory (MiMo-Code pattern)
	globalEntries  []MemoryEntry // cross-project preferences
	globalPath     string        // ~/.claude/memory/global.md
	globalDirty    bool
	projectEntries []MemoryEntry // project-level rules
	projectPath    string        // {projectDir}/.claude/memory/project.md
	projectDirty   bool
}

// NewSessionMemory creates a new SessionMemory for the given project.
// Loads all three memory levels: global, project, and session.
// Default global path: {projectDir}/.claude/memory/global.md (project-local).
// Use NewSessionMemoryWithPaths to set a custom global path (e.g. ~/.claude/memory/global.md).
func NewSessionMemory(projectDir string) *SessionMemory {
	// Default global path is project-local for test isolation.
	// Main code should use NewSessionMemoryWithPaths for cross-project global memory.
	globalPath := filepath.Join(projectDir, ".claude", "memory", "global.md")
	return NewSessionMemoryWithPaths(projectDir, globalPath)
}

// NewSessionMemoryWithPaths creates a new SessionMemory with an optional custom global path.
// If globalPath is empty, defaults to ~/.claude/memory/global.md.
func NewSessionMemoryWithPaths(projectDir, globalPath string) *SessionMemory {
	// Global memory path: ~/.claude/memory/global.md (or custom)
	if globalPath == "" {
		homeDir, _ := os.UserHomeDir()
		if homeDir != "" {
			globalPath = filepath.Join(homeDir, ".claude", "memory", "global.md")
		}
	}

	// Project memory path: {projectDir}/.claude/memory/project.md
	projectPath := filepath.Join(projectDir, ".claude", "memory", "project.md")

	// Session memory path: {projectDir}/.claude/session_memory.md (legacy location)
	sessionPath := filepath.Join(projectDir, ".claude", "session_memory.md")

	sm := &SessionMemory{
		entries:        make([]MemoryEntry, 0),
		globalEntries:  make([]MemoryEntry, 0),
		projectEntries: make([]MemoryEntry, 0),
		projectDir:     projectDir,
		filePath:       sessionPath,
		globalPath:     globalPath,
		projectPath:    projectPath,
		stopCh:         make(chan struct{}),
		maxEntries:     100,
	}

	// Load all three memory levels
	sm.loadGlobalFromDisk()
	sm.loadProjectFromDisk()
	sm.loadFromDisk()

	// Clear state entries loaded from disk — they are stale session context
	// that should not bleed into new sessions.
	sm.ClearStateEntries()
	// Remove expired entries from other categories.
	sm.removeExpiredEntries()
	return sm
}

// SetOnAdd sets the callback invoked when a note is added.
func (sm *SessionMemory) SetOnAdd(fn func()) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onAdd = fn
}

// AddNote adds a new memory entry and schedules a flush to disk.
func (sm *SessionMemory) AddNote(category, content, source string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Exact deduplication: if same category+content exists, update timestamp
	for i, e := range sm.entries {
		if e.Category == category && e.Content == content {
			sm.entries[i].Timestamp = time.Now()
			sm.dirty = true
			sm.maybeInvokeOnAdd()
			return
		}
	}

	// Fuzzy deduplication: check for similar entries
	for i, e := range sm.entries {
		if e.Category == category {
			similarity := ContentSimilarity(e.Content, content)
			if similarity >= SimilarityThreshold {
				// Merge with existing entry
				sm.entries[i] = MergeEntries(e, MemoryEntry{
					Category:  category,
					Content:   content,
					Timestamp: time.Now(),
					Source:    source,
				})
				sm.dirty = true
				sm.maybeInvokeOnAdd()
				return
			}
		}
	}

	sm.entries = append(sm.entries, MemoryEntry{
		Category:  category,
		Content:   content,
		Timestamp: time.Now(),
		Source:    source,
		Version:   1,
		UpdatedAt: time.Now(),
	})

	// Enforce per-category max entries (keep newest)
	sm.trimCategoryEntriesLocked(category)

	sm.dirty = true
	sm.maybeInvokeOnAdd()
}

// AddScopedNote adds a memory entry to the specified scope.
// This is the primary API for the three-level memory system.
func (sm *SessionMemory) AddScopedNote(scope MemoryScope, category, content, source string) {
	switch scope {
	case ScopeGlobal:
		sm.addGlobalNote(category, content, source)
	case ScopeProject:
		sm.addProjectNote(category, content, source)
	case ScopeSession:
		sm.AddNote(category, content, source)
	}
}

// addGlobalNote adds a note to global memory (cross-project preferences).
func (sm *SessionMemory) addGlobalNote(category, content, source string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Deduplicate
	for i, e := range sm.globalEntries {
		if e.Category == category && e.Content == content {
			sm.globalEntries[i].Timestamp = time.Now()
			sm.globalDirty = true
			return
		}
	}

	sm.globalEntries = append(sm.globalEntries, MemoryEntry{
		Category:  category,
		Content:   content,
		Timestamp: time.Now(),
		Source:    source,
	})
	sm.globalDirty = true
}

// addProjectNote adds a note to project memory (project-level rules).
func (sm *SessionMemory) addProjectNote(category, content, source string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Deduplicate
	for i, e := range sm.projectEntries {
		if e.Category == category && e.Content == content {
			sm.projectEntries[i].Timestamp = time.Now()
			sm.projectDirty = true
			return
		}
	}

	sm.projectEntries = append(sm.projectEntries, MemoryEntry{
		Category:  category,
		Content:   content,
		Timestamp: time.Now(),
		Source:    source,
	})
	sm.projectDirty = true
}

// GetScopedNotes returns entries from the specified scope.
func (sm *SessionMemory) GetScopedNotes(scope MemoryScope) []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var source []MemoryEntry
	switch scope {
	case ScopeGlobal:
		source = sm.globalEntries
	case ScopeProject:
		source = sm.projectEntries
	case ScopeSession:
		source = sm.entries
	}

	result := make([]MemoryEntry, len(source))
	copy(result, source)

	sort.Slice(result, func(i, j int) bool {
		if result[i].Category != result[j].Category {
			return result[i].Category < result[j].Category
		}
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	return result
}

// SearchScopedNotes searches entries across specified scopes.
func (sm *SessionMemory) SearchScopedNotes(query string, scopes ...MemoryScope) []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(scopes) == 0 {
		scopes = []MemoryScope{ScopeGlobal, ScopeProject, ScopeSession}
	}

	lower := strings.ToLower(query)
	var result []MemoryEntry

	for _, scope := range scopes {
		var source []MemoryEntry
		switch scope {
		case ScopeGlobal:
			source = sm.globalEntries
		case ScopeProject:
			source = sm.projectEntries
		case ScopeSession:
			source = sm.entries
		}

		for _, e := range source {
			if strings.Contains(strings.ToLower(e.Content), lower) ||
				strings.Contains(strings.ToLower(e.Category), lower) {
				result = append(result, e)
			}
		}
	}

	return result
}

// SearchOptions configures intelligent search behavior.
type SearchOptions struct {
	Scopes          []MemoryScope
	Categories      []string
	Limit           int
	RecencyWeight   float64
	ExactMatchBonus float64
	MinScore        float64
}

// DefaultSearchOptions returns sensible defaults.
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		RecencyWeight:   0.3,
		ExactMatchBonus: 0.5,
		MinScore:        0.1,
	}
}

// SmartSearch performs intelligent memory retrieval with relevance ranking.
func (sm *SessionMemory) SmartSearch(query string, opts SearchOptions) []SearchResult {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(opts.Scopes) == 0 {
		opts.Scopes = []MemoryScope{ScopeGlobal, ScopeProject, ScopeSession}
	}

	var results []SearchResult
	lower := strings.ToLower(query)

	for _, scope := range opts.Scopes {
		var entries []MemoryEntry
		switch scope {
		case ScopeGlobal:
			entries = sm.globalEntries
		case ScopeProject:
			entries = sm.projectEntries
		case ScopeSession:
			entries = sm.entries
		}

		for _, e := range entries {
			// Apply category filter
			if len(opts.Categories) > 0 {
				found := false
				for _, cat := range opts.Categories {
					if e.Category == cat {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			score, matchType := calculateSearchRelevance(e, lower)
			if score > 0 {
				results = append(results, SearchResult{
					Entry:     e,
					Score:     score,
					Scope:     scope,
					MatchType: matchType,
				})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results
}

// ─── Advanced Search & Filtering ─────────────────────────────────────────────

// SearchResult represents a memory search result with relevance scoring.
type SearchResult struct {
	Entry     MemoryEntry `json:"entry"`
	Score     float64     `json:"score"`      // relevance score (0-1)
	Scope     MemoryScope `json:"scope"`      // which scope the entry is from
	MatchType string      `json:"match_type"` // "exact", "partial", "category", "recent", "filter"
}

// MemoryFilter represents filter criteria for memory search.
type MemoryFilter struct {
	Scopes     []MemoryScope `json:"scopes,omitempty"`
	Categories []string      `json:"categories,omitempty"`
	Sources    []string      `json:"sources,omitempty"`
	After      *time.Time    `json:"after,omitempty"`
	Before     *time.Time    `json:"before,omitempty"`
	Query      string        `json:"query,omitempty"`
	SortBy     string        `json:"sort_by,omitempty"`   // "time", "category", "relevance"
	SortDesc   bool          `json:"sort_desc,omitempty"`
	Limit      int           `json:"limit,omitempty"`
}

// FilteredSearch performs a search with advanced filtering.
func (sm *SessionMemory) FilteredSearch(filter MemoryFilter) []SearchResult {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(filter.Scopes) == 0 {
		filter.Scopes = []MemoryScope{ScopeGlobal, ScopeProject, ScopeSession}
	}

	var results []SearchResult
	query := strings.ToLower(filter.Query)

	for _, scope := range filter.Scopes {
		var entries []MemoryEntry
		switch scope {
		case ScopeGlobal:
			entries = sm.globalEntries
		case ScopeProject:
			entries = sm.projectEntries
		case ScopeSession:
			entries = sm.entries
		}

		for _, e := range entries {
			// Apply filters
			if !matchesFilter(e, filter) {
				continue
			}

			// Calculate relevance score
			score := 0.0
			matchType := "filter"
			if query != "" {
				score, matchType = calculateSearchRelevance(e, query)
				if score == 0 {
					continue
				}
			} else {
				score = 1.0 // No query = all match
			}

			results = append(results, SearchResult{
				Entry:     e,
				Score:     score,
				Scope:     scope,
				MatchType: matchType,
			})
		}
	}

	// Sort results
	sortResults(results, filter.SortBy, filter.SortDesc)

	// Apply limit
	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}

	return results
}

// matchesFilter checks if an entry matches the filter criteria.
func matchesFilter(entry MemoryEntry, filter MemoryFilter) bool {
	// Category filter
	if len(filter.Categories) > 0 {
		found := false
		for _, cat := range filter.Categories {
			if entry.Category == cat {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Source filter
	if len(filter.Sources) > 0 {
		found := false
		for _, src := range filter.Sources {
			if entry.Source == src {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Time filters
	if filter.After != nil && entry.Timestamp.Before(*filter.After) {
		return false
	}
	if filter.Before != nil && entry.Timestamp.After(*filter.Before) {
		return false
	}

	return true
}

// calculateSearchRelevance computes relevance score for search.
func calculateSearchRelevance(entry MemoryEntry, query string) (float64, string) {
	content := strings.ToLower(entry.Content)
	category := strings.ToLower(entry.Category)

	score := 0.0
	matchType := ""

	// Exact content match
	if strings.Contains(content, query) {
		score += 0.6
		matchType = "exact"
	}

	// Category match
	if strings.Contains(category, query) {
		score += 0.3
		if matchType == "" {
			matchType = "category"
		}
	}

	// Word matching
	queryWords := strings.Fields(query)
	contentWords := strings.Fields(content)
	matchedWords := 0
	for _, qw := range queryWords {
		for _, cw := range contentWords {
			if strings.Contains(cw, qw) {
				matchedWords++
				break
			}
		}
	}
	if len(queryWords) > 0 {
		wordScore := float64(matchedWords) / float64(len(queryWords))
		score += wordScore * 0.4
		if matchType == "" && wordScore > 0 {
			matchType = "partial"
		}
	}

	if score < 0.1 {
		return 0, ""
	}
	return score, matchType
}

// sortResults sorts search results by the specified field.
func sortResults(results []SearchResult, sortBy string, desc bool) {
	less := func(i, j int) bool {
		switch sortBy {
		case "time":
			if desc {
				return results[i].Entry.Timestamp.After(results[j].Entry.Timestamp)
			}
			return results[i].Entry.Timestamp.Before(results[j].Entry.Timestamp)
		case "category":
			if results[i].Entry.Category != results[j].Entry.Category {
				if desc {
					return results[i].Entry.Category > results[j].Entry.Category
				}
				return results[i].Entry.Category < results[j].Entry.Category
			}
			return results[i].Entry.Timestamp.After(results[j].Entry.Timestamp)
		default: // "relevance"
			if desc {
				return results[i].Score > results[j].Score
			}
			return results[i].Score < results[j].Score
		}
	}
	sort.Slice(results, less)
}

// GetCategories returns all unique categories across all scopes.
func (sm *SessionMemory) GetCategories() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	seen := make(map[string]bool)
	for _, e := range sm.entries {
		seen[e.Category] = true
	}
	for _, e := range sm.projectEntries {
		seen[e.Category] = true
	}
	for _, e := range sm.globalEntries {
		seen[e.Category] = true
	}

	result := make([]string, 0, len(seen))
	for cat := range seen {
		result = append(result, cat)
	}
	sort.Strings(result)
	return result
}

// GetSources returns all unique sources across all scopes.
func (sm *SessionMemory) GetSources() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	seen := make(map[string]bool)
	for _, e := range sm.entries {
		seen[e.Source] = true
	}
	for _, e := range sm.projectEntries {
		seen[e.Source] = true
	}
	for _, e := range sm.globalEntries {
		seen[e.Source] = true
	}

	result := make([]string, 0, len(seen))
	for src := range seen {
		result = append(result, src)
	}
	sort.Strings(result)
	return result
}

// GetTimeRange returns the earliest and latest timestamps across all entries.
func (sm *SessionMemory) GetTimeRange() (earliest, latest time.Time) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	first := true
	for _, e := range append(append(sm.entries, sm.projectEntries...), sm.globalEntries...) {
		if first {
			earliest = e.Timestamp
			latest = e.Timestamp
			first = false
		} else {
			if e.Timestamp.Before(earliest) {
				earliest = e.Timestamp
			}
			if e.Timestamp.After(latest) {
				latest = e.Timestamp
			}
		}
	}
	return
}

// FormatSearchResults formats search results for display.
func FormatSearchResults(results []SearchResult, query string) string {
	if len(results) == 0 {
		return fmt.Sprintf("No results found for '%s'.", query)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for '%s' (%d matches):\n\n", query, len(results)))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, r.Entry.Category, r.Entry.Content))
		sb.WriteString(fmt.Sprintf("   Scope: %s | Score: %.1f%% | Match: %s\n", r.Scope, r.Score*100, r.MatchType))
		sb.WriteString(fmt.Sprintf("   Source: %s | Time: %s\n\n", r.Entry.Source, r.Entry.Timestamp.Format("2006-01-02 15:04")))
	}

	return sb.String()
}

// SearchByCategory returns all entries in a specific category.
func (sm *SessionMemory) SearchByCategory(category string) []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var result []MemoryEntry
	for _, e := range sm.entries {
		if e.Category == category {
			result = append(result, e)
		}
	}
	for _, e := range sm.projectEntries {
		if e.Category == category {
			result = append(result, e)
		}
	}
	for _, e := range sm.globalEntries {
		if e.Category == category {
			result = append(result, e)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})
	return result
}

// SearchBySource returns all entries from a specific source.
func (sm *SessionMemory) SearchBySource(source string) []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var result []MemoryEntry
	for _, e := range sm.entries {
		if e.Source == source {
			result = append(result, e)
		}
	}
	for _, e := range sm.projectEntries {
		if e.Source == source {
			result = append(result, e)
		}
	}
	for _, e := range sm.globalEntries {
		if e.Source == source {
			result = append(result, e)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})
	return result
}

// SearchByTimeRange returns entries within a time range.
func (sm *SessionMemory) SearchByTimeRange(after, before time.Time) []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var result []MemoryEntry
	for _, e := range append(append(sm.entries, sm.projectEntries...), sm.globalEntries...) {
		if (e.Timestamp.Equal(after) || e.Timestamp.After(after)) &&
			(e.Timestamp.Equal(before) || e.Timestamp.Before(before)) {
			result = append(result, e)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})
	return result
}

// GetRecentEntries returns the N most recent entries across all scopes.
func (sm *SessionMemory) GetRecentEntries(n int) []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	all := append(append(sm.entries, sm.projectEntries...), sm.globalEntries...)
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.After(all[j].Timestamp)
	})

	if n > 0 && len(all) > n {
		all = all[:n]
	}
	return all
}

// ─── Intelligent Memory Retrieval ───────────────────────────────────────────

// ContextSearch searches memory based on current conversation context.
func (sm *SessionMemory) ContextSearch(contextMessages []string, limit int) []SearchResult {
	keywords := extractKeywords(contextMessages)
	if len(keywords) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var results []SearchResult

	for _, kw := range keywords {
		opts := DefaultSearchOptions()
		opts.Limit = 5
		opts.MinScore = 0.2
		hits := sm.SmartSearch(kw, opts)
		for _, hit := range hits {
			key := hit.Entry.Category + ":" + hit.Entry.Content
			if !seen[key] {
				seen[key] = true
				results = append(results, hit)
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// extractKeywords extracts significant keywords from context messages.
func extractKeywords(messages []string) []string {
	wordFreq := make(map[string]int)
	for _, msg := range messages {
		words := strings.Fields(strings.ToLower(msg))
		for _, w := range words {
			if isStopWord(w) || len(w) < 3 {
				continue
			}
			wordFreq[w]++
		}
	}

	type wordCount struct {
		word  string
		count int
	}
	var words []wordCount
	for w, c := range wordFreq {
		words = append(words, wordCount{w, c})
	}
	sort.Slice(words, func(i, j int) bool {
		return words[i].count > words[j].count
	})

	result := make([]string, 0, 10)
	for i, w := range words {
		if i >= 10 {
			break
		}
		result = append(result, w.word)
	}
	return result
}

// isStopWord checks if a word is a common stop word.
func isStopWord(w string) bool {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "shall": true, "can": true,
		"this": true, "that": true, "these": true, "those": true, "i": true,
		"you": true, "he": true, "she": true, "it": true, "we": true,
		"they": true, "me": true, "him": true, "her": true, "us": true,
		"them": true, "my": true, "your": true, "his": true, "its": true,
		"our": true, "their": true, "what": true, "which": true, "who": true,
		"when": true, "where": true, "why": true, "how": true, "all": true,
		"each": true, "every": true, "both": true, "few": true, "more": true,
		"most": true, "other": true, "some": true, "such": true, "no": true,
		"not": true, "only": true, "own": true, "same": true, "so": true,
		"than": true, "too": true, "very": true, "just": true, "because": true,
		"as": true, "until": true, "while": true, "of": true, "at": true,
		"by": true, "for": true, "with": true, "about": true, "against": true,
		"between": true, "through": true, "during": true, "before": true,
		"after": true, "above": true, "below": true, "to": true, "from": true,
		"up": true, "down": true, "in": true, "out": true, "on": true,
		"off": true, "over": true, "under": true, "again": true, "further": true,
		"then": true, "once": true, "here": true, "there": true, "any": true,
		"also": true, "well": true, "back": true, "even": true, "still": true,
		"new": true, "way": true, "use": true, "used": true, "like": true,
	}
	return stopWords[w]
}

// GetRelevantMemories returns memories relevant to the current task context.
func (sm *SessionMemory) GetRelevantMemories(taskContext string, limit int) string {
	keywords := extractKeywords([]string{taskContext})
	if len(keywords) == 0 {
		return ""
	}

	var allResults []SearchResult
	for _, kw := range keywords {
		opts := DefaultSearchOptions()
		opts.Limit = 3
		opts.MinScore = 0.15
		results := sm.SmartSearch(kw, opts)
		allResults = append(allResults, results...)
	}

	seen := make(map[string]bool)
	var unique []SearchResult
	for _, r := range allResults {
		key := r.Entry.Category + ":" + r.Entry.Content
		if !seen[key] {
			seen[key] = true
			unique = append(unique, r)
		}
	}

	sort.Slice(unique, func(i, j int) bool {
		return unique[i].Score > unique[j].Score
	})

	if limit > 0 && len(unique) > limit {
		unique = unique[:limit]
	}

	if len(unique) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Relevant Memories\n\n")
	for _, r := range unique {
		sb.WriteString(fmt.Sprintf("- [%s] %s (relevance: %.1f%%)\n",
			r.Entry.Category, r.Entry.Content, r.Score*100))
	}

	return sb.String()
}

// ─── Memory Consolidation ───────────────────────────────────────────────────

// ConsolidationConfig configures memory consolidation behavior.
type ConsolidationConfig struct {
	MaxEntriesPerCategory int           `json:"max_entries_per_category"`
	SimilarityThreshold   float64       `json:"similarity_threshold"`
	CompressOlderThan     time.Duration `json:"compress_older_than"`
	KeepRecent            int           `json:"keep_recent"`
}

// DefaultConsolidationConfig returns sensible defaults.
func DefaultConsolidationConfig() ConsolidationConfig {
	return ConsolidationConfig{
		MaxEntriesPerCategory: 30,
		SimilarityThreshold:   0.85,
		CompressOlderThan:     7 * 24 * time.Hour,
		KeepRecent:            5,
	}
}

// ConsolidationResult holds the result of a consolidation operation.
type ConsolidationResult struct {
	Merged     int `json:"merged"`
	Removed    int `json:"removed"`
	Compressed int `json:"compressed"`
	Remaining  int `json:"remaining"`
}

// ConsolidateMemory performs full memory consolidation: dedup, prune, compress.
func (sm *SessionMemory) ConsolidateMemory(config ConsolidationConfig) ConsolidationResult {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	result := ConsolidationResult{}
	var merged, removed, compressed int

	// Phase 1: Deduplicate entries
	sm.entries, merged = DeduplicateEntries(sm.entries)
	result.Merged += merged

	sm.projectEntries, merged = DeduplicateEntries(sm.projectEntries)
	result.Merged += merged

	sm.globalEntries, merged = DeduplicateEntries(sm.globalEntries)
	result.Merged += merged

	// Phase 2: Remove expired entries
	sm.entries, removed = removeExpired(sm.entries)
	result.Removed += removed

	sm.projectEntries, removed = removeExpired(sm.projectEntries)
	result.Removed += removed

	sm.globalEntries, removed = removeExpired(sm.globalEntries)
	result.Removed += removed

	// Phase 3: Compress old entries
	sm.entries, compressed = compressOldEntries(sm.entries, config.CompressOlderThan, config.KeepRecent)
	result.Compressed += compressed

	sm.projectEntries, compressed = compressOldEntries(sm.projectEntries, config.CompressOlderThan, config.KeepRecent)
	result.Compressed += compressed

	sm.globalEntries, compressed = compressOldEntries(sm.globalEntries, config.CompressOlderThan, config.KeepRecent)
	result.Compressed += compressed

	// Phase 4: Enforce max entries per category
	sm.entries = enforceMaxEntries(sm.entries, config.MaxEntriesPerCategory)
	sm.projectEntries = enforceMaxEntries(sm.projectEntries, config.MaxEntriesPerCategory)
	sm.globalEntries = enforceMaxEntries(sm.globalEntries, config.MaxEntriesPerCategory)

	result.Remaining = len(sm.entries) + len(sm.projectEntries) + len(sm.globalEntries)

	if result.Merged > 0 || result.Removed > 0 || result.Compressed > 0 {
		sm.dirty = true
	}

	return result
}

// removeExpired removes entries older than their category TTL.
func removeExpired(entries []MemoryEntry) ([]MemoryEntry, int) {
	removed := 0
	result := make([]MemoryEntry, 0, len(entries))
	for _, e := range entries {
		if e.isExpired() {
			removed++
		} else {
			result = append(result, e)
		}
	}
	return result, removed
}

// compressOldEntries compresses old entries by merging similar ones.
func compressOldEntries(entries []MemoryEntry, olderThan time.Duration, keepRecent int) ([]MemoryEntry, int) {
	if len(entries) <= keepRecent {
		return entries, 0
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	recent := entries[:keepRecent]
	old := entries[keepRecent:]

	cutoff := time.Now().Add(-olderThan)
	var toCompress, toKeep []MemoryEntry
	for _, e := range old {
		if e.Timestamp.Before(cutoff) {
			toCompress = append(toCompress, e)
		} else {
			toKeep = append(toKeep, e)
		}
	}

	if len(toCompress) <= 1 {
		return entries, 0
	}

	compressed, _ := DeduplicateEntries(toCompress)
	return append(append(recent, toKeep...), compressed...), len(toCompress) - len(compressed)
}

// enforceMaxEntries keeps only the newest entries per category.
func enforceMaxEntries(entries []MemoryEntry, maxPerCategory int) []MemoryEntry {
	groups := make(map[string][]MemoryEntry)
	for _, e := range entries {
		groups[e.Category] = append(groups[e.Category], e)
	}

	var result []MemoryEntry
	for _, group := range groups {
		if len(group) <= maxPerCategory {
			result = append(result, group...)
			continue
		}

		sort.Slice(group, func(i, j int) bool {
			return group[i].Timestamp.After(group[j].Timestamp)
		})
		result = append(result, group[:maxPerCategory]...)
	}

	return result
}

// ─── Memory Health & Reporting ──────────────────────────────────────────────

// MemoryHealth holds health metrics about the memory system.
type MemoryHealth struct {
	SessionEntries  int            `json:"session_entries"`
	ProjectEntries  int            `json:"project_entries"`
	GlobalEntries   int            `json:"global_entries"`
	TotalEntries    int            `json:"total_entries"`
	ExpiredEntries  int            `json:"expired_entries"`
	OldEntries      int            `json:"old_entries"`
	ByCategory      map[string]int `json:"by_category"`
	AvgAge          time.Duration  `json:"avg_age"`
	Score           int            `json:"score"` // 0-100
}

// GetMemoryHealth returns health metrics about the memory system.
func (sm *SessionMemory) GetMemoryHealth() MemoryHealth {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	health := MemoryHealth{
		SessionEntries: len(sm.entries),
		ProjectEntries: len(sm.projectEntries),
		GlobalEntries:  len(sm.globalEntries),
		TotalEntries:   len(sm.entries) + len(sm.projectEntries) + len(sm.globalEntries),
		ByCategory:     make(map[string]int),
	}

	for _, e := range sm.entries {
		health.ByCategory[e.Category]++
	}

	now := time.Now()
	for _, e := range sm.entries {
		if e.isExpired() {
			health.ExpiredEntries++
		}
		if now.Sub(e.Timestamp) > 30*24*time.Hour {
			health.OldEntries++
		}
	}

	if len(sm.entries) > 0 {
		var totalAge time.Duration
		for _, e := range sm.entries {
			totalAge += now.Sub(e.Timestamp)
		}
		health.AvgAge = totalAge / time.Duration(len(sm.entries))
	}

	health.Score = calculateHealthScore(health)
	return health
}

// calculateHealthScore returns a health score from 0-100.
func calculateHealthScore(health MemoryHealth) int {
	score := 100

	if health.TotalEntries > 200 {
		score -= 20
	} else if health.TotalEntries > 100 {
		score -= 10
	}

	if health.ExpiredEntries > 10 {
		score -= 15
	} else if health.ExpiredEntries > 5 {
		score -= 10
	}

	if health.OldEntries > 50 {
		score -= 15
	} else if health.OldEntries > 20 {
		score -= 10
	}

	if health.AvgAge > 30*24*time.Hour {
		score -= 10
	} else if health.AvgAge > 14*24*time.Hour {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	return score
}

// FormatConsolidationReport returns a human-readable consolidation report.
func FormatConsolidationReport(result ConsolidationResult, health MemoryHealth) string {
	var sb strings.Builder
	sb.WriteString("## Memory Consolidation Report\n\n")
	sb.WriteString(fmt.Sprintf("- Merged: %d entries\n", result.Merged))
	sb.WriteString(fmt.Sprintf("- Removed: %d expired entries\n", result.Removed))
	sb.WriteString(fmt.Sprintf("- Compressed: %d old entries\n", result.Compressed))
	sb.WriteString(fmt.Sprintf("- Remaining: %d entries\n\n", result.Remaining))
	sb.WriteString(fmt.Sprintf("Health Score: %d/100\n", health.Score))
	sb.WriteString(fmt.Sprintf("Total Entries: %d\n", health.TotalEntries))
	sb.WriteString(fmt.Sprintf("Expired: %d\n", health.ExpiredEntries))
	sb.WriteString(fmt.Sprintf("Old (>30d): %d\n", health.OldEntries))

	return sb.String()
}

// ─── Fuzzy Deduplication ────────────────────────────────────────────────────

// SimilarityThreshold is the minimum similarity score (0-1) to consider entries as duplicates.
const SimilarityThreshold = 0.85

// DeduplicateEntries removes near-duplicate entries using fuzzy matching.
// Returns the deduplicated list and the number of entries merged.
func DeduplicateEntries(entries []MemoryEntry) ([]MemoryEntry, int) {
	if len(entries) <= 1 {
		return entries, 0
	}

	merged := 0
	result := make([]MemoryEntry, 0, len(entries))
	used := make([]bool, len(entries))

	for i := 0; i < len(entries); i++ {
		if used[i] {
			continue
		}

		current := entries[i]
		// Look for similar entries to merge
		for j := i + 1; j < len(entries); j++ {
			if used[j] {
				continue
			}
			if current.Category != entries[j].Category {
				continue
			}

			similarity := ContentSimilarity(current.Content, entries[j].Content)
			if similarity >= SimilarityThreshold {
				// Merge: keep the newer version with combined content
				current = MergeEntries(current, entries[j])
				used[j] = true
				merged++
			}
		}

		result = append(result, current)
	}

	return result, merged
}

// ContentSimilarity returns a similarity score between 0 and 1.
// Uses a combination of length ratio and common substring matching.
func ContentSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	// Normalize for comparison
	aNorm := strings.ToLower(strings.TrimSpace(a))
	bNorm := strings.ToLower(strings.TrimSpace(b))

	if aNorm == bNorm {
		return 1.0
	}

	// Length ratio
	lenRatio := float64(min(len(aNorm), len(bNorm))) / float64(max(len(aNorm), len(bNorm)))
	if lenRatio < 0.5 {
		return 0.0 // Too different in length
	}

	// Common words ratio
	aWords := strings.Fields(aNorm)
	bWords := strings.Fields(bNorm)

	if len(aWords) == 0 || len(bWords) == 0 {
		return lenRatio
	}

	// Count common words
	commonWords := 0
	bWordSet := make(map[string]bool)
	for _, w := range bWords {
		bWordSet[w] = true
	}
	for _, w := range aWords {
		if bWordSet[w] {
			commonWords++
		}
	}

	wordSimilarity := float64(commonWords*2) / float64(len(aWords)+len(bWords))

	// Combine length ratio and word similarity
	return (lenRatio + wordSimilarity) / 2.0
}

// MergeEntries combines two similar entries into one.
// The result has the newer timestamp and merged content.
func MergeEntries(a, b MemoryEntry) MemoryEntry {
	// Keep the newer entry as base
	newer := a
	older := b
	if b.Timestamp.After(a.Timestamp) {
		newer = b
		older = a
	}

	// If content is very similar, just keep the newer one
	if ContentSimilarity(a.Content, b.Content) > 0.95 {
		newer.Version++
		newer.PrevHash = older.ContentHash()
		newer.UpdatedAt = time.Now()
		return newer
	}

	// Otherwise, combine key information
	merged := newer
	merged.Version = max(a.Version, b.Version) + 1
	merged.PrevHash = newer.ContentHash()
	merged.UpdatedAt = time.Now()

	// Keep the longer, more detailed content
	if len(b.Content) > len(a.Content) {
		merged.Content = b.Content
	}

	return merged
}

// ─── Version Control ────────────────────────────────────────────────────────

// GetVersions returns the version history of an entry (if tracked).
// Since we only track current + prev hash, this returns limited history.
func (sm *SessionMemory) GetVersions(category, content string) []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var versions []MemoryEntry
	for _, e := range sm.entries {
		if e.Category == category && (e.Content == content || e.PrevHash != "") {
			versions = append(versions, e)
		}
	}
	return versions
}

// UpdateNote updates an existing note, creating a new version.
// Returns true if the note was found and updated.
func (sm *SessionMemory) UpdateNote(category, oldContent, newContent, source string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for i, e := range sm.entries {
		if e.Category == category && e.Content == oldContent {
			sm.entries[i] = MemoryEntry{
				Category:  category,
				Content:   newContent,
				Timestamp: e.Timestamp, // preserve original creation time
				Source:    source,
				Version:   e.Version + 1,
				PrevHash:  e.ContentHash(),
				UpdatedAt: time.Now(),
			}
			sm.dirty = true
			return true
		}
	}
	return false
}

// MergeDuplicates finds and merges duplicate entries across all scopes.
// Returns the number of entries merged.
func (sm *SessionMemory) MergeDuplicates() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	totalMerged := 0
	var merged int

	// Deduplicate session entries
	sm.entries, merged = DeduplicateEntries(sm.entries)
	totalMerged += merged

	// Deduplicate project entries
	sm.projectEntries, merged = DeduplicateEntries(sm.projectEntries)
	totalMerged += merged

	// Deduplicate global entries
	sm.globalEntries, merged = DeduplicateEntries(sm.globalEntries)
	totalMerged += merged

	if totalMerged > 0 {
		sm.dirty = true
	}

	return totalMerged
}

// GetStats returns statistics about the memory system.
func (sm *SessionMemory) GetStats() MemoryStats {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stats := MemoryStats{
		SessionEntries: len(sm.entries),
		ProjectEntries: len(sm.projectEntries),
		GlobalEntries:  len(sm.globalEntries),
		ByCategory:     make(map[string]int),
	}

	// Count by category
	for _, e := range sm.entries {
		stats.ByCategory[e.Category]++
	}
	for _, e := range sm.projectEntries {
		stats.ByCategory[e.Category]++
	}
	for _, e := range sm.globalEntries {
		stats.ByCategory[e.Category]++
	}

	// Count versions
	for _, e := range sm.entries {
		if e.Version > 1 {
			stats.VersionedEntries++
		}
	}

	return stats
}

// MemoryStats holds statistics about the memory system.
type MemoryStats struct {
	SessionEntries    int            `json:"session_entries"`
	ProjectEntries    int            `json:"project_entries"`
	GlobalEntries     int            `json:"global_entries"`
	ByCategory        map[string]int `json:"by_category"`
	VersionedEntries  int            `json:"versioned_entries"`
}

// maybeInvokeOnAdd invokes the onAdd callback if set (must hold lock).
func (sm *SessionMemory) maybeInvokeOnAdd() {
	if sm.onAdd != nil {
		sm.onAdd()
	}
}

// trimCategoryEntriesLocked removes oldest entries in a category to enforce max.
// Caller must hold sm.mu write lock.
func (sm *SessionMemory) trimCategoryEntriesLocked(category string) {
	max := maxEntriesForCategory(category)
	count := 0
	// Count entries in this category
	for _, e := range sm.entries {
		if e.Category == category {
			count++
		}
	}
	if count <= max {
		return
	}
	// Remove oldest entries in this category until count == max
	excess := count - max
	removed := 0
	result := make([]MemoryEntry, 0, len(sm.entries))
	for _, e := range sm.entries {
		if e.Category == category && removed < excess {
			removed++
			continue
		}
		result = append(result, e)
	}
	sm.entries = result
}

// GetNotes returns all memory entries from all three levels, sorted by category then timestamp.
func (sm *SessionMemory) GetNotes() []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Combine all three levels
	total := len(sm.globalEntries) + len(sm.projectEntries) + len(sm.entries)
	result := make([]MemoryEntry, 0, total)
	result = append(result, sm.globalEntries...)
	result = append(result, sm.projectEntries...)
	result = append(result, sm.entries...)

	sort.Slice(result, func(i, j int) bool {
		if result[i].Category != result[j].Category {
			return result[i].Category < result[j].Category
		}
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	return result
}

// SearchNotes returns memory entries whose content contains the query (case-insensitive).
// Searches across all three memory levels: global, project, and session.
func (sm *SessionMemory) SearchNotes(query string) []MemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	lower := strings.ToLower(query)
	var result []MemoryEntry

	// Search all three levels
	allEntries := [][]MemoryEntry{sm.globalEntries, sm.projectEntries, sm.entries}
	for _, entries := range allEntries {
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Content), lower) ||
				strings.Contains(strings.ToLower(e.Category), lower) {
				result = append(result, e)
			}
		}
	}
	return result
}

// removeExpiredEntries removes entries older than their category TTL.
// Must be called while holding write lock.
func (sm *SessionMemory) removeExpiredEntries() {
	before := len(sm.entries)
	sm.entries = filterEntries(sm.entries, func(e MemoryEntry) bool {
		// Expire all entries older than category TTL.
		// State entries are always cleared on session start, so they
		// shouldn't be here. But we expire other categories too.
		return !e.isExpired()
	})
	if len(sm.entries) < before {
		sm.dirty = true
	}
}

// filterEntries returns entries that match the predicate.
func filterEntries(entries []MemoryEntry, keep func(MemoryEntry) bool) []MemoryEntry {
	result := make([]MemoryEntry, 0, len(entries))
	for _, e := range entries {
		if keep(e) {
			result = append(result, e)
		}
	}
	return result
}

// FormatForPrompt formats memory entries for injection into the system prompt.
// Includes all three memory levels: global, project, and session.
func (sm *SessionMemory) FormatForPrompt() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	hasAny := len(sm.globalEntries) > 0 || len(sm.projectEntries) > 0 || len(sm.entries) > 0
	if !hasAny {
		return ""
	}

	var sb strings.Builder

	// Level 1: Global memory
	if len(sm.globalEntries) > 0 {
		sb.WriteString("## Global Memory (cross-project preferences)\n\n")
		groups := groupByCategory(sm.globalEntries)
		for _, cat := range sortedKeys(groups) {
			sb.WriteString(fmt.Sprintf("### %s\n", cat))
			for _, e := range groups[cat] {
				sb.WriteString(fmt.Sprintf("- %s\n", e.Content))
			}
			sb.WriteString("\n")
		}
	}

	// Level 2: Project memory
	if len(sm.projectEntries) > 0 {
		sb.WriteString("## Project Memory (project rules and architecture)\n\n")
		groups := groupByCategory(sm.projectEntries)
		for _, cat := range sortedKeys(groups) {
			sb.WriteString(fmt.Sprintf("### %s\n", cat))
			for _, e := range groups[cat] {
				sb.WriteString(fmt.Sprintf("- %s\n", e.Content))
			}
			sb.WriteString("\n")
		}
	}

	// Level 3: Session memory
	if len(sm.entries) > 0 {
		sb.WriteString("## Session Memory\n\n")
		sb.WriteString("The following notes were recorded during this or previous sessions. Use them as context.\n\n")
		groups := groupByCategory(sm.entries)
		for _, cat := range sortedKeys(groups) {
			sb.WriteString(fmt.Sprintf("### %s\n", cat))
			for _, e := range groups[cat] {
				sb.WriteString(fmt.Sprintf("- %s\n", e.Content))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// groupByCategory groups entries by category.
func groupByCategory(entries []MemoryEntry) map[string][]MemoryEntry {
	groups := make(map[string][]MemoryEntry)
	for _, e := range entries {
		groups[e.Category] = append(groups[e.Category], e)
	}
	return groups
}

// sortedKeys returns sorted keys from a map.
func sortedKeys(m map[string][]MemoryEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// GetLastSummarizedMessageUUID returns the UUID of the most recently summarized
// message for incremental SM-compact. Returns "" if no compaction has occurred.
func (s *SessionMemory) GetLastSummarizedMessageUUID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastSummarizedMessageUUID
}

// SetLastSummarizedMessageUUID sets the UUID of the most recently summarized
// message for incremental SM-compact.
func (s *SessionMemory) SetLastSummarizedMessageUUID(uuid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastSummarizedMessageUUID = uuid
	s.dirty = true
}

// FormatForPromptCompact formats memory entries for injection after compaction.
// Includes all three memory levels (global, project, session) with separate budgets.
// MiMo-Code pattern: global (6K tokens) + project (10K tokens) + session (10K tokens).
func (sm *SessionMemory) FormatForPromptCompact() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	hasAny := len(sm.globalEntries) > 0 || len(sm.projectEntries) > 0 || len(sm.entries) > 0
	if !hasAny {
		return ""
	}

	var sb strings.Builder

	// Level 1: Global memory (cross-project preferences)
	if len(sm.globalEntries) > 0 {
		sb.WriteString("## Global Memory (cross-project preferences)\n\n")
		formatEntriesInto(&sb, sm.globalEntries, maxGlobalMemoryTokens*4, maxTokensPerSection*4)
		sb.WriteString("\n")
	}

	// Level 2: Project memory (project-level rules and architecture)
	if len(sm.projectEntries) > 0 {
		sb.WriteString("## Project Memory (project rules and architecture)\n\n")
		formatEntriesInto(&sb, sm.projectEntries, maxProjectMemoryTokens*4, maxTokensPerSection*4)
		sb.WriteString("\n")
	}

	// Level 3: Session memory (per-session state)
	if len(sm.entries) > 0 {
		sb.WriteString("## Session Memory\n\n")
		sb.WriteString("The following notes were recorded during this or previous sessions. Use them as context.\n\n")
		formatEntriesInto(&sb, sm.entries, maxTotalSessionMemoryTokens*4, maxTokensPerSection*4)
	}

	return sb.String()
}

// formatEntriesInto writes entries grouped by category into sb with budget limits.
func formatEntriesInto(sb *strings.Builder, entries []MemoryEntry, totalBudget, maxSectionChars int) {
	// Group by category
	groups := make(map[string][]MemoryEntry)
	var categories []string
	for _, e := range entries {
		if _, ok := groups[e.Category]; !ok {
			categories = append(categories, e.Category)
		}
		groups[e.Category] = append(groups[e.Category], e)
	}
	sort.Strings(categories)

	totalUsed := 0
	for _, cat := range categories {
		entryList := groups[cat]
		sectionHeader := fmt.Sprintf("### %s\n", cat)
		sb.WriteString(sectionHeader)
		sectionUsed := len(sectionHeader)

		for _, e := range entryList {
			line := fmt.Sprintf("- %s\n", e.Content)
			lineLen := len(line)
			if totalUsed+lineLen > totalBudget {
				break
			}
			if sectionUsed+lineLen > maxSectionChars {
				remaining := maxSectionChars - sectionUsed - len("  [... truncated ...]\n")
				if remaining > 0 {
					truncated := truncateLine(line, remaining)
					sb.WriteString(truncated)
					sb.WriteString("  [... truncated ...]\n")
				}
				break
			}
			sb.WriteString(line)
			sectionUsed += lineLen
			totalUsed += lineLen
		}
		sb.WriteString("\n")
	}
}

// truncateLine truncates a line to fit within remaining budget.
// It finds a good break point (sentence boundary, line boundary, or char limit).
func truncateLine(line string, maxLen int) string {
	if len(line) <= maxLen {
		return line
	}
	// Try sentence boundary (. )
	if idx := strings.LastIndex(line[:maxLen], ". "); idx > 0 {
		return line[:idx+1] + "\n"
	}
	// Try newline
	if idx := strings.LastIndex(line[:maxLen], "\n"); idx > 0 {
		return line[:idx] + "\n"
	}
	return line[:maxLen] + "\n"
}

// LoadSessionMemoryTemplate returns the default session memory template.
func LoadSessionMemoryTemplate() string {
	return defaultSessionMemoryTemplate
}

// IsSessionMemoryTemplateOnly checks if the given content is essentially just the
// default template (no user-written content). This is used to detect whether
// session memory has actual extracted content or is just the empty template.
// Matches upstream's isSessionMemoryEmpty() in prompts.ts.
func IsSessionMemoryTemplateOnly(content string) bool {
	return strings.TrimSpace(content) == strings.TrimSpace(defaultSessionMemoryTemplate)
}

// truncateSessionMemoryForCompact truncates session memory sections for inclusion
// in a compact summary. Used when session memory is too large to fit in the
// post-compact token budget. Matches upstream's truncateSessionMemoryForCompact
// in prompts.ts.
//
// Per-section truncation keeps section headers intact while limiting content.
// maxTokens is the maximum token budget for the entire session memory content
// (upstream uses 40,000 for SM-compact).
func truncateSessionMemoryForCompact(content string, maxTokens int) string {
	const maxSectionTokens = 2000 // per-section limit matching FormatForPromptCompact
	const maxCharsPerSection = maxSectionTokens * 4

	lines := strings.Split(content, "\n")
	var outputLines []string
	var currentSectionLines []string
	currentSectionHeader := ""

	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			// Flush previous section
			outputLines = append(outputLines, flushSessionSectionForCompact(currentSectionHeader, currentSectionLines, maxCharsPerSection)...)
			currentSectionHeader = line
			currentSectionLines = nil
		} else {
			currentSectionLines = append(currentSectionLines, line)
		}
	}
	// Flush last section
	outputLines = append(outputLines, flushSessionSectionForCompact(currentSectionHeader, currentSectionLines, maxCharsPerSection)...)

	truncated := strings.Join(outputLines, "\n")

	// Global truncation: if still over budget, truncate at the end
	if EstimateTokens(truncated) > maxTokens {
		overallLimit := maxTokens * 4
		if len(truncated) > overallLimit {
			truncated = truncated[:overallLimit]
			if idx := strings.LastIndex(truncated, "\n"); idx > 0 {
				truncated = truncated[:idx]
			}
			truncated += "\n\n[... session memory truncated for length. Read the full session memory file for details ...]"
		}
	}
	return truncated
}

func flushSessionSectionForCompact(header string, lines []string, maxCharsPerSection int) []string {
	if header == "" {
		return lines
	}
	result := []string{header}
	charCount := 0
	for _, line := range lines {
		if charCount+len(line)+1 > maxCharsPerSection {
			result = append(result, "\n[... section truncated for length ...]")
			return result
		}
		result = append(result, line)
		charCount += len(line) + 1
	}
	return result
}

// FormatForTemplate returns the current session memory formatted as a markdown
// file, preserving the template structure (headers and descriptions).
// Uses the structured template format matching upstream.
func (sm *SessionMemory) FormatForTemplate() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.formatForTemplateLocked()
}

// formatForTemplateLocked formats session memory as the 10-section template.
// Caller must hold sm.mu (read or write lock).
func (sm *SessionMemory) formatForTemplateLocked() string {
	if len(sm.entries) == 0 {
		return defaultSessionMemoryTemplate
	}

	// Group entries by category into a simple map
	sectionContent := make(map[string][]string)
	for _, e := range sm.entries {
		sectionContent[e.Category] = append(sectionContent[e.Category], e.Content)
	}

	// Build structured output based on template sections
	var sb strings.Builder

	// Session Title
	sb.WriteString("# Session Title\n")
	sb.WriteString("_A short and distinctive 5-10 word descriptive title. Super info dense, no filler_\n")
	if items, ok := sectionContent["state"]; ok {
		// Use first state entry as a hint for the title
		sb.WriteString(items[0])
		if len(items) > 1 {
			for _, item := range items[1:] {
				if len(sb.String()) < 200 {
					sb.WriteString(" | " + item)
				}
			}
		}
	}
	sb.WriteString("\n\n")

	// Current State
	sb.WriteString("# Current State\n")
	sb.WriteString("_What is actively being worked on right now? Pending tasks not yet completed. Immediate next steps._\n")
	for _, item := range sectionContent["state"] {
		sb.WriteString("- " + item + "\n")
	}
	sb.WriteString("\n")

	// Task Specification (use decision entries)
	sb.WriteString("# Task Specification\n")
	sb.WriteString("_What did the user ask to build? Any design decisions or explanatory context._\n")
	for _, item := range sectionContent["decision"] {
		sb.WriteString("- " + item + "\n")
	}
	sb.WriteString("\n")

	// Files and Functions (use reference entries)
	sb.WriteString("# Files and Functions\n")
	sb.WriteString("_What are the important files? What do they contain and why are they relevant?_\n")
	for _, item := range sectionContent["reference"] {
		sb.WriteString("- " + item + "\n")
	}
	sb.WriteString("\n")

	// Workflow (use reference entries that mention commands/workflows)
	sb.WriteString("# Workflow\n")
	sb.WriteString("_What bash commands are usually run and in what order? How to interpret their output?_\n")
	for _, item := range sectionContent["reference"] {
		if isWorkflowItem(item) {
			sb.WriteString("- " + item + "\n")
		}
	}
	sb.WriteString("\n")

	// Errors & Corrections (use error entries + decision entries mentioning errors)
	sb.WriteString("# Errors & Corrections\n")
	sb.WriteString("_Errors encountered and how they were fixed. What did the user correct? What approaches failed?_\n")
	for _, item := range sectionContent["error"] {
		sb.WriteString("- " + item + "\n")
	}
	for _, item := range sectionContent["decision"] {
		if isErrorRelated(item) {
			sb.WriteString("- " + item + "\n")
		}
	}
	sb.WriteString("\n")

	// Codebase and System Documentation (use reference entries about architecture)
	sb.WriteString("# Codebase and System Documentation\n")
	sb.WriteString("_What are the important system components? How do they work/fit together?_\n")
	for _, item := range sectionContent["reference"] {
		if isArchitectureItem(item) {
			sb.WriteString("- " + item + "\n")
		}
	}
	sb.WriteString("\n")

	// Learnings (use preference entries)
	sb.WriteString("# Learnings\n")
	sb.WriteString("_What has worked well? What has not? What to avoid? Do not duplicate items from other sections._\n")
	for _, item := range sectionContent["preference"] {
		sb.WriteString("- " + item + "\n")
	}
	sb.WriteString("\n")

	// Key Results (use result entries)
	sb.WriteString("# Key Results\n")
	sb.WriteString("_If the user asked for a specific output (answer, table, document), repeat the exact result here._\n")
	for _, item := range sectionContent["result"] {
		sb.WriteString("- " + item + "\n")
	}
	sb.WriteString("\n")

	// Worklog (use worklog entries)
	sb.WriteString("# Worklog\n")
	sb.WriteString("_Step by step, what was attempted and done? Very terse summary for each step._\n")
	for _, item := range sectionContent["worklog"] {
		sb.WriteString("- " + item + "\n")
	}
	sb.WriteString("\n")

	return sb.String()
}

// loadFromDisk reads memory entries from the session memory file.
// Parses both the structured template format (upstream-compatible) and the
// simple list format (legacy).
func (sm *SessionMemory) loadFromDisk() {
	data, err := os.ReadFile(sm.filePath)
	if err != nil {
		return // no file yet
	}

	sm.entries = sm.parseMarkdownEntries(string(data))
}

// loadGlobalFromDisk reads global memory entries from ~/.claude/memory/global.md.
// Global memory contains cross-project user preferences that persist across all projects.
func (sm *SessionMemory) loadGlobalFromDisk() {
	if sm.globalPath == "" {
		return
	}
	data, err := os.ReadFile(sm.globalPath)
	if err != nil {
		return // no file yet
	}
	sm.globalEntries = sm.parseSimpleEntries(string(data))
}

// loadProjectFromDisk reads project memory entries from {projectDir}/.claude/memory/project.md.
// Project memory contains project-level rules, architecture decisions, and durable knowledge.
func (sm *SessionMemory) loadProjectFromDisk() {
	data, err := os.ReadFile(sm.projectPath)
	if err != nil {
		return // no file yet
	}
	sm.projectEntries = sm.parseSimpleEntries(string(data))
}

// parseSimpleEntries parses entries from a simple markdown list format.
// Format: "## Category\n- content\n- content\n\n## Category2\n..."
func (sm *SessionMemory) parseSimpleEntries(data string) []MemoryEntry {
	var entries []MemoryEntry
	lines := strings.Split(data, "\n")
	var currentCategory string
	lastTimestamp := time.Now()

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Category header: ## Category
		if strings.HasPrefix(trimmed, "## ") {
			currentCategory = strings.TrimSpace(trimmed[3:])
			continue
		}

		// Timestamp comment: <!-- timestamp -->
		if strings.HasPrefix(trimmed, "<!-- ") && strings.HasSuffix(trimmed, " -->") {
			ts := strings.TrimPrefix(trimmed, "<!-- ")
			ts = strings.TrimSuffix(ts, " -->")
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				lastTimestamp = t
			}
			continue
		}

		// Bullet point: - content
		if strings.HasPrefix(trimmed, "- ") && currentCategory != "" {
			content := strings.TrimSpace(trimmed[2:])
			if content == "" {
				continue
			}
			entries = append(entries, MemoryEntry{
				Category:  currentCategory,
				Content:   content,
				Timestamp: lastTimestamp,
				Source:    "disk",
			})
		}
	}

	return entries
}

// parseMarkdownEntries parses entries from a markdown session memory file.
// Handles both structured template format (with section headers like "# Section")
// and simple list format (with "### Category" headers).
func (sm *SessionMemory) parseMarkdownEntries(data string) []MemoryEntry {
	var entries []MemoryEntry
	lines := strings.Split(data, "\n")
	var currentCategory string
	lastTimestamp := time.Now() // default for entries without explicit timestamp

	for _, line := range lines {
		// Structured template section (upstream format): # Section Title
		if strings.HasPrefix(line, "# ") {
			// Map template sections to categories
			lower := strings.ToLower(strings.TrimSpace(line[2:]))
			switch {
			case strings.Contains(lower, "current state"):
				currentCategory = "state"
			case strings.Contains(lower, "task spec"):
				currentCategory = "decision"
			case strings.Contains(lower, "files"):
				currentCategory = "reference"
			case strings.Contains(lower, "workflow"):
				currentCategory = "reference"
			case strings.Contains(lower, "error"):
				currentCategory = "decision"
			case strings.Contains(lower, "learn"):
				currentCategory = "preference"
			case strings.Contains(lower, "key result"):
				currentCategory = "reference"
			case strings.Contains(lower, "worklog"):
				currentCategory = "reference"
			case strings.Contains(lower, "codebase"):
				currentCategory = "reference"
			case strings.Contains(lower, "session title"):
				currentCategory = "state"
			default:
				currentCategory = ""
			}
			continue
		}

		// Simple list category header (legacy format): ### Category
		if strings.HasPrefix(line, "### ") {
			currentCategory = strings.TrimSpace(strings.TrimPrefix(line, "### "))
			continue
		}

		// Template description line (italic, starts with "_"): skip
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 0 && trimmed[0] == '_' && strings.HasSuffix(trimmed, "_") {
			continue // description line, skip
		}

		// Timestamp comment: <!-- timestamp -->
		if strings.HasPrefix(line, "<!-- ") && strings.HasSuffix(line, " -->") {
			ts := strings.TrimPrefix(line, "<!-- ")
			ts = strings.TrimSuffix(ts, " -->")
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				lastTimestamp = t
			}
			continue
		}

		// Bullet point: - content
		if strings.HasPrefix(line, "- ") && currentCategory != "" {
			content := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if content == "" {
				continue
			}
			entries = append(entries, MemoryEntry{
				Category:  currentCategory,
				Content:   content,
				Timestamp: lastTimestamp,
				Source:    "disk",
			})
		}
	}

	return entries
}

// ClearStateEntries removes all entries in the "state" category.
// Called at session start to prevent stale session context from
// previous sessions from bleeding in.
func (sm *SessionMemory) ClearStateEntries() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	before := len(sm.entries)
	sm.entries = filterEntries(sm.entries, func(e MemoryEntry) bool {
		return e.Category != "state"
	})
	if len(sm.entries) < before {
		sm.dirty = true
	}
}

// SaveConclusions appends conclusion entries to session memory, categorizing
// them based on content type. Called before compaction so the agent's
// accumulated work knowledge is preserved across compaction.
//
// Categories:
//   - error: contains error/fix/bug/fail/issue keywords
//   - result: contains result/output/completed/answer/created keywords
//   - worklog: contains action verbs describing what was done
//   - state: everything else (task progress, current work)
func (sm *SessionMemory) SaveConclusions(conclusions []string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if len(conclusions) == 0 {
		return
	}

	for _, c := range conclusions {
		if c == "" {
			continue
		}
		// Determine category based on content keywords
		category := categorizeConclusion(c)

		// Check if this conclusion already exists in the same category
		exists := false
		for _, e := range sm.entries {
			if e.Category == category && e.Content == c {
				exists = true
				break
			}
		}
		if !exists {
			sm.entries = append(sm.entries, MemoryEntry{
				Category:  category,
				Content:   c,
				Timestamp: time.Now(),
				Source:    "auto",
			})
		}
	}

	// Enforce max entries per category
	sm.trimCategoryEntriesLocked("state")
	sm.trimCategoryEntriesLocked("error")
	sm.trimCategoryEntriesLocked("result")
	sm.trimCategoryEntriesLocked("worklog")

	sm.dirty = true
}

// categorizeConclusion determines which category a conclusion belongs to
// based on its content keywords.
func categorizeConclusion(c string) string {
	lower := strings.ToLower(c)

	// Error-related keywords -> error category
	if strings.Contains(lower, "error") ||
		strings.Contains(lower, "fix:") ||
		strings.Contains(lower, "fixed") ||
		strings.Contains(lower, "bug:") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "failure") ||
		strings.Contains(lower, "issue:") ||
		strings.Contains(lower, "broken") ||
		strings.Contains(lower, "incorrect") ||
		strings.Contains(lower, "wrong") {
		return "error"
	}

	// Result-related keywords -> result category
	if strings.Contains(lower, "result:") ||
		strings.Contains(lower, "output:") ||
		strings.Contains(lower, "completed") ||
		strings.Contains(lower, "created:") ||
		strings.Contains(lower, "generated:") ||
		strings.Contains(lower, "answer:") ||
		strings.Contains(lower, "summary:") ||
		strings.Contains(lower, "key finding") ||
		strings.Contains(lower, "discovered:") {
		return "result"
	}

	// Worklog-style entries (action verbs at start) -> worklog category
	// These typically describe what was done in a turn
	if strings.HasPrefix(lower, "added ") ||
		strings.HasPrefix(lower, "created ") ||
		strings.HasPrefix(lower, "implemented ") ||
		strings.HasPrefix(lower, "fixed ") ||
		strings.HasPrefix(lower, "updated ") ||
		strings.HasPrefix(lower, "modified ") ||
		strings.HasPrefix(lower, "removed ") ||
		strings.HasPrefix(lower, "refactored ") ||
		strings.HasPrefix(lower, "wrote ") ||
		strings.HasPrefix(lower, "ran ") ||
		strings.HasPrefix(lower, "tested ") ||
		strings.HasPrefix(lower, "built ") ||
		strings.HasPrefix(lower, "deployed ") {
		return "worklog"
	}

	// Default: state category for task progress, current work, etc.
	return "state"
}

// ─── Phase 4: User Instructions & Discoveries Extraction ────────────────────

// ExtractedItem represents an extracted item from conversation history.
type ExtractedItem struct {
	Type     string // "instruction" | "discovery" | "constraint" | "preference"
	Content  string
	Source   string // "user" | "assistant"
	Category string // memory category
}

// ExtractFromMessages analyzes conversation messages and extracts user instructions
// and discoveries. This is the Phase 4 enhancement for better context preservation.
func ExtractFromMessages(messages []ConversationMessage) []ExtractedItem {
	var items []ExtractedItem

	for i, msg := range messages {
		switch msg.Role {
		case "user":
			items = append(items, extractUserInstructions(msg.Content, i)...)
		case "assistant":
			items = append(items, extractDiscoveries(msg.Content, i)...)
		}
	}

	return deduplicateItems(items)
}

// ConversationMessage represents a message in the conversation.
type ConversationMessage struct {
	Role    string
	Content string
}

// extractUserInstructions extracts explicit user requests, constraints, and preferences.
func extractUserInstructions(content string, msgIndex int) []ExtractedItem {
	var items []ExtractedItem
	lower := strings.ToLower(content)

	// Explicit instruction patterns
	instructionPatterns := []struct {
		patterns []string
		category string
	}{
		{[]string{"please", "make sure", "ensure that", "don't forget", "remember to"}, "instruction"},
		{[]string{"must", "should", "need to", "have to", "required"}, "constraint"},
		{[]string{"prefer", "i like", "i want", "i'd like", "would prefer"}, "preference"},
		{[]string{"use ", "use the", "use a"}, "instruction"},
		{[]string{"don't", "do not", "avoid", "never", "stop"}, "constraint"},
		{[]string{"fix", "bug", "issue", "problem", "error"}, "instruction"},
		{[]string{"add", "create", "implement", "build", "write"}, "instruction"},
		{[]string{"change", "update", "modify", "refactor", "improve"}, "instruction"},
	}

	for _, p := range instructionPatterns {
		for _, pattern := range p.patterns {
			if strings.Contains(lower, pattern) {
				// Extract the sentence containing the pattern
				sentence := extractSentence(content, pattern)
				if sentence != "" && len(sentence) > 10 && len(sentence) < 500 {
					items = append(items, ExtractedItem{
						Type:     p.category,
						Content:  sentence,
						Source:   "user",
						Category: "instruction",
					})
					break // One pattern per message
				}
			}
		}
	}

	return items
}

// extractDiscoveries extracts technical findings and solutions from assistant messages.
func extractDiscoveries(content string, msgIndex int) []ExtractedItem {
	var items []ExtractedItem
	lower := strings.ToLower(content)

	// Discovery patterns
	discoveryPatterns := []struct {
		patterns []string
		category string
	}{
		{[]string{"found that", "discovered", "turns out", "it appears", "the issue is"}, "discovery"},
		{[]string{"the fix", "the solution", "to fix this", "the problem was"}, "discovery"},
		{[]string{"important:", "note:", "warning:", "caution:", "be careful"}, "discovery"},
		{[]string{"this works because", "the reason", "this is because"}, "discovery"},
		{[]string{"pattern:", "approach:", "strategy:", "technique:"}, "discovery"},
		{[]string{"best practice", "recommended", "should use", "prefer using"}, "discovery"},
		{[]string{"error:", "failed:", "broken:", "issue:"}, "error"},
		{[]string{"fixed:", "resolved:", "solved:", "working now"}, "result"},
	}

	for _, p := range discoveryPatterns {
		for _, pattern := range p.patterns {
			if strings.Contains(lower, pattern) {
				sentence := extractSentence(content, pattern)
				if sentence != "" && len(sentence) > 10 && len(sentence) < 500 {
					items = append(items, ExtractedItem{
						Type:     "discovery",
						Content:  sentence,
						Source:   "assistant",
						Category: p.category,
					})
					break
				}
			}
		}
	}

	return items
}

// extractSentence extracts the sentence containing a pattern.
func extractSentence(content, pattern string) string {
	lower := strings.ToLower(content)
	idx := strings.Index(lower, pattern)
	if idx < 0 {
		return ""
	}

	// Find sentence boundaries
	start := idx
	for start > 0 && content[start-1] != '\n' && content[start-1] != '.' && content[start-1] != '!' && content[start-1] != '?' {
		start--
	}

	end := idx + len(pattern)
	for end < len(content) && content[end] != '\n' && content[end] != '.' && content[end] != '!' && content[end] != '?' {
		end++
	}

	sentence := strings.TrimSpace(content[start:end])
	// Clean up markdown
	sentence = strings.TrimPrefix(sentence, "- ")
	sentence = strings.TrimPrefix(sentence, "* ")
	sentence = strings.TrimPrefix(sentence, "> ")

	return sentence
}

// deduplicateItems removes duplicate items based on content similarity.
func deduplicateItems(items []ExtractedItem) []ExtractedItem {
	seen := make(map[string]bool)
	var result []ExtractedItem

	for _, item := range items {
		// Normalize for dedup
		key := strings.ToLower(strings.TrimSpace(item.Content))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}

	return result
}

// SaveExtractedItems saves extracted items to session memory.
func (sm *SessionMemory) SaveExtractedItems(items []ExtractedItem) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if len(items) == 0 {
		return
	}

	for _, item := range items {
		// Check if already exists
		exists := false
		for _, e := range sm.entries {
			if e.Category == item.Category && e.Content == item.Content {
				exists = true
				break
			}
		}
		if !exists {
			sm.entries = append(sm.entries, MemoryEntry{
				Category:  item.Category,
				Content:   item.Content,
				Timestamp: time.Now(),
				Source:    item.Source,
			})
		}
	}

	// Enforce max entries
	sm.trimCategoryEntriesLocked("instruction")
	sm.trimCategoryEntriesLocked("constraint")
	sm.trimCategoryEntriesLocked("preference")
	sm.trimCategoryEntriesLocked("discovery")

	sm.dirty = true
}

// ExtractAndSave is a convenience function that extracts and saves in one call.
func (sm *SessionMemory) ExtractAndSave(messages []ConversationMessage) int {
	items := ExtractFromMessages(messages)
	sm.SaveExtractedItems(items)
	return len(items)
}

// FlushToDisk writes memory entries to disk if dirty.
// Uses file locking to prevent corruption from concurrent writes.
func (sm *SessionMemory) FlushToDisk() error {
	return sm.flushToDisk()
}

func (sm *SessionMemory) flushToDisk() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Flush global memory if dirty
	if sm.globalDirty && sm.globalPath != "" {
		if err := sm.writeSimpleEntriesLocked(sm.globalPath, sm.globalEntries); err != nil {
			fmt.Fprintf(os.Stderr, "[memory] flush global error: %v\n", err)
		} else {
			sm.globalDirty = false
		}
	}

	// Flush project memory if dirty
	if sm.projectDirty {
		if err := sm.writeSimpleEntriesLocked(sm.projectPath, sm.projectEntries); err != nil {
			fmt.Fprintf(os.Stderr, "[memory] flush project error: %v\n", err)
		} else {
			sm.projectDirty = false
		}
	}

	// Flush session memory if dirty
	if !sm.dirty {
		return nil
	}

	if err := sm.writeAllEntriesLocked(); err != nil {
		return err
	}
	sm.dirty = false
	return nil
}

// writeSimpleEntriesLocked writes entries in simple "## Category\n- content" format.
// Used for global and project memory files.
func (sm *SessionMemory) writeSimpleEntriesLocked(path string, entries []MemoryEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}

	// Group by category
	groups := make(map[string][]MemoryEntry)
	var categories []string
	for _, e := range entries {
		if _, ok := groups[e.Category]; !ok {
			categories = append(categories, e.Category)
		}
		groups[e.Category] = append(groups[e.Category], e)
	}
	sort.Strings(categories)

	var sb strings.Builder
	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("## %s\n", cat))
		for _, e := range groups[cat] {
			sb.WriteString(fmt.Sprintf("- %s\n", e.Content))
			sb.WriteString(fmt.Sprintf("<!-- %s -->\n", e.Timestamp.Format(time.RFC3339)))
		}
		sb.WriteString("\n")
	}

	// Atomic write
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("write memory file tmp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename memory file: %w", err)
	}
	return nil
}

// writeAllEntriesLocked writes all entries to disk. Caller must hold write lock.
func (sm *SessionMemory) writeAllEntriesLocked() error {
	// Ensure directory exists
	dir := filepath.Dir(sm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}

	// Write in the 10-section template format (matching what the forked agent
	// sees/edits and what compaction reads). This ensures the disk file is the
	// single source of truth, matching upstream's behavior.
	content := sm.formatForTemplateLocked()

	// Atomic write: write to temp file in same directory, then rename.
	// This avoids locking issues on Windows (syscall.LockFileEx crashes on Go 1.23+)
	// and is safe for single-process access.
	tmpPath := sm.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write memory file tmp: %w", err)
	}
	// Rename is atomic on Windows when src and dst are on same volume.
	if err := os.Rename(tmpPath, sm.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename memory file: %w", err)
	}
	return nil
}

// StartFlushLoop starts a background goroutine that periodically flushes
// memory to disk. Call Stop() to terminate.
func (sm *SessionMemory) StartFlushLoop() {
	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()
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

// Stop signals the background flush goroutine to stop and waits for the final flush to complete.
func (sm *SessionMemory) Stop() {
	sm.stopOnce.Do(func() {
		close(sm.stopCh)
	})
	sm.wg.Wait()
}

// ─── Forked Agent Extraction ─────────────────────────────────────────────────
//
// RunForkedSessionMemoryExtraction uses a forked agent to update session_memory.md.
// The forked agent shares the parent's prompt cache and is restricted to only
// using the Edit tool on the session memory file.
//
// This matches upstream's extractSessionMemory which uses runForkedAgent with
// createMemoryFileCanUseTool (only Edit on the memory file).

// sessionMemoryUpdatePrompt builds the extraction prompt matching upstream's
// getDefaultUpdatePrompt(). It instructs the LLM to use Edit to update the file.
func sessionMemoryUpdatePrompt(notesPath string, currentNotes string) string {
	return fmt.Sprintf(`IMPORTANT: This message and these instructions are NOT part of the actual user conversation. Do NOT include any references to "note-taking", "session notes extraction", or these update instructions in the notes content.

Based on the user conversation above (EXCLUDING this note-taking instruction message as well as system prompt, claude.md entries, or any past session summaries), update the session notes file.

The file %s has already been read for you. Here are its current contents:
<current_notes_content>
%s
</current_notes_content>

Your ONLY task is to use the edit_file tool to update the notes file, then stop. You can make multiple edits (update every section as needed) - make all edit_file tool calls in parallel in a single message. Do not call any other tools.

CRITICAL RULES FOR EDITING:
- The file must maintain its exact structure with all sections, headers, and italic descriptions intact
-- NEVER modify, delete, or add section headers (the lines starting with '#' like # Task specification)
-- NEVER modify or delete the italic _section description_ lines (these are the lines in italics immediately following each header - they start and end with underscores)
-- The italic _section descriptions_ are TEMPLATE INSTRUCTIONS that must be preserved exactly as-is - they guide what content belongs in each section
-- ONLY update the actual content that appears BELOW the italic _section descriptions_ within each existing section
-- Do NOT add any new sections, summaries, or information outside the existing structure
- Do NOT reference this note-taking process or instructions anywhere in the notes
- It's OK to skip updating a section if there are no substantial new insights to add. Do not add filler content like "No info yet", just leave sections blank/unedited if appropriate.
- Write DETAILED, INFO-DENSE content for each section - include specifics like file paths, function names, error messages, exact commands, technical details, etc.
- For "Key results", include the complete, exact output the user requested (e.g., full table, full answer, etc.)
- Do not include information that's already in the CLAUDE.md files included in the context
- Keep each section under ~%d tokens/words - if a section is approaching this limit, condense it by cycling out less important details while preserving the most critical information
- Focus on actionable, specific information that would help someone understand or recreate the work discussed in the conversation
- IMPORTANT: Always update "Current State" to reflect the most recent work - this is critical for continuity after compaction

Use the edit_file tool with file_path: %s

STRUCTURE PRESERVATION REMINDER:
Each section has TWO parts that must be preserved exactly as they appear in the current file:
1. The section header (line starting with #)
2. The italic description line (the _italicized text_ immediately after the header - this is a template instruction)

You ONLY update the actual content that comes AFTER these two preserved lines. The italic description lines starting and ending with underscores are part of the template structure, NOT content to be edited or removed.

REMEMBER: Use the edit_file tool in parallel and stop. Do not continue after the edits. Only include insights from the actual user conversation, never from these note-taking instructions. Do not delete or change section headers or italic _section descriptions_.`,
		notesPath, currentNotes, maxTokensPerSection, notesPath)
}

// createMemoryFileCanUseTool returns a CanUseToolFn that only allows
// edit_file on the session memory file. All other tools are denied.
// This matches upstream's createMemoryFileCanUseTool.
func createMemoryFileCanUseTool(memoryPath string) CanUseToolFn {
	// Normalize path for comparison
	normalizedPath := filepath.Clean(memoryPath)
	return func(toolName string, args map[string]any) (bool, string) {
		if toolName != "edit_file" && toolName != "multi_edit" {
			return false, fmt.Sprintf("only edit_file/multi_edit on session memory file allowed in extraction mode (got %s)", toolName)
		}
		// Check that the file_path matches the session memory file
		if fp, ok := args["file_path"].(string); ok {
			if filepath.Clean(fp) != normalizedPath {
				return false, fmt.Sprintf("can only edit session memory file %s, not %s", normalizedPath, fp)
			}
			return true, ""
		}
		return false, "file_path argument missing"
	}
}

// ─── Extraction Thresholds ───────────────────────────────────────────────────
//
// Raised from upstream defaults to reduce forked agent API calls.
//   - minimumMessageTokensToInit: 20000 (delay first extraction until more context)
//   - minimumTokensBetweenUpdate: 10000 (reduce extraction frequency)
//   - toolCallsBetweenUpdates: 3 (minimum tool calls between updates)

const (
	minimumMessageTokensToInit = 20000
	minimumTokensBetweenUpdate = 10000
	toolCallsBetweenUpdates    = 3
)

// ExtractionState tracks when the next extraction should happen.
type ExtractionState struct {
	mu                  sync.Mutex
	initialized         bool
	tokensAtLastExtract int64
	toolCallsSinceLast  int
	// extractionInProgress is set to true when a goroutine extraction is running
	// and false when it completes. SM-compact waits for this to be false before
	// proceeding, so it uses the freshest session memory content.
	extractionInProgress bool
	extractionStartedAt  time.Time // timestamp when extraction started (for staleness check)
}

// NewExtractionState creates a new extraction state tracker.
func NewExtractionState() *ExtractionState {
	return &ExtractionState{}
}

// ShouldExtract checks if the extraction thresholds have been met.
// Matches upstream: token threshold AND (tool call threshold OR no tool calls in last turn).
func (es *ExtractionState) ShouldExtract(currentTokens int64, hasToolCallsInLastTurn bool) bool {
	es.mu.Lock()
	defer es.mu.Unlock()

	if !es.initialized {
		if currentTokens >= int64(minimumMessageTokensToInit) {
			return true
		}
		return false
	}

	tokensSinceLast := currentTokens - es.tokensAtLastExtract
	hasMetTokenThreshold := tokensSinceLast >= int64(minimumTokensBetweenUpdate)
	hasMetToolCallThreshold := es.toolCallsSinceLast >= toolCallsBetweenUpdates
	if hasMetTokenThreshold && (hasMetToolCallThreshold || !hasToolCallsInLastTurn) {
		return true
	}
	return false
}

// MarkExtracted records that an extraction was performed.
func (es *ExtractionState) MarkExtracted(currentTokens int64) {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.initialized = true
	es.tokensAtLastExtract = currentTokens
	es.toolCallsSinceLast = 0
	es.extractionInProgress = false
}

// IncrementToolCall increments the tool call counter.
func (es *ExtractionState) IncrementToolCall() {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.toolCallsSinceLast++
}

// MarkExtractionInProgress signals that extraction has started.
func (es *ExtractionState) MarkExtractionInProgress() {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.extractionInProgress = true
	es.extractionStartedAt = time.Now()
}

// WaitForExtraction waits (with timeout) for any in-progress extraction to
// complete. Returns immediately if extraction is stale (> 60s old, assumed abandoned).
// Returns true if extraction completed, false if timed out.
// This matches upstream's waitForSessionMemoryExtraction().
func (es *ExtractionState) WaitForExtraction(timeout time.Duration) bool {
	const checkInterval = 1 * time.Second
	const staleThreshold = 60 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		es.mu.Lock()
		if !es.extractionInProgress {
			es.mu.Unlock()
			return true
		}
		// If extraction is stale (> 60s old), don't wait — assume it crashed.
		// Matching upstream's EXTRACTION_STALE_THRESHOLD_MS = 60000.
		if time.Since(es.extractionStartedAt) > staleThreshold {
			es.mu.Unlock()
			return true
		}
		es.mu.Unlock()

		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(checkInterval)
	}
}
