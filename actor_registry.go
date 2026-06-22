package main

import (
	"fmt"
	"sync"
	"time"
)

// ─── Actor Registry (MiMo-Code 4) ──────────────────────────────────────────
//
// Persistent registry that tracks all spawned agents with lifecycle management.
//
// MiMo-Code source: actor/registry.ts (412 lines)

// ActorMode represents the mode of an actor.
type ActorMode string

const (
	ActorModeSubagent ActorMode = "subagent"
	ActorModePeer     ActorMode = "peer"
)

// ActorStatus represents the status of an actor.
type ActorStatus string

const (
	ActorStatusRunning   ActorStatus = "running"
	ActorStatusCompleted ActorStatus = "completed"
	ActorStatusFailed    ActorStatus = "failed"
	ActorStatusKilled    ActorStatus = "killed"
	ActorStatusStuck     ActorStatus = "stuck"
)

// Actor represents a spawned agent in the registry.
type Actor struct {
	ID             string      `json:"id"`
	SessionID      string      `json:"session_id"`
	Mode           ActorMode   `json:"mode"`
	Status         ActorStatus `json:"status"`
	TaskID         string      `json:"task_id,omitempty"`
	ParentID       string      `json:"parent_id,omitempty"`
	IsSystemSpawned bool       `json:"is_system_spawned"`
	LastActivity   time.Time   `json:"last_activity"`
	StartedAt      time.Time   `json:"started_at"`
	CompletedAt    *time.Time  `json:"completed_at,omitempty"`
}

// ActorRegistry manages spawned agents.
type ActorRegistry struct {
	mu      sync.Mutex
	actors  map[string]*Actor
	stuckThreshold time.Duration
}

// NewActorRegistry creates a new actor registry.
func NewActorRegistry() *ActorRegistry {
	return &ActorRegistry{
		actors:         make(map[string]*Actor),
		stuckThreshold: 5 * time.Minute,
	}
}

// Register registers a new actor.
func (r *ActorRegistry) Register(id, sessionID string, mode ActorMode, isSystem bool) *Actor {
	r.mu.Lock()
	defer r.mu.Unlock()

	actor := &Actor{
		ID:              id,
		SessionID:       sessionID,
		Mode:            mode,
		Status:          ActorStatusRunning,
		IsSystemSpawned: isSystem,
		LastActivity:    time.Now(),
		StartedAt:       time.Now(),
	}

	r.actors[id] = actor
	return actor
}

// UpdateStatus updates the status of an actor.
func (r *ActorRegistry) UpdateStatus(id string, status ActorStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	actor, exists := r.actors[id]
	if !exists {
		return fmt.Errorf("actor not found: %s", id)
	}

	actor.Status = status
	actor.LastActivity = time.Now()

	if status == ActorStatusCompleted || status == ActorStatusFailed || status == ActorStatusKilled {
		now := time.Now()
		actor.CompletedAt = &now
	}

	return nil
}

// RecordActivity records activity for an actor.
func (r *ActorRegistry) RecordActivity(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if actor, exists := r.actors[id]; exists {
		actor.LastActivity = time.Now()
	}
}

// GetActor returns an actor by ID.
func (r *ActorRegistry) GetActor(id string) *Actor {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.actors[id]
}

// ListActors returns all actors.
func (r *ActorRegistry) ListActors() []*Actor {
	r.mu.Lock()
	defer r.mu.Unlock()

	var actors []*Actor
	for _, a := range r.actors {
		actors = append(actors, a)
	}
	return actors
}

// ListRunning returns all running actors.
func (r *ActorRegistry) ListRunning() []*Actor {
	r.mu.Lock()
	defer r.mu.Unlock()

	var running []*Actor
	for _, a := range r.actors {
		if a.Status == ActorStatusRunning {
			running = append(running, a)
		}
	}
	return running
}

// IsSystemSpawned checks if an actor is system-spawned.
func (r *ActorRegistry) IsSystemSpawned(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	actor, exists := r.actors[id]
	return exists && actor.IsSystemSpawned
}

// DetectStuck detects stuck actors (no activity for threshold).
func (r *ActorRegistry) DetectStuck() []*Actor {
	r.mu.Lock()
	defer r.mu.Unlock()

	var stuck []*Actor
	now := time.Now()

	for _, actor := range r.actors {
		if actor.Status == ActorStatusRunning {
			if now.Sub(actor.LastActivity) > r.stuckThreshold {
				actor.Status = ActorStatusStuck
				stuck = append(stuck, actor)
			}
		}
	}

	return stuck
}

// Remove removes an actor from the registry.
func (r *ActorRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.actors, id)
}

// Count returns the number of actors.
func (r *ActorRegistry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.actors)
}

// FormatActor formats an actor for display.
func FormatActor(actor *Actor) string {
	if actor == nil {
		return "No actor."
	}
	return fmt.Sprintf("%s (%s) [%s] %s", actor.ID, actor.Mode, actor.Status, actor.TaskID)
}
