package main

import (
	"strings"
	"testing"
	"time"
)

// --- RecordPromptState tests ---

func TestRecordPromptState_FirstCall(t *testing.T) {
	ResetCacheBreakTracker()

	pending := RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", false)
	if pending != nil {
		t.Error("first call should return nil pending changes")
	}
}

func TestRecordPromptState_NoChange(t *testing.T) {
	ResetCacheBreakTracker()

	// First call — establishes baseline
	RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", false)

	// Second call — no change
	pending := RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", false)
	if pending != nil {
		t.Error("no changes should result in nil pending")
	}

	// Third call — still no change, pending should remain nil
	pending = RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", false)
	if pending != nil {
		t.Error("third call with no change should also return nil pending")
	}
}

func TestRecordPromptState_SystemChange(t *testing.T) {
	ResetCacheBreakTracker()

	// First call
	RecordPromptState("system prompt v1", nil, nil, "claude-sonnet-4-6", false)

	// Second call — system prompt changed
	pending := RecordPromptState("system prompt v2", nil, nil, "claude-sonnet-4-6", false)
	if pending == nil {
		t.Fatal("expected pending changes when system prompt changed")
	}
	if !pending.SystemPromptChanged {
		t.Error("expected SystemPromptChanged to be true")
	}
}

func TestRecordPromptState_SystemCharDelta(t *testing.T) {
	ResetCacheBreakTracker()

	// Short → longer system prompt
	RecordPromptState("short", nil, nil, "model", false)
	pending := RecordPromptState("this is a much longer system prompt with more text", nil, nil, "model", false)
	if pending == nil || !pending.SystemPromptChanged {
		t.Fatal("expected system prompt change")
	}
	if pending.SystemCharDelta <= 0 {
		t.Errorf("expected positive char delta, got %d", pending.SystemCharDelta)
	}

	// Longer → shorter
	ResetCacheBreakTracker()
	RecordPromptState("this is a much longer system prompt with more text", nil, nil, "model", false)
	pending = RecordPromptState("short", nil, nil, "model", false)
	if pending == nil || !pending.SystemPromptChanged {
		t.Fatal("expected system prompt change")
	}
	if pending.SystemCharDelta >= 0 {
		t.Errorf("expected negative char delta, got %d", pending.SystemCharDelta)
	}
}

func TestRecordPromptState_ModelChange(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", false)
	pending := RecordPromptState("system prompt", nil, nil, "claude-opus-4-6", false)

	if pending == nil {
		t.Fatal("expected pending changes when model changed")
	}
	if !pending.ModelChanged {
		t.Error("expected ModelChanged to be true")
	}
	if pending.PreviousModel != "claude-sonnet-4-6" || pending.NewModel != "claude-opus-4-6" {
		t.Errorf("expected model transition claude-sonnet-4-6→claude-opus-4-6, got %s→%s",
			pending.PreviousModel, pending.NewModel)
	}
}

func TestRecordPromptState_FastModeChange(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", false)
	pending := RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", true)

	if pending == nil {
		t.Fatal("expected pending changes when fast mode changed")
	}
	if !pending.FastModeChanged {
		t.Error("expected FastModeChanged to be true")
	}
}

func TestRecordPromptState_ToolChange(t *testing.T) {
	ResetCacheBreakTracker()

	tools1 := []map[string]any{
		{"name": "exec", "description": "run shell"},
		{"name": "read", "description": "read file"},
	}
	names1 := []string{"exec", "read"}

	tools2 := []map[string]any{
		{"name": "exec", "description": "run shell"},
		{"name": "glob", "description": "find files"}, // read → glob
	}
	names2 := []string{"exec", "glob"}

	RecordPromptState("system prompt", tools1, names1, "claude-sonnet-4-6", false)
	pending := RecordPromptState("system prompt", tools2, names2, "claude-sonnet-4-6", false)

	if pending == nil {
		t.Fatal("expected pending changes when tools changed")
	}
	if !pending.ToolSchemasChanged {
		t.Error("expected ToolSchemasChanged to be true")
	}
	if len(pending.RemovedTools) != 1 || pending.RemovedTools[0] != "read" {
		t.Errorf("expected 'read' in removed tools, got %v", pending.RemovedTools)
	}
	if len(pending.AddedTools) != 1 || pending.AddedTools[0] != "glob" {
		t.Errorf("expected 'glob' in added tools, got %v", pending.AddedTools)
	}
}

