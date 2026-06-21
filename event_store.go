package main

import (
	"fmt"
	"sync"
	"time"
)

// ─── Event Sourcing / Sync (MiMo-Code 6) ───────────────────────────────────
//
// Event-sourcing layer for session state mutations.
// Supports replay, projector pattern, and dual-bus publishing.
//
// MiMo-Code source: sync/index.ts (278 lines)

// EventType represents the type of an event.
type EventType string

const (
	EventSessionCreated   EventType = "session.created"
	EventSessionUpdated   EventType = "session.updated"
	EventMessageAdded     EventType = "message.added"
	EventMessageRemoved   EventType = "message.removed"
	EventPartUpdated      EventType = "part.updated"
	EventPartRemoved      EventType = "part.removed"
)

// Event represents a state mutation event.
type Event struct {
	ID         string         `json:"id"`
	Type       EventType      `json:"type"`
	Sequence   int            `json:"sequence"`
	Timestamp  time.Time      `json:"timestamp"`
	AggregateID string        `json:"aggregate_id"`
	Data       map[string]any `json:"data"`
}

// Projector projects events into state.
type Projector interface {
	Apply(event Event) error
}

// EventStore stores events.
type EventStore struct {
	mu       sync.Mutex
	events   []Event
	sequences map[string]int // aggregateID -> last sequence
}

// NewEventStore creates a new event store.
func NewEventStore() *EventStore {
	return &EventStore{
		events:    make([]Event, 0),
		sequences: make(map[string]int),
	}
}

// Append appends an event to the store.
func (s *EventStore) Append(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Assign sequence
	s.sequences[event.AggregateID]++
	event.Sequence = s.sequences[event.AggregateID]

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	s.events = append(s.events, event)
	return nil
}

// GetEvents returns all events for an aggregate.
func (s *EventStore) GetEvents(aggregateID string) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []Event
	for _, e := range s.events {
		if e.AggregateID == aggregateID {
			result = append(result, e)
		}
	}
	return result
}

// GetAllEvents returns all events.
func (s *EventStore) GetAllEvents() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]Event, len(s.events))
	copy(result, s.events)
	return result
}

// Replay replays events through a projector.
func (s *EventStore) Replay(aggregateID string, projector Projector) error {
	events := s.GetEvents(aggregateID)
	for _, event := range events {
		if err := projector.Apply(event); err != nil {
			return err
		}
	}
	return nil
}

// ReplayAll replays all events through a projector.
func (s *EventStore) ReplayAll(projector Projector) error {
	events := s.GetAllEvents()
	for _, event := range events {
		if err := projector.Apply(event); err != nil {
			return err
		}
	}
	return nil
}

// Count returns the number of events.
func (s *EventStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

// Clear clears all events.
func (s *EventStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = make([]Event, 0)
	s.sequences = make(map[string]int)
}

// FormatEvent formats an event for display.
func FormatEvent(event Event) string {
	return fmt.Sprintf("[%s] %s (seq=%d) %s", event.Type, event.AggregateID, event.Sequence, event.Timestamp.Format(time.RFC3339))
}
