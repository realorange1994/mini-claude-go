package main

import (
	"strings"
	"testing"

	"miniclaudecode-go/tools"
)

func TestDoomLoopDetector_NoLoop(t *testing.T) {
	d := NewDoomLoopDetector()

	// Different calls should not trigger doom loop
	if d.CheckRecord("read_file", map[string]any{"path": "a.txt"}) {
		t.Error("should not trigger on first call")
	}
	if d.CheckRecord("read_file", map[string]any{"path": "b.txt"}) {
		t.Error("should not trigger on different args")
	}
	if d.CheckRecord("exec", map[string]any{"command": "ls"}) {
		t.Error("should not trigger on different tool")
	}
}

func TestDoomLoopDetector_DetectsLoop(t *testing.T) {
	d := NewDoomLoopDetector()

	args := map[string]any{"path": "a.txt"}
	d.CheckRecord("read_file", args)
	d.CheckRecord("read_file", args)
	triggered := d.CheckRecord("read_file", args)

	if !triggered {
		t.Error("should trigger after 3 identical calls")
	}
}

func TestDoomLoopDetector_ResetOnDifferent(t *testing.T) {
	d := NewDoomLoopDetector()

	args := map[string]any{"path": "a.txt"}
	d.CheckRecord("read_file", args)
	d.CheckRecord("read_file", args)
	// Different call resets the counter
	d.CheckRecord("exec", map[string]any{"command": "ls"})
	triggered := d.CheckRecord("read_file", args)

	if triggered {
		t.Error("should not trigger after counter was reset")
	}
}

func TestDoomLoopDetector_Clear(t *testing.T) {
	d := NewDoomLoopDetector()

	args := map[string]any{"path": "a.txt"}
	d.CheckRecord("read_file", args)
	d.CheckRecord("read_file", args)
	d.Clear()
	triggered := d.CheckRecord("read_file", args)

	if triggered {
		t.Error("should not trigger after clear")
	}
}

func TestSmartTruncate_SmallOutput(t *testing.T) {
	output := "hello world"
	result, truncated := tools.SmartTruncate(output, 100)
	if truncated {
		t.Error("should not truncate small output")
	}
	if result != output {
		t.Errorf("expected unchanged output, got %q", result)
	}
}

func TestSmartTruncate_LargeOutputNoError(t *testing.T) {
	// Create large output without errors
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "normal line"
	}
	output := ""
	for _, l := range lines {
		output += l + "\n"
	}

	result, truncated := tools.SmartTruncate(output, 200)
	if !truncated {
		t.Error("should truncate large output")
	}
	if len(result) >= len(output) {
		t.Error("truncated result should be smaller")
	}
}

func TestSmartTruncate_LargeOutputWithError(t *testing.T) {
	// Create large output with error in tail
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "normal line"
	}
	lines[95] = "ERROR: something failed"
	lines[96] = "panic: oh no"

	output := ""
	for _, l := range lines {
		output += l + "\n"
	}

	result, truncated := tools.SmartTruncate(output, 200)
	if !truncated {
		t.Error("should truncate large output")
	}
	// Should preserve error context
	if !strings.Contains(result, "ERROR") && !strings.Contains(result, "panic") {
		t.Error("should preserve error context from tail")
	}
}

func TestToolResultRecoverable(t *testing.T) {
	result := tools.ToolResultRecoverable("bad arguments")
	if !result.IsError {
		t.Error("expected error result")
	}
	if !result.Metadata.Recoverable {
		t.Error("expected recoverable flag")
	}
}