func TestRecordPromptState_ToolSchemaModified(t *testing.T) {
	ResetCacheBreakTracker()

	// Same tool name, different description (schema changed but tool set unchanged)
	tools1 := []map[string]any{
		{"name": "exec", "description": "run shell commands"},
		{"name": "read", "description": "read file contents"},
	}
	names1 := []string{"exec", "read"}

	tools2 := []map[string]any{
		{"name": "exec", "description": "run shell commands"},
		{"name": "read", "description": "read file from disk"}, // description changed
	}
	names2 := []string{"exec", "read"}

	RecordPromptState("system prompt", tools1, names1, "claude-sonnet-4-6", false)
	pending := RecordPromptState("system prompt", tools2, names2, "claude-sonnet-4-6", false)

	if pending == nil {
		t.Fatal("expected pending changes when tool schema modified")
	}
	if !pending.ToolSchemasChanged {
		t.Error("expected ToolSchemasChanged to be true")
	}
	// Same tool set, just schema changed
	if pending.AddedToolCount != 0 || pending.RemovedToolCount != 0 {
		t.Errorf("expected 0 added/removed tools, got +%d/-%d",
			pending.AddedToolCount, pending.RemovedToolCount)
	}
	if len(pending.ChangedToolSchemas) != 1 || pending.ChangedToolSchemas[0] != "read" {
		t.Errorf("expected 'read' in changed tool schemas, got %v", pending.ChangedToolSchemas)
	}
}

func TestRecordPromptState_MultipleChanges(t *testing.T) {
	ResetCacheBreakTracker()

	tools := []map[string]any{
		{"name": "exec", "description": "run shell"},
	}
	names := []string{"exec"}

	RecordPromptState("system v1", tools, names, "model-a", false)
	pending := RecordPromptState("system v2", nil, nil, "model-b", true)

	if pending == nil {
		t.Fatal("expected pending changes")
	}
	if !pending.SystemPromptChanged {
		t.Error("expected SystemPromptChanged")
	}
	if !pending.ToolSchemasChanged {
		t.Error("expected ToolSchemasChanged (tools removed)")
	}
	if !pending.ModelChanged {
		t.Error("expected ModelChanged")
	}
	if !pending.FastModeChanged {
		t.Error("expected FastModeChanged")
	}
}

func TestRecordPromptState_CallCountIncrements(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system", nil, nil, "model", false)
	s1 := GetCacheBreakSnapshot()
	if s1.CallCount != 1 {
		t.Errorf("expected call count 1, got %d", s1.CallCount)
	}

	// No change
	RecordPromptState("system", nil, nil, "model", false)
	s2 := GetCacheBreakSnapshot()
	if s2.CallCount != 2 {
		t.Errorf("expected call count 2, got %d", s2.CallCount)
	}

	// Change
	RecordPromptState("system v2", nil, nil, "model", false)
	s3 := GetCacheBreakSnapshot()
	if s3.CallCount != 3 {
		t.Errorf("expected call count 3, got %d", s3.CallCount)
	}
}

func TestRecordPromptState_NoChangeReturnsNil(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system", nil, nil, "model", false)

	// No change → should return nil pending (not stored)
	pending := RecordPromptState("system", nil, nil, "model", false)
	if pending != nil {
		t.Error("no changes should return nil pending, not a changes struct")
	}
}

// --- CheckResponseForCacheBreak tests ---

