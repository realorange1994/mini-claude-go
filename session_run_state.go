package main

import (
	"fmt"
	"sync"
	"time"
)

// ─── Session Run State Machine (MiMo-Code 3) ───────────────────────────────
//
// Fiber-based state machine managing concurrent session execution.
//
// MiMo-Code source: effect/runner.ts (210 lines), session/run-state.ts (135 lines)

// RunState represents the state of a session run.
type RunState string

const (
	RunStateIdle         RunState = "idle"
	RunStateRunning      RunState = "running"
	RunStateShell        RunState = "shell"
	RunStateShellThenRun RunState = "shell_then_run"
)

// RunStateEvent represents a state transition event.
type RunStateEvent struct {
	From      RunState
	To        RunState
	Timestamp int64
}

// SessionRunManager manages session run states.
type SessionRunManager struct {
	mu       sync.Mutex
	sessions map[string]*SessionRun
}

// SessionRun represents the run state for a session.
type SessionRun struct {
	SessionID string
	State     RunState
	History   []RunStateEvent
}

// NewSessionRunManager creates a new run manager.
func NewSessionRunManager() *SessionRunManager {
	return &SessionRunManager{
		sessions: make(map[string]*SessionRun),
	}
}

// GetState returns the current state for a session.
func (m *SessionRunManager) GetState(sessionID string) RunState {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, exists := m.sessions[sessionID]
	if !exists {
		return RunStateIdle
	}
	return run.State
}

// TransitionTo transitions a session to a new state.
func (m *SessionRunManager) TransitionTo(sessionID string, newState RunState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, exists := m.sessions[sessionID]
	if !exists {
		run = &SessionRun{
			SessionID: sessionID,
			State:     RunStateIdle,
		}
		m.sessions[sessionID] = run
	}

	// Validate transition
	if !isValidTransition(run.State, newState) {
		return fmt.Errorf("invalid transition: %s -> %s", run.State, newState)
	}

	event := RunStateEvent{
		From:      run.State,
		To:        newState,
		Timestamp: timeNowUnix(),
	}

	run.History = append(run.History, event)
	run.State = newState

	return nil
}

// EnsureRunning ensures a session is running, queuing work if needed.
func (m *SessionRunManager) EnsureRunning(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, exists := m.sessions[sessionID]
	if !exists {
		run = &SessionRun{
			SessionID: sessionID,
			State:     RunStateIdle,
		}
		m.sessions[sessionID] = run
	}

	switch run.State {
	case RunStateIdle:
		run.State = RunStateRunning
		return nil
	case RunStateRunning:
		return nil // Already running
	case RunStateShell:
		run.State = RunStateShellThenRun
		return nil
	default:
		return fmt.Errorf("cannot ensure running from state: %s", run.State)
	}
}

// Cancel cancels a running session.
func (m *SessionRunManager) Cancel(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	run.State = RunStateIdle
	return nil
}

// GetHistory returns the state history for a session.
func (m *SessionRunManager) GetHistory(sessionID string) []RunStateEvent {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, exists := m.sessions[sessionID]
	if !exists {
		return nil
	}

	result := make([]RunStateEvent, len(run.History))
	copy(result, run.History)
	return result
}

// isValidTransition checks if a state transition is valid.
func isValidTransition(from, to RunState) bool {
	transitions := map[RunState][]RunState{
		RunStateIdle:         {RunStateRunning, RunStateShell},
		RunStateRunning:      {RunStateIdle, RunStateShell},
		RunStateShell:        {RunStateIdle, RunStateRunning, RunStateShellThenRun},
		RunStateShellThenRun: {RunStateRunning, RunStateShell},
	}

	allowed, exists := transitions[from]
	if !exists {
		return false
	}

	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// timeNowUnix returns current Unix timestamp.
func timeNowUnix() int64 {
	return time.Now().Unix()
}

// FormatRunState formats a run state for display.
func FormatRunState(state RunState) string {
	switch state {
	case RunStateIdle:
		return "Idle"
	case RunStateRunning:
		return "Running"
	case RunStateShell:
		return "Shell"
	case RunStateShellThenRun:
		return "Shell (queued)"
	default:
		return "Unknown"
	}
}
