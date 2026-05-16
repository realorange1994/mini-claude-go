package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"miniclaudecode-go/tools"
)

// StreamingToolExecutor executes tool calls as they complete during streaming,
// overlapping tool execution with ongoing stream processing.
//
// Follows the upstream TypeScript claude-code design:
//   - Queue-based tool management with TrackedTool lifecycle
//   - canExecuteTool + processQueue pattern for ordered execution
//   - Only Bash tool errors cancel sibling tools (via siblingAbortCtx)
//   - Non-Bash errors are returned normally without affecting siblings
//   - Synthetic error messages for cancelled tools include description
type StreamingToolExecutor struct {
	registry   *tools.Registry
	gate       *PermissionGate
	hooks      HookConfig // shell command hooks loaded from settings
	snapshots  *SnapshotHistory // file version history for auto-snapshots

	// Tool queue
	tools     []*TrackedTool
	toolsSeq  int32 // tracks number of tools added, for Wait() counting
	processing atomic.Bool
	mu         sync.Mutex

	// State
	dispatched             atomic.Int32
	completed              atomic.Int32
	hasErrored             atomic.Bool
	erroredToolDescription string

	// Results
	results   []toolExecResult
	resultsMu sync.Mutex

	// Concurrency gating
	semaphore chan struct{}

	discarded bool

	// Sibling abort context: cancelled only when a Bash tool errors.
	// Non-Bash tool errors do NOT cancel this.
	siblingCtx    context.Context
	siblingCancel context.CancelFunc
}

type toolStatus string

const (
	toolQueued     toolStatus = "queued"
	toolExecuting  toolStatus = "executing"
	toolCompleted  toolStatus = "completed"
)

type toolExecResult struct {
	index     int
	toolName  string
	toolUseID string
	output    string
	isError   bool
	duration  time.Duration
}

// TrackedTool tracks a tool through its execution lifecycle,
// matching upstream's TrackedTool design.
type TrackedTool struct {
	tc               ToolCallInfo
	tool             tools.Tool
	status           toolStatus
	isConcurrencySafe bool
	index            int
	execDoneCh       chan struct{} // signaled when unsafe tool execution completes
	cancelled        bool           // marked when cancelled by sibling error
}

// NewStreamingToolExecutor creates a new executor.
func NewStreamingToolExecutor(registry *tools.Registry, gate *PermissionGate, hooks HookConfig, snapshots *SnapshotHistory) *StreamingToolExecutor {
	siblingCtx, siblingCancel := context.WithCancel(context.Background())
	return &StreamingToolExecutor{
		registry:      registry,
		gate:          gate,
		hooks:         hooks,
		snapshots:     snapshots,
		semaphore:     make(chan struct{}, 10), // up to 10 concurrent tools
		siblingCtx:    siblingCtx,
		siblingCancel: siblingCancel,
	}
}