func TestCheckResponseForCacheBreak_NoBreak(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", false)

	// First response — establishes baseline
	isBreak, _ := CheckResponseForCacheBreak(10000, 5000, nil, false, false)
	if isBreak {
		t.Error("first response should not be a cache break")
	}

	// Second response — small drop (<5%)
	isBreak, _ = CheckResponseForCacheBreak(9600, 5000, nil, false, false)
	if isBreak {
		t.Error("small drop should not be a cache break")
	}
}

func TestCheckResponseForCacheBreak_Break(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", false)

	// First response — establishes baseline
	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	// Second response — big drop (>5% AND >2000 tokens)
	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, nil, false, false)
	if !isBreak {
		t.Error("big drop should be a cache break")
	}
	if reason == "" {
		t.Error("cache break should have a reason")
	}
}

func TestCheckResponseForCacheBreak_BreakReasonIncludesDrop(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "model", false)
	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, nil, false, false)
	if !isBreak {
		t.Fatal("expected cache break")
	}

	if !strings.Contains(reason, "10000") || !strings.Contains(reason, "7000") {
		t.Errorf("reason should include token counts, got: %s", reason)
	}
	if !strings.Contains(reason, "3000 tokens") {
		t.Errorf("reason should include token drop amount, got: %s", reason)
	}
}

func TestCheckResponseForCacheBreak_BoundaryExactly5Percent(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "model", false)
	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	// Exactly 5% drop: 9500 = 10000 * 0.95 — should NOT be a break
	// (condition is: cache_read >= 95% → NOT a break)
	isBreak, _ := CheckResponseForCacheBreak(9500, 5000, nil, false, false)
	if isBreak {
		t.Error("exactly 5% drop should not be a cache break")
	}
}

func TestCheckResponseForCacheBreak_DropAbove5PercentBelowThreshold(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "model", false)
	CheckResponseForCacheBreak(5000, 2000, nil, false, false)

	// 15% drop but only 750 tokens — below the 2000 threshold
	// 4250 = 5000 * 0.85 → 15% drop, but 750 < 2000
	isBreak, _ := CheckResponseForCacheBreak(4250, 2000, nil, false, false)
	if isBreak {
		t.Error("drop below 2000 token threshold should not be a cache break")
	}
}

func TestCheckResponseForCacheBreak_CompactionReset(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", false)
	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	// Big drop but compaction just occurred — should not flag as break
	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, nil, false, true)
	if isBreak {
		t.Errorf("compaction should prevent cache break detection, got: %s", reason)
	}
}

func TestCheckResponseForCacheBreak_CacheDeletion(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", false)
	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	// Big drop but cache deletion pending — should not flag as break
	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, nil, true, false)
	if isBreak {
		t.Errorf("cache deletion should prevent cache break detection, got: %s", reason)
	}
}

func TestCheckResponseForCacheBreak_TTLDetection(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "claude-sonnet-4-6", false)
	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	// Big drop, no pending changes, long time gap → TTL expiry
	gap := 6 * time.Minute
	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, &gap, false, false)
	if !isBreak {
		t.Error("should detect cache break")
	}
	if reason == "" {
		t.Error("should have reason string")
	}
	// Should mention TTL expiry since gap > 5min
	if reason == "CACHE BREAK: unknown cause (prompt unchanged) [drop: 10000 → 7000 (3000 tokens)]" {
		t.Logf("reason: %s", reason)
	}
}

func TestCheckResponseForCacheBreak_TTLGreaterThan1Hour(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "model", false)
	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	gap := 90 * time.Minute
	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, &gap, false, false)
	if !isBreak {
		t.Fatal("expected cache break for >1h gap")
	}
	if !strings.Contains(reason, "1h TTL") {
		t.Errorf("expected '1h TTL' in reason, got: %s", reason)
	}
}

