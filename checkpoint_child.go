package main

import (
	"fmt"
	"sync"
)

// ─── Checkpoint Writer Child Session (MiMo-Code 7) ──────────────────────────
//
// Runs the checkpoint writer in a fresh child session for isolation.
// Prevents writer messages from polluting parent session.
//
// MiMo-Code source: session/checkpoint.ts (784-908 lines)

// CheckpointChildConfig holds configuration for child session checkpoint writing.
type CheckpointChildConfig struct {
	Enabled       bool `json:"enabled"`
	MaxFailures   int  `json:"max_failures"`
	SettleTimeout int  `json:"settle_timeout_ms"`
}

// CheckpointChildSession manages checkpoint writing in an isolated child session.
type CheckpointChildSession struct {
	mu              sync.Mutex
	config          CheckpointChildConfig
	parentSessionID string
	writer          *CheckpointWriter
	pending         *CheckpointRequest
	lastCheckpointID string
	consecutiveFails int
}

// NewCheckpointChildSession creates a new child session checkpoint writer.
func NewCheckpointChildSession(parentSessionID string, writer *CheckpointWriter) *CheckpointChildSession {
	return &CheckpointChildSession{
		config: CheckpointChildConfig{
			Enabled:       true,
			MaxFailures:   3,
			SettleTimeout: 5000,
		},
		parentSessionID: parentSessionID,
		writer:          writer,
	}
}

// Submit submits a checkpoint write request.
// If a writer is already running, the request is queued (newest wins).
func (s *CheckpointChildSession) Submit(req CheckpointRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.config.Enabled {
		return
	}

	// Check failure limit
	if s.consecutiveFails >= s.config.MaxFailures {
		return
	}

	// Queue request (newest wins)
	s.pending = &req

	// Start writer if not running
	if s.writer != nil && !s.writer.IsRunning() {
		go s.runWriter()
	}
}

// runWriter runs the checkpoint writer.
func (s *CheckpointChildSession) runWriter() {
	s.mu.Lock()
	req := s.pending
	s.pending = nil
	s.mu.Unlock()

	if req == nil {
		return
	}

	// Run writer
	result := s.writer.writeCheckpoint(*req)

	s.mu.Lock()
	if result.Error != nil {
		s.consecutiveFails++
	} else {
		s.consecutiveFails = 0
		s.lastCheckpointID = result.CheckpointID
	}
	s.mu.Unlock()
}

// GetLastCheckpointID returns the last checkpoint ID.
func (s *CheckpointChildSession) GetLastCheckpointID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastCheckpointID
}

// IsEnabled returns true if the child session is enabled.
func (s *CheckpointChildSession) IsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config.Enabled
}

// SetEnabled enables or disables the child session.
func (s *CheckpointChildSession) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.Enabled = enabled
}

// FormatCheckpointChildStatus formats the status for display.
func FormatCheckpointChildStatus(session *CheckpointChildSession) string {
	if session == nil {
		return "No checkpoint child session."
	}

	lastID := session.GetLastCheckpointID()
	if lastID == "" {
		return "No checkpoint written yet."
	}

	return fmt.Sprintf("Last checkpoint: %s", lastID)
}
