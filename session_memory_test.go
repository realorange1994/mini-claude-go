package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ─── SessionMemory core operations ──────────────────────────────────────────

func TestSessionMemoryAddNote(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	sm.AddNote("state", "working on bug fix", "auto")
	sm.AddNote("decision", "use Go channels", "user")

	notes := sm.GetNotes()
	if len(notes) != 2 {
		t.Errorf("expected 2 notes, got %d", len(notes))
	}
}

func TestSessionMemoryAddNoteDedup(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	sm.AddNote("state", "working on bug fix", "auto")
	sm.AddNote("state", "working on bug fix", "auto") // duplicate

	notes := sm.GetNotes()
	if len(notes) != 1 {
		t.Errorf("expected 1 note (dedup), got %d", len(notes))
	}
}

func TestSessionMemoryAddNoteDifferentCategory(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	sm.AddNote("state", "working on bug fix", "auto")
	sm.AddNote("preference", "working on bug fix", "auto") // same content, different category

	notes := sm.GetNotes()
	if len(notes) != 2 {
		t.Errorf("expected 2 notes (different categories), got %d", len(notes))
	}
}

func TestSessionMemoryClearStateEntries(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	sm.AddNote("state", "working on bug fix", "auto")
	sm.AddNote("decision", "use Go channels", "user")
	sm.ClearStateEntries()

	notes := sm.GetNotes()
	if len(notes) != 1 {
		t.Errorf("expected 1 note (state cleared), got %d", len(notes))
	}
	if notes[0].Category != "decision" {
		t.Errorf("expected decision category, got %s", notes[0].Category)
	}
}

func TestSessionMemorySaveConclusions(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	// "bug fixed" -> "error" category (contains "bug")
	// "tests pass" -> "state" category (default, no result keywords)
	sm.SaveConclusions([]string{"bug fixed", "tests pass"})

	notes := sm.GetNotes()
	if len(notes) != 2 {
		t.Errorf("expected 2 conclusions, got %d", len(notes))
	}
	// Check that we have error and state categories
	categories := make(map[string]bool)
	for _, n := range notes {
		categories[n.Category] = true
	}
	if !categories["error"] {
		t.Errorf("expected error category for 'bug fixed', got categories: %v", categories)
	}
	if !categories["state"] {
		t.Errorf("expected state category for 'tests pass', got categories: %v", categories)
	}
}

func TestSessionMemorySaveConclusionsDedup(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	sm.AddNote("error", "bug fixed", "auto") // categorized as error now
	sm.SaveConclusions([]string{"bug fixed"}) // should dedup within error category

	notes := sm.GetNotes()
	if len(notes) != 1 {
		t.Errorf("expected 1 note (dedup), got %d", len(notes))
	}
	if notes[0].Category != "error" {
		t.Errorf("expected error category, got %s", notes[0].Category)
	}
}

func TestSessionMemorySaveConclusionsEmpty(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	sm.SaveConclusions(nil)
	sm.SaveConclusions([]string{})

	if len(sm.GetNotes()) != 0 {
		t.Error("empty conclusions should not add entries")
	}
}

// ─── Per-category max entries ────────────────────────────────────────────────

func TestSessionMemoryMaxStateEntries(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	for i := 0; i < 25; i++ {
		sm.AddNote("state", "entry "+strings.Repeat("x", i), "auto")
	}
	notes := sm.GetNotes()
	stateCount := 0
	for _, n := range notes {
		if n.Category == "state" {
			stateCount++
		}
	}
	if stateCount > maxStateEntries {
		t.Errorf("expected <= %d state entries, got %d", maxStateEntries, stateCount)
	}
}

func TestSessionMemoryMaxReferenceEntries(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	for i := 0; i < 60; i++ {
		sm.AddNote("reference", "ref "+strings.Repeat("y", i), "auto")
	}
	notes := sm.GetNotes()
	refCount := 0
	for _, n := range notes {
		if n.Category == "reference" {
			refCount++
		}
	}
	if refCount > maxReferenceEntries {
		t.Errorf("expected <= %d reference entries, got %d", maxReferenceEntries, refCount)
	}
}

// ─── Search ──────────────────────────────────────────────────────────────────

func TestSessionMemorySearchNotes(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	sm.AddNote("state", "working on bug fix", "auto")
	sm.AddNote("decision", "use Go channels", "user")

	results := sm.SearchNotes("bug")
	if len(results) != 1 {
		t.Errorf("expected 1 search result, got %d", len(results))
	}
}

func TestSessionMemorySearchNotesCaseInsensitive(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	sm.AddNote("state", "Working on BUG FIX", "auto")

	results := sm.SearchNotes("bug")
	if len(results) != 1 {
		t.Errorf("expected 1 result (case insensitive), got %d", len(results))
	}
}

// ─── FormatForTemplate (10-section template) ────────────────────────────────

func TestSessionMemoryFormatForTemplateEmpty(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	result := sm.FormatForTemplate()
	if result != defaultSessionMemoryTemplate {
		t.Error("empty session memory should return default template")
	}
}

func TestSessionMemoryFormatForTemplateWithEntries(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	sm.AddNote("state", "working on bug fix", "auto")
	sm.AddNote("decision", "use Go channels", "user")
	sm.AddNote("reference", "main.go: handler function", "auto")

	result := sm.FormatForTemplate()
	if !strings.Contains(result, "# Session Title") {
		t.Error("template should contain Session Title header")
	}
	if !strings.Contains(result, "# Current State") {
		t.Error("template should contain Current State header")
	}
	if !strings.Contains(result, "# Task Specification") {
		t.Error("template should contain Task Specification header")
	}
	if !strings.Contains(result, "# Files and Functions") {
		t.Error("template should contain Files and Functions header")
	}
	if !strings.Contains(result, "bug fix") {
		t.Error("template should contain state content")
	}
	if !strings.Contains(result, "Go channels") {
		t.Error("template should contain decision content")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("template should contain reference content")
	}
	// Verify italic descriptions are preserved
	if !strings.Contains(result, "_A short and distinctive 5-10 word") {
		t.Error("template should preserve italic description for Session Title")
	}
	if !strings.Contains(result, "_What is actively being worked on right now?") {
		t.Error("template should preserve italic description for Current State")
	}
}

func TestSessionMemoryFormatForTemplatePreservesAllSections(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	sm.AddNote("state", "test state", "auto")

	result := sm.FormatForTemplate()
	requiredHeaders := []string{
		"# Session Title", "# Current State", "# Task Specification",
		"# Files and Functions", "# Workflow", "# Errors & Corrections",
		"# Codebase and System Documentation", "# Learnings",
		"# Key Results", "# Worklog",
	}
	for _, h := range requiredHeaders {
		if !strings.Contains(result, h) {
			t.Errorf("template missing header: %s", h)
		}
	}
}

// ─── FormatForPromptCompact ──────────────────────────────────────────────────

func TestSessionMemoryFormatForPromptCompactEmpty(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	result := sm.FormatForPromptCompact()
	if result != "" {
		t.Error("empty session memory should return empty string for compact format")
	}
}

func TestSessionMemoryFormatForPromptCompactWithEntries(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	sm.AddNote("state", "working on bug fix", "auto")
	sm.AddNote("decision", "use Go channels", "user")

	result := sm.FormatForPromptCompact()
	if !strings.Contains(result, "## Session Memory") {
		t.Error("compact format should have Session Memory header")
	}
	if !strings.Contains(result, "bug fix") {
		t.Error("compact format should contain state content")
	}
}

// ─── Disk persistence ────────────────────────────────────────────────────────

func TestSessionMemoryFlushToDisk(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)
	sm.AddNote("state", "working on bug fix", "auto")
	sm.AddNote("decision", "use Go channels", "user")

	if err := sm.FlushToDisk(); err != nil {
		t.Fatalf("FlushToDisk failed: %v", err)
	}

	// Verify file exists
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "session_memory.md"))
	if err != nil {
		t.Fatalf("failed to read memory file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "# Current State") {
		t.Error("disk file should contain template format with Current State header")
	}
	if !strings.Contains(content, "bug fix") {
		t.Error("disk file should contain state content")
	}
	if !strings.Contains(content, "Go channels") {
		t.Error("disk file should contain decision content")
	}
}

