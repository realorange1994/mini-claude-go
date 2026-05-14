package main

import (
	"sync"
	"testing"
	"time"

	"miniclaudecode-go/tools"
)

// mockExecTool implements tools.Tool with a customizable Execute function.
type mockExecTool struct {
	name        string
	inputSchema map[string]any
	executeFn   func(params map[string]any) tools.ToolResult
}

func (m *mockExecTool) Name() string                                   { return m.name }
func (m *mockExecTool) Description() string                            { return "mock tool for testing" }
func (m *mockExecTool) InputSchema() map[string]any                    { return m.inputSchema }
func (m *mockExecTool) CheckPermissions(params map[string]any) tools.PermissionResult {
	return tools.PermissionResult{Behavior: tools.PermissionAllow}
}
func (m *mockExecTool) Execute(params map[string]any) tools.ToolResult {
	if m.executeFn != nil {
		return m.executeFn(params)
	}
	return tools.ToolResult{Output: "ok", IsError: false}
}

// ---------------------------------------------------------------------------
// Core construction and configuration tests
// ---------------------------------------------------------------------------

func TestNewStreamingToolExecutor(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	if exec == nil {
		t.Fatal("expected non-nil executor")
	}
	if exec.semaphore == nil {
		t.Error("expected semaphore to be initialized")
	}
	if cap(exec.semaphore) != 10 {
		t.Errorf("expected semaphore capacity 10, got %d", cap(exec.semaphore))
	}
	if exec.dispatched.Load() != 0 {
		t.Error("expected dispatched counter to be 0")
	}
	if exec.completed.Load() != 0 {
		t.Error("expected completed counter to be 0")
	}
	if exec.hasErrored.Load() {
		t.Error("expected hasErrored to be false")
	}
	if exec.siblingCtx == nil {
		t.Error("expected siblingCtx to be initialized")
	}
	if exec.siblingCancel == nil {
		t.Error("expected siblingCancel to be initialized")
	}
}

func TestSetMaxConcurrency(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	exec.SetMaxConcurrency(5)
	if cap(exec.semaphore) != 5 {
		t.Errorf("expected semaphore cap 5, got %d", cap(exec.semaphore))
	}

	exec.SetMaxConcurrency(0)
	if cap(exec.semaphore) != 1 {
		t.Errorf("expected semaphore cap 1 for zero input, got %d", cap(exec.semaphore))
	}

	exec.SetMaxConcurrency(-5)
	if cap(exec.semaphore) != 1 {
		t.Errorf("expected semaphore cap 1 for negative input, got %d", cap(exec.semaphore))
	}
}

// ---------------------------------------------------------------------------
// isConcurrencySafe tests
// ---------------------------------------------------------------------------

func TestIsConcurrencySafe(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	safeTools := []struct {
		name string
		args string // JSON arguments string
	}{
		{"read_file", `{"file_path":"test.txt"}`},
		{"glob", `{"pattern":"*.go"}`},
		{"grep", `{"pattern":"test"}`},
		{"web_search", `{"query":"test"}`},
		{"web_fetch", `{"url":"https://example.com"}`},
		{"read_skill", `{"name":"test"}`},
		{"tool_search", `{"query":"test"}`},
		{"agent_list", ""},
		{"agent_get", `{"name":"test"}`},
		// exec with read-only command
		{"exec", `{"command":"ls -la"}`},
		{"exec", `{"command":"cat test.txt"}`},
		{"exec", `{"command":"grep test file"}`},
	}
	for _, tool := range safeTools {
		if !exec.isConcurrencySafe(tool.name, tool.args) {
			t.Errorf("expected %s to be concurrency-safe", tool.name)
		}
	}

	unsafeTools := []struct {
		name string
		args string
	}{
		{"write_file", `{"file_path":"test.txt","content":"hello"}`},
		{"edit_file", `{"file_path":"test.txt"}`},
		{"multi_edit", `{"file_path":"test.txt"}`},
		{"file_write", `{"file_path":"test.txt","content":"hello"}`},
		{"file_edit", `{"file_path":"test.txt"}`},
		{"bash", `{"command":"echo hello"}`},
		{"notebook_edit", `{"notebook_path":"test.ipynb"}`},
		// exec with non-read-only commands
		{"exec", `{"command":"rm -rf test"}`},
		{"exec", `{"command":"go build"}`},
	}
	for _, tool := range unsafeTools {
		if exec.isConcurrencySafe(tool.name, tool.args) {
			t.Errorf("expected %s to NOT be concurrency-safe", tool.name)
		}
	}
}