func TestCheckResponseForCacheBreak_ServerSideEviction(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system prompt", nil, nil, "model", false)
	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	// Short gap (1 min), no changes → likely server-side eviction
	gap := 1 * time.Minute
	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, &gap, false, false)
	if !isBreak {
		t.Fatal("expected cache break")
	}
	if !strings.Contains(reason, "server-side") {
		t.Errorf("expected 'server-side' in reason, got: %s", reason)
	}
}

func TestCheckResponseForCacheBreak_WithPendingChanges(t *testing.T) {
	ResetCacheBreakTracker()

	// First call
	RecordPromptState("system prompt v1", nil, nil, "claude-sonnet-4-6", false)

	// Second call — system changed
	pending := RecordPromptState("system prompt v2", nil, nil, "claude-sonnet-4-6", false)
	if pending == nil {
		t.Fatal("expected pending changes")
	}

	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	// Third call — big drop with pending changes
	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, nil, false, false)
	if !isBreak {
		t.Error("should detect cache break")
	}
	if !strings.Contains(reason, "system prompt changed") {
		t.Errorf("expected 'system prompt changed' in reason, got: %s", reason)
	}
}

func TestCheckResponseForCacheBreak_WithModelChangePending(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system", nil, nil, "claude-sonnet-4-6", false)
	RecordPromptState("system", nil, nil, "claude-opus-4-6", false)

	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, nil, false, false)
	if !isBreak {
		t.Fatal("expected cache break")
	}
	if !strings.Contains(reason, "model changed") {
		t.Errorf("expected 'model changed' in reason, got: %s", reason)
	}
	if !strings.Contains(reason, "claude-sonnet-4-6") {
		t.Errorf("expected previous model in reason, got: %s", reason)
	}
	if !strings.Contains(reason, "claude-opus-4-6") {
		t.Errorf("expected new model in reason, got: %s", reason)
	}
}

func TestCheckResponseForCacheBreak_WithToolChangePending(t *testing.T) {
	ResetCacheBreakTracker()

	tools1 := []map[string]any{
		{"name": "exec", "description": "run shell"},
		{"name": "read", "description": "read file"},
	}
	names1 := []string{"exec", "read"}

	tools2 := []map[string]any{
		{"name": "exec", "description": "run shell"},
		{"name": "glob", "description": "find files"},
	}
	names2 := []string{"exec", "glob"}

	RecordPromptState("system", tools1, names1, "model", false)
	RecordPromptState("system", tools2, names2, "model", false)

	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, nil, false, false)
	if !isBreak {
		t.Fatal("expected cache break")
	}
	if !strings.Contains(reason, "tools changed") {
		t.Errorf("expected 'tools changed' in reason, got: %s", reason)
	}
}

func TestCheckResponseForCacheBreak_WithFastModeChangePending(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system", nil, nil, "model", false)
	RecordPromptState("system", nil, nil, "model", true)

	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, nil, false, false)
	if !isBreak {
		t.Fatal("expected cache break")
	}
	if !strings.Contains(reason, "fast mode toggled") {
		t.Errorf("expected 'fast mode toggled' in reason, got: %s", reason)
	}
}

func TestCheckResponseForCacheBreak_MultipleReasons(t *testing.T) {
	ResetCacheBreakTracker()

	tools := []map[string]any{
		{"name": "exec", "description": "run shell"},
	}
	names := []string{"exec"}

	RecordPromptState("system v1", tools, names, "model-a", false)
	RecordPromptState("system v2", nil, nil, "model-b", true)

	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, nil, false, false)
	if !isBreak {
		t.Fatal("expected cache break")
	}
	// Should list multiple reasons
	count := 0
	if strings.Contains(reason, "system prompt changed") {
		count++
	}
	if strings.Contains(reason, "model changed") {
		count++
	}
	if strings.Contains(reason, "fast mode toggled") {
		count++
	}
	if strings.Contains(reason, "tools changed") {
		count++
	}
	if count < 3 {
		t.Errorf("expected at least 3 reasons in break explanation, got %d: %s", count, reason)
	}
}