func TestSessionMemoryLoadFromDisk(t *testing.T) {
	dir := t.TempDir()
	sm1 := NewSessionMemory(dir)
	sm1.AddNote("state", "working on bug fix", "auto")
	sm1.AddNote("decision", "use Go channels", "user")
	sm1.FlushToDisk()

	// Load from disk in a new SessionMemory
	sm2 := NewSessionMemory(dir)
	notes := sm2.GetNotes()
	// state entries are cleared on load, so only decision should remain
	if len(notes) != 1 {
		t.Errorf("expected 1 note (state cleared on load), got %d", len(notes))
	}
	if notes[0].Category != "decision" {
		t.Errorf("expected decision category, got %s", notes[0].Category)
	}
}

func TestSessionMemoryLoadFromDiskTemplateFormat(t *testing.T) {
	dir := t.TempDir()
	// Write a template-format file directly
	templateContent := `# Session Title
_A short and distinctive 5-10 word descriptive title_

# Current State
_What is actively being worked on right now?_
- Working on auth bug
- Tests passing

# Task Specification
_What did the user ask to build?_
- Use JWT tokens

# Files and Functions
_What are the important files?_
- auth.go: handler function
`
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
	os.WriteFile(filepath.Join(dir, ".claude", "session_memory.md"), []byte(templateContent), 0644)

	sm := NewSessionMemory(dir)
	notes := sm.GetNotes()
	// state entries are cleared on load
	stateCount := 0
	decisionCount := 0
	refCount := 0
	for _, n := range notes {
		switch n.Category {
		case "state":
			stateCount++
		case "decision":
			decisionCount++
		case "reference":
			refCount++
		}
	}
	if stateCount != 0 {
		t.Errorf("state entries should be cleared on load, got %d", stateCount)
	}
	if decisionCount != 1 {
		t.Errorf("expected 1 decision entry, got %d", decisionCount)
	}
	if refCount != 1 {
		t.Errorf("expected 1 reference entry, got %d", refCount)
	}
}

func TestSessionMemoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sm1 := NewSessionMemory(dir)
	sm1.AddNote("decision", "use Go channels", "user")
	sm1.AddNote("reference", "main.go: handler", "auto")
	sm1.FlushToDisk()

	sm2 := NewSessionMemory(dir)
	sm2.AddNote("preference", "prefer short responses", "user")
	sm2.FlushToDisk()

	// Load again
	sm3 := NewSessionMemory(dir)
	notes := sm3.GetNotes()
	// state cleared, but decision + reference + preference should remain
	found := map[string]bool{}
	for _, n := range notes {
		found[n.Category] = true
	}
	if !found["decision"] {
		t.Error("decision entries should survive round-trip")
	}
	if !found["reference"] {
		t.Error("reference entries should survive round-trip")
	}
	if !found["preference"] {
		t.Error("preference entries should survive round-trip")
	}
}

// ─── Expiration ──────────────────────────────────────────────────────────────

func TestSessionMemoryExpiration(t *testing.T) {
	sm := NewSessionMemory(t.TempDir())
	// Manually add an expired entry
	sm.mu.Lock()
	sm.entries = append(sm.entries, MemoryEntry{
		Category:  "decision",
		Content:   "old decision",
		Timestamp: time.Now().Add(-31 * 24 * time.Hour), // 31 days old
		Source:    "auto",
	})
	sm.entries = append(sm.entries, MemoryEntry{
		Category:  "decision",
		Content:   "recent decision",
		Timestamp: time.Now(),
		Source:    "auto",
	})
	sm.mu.Unlock()

	sm.removeExpiredEntries()
	notes := sm.GetNotes()
	if len(notes) != 1 {
		t.Errorf("expected 1 note (expired removed), got %d", len(notes))
	}
	if notes[0].Content != "recent decision" {
		t.Errorf("expected recent decision, got %s", notes[0].Content)
	}
}

// ─── Extraction thresholds ───────────────────────────────────────────────────

func TestExtractionStateShouldExtractNotInitialized(t *testing.T) {
	es := NewExtractionState()
	// Below threshold
	if es.ShouldExtract(5000, false) {
		t.Error("should not extract below minimumMessageTokensToInit")
	}
	// At threshold
	if !es.ShouldExtract(20000, false) {
		t.Error("should extract at minimumMessageTokensToInit")
	}
}

func TestExtractionStateShouldExtractAfterInit(t *testing.T) {
	es := NewExtractionState()
	es.MarkExtracted(20000)

	// Below growth threshold
	if es.ShouldExtract(25000, true) {
		t.Error("should not extract below minimumTokensBetweenUpdate")
	}
	// At growth threshold + 3 tool calls
	es.IncrementToolCall()
	es.IncrementToolCall()
	es.IncrementToolCall()
	if !es.ShouldExtract(30000, true) {
		t.Error("should extract at growth threshold with tool calls")
	}
}

func TestExtractionStateShouldExtractWithToolCalls(t *testing.T) {
	es := NewExtractionState()
	es.MarkExtracted(20000)

	// Growth threshold met but no tool calls
	es.IncrementToolCall()
	es.IncrementToolCall()
	// Only 2 tool calls, need 3
	if es.ShouldExtract(30000, true) {
		t.Error("should not extract with only 2 tool calls")
	}
	es.IncrementToolCall()
	// Now 3 tool calls
	if !es.ShouldExtract(30000, true) {
		t.Error("should extract with 3 tool calls and growth threshold")
	}
}

func TestExtractionStateShouldExtractNoToolCallsInLastTurn(t *testing.T) {
	es := NewExtractionState()
	es.MarkExtracted(20000)

	// Growth threshold met, no tool calls in last turn → extract
	if !es.ShouldExtract(30000, false) {
		t.Error("should extract when no tool calls in last turn and growth threshold met")
	}
}

func TestExtractionStateMarkExtractionInProgress(t *testing.T) {
	es := NewExtractionState()
	es.MarkExtractionInProgress()

	if !es.extractionInProgress {
		t.Error("extraction should be in progress")
	}
}

func TestExtractionStateWaitForExtraction(t *testing.T) {
	es := NewExtractionState()
	es.MarkExtractionInProgress()

	// Should timeout since extraction never completes
	result := es.WaitForExtraction(2 * time.Second)
	if result {
		t.Error("should timeout since extraction never completes")
	}
}

func TestExtractionStateWaitForExtractionCompleted(t *testing.T) {
	es := NewExtractionState()
	es.MarkExtractionInProgress()

	// Complete extraction in background
	go func() {
		time.Sleep(100 * time.Millisecond)
		es.MarkExtracted(10000)
	}()

	result := es.WaitForExtraction(5 * time.Second)
	if !result {
		t.Error("should complete since extraction finishes")
	}
}

// ─── IsSessionMemoryTemplateOnly ─────────────────────────────────────────────

func TestIsSessionMemoryTemplateOnly(t *testing.T) {
	if !IsSessionMemoryTemplateOnly(defaultSessionMemoryTemplate) {
		t.Error("default template should be recognized as template-only")
	}
	if IsSessionMemoryTemplateOnly(defaultSessionMemoryTemplate + "\n- Some content") {
		t.Error("template with content should not be recognized as template-only")
	}
	if !IsSessionMemoryTemplateOnly(strings.TrimSpace(defaultSessionMemoryTemplate)) {
		t.Error("trimmed template should still be recognized as template-only")
	}
}

// ─── truncateSessionMemoryForCompact ─────────────────────────────────────────

func TestTruncateSessionMemoryForCompact(t *testing.T) {
	// Build a long session memory
	var sb strings.Builder
	sb.WriteString("# Session Title\n_A title_\nTest Session\n\n")
	sb.WriteString("# Current State\n_State_\n")
	for i := 0; i < 100; i++ {
		sb.WriteString("- " + strings.Repeat("x", 50) + "\n")
	}
	sb.WriteString("\n# Worklog\n_Log_\n- Step 1\n")

	content := sb.String()
	truncated := truncateSessionMemoryForCompact(content, 1000)
	if EstimateTokens(truncated) > 1200 { // some tolerance
		t.Errorf("truncated content should be under ~1000 tokens, got %d", EstimateTokens(truncated))
	}
	if !strings.Contains(truncated, "# Session Title") {
		t.Error("truncated content should preserve section headers")
	}
}

// ─── createMemoryFileCanUseTool ──────────────────────────────────────────────