// ---------------------------------------------------------------------------
// canExecuteTool tests — matching upstream's concurrency gating
// ---------------------------------------------------------------------------

func TestCanExecuteToolNoToolsExecuting(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)
	// No tools executing -> any tool can start
	exec.mu.Lock()
	if !exec.canExecuteToolLocked(true) {
		t.Error("safe tool should execute when nothing is running")
	}
	if !exec.canExecuteToolLocked(false) {
		t.Error("unsafe tool should execute when nothing is running")
	}
	exec.mu.Unlock()
}

func TestCanExecuteToolSafeToolWithSafeExecuting(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	// Simulate a safe tool executing
	exec.mu.Lock()
	exec.tools = append(exec.tools, &TrackedTool{
		status:            toolExecuting,
		isConcurrencySafe: true,
	})

	// Another safe tool should be allowed
	if !exec.canExecuteToolLocked(true) {
		t.Error("safe tool should execute alongside other safe tools")
	}
	// Unsafe tool should NOT be allowed
	if exec.canExecuteToolLocked(false) {
		t.Error("unsafe tool should NOT execute while safe tools are running")
	}
	exec.mu.Unlock()
}

func TestCanExecuteToolSafeToolWithUnsafeExecuting(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	// Simulate an unsafe tool executing
	exec.mu.Lock()
	exec.tools = append(exec.tools, &TrackedTool{
		status:            toolExecuting,
		isConcurrencySafe: false,
	})

	// Safe tool should NOT be allowed (upstream: all executing must be safe)
	if exec.canExecuteToolLocked(true) {
		t.Error("safe tool should NOT execute while unsafe tool is running")
	}
	// Another unsafe tool should NOT be allowed
	if exec.canExecuteToolLocked(false) {
		t.Error("unsafe tool should NOT execute while another unsafe tool is running")
	}
	exec.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Close and discard tests
// ---------------------------------------------------------------------------

func TestClose(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	if exec.discarded {
		t.Error("expected discarded to be false initially")
	}

	exec.Close()
	if !exec.discarded {
		t.Error("expected discarded to be true after Close")
	}
	// siblingCtx should be cancelled
	if exec.siblingCtx.Err() == nil {
		t.Error("expected siblingCtx to be cancelled after Close")
	}
}

// ---------------------------------------------------------------------------
// Dispatch tests
// ---------------------------------------------------------------------------

func TestDispatchUnknownTool(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "toolu_unknown", Name: "nonexistent_tool", Arguments: `{"key": "value"}`},
	}

	exec.dispatch(0, &toolCalls)

	results := exec.Wait(1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].isError {
		t.Error("expected error result for unknown tool")
	}
	if results[0].toolName != "nonexistent_tool" {
		t.Errorf("expected toolName 'nonexistent_tool', got %s", results[0].toolName)
	}
	if results[0].index != 0 {
		t.Errorf("expected index 0, got %d", results[0].index)
	}
}

func TestDispatchKnownSafeTool(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &mockExecTool{
		name: "read_file",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "file contents", IsError: false}
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "toolu_read", Name: "read_file", Arguments: `{"path": "/tmp/test.txt"}`},
	}

	exec.dispatch(0, &toolCalls)

	results := exec.Wait(1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].isError {
		t.Errorf("expected no error, got: %s", results[0].output)
	}
	if results[0].output != "file contents" {
		t.Errorf("expected output 'file contents', got %s", results[0].output)
	}
	if results[0].toolUseID != "toolu_read" {
		t.Errorf("expected toolUseID 'toolu_read', got %s", results[0].toolUseID)
	}
}

func TestDispatchKnownUnsafeTool(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &mockExecTool{
		name: "write_file",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "written", IsError: false}
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "toolu_write", Name: "write_file", Arguments: `{"path": "/tmp/test.txt", "content": "hello"}`},
	}

	exec.dispatch(0, &toolCalls)

	results := exec.Wait(1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].isError {
		t.Errorf("expected no error, got: %s", results[0].output)
	}
	if results[0].output != "written" {
		t.Errorf("expected output 'written', got %s", results[0].output)
	}
}