// SetMaxConcurrency sets the maximum number of concurrent tool executions.
func (e *StreamingToolExecutor) SetMaxConcurrency(n int) {
	if n <= 0 {
		n = 1
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.semaphore = make(chan struct{}, n)
}

// isConcurrencySafe returns true if the tool can safely run alongside other tools.
// Matching upstream's tool.isConcurrencySafe(input) which checks isReadOnly(cmd)
// for BashTool — only read-only bash commands (ls, cat, grep, etc.) are safe.
func (e *StreamingToolExecutor) isConcurrencySafe(toolName string, arguments string) bool {
	if toolName == "exec" {
		// Parse the command from arguments and check if it's read-only
		var input map[string]any
		if arguments != "" {
			_ = json.Unmarshal([]byte(arguments), &input)
		}
		if cmd, ok := input["command"].(string); ok {
			return tools.IsReadOnlyCommand(cmd)
		}
		return false
	}
	switch toolName {
	case "read_file", "glob", "grep", "web_search", "web_fetch",
		"read_skill", "tool_search", "agent_list", "agent_get":
		return true
	default:
		return false
	}
}

// canExecuteToolLocked checks if a tool can execute based on current concurrency state.
// MUST be called with e.mu held.
// Matching upstream's canExecuteTool():
//   - No tools executing: YES (first tool always starts)
//   - This tool safe + all executing tools safe: YES (parallel safe tools)
//   - Otherwise: NO (non-concurrent tools need exclusive access)
func (e *StreamingToolExecutor) canExecuteToolLocked(isSafe bool) bool {
	executing := 0
	allExecutingSafe := true
	for _, t := range e.tools {
		if t.status == toolExecuting {
			executing++
			if !t.isConcurrencySafe {
				allExecutingSafe = false
			}
		}
	}
	// If a Bash sibling errored (hasErrored), prevent new unsafe tools from
	// starting. Safe tools are allowed to continue since they're independent.
	// This matches upstream's getAbortReason() returning 'sibling_error' when
	// hasErrored is true, which causes collectResults() to return a synthetic error.
	if !isSafe && e.hasErrored.Load() {
		return false
	}
	return executing == 0 || (isSafe && allExecutingSafe)
}

// processQueue scans the tool queue in order, starting tools when concurrency
// conditions allow. Non-concurrent tools that can't start yet block later tools.
// Matching upstream's processQueue() design.
func (e *StreamingToolExecutor) processQueue() {
	if !e.processing.CompareAndSwap(false, true) {
		return // already processing
	}
	defer e.processing.Store(false)

	e.mu.Lock()
	if e.discarded {
		e.mu.Unlock()
		return
	}
	tools := make([]*TrackedTool, len(e.tools))
	copy(tools, e.tools)
	e.mu.Unlock()

	for _, t := range tools {
		e.mu.Lock()
		if t.status != toolQueued || t.cancelled {
			e.mu.Unlock()
			continue
		}
		if e.canExecuteToolLocked(t.isConcurrencySafe) {
			t.status = toolExecuting
			e.mu.Unlock()
			e.startTool(t)
		} else {
			e.mu.Unlock()
			// Upstream: "since we need to maintain order for non-concurrent tools, stop here"
			if !t.isConcurrencySafe {
				break
			}
		}
	}
}

// Close discards the executor, preventing new tool dispatches.
func (e *StreamingToolExecutor) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.discarded = true
	if e.siblingCancel != nil {
		e.siblingCancel()
	}
}

// Start begins watching the channel and dispatching tool calls as they complete.
// The toolCalls slice is read-only after being passed to Start (the streaming
// handler appends to it, but we only access it at indices that have been sent
// on the channel, meaning they've already been appended).
func (e *StreamingToolExecutor) Start(toolCallDoneCh <-chan int, toolCalls *[]ToolCallInfo) {
	go func() {
		for idx := range toolCallDoneCh {
			e.mu.Lock()
			if e.discarded {
				e.mu.Unlock()
				return
			}
			e.mu.Unlock()

			if idx < 0 || idx >= len(*toolCalls) {
				continue
			}
			e.dispatch(idx, toolCalls)
		}
	}()
}

// dispatch queues a tool call and attempts to start execution.
// This is the Go equivalent of upstream's addTool() + processQueue().
func (e *StreamingToolExecutor) dispatch(idx int, toolCalls *[]ToolCallInfo) {
	tc := (*toolCalls)[idx]

	e.mu.Lock()
	if e.discarded {
		e.mu.Unlock()
		return
	}

	tool, exists := e.registry.Get(tc.Name)
	if !exists {
		// Unknown tool: record error immediately, no execution needed
		e.dispatched.Add(1)
		e.recordResult(toolExecResult{
			index:     idx,
			toolName:  tc.Name,
			toolUseID: tc.ID,
			isError:   true,
			output:    fmt.Sprintf("Error: Unknown tool %q", tc.Name),
		})
		e.completed.Add(1)
		e.mu.Unlock()
		return
	}

	tracked := &TrackedTool{
		tc:               tc,
		tool:             tool,
		status:           toolQueued,
		isConcurrencySafe: e.isConcurrencySafe(tc.Name, tc.Arguments),
		index:            idx,
	}
	e.tools = append(e.tools, tracked)
	e.toolsSeq++
	e.dispatched.Add(1) // Track dispatch immediately, execute() will add to completed
	seq := e.toolsSeq
	e.mu.Unlock()

	e.processQueue()

	// For unsafe tools, startTool executes synchronously on THIS goroutine.
	// Wait until it completes before returning, ensuring sequential execution
	// for unsafe tools (matching upstream's await executeTool for non-concurrent).
	e.waitForUnsafe(seq)
}