func TestCreateMemoryFileCanUseToolAllowsEditOnMemoryFile(t *testing.T) {
	path := filepath.Clean("/project/.claude/session_memory.md")
	fn := createMemoryFileCanUseTool(path)

	allowed, reason := fn("edit_file", map[string]any{"file_path": "/project/.claude/session_memory.md"})
	if !allowed {
		t.Errorf("edit_file on memory file should be allowed, reason: %s", reason)
	}
}

func TestCreateMemoryFileCanUseToolDeniesOtherTools(t *testing.T) {
	path := filepath.Clean("/project/.claude/session_memory.md")
	fn := createMemoryFileCanUseTool(path)

	allowed, _ := fn("exec", map[string]any{"command": "ls"})
	if allowed {
		t.Error("exec should be denied in extraction mode")
	}

	allowed, _ = fn("file_read", map[string]any{"path": "/other/file.go"})
	if allowed {
		t.Error("file_read should be denied in extraction mode")
	}
}

func TestCreateMemoryFileCanUseToolDeniesEditOnOtherFile(t *testing.T) {
	path := filepath.Clean("/project/.claude/session_memory.md")
	fn := createMemoryFileCanUseTool(path)

	allowed, reason := fn("edit_file", map[string]any{"file_path": "/project/other.go"})
	if allowed {
		t.Error("edit_file on non-memory file should be denied")
	}
	if !strings.Contains(reason, "can only edit session memory file") {
		t.Errorf("reason should mention session memory file, got: %s", reason)
	}
}

// ─── sessionMemoryUpdatePrompt ───────────────────────────────────────────────

func TestSessionMemoryUpdatePrompt(t *testing.T) {
	prompt := sessionMemoryUpdatePrompt("/path/to/session_memory.md", "current content")
	if !strings.Contains(prompt, "IMPORTANT") {
		t.Error("prompt should contain IMPORTANT header")
	}
	if !strings.Contains(prompt, "/path/to/session_memory.md") {
		t.Error("prompt should contain the file path")
	}
	if !strings.Contains(prompt, "current content") {
		t.Error("prompt should contain current notes content")
	}
	if !strings.Contains(prompt, "edit_file") {
		t.Error("prompt should reference edit_file tool")
	}
	if !strings.Contains(prompt, "CRITICAL RULES") {
		t.Error("prompt should contain CRITICAL RULES section")
	}
}

// ─── Disk format consistency ─────────────────────────────────────────────────

func TestSessionMemoryDiskFormatIsTemplate(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)
	sm.AddNote("state", "fixing auth bug", "auto")
	sm.AddNote("decision", "use JWT tokens", "user")
	sm.AddNote("reference", "auth.go: ValidateToken()", "auto")
	sm.FlushToDisk()

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "session_memory.md"))
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	content := string(data)

	// Disk format should be the 10-section template, not ### category format
	if strings.Contains(content, "### state") {
		t.Error("disk format should NOT use ### category headers")
	}
	if !strings.Contains(content, "# Current State") {
		t.Error("disk format should use # Current State header")
	}
	if !strings.Contains(content, "# Task Specification") {
		t.Error("disk format should use # Task Specification header")
	}
	if !strings.Contains(content, "# Files and Functions") {
		t.Error("disk format should use # Files and Functions header")
	}
	// Verify italic descriptions are preserved
	if !strings.Contains(content, "_What is actively being worked on right now?") {
		t.Error("disk format should preserve italic description for Current State")
	}
}

func TestSessionMemoryDiskFormatRoundTripConsistency(t *testing.T) {
	dir := t.TempDir()
	sm1 := NewSessionMemory(dir)
	sm1.AddNote("decision", "use JWT", "user")
	sm1.AddNote("reference", "auth.go", "auto")
	sm1.FlushToDisk()

	// Read raw file
	data1, _ := os.ReadFile(filepath.Join(dir, ".claude", "session_memory.md"))

	// Load, add more, flush
	sm2 := NewSessionMemory(dir)
	sm2.AddNote("preference", "short responses", "user")
	sm2.FlushToDisk()

	data2, _ := os.ReadFile(filepath.Join(dir, ".claude", "session_memory.md"))

	// Both should be template format
	if !strings.Contains(string(data1), "# Task Specification") {
		t.Error("first flush should be template format")
	}
	if !strings.Contains(string(data2), "# Task Specification") {
		t.Error("second flush should be template format")
	}
	if !strings.Contains(string(data2), "short responses") {
		t.Error("second flush should contain new content")
	}
}

// ─── Three-Level Memory System ──────────────────────────────────────────────

func TestThreeLevelMemoryAddScopedNote(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Add notes to different scopes
	sm.AddScopedNote(ScopeGlobal, "preference", "Use Go 1.25+", "user")
	sm.AddScopedNote(ScopeProject, "decision", "Use SQLite for persistence", "user")
	sm.AddScopedNote(ScopeSession, "state", "Working on P0", "auto")

	// Verify each scope has its entry
	globalNotes := sm.GetScopedNotes(ScopeGlobal)
	if len(globalNotes) != 1 {
		t.Errorf("expected 1 global note, got %d", len(globalNotes))
	}
	if globalNotes[0].Content != "Use Go 1.25+" {
		t.Errorf("unexpected global content: %s", globalNotes[0].Content)
	}

	projectNotes := sm.GetScopedNotes(ScopeProject)
	if len(projectNotes) != 1 {
		t.Errorf("expected 1 project note, got %d", len(projectNotes))
	}
	if projectNotes[0].Content != "Use SQLite for persistence" {
		t.Errorf("unexpected project content: %s", projectNotes[0].Content)
	}

	sessionNotes := sm.GetScopedNotes(ScopeSession)
	if len(sessionNotes) != 1 {
		t.Errorf("expected 1 session note, got %d", len(sessionNotes))
	}
	if sessionNotes[0].Content != "Working on P0" {
		t.Errorf("unexpected session content: %s", sessionNotes[0].Content)
	}
}

func TestThreeLevelMemoryGetNotesIncludesAll(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddScopedNote(ScopeGlobal, "preference", "global-pref", "user")
	sm.AddScopedNote(ScopeProject, "decision", "project-dec", "user")
	sm.AddScopedNote(ScopeSession, "state", "session-state", "auto")

	// GetNotes should return all three levels
	allNotes := sm.GetNotes()
	if len(allNotes) != 3 {
		t.Errorf("expected 3 total notes, got %d", len(allNotes))
	}
}

func TestThreeLevelMemorySearchAcrossScopes(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddScopedNote(ScopeGlobal, "preference", "Use Go channels for concurrency", "user")
	sm.AddScopedNote(ScopeProject, "decision", "Use goroutine pool", "user")
	sm.AddScopedNote(ScopeSession, "state", "Working on channel implementation", "auto")

	// Search should find matches across all scopes
	results := sm.SearchNotes("channel")
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'channel', got %d", len(results))
	}

	// Scoped search should filter
	scopedResults := sm.SearchScopedNotes("channel", ScopeGlobal)
	if len(scopedResults) != 1 {
		t.Errorf("expected 1 global result for 'channel', got %d", len(scopedResults))
	}
}

func TestThreeLevelMemoryDeduplication(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Add same content twice to same scope
	sm.AddScopedNote(ScopeGlobal, "preference", "Use Go 1.25+", "user")
	sm.AddScopedNote(ScopeGlobal, "preference", "Use Go 1.25+", "user")

	globalNotes := sm.GetScopedNotes(ScopeGlobal)
	if len(globalNotes) != 1 {
		t.Errorf("expected deduplication, got %d notes", len(globalNotes))
	}
}

