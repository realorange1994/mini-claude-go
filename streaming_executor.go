package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	registry *tools.Registry
	gate     *PermissionGate

	// Tool queue
	tools     []*TrackedTool
	toolsSeq  int32 // tracks number of tools added, for Wait() counting
	processing atomic.Bool
	mu         sync.Mutex

	// State
	dispatched atomic.Int32
	completed  atomic.Int32
	hasErrored atomic.Bool

	// Results
	results   []toolExecResult
	resultsMu sync.Mutex

	// Concurrency gating
	semaphore chan struct{}

	stopped   bool
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
func NewStreamingToolExecutor(registry *tools.Registry, gate *PermissionGate) *StreamingToolExecutor {
	siblingCtx, siblingCancel := context.WithCancel(context.Background())
	return &StreamingToolExecutor{
		registry:    registry,
		gate:        gate,
		semaphore:   make(chan struct{}, 10), // up to 10 concurrent tools
		siblingCtx:  siblingCtx,
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
// Read-only tools are safe; anything that writes or executes commands is not.
func (e *StreamingToolExecutor) isConcurrencySafe(toolName string) bool {
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
	if e.stopped || e.discarded {
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

// Close stops dispatching new tool calls.
func (e *StreamingToolExecutor) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopped = true
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
			if e.stopped {
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
	if e.stopped || e.discarded {
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
		isConcurrencySafe: e.isConcurrencySafe(tc.Name),
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

	e.recordResult(toolExecResult{
		index:     idx,
		toolName:  tc.Name,
		toolUseID: tc.ID,
		output:    result.Output,
		isError:   result.IsError,
		duration:  time.Since(start),
	})

	// CRITICAL: Only Bash errors cancel sibling tools.
	// Matching upstream (StreamingToolExecutor.ts:368):
	// "Only Bash errors cancel siblings. Bash commands often have implicit
	// dependency chains (e.g. mkdir fails -> subsequent commands pointless).
	// Read/WebFetch/etc are independent - one failure shouldn't nuke the rest."
	if wasError && tc.Name == "exec" {
		e.hasErrored.Store(true)
		desc := e.getToolDescription(tc)
		_ = desc // available for synthetic error messages
		e.siblingCtxDone()
		e.mu.Lock()
		e.stopped = true
		e.mu.Unlock()
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

// cancelRemaining marks all queued tools (except the one at errorIdx) as
// cancelled and records synthetic error results for them.
func (e *StreamingToolExecutor) cancelRemaining(errorIdx int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, t := range e.tools {
		if t.index != errorIdx && t.status == toolQueued && !t.cancelled {
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

// recordResult stores a tool execution result.
func (e *StreamingToolExecutor) recordResult(r toolExecResult) {
	e.resultsMu.Lock()
	defer e.resultsMu.Unlock()
	e.results = append(e.results, r)
}

// Wait blocks until all dispatched tool calls have completed, then returns
// results in tool call index order.
func (e *StreamingToolExecutor) Wait(totalCalls int) []toolExecResult {
	deadline := time.Now().Add(5 * time.Minute)
	for {
		e.mu.Lock()
		if e.discarded {
			e.mu.Unlock()
			break
		}
		e.mu.Unlock()

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