// waitForUnsafe blocks until the tool added at the given sequence number completes,
// if it was an unsafe (non-concurrent-safe) tool. Safe tools run asynchronously
// in goroutines and don't need waiting.
func (e *StreamingToolExecutor) waitForUnsafe(seq int32) {
	var doneCh chan struct{}
	e.mu.Lock()
	if int(seq) <= len(e.tools) {
		t := e.tools[seq-1]
		if !t.isConcurrencySafe {
			doneCh = t.execDoneCh
		}
	}
	e.mu.Unlock()
	if doneCh != nil {
		<-doneCh
	}
}

// startTool begins executing a tracked tool.
// For safe tools: runs in a goroutine (concurrent).
// For unsafe tools: runs synchronously on caller goroutine (exclusive).
func (e *StreamingToolExecutor) startTool(t *TrackedTool) {
	if t.isConcurrencySafe {
		e.semaphore <- struct{}{} // acquire slot
		go func() {
			defer func() { <-e.semaphore }() // release slot
			e.execute(t.index, t.tc, t.tool, t)
			e.processQueue() // try to start more queued tools
		}()
	} else {
		// Unsafe tool: execute synchronously to block processQueue
		t.execDoneCh = make(chan struct{})
		e.execute(t.index, t.tc, t.tool, t)
		close(t.execDoneCh)
		e.processQueue() // try to start more queued tools
	}
}

// getToolDescription returns a short description of the tool for error messages,
// matching upstream's getToolDescription().
func (e *StreamingToolExecutor) getToolDescription(tc ToolCallInfo) string {
	var input map[string]any
	if tc.Arguments != "" {
		_ = json.Unmarshal([]byte(tc.Arguments), &input)
	}
	if cmd, ok := input["command"].(string); ok && cmd != "" {
		truncated := cmd
		if len(truncated) > 40 {
			truncated = truncated[:40] + "\u2026"
		}
		return fmt.Sprintf("%s(%s)", tc.Name, truncated)
	}
	if path, ok := input["file_path"].(string); ok && path != "" {
		return fmt.Sprintf("%s(%s)", tc.Name, path)
	}
	if pattern, ok := input["pattern"].(string); ok && pattern != "" {
		return fmt.Sprintf("%s(%s)", tc.Name, pattern)
	}
	return tc.Name
}

// createSyntheticErrorMessage creates an error message for a cancelled tool,
// matching upstream's createSyntheticErrorMessage() for the sibling_error reason.
func (e *StreamingToolExecutor) createSyntheticErrorMessage(tc ToolCallInfo) string {
	desc := e.getToolDescription(tc)
	if desc != "" {
		return fmt.Sprintf("Cancelled: parallel tool call %s errored", desc)
	}
	return "Cancelled: parallel tool call errored"
}

