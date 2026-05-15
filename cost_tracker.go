package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// CostTracker tracks token usage across models.
// Instead of computing dollar amounts (which vary by provider and pricing tier),
// it tracks raw token counts — a more stable and universally meaningful metric.
type CostTracker struct {
	mu               sync.Mutex
	TotalInputTokens  int64
	TotalOutputTokens int64
	PerModel          map[string]*ModelUsage
}

// ModelUsage tracks per-model token consumption.
type ModelUsage struct {
	InputTokens  int64
	OutputTokens int64
}

func NewCostTracker() *CostTracker {
	return &CostTracker{
		PerModel: make(map[string]*ModelUsage),
	}
}

// AddUsage records token usage from an API response.
func (ct *CostTracker) AddUsage(model string, inputTokens, outputTokens int64) {
	if inputTokens == 0 && outputTokens == 0 {
		return
	}
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.TotalInputTokens += inputTokens
	ct.TotalOutputTokens += outputTokens

	entry, ok := ct.PerModel[model]
	if !ok {
		entry = &ModelUsage{}
		ct.PerModel[model] = entry
	}
	entry.InputTokens += inputTokens
	entry.OutputTokens += outputTokens
}

// FormatCostDisplay returns a human-readable summary string of token usage.
// Example: "Total: 150k tokens (100k in, 50k out) | Sonnet 4: 80k in, 30k out, m2.7: 20k in, 20k out"
func (ct *CostTracker) FormatCostDisplay() string {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if len(ct.PerModel) == 0 {
		return "Total: 0 tokens"
	}

	// Aggregate by family
	type famTokens struct {
		input  int64
		output int64
	}
	families := make(map[string]famTokens)
	for model, entry := range ct.PerModel {
		fam := familyName(model)
		f := families[fam]
		f.input += entry.InputTokens
		f.output += entry.OutputTokens
		families[fam] = f
	}

	totalTokens := ct.TotalInputTokens + ct.TotalOutputTokens

	// Build family breakdown string
	var parts []string
	for fam, tokens := range families {
		parts = append(parts, fmt.Sprintf("%s: %s in, %s out", fam, formatTokenCount(tokens.input), formatTokenCount(tokens.output)))
	}

	return fmt.Sprintf("Total: %s tokens (%s in, %s out) | %s",
		formatTokenCount(totalTokens),
		formatTokenCount(ct.TotalInputTokens),
		formatTokenCount(ct.TotalOutputTokens),
		strings.Join(parts, ", "))
}

// formatTokenCount renders token counts with k/M suffixes for readability.
func formatTokenCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// familyName maps a full model ID to a short display name.
func familyName(model string) string {
	// Handle common model ID patterns
	switch {
	case strings.Contains(model, "opus"):
		return "Opus"
	case strings.Contains(model, "sonnet"):
		return "Sonnet"
	case strings.Contains(model, "haiku"):
		return "Haiku"
	case strings.Contains(model, "m2.7") || strings.Contains(model, "M2.7"):
		return "m2.7"
	case strings.Contains(model, "m2.5") || strings.Contains(model, "M2.5"):
		return "m2.5"
	case strings.Contains(model, "m2.1") || strings.Contains(model, "M2.1"):
		return "m2.1"
	case strings.Contains(model, "deepseek"):
		return "DeepSeek"
	case strings.Contains(model, "kimi"):
		return "Kimi"
	case strings.Contains(model, "glm"):
		return "GLM"
	case strings.Contains(model, "qwen"):
		return "Qwen"
	case strings.Contains(model, "doubao"):
		return "Doubao"
	default:
		// Trim provider prefix and version suffix for unknown models
		parts := strings.Split(model, "-")
		if len(parts) > 2 {
			return strings.Join(parts[:2], "-")
		}
		return model
	}
}

// GetPerModelUsage returns a copy of the per-model usage breakdown.
func (ct *CostTracker) GetPerModelUsage() map[string]ModelUsage {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	out := make(map[string]ModelUsage, len(ct.PerModel))
	for k, v := range ct.PerModel {
		if v != nil {
			out[k] = *v
		}
	}
	return out
}

// costTrackerFile is the on-disk representation.
type costTrackerFile struct {
	TotalInputTokens  int64
	TotalOutputTokens int64
	PerModel          map[string]ModelUsage
}

// SaveToFile persists the cost tracker state to a JSON file.
func (ct *CostTracker) SaveToFile(path string) error {
	ct.mu.Lock()
	data := costTrackerFile{
		TotalInputTokens:  ct.TotalInputTokens,
		TotalOutputTokens: ct.TotalOutputTokens,
		PerModel:          make(map[string]ModelUsage, len(ct.PerModel)),
	}
	for k, v := range ct.PerModel {
		if v != nil {
			data.PerModel[k] = *v
		}
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
	ct.TotalInputTokens = data.TotalInputTokens
	ct.TotalOutputTokens = data.TotalOutputTokens
	for k, v := range data.PerModel {
		v := v // copy to avoid loop variable capture
		ct.PerModel[k] = &ModelUsage{
			InputTokens:  v.InputTokens,
			OutputTokens: v.OutputTokens,
		}
	}
	return nil
}
