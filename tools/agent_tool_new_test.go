package tools

import (
	"strings"
	"testing"
)

// ─── AgentHandleStore ──────────────────────────────────────────────────────

func TestNewAgentHandleStore(t *testing.T) {
	s := NewAgentHandleStore()
	if s == nil {
		t.Fatal("NewAgentHandleStore returned nil")
	}
	if s.agents == nil {
		t.Error("agents map should be initialized")
	}
}

func TestAgentHandleStoreRegisterAndLookup(t *testing.T) {
	s := NewAgentHandleStore()
	done := make(chan struct{})
	s.Register("worker1", &AgentHandle{
		Name:   "worker1",
		TaskID: "agent-abc123",
		Status: "running",
		Done:   done,
	})

	h, ok := s.Lookup("worker1")
	if !ok {
		t.Fatal("expected to find worker1")
	}
	if h.TaskID != "agent-abc123" {
		t.Errorf("expected TaskID 'agent-abc123', got %s", h.TaskID)
	}
	if h.Status != "running" {
		t.Errorf("expected status 'running', got %s", h.Status)
	}
}

func TestAgentHandleStoreLookupNotFound(t *testing.T) {
	s := NewAgentHandleStore()
	_, ok := s.Lookup("nonexistent")
	if ok {
		t.Error("should not find nonexistent agent")
	}
}

func TestAgentHandleStoreList(t *testing.T) {
	s := NewAgentHandleStore()
	s.Register("a", &AgentHandle{Name: "a", TaskID: "1", Done: make(chan struct{})})
	s.Register("b", &AgentHandle{Name: "b", TaskID: "2", Done: make(chan struct{})})

	list := s.List()
	if len(list) != 2 {
		t.Errorf("expected 2 handles, got %d", len(list))
	}
}

func TestAgentHandleStoreComplete(t *testing.T) {
	s := NewAgentHandleStore()
	done := make(chan struct{})
	s.Register("worker", &AgentHandle{
		Name:   "worker",
		TaskID: "agent-1",
		Status: "running",
		Done:   done,
	})

	s.Complete("worker", "all done")

	h, ok := s.Lookup("worker")
	if !ok {
		t.Fatal("expected to find worker after Complete")
	}
	if h.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", h.Status)
	}
	if h.Result != "all done" {
		t.Errorf("expected result 'all done', got %s", h.Result)
	}
	// Done channel should be closed
	select {
	case <-done:
		// OK
	default:
		t.Error("Done channel should be closed after Complete")
	}
}

func TestAgentHandleStoreFail(t *testing.T) {
	s := NewAgentHandleStore()
	done := make(chan struct{})
	s.Register("worker", &AgentHandle{
		Name:   "worker",
		TaskID: "agent-1",
		Status: "running",
		Done:   done,
	})

	s.Fail("worker", "something went wrong")

	h, ok := s.Lookup("worker")
	if !ok {
		t.Fatal("expected to find worker after Fail")
	}
	if h.Status != "failed" {
		t.Errorf("expected status 'failed', got %s", h.Status)
	}
	if h.Result != "something went wrong" {
		t.Errorf("expected result 'something went wrong', got %s", h.Result)
	}
	select {
	case <-done:
		// OK
	default:
		t.Error("Done channel should be closed after Fail")
	}
}

func TestAgentHandleStoreCompleteNonexistent(t *testing.T) {
	s := NewAgentHandleStore()
	// Should not panic
	s.Complete("ghost", "result")
}

func TestAgentHandleStoreCount(t *testing.T) {
	s := NewAgentHandleStore()
	if s.Count() != 0 {
		t.Errorf("expected 0, got %d", s.Count())
	}
	s.Register("a", &AgentHandle{Name: "a", Done: make(chan struct{})})
	s.Register("b", &AgentHandle{Name: "b", Done: make(chan struct{})})
	if s.Count() != 2 {
		t.Errorf("expected 2, got %d", s.Count())
	}
}

// ─── Handoff Classifier ────────────────────────────────────────────────────

func TestClassifyHandoffSafe(t *testing.T) {
	result := ClassifyHandoff("Hello, the task is complete.")
	if !result.Safe {
		t.Error("expected safe for clean output")
	}
	if result.Reason != "" {
		t.Errorf("expected no reason, got %q", result.Reason)
	}
}