// execute runs a single tool call and records the result.
// Enhanced with per-tool abort context from siblingCtx.
func (e *StreamingToolExecutor) execute(idx int, tc ToolCallInfo, tool tools.Tool, tracked *TrackedTool) {
	start := time.Now()

	// Check if cancelled by sibling error (pre-execution abort check)
	if tracked.cancelled {
		e.recordResult(toolExecResult{
			index:     idx,
			toolName:  tc.Name,
			toolUseID: tc.ID,
			isError:   true,
			output:    e.createSyntheticErrorMessage(tc),
			duration:  time.Since(start),
		})
		e.completed.Add(1)
		return
	}

	// Create per-tool abort context as child of sibling context.
	// When siblingCtx is cancelled (Bash error), this context is also cancelled.
	// This enables tool implementations to check for cancellation if they support it.
	// (Most Go tools don't check context yet, but the structure matches upstream.)
	execCtx, execCancel := context.WithCancel(e.siblingCtx)
	defer execCancel()
	_ = execCtx // reserved for future context-aware tool execution

	// Guard against empty toolUseID
	if tc.ID == "" {
		tc.ID = "synthetic_tool_use_id"
	}

	input := make(map[string]any)
	if tc.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
			e.recordResult(toolExecResult{
				index:     idx,
				toolName:  tc.Name,
				toolUseID: tc.ID,
				isError:   true,
				output:    fmt.Sprintf("Error: failed to parse tool arguments: %v\nRaw arguments: %s", err, tc.Arguments),
				duration:  time.Since(start),
			})
			e.completed.Add(1)
			return
		}
	}

	// Check permissions
	if e.gate != nil {
		denial := e.gate.Check(tool, input)
		if denial != nil {
			e.recordResult(toolExecResult{
				index:     idx,
				toolName:  tc.Name,
				toolUseID: tc.ID,
				isError:   true,
				output:    denial.Output,
				duration:  time.Since(start),
			})
			e.completed.Add(1)
			return
		}
	}

	// PreToolUse shell hooks: execute matching hooks before tool execution.
	// Hooks can block, modify input, or allow through.
	// Matching upstream's executePreToolHooks().
	if e.hooks != nil {
		if blockErr := e.executePreToolUseHooks(tc, input); blockErr != nil {
			e.recordResult(toolExecResult{
				index:     idx,
				toolName:  tc.Name,
				toolUseID: tc.ID,
				isError:   true,
				output:    fmt.Sprintf("Blocked by PreToolUse hook: %v", blockErr),
				duration:  time.Since(start),
			})
			e.completed.Add(1)
			return
		}
	}

	// Auto-snapshot before write/edit tools (matches agent_loop.go)
	if e.snapshots != nil && (tc.Name == "write_file" || tc.Name == "edit_file" || tc.Name == "multi_edit") {
		if path := extractFilePathStreaming(input); path != "" {
			_ = e.snapshots.TakeSnapshotWithDesc(path, "before "+tc.Name)
		}
	}

	// Recover from panics inside tool execution, matching agent_loop.go's
	// panic safety net.
	var result tools.ToolResult
	func() {
		defer func() {
			if r := recover(); r != nil {
				result = tools.ToolResult{
					Output:  fmt.Sprintf("Error: tool execution panicked: %v", r),
					IsError: true,
				}
			}
		}()
		result = tool.Execute(input)
	}()

	wasError := result.IsError

	// Post-snapshot and diff for write/edit tools (matches agent_loop.go)
	if e.snapshots != nil && !result.IsError && (tc.Name == "write_file" || tc.Name == "edit_file" || tc.Name == "multi_edit") {
		if path := extractFilePathStreaming(input); path != "" {
			desc := tc.Name
			if tc.Name == "edit_file" {
				if oldStr, ok := input["old_string"].(string); ok {
					if newStr, ok2 := input["new_string"].(string); ok2 {
						desc = fmt.Sprintf("edit: '%s' -> '%s'", limitStrStreaming(oldStr, 50), limitStrStreaming(newStr, 50))
					}
				}
			}
			_ = e.snapshots.TakeSnapshotWithDesc(path, desc)
			if diffStr := diffLastTwoSnapshots(e.snapshots, path); diffStr != "" {
				result.Output += "\n\n--- diff ---\n" + diffStr
			}
		}
	}

	// CRITICAL: After execution, check if a sibling tool errored while this
	// tool was running. Matching upstream's getAbortReason() check inside
	// the for-await loop (StreamingToolExecutor.ts:344):
	//   const abortReason = this.getAbortReason(tool)
	//   if (abortReason && !thisToolErrored) { synthetic error; break }
	// Since Go tools execute synchronously (no streaming loop), we can only
	// check after execution returns. If siblingCtx was cancelled, a Bash
	// sibling errored — this tool should report a synthetic error unless it
	// was the one that errored.
	if e.siblingCtx.Err() != nil && !(wasError && tc.Name == "exec") {
		// Sibling Bash errored — cancel this tool
		e.recordResult(toolExecResult{
			index:     idx,
			toolName:  tc.Name,
			toolUseID: tc.ID,
			isError:   true,
			output:    e.createSyntheticErrorMessage(tc),
			duration:  time.Since(start),
		})
		e.completed.Add(1)
		return
	}

	e.recordResult(toolExecResult{
		index:     idx,
		toolName:  tc.Name,
		toolUseID: tc.ID,
		output:    result.Output,
		isError:   result.IsError,
		duration:  time.Since(start),
	})

	// PostToolUse shell hooks: execute matching hooks after tool execution.
	// Matching upstream's executePostToolHooks().
	if e.hooks != nil {
		e.executePostToolUseHooks(tc, input, result)
	}

	// Mark the tracked tool as completed so subsequent tools can start.
	// This was a subtle bug: execute() incremented completed but never
	// updated status from toolExecuting to toolCompleted, causing
	// canExecuteToolLocked() to think an unsafe tool was still running.
	e.mu.Lock()
	for _, t := range e.tools {
		if t.index == idx && t.status == toolExecuting {
			t.status = toolCompleted
			break
		}
	}
	e.mu.Unlock()

	// CRITICAL: Only Bash errors cancel sibling tools.
	// Matching upstream (StreamingToolExecutor.ts:368):
	// "Only Bash errors cancel siblings. Bash commands often have implicit
	// dependency chains (e.g. mkdir fails -> subsequent commands pointless).
	// Read/WebFetch/etc are independent - one failure shouldn't nuke the rest."
	if wasError && tc.Name == "exec" {
		e.hasErrored.Store(true)
		e.erroredToolDescription = e.getToolDescription(tc)
		e.siblingCtxDone()
		// Cancel queued unsafe tools. Safe tools are left queued and will
		// be caught by the post-execution siblingCtx.Err() check.
		e.cancelRemaining(idx)
	}

	e.completed.Add(1)
}

