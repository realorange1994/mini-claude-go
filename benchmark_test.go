package main

import (
	"testing"
)

// ─── Benchmark Tests ──────────────────────────────────────────────────────
// Performance benchmarks for critical paths.

func BenchmarkFTSIndex_Search(b *testing.B) {
	idx := NewMemoryFTSIndex()

	// Add 1000 entries
	for i := 0; i < 1000; i++ {
		idx.Index(ScopeSession, "test", "This is test entry number "+string(rune(i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search("test", 10)
	}
}

func BenchmarkFTSIndex_Index(b *testing.B) {
	idx := NewMemoryFTSIndex()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Index(ScopeSession, "test", "This is test entry number "+string(rune(i)))
	}
}

func BenchmarkConversationContext_BuildMessages(b *testing.B) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// Add 100 messages
	for i := 0; i < 100; i++ {
		ctx.AddUserMessage("test message " + string(rune(i)))
		ctx.AddAssistantText("response " + string(rune(i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.BuildMessages()
	}
}

func BenchmarkConversationContext_AddUserMessage(b *testing.B) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.AddUserMessage("test message")
	}
}

func BenchmarkWorkTaskStore_CreateTask(b *testing.B) {
	store := NewWorkTaskStore(b.TempDir())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.CreateTask("Task "+string(rune(i)), "", "", nil)
	}
}

func BenchmarkWorkTaskStore_ListActiveTasks(b *testing.B) {
	store := NewWorkTaskStore(b.TempDir())

	// Add 100 tasks
	for i := 0; i < 100; i++ {
		store.CreateTask("Task "+string(rune(i)), "", "", nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.ListActiveTasks()
	}
}

func BenchmarkSessionMemory_AddNote(b *testing.B) {
	dir := b.TempDir()
	sm := NewSessionMemory(dir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sm.AddNote("test", "Test note content", "test")
	}
}

func BenchmarkSessionMemory_SearchMemory(b *testing.B) {
	dir := b.TempDir()
	sm := NewSessionMemory(dir)

	// Add 100 notes
	for i := 0; i < 100; i++ {
		sm.AddNote("test", "Test note content "+string(rune(i)), "test")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sm.SearchMemory("test", 10)
	}
}

func BenchmarkStepClassification(b *testing.B) {
	toolCalls := []map[string]any{{"name": "bash"}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClassifyAssistantStep("test", toolCalls, "", nil)
	}
}

func BenchmarkDoomLoopDetector_CheckRecord(b *testing.B) {
	detector := NewDoomLoopDetector()
	args := map[string]any{"command": "ls"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.CheckRecord("bash", args)
	}
}
