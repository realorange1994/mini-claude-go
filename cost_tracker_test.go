package main

import (
	"math"
	"path/filepath"
	"testing"
)

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

func TestCalculateUSDCost(t *testing.T) {
	tests := []struct {
		model          string
		input, output  int64
		cacheWrite, cacheRead int64
		want           float64
	}{
		// Sonnet: 3.0/15.0/3.75/0.30 per M tokens
		{"claude-sonnet-4-20250514", 1_000_000, 500_000, 0, 0, 3.0 + 7.5},
		{"claude-3-5-sonnet-20241022", 1_000_000, 0, 1_000_000, 500_000, 3.0 + 3.75 + 0.15},
		// Opus 4: 15.0/75.0/18.75/1.50
		{"claude-opus-4-20250514", 1_000_000, 1_000_000, 0, 0, 15.0 + 75.0},
		// Opus 4.5: 5.0/25.0/6.25/0.50
		{"claude-opus-4-5-20250610", 2_000_000, 1_000_000, 0, 0, 10.0 + 25.0},
		// Haiku 3.5: 0.80/4.0/1.0/0.08
		{"claude-3-5-haiku-20241022", 1_000_000, 1_000_000, 0, 0, 0.80 + 4.0},
		// Haiku 4.5: 1.0/5.0/1.25/0.10
		{"claude-haiku-4-5-20250610", 1_000_000, 1_000_000, 0, 0, 1.0 + 5.0},
		// Zero usage
		{"claude-sonnet-4-20250514", 0, 0, 0, 0, 0},
		// Unknown model => zero cost
		{"unknown-model", 1_000_000, 0, 0, 0, 0},
	}

	for _, tt := range tests {
		got := CalculateUSDCost(tt.model, tt.input, tt.output, tt.cacheWrite, tt.cacheRead)
		if !almostEqual(got, tt.want, 0.001) {
			t.Errorf("CalculateUSDCost(%s, %d, %d, %d, %d) = %.4f, want %.4f",
				tt.model, tt.input, tt.output, tt.cacheWrite, tt.cacheRead, got, tt.want)
		}
	}
}

func TestCostTrackerRecordUsage(t *testing.T) {
	ct := NewCostTracker()

	// Record 3 calls to Sonnet
	ct.RecordUsage("claude-sonnet-4-20250514", 1000, 500, 0, 0)
	ct.RecordUsage("claude-sonnet-4-20250514", 2000, 1000, 0, 0)
	// 1 call to Opus
	ct.RecordUsage("claude-opus-4-20250514", 500, 200, 0, 0)

	total := ct.GetTotalCost()
	if total <= 0 {
		t.Errorf("GetTotalCost() = %.6f, expected > 0", total)
	}

	// Verify per-model breakdown
	costs := ct.GetPerModelCosts()
	if len(costs) != 2 {
		t.Errorf("expected 2 models, got %d", len(costs))
	}

	sonnet := costs["claude-sonnet-4-20250514"]
	if sonnet.InputTokens != 3000 {
		t.Errorf("sonnet input tokens = %d, want 3000", sonnet.InputTokens)
	}
	if sonnet.OutputTokens != 1500 {
		t.Errorf("sonnet output tokens = %d, want 1500", sonnet.OutputTokens)
	}

	opus := costs["claude-opus-4-20250514"]
	if opus.InputTokens != 500 {
		t.Errorf("opus input tokens = %d, want 500", opus.InputTokens)
	}
}

func TestCostTrackerCacheTokens(t *testing.T) {
	ct := NewCostTracker()

	// Record usage with cache tokens
	ct.RecordUsage("claude-sonnet-4-20250514", 1000, 500, 800, 200)

	costs := ct.GetPerModelCosts()
	entry := costs["claude-sonnet-4-20250514"]

	if entry.CacheWriteTokens != 800 {
		t.Errorf("cache write tokens = %d, want 800", entry.CacheWriteTokens)
	}
	if entry.CacheReadTokens != 200 {
		t.Errorf("cache read tokens = %d, want 200", entry.CacheReadTokens)
	}

	// Verify cost includes cache: (1000*3 + 500*15 + 800*3.75 + 200*0.30) / 1M
	expectedCost := (1000*3.0 + 500*15.0 + 800*3.75 + 200*0.30) / 1_000_000.0
	if !almostEqual(ct.GetTotalCost(), expectedCost, 0.000001) {
		t.Errorf("total cost = %.6f, want %.6f", ct.GetTotalCost(), expectedCost)
	}
}