func TestThreeLevelMemoryFlushAndLoad(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, ".claude", "memory", "global.md")

	// Create and populate
	sm1 := NewSessionMemoryWithPaths(dir, globalPath)
	sm1.AddScopedNote(ScopeGlobal, "preference", "global-pref-1", "user")
	sm1.AddScopedNote(ScopeProject, "decision", "project-dec-1", "user")
	sm1.AddScopedNote(ScopeSession, "decision", "session-dec-1", "user") // Use "decision" not "state" — state is cleared on reload
	sm1.FlushToDisk()

	// Verify files exist
	projectPath := filepath.Join(dir, ".claude", "memory", "project.md")
	sessionPath := filepath.Join(dir, ".claude", "session_memory.md")

	if _, err := os.Stat(globalPath); os.IsNotExist(err) {
		t.Error("global memory file should exist")
	}
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		t.Error("project memory file should exist")
	}
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		t.Error("session memory file should exist")
	}

	// Load in new instance
	sm2 := NewSessionMemoryWithPaths(dir, globalPath)

	globalNotes := sm2.GetScopedNotes(ScopeGlobal)
	if len(globalNotes) != 1 {
		t.Errorf("expected 1 global note after reload, got %d", len(globalNotes))
	}
	if globalNotes[0].Content != "global-pref-1" {
		t.Errorf("unexpected global content after reload: %s", globalNotes[0].Content)
	}

	projectNotes := sm2.GetScopedNotes(ScopeProject)
	if len(projectNotes) != 1 {
		t.Errorf("expected 1 project note after reload, got %d", len(projectNotes))
	}
	if projectNotes[0].Content != "project-dec-1" {
		t.Errorf("unexpected project content after reload: %s", projectNotes[0].Content)
	}

	// Session notes are loaded from template format
	sessionNotes := sm2.GetScopedNotes(ScopeSession)
	if len(sessionNotes) != 1 {
		t.Errorf("expected 1 session note after reload, got %d", len(sessionNotes))
	}
	if sessionNotes[0].Content != "session-dec-1" {
		t.Errorf("unexpected session content after reload: %s", sessionNotes[0].Content)
	}
}

func TestThreeLevelMemoryFormatForPromptCompact(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddScopedNote(ScopeGlobal, "preference", "Use Go 1.25+", "user")
	sm.AddScopedNote(ScopeProject, "decision", "Use SQLite", "user")
	sm.AddScopedNote(ScopeSession, "state", "Working on P0", "auto")

	compact := sm.FormatForPromptCompact()

	// Should contain all three level headers
	if !strings.Contains(compact, "## Global Memory") {
		t.Error("compact should contain Global Memory section")
	}
	if !strings.Contains(compact, "## Project Memory") {
		t.Error("compact should contain Project Memory section")
	}
	if !strings.Contains(compact, "## Session Memory") {
		t.Error("compact should contain Session Memory section")
	}

	// Should contain actual content
	if !strings.Contains(compact, "Use Go 1.25+") {
		t.Error("compact should contain global content")
	}
	if !strings.Contains(compact, "Use SQLite") {
		t.Error("compact should contain project content")
	}
	if !strings.Contains(compact, "Working on P0") {
		t.Error("compact should contain session content")
	}
}

func TestThreeLevelMemoryFormatForPrompt(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddScopedNote(ScopeGlobal, "preference", "global-pref", "user")
	sm.AddScopedNote(ScopeProject, "decision", "project-dec", "user")
	sm.AddScopedNote(ScopeSession, "state", "session-state", "auto")

	prompt := sm.FormatForPrompt()

	if !strings.Contains(prompt, "## Global Memory") {
		t.Error("prompt should contain Global Memory section")
	}
	if !strings.Contains(prompt, "## Project Memory") {
		t.Error("prompt should contain Project Memory section")
	}
	if !strings.Contains(prompt, "## Session Memory") {
		t.Error("prompt should contain Session Memory section")
	}
}

func TestThreeLevelMemoryFormatForPromptCompactEmpty(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Empty memory should return empty string
	compact := sm.FormatForPromptCompact()
	if compact != "" {
		t.Errorf("expected empty compact for empty memory, got %q", compact)
	}
}

func TestThreeLevelMemoryClearStateEntries(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Add state entries to session scope
	sm.AddScopedNote(ScopeSession, "state", "session-state-1", "auto")
	sm.AddScopedNote(ScopeSession, "state", "session-state-2", "auto")
	sm.AddScopedNote(ScopeSession, "decision", "session-dec-1", "user")

	// ClearStateEntries should only clear session state
	sm.ClearStateEntries()

	sessionNotes := sm.GetScopedNotes(ScopeSession)
	if len(sessionNotes) != 1 {
		t.Errorf("expected 1 session note after clear, got %d", len(sessionNotes))
	}
	if sessionNotes[0].Category != "decision" {
		t.Errorf("expected remaining note to be decision, got %s", sessionNotes[0].Category)
	}
}

func TestThreeLevelMemoryFileContent(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, ".claude", "memory", "global.md")
	sm := NewSessionMemoryWithPaths(dir, globalPath)

	sm.AddScopedNote(ScopeGlobal, "preference", "Use Go 1.25+", "user")
	sm.AddScopedNote(ScopeGlobal, "preference", "Prefer table-driven tests", "user")
	sm.FlushToDisk()

	// Read global file and verify format
	data, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatalf("failed to read global memory: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "## preference") {
		t.Error("global file should contain category header")
	}
	if !strings.Contains(content, "- Use Go 1.25+") {
		t.Error("global file should contain first entry")
	}
	if !strings.Contains(content, "- Prefer table-driven tests") {
		t.Error("global file should contain second entry")
	}
}

func TestThreeLevelMemoryProjectFileContent(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddScopedNote(ScopeProject, "decision", "Use SQLite for persistence", "user")
	sm.AddScopedNote(ScopeProject, "reference", "src/db/schema.go", "auto")
	sm.FlushToDisk()

	// Read project file and verify format
	projectPath := filepath.Join(dir, ".claude", "memory", "project.md")
	data, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatalf("failed to read project memory: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "## decision") {
		t.Error("project file should contain decision header")
	}
	if !strings.Contains(content, "## reference") {
		t.Error("project file should contain reference header")
	}
	if !strings.Contains(content, "- Use SQLite for persistence") {
		t.Error("project file should contain decision content")
	}
	if !strings.Contains(content, "- src/db/schema.go") {
		t.Error("project file should contain reference content")
	}
}

func TestThreeLevelMemoryTimestampPersistence(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, ".claude", "memory", "global.md")
	sm := NewSessionMemoryWithPaths(dir, globalPath)

	// Add a note with a known timestamp
	before := time.Now().Truncate(time.Second)
	sm.AddScopedNote(ScopeGlobal, "preference", "test-timestamp", "user")
	sm.FlushToDisk()

	// Reload and check timestamp is preserved
	sm2 := NewSessionMemoryWithPaths(dir, globalPath)
	notes := sm2.GetScopedNotes(ScopeGlobal)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}

	// Timestamp should be after 'before' (with some tolerance)
	if notes[0].Timestamp.Before(before) {
		t.Errorf("timestamp should be preserved: got %v, expected >= %v", notes[0].Timestamp, before)
	}
}

func TestThreeLevelMemoryMultipleFlushes(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, ".claude", "memory", "global.md")
	sm := NewSessionMemoryWithPaths(dir, globalPath)

	// First flush
	sm.AddScopedNote(ScopeGlobal, "preference", "first-global", "user")
	sm.FlushToDisk()

	// Second flush with additional entries
	sm.AddScopedNote(ScopeGlobal, "preference", "second-global", "user")
	sm.FlushToDisk()

	// Reload and verify both entries exist
	sm2 := NewSessionMemoryWithPaths(dir, globalPath)
	notes := sm2.GetScopedNotes(ScopeGlobal)
	if len(notes) != 2 {
		t.Errorf("expected 2 global notes after multiple flushes, got %d", len(notes))
	}
}

// ─── Phase 4: Extraction Tests ──────────────────────────────────────────────

func TestExtractUserInstructions(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string // expected type
	}{
		{
			name:     "explicit request",
			content:  "Please make sure to use Go 1.25 for this project",
			expected: "instruction",
		},
		{
			name:     "constraint",
			content:  "You must not use any external dependencies",
			expected: "constraint",
		},
		{
			name:     "preference",
			content:  "I prefer using table-driven tests",
			expected: "preference",
		},
		{
			name:     "fix request",
			content:  "Fix the authentication bug in the login flow",
			expected: "instruction",
		},
		{
			name:     "create request",
			content:  "Add a new REST endpoint for user management",
			expected: "instruction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := extractUserInstructions(tt.content, 0)
			if len(items) == 0 {
				t.Errorf("expected at least 1 item, got 0")
				return
			}
			found := false
			for _, item := range items {
				if item.Type == tt.expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected type '%s', got items: %v", tt.expected, items)
			}
		})
	}
}

