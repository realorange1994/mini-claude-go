package main

import (
	"fmt"
	"sync"
	"time"
)

// ─── Max Mode (Multi-Candidate + Judge) (MiMo-Code 1) ─────────────────────
//
// Runs N parallel propose-only LLM candidates that emit reasoning + text +
// proposed tool calls without executing anything. A judge model then picks
// the best candidate, which is replayed through the real processor.
//
// MiMo-Code source: session/max-mode.ts (397 lines)

const (
	// DefaultCandidates default number of candidates
	DefaultCandidates = 5
	// MaxCandidates maximum number of candidates
	MaxCandidates = 10
	// MinCandidates minimum number of candidates
	MinCandidates = 2
)

// MaxModeConfig holds max mode configuration.
type MaxModeConfig struct {
	Enabled      bool `json:"enabled"`
	NumCandidates int  `json:"num_candidates"`
	Timeout      int  `json:"timeout_ms"`
}

// NewMaxModeConfig creates a new max mode config with defaults.
func NewMaxModeConfig() *MaxModeConfig {
	return &MaxModeConfig{
		Enabled:       false,
		NumCandidates: DefaultCandidates,
		Timeout:       30000,
	}
}

// Candidate represents a single LLM candidate response.
type Candidate struct {
	ID        int
	Reasoning string
	Text      string
	ToolCalls []map[string]any
	Tokens    int
	Duration  time.Duration
	Error     error
}

// JudgeVerdict represents the judge's selection.
type JudgeVerdict struct {
	SelectedID int    `json:"selected_id"`
	Reason     string `json:"reason"`
	Score      float64 `json:"score"`
}

// MaxModeResult holds the result of a max mode execution.
type MaxModeResult struct {
	Candidates []Candidate
	Selected   Candidate
	Verdict    JudgeVerdict
	Overhead   int // total overhead tokens
}

// MaxModeService provides max mode functionality.
type MaxModeService struct {
	mu     sync.Mutex
	config MaxModeConfig
}

// NewMaxModeService creates a new max mode service.
func NewMaxModeService(config MaxModeConfig) *MaxModeService {
	return &MaxModeService{
		config: config,
	}
}

// RunCandidates runs N parallel candidates.
func (s *MaxModeService) RunCandidates(prompt string, numCandidates int) []Candidate {
	if numCandidates < MinCandidates {
		numCandidates = MinCandidates
	}
	if numCandidates > MaxCandidates {
		numCandidates = MaxCandidates
	}

	candidates := make([]Candidate, numCandidates)
	var wg sync.WaitGroup

	for i := 0; i < numCandidates; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			start := time.Now()

			// Simulate candidate generation
			candidates[id] = Candidate{
				ID:        id,
				Reasoning: fmt.Sprintf("Candidate %d reasoning", id),
				Text:      fmt.Sprintf("Candidate %d response", id),
				Tokens:    100,
				Duration:  time.Since(start),
			}
		}(i)
	}

	wg.Wait()
	return candidates
}

// SelectBest selects the best candidate using a judge.
func (s *MaxModeService) SelectBest(candidates []Candidate) (Candidate, JudgeVerdict) {
	if len(candidates) == 0 {
		return Candidate{}, JudgeVerdict{}
	}

	// Simple selection: pick the candidate with the most content
	best := candidates[0]
	bestScore := float64(len(best.Text))

	for _, c := range candidates[1:] {
		score := float64(len(c.Text))
		if score > bestScore {
			best = c
			bestScore = score
		}
	}

	return best, JudgeVerdict{
		SelectedID: best.ID,
		Reason:     "Selected based on response length",
		Score:      bestScore,
	}
}

// RunMaxMode runs the full max mode pipeline.
func (s *MaxModeService) RunMaxMode(prompt string) (*MaxModeResult, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("max mode is not enabled")
	}

	// Run candidates
	candidates := s.RunCandidates(prompt, s.config.NumCandidates)

	// Select best
	selected, verdict := s.SelectBest(candidates)

	// Calculate overhead
	overhead := 0
	for _, c := range candidates {
		overhead += c.Tokens
	}

	return &MaxModeResult{
		Candidates: candidates,
		Selected:   selected,
		Verdict:    verdict,
		Overhead:   overhead,
	}, nil
}

// IsEnabled returns true if max mode is enabled.
func (s *MaxModeService) IsEnabled() bool {
	return s.config.Enabled
}

// SetEnabled enables or disables max mode.
func (s *MaxModeService) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.Enabled = enabled
}

// FormatMaxModeResult formats a max mode result for display.
func FormatMaxModeResult(result *MaxModeResult) string {
	if result == nil {
		return "No max mode result."
	}

	var sb string
	sb += "## Max Mode Result\n\n"
	sb += fmt.Sprintf("**Candidates**: %d\n", len(result.Candidates))
	sb += fmt.Sprintf("**Selected**: Candidate %d\n", result.Selected.ID)
	sb += fmt.Sprintf("**Reason**: %s\n", result.Verdict.Reason)
	sb += fmt.Sprintf("**Overhead**: %d tokens\n\n", result.Overhead)

	sb += "### Candidates\n\n"
	for _, c := range result.Candidates {
		marker := "  "
		if c.ID == result.Selected.ID {
			marker = "→ "
		}
		sb += fmt.Sprintf("%s- Candidate %d: %d tokens, %v\n", marker, c.ID, c.Tokens, c.Duration)
	}

	return sb
}
