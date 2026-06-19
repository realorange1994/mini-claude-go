package main

import (
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