func TestExtractDiscoveries(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "discovery",
			content:  "Found that the issue was caused by a race condition in the mutex",
			expected: "discovery",
		},
		{
			name:     "solution",
			content:  "The fix is to use sync.RWMutex instead of sync.Mutex",
			expected: "discovery",
		},
		{
			name:     "error finding",
			content:  "Error: the database connection string was malformed",
			expected: "error",
		},
		{
			name:     "result",
			content:  "Fixed: the authentication now works correctly",
			expected: "result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := extractDiscoveries(tt.content, 0)
			if len(items) == 0 {
				t.Errorf("expected at least 1 item, got 0")
				return
			}
			found := false
			for _, item := range items {
				if item.Category == tt.expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected category '%s', got items: %v", tt.expected, items)
			}
		})
	}
}

func TestExtractFromMessages(t *testing.T) {
	messages := []ConversationMessage{
		{Role: "user", Content: "Please implement the REST API using Go standard library"},
		{Role: "assistant", Content: "I found that the issue was caused by missing error handling"},
		{Role: "user", Content: "Make sure to add unit tests for all endpoints"},
		{Role: "assistant", Content: "Fixed: the authentication now works correctly"},
	}

	items := ExtractFromMessages(messages)
	if len(items) < 2 {
		t.Errorf("expected at least 2 items, got %d", len(items))
	}

	// Check for user instruction
	hasInstruction := false
	for _, item := range items {
		if item.Source == "user" && item.Type == "instruction" {
			hasInstruction = true
			break
		}
	}
	if !hasInstruction {
		t.Error("expected at least 1 user instruction")
	}

	// Check for discovery
	hasDiscovery := false
	for _, item := range items {
		if item.Source == "assistant" && item.Type == "discovery" {
			hasDiscovery = true
			break
		}
	}
	if !hasDiscovery {
		t.Error("expected at least 1 discovery")
	}
}

func TestSaveExtractedItems(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	items := []ExtractedItem{
		{Type: "instruction", Content: "Use Go 1.25", Source: "user", Category: "instruction"},
		{Type: "discovery", Content: "Found race condition in mutex", Source: "assistant", Category: "discovery"},
		{Type: "constraint", Content: "No external dependencies", Source: "user", Category: "constraint"},
	}

	sm.SaveExtractedItems(items)

	// Verify items were saved
	notes := sm.GetNotes()
	if len(notes) < 3 {
		t.Errorf("expected at least 3 notes, got %d", len(notes))
	}

	// Check categories
	foundCategories := make(map[string]bool)
	for _, note := range notes {
		foundCategories[note.Category] = true
	}
	if !foundCategories["instruction"] {
		t.Error("expected instruction category")
	}
	if !foundCategories["discovery"] {
		t.Error("expected discovery category")
	}
	if !foundCategories["constraint"] {
		t.Error("expected constraint category")
	}
}

func TestSaveExtractedItems_Dedup(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	items := []ExtractedItem{
		{Type: "instruction", Content: "Use Go 1.25", Source: "user", Category: "instruction"},
		{Type: "instruction", Content: "Use Go 1.25", Source: "user", Category: "instruction"}, // duplicate
	}

	sm.SaveExtractedItems(items)

	notes := sm.GetScopedNotes(ScopeSession)
	instructionCount := 0
	for _, note := range notes {
		if note.Category == "instruction" {
			instructionCount++
		}
	}
	if instructionCount != 1 {
		t.Errorf("expected 1 instruction (deduped), got %d", instructionCount)
	}
}

func TestExtractAndSave(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	messages := []ConversationMessage{
		{Role: "user", Content: "Please implement the REST API"},
		{Role: "assistant", Content: "Found that the issue was caused by missing error handling"},
	}

	count := sm.ExtractAndSave(messages)
	if count < 1 {
		t.Errorf("expected at least 1 extracted item, got %d", count)
	}

	notes := sm.GetNotes()
	if len(notes) < 1 {
		t.Errorf("expected at least 1 note, got %d", len(notes))
	}
}

func TestExtractSentence(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		pattern  string
		contains string
	}{
		{
			name:     "simple sentence",
			content:  "Please make sure to use Go for this project",
			pattern:  "make sure",
			contains: "make sure",
		},
		{
			name:     "multiline",
			content:  "Line 1\nPlease make sure to use Go for this project\nLine 3",
			pattern:  "make sure",
			contains: "make sure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSentence(tt.content, tt.pattern)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected to contain '%s', got '%s'", tt.contains, result)
			}
		})
	}
}

func TestExtractUserInstructions_Empty(t *testing.T) {
	items := extractUserInstructions("", 0)
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty content, got %d", len(items))
	}
}

func TestExtractDiscoveries_Empty(t *testing.T) {
	items := extractDiscoveries("", 0)
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty content, got %d", len(items))
	}
}

func TestDeduplicateItems(t *testing.T) {
	items := []ExtractedItem{
		{Type: "instruction", Content: "Use Go 1.25", Source: "user"},
		{Type: "instruction", Content: "Use Go 1.25", Source: "user"},
		{Type: "discovery", Content: "Found race condition", Source: "assistant"},
	}

	result := deduplicateItems(items)
	if len(result) != 2 {
		t.Errorf("expected 2 items after dedup, got %d", len(result))
	}
}

// ─── Deduplication and Version Control Tests ─────────────────────────────────

func TestContentSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		minScore float64
		maxScore float64
	}{
		{"identical", "hello world", "hello world", 1.0, 1.0},
		{"empty", "", "", 1.0, 1.0},
		{"one empty", "hello", "", 0.0, 0.0},
		{"similar", "Use Go 1.25 for this project", "Use Go 1.25 for the project", 0.7, 1.0},
		{"different", "hello world", "foo bar baz", 0.0, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContentSimilarity(tt.a, tt.b)
			if result < tt.minScore || result > tt.maxScore {
				t.Errorf("expected %.2f-%.2f, got %.2f", tt.minScore, tt.maxScore, result)
			}
		})
	}
}

func TestDeduplicateEntries_ExactMatch(t *testing.T) {
	entries := []MemoryEntry{
		{Category: "state", Content: "Working on auth", Timestamp: time.Now()},
		{Category: "state", Content: "Working on auth", Timestamp: time.Now()},
		{Category: "decision", Content: "Use SQLite", Timestamp: time.Now()},
	}

	result, merged := DeduplicateEntries(entries)
	if len(result) != 2 {
		t.Errorf("expected 2 entries after dedup, got %d", len(result))
	}
	if merged != 1 {
		t.Errorf("expected 1 merge, got %d", merged)
	}
}

func TestDeduplicateEntries_FuzzyMatch(t *testing.T) {
	entries := []MemoryEntry{
		{Category: "state", Content: "Working on authentication module", Timestamp: time.Now()},
		{Category: "state", Content: "Working on the authentication module", Timestamp: time.Now()},
		{Category: "decision", Content: "Use SQLite", Timestamp: time.Now()},
	}

	result, _ := DeduplicateEntries(entries)
	if len(result) < 2 {
		t.Errorf("expected at least 2 entries after fuzzy dedup, got %d", len(result))
	}
}

func TestMergeEntries(t *testing.T) {
	a := MemoryEntry{
		Category:  "state",
		Content:   "Working on auth",
		Timestamp: time.Now().Add(-time.Hour),
		Version:   1,
	}
	b := MemoryEntry{
		Category:  "state",
		Content:   "Working on authentication",
		Timestamp: time.Now(),
		Version:   1,
	}

	merged := MergeEntries(a, b)
	if merged.Version != 2 {
		t.Errorf("expected version 2, got %d", merged.Version)
	}
	if merged.PrevHash == "" {
		t.Error("expected non-empty PrevHash")
	}
}

func TestUpdateNote(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working on auth", "user")
	sm.UpdateNote("state", "Working on auth", "Working on authentication module", "assistant")

	notes := sm.GetScopedNotes(ScopeSession)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Content != "Working on authentication module" {
		t.Errorf("expected updated content, got '%s'", notes[0].Content)
	}
	if notes[0].Version != 2 {
		t.Errorf("expected version 2, got %d", notes[0].Version)
	}
}

func TestUpdateNote_NotFound(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	result := sm.UpdateNote("state", "nonexistent", "new content", "user")
	if result {
		t.Error("expected false for non-existent note")
	}
}