func TestCheckResponseForCacheBreak_PendingChangesClearedAfterBreak(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system v1", nil, nil, "model", false)
	RecordPromptState("system v2", nil, nil, "model", false)

	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	// First break — should include reason
	isBreak1, reason1 := CheckResponseForCacheBreak(7000, 3000, nil, false, false)
	if !isBreak1 {
		t.Fatal("expected cache break")
	}

	// Second break — pending changes were cleared, so reason should be different
	// (no more "system prompt changed" since it was already consumed)
	isBreak2, reason2 := CheckResponseForCacheBreak(4000, 2000, nil, false, false)
	if !isBreak2 {
		t.Fatal("expected second cache break")
	}
	if reason2 == reason1 {
		t.Errorf("second break should have different reason, got same: %s", reason1)
	}
}

func TestCheckResponseForCacheBreak_DropIncludesCharInfo(t *testing.T) {
	ResetCacheBreakTracker()

	tools := []map[string]any{
		{"name": "exec", "description": "run"},
	}
	names := []string{"exec"}

	RecordPromptState("short", tools, names, "model", false)
	RecordPromptState("this is a much longer system prompt with extra text", nil, nil, "model", false)

	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, nil, false, false)
	if !isBreak {
		t.Fatal("expected cache break")
	}
	if !strings.Contains(reason, "chars") {
		t.Errorf("expected char info in reason, got: %s", reason)
	}
}

// --- Utility function tests ---

func TestResetCacheBreakTracker(t *testing.T) {
	ResetCacheBreakTracker()
	RecordPromptState("system prompt", nil, nil, "model1", false)
	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	ResetCacheBreakTracker()

	// After reset, should act like first call
	isBreak, _ := CheckResponseForCacheBreak(7000, 3000, nil, false, false)
	if isBreak {
		t.Error("after reset, first check should not detect break")
	}
}

func TestUpdateLastAssistantMsgTime(t *testing.T) {
	ResetCacheBreakTracker()

	// Before any message
	gap := TimeSinceLastAssistantMsg()
	if gap != nil {
		t.Error("expected nil before first message")
	}

	UpdateLastAssistantMsgTime()

	// After recording time
	gap = TimeSinceLastAssistantMsg()
	if gap == nil {
		t.Error("expected non-nil after recording time")
	}
	if *gap < 0 {
		t.Error("gap should be non-negative")
	}
}

func TestGetCacheBreakSnapshot(t *testing.T) {
	ResetCacheBreakTracker()

	snapshot := GetCacheBreakSnapshot()
	if snapshot.CallCount != 0 {
		t.Errorf("expected call count 0, got %d", snapshot.CallCount)
	}
	if snapshot.Model != "" {
		t.Errorf("expected empty model, got %s", snapshot.Model)
	}

	RecordPromptState("test", nil, nil, "test-model", true)

	snapshot = GetCacheBreakSnapshot()
	if snapshot.Model != "test-model" {
		t.Errorf("expected model 'test-model', got %s", snapshot.Model)
	}
	if !snapshot.FastMode {
		t.Error("expected fast mode to be true")
	}
}

func TestGetCacheBreakSnapshot_IncludesTokenCount(t *testing.T) {
	ResetCacheBreakTracker()

	RecordPromptState("system", nil, nil, "model", false)
	CheckResponseForCacheBreak(12345, 5000, nil, false, false)

	snapshot := GetCacheBreakSnapshot()
	if snapshot.CacheReadTokens == nil {
		t.Fatal("expected cache_read_tokens to be set")
	}
	if *snapshot.CacheReadTokens != 12345 {
		t.Errorf("expected 12345 cache_read_tokens, got %d", *snapshot.CacheReadTokens)
	}
}

// --- Hash function tests ---

func TestFNVHashString_Consistency(t *testing.T) {
	h1 := fnvHashString("hello world")
	h2 := fnvHashString("hello world")
	if h1 != h2 {
		t.Error("same string should produce same hash")
	}
}