// siblingCtxDone cancels the sibling abort context, cascading the error to all
// currently executing and queued tools whose execCtx derives from siblingCtx.
func (e *StreamingToolExecutor) siblingCtxDone() {
	if e.siblingCancel != nil {
		e.siblingCancel()
	}
}

// cancelRemaining marks queued tools that cannot be cancelled via siblingCtx
// as cancelled. Matching upstream's design:
//   - Non-concurrency-safe (unsafe) queued tools are cancelled here directly,
//     since they haven't started yet and need immediate cancellation.
//   - Concurrency-safe (safe) queued tools are left in queued state; they
//     will be cancelled by the post-execution siblingCtx.Err() check in
//     execute() when they eventually run.
//   - Safe tools ALREADY executing in goroutines are also caught by the
//     post-execution siblingCtx.Err() check.
func (e *StreamingToolExecutor) cancelRemaining(errorIdx int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, t := range e.tools {
		if t.index != errorIdx && t.status == toolQueued && !t.cancelled {
			// Only cancel unsafe tools here. Safe tools will be caught by
			// the post-execution siblingCtx.Err() check in execute().
			if !t.isConcurrencySafe {
				t.cancelled = true
				t.status = toolCompleted
				if t.execDoneCh != nil {
					close(t.execDoneCh)
				}
				e.recordResult(toolExecResult{
					index:     t.index,
					toolName:  t.tc.Name,
					toolUseID: t.tc.ID,
					isError:   true,
					output:    e.createSyntheticErrorMessage(t.tc),
				})
				e.completed.Add(1)
			}
		}
	}
}

// recordResult stores a tool execution result.
func (e *StreamingToolExecutor) recordResult(r toolExecResult) {
	e.resultsMu.Lock()
	defer e.resultsMu.Unlock()
	e.results = append(e.results, r)
}