func TestDispatchWhenStopped(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &mockExecTool{
		name: "test_tool",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "ok", IsError: false}
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)
	exec.Close() // stop before dispatch

	toolCalls := []ToolCallInfo{
		{ID: "toolu_1", Name: "test_tool", Arguments: `{}`},
	}

	exec.dispatch(0, &toolCalls)

	time.Sleep(100 * time.Millisecond)
	if exec.dispatched.Load() != 0 {
		t.Error("dispatch should be skipped when stopped")
	}
}

// ---------------------------------------------------------------------------
// Execute tests — JSON handling, panic recovery, etc.
// ---------------------------------------------------------------------------

func TestExecuteMalformedJSON(t *testing.T) {
	reg := tools.NewRegistry()
	callCount := 0
	tool := &mockExecTool{
		name: "test_tool",
		executeFn: func(params map[string]any) tools.ToolResult {
			callCount++
			return tools.ToolResult{Output: "called", IsError: false}
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "toolu_bad", Name: "test_tool", Arguments: `{not valid json`},
	}

	exec.dispatch(0, &toolCalls)

	results := exec.Wait(1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].isError {
		t.Error("expected error for malformed JSON")
	}
	if results[0].output == "" {
		t.Error("expected non-empty error message for malformed JSON")
	}
	if callCount != 0 {
		t.Errorf("tool should NOT have been called with malformed JSON, got %d calls", callCount)
	}
}

func TestExecuteEmptyArguments(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &mockExecTool{
		name: "test_tool",
		executeFn: func(params map[string]any) tools.ToolResult {
			if len(params) != 0 {
				return tools.ToolResult{Output: "unexpected params", IsError: true}
			}
			return tools.ToolResult{Output: "ok", IsError: false}
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "toolu_empty", Name: "test_tool", Arguments: ""},
	}

	exec.dispatch(0, &toolCalls)

	results := exec.Wait(1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].isError {
		t.Errorf("expected no error for empty args, got: %s", results[0].output)
	}
}

func TestExecuteEmptyJSONObject(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &mockExecTool{
		name: "test_tool",
		executeFn: func(params map[string]any) tools.ToolResult {
			if len(params) != 0 {
				return tools.ToolResult{Output: "unexpected params", IsError: true}
			}
			return tools.ToolResult{Output: "ok", IsError: false}
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "toolu_empty_obj", Name: "test_tool", Arguments: `{}`},
	}

	exec.dispatch(0, &toolCalls)

	results := exec.Wait(1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].isError {
		t.Errorf("expected no error for empty JSON object, got: %s", results[0].output)
	}
}

func TestExecutePanicRecovery(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &mockExecTool{
		name: "panic_tool",
		executeFn: func(params map[string]any) tools.ToolResult {
			panic("intentional panic for testing")
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "toolu_panic", Name: "panic_tool", Arguments: `{}`},
	}

	exec.dispatch(0, &toolCalls)

	results := exec.Wait(1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].isError {
		t.Error("expected error result after panic")
	}
	if results[0].output == "" {
		t.Error("expected non-empty error message after panic")
	}
}

func TestExecuteEmptyToolUseIDGuard(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &mockExecTool{
		name: "test_tool",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "ok", IsError: false}
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "", Name: "test_tool", Arguments: `{}`},
	}

	exec.dispatch(0, &toolCalls)

	results := exec.Wait(1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].toolUseID != "synthetic_tool_use_id" {
		t.Errorf("expected synthetic ID, got %s", results[0].toolUseID)
	}
}

// ---------------------------------------------------------------------------
// CRITICAL: Bash-only error cancellation tests
// Matching upstream: only Bash errors cancel sibling tools
// ---------------------------------------------------------------------------