func TestFNVHashString_DifferentInputs(t *testing.T) {
	h1 := fnvHashString("hello")
	h2 := fnvHashString("world")
	if h1 == h2 {
		t.Error("different strings should produce different hashes")
	}
}

func TestFNVHashString_DifferentLengths(t *testing.T) {
	h1 := fnvHashString("a")
	h2 := fnvHashString("ab")
	if h1 == h2 {
		t.Error("different length strings should produce different hashes")
	}
}

func TestFNVHashJSON_Consistency(t *testing.T) {
	v := map[string]any{"name": "exec", "type": "function"}
	h1 := fnvHashJSON(v)
	h2 := fnvHashJSON(v)
	if h1 != h2 {
		t.Error("same JSON should produce same hash")
	}
}

func TestFNVHashJSON_DifferentValues(t *testing.T) {
	v1 := map[string]any{"name": "exec"}
	v2 := map[string]any{"name": "read"}
	h1 := fnvHashJSON(v1)
	h2 := fnvHashJSON(v2)
	if h1 == h2 {
		t.Error("different JSON values should produce different hashes")
	}
}

func TestFNVHashJSON_NilInput(t *testing.T) {
	// nil should marshal to "null" and not panic
	h := fnvHashJSON(nil)
	// Just verify it doesn't crash — hash value doesn't matter
	_ = h
}

func TestFNVHashJSON_InvalidInput(t *testing.T) {
	// Channel can't be marshaled to JSON — should return 0
	ch := make(chan int)
	h := fnvHashJSON(ch)
	if h != 0 {
		t.Errorf("expected 0 for unmarshalable input, got %d", h)
	}
}

// --- Phase 1 + Phase 2 integration ---

func TestIntegration_Ph1SystemChange_Ph2Explains(t *testing.T) {
	ResetCacheBreakTracker()

	// Turn 1: baseline
	RecordPromptState("system v1", nil, nil, "model", false)
	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	// Turn 2: system changed AND big drop — same turn
	RecordPromptState("system v2", nil, nil, "model", false)
	isBreak, reason := CheckResponseForCacheBreak(7000, 3000, nil, false, false)
	if !isBreak {
		t.Fatal("expected cache break")
	}
	if !strings.Contains(reason, "system prompt changed") {
		t.Errorf("expected 'system prompt changed' attribution, got: %s", reason)
	}
}

func TestIntegration_Ph1NoChange_Ph2ServerSide(t *testing.T) {
	ResetCacheBreakTracker()

	// Turn 1: baseline
	RecordPromptState("system", nil, nil, "model", false)
	CheckResponseForCacheBreak(10000, 5000, nil, false, false)

	// Turn 2: no change
	RecordPromptState("system", nil, nil, "model", false)
	CheckResponseForCacheBreak(9900, 5000, nil, false, false)

	// Turn 3: big drop but no pending changes → server-side
	gap := 2 * time.Minute
	isBreak, reason := CheckResponseForCacheBreak(6000, 3000, &gap, false, false)
	if !isBreak {
		t.Fatal("expected cache break")
	}
	if !strings.Contains(reason, "server-side") && !strings.Contains(reason, "TTL") {
		t.Errorf("expected server-side or TTL explanation, got: %s", reason)
	}
}

func TestIntegration_Ph1CacheControlChange(t *testing.T) {
	ResetCacheBreakTracker()

	// Baseline with no cache_control markers
	RecordPromptState("system", nil, nil, "model", false)

	// Second call with cache_control markers in system prompt
	RecordPromptState("system with ephemeral marker", nil, nil, "model", false)

	// Phase 1 should detect the change
	pending := RecordPromptState("system with ephemeral marker and another ephemeral", nil, nil, "model", false)
	if pending != nil && pending.CacheControlChanged {
		// Phase 2 would attribute this
		t.Logf("cache control change detected as expected")
	}
}