func TestMergeDuplicates(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Directly add entries (bypass AddNote's fuzzy dedup)
	sm.mu.Lock()
	sm.entries = []MemoryEntry{
		{Category: "state", Content: "Working on authentication module for the app", Timestamp: time.Now(), Version: 1},
		{Category: "state", Content: "Working on authentication module for the app now", Timestamp: time.Now(), Version: 1},
		{Category: "decision", Content: "Use SQLite", Timestamp: time.Now(), Version: 1},
	}
	sm.mu.Unlock()

	merged := sm.MergeDuplicates()
	if merged < 1 {
		t.Errorf("expected at least 1 merge, got %d", merged)
	}

	notes := sm.GetScopedNotes(ScopeSession)
	stateNotes := 0
	for _, n := range notes {
		if n.Category == "state" {
			stateNotes++
		}
	}
	if stateNotes > 1 {
		t.Errorf("expected at most 1 state note after merge, got %d", stateNotes)
	}
}

func TestGetStats(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working on auth", "user")
	sm.AddNote("decision", "Use SQLite", "user")
	sm.AddNote("reference", "src/auth.go", "auto")

	stats := sm.GetStats()
	if stats.SessionEntries != 3 {
		t.Errorf("expected 3 session entries, got %d", stats.SessionEntries)
	}
	if stats.ByCategory["state"] != 1 {
		t.Errorf("expected 1 state entry, got %d", stats.ByCategory["state"])
	}
}

func TestAddNote_VersionTracking(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// First add
	sm.AddNote("state", "Working on auth", "user")

	// Exact duplicate - should just update timestamp
	sm.AddNote("state", "Working on auth", "user")

	notes := sm.GetScopedNotes(ScopeSession)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}

	// Version should still be 1 (exact dedup doesn't increment)
	if notes[0].Version != 1 {
		t.Errorf("expected version 1 after exact dedup, got %d", notes[0].Version)
	}
}

func TestAddNote_FuzzyMerge(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// First add
	sm.AddNote("state", "Working on authentication module", "user")

	// Similar content - should merge
	sm.AddNote("state", "Working on the authentication module", "user")

	notes := sm.GetScopedNotes(ScopeSession)
	stateNotes := 0
	for _, n := range notes {
		if n.Category == "state" {
			stateNotes++
		}
	}

	// Should have merged into 1 entry
	if stateNotes != 1 {
		t.Errorf("expected 1 state note after fuzzy merge, got %d", stateNotes)
	}
}

func TestContentHash(t *testing.T) {
	e := MemoryEntry{Content: "hello world"}
	hash := e.ContentHash()
	if hash == "" {
		t.Error("expected non-empty hash")
	}

	// Same content should produce same hash
	e2 := MemoryEntry{Content: "hello world"}
	if e.ContentHash() != e2.ContentHash() {
		t.Error("same content should produce same hash")
	}

	// Different content should produce different hash
	e3 := MemoryEntry{Content: "goodbye world"}
	if e.ContentHash() == e3.ContentHash() {
		t.Error("different content should produce different hash")
	}
}

// ─── Intelligent Memory Retrieval Tests ─────────────────────────────────────

func TestSmartSearch_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("preference", "Use Go 1.25 for all projects", "user")
	sm.AddNote("decision", "Use SQLite for persistence", "user")
	sm.AddNote("state", "Working on authentication module", "user")

	opts := DefaultSearchOptions()
	results := sm.SmartSearch("Go 1.25", opts)

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Entry.Content != "Use Go 1.25 for all projects" {
		t.Errorf("expected 'Use Go 1.25 for all projects', got '%s'", results[0].Entry.Content)
	}
	if results[0].Score <= 0.5 {
		t.Errorf("expected high score for exact match, got %.2f", results[0].Score)
	}
}

func TestSmartSearch_PartialMatch(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("preference", "Use Go 1.25 for all projects", "user")
	sm.AddNote("decision", "Use SQLite for persistence", "user")

	opts := DefaultSearchOptions()
	results := sm.SmartSearch("SQLite persistence", opts)

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	found := false
	for _, r := range results {
		if strings.Contains(r.Entry.Content, "SQLite") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find SQLite entry")
	}
}

func TestSmartSearch_CategoryFilter(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("preference", "Use Go 1.25", "user")
	sm.AddNote("decision", "Use Go 1.25 for backend", "user")

	opts := DefaultSearchOptions()
	opts.Categories = []string{"preference"}
	results := sm.SmartSearch("Go 1.25", opts)

	if len(results) != 1 {
		t.Errorf("expected 1 result with category filter, got %d", len(results))
	}
	if len(results) > 0 && results[0].Entry.Category != "preference" {
		t.Errorf("expected category 'preference', got '%s'", results[0].Entry.Category)
	}
}

func TestSmartSearch_RecencyRanking(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Add old entry
	sm.mu.Lock()
	sm.entries = append(sm.entries, MemoryEntry{
		Category:  "state",
		Content:   "Old state about authentication module",
		Timestamp: time.Now().Add(-24 * time.Hour),
		Source:    "user",
	})
	// Add recent entry
	sm.entries = append(sm.entries, MemoryEntry{
		Category:  "state",
		Content:   "Recent state about authentication module",
		Timestamp: time.Now(),
		Source:    "user",
	})
	sm.mu.Unlock()

	opts := DefaultSearchOptions()
	opts.RecencyWeight = 0.5
	results := sm.SmartSearch("authentication module", opts)

	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	// Both should match, but scores should be close
	t.Logf("Result 0: %s (score: %.2f)", results[0].Entry.Content, results[0].Score)
	t.Logf("Result 1: %s (score: %.2f)", results[1].Entry.Content, results[1].Score)
}

func TestSmartSearch_Limit(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Task 1", "user")
	sm.AddNote("state", "Task 2", "user")
	sm.AddNote("state", "Task 3", "user")

	opts := DefaultSearchOptions()
	opts.Limit = 2
	results := sm.SmartSearch("Task", opts)

	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestContextSearch(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("preference", "Use Go for backend development", "user")
	sm.AddNote("decision", "SQLite for database", "user")
	sm.AddNote("state", "Working on REST API", "user")

	context := []string{
		"I need to implement the REST API using Go",
		"The database should use SQLite",
	}

	results := sm.ContextSearch(context, 5)
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
}

func TestExtractKeywords(t *testing.T) {
	messages := []string{
		"I need to implement the REST API using Go",
		"The database should use SQLite for persistence",
		"Make sure to add unit tests for all endpoints",
	}

	keywords := extractKeywords(messages)
	if len(keywords) == 0 {
		t.Fatal("expected at least 1 keyword")
	}

	// Should contain significant words
	found := false
	for _, kw := range keywords {
		if kw == "rest" || kw == "api" || kw == "sqlite" || kw == "database" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find significant keywords, got %v", keywords)
	}
}

func TestIsStopWord(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"the", true},
		{"is", true},
		{"a", true},
		{"go", false},
		{"sqlite", false},
		{"implement", false},
	}

	for _, tt := range tests {
		if isStopWord(tt.word) != tt.expected {
			t.Errorf("isStopWord(%q) = %v, want %v", tt.word, isStopWord(tt.word), tt.expected)
		}
	}
}

func TestGetRelevantMemories(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("preference", "Use Go 1.25 for all projects", "user")
	sm.AddNote("decision", "Use SQLite for persistence", "user")
	sm.AddNote("state", "Working on REST API", "user")

	result := sm.GetRelevantMemories("implement REST API with Go and SQLite", 5)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !strings.Contains(result, "Relevant Memories") {
		t.Error("expected 'Relevant Memories' header")
	}
}

func TestGetRelevantMemories_NoMatch(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("preference", "Use Go 1.25 for all projects", "user")

	// Search for something completely unrelated
	result := sm.GetRelevantMemories("quantum physics theory", 5)
	// May return low-relevance results or empty
	t.Logf("Result: %q", result)
}

func TestSearchOptions_Defaults(t *testing.T) {
	opts := DefaultSearchOptions()
	if opts.RecencyWeight != 0.3 {
		t.Errorf("expected RecencyWeight 0.3, got %.2f", opts.RecencyWeight)
	}
	if opts.ExactMatchBonus != 0.5 {
		t.Errorf("expected ExactMatchBonus 0.5, got %.2f", opts.ExactMatchBonus)
	}
	if opts.MinScore != 0.1 {
		t.Errorf("expected MinScore 0.1, got %.2f", opts.MinScore)
	}
}