func TestBashErrorCancelsSiblings(t *testing.T) {
	reg := tools.NewRegistry()

	// Register exec (Bash) tool that errors
	execTool := &mockExecTool{
		name: "exec",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "command failed", IsError: true}
		},
	}
	reg.Register(execTool)

	// Register a safe tool that succeeds
	readTool := &mockExecTool{
		name: "read_file",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "file contents", IsError: false}
		},
	}
	reg.Register(readTool)

	executor := NewStreamingToolExecutor(reg, nil, nil)

	// Dispatch exec first (it's unsafe, so it blocks)
	toolCalls := []ToolCallInfo{
		{ID: "toolu_exec", Name: "exec", Arguments: `{"command": "mkdir /bad"}`},
		{ID: "toolu_read", Name: "read_file", Arguments: `{"path": "/tmp/test.txt"}`},
	}

	executor.dispatch(0, &toolCalls)

	// After exec errors, sibling should be cancelled
	if !executor.hasErrored.Load() {
		t.Error("expected hasErrored to be true after Bash error")
	}

	// siblingCtx should be cancelled
	if executor.siblingCtx.Err() == nil {
		t.Error("expected siblingCtx to be cancelled after Bash error")
	}
}

func TestNonBashErrorDoesNotCancelSiblings(t *testing.T) {
	reg := tools.NewRegistry()

	// Register a safe tool that errors (NOT Bash)
	errorTool := &mockExecTool{
		name: "read_file",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "file not found", IsError: true}
		},
	}
	reg.Register(errorTool)

	// Register another safe tool
	globTool := &mockExecTool{
		name: "glob",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "matches", IsError: false}
		},
	}
	reg.Register(globTool)

	executor := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "toolu_read", Name: "read_file", Arguments: `{"path": "/nonexistent"}`},
		{ID: "toolu_glob", Name: "glob", Arguments: `{"pattern": "*.txt"}`},
	}

	// Dispatch read_file (safe tool) first
	executor.dispatch(0, &toolCalls)
	// Dispatch glob (safe tool) second
	executor.dispatch(1, &toolCalls)

	results := executor.Wait(2)

	// Non-Bash error should NOT set hasErrored
	if executor.hasErrored.Load() {
		t.Error("non-Bash error should NOT set hasErrored")
	}

	// siblingCtx should NOT be cancelled
	if executor.siblingCtx.Err() != nil {
		t.Error("non-Bash error should NOT cancel siblingCtx")
	}

	// Both tools should have results (error for read, success for glob)
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
}

func TestWriteErrorDoesNotCancelSiblings(t *testing.T) {
	reg := tools.NewRegistry()

	// Register write_file tool that errors
	writeTool := &mockExecTool{
		name: "write_file",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "permission denied", IsError: true}
		},
	}
	reg.Register(writeTool)

	executor := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "toolu_write", Name: "write_file", Arguments: `{"path": "/root/test.txt"}`},
	}

	executor.dispatch(0, &toolCalls)

	// write_file error should NOT set hasErrored (only Bash/exec does)
	if executor.hasErrored.Load() {
		t.Error("write_file error should NOT set hasErrored")
	}

	// siblingCtx should NOT be cancelled
	if executor.siblingCtx.Err() != nil {
		t.Error("write_file error should NOT cancel siblingCtx")
	}
}

// ---------------------------------------------------------------------------
// Synthetic error message tests
// ---------------------------------------------------------------------------

