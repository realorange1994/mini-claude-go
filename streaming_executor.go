package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"miniclaudecode-go/tools"
)

// StreamingToolExecutor executes tool calls as they complete during streaming,
// overlapping tool execution with ongoing stream processing.
type StreamingToolExecutor struct {
	registry   *tools.Registry
	gate       *PermissionGate

	// State
	dispatched atomic.Int32
	completed  atomic.Int32
	hasErrored atomic.Bool

	results   []toolExecResult
	resultsMu sync.Mutex

	// Concurrency gating
	semaphore chan struct{}
	mu        sync.Mutex

	stopped bool
}

type toolExecResult struct {
	index     int
	toolName  string
	toolUseID string
	output    string
	isError   bool
	duration  time.Duration
}

// NewStreamingToolExecutor creates a new executor.
func NewStreamingToolExecutor(registry *tools.Registry, gate *PermissionGate) *StreamingToolExecutor {
	return &StreamingToolExecutor{
		registry:  registry,
		gate:      gate,
		semaphore: make(chan struct{}, 10), // up to 10 concurrent tools
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

// dispatch sends a tool call for execution.
func (e *StreamingToolExecutor) dispatch(idx int, toolCalls *[]ToolCallInfo) {
	if e.hasErrored.Load() {
		return
	}

	tc := (*toolCalls)[idx]
	tool, exists := e.registry.Get(tc.Name)
	if !exists {
		e.recordResult(toolExecResult{
			index:     idx,
			toolName:  tc.Name,
			toolUseID: tc.ID,
			isError:   true,
			output:    fmt.Sprintf("Error: Unknown tool %q", tc.Name),
		})
		return
	}

	if e.isConcurrencySafe(tc.Name) {
		// Safe to run in parallel — dispatch immediately
		e.semaphore <- struct{}{} // acquire slot
		go func() {
			defer func() { <-e.semaphore }() // release slot
			e.execute(idx, tc, tool)
		}()
	} else {
		// Not safe for concurrent execution — acquire semaphore exclusively
		e.semaphore <- struct{}{}
		e.execute(idx, tc, tool)
		<-e.semaphore
	}
}

// execute runs a single tool call and records the result.
func (e *StreamingToolExecutor) execute(idx int, tc ToolCallInfo, tool tools.Tool) {
	e.dispatched.Add(1)
	start := time.Now()

	input := make(map[string]any)
	if tc.Arguments != "" {
		_ = json.Unmarshal([]byte(tc.Arguments), &input)
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

	result := tool.Execute(input)
	e.recordResult(toolExecResult{
		index:     idx,
		toolName:  tc.Name,
		toolUseID: tc.ID,
		output:    result.Output,
		isError:   result.IsError,
		duration:  time.Since(start),
	})

	if result.IsError {
		e.hasErrored.Store(true)
		e.mu.Lock()
		e.stopped = true
		e.mu.Unlock()
	}

	e.completed.Add(1)
}

// recordResult stores a tool execution result.
func (e *StreamingToolExecutor) recordResult(r toolExecResult) {
	e.resultsMu.Lock()
	defer e.resultsMu.Unlock()
	e.results = append(e.results, r)
}

// Wait blocks until all dispatched tool calls have completed, then returns
// results in tool call index order.
// totalCalls is the total number of tool calls expected.
func (e *StreamingToolExecutor) Wait(totalCalls int) []toolExecResult {
	deadline := time.Now().Add(5 * time.Minute)
	for {
		completed := e.completed.Load()
		if int(completed) >= totalCalls {
			break
		}
		if e.hasErrored.Load() && int(completed) >= int(e.dispatched.Load()) {
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

// Close stops dispatching new tool calls.
func (e *StreamingToolExecutor) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopped = true
}
