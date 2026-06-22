package main

import (
	"sync"
	"time"
)

// ─── Session Prune Service (MiMo-Code 4) ───────────────────────────────────
//
// Sophisticated context-window management with threshold tracking,
// cache-cold detection, and progressive pruning.
//
// MiMo-Code source: session/prune.ts (481 lines)

// PruneConfig holds pruning configuration.
type PruneConfig struct {
	Enabled         bool    `json:"enabled"`
	Thresholds      []int   `json:"thresholds"`       // token thresholds
	SoftTrimSize    int     `json:"soft_trim_size"`    // chars to keep in soft trim
	HardPruneAge    int     `json:"hard_prune_age"`    // turns to keep in hard prune
	CacheTTL        int     `json:"cache_ttl_ms"`      // cache TTL in milliseconds
}

// ThresholdState tracks which thresholds have been crossed.
type ThresholdState struct {
	Crossed   map[int]bool
	Failures  int
	MaxFails  int
}

// SessionPruneService manages context pruning.
type SessionPruneService struct {
	mu          sync.Mutex
	config      PruneConfig
	thresholds  map[string]*ThresholdState // sessionID -> state
	lastActive  map[string]time.Time       // sessionID -> last activity
}

// NewSessionPruneService creates a new prune service.
func NewSessionPruneService(config PruneConfig) *SessionPruneService {
	if config.SoftTrimSize <= 0 {
		config.SoftTrimSize = 1536
	}
	if config.HardPruneAge <= 0 {
		config.HardPruneAge = 3
	}
	if config.CacheTTL <= 0 {
		config.CacheTTL = 300000 // 5 minutes
	}

	return &SessionPruneService{
		config:     config,
		thresholds: make(map[string]*ThresholdState),
		lastActive: make(map[string]time.Time),
	}
}

// ShouldPrune checks if pruning should occur based on current token count.
func (s *SessionPruneService) ShouldPrune(sessionID string, currentTokens int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.config.Enabled {
		return false
	}

	// Check if any threshold is crossed
	for _, threshold := range s.config.Thresholds {
		if currentTokens >= threshold {
			state := s.getOrCreateState(sessionID)
			if !state.Crossed[threshold] {
				state.Crossed[threshold] = true
				return true
			}
		}
	}

	return false
}

// IsCacheCold checks if the cache has expired.
func (s *SessionPruneService) IsCacheCold(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	lastActive, exists := s.lastActive[sessionID]
	if !exists {
		return true
	}

	return time.Since(lastActive).Milliseconds() > int64(s.config.CacheTTL)
}

// RecordActivity records activity for a session.
func (s *SessionPruneService) RecordActivity(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActive[sessionID] = time.Now()
}

// GetPruneLevel returns the pruning level (0=none, 1=soft, 2=hard).
func (s *SessionPruneService) GetPruneLevel(sessionID string, currentTokens int) int {
	if !s.config.Enabled {
		return 0
	}

	// Check thresholds
	for _, threshold := range s.config.Thresholds {
		if currentTokens >= threshold {
			// Hard prune at 80%+ of max threshold
			maxThreshold := s.config.Thresholds[len(s.config.Thresholds)-1]
			if currentTokens >= maxThreshold*80/100 {
				return 2
			}
			return 1
		}
	}

	return 0
}

// Reset resets the threshold state for a session.
func (s *SessionPruneService) Reset(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.thresholds, sessionID)
}

// getOrCreateState gets or creates threshold state for a session.
func (s *SessionPruneService) getOrCreateState(sessionID string) *ThresholdState {
	state, exists := s.thresholds[sessionID]
	if !exists {
		state = &ThresholdState{
			Crossed:  make(map[int]bool),
			MaxFails: 3,
		}
		s.thresholds[sessionID] = state
	}
	return state
}

// FormatPruneStatus formats prune status for display.
func FormatPruneStatus(service *SessionPruneService, sessionID string) string {
	if service == nil {
		return "No prune service."
	}

	level := service.GetPruneLevel(sessionID, 0)
	return "Prune level: " + itoaPrune(level)
}

// itoaPrune converts int to string.
func itoaPrune(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
