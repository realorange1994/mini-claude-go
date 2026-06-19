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

// MemoryEntry represents a single memory note.
type MemoryEntry struct {
	Category  string    // "preference" | "decision" | "state" | "reference" | "test"
	Content   string    // the actual note text
	Timestamp time.Time // when it was created
	Source    string    // "user" | "assistant" | "auto" | "disk"
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

	// Deduplicate: if same category+content exists, update timestamp
	for i, e := range sm.entries {
		if e.Category == category && e.Content == content {
			sm.entries[i].Timestamp = time.Now()
			sm.dirty = true
			sm.maybeInvokeOnAdd()
			return
		}
	}

	sm.entries = append(sm.entries, MemoryEntry{
		Category:  category,
		Content:   content,
		Timestamp: time.Now(),
		Source:    source,
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