func TestSmartSearch_MultipleScopes(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, ".claude", "memory", "global.md")
	sm := NewSessionMemoryWithPaths(dir, globalPath)

	// Add to different scopes
	sm.addGlobalNote("preference", "Use Go globally", "user")
	sm.addProjectNote("decision", "Use SQLite for this project", "user")
	sm.AddNote("state", "Working on API", "user")

	opts := DefaultSearchOptions()
	opts.Limit = 10
	results := sm.SmartSearch("Go", opts)

	// Should find global entry
	foundGlobal := false
	for _, r := range results {
		if r.Scope == ScopeGlobal {
			foundGlobal = true
			break
		}
	}
	if !foundGlobal {
		t.Error("expected to find global entry")
	}
}

// ─── Memory Consolidation Tests ─────────────────────────────────────────────

func TestConsolidateMemory_Basic(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Add some entries
	sm.AddNote("state", "Task 1", "user")
	sm.AddNote("state", "Task 2", "user")
	sm.AddNote("decision", "Use Go", "user")

	config := DefaultConsolidationConfig()
	result := sm.ConsolidateMemory(config)

	if result.Remaining < 3 {
		t.Errorf("expected at least 3 remaining entries, got %d", result.Remaining)
	}
}

func TestConsolidateMemory_Deduplication(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Directly add duplicate entries (bypass AddNote's fuzzy dedup)
	sm.mu.Lock()
	sm.entries = []MemoryEntry{
		{Category: "state", Content: "Working on authentication", Timestamp: time.Now(), Version: 1},
		{Category: "state", Content: "Working on authentication", Timestamp: time.Now(), Version: 1},
		{Category: "state", Content: "Working on authentication", Timestamp: time.Now(), Version: 1},
	}
	sm.mu.Unlock()

	config := DefaultConsolidationConfig()
	result := sm.ConsolidateMemory(config)

	if result.Merged < 2 {
		t.Errorf("expected at least 2 merges, got %d", result.Merged)
	}

	notes := sm.GetScopedNotes(ScopeSession)
	stateCount := 0
	for _, n := range notes {
		if n.Category == "state" {
			stateCount++
		}
	}
	if stateCount != 1 {
		t.Errorf("expected 1 state entry after dedup, got %d", stateCount)
	}
}

func TestConsolidateMemory_ExpiredEntries(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Add expired entry
	sm.mu.Lock()
	sm.entries = append(sm.entries, MemoryEntry{
		Category:  "state",
		Content:   "Old state",
		Timestamp: time.Now().Add(-8 * 24 * time.Hour), // 8 days old (state expires in 7)
		Source:    "user",
	})
	sm.entries = append(sm.entries, MemoryEntry{
		Category:  "state",
		Content:   "Recent state",
		Timestamp: time.Now(),
		Source:    "user",
	})
	sm.mu.Unlock()

	config := DefaultConsolidationConfig()
	result := sm.ConsolidateMemory(config)

	if result.Removed < 1 {
		t.Errorf("expected at least 1 removal, got %d", result.Removed)
	}
}

func TestConsolidateMemory_CompressOld(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Directly add old entries
	sm.mu.Lock()
	for i := 0; i < 10; i++ {
		sm.entries = append(sm.entries, MemoryEntry{
			Category:  "state",
			Content:   fmt.Sprintf("Working on authentication module for the app %d", i),
			Timestamp: time.Now().Add(-8 * 24 * time.Hour),
			Source:    "user",
			Version:   1,
		})
	}
	sm.mu.Unlock()

	config := DefaultConsolidationConfig()
	config.CompressOlderThan = 7 * 24 * time.Hour
	config.KeepRecent = 0
	result := sm.ConsolidateMemory(config)

	// Should have compressed some entries
	t.Logf("Compressed: %d, Merged: %d, Removed: %d, Remaining: %d",
		result.Compressed, result.Merged, result.Removed, result.Remaining)
}

func TestConsolidateMemory_MaxEntries(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Add many entries
	for i := 0; i < 50; i++ {
		sm.AddNote("state", fmt.Sprintf("State %d", i), "user")
	}

	config := DefaultConsolidationConfig()
	config.MaxEntriesPerCategory = 20
	sm.ConsolidateMemory(config)

	stateCount := 0
	notes := sm.GetScopedNotes(ScopeSession)
	for _, n := range notes {
		if n.Category == "state" {
			stateCount++
		}
	}
	if stateCount > 20 {
		t.Errorf("expected at most 20 state entries, got %d", stateCount)
	}
}

func TestGetMemoryHealth(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working on auth", "user")
	sm.AddNote("decision", "Use Go", "user")
	sm.AddNote("preference", "Dark theme", "user")

	health := sm.GetMemoryHealth()

	if health.TotalEntries < 3 {
		t.Errorf("expected at least 3 total entries, got %d", health.TotalEntries)
	}
	if health.Score < 0 || health.Score > 100 {
		t.Errorf("expected score 0-100, got %d", health.Score)
	}
}

func TestFormatConsolidationReport(t *testing.T) {
	result := ConsolidationResult{
		Merged:     5,
		Removed:    2,
		Compressed: 3,
		Remaining:  20,
	}
	health := MemoryHealth{
		TotalEntries:   20,
		ExpiredEntries: 2,
		Score:          85,
	}

	report := FormatConsolidationReport(result, health)

	if !strings.Contains(report, "Merged: 5") {
		t.Error("expected 'Merged: 5' in report")
	}
	if !strings.Contains(report, "Health Score: 85/100") {
		t.Error("expected 'Health Score: 85/100' in report")
	}
}

func TestDefaultConsolidationConfig(t *testing.T) {
	config := DefaultConsolidationConfig()

	if config.MaxEntriesPerCategory <= 0 {
		t.Error("expected positive MaxEntriesPerCategory")
	}
	if config.SimilarityThreshold <= 0 || config.SimilarityThreshold > 1 {
		t.Errorf("expected SimilarityThreshold 0-1, got %.2f", config.SimilarityThreshold)
	}
	if config.CompressOlderThan <= 0 {
		t.Error("expected positive CompressOlderThan")
	}
	if config.KeepRecent <= 0 {
		t.Error("expected positive KeepRecent")
	}
}

func TestRemoveExpired(t *testing.T) {
	entries := []MemoryEntry{
		{Category: "state", Content: "Recent", Timestamp: time.Now()},
		{Category: "state", Content: "Old", Timestamp: time.Now().Add(-8 * 24 * time.Hour)},
		{Category: "decision", Content: "Recent decision", Timestamp: time.Now()},
	}

	result, removed := removeExpired(entries)
	if removed != 1 {
		t.Errorf("expected 1 removal, got %d", removed)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 remaining, got %d", len(result))
	}
}

func TestEnforceMaxEntries(t *testing.T) {
	entries := make([]MemoryEntry, 30)
	for i := range entries {
		entries[i] = MemoryEntry{
			Category:  "state",
			Content:   fmt.Sprintf("Entry %d", i),
			Timestamp: time.Now().Add(-time.Duration(i) * time.Hour),
		}
	}

	result := enforceMaxEntries(entries, 10)
	if len(result) > 10 {
		t.Errorf("expected at most 10 entries, got %d", len(result))
	}
}

func TestCalculateHealthScore(t *testing.T) {
	tests := []struct {
		name    string
		health  MemoryHealth
		minScore int
		maxScore int
	}{
		{"healthy", MemoryHealth{TotalEntries: 20, ExpiredEntries: 0, OldEntries: 0}, 90, 100},
		{"too many entries", MemoryHealth{TotalEntries: 250, ExpiredEntries: 0, OldEntries: 0}, 70, 85},
		{"expired entries", MemoryHealth{TotalEntries: 20, ExpiredEntries: 15, OldEntries: 0}, 75, 90},
		{"old entries", MemoryHealth{TotalEntries: 20, ExpiredEntries: 0, OldEntries: 60}, 75, 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calculateHealthScore(tt.health)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("expected score %d-%d, got %d", tt.minScore, tt.maxScore, score)
			}
		})
	}
}

// ─── Search & Filtering Tests ────────────────────────────────────────────────

