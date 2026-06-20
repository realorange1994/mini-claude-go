package main

// ─── Step Classification (MiMo-Code 3C) ─────────────────────────────────────
//
// Classifies every assistant step into one of 6 categories for loop control.
// Centralizes scattered if/else logic into a single pure function.
//
// MiMo-Code source: session/classify.ts (92 lines)

// StepCategory represents the classification of an assistant step.
type StepCategory string

const (
	StepFinal      StepCategory = "final"      // completed normally
	StepContinue   StepCategory = "continue"   // has pending tool calls
	StepFiltered   StepCategory = "filtered"   // content-filter
	StepThinkOnly  StepCategory = "think-only" // reasoning without text
	StepInvalid    StepCategory = "invalid"     // empty output
	StepFailed     StepCategory = "failed"      // error
)

// StepClassification holds the result of classifying an assistant step.
type StepClassification struct {
	Category    StepCategory
	Degraded    bool   // true if final but with issues
	HasText     bool   // has text content
	HasTools    bool   // has tool calls
	HasThinking bool   // has thinking/reasoning
	ErrorMsg    string // error message if failed
}

// ClassifyAssistantStep classifies an assistant step into a category.
// This is a pure, stateless function that centralizes loop control logic.
func ClassifyAssistantStep(text string, toolCalls []map[string]any, thinking string, err error) StepClassification {
	hasText := len(text) > 0
	hasTools := len(toolCalls) > 0
	hasThinking := len(thinking) > 0

	// Error check first
	if err != nil {
		return StepClassification{
			Category:    StepFailed,
			HasText:     hasText,
			HasTools:    hasTools,
			HasThinking: hasThinking,
			ErrorMsg:    err.Error(),
		}
	}

	// Has pending tool calls → continue
	if hasTools {
		return StepClassification{
			Category:    StepContinue,
			HasText:     hasText,
			HasTools:    true,
			HasThinking: hasThinking,
		}
	}

	// No text and no tools → check thinking
	if !hasText {
		if hasThinking {
			return StepClassification{
				Category:    StepThinkOnly,
				HasThinking: true,
			}
		}
		return StepClassification{
			Category: StepInvalid,
		}
	}

	// Has text, no tools → final answer
	return StepClassification{
		Category:    StepFinal,
		HasText:     true,
		HasTools:    false,
		HasThinking: hasThinking,
	}
}

// ShouldContinue returns true if the loop should continue based on classification.
func ShouldContinue(classification StepClassification) bool {
	switch classification.Category {
	case StepContinue, StepThinkOnly:
		return true
	default:
		return false
	}
}