func TestGetToolDescription(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	tests := []struct {
		name     string
		tc       ToolCallInfo
		expected string
	}{
		{
			name:     "bash with command",
			tc:       ToolCallInfo{Name: "exec", Arguments: `{"command": "mkdir -p /tmp/test"}`},
			expected: "exec(mkdir -p /tmp/test)",
		},
		{
			name:     "bash with long command",
			tc:       ToolCallInfo{Name: "exec", Arguments: `{"command": "this is a very long command that exceeds forty characters in length"}`},
			expected: "exec(this is a very long command that exceeds\u2026)",
		},
		{
			name:     "read with file_path",
			tc:       ToolCallInfo{Name: "read_file", Arguments: `{"file_path": "/tmp/test.txt"}`},
			expected: "read_file(/tmp/test.txt)",
		},
		{
			name:     "grep with pattern",
			tc:       ToolCallInfo{Name: "grep", Arguments: `{"pattern": "TODO"}`},
			expected: "grep(TODO)",
		},
		{
			name:     "tool with no recognizable field",
			tc:       ToolCallInfo{Name: "unknown_tool", Arguments: `{}`},
			expected: "unknown_tool",
		},
		{
			name:     "tool with empty arguments",
			tc:       ToolCallInfo{Name: "some_tool", Arguments: ""},
			expected: "some_tool",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exec.getToolDescription(tt.tc)
			if got != tt.expected {
				t.Errorf("getToolDescription() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestCreateSyntheticErrorMessage(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	tc := ToolCallInfo{Name: "exec", Arguments: `{"command": "rm -rf /"}`}
	msg := exec.createSyntheticErrorMessage(tc)
	if msg == "" {
		t.Error("expected non-empty synthetic error message")
	}
	// Should contain the tool description
	if !containsSubstring(msg, "exec") {
		t.Errorf("synthetic error message should contain tool name, got: %s", msg)
	}
	if !containsSubstring(msg, "Cancelled") {
		t.Errorf("synthetic error message should contain 'Cancelled', got: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// Start + Wait tests (streaming mode)
// ---------------------------------------------------------------------------

func TestStartAndStop(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &mockExecTool{
		name: "read_file", // concurrency-safe tool name
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "ok", IsError: false}
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	doneCh := make(chan int, 3)
	toolCalls := []ToolCallInfo{
		{ID: "toolu_1", Name: "read_file", Arguments: `{}`},
		{ID: "toolu_2", Name: "read_file", Arguments: `{}`},
		{ID: "toolu_3", Name: "read_file", Arguments: `{}`},
	}

	exec.Start(doneCh, &toolCalls)

	doneCh <- 0
	doneCh <- 1
	doneCh <- 2
	close(doneCh)

	results := exec.Wait(3)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Results should be sorted by index
	for i, r := range results {
		if r.index != i {
			t.Errorf("expected result index %d, got %d", i, r.index)
		}
	}
}

func TestStartIgnoresOutOfBounds(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	doneCh := make(chan int, 1)
	toolCalls := []ToolCallInfo{
		{ID: "toolu_1", Name: "test_tool", Arguments: `{}`},
	}

	exec.Start(doneCh, &toolCalls)

	doneCh <- 99
	close(doneCh)

	time.Sleep(100 * time.Millisecond)
	if exec.dispatched.Load() != 0 {
		t.Error("out of bounds index should not trigger dispatch")
	}
}

func TestStartNegativeIndex(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	doneCh := make(chan int, 1)
	toolCalls := []ToolCallInfo{}

	exec.Start(doneCh, &toolCalls)

	doneCh <- -1
	close(doneCh)

	time.Sleep(100 * time.Millisecond)
	if exec.dispatched.Load() != 0 {
		t.Error("negative index should not trigger dispatch")
	}
}

func TestCloseStopsStart(t *testing.T) {
	reg := tools.NewRegistry()
	exec := NewStreamingToolExecutor(reg, nil, nil)

	doneCh := make(chan int, 1)
	toolCalls := []ToolCallInfo{}

	exec.Start(doneCh, &toolCalls)
	exec.Close() // Close immediately

	doneCh <- 0
	close(doneCh)

	time.Sleep(100 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// Result ordering tests
// ---------------------------------------------------------------------------

func TestWaitReturnsOrderedResults(t *testing.T) {
	reg := tools.NewRegistry()

	slowTool := &mockExecTool{
		name: "slow_tool",
		executeFn: func(params map[string]any) tools.ToolResult {
			time.Sleep(200 * time.Millisecond)
			return tools.ToolResult{Output: "slow", IsError: false}
		},
	}
	fastTool := &mockExecTool{
		name: "fast_tool",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "fast", IsError: false}
		},
	}
	reg.Register(slowTool)
	reg.Register(fastTool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	// Execute fast first (index 1), then slow (index 0)
	// Fast should complete first, but results should be ordered
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// Use direct execute for ordering test
		tc := ToolCallInfo{ID: "toolu_fast", Name: "fast_tool", Arguments: `{}`}
		tracked := &TrackedTool{tc: tc, tool: fastTool, status: toolExecuting, isConcurrencySafe: true, index: 1}
		exec.execute(1, tc, fastTool, tracked)
	}()
	go func() {
		defer wg.Done()
		tc := ToolCallInfo{ID: "toolu_slow", Name: "slow_tool", Arguments: `{}`}
		tracked := &TrackedTool{tc: tc, tool: slowTool, status: toolExecuting, isConcurrencySafe: true, index: 0}
		exec.execute(0, tc, slowTool, tracked)
	}()
	wg.Wait()

	results := exec.Wait(2)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Results should be ordered by index
	if results[0].index != 0 || results[1].index != 1 {
		t.Errorf("expected ordered results by index, got indices %d, %d", results[0].index, results[1].index)
	}
}

func TestToolResultPreservesIndexOnConcurrentExecution(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &mockExecTool{
		name: "test_tool",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "ok", IsError: false}
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	// Execute 5 tools with indices in random order
	indices := []int{3, 0, 4, 1, 2}
	var wg sync.WaitGroup
	for _, idx := range indices {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tc := ToolCallInfo{ID: "toolu_test", Name: "test_tool", Arguments: `{}`}
			tracked := &TrackedTool{tc: tc, tool: tool, status: toolExecuting, isConcurrencySafe: true, index: i}
			exec.execute(i, tc, tool, tracked)
		}(idx)
	}
	wg.Wait()

	results := exec.Wait(5)
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	// Verify results are sorted by index
	for i, r := range results {
		if r.index != i {
			t.Errorf("result[%d].index = %d, expected %d", i, r.index, i)
		}
	}
}

// ---------------------------------------------------------------------------
// JSON round-trip tests
// ---------------------------------------------------------------------------

func TestJSONRoundTripToolArguments(t *testing.T) {
	testCases := []struct {
		name      string
		arguments string
		expectErr bool
	}{
		{
			name:      "simple object",
			arguments: `{"path": "/tmp/test.txt", "limit": 100}`,
			expectErr: false,
		},
		{
			name:      "nested arrays",
			arguments: `{"files": ["/a.txt", "/b.txt"], "options": {"recursive": true}}`,
			expectErr: false,
		},
		{
			name:      "unicode content",
			arguments: `{"content": "你好世界\nこんにちは"}`,
			expectErr: false,
		},
		{
			name:      "null values",
			arguments: `{"path": null, "content": "test"}`,
			expectErr: false,
		},
		{
			name:      "truncated JSON",
			arguments: `{"path": "/tmp/test`,
			expectErr: true,
		},
		{
			name:      "bare string",
			arguments: `"just a string"`,
			expectErr: true,
		},
		{
			name:      "array instead of object",
			arguments: `[1, 2, 3]`,
			expectErr: true,
		},
	}

	reg := tools.NewRegistry()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &mockExecTool{
				name: "test_tool",
				executeFn: func(params map[string]any) tools.ToolResult {
					return tools.ToolResult{Output: "ok", IsError: false}
				},
			}
			reg.Register(tool)

			exec := NewStreamingToolExecutor(reg, nil, nil)

			toolCalls := []ToolCallInfo{
				{ID: "toolu_test", Name: "test_tool", Arguments: tc.arguments},
			}

			exec.dispatch(0, &toolCalls)

			results := exec.Wait(1)

			if tc.expectErr {
				if len(results) == 0 {
					t.Fatal("expected error result")
				}
				if !results[0].isError {
					t.Errorf("expected error for malformed JSON, got: %s", results[0].output)
				}
			} else {
				if len(results) == 0 {
					t.Fatal("expected result")
				}
				if results[0].isError {
					t.Errorf("unexpected error: %s", results[0].output)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Struct field tests
// ---------------------------------------------------------------------------

func TestToolExecResultStructFields(t *testing.T) {
	r := toolExecResult{
		index:     5,
		toolName:  "test_tool",
		toolUseID: "toolu_123",
		output:    "test output",
		isError:   true,
		duration:  100 * time.Millisecond,
	}
	if r.index != 5 {
		t.Errorf("expected index 5, got %d", r.index)
	}
	if r.toolName != "test_tool" {
		t.Errorf("expected toolName 'test_tool', got %s", r.toolName)
	}
	if r.toolUseID != "toolu_123" {
		t.Errorf("expected toolUseID 'toolu_123', got %s", r.toolUseID)
	}
	if r.output != "test output" {
		t.Errorf("expected output 'test output', got %s", r.output)
	}
	if !r.isError {
		t.Error("expected isError to be true")
	}
	if r.duration != 100*time.Millisecond {
		t.Errorf("expected duration 100ms, got %v", r.duration)
	}
}

func TestTrackedToolStructFields(t *testing.T) {
	tc := ToolCallInfo{ID: "toolu_1", Name: "exec", Arguments: `{"command": "ls"}`}
	tracked := &TrackedTool{
		tc:               tc,
		status:           toolQueued,
		isConcurrencySafe: false,
		index:            0,
	}
	if tracked.tc.Name != "exec" {
		t.Errorf("expected Name 'exec', got %s", tracked.tc.Name)
	}
	if tracked.status != toolQueued {
		t.Errorf("expected status 'queued', got %s", tracked.status)
	}
	if tracked.isConcurrencySafe {
		t.Error("expected isConcurrencySafe to be false for exec")
	}
	if tracked.index != 0 {
		t.Errorf("expected index 0, got %d", tracked.index)
	}
}

// ---------------------------------------------------------------------------
// ProcessQueue ordering tests
// ---------------------------------------------------------------------------

func TestProcessQueueOrdersSafeTools(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &mockExecTool{
		name: "read_file",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "ok", IsError: false}
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	// Add multiple safe tools
	toolCalls := []ToolCallInfo{
		{ID: "toolu_1", Name: "read_file", Arguments: `{"path": "/a"}`},
		{ID: "toolu_2", Name: "read_file", Arguments: `{"path": "/b"}`},
		{ID: "toolu_3", Name: "read_file", Arguments: `{"path": "/c"}`},
	}

	exec.dispatch(0, &toolCalls)
	exec.dispatch(1, &toolCalls)
	exec.dispatch(2, &toolCalls)

	results := exec.Wait(3)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// All should succeed
	for i, r := range results {
		if r.isError {
			t.Errorf("result[%d] unexpected error: %s", i, r.output)
		}
	}
}

// ---------------------------------------------------------------------------
// Cancel remaining tools on Bash error
// ---------------------------------------------------------------------------

func TestCancelRemainingOnBashError(t *testing.T) {
	reg := tools.NewRegistry()

	execTool := &mockExecTool{
		name: "exec",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "command failed", IsError: true}
		},
	}
	readTool := &mockExecTool{
		name: "read_file",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "file contents", IsError: false}
		},
	}
	reg.Register(execTool)
	reg.Register(readTool)

	executor := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "toolu_exec", Name: "exec", Arguments: `{"command": "bad_cmd"}`},
		{ID: "toolu_read", Name: "read_file", Arguments: `{"path": "/tmp/test"}`},
	}

	// Dispatch exec first (unsafe, blocks until done)
	executor.dispatch(0, &toolCalls)

	// After exec errors, the executor should be stopped
	if !executor.hasErrored.Load() {
		t.Error("expected hasErrored after Bash error")
	}

	// Second dispatch should be skipped because executor is stopped
	executor.dispatch(1, &toolCalls)

	// Should have result for exec error, read_file dispatch was skipped
	results := executor.Wait(1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result (exec error), got %d", len(results))
	}
	if !results[0].isError {
		t.Errorf("expected exec error result, got success: %s", results[0].output)
	}

	// Verify siblingCtx is cancelled
	if executor.siblingCtx.Err() == nil {
		t.Error("expected siblingCtx to be cancelled after Bash error")
	}
}

// ---------------------------------------------------------------------------
// Wait error handling
// ---------------------------------------------------------------------------

func TestWaitErrorReturnsQuickly(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &mockExecTool{
		name: "exec",
		executeFn: func(params map[string]any) tools.ToolResult {
			return tools.ToolResult{Output: "error", IsError: true}
		},
	}
	reg.Register(tool)

	exec := NewStreamingToolExecutor(reg, nil, nil)

	toolCalls := []ToolCallInfo{
		{ID: "toolu_1", Name: "exec", Arguments: `{}`},
	}

	start := time.Now()
	exec.dispatch(0, &toolCalls)
	results := exec.Wait(1)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("Wait took too long after error: %v", elapsed)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].isError {
		t.Error("expected error result")
	}
}