func TestFilteredSearch_ByCategory(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working on auth", "user")
	sm.AddNote("decision", "Use Go", "user")
	sm.AddNote("preference", "Dark theme", "user")

	filter := MemoryFilter{
		Categories: []string{"state"},
	}
	results := sm.FilteredSearch(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if len(results) > 0 && results[0].Entry.Category != "state" {
		t.Errorf("expected category 'state', got '%s'", results[0].Entry.Category)
	}
}

func TestFilteredSearch_BySource(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working on auth", "user")
	sm.AddNote("decision", "Use Go", "assistant")
	sm.AddNote("preference", "Dark theme", "auto")

	filter := MemoryFilter{
		Sources: []string{"user"},
	}
	results := sm.FilteredSearch(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestFilteredSearch_ByTimeRange(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Add old entry
	sm.mu.Lock()
	sm.entries = append(sm.entries, MemoryEntry{
		Category:  "state",
		Content:   "Old state",
		Timestamp: time.Now().Add(-24 * time.Hour),
		Source:    "user",
	})
	// Add recent entry
	sm.entries = append(sm.entries, MemoryEntry{
		Category:  "state",
		Content:   "Recent state",
		Timestamp: time.Now(),
		Source:    "user",
	})
	sm.mu.Unlock()

	after := time.Now().Add(-1 * time.Hour)
	filter := MemoryFilter{
		After: &after,
	}
	results := sm.FilteredSearch(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if len(results) > 0 && results[0].Entry.Content != "Recent state" {
		t.Errorf("expected 'Recent state', got '%s'", results[0].Entry.Content)
	}
}

func TestFilteredSearch_WithQuery(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working on authentication module", "user")
	sm.AddNote("decision", "Use SQLite for persistence", "user")

	filter := MemoryFilter{
		Query: "authentication",
	}
	results := sm.FilteredSearch(filter)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestFilteredSearch_SortByTime(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.mu.Lock()
	sm.entries = []MemoryEntry{
		{Category: "state", Content: "Old", Timestamp: time.Now().Add(-time.Hour), Source: "user"},
		{Category: "state", Content: "New", Timestamp: time.Now(), Source: "user"},
	}
	sm.mu.Unlock()

	filter := MemoryFilter{
		SortBy:   "time",
		SortDesc: true,
	}
	results := sm.FilteredSearch(filter)

	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	if results[0].Entry.Content != "New" {
		t.Errorf("expected 'New' first (desc), got '%s'", results[0].Entry.Content)
	}
}

func TestFilteredSearch_Limit(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Task 1", "user")
	sm.AddNote("state", "Task 2", "user")
	sm.AddNote("state", "Task 3", "user")

	filter := MemoryFilter{
		Limit: 2,
	}
	results := sm.FilteredSearch(filter)

	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestGetCategories(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working", "user")
	sm.AddNote("decision", "Use Go", "user")
	sm.AddNote("preference", "Dark theme", "user")

	categories := sm.GetCategories()
	if len(categories) < 3 {
		t.Errorf("expected at least 3 categories, got %d", len(categories))
	}
}

func TestGetSources(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working", "user")
	sm.AddNote("decision", "Use Go", "assistant")

	sources := sm.GetSources()
	if len(sources) < 2 {
		t.Errorf("expected at least 2 sources, got %d", len(sources))
	}
}

func TestSearchByCategory(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working on auth", "user")
	sm.AddNote("state", "Working on API", "user")
	sm.AddNote("decision", "Use Go", "user")

	results := sm.SearchByCategory("state")
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestSearchBySource(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working", "user")
	sm.AddNote("decision", "Use Go", "assistant")

	results := sm.SearchBySource("user")
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestSearchByTimeRange(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.mu.Lock()
	sm.entries = []MemoryEntry{
		{Category: "state", Content: "Old", Timestamp: time.Now().Add(-2 * time.Hour), Source: "user"},
		{Category: "state", Content: "Recent", Timestamp: time.Now(), Source: "user"},
	}
	sm.mu.Unlock()

	after := time.Now().Add(-1 * time.Hour)
	before := time.Now().Add(1 * time.Hour)
	results := sm.SearchByTimeRange(after, before)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestGetRecentEntries(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Task 1", "user")
	sm.AddNote("state", "Task 2", "user")
	sm.AddNote("state", "Task 3", "user")

	results := sm.GetRecentEntries(2)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestGetTimeRange(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.mu.Lock()
	sm.entries = []MemoryEntry{
		{Category: "state", Content: "Old", Timestamp: time.Now().Add(-time.Hour), Source: "user"},
		{Category: "state", Content: "New", Timestamp: time.Now(), Source: "user"},
	}
	sm.mu.Unlock()

	earliest, latest := sm.GetTimeRange()
	if earliest.After(latest) {
		t.Error("earliest should be before latest")
	}
}

func TestFormatSearchResults(t *testing.T) {
	results := []SearchResult{
		{
			Entry:     MemoryEntry{Category: "state", Content: "Working on auth", Timestamp: time.Now()},
			Score:     0.8,
			Scope:     ScopeSession,
			MatchType: "exact",
		},
	}

	output := FormatSearchResults(results, "auth")
	if !strings.Contains(output, "Working on auth") {
		t.Error("expected to contain 'Working on auth'")
	}
	if !strings.Contains(output, "80.0%") {
		t.Error("expected to contain score")
	}
}

func TestFormatSearchResults_NoResults(t *testing.T) {
	output := FormatSearchResults(nil, "nonexistent")
	if !strings.Contains(output, "No results found") {
		t.Error("expected 'No results found'")
	}
}

// ─── Session Checkpoint Tests ────────────────────────────────────────────────

func TestWriteCheckpoint(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working on auth", "user")
	sm.AddNote("decision", "Use Go", "user")

	checkpointID, err := sm.WriteCheckpoint(nil)
	if err != nil {
		t.Fatal(err)
	}
	if checkpointID == "" {
		t.Error("expected non-empty checkpoint ID")
	}

	// Verify checkpoint file exists
	checkpointPath := filepath.Join(dir, ".claude", "checkpoints", checkpointID+".json")
	if _, err := os.Stat(checkpointPath); os.IsNotExist(err) {
		t.Error("checkpoint file should exist")
	}
}

func TestLoadCheckpoint(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working on auth", "user")
	checkpointID, _ := sm.WriteCheckpoint(nil)

	// Load checkpoint
	checkpoint, err := sm.LoadCheckpoint(checkpointID)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.ID != checkpointID {
		t.Errorf("expected checkpoint ID '%s', got '%s'", checkpointID, checkpoint.ID)
	}
	if len(checkpoint.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(checkpoint.Entries))
	}
}

func TestListCheckpoints(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Create checkpoint
	sm.AddNote("state", "State 1", "user")
	sm.WriteCheckpoint(nil)

	// Wait a bit to ensure different timestamp
	time.Sleep(1100 * time.Millisecond)

	// Create another checkpoint
	sm.AddNote("state", "State 2", "user")
	sm.WriteCheckpoint(nil)

	checkpoints := sm.ListCheckpoints()
	if len(checkpoints) < 1 {
		t.Errorf("expected at least 1 checkpoint, got %d", len(checkpoints))
	}
}

func TestRevertToCheckpoint(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Create checkpoint with initial state
	sm.AddNote("state", "Initial state", "user")
	checkpointID, _ := sm.WriteCheckpoint(nil)

	// Modify state
	sm.AddNote("state", "Modified state", "user")
	if len(sm.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(sm.entries))
	}

	// Revert to checkpoint
	err := sm.RevertToCheckpoint(checkpointID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify reverted state
	if len(sm.entries) != 1 {
		t.Errorf("expected 1 entry after revert, got %d", len(sm.entries))
	}
	if sm.entries[0].Content != "Initial state" {
		t.Errorf("expected 'Initial state', got '%s'", sm.entries[0].Content)
	}
}

func TestGetCheckpointSummary(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working on auth", "user")
	sm.AddNote("decision", "Use Go", "user")

	summary := sm.GetCheckpointSummary()
	if !strings.Contains(summary, "Session Checkpoint") {
		t.Error("expected 'Session Checkpoint' header")
	}
	if !strings.Contains(summary, "Session Notes") {
		t.Error("expected 'Session Notes' section")
	}
}

func TestCheckpointPersistence(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	sm.AddNote("state", "Working on auth", "user")
	checkpointID, _ := sm.WriteCheckpoint(nil)

	// Create new session memory instance
	sm2 := NewSessionMemoryWithPaths(dir, filepath.Join(dir, ".claude", "memory", "global.md"))

	// Load checkpoint from new instance
	checkpoint, err := sm2.LoadCheckpoint(checkpointID)
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoint.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(checkpoint.Entries))
	}
}
