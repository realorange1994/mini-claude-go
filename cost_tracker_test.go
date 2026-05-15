package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCostTrackerAddUsage(t *testing.T) {
	ct := NewCostTracker()

	ct.AddUsage("claude-sonnet-4-20250514", 1000, 500)
	ct.AddUsage("claude-sonnet-4-20250514", 2000, 1000)
	ct.AddUsage("claude-opus-4-5-20250610", 500, 200)

	// Verify totals
	if ct.TotalInputTokens != 3500 {
		t.Errorf("TotalInputTokens = %d, want 3500", ct.TotalInputTokens)
	}
	if ct.TotalOutputTokens != 1700 {
		t.Errorf("TotalOutputTokens = %d, want 1700", ct.TotalOutputTokens)
	}

	// Verify per-model breakdown
	usage := ct.GetPerModelUsage()
	if len(usage) != 2 {
		t.Errorf("expected 2 models, got %d", len(usage))
	}

	sonnet := usage["claude-sonnet-4-20250514"]
	if sonnet.InputTokens != 3000 {
		t.Errorf("sonnet input tokens = %d, want 3000", sonnet.InputTokens)
	}
	if sonnet.OutputTokens != 1500 {
		t.Errorf("sonnet output tokens = %d, want 1500", sonnet.OutputTokens)
	}

	opus := usage["claude-opus-4-5-20250610"]
	if opus.InputTokens != 500 {
		t.Errorf("opus input tokens = %d, want 500", opus.InputTokens)
	}
	if opus.OutputTokens != 200 {
		t.Errorf("opus output tokens = %d, want 200", opus.OutputTokens)
	}
}

func TestCostTrackerAddUsageZeroSkipped(t *testing.T) {
	ct := NewCostTracker()
	ct.AddUsage("test-model", 0, 0)

	if len(ct.PerModel) != 0 {
		t.Error("zero usage should not create a model entry")
	}
}

func TestFormatCostDisplay(t *testing.T) {
	ct := NewCostTracker()

	// Empty tracker
	display := ct.FormatCostDisplay()
	if display != "Total: 0 tokens" {
		t.Errorf("empty display = %q, want %q", display, "Total: 0 tokens")
	}

	// After some usage
	ct.AddUsage("claude-sonnet-4-20250514", 1_000_000, 500_000)
	ct.AddUsage("claude-opus-4-5-20250610", 1_000_000, 500_000)

	display = ct.FormatCostDisplay()
	if len(display) < 10 {
		t.Errorf("display too short: %q", display)
	}
	// Should contain both families and token counts
	if !strings.Contains(display, "Total") {
		t.Errorf("display missing Total: %q", display)
	}
	if !strings.Contains(display, "Sonnet") {
		t.Errorf("display missing Sonnet: %q", display)
	}
	if !strings.Contains(display, "Opus") {
		t.Errorf("display missing Opus: %q", display)
	}
	// Should show token counts, not dollar amounts
	if strings.Contains(display, "$") {
		t.Errorf("display should not contain $: %q", display)
	}
}

