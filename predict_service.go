package main

import (
	"strings"
	"sync"
)

// ─── Predict Next Prompt (MiMo-Code 4) ─────────────────────────────────────
//
// Predicts the user's most likely next message based on conversation context.
// Used for tab-completion or pre-filling the next prompt.
//
// MiMo-Code source: session/prompt.ts (392-454 lines)

const (
	MaxPredictLength = 120
)

// PredictService predicts the user's next message.
type PredictService struct {
	mu        sync.Mutex
	enabled   bool
	lastInput string
	lastOutput string
	prediction string
}

// NewPredictService creates a new predict service.
func NewPredictService(enabled bool) *PredictService {
	return &PredictService{
		enabled: enabled,
	}
}

// Predict generates a prediction for the next user message.
func (s *PredictService) Predict(userMessage, assistantResponse string) string {
	if !s.enabled {
		return ""
	}

	s.mu.Lock()
	s.lastInput = userMessage
	s.lastOutput = assistantResponse
	s.mu.Unlock()

	// Simple heuristic-based prediction
	prediction := s.generatePrediction(userMessage, assistantResponse)

	s.mu.Lock()
	s.prediction = prediction
	s.mu.Unlock()

	return prediction
}

// generatePrediction generates a prediction based on context.
func (s *PredictService) generatePrediction(userMessage, assistantResponse string) string {
	// Pattern-based predictions
	lower := strings.ToLower(userMessage)

	// If user asked a question, predict a follow-up
	if strings.Contains(lower, "what") || strings.Contains(lower, "how") || strings.Contains(lower, "why") {
		return "Can you explain more about this?"
	}

	// If user asked to create something, predict a modification
	if strings.Contains(lower, "create") || strings.Contains(lower, "make") || strings.Contains(lower, "build") {
		return "Now add error handling"
	}

	// If user asked to fix something, predict a verification
	if strings.Contains(lower, "fix") || strings.Contains(lower, "debug") || strings.Contains(lower, "error") {
		return "Run the tests to verify"
	}

	// Default prediction
	return "Continue"
}

// GetLastPrediction returns the last prediction.
func (s *PredictService) GetLastPrediction() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prediction
}

// SetEnabled enables or disables the predict service.
func (s *PredictService) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
}

// IsEnabled returns true if the predict service is enabled.
func (s *PredictService) IsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}