func TestFormatCostDisplay(t *testing.T) {
	ct := NewCostTracker()

	// Empty tracker
	display := ct.FormatCostDisplay()
	if display != "Total: $0.00" {
		t.Errorf("empty display = %q, want %q", display, "Total: $0.00")
	}

	// After some usage
	ct.RecordUsage("claude-sonnet-4-20250514", 1_000_000, 0, 0, 0)
	ct.RecordUsage("claude-opus-4-20250514", 1_000_000, 0, 0, 0)

	display = ct.FormatCostDisplay()
	if len(display) < 10 {
		t.Errorf("display too short: %q", display)
	}
	// Should contain both families
	if !containsAll(display, "Total", "Sonnet", "Opus") {
		t.Errorf("display missing family names: %q", display)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if len(s) > 0 {
			found := false
			for i := 0; i <= len(s)-len(p); i++ {
				if s[i:i+len(p)] == p {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func TestCostTrackerPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cost_tracker.json")

	// Create and populate tracker
	ct1 := NewCostTracker()
	ct1.RecordUsage("claude-sonnet-4-20250514", 5000, 2000, 3000, 1000)
	ct1.RecordUsage("claude-opus-4-20250514", 1000, 500, 0, 0)

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
	if ct1.GetTotalCost() != ct2.GetTotalCost() {
		t.Errorf("total cost mismatch: %.6f vs %.6f", ct1.GetTotalCost(), ct2.GetTotalCost())
	}

	costs1 := ct1.GetPerModelCosts()
	costs2 := ct2.GetPerModelCosts()

	for model, entry1 := range costs1 {
		entry2, ok := costs2[model]
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
		if !almostEqual(entry1.CostUSD, entry2.CostUSD, 0.000001) {
			t.Errorf("%s: cost %.6f vs %.6f", model, entry1.CostUSD, entry2.CostUSD)
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

func TestFamilyName(t *testing.T) {
	tests := []struct {
		model, want string
	}{
		{"claude-sonnet-4-20250514", "Sonnet 4"},
		{"claude-3-5-sonnet-20241022", "Sonnet 3.5"},
		{"claude-opus-4-20250514", "Opus 4"},
		{"claude-opus-4-5-20250610", "Opus 4.5"},
		{"claude-3-5-haiku-20241022", "Haiku 3.5"},
		{"claude-haiku-4-5-20250610", "Haiku 4.5"},
		{"unknown-model", "unknown-model"},
	}

	for _, tt := range tests {
		got := familyName(tt.model)
		if got != tt.want {
			t.Errorf("familyName(%q) = %q, want %q", tt.model, got, tt.want)
		}
	}
}

func TestCostTrackerConcurrentAccess(t *testing.T) {
	ct := NewCostTracker()
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				ct.RecordUsage("claude-sonnet-4-20250514", 100, 50, 0, 0)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// 10 goroutines * 100 calls * (100 input + 50 output) tokens
	sonnet := ct.GetPerModelCosts()["claude-sonnet-4-20250514"]
	if sonnet.InputTokens != 10*100*100 {
		t.Errorf("concurrent input tokens = %d, want %d", sonnet.InputTokens, 10*100*100)
	}
	if sonnet.OutputTokens != 10*100*50 {
		t.Errorf("concurrent output tokens = %d, want %d", sonnet.OutputTokens, 10*100*50)
	}
}

func TestGetPerModelCostsReturnsCopy(t *testing.T) {
	ct := NewCostTracker()
	ct.RecordUsage("claude-sonnet-4-20250514", 1000, 500, 0, 0)

	costs := ct.GetPerModelCosts()
	// Modify the returned map (value in map, so original should be unchanged)
	entry := costs["claude-sonnet-4-20250514"]
	entry.InputTokens = 999999
	costs["claude-sonnet-4-20250514"] = entry

	// Original should be unchanged
	costs2 := ct.GetPerModelCosts()
	if costs2["claude-sonnet-4-20250514"].InputTokens == 999999 {
		t.Error("GetPerModelCosts returned mutable reference, not a copy")
	}
}

func TestCostTrackerSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.json")

	ct1 := NewCostTracker()
	// Mix of models and cache tokens
	ct1.RecordUsage("claude-sonnet-4-20250514", 10_000, 5_000, 8_000, 2_000)
	ct1.RecordUsage("claude-haiku-4-5-20250610", 3_000, 1_000, 0, 0)

	if err := ct1.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	ct2 := NewCostTracker()
	if err := ct2.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if ct2.TotalInputTokens != ct1.TotalInputTokens {
		t.Errorf("total input tokens: %d vs %d", ct2.TotalInputTokens, ct1.TotalInputTokens)
	}
	if ct2.TotalOutputTokens != ct1.TotalOutputTokens {
		t.Errorf("total output tokens: %d vs %d", ct2.TotalOutputTokens, ct1.TotalOutputTokens)
	}
	if len(ct2.GetPerModelCosts()) != len(ct1.GetPerModelCosts()) {
		t.Errorf("model count: %d vs %d", len(ct2.GetPerModelCosts()), len(ct1.GetPerModelCosts()))
	}
}

func TestLookupPrice(t *testing.T) {
	// Known model
	p := lookupPrice("claude-sonnet-4-20250514")
	if p.Input != 3.0 || p.Output != 15.0 {
		t.Errorf("unexpected pricing: %+v", p)
	}

	// Unknown model => zero
	p2 := lookupPrice("gpt-4")
	if p2 != (ModelPrice{}) {
		t.Errorf("unknown model should return zero pricing: %+v", p2)
	}
}

// ─── Upstream Quality: Pricing table completeness invariant ──────────────────

func TestModelPricingNonZeroPrices(t *testing.T) {
	// Every entry in ModelPricing must have non-zero Input and Output prices
	// This catches the bug where a new model is added without pricing
	for model, price := range ModelPricing {
		if price.Input <= 0 {
			t.Errorf("ModelPricing[%q].Input = %.2f, expected > 0", model, price.Input)
		}
		if price.Output <= 0 {
			t.Errorf("ModelPricing[%q].Output = %.2f, expected > 0", model, price.Output)
		}
		if price.CacheWrite <= 0 {
			t.Errorf("ModelPricing[%q].CacheWrite = %.2f, expected > 0", model, price.CacheWrite)
		}
		if price.CacheRead < 0 {
			t.Errorf("ModelPricing[%q].CacheRead = %.2f, expected >= 0", model, price.CacheRead)
		}
	}
}

// ─── Upstream Quality: familyName completeness ───────────────────────────────

func TestFamilyNameCompleteness(t *testing.T) {
	// Every model in ModelPricing should have a readable family name
	for model := range ModelPricing {
		name := familyName(model)
		if name == model {
			t.Logf("ModelPricing[%q] has no human-readable family name", model)
		}
	}
}

// ─── Upstream Quality: Cost calculation edge cases ───────────────────────────

func TestCalculateUSDCostEdgeCases(t *testing.T) {
	tests := []struct {
		name                    string
		model                   string
		input, output           int64
		cacheWrite, cacheRead   int64
		expectedGreaterThanZero bool
	}{
		{"large input", "claude-sonnet-4-20250514", 10_000_000, 0, 0, 0, true},
		{"large output", "claude-sonnet-4-20250514", 0, 10_000_000, 0, 0, true},
		{"all cache tokens", "claude-sonnet-4-20250514", 0, 0, 1_000_000, 1_000_000, true},
		{"mixed tokens", "claude-opus-4-5-20250610", 100_000, 50_000, 80_000, 20_000, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := CalculateUSDCost(tt.model, tt.input, tt.output, tt.cacheWrite, tt.cacheRead)
			if tt.expectedGreaterThanZero && cost <= 0 {
				t.Errorf("expected cost > 0, got %.6f", cost)
			}
		})
	}
}

// ─── Upstream Quality: Cost tracker token count accuracy ─────────────────────

func TestCostTrackerTokenCountAccuracy(t *testing.T) {
	ct := NewCostTracker()
	ct.RecordUsage("claude-sonnet-4-20250514", 1_000, 500, 800, 200)
	ct.RecordUsage("claude-sonnet-4-20250514", 2_000, 1_000, 0, 0)

	if ct.TotalInputTokens != 3_000 {
		t.Errorf("TotalInputTokens = %d, want 3000", ct.TotalInputTokens)
	}
	if ct.TotalOutputTokens != 1_500 {
		t.Errorf("TotalOutputTokens = %d, want 1500", ct.TotalOutputTokens)
	}

	entry := ct.GetPerModelCosts()["claude-sonnet-4-20250514"]
	if entry.CacheWriteTokens != 800 {
		t.Errorf("CacheWriteTokens = %d, want 800", entry.CacheWriteTokens)
	}
	if entry.CacheReadTokens != 200 {
		t.Errorf("CacheReadTokens = %d, want 200", entry.CacheReadTokens)
	}
}