func TestClassifyHandoffAnthropicKey(t *testing.T) {
	result := ClassifyHandoff("Here is the key: sk-ant-api03-abcdef123456")
	if result.Safe {
		t.Error("expected NOT safe for Anthropic API key")
	}
	if result.Filtered == "" {
		t.Error("expected filtered output")
	}
}

func TestClassifyHandoffAWSKey(t *testing.T) {
	result := ClassifyHandoff("AWS key: AKIAIOSFODNN7EXAMPLE")
	if result.Safe {
		t.Error("expected NOT safe for AWS key")
	}
}

func TestClassifyHandoffGitHubToken(t *testing.T) {
	result := ClassifyHandoff("Token: ghp_1234567890abcdef")
	if result.Safe {
		t.Error("expected NOT safe for GitHub token")
	}
}

func TestClassifyHandoffPrivateKey(t *testing.T) {
	result := ClassifyHandoff("-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkq")
	if result.Safe {
		t.Error("expected NOT safe for private key")
	}
}

func TestClassifyHandoffLongOutput(t *testing.T) {
	longOutput := strings.Repeat("x", 50001)
	result := ClassifyHandoff(longOutput)
	if !result.Safe {
		t.Error("long output should still be safe (just with a reason)")
	}
	if result.Reason == "" {
		t.Error("expected a reason for very long output")
	}
}

func TestClassifyHandoffNormalLength(t *testing.T) {
	output := strings.Repeat("x", 1000)
	result := ClassifyHandoff(output)
	if !result.Safe {
		t.Error("normal output should be safe")
	}
}

func TestSanitizeHandoffOutputSafe(t *testing.T) {
	output, safe := SanitizeHandoffOutput("Clean output")
	if !safe {
		t.Error("expected safe")
	}
	if output != "Clean output" {
		t.Errorf("expected 'Clean output', got %q", output)
	}
}

func TestSanitizeHandoffOutputUnsafe(t *testing.T) {
	output, safe := SanitizeHandoffOutput("sk-ant-api03-secret")
	if safe {
		t.Error("expected NOT safe")
	}
	if output == "sk-ant-api03-secret" {
		t.Error("output should be filtered, not the raw secret")
	}
}

// ─── isValidAgentName ──────────────────────────────────────────────────────

