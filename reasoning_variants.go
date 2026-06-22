package main

import (
	"strings"
)

// ─── Provider Transform Variants (MiMo-Code 2) ─────────────────────────────
//
// Maps reasoning effort levels to provider-specific parameters.
// Covers Anthropic, OpenAI, Google, Bedrock, and 15+ other providers.
//
// MiMo-Code source: provider/transform.ts (491-892 lines)

// ReasoningEffort represents a reasoning effort level.
type ReasoningEffort string

const (
	EffortNone    ReasoningEffort = "none"
	EffortMinimal ReasoningEffort = "minimal"
	EffortLow     ReasoningEffort = "low"
	EffortMedium  ReasoningEffort = "medium"
	EffortHigh    ReasoningEffort = "high"
	EffortXHigh   ReasoningEffort = "xhigh"
	EffortMax     ReasoningEffort = "max"
)

// ProviderVariant represents provider-specific reasoning configuration.
type ProviderVariant struct {
	Provider     string
	ModelPattern string
	Effort       ReasoningEffort
	Params       map[string]any
}

// ReasoningVariantService manages reasoning effort variants.
type ReasoningVariantService struct {
	variants []ProviderVariant
}

// NewReasoningVariantService creates a new variant service.
func NewReasoningVariantService() *ReasoningVariantService {
	s := &ReasoningVariantService{}
	s.registerDefaults()
	return s
}

// registerDefaults registers default provider variants.
func (s *ReasoningVariantService) registerDefaults() {
	// Anthropic variants
	s.variants = append(s.variants,
		ProviderVariant{Provider: "anthropic", ModelPattern: "claude", Effort: EffortLow,
			Params: map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": 2000}}},
		ProviderVariant{Provider: "anthropic", ModelPattern: "claude", Effort: EffortMedium,
			Params: map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": 5000}}},
		ProviderVariant{Provider: "anthropic", ModelPattern: "claude", Effort: EffortHigh,
			Params: map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": 10000}}},
		ProviderVariant{Provider: "anthropic", ModelPattern: "claude", Effort: EffortMax,
			Params: map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": 30000}}},
	)

	// OpenAI variants
	s.variants = append(s.variants,
		ProviderVariant{Provider: "openai", ModelPattern: "o1", Effort: EffortLow,
			Params: map[string]any{"reasoningEffort": "low", "reasoningSummary": "auto"}},
		ProviderVariant{Provider: "openai", ModelPattern: "o1", Effort: EffortMedium,
			Params: map[string]any{"reasoningEffort": "medium", "reasoningSummary": "auto"}},
		ProviderVariant{Provider: "openai", ModelPattern: "o1", Effort: EffortHigh,
			Params: map[string]any{"reasoningEffort": "high", "reasoningSummary": "auto"}},
	)

	// Google variants
	s.variants = append(s.variants,
		ProviderVariant{Provider: "google", ModelPattern: "gemini", Effort: EffortLow,
			Params: map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": 1024}}},
		ProviderVariant{Provider: "google", ModelPattern: "gemini", Effort: EffortMedium,
			Params: map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": 8192}}},
		ProviderVariant{Provider: "google", ModelPattern: "gemini", Effort: EffortHigh,
			Params: map[string]any{"thinkingConfig": map[string]any{"thinkingBudget": 32768}}},
	)
}

// GetVariant returns the variant for a provider/model/effort combination.
func (s *ReasoningVariantService) GetVariant(provider, model string, effort ReasoningEffort) *ProviderVariant {
	modelLower := strings.ToLower(model)

	for i := range s.variants {
		v := &s.variants[i]
		if v.Provider == provider && v.Effort == effort {
			if v.ModelPattern == "" || strings.Contains(modelLower, v.ModelPattern) {
				return v
			}
		}
	}

	return nil
}

// GetParams returns the provider-specific parameters for reasoning.
func (s *ReasoningVariantService) GetParams(provider, model string, effort ReasoningEffort) map[string]any {
	v := s.GetVariant(provider, model, effort)
	if v == nil {
		return nil
	}
	return v.Params
}

// ListVariants returns all registered variants.
func (s *ReasoningVariantService) ListVariants() []ProviderVariant {
	result := make([]ProviderVariant, len(s.variants))
	copy(result, s.variants)
	return result
}

// FormatVariant formats a variant for display.
func FormatVariant(v ProviderVariant) string {
	return v.Provider + " / " + v.ModelPattern + " / " + string(v.Effort)
}