// Wait blocks until all dispatched tool calls have completed, then returns
// results in tool call index order.
// If ctx is cancelled (e.g. user Ctrl+C), Wait returns early with whatever
// results have been collected so far.
func (e *StreamingToolExecutor) Wait(ctx context.Context, totalCalls int) []toolExecResult {
	deadline := time.Now().Add(5 * time.Minute)
	graceDeadline := time.Now().Add(1 * time.Second)
	for {
		e.mu.Lock()
		if e.discarded {
			e.mu.Unlock()
			break
		}
		e.mu.Unlock()

		// Check for external cancellation (user Ctrl+C, parent context, etc.)
		if ctx != nil {
			select {
			case <-ctx.Done():
				e.mu.Lock()
				e.discarded = true
				e.mu.Unlock()
				// Note: Go's 'break' in select only breaks the select, not the for loop.
				// The discarded check at the top of the loop will catch this on next iteration.
			default:
			}
		}
		if e.discarded {
			break
		}

		completed := e.completed.Load()
		dispatched := e.dispatched.Load()
		if dispatched > 0 && int(completed) >= int(dispatched) {
			break
		}
		// Fallback: if we have enough completed results
		if int(completed) >= totalCalls {
			break
		}
		// Error: all dispatched tools have finished (including cancelled ones)
		if e.hasErrored.Load() && int(completed) >= int(dispatched) {
			break
		}
		// If nothing was dispatched after a grace period, the Start
		// goroutine has likely exited (channel closed before Wait) or
		// never started. Break to avoid a 5-minute hang.
		// We give a 1-second grace period because the Start goroutine
		// may not have processed items yet.
		if dispatched == 0 && time.Now().After(graceDeadline) {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	e.resultsMu.Lock()
	defer e.resultsMu.Unlock()

	results := make([]toolExecResult, len(e.results))
	copy(results, e.results)

	// Insertion sort by index (small N)
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].index < results[j-1].index; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	return results
}

// executePreToolUseHooks runs PreToolUse shell hooks matching the tool name.
// Returns a HookBlockError if any hook blocks execution.
// Matching upstream's executePreToolHooks() in hooks.ts.
func (e *StreamingToolExecutor) executePreToolUseHooks(tc ToolCallInfo, input map[string]any) error {
	hooks := e.hooks["PreToolUse"]
	if len(hooks) == 0 {
		return nil
	}

	for _, hook := range hooks {
		if !MatchHook(hook, tc.Name) {
			continue
		}

		// Build hook input matching upstream's PreToolUseInput schema
		hookInput := map[string]interface{}{
			"tool_name":  tc.Name,
			"tool_input": input,
			"session_id": getSessionID(),
		}

		jsonBytes, err := json.Marshal(hookInput)
		if err != nil {
			log.Printf("PreToolUse hook: failed to marshal input: %v", err)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultHookTimeout)
		defer cancel()

		result, err := ExecuteShellHook(ctx, hook, HookPreToolUse, string(jsonBytes), nil)
		if err != nil {
			log.Printf("PreToolUse hook %q failed: %v", hook.Command, err)
			continue
		}

		if result.ShouldBlock() {
			return HookBlockError{
				ToolName: tc.Name,
				Command:  hook.Command,
				Reason:   result.BlockReason(),
			}
		}

		// If hook provides updated input, merge it
		if result.UpdatedInput != nil {
			for k, v := range result.UpdatedInput {
				input[k] = v
			}
		}
	}

	return nil
}

// executePostToolUseHooks runs PostToolUse shell hooks matching the tool name.
// Matching upstream's executePostToolHooks() in hooks.ts.
func (e *StreamingToolExecutor) executePostToolUseHooks(tc ToolCallInfo, input map[string]any, result tools.ToolResult) {
	hooks := e.hooks["PostToolUse"]
	if len(hooks) == 0 {
		return
	}

	for _, hook := range hooks {
		if !MatchHook(hook, tc.Name) {
			continue
		}

		// Build hook input matching upstream's PostToolUseInput schema
		hookInput := map[string]interface{}{
			"tool_name":   tc.Name,
			"tool_input":  input,
			"tool_output": result.Output,
			"session_id":  getSessionID(),
		}

		if result.IsError {
			hookInput["tool_error"] = true
		}

		jsonBytes, err := json.Marshal(hookInput)
		if err != nil {
			log.Printf("PostToolUse hook: failed to marshal input: %v", err)
			continue
		}

		// PostToolUse hooks run asynchronously (fire-and-forget)
		// Matching upstream's async PostToolUse execution
		go func(h HookCommand, jsonStr string) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultHookTimeout)
			defer cancel()

			_, err := ExecuteShellHook(ctx, h, HookPostToolUse, jsonStr, nil)
			if err != nil {
				log.Printf("PostToolUse hook %q failed: %v", h.Command, err)
			}
		}(hook, string(jsonBytes))
	}
}

// getSessionID returns the current session ID for hook context.
// Falls back to a process-level ID if session tracking is not available.
func getSessionID() string {
	// Try to get from the global session tracker if available
	if sid := os.Getenv("CLAUDE_SESSION_ID"); sid != "" {
		return sid
	}
	return "unknown"
}

// extractFilePathStreaming extracts a file path from tool input parameters,
// matching the logic in agent_loop.go's extractFilePath.
func extractFilePathStreaming(input map[string]any) string {
	if path, ok := input["file_path"].(string); ok && path != "" {
		return expandPath(path)
	}
	return ""
}

// limitStrStreaming returns at most n characters of s, with "..." appended if truncated.
func limitStrStreaming(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
