package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── End-to-End Integration Tests ──────────────────────────────────────────
// Verify module interactions work correctly together.

func TestE2E_MemoryFTSIntegration(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)

	// Add notes across scopes
	sm.AddNote("state", "Working on authentication module", "test")
	sm.AddNote("decision", "Use Go 1.25 for all projects", "test")
	sm.AddNote("preference", "Dark theme preferred", "test")

	// Search should find relevant entries
	results := sm.SearchMemory("authentication", 10)
	if len(results) == 0 {
		t.Fatal("expected FTS results for 'authentication'")
	}
	if results[0].Content != "Working on authentication module" {
		t.Errorf("expected 'Working on authentication module', got %q", results[0].Content)
	}

	// Add more notes
	sm.AddNote("state", "Authentication module completed", "test")

	// Search should reflect updates
	results = sm.SearchMemory("authentication", 10)
	if len(results) < 1 {
		t.Fatal("expected FTS results after update")
	}
}

func TestE2E_MemoryFTSWithScopedNotes(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)

	// Add notes to different scopes
	sm.AddScopedNote(ScopeSession, "state", "Session state note", "test")
	sm.AddScopedNote(ScopeProject, "decision", "Project decision", "test")
	sm.AddScopedNote(ScopeGlobal, "preference", "Global preference", "test")

	// Search should find across all scopes
	results := sm.SearchMemory("note", 10)
	if len(results) < 1 {
		t.Fatal("expected FTS results across scopes")
	}

	// Verify scope filtering
	sessionResults := sm.SearchMemory("Session", 10)
	found := false
	for _, r := range sessionResults {
		if r.Scope == ScopeSession {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find session-scoped result")
	}
}

func TestE2E_TaskGateWithWorkTaskStore(t *testing.T) {
	dir := t.TempDir()
	store := NewWorkTaskStore(dir)

	// Create tasks
	store.CreateTask("Implement auth", "", "", nil)
	store.CreateTask("Write tests", "", "", nil)

	// Gate should detect incomplete tasks
	decision := TaskGateDecide(TaskGateInput{
		TaskStore:  store,
		ReactCount: 0,
		MaxReact:   3,
		Mode:       GateModeMain,
	})

	if !decision.NeedReentry {
		t.Error("expected reentry with incomplete tasks")
	}
	if len(decision.IncompleteTasks) != 2 {
		t.Errorf("expected 2 incomplete tasks, got %d", len(decision.IncompleteTasks))
	}

	// Complete one task
	tasks := store.ListActiveTasks()
	if len(tasks) > 0 {
		store.TransitionTo(tasks[0].ID, WorkTaskInProgress, "started")
		store.TransitionTo(tasks[0].ID, WorkTaskCompleted, "done")
	}

	// Gate should still detect remaining task
	decision = TaskGateDecide(TaskGateInput{
		TaskStore:  store,
		ReactCount: 0,
		MaxReact:   3,
		Mode:       GateModeMain,
	})

	if !decision.NeedReentry {
		t.Error("expected reentry with remaining task")
	}
	if len(decision.IncompleteTasks) != 1 {
		t.Errorf("expected 1 incomplete task, got %d", len(decision.IncompleteTasks))
	}

	// Complete all tasks
	tasks = store.ListActiveTasks()
	for _, task := range tasks {
		store.TransitionTo(task.ID, WorkTaskInProgress, "started")
		store.TransitionTo(task.ID, WorkTaskCompleted, "done")
	}

	// Gate should not trigger
	decision = TaskGateDecide(TaskGateInput{
		TaskStore:  store,
		ReactCount: 0,
		MaxReact:   3,
		Mode:       GateModeMain,
	})

	if decision.NeedReentry {
		t.Error("expected no reentry when all tasks completed")
	}
}

func TestE2E_BudgetedReadWithCheckpoint(t *testing.T) {
	dir := t.TempDir()

	// Create a checkpoint file
	checkpointContent := `# Session Checkpoint

## §1 Goal
Implement authentication module

## §2 Instructions
Use Go 1.25, follow MiMo-Code patterns

## §3 Discoveries
- FTS index works well for memory search
- Task gate prevents incomplete work

## §4 Accomplished
- P1-P9 all implemented
- Integration complete

## §5 Relevant files
- session_memory.go
- agent_loop.go

## §6 Conversation tail
User: P9完成了，接着做P10
Assistant: P1-P9 已全部完成...`

	// Write large checkpoint
	largeContent := checkpointContent + strings.Repeat("\nExtra content line.\n", 100)
	checkpointPath := dir + "/checkpoint.md"
	writeTestFile(checkpointPath, largeContent)

	// Budgeted read should truncate
	result, err := ReadBudgeted(checkpointPath, 500)
	if err != nil {
		t.Fatalf("read budgeted: %v", err)
	}

	if !result.Truncated {
		t.Error("expected truncation for large checkpoint")
	}

	// Section-aware read should preserve structure
	result2, err := ReadBudgetedSectionAware(checkpointPath, 500)
	if err != nil {
		t.Fatalf("read budgeted section aware: %v", err)
	}

	if !result2.Truncated {
		t.Error("expected truncation for section-aware read")
	}
	if !strings.Contains(result2.Text, "## §1 Goal") {
		t.Error("expected section headers to be preserved")
	}
}

func TestE2E_AutoDreamWithSessionMemory(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)

	// Add notes
	sm.AddNote("state", "Working on auth", "test")
	sm.AddNote("decision", "Use Go 1.25", "test")
	sm.AddNote("error", "Build failed", "test")

	// Create auto-dream manager
	manager := NewAutoDreamManager(dir)

	// Should not auto-dream (project too young)
	if manager.ShouldAutoDream() {
		t.Error("expected no auto-dream for young project")
	}

	// Verify FTS works with auto-dream context
	results := sm.SearchMemory("auth", 10)
	if len(results) == 0 {
		t.Error("expected FTS results")
	}

	// Verify search across scopes
	results = sm.SearchMemory("Go 1.25", 10)
	if len(results) == 0 {
		t.Error("expected FTS results for version")
	}
}

func TestE2E_AgentIDWithMessages(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// Add main agent message
	ctx.AddUserMessage("hello from main")
	ctx.AddAssistantText("response from main")

	// Add subagent message
	ctx.mu.Lock()
	ctx.entries = append(ctx.entries, conversationEntry{
		role:    "assistant",
		content: TextContent("response from subagent"),
		agentID: "agent-1",
	})
	ctx.mu.Unlock()

	// Build messages for main agent
	mainMsgs := ctx.BuildMessagesForAgent("main")
	if len(mainMsgs) != 2 {
		t.Errorf("expected 2 main messages, got %d", len(mainMsgs))
	}

	// Build messages for subagent
	subMsgs := ctx.BuildMessagesForAgent("agent-1")
	if len(subMsgs) != 1 {
		t.Errorf("expected 1 subagent message, got %d", len(subMsgs))
	}

	// Build messages for all
	allMsgs := ctx.BuildMessagesForAgent("*")
	if len(allMsgs) != 3 { // 2 main + 1 subagent
		t.Errorf("expected 3 messages for all, got %d", len(allMsgs))
	}
}

// Helper to write test files
func writeTestFile(path, content string) {
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(content), 0644)
}
