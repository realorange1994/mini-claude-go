package main

import (
	"context"
	"testing"
	"time"

	"miniclaudecode-go/tools"
)

func TestStreamingExecutorBothSafeExec(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&tools.ExecTool{})
	executor := NewStreamingToolExecutor(registry, nil, nil, nil)

	toolCalls := []ToolCallInfo{
		{
			Name:      "exec",
			ID:        "call_0",
			Arguments: `{"command":"env | grep -i rust"}`,
		},
		{
			Name:      "exec",
			ID:        "call_1",
			Arguments: `{"command":"ls -la ~/.cargo/ 2>/dev/null || echo no"}`,
		},
	}

	doneCh := make(chan int, 20)
	executor.Start(doneCh, &toolCalls)

	doneCh <- 0
	doneCh <- 1
	close(doneCh)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results := executor.Wait(ctx, len(toolCalls))

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		t.Logf("tool[%d] name=%s isError=%v outputLen=%d",
			r.index, r.toolName, r.isError, len(r.output))
	}
}

func TestStreamingExecutorUnsafeExecSequential(t *testing.T) {
	// Test with commands that have shell operators (&&) making them unsafe
	registry := tools.NewRegistry()
	registry.Register(&tools.ExecTool{})
	executor := NewStreamingToolExecutor(registry, nil, nil, nil)

	toolCalls := []ToolCallInfo{
		{
			Name:      "exec",
			ID:        "call_0",
			Arguments: `{"command":"echo a && ls"}`,
		},
		{
			Name:      "exec",
			ID:        "call_1",
			Arguments: `{"command":"echo b && ls"}`,
		},
	}

	doneCh := make(chan int, 20)
	executor.Start(doneCh, &toolCalls)

	doneCh <- 0
	doneCh <- 1
	close(doneCh)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results := executor.Wait(ctx, len(toolCalls))

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		t.Logf("tool[%d] name=%s isError=%v outputLen=%d",
			r.index, r.toolName, r.isError, len(r.output))
	}
}