func TestIsValidAgentName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"worker1", true},
		{"my-agent", true},
		{"agent_01", true},
		{"", false},
		{"a-very-long-name-that-exceeds-32-characters", false},
		{"bad name", false},
		{"bad!name", false},
		{"123start", true},
		{"A", true},
	}

	for _, tt := range tests {
		if got := isValidAgentName(tt.name); got != tt.want {
			t.Errorf("isValidAgentName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// ─── AgentExecutionMode ───────────────────────────────────────────────────

func TestAgentExecutionModeConstants(t *testing.T) {
	if AgentModeSync != "sync" {
		t.Errorf("expected 'sync', got %q", AgentModeSync)
	}
	if AgentModeAsync != "async" {
		t.Errorf("expected 'async', got %q", AgentModeAsync)
	}
}

// ─── AgentTool with mode ──────────────────────────────────────────────────

func TestAgentToolExecuteSyncMode(t *testing.T) {
	var gotBackground bool
	tool := &AgentTool{
		SpawnFunc: func(desc, prompt, subagent, model string, bg bool, allowed, disallowed []string, inherit bool, maxTurns int, parentMsg []map[string]any) (string, string, string, string, int, int64) {
			gotBackground = bg
			return "agent-sync-1", "sync result here", "", "", 2, 500
		},
		SpawnSyncFunc: func(desc, prompt, subagent, model string, bg bool, allowed, disallowed []string, inherit bool, maxTurns int, parentMsg []map[string]any) (string, string, string, string, int, int64) {
			gotBackground = bg
			return "agent-sync-1", "sync result here", "", "", 2, 500
		},
	}

	result := tool.Execute(map[string]any{
		"prompt":      "do the thing",
		"description": "test sync",
		"mode":        "sync",
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if gotBackground {
		t.Error("sync mode should pass runInBackground=false")
	}
	if !strings.Contains(result.Output, "sync result here") {
		t.Errorf("result should contain the sync result text, got: %s", result.Output)
	}
}

func TestAgentToolExecuteAsyncMode(t *testing.T) {
	var gotBackground bool
	tool := &AgentTool{
		SpawnFunc: func(desc, prompt, subagent, model string, bg bool, allowed, disallowed []string, inherit bool, maxTurns int, parentMsg []map[string]any) (string, string, string, string, int, int64) {
			gotBackground = bg
			return "agent-async-1", "", "", "output.txt", 0, 0
		},
	}

	result := tool.Execute(map[string]any{
		"prompt":      "do the thing",
		"description": "test async",
		"mode":        "async",
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !gotBackground {
		t.Error("async mode should pass runInBackground=true")
	}
	if !strings.Contains(result.Output, "agent-async-1") {
		t.Error("result should contain agent ID")
	}
	if !strings.Contains(result.Output, "async_launched") {
		t.Error("result should indicate async launch")
	}
}

func TestAgentToolExecuteDefaultAsyncMode(t *testing.T) {
	var gotBackground bool
	tool := &AgentTool{
		SpawnFunc: func(desc, prompt, subagent, model string, bg bool, allowed, disallowed []string, inherit bool, maxTurns int, parentMsg []map[string]any) (string, string, string, string, int, int64) {
			gotBackground = bg
			return "agent-1", "", "", "", 0, 0
		},
	}

	tool.Execute(map[string]any{
		"prompt":      "test",
		"description": "test",
		// no mode specified — should default to async
	})

	if !gotBackground {
		t.Error("default mode should be async (runInBackground=true)")
	}
}

func TestAgentToolExecuteWithNamedAgent(t *testing.T) {
	store := NewAgentHandleStore()
	tool := &AgentTool{
		SpawnFunc: func(desc, prompt, subagent, model string, bg bool, allowed, disallowed []string, inherit bool, maxTurns int, parentMsg []map[string]any) (string, string, string, string, int, int64) {
			return "agent-named-1", "", "", "out.txt", 0, 0
		},
		HandleStore: store,
	}

	result := tool.Execute(map[string]any{
		"prompt":      "test named agent",
		"description": "named test",
		"mode":        "async",
		"name":        "researcher",
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if store.Count() != 1 {
		t.Errorf("expected 1 registered agent, got %d", store.Count())
	}
	h, ok := store.Lookup("researcher")
	if !ok {
		t.Fatal("expected to find 'researcher' in handle store")
	}
	if h.TaskID != "agent-named-1" {
		t.Errorf("expected TaskID 'agent-named-1', got %s", h.TaskID)
	}
}

func TestAgentToolExecuteInvalidName(t *testing.T) {
	store := NewAgentHandleStore()
	tool := &AgentTool{
		SpawnFunc: func(desc, prompt, subagent, model string, bg bool, allowed, disallowed []string, inherit bool, maxTurns int, parentMsg []map[string]any) (string, string, string, string, int, int64) {
			return "agent-1", "", "", "", 0, 0
		},
		HandleStore: store,
	}

	result := tool.Execute(map[string]any{
		"prompt":      "test",
		"description": "test",
		"mode":        "async",
		"name":        "bad name!",
	})

	if !result.IsError {
		t.Error("expected error for invalid agent name")
	}
}

func TestAgentToolExecuteSyncWithHandoffFilter(t *testing.T) {
	secretOutput := "here is a key: sk-ant-api03-secret"
	spawner := func(desc, prompt, subagent, model string, bg bool, allowed, disallowed []string, inherit bool, maxTurns int, parentMsg []map[string]any) (string, string, string, string, int, int64) {
		return "agent-1", secretOutput, "", "", 1, 100
	}
	tool := &AgentTool{
		SpawnFunc:     spawner,
		SpawnSyncFunc: spawner,
	}

	result := tool.Execute(map[string]any{
		"prompt":      "get key",
		"description": "secret test",
		"mode":        "sync",
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if strings.Contains(result.Output, "sk-ant-api03-secret") {
		t.Error("sync result should be filtered by handoff classifier, secret should not appear")
	}
	if !strings.Contains(result.Output, "filtered") {
		t.Error("result should indicate it was filtered")
	}
}

func TestAgentToolSchemaHasModeField(t *testing.T) {
	tool := &AgentTool{}
	schema := tool.InputSchema()
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["mode"]; !ok {
		t.Error("schema should have 'mode' property")
	}
}

func TestAgentToolSchemaHasNameField(t *testing.T) {
	tool := &AgentTool{}
	schema := tool.InputSchema()
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["name"]; !ok {
		t.Error("schema should have 'name' property")
	}
}

func TestAgentToolSchemaHasWorktreeField(t *testing.T) {
	tool := &AgentTool{}
	schema := tool.InputSchema()
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["worktree"]; !ok {
		t.Error("schema should have 'worktree' property")
	}
}