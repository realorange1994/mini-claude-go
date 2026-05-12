package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// ModelPrice holds per-million-token pricing for a model (USD).
type ModelPrice struct {
	Input      float64 // per million input tokens
	Output     float64 // per million output tokens
	CacheWrite float64 // per million cache write tokens
	CacheRead  float64 // per million cache read tokens
}

// ModelCostEntry holds accumulated token counts and USD cost for a single model.
type ModelCostEntry struct {
	InputTokens    int64
	OutputTokens   int64
	CacheWriteTokens int64
	CacheReadTokens  int64
	CostUSD        float64
}

// CostTracker tracks per-model USD cost across a session.
type CostTracker struct {
	mu               sync.Mutex
	TotalCost        float64
	PerModel         map[string]ModelCostEntry // model ID -> cost breakdown
	TotalInputTokens  int64
	TotalOutputTokens int64
}

// ModelPricing is the pricing table (USD per million tokens) for supported Claude models.
var ModelPricing = map[string]ModelPrice{
	// Sonnet family
	"claude-sonnet-4-20250514":   {Input: 3.0, Output: 15.0, CacheWrite: 3.75, CacheRead: 0.30},
	"claude-3-5-sonnet-20241022": {Input: 3.0, Output: 15.0, CacheWrite: 3.75, CacheRead: 0.30},
	// Opus 4/4.1 family
	"claude-opus-4-20250514":     {Input: 15.0, Output: 75.0, CacheWrite: 18.75, CacheRead: 1.50},
	// Opus 4.5+ family
	"claude-opus-4-5-20250610":   {Input: 5.0, Output: 25.0, CacheWrite: 6.25, CacheRead: 0.50},
	// Haiku 3.5
	"claude-3-5-haiku-20241022":  {Input: 0.80, Output: 4.0, CacheWrite: 1.0, CacheRead: 0.08},
	// Haiku 4.5+
	"claude-haiku-4-5-20250610":  {Input: 1.0, Output: 5.0, CacheWrite: 1.25, CacheRead: 0.10},
}

// familyName returns a human-readable family label for a model ID.
func familyName(model string) string {
	model = strings.ToLower(model)
	switch {
	case strings.Contains(model, "opus-4-5"):
		return "Opus 4.5"
	case strings.Contains(model, "opus-4"):
		return "Opus 4"
	case strings.Contains(model, "sonnet-4"):
		return "Sonnet 4"
	case strings.Contains(model, "3-5-sonnet"):
		return "Sonnet 3.5"
	case strings.Contains(model, "haiku-4-5"):
		return "Haiku 4.5"
	case strings.Contains(model, "3-5-haiku"):
		return "Haiku 3.5"
	default:
		return model
	}
}

// lookupPrice returns the pricing for a model, or a zero struct if unknown.
func lookupPrice(model string) ModelPrice {
	if p, ok := ModelPricing[model]; ok {
		return p
	}
	return ModelPrice{}
}

// NewCostTracker creates a fresh CostTracker.
func NewCostTracker() *CostTracker {
	return &CostTracker{
		PerModel: make(map[string]ModelCostEntry),
	}
}

// CalculateUSDCost computes the USD cost for a single API call.
// Prices are per million tokens.
func CalculateUSDCost(model string, inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens int64) float64 {
	p := lookupPrice(model)
	cost := float64(inputTokens)/1_000_000.0*p.Input +
		float64(outputTokens)/1_000_000.0*p.Output +
		float64(cacheWriteTokens)/1_000_000.0*p.CacheWrite +
		float64(cacheReadTokens)/1_000_000.0*p.CacheRead
	return cost
}

// RecordUsage records usage for a single API call and accumulates totals.
func (ct *CostTracker) RecordUsage(model string, inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens int64) {
	cost := CalculateUSDCost(model, inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens)

	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.TotalCost += cost
	ct.TotalInputTokens += inputTokens
	ct.TotalOutputTokens += outputTokens

	entry := ct.PerModel[model]
	entry.InputTokens += inputTokens
	entry.OutputTokens += outputTokens
	entry.CacheWriteTokens += cacheWriteTokens
	entry.CacheReadTokens += cacheReadTokens
	entry.CostUSD += cost
	ct.PerModel[model] = entry
}

// GetTotalCost returns the total USD cost accumulated this session.
func (ct *CostTracker) GetTotalCost() float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.TotalCost
}

// GetPerModelCosts returns a copy of the per-model cost breakdown.
func (ct *CostTracker) GetPerModelCosts() map[string]ModelCostEntry {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	out := make(map[string]ModelCostEntry, len(ct.PerModel))
	for k, v := range ct.PerModel {
		out[k] = v
	}
	return out
}

// FormatCostDisplay returns a human-readable summary string.
// Example: "Total: $0.05 | Sonnet 4: $0.03, Opus 4: $0.02"
func (ct *CostTracker) FormatCostDisplay() string {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if len(ct.PerModel) == 0 {
		return "Total: $0.00"
	}

	// Aggregate by family
	families := make(map[string]float64)
	for model, entry := range ct.PerModel {
		fam := familyName(model)
		families[fam] += entry.CostUSD
	}

	// Build family breakdown string
	var parts []string
	for fam, cost := range families {
		parts = append(parts, fmt.Sprintf("%s: $%.2f", fam, cost))
	}

	return fmt.Sprintf("Total: $%.2f | %s", ct.TotalCost, strings.Join(parts, ", "))
}

// costTrackerFile is the on-disk representation.
type costTrackerFile struct {
	TotalCost        float64
	PerModel         map[string]ModelCostEntry
	TotalInputTokens  int64
	TotalOutputTokens int64
}

// SaveToFile persists the cost tracker state to a JSON file.
func (ct *CostTracker) SaveToFile(path string) error {
	ct.mu.Lock()
	data := costTrackerFile{
		TotalCost:        ct.TotalCost,
		PerModel:         make(map[string]ModelCostEntry),
		TotalInputTokens:  ct.TotalInputTokens,
		TotalOutputTokens: ct.TotalOutputTokens,
	}
	for k, v := range ct.PerModel {
		data.PerModel[k] = v
	}
	ct.mu.Unlock()

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("cost_tracker: marshal: %w", err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		return fmt.Errorf("cost_tracker: write %s: %w", path, err)
	}
	return nil
}

// LoadFromFile restores the cost tracker state from a JSON file.
func (ct *CostTracker) LoadFromFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no existing file, start fresh
		}
		return fmt.Errorf("cost_tracker: read %s: %w", path, err)
	}

	var data costTrackerFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("cost_tracker: unmarshal %s: %w", path, err)
	}

	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.TotalCost = data.TotalCost
	ct.PerModel = data.PerModel
	ct.TotalInputTokens = data.TotalInputTokens
	ct.TotalOutputTokens = data.TotalOutputTokens
	return nil
}