func TestFormatCostDisplayMiniMax(t *testing.T) {
	ct := NewCostTracker()
	ct.AddUsage("MiniMax-M2.7", 100_000, 50_000)

	display := ct.FormatCostDisplay()
	if !strings.Contains(display, "m2.7") {
		t.Errorf("display missing m2.7 family name: %q", display)
	}
	if !strings.Contains(display, "100") {
		t.Errorf("display missing input token count: %q", display)
	}
}

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{500, "500"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{1_000_000, "1.0M"},
		{1_500_000, "1.5M"},
	}
	for _, tt := range tests {
		got := formatTokenCount(tt.n)
		if got != tt.want {
			t.Errorf("formatTokenCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFamilyName(t *testing.T) {
	tests := []struct {
		model, want string
	}{
		{"claude-sonnet-4-20250514", "Sonnet"},
		{"claude-3-5-sonnet-20241022", "Sonnet"},
		{"claude-opus-4-20250514", "Opus"},
		{"claude-opus-4-5-20250610", "Opus"},
		{"claude-3-5-haiku-20241022", "Haiku"},
		{"claude-haiku-4-5-20250610", "Haiku"},
		{"MiniMax-M2.7", "m2.7"},
		{"MiniMax-M2.5-highspeed", "m2.5"},
		{"deepseek-v3-2-250612", "DeepSeek"},
		{"kimi-k2-5-250625", "Kimi"},
		{"glm-5-0-250625", "GLM"},
		{"qwen-3-5-plus-250625", "Qwen"},
		{"doubao-seed-2-0-pro-260215", "Doubao"},
	}

	for _, tt := range tests {
		got := familyName(tt.model)
		if got != tt.want {
			t.Errorf("familyName(%q) = %q, want %q", tt.model, got, tt.want)
		}
	}
}

func TestCostTrackerPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cost_tracker.json")

	// Create and populate tracker
	ct1 := NewCostTracker()
	ct1.AddUsage("claude-sonnet-4-20250514", 5000, 2000)
	ct1.AddUsage("claude-opus-4-5-20250610", 1000, 500)

	// Save
	if err := ct1.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	// Load into fresh tracker
	ct2 := NewCostTracker()
	if err := ct2.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	// Verify totals match
	if ct1.TotalInputTokens != ct2.TotalInputTokens {
		t.Errorf("total input mismatch: %d vs %d", ct1.TotalInputTokens, ct2.TotalInputTokens)
	}
	if ct1.TotalOutputTokens != ct2.TotalOutputTokens {
		t.Errorf("total output mismatch: %d vs %d", ct1.TotalOutputTokens, ct2.TotalOutputTokens)
	}

	usage1 := ct1.GetPerModelUsage()
	usage2 := ct2.GetPerModelUsage()

	for model, entry1 := range usage1 {
		entry2, ok := usage2[model]
		if !ok {
			t.Errorf("missing model %s in loaded tracker", model)
			continue
		}
		if entry1.InputTokens != entry2.InputTokens {
			t.Errorf("%s: input tokens %d vs %d", model, entry1.InputTokens, entry2.InputTokens)
		}
		if entry1.OutputTokens != entry2.OutputTokens {
			t.Errorf("%s: output tokens %d vs %d", model, entry1.OutputTokens, entry2.OutputTokens)
		}
	}
}

func TestCostTrackerLoadNonexistentFile(t *testing.T) {
	ct := NewCostTracker()
	err := ct.LoadFromFile("/tmp/nonexistent_cost_tracker_12345.json")
	if err != nil {
		t.Errorf("LoadFromFile nonexistent file should return nil, got: %v", err)
	}
}

func TestCostTrackerConcurrentAccess(t *testing.T) {
	ct := NewCostTracker()
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				ct.AddUsage("claude-sonnet-4-20250514", 100, 50)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// 10 goroutines * 100 calls * (100 input + 50 output) tokens
	usage := ct.GetPerModelUsage()
	sonnet := usage["claude-sonnet-4-20250514"]
	if sonnet.InputTokens != 10*100*100 {
		t.Errorf("concurrent input tokens = %d, want %d", sonnet.InputTokens, 10*100*100)
	}
	if sonnet.OutputTokens != 10*100*50 {
		t.Errorf("concurrent output tokens = %d, want %d", sonnet.OutputTokens, 10*100*50)
	}
}

func TestGetPerModelUsageReturnsCopy(t *testing.T) {
	ct := NewCostTracker()
	ct.AddUsage("claude-sonnet-4-20250514", 1000, 500)

	// GetPerModelUsage returns map[string]ModelUsage (value types)
	// Modifying the returned value doesn't affect the original
	usage := ct.GetPerModelUsage()
	sonnet := usage["claude-sonnet-4-20250514"]
	sonnet.InputTokens = 999999 // modifies local copy, not the map value

	// Original should be unchanged
	usage2 := ct.GetPerModelUsage()
	if usage2["claude-sonnet-4-20250514"].InputTokens == 999999 {
		t.Error("GetPerModelUsage returned mutable reference, not a copy")
	}
	if usage2["claude-sonnet-4-20250514"].InputTokens != 1000 {
		t.Errorf("expected original input tokens 1000, got %d", usage2["claude-sonnet-4-20250514"].InputTokens)
	}
}

func TestCostTrackerTokenCountAccuracy(t *testing.T) {
	ct := NewCostTracker()
	ct.AddUsage("claude-sonnet-4-20250514", 1_000, 500)
	ct.AddUsage("claude-sonnet-4-20250514", 2_000, 1_000)

	if ct.TotalInputTokens != 3_000 {
		t.Errorf("TotalInputTokens = %d, want 3000", ct.TotalInputTokens)
	}
	if ct.TotalOutputTokens != 1_500 {
		t.Errorf("TotalOutputTokens = %d, want 1500", ct.TotalOutputTokens)
	}
}
