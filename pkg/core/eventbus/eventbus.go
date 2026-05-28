package eventbus

import (
	"fmt"
	"sync"
	"time"
)

// Handler is a function that processes an event payload.
// Returning an error signals handler failure (non-blocking: errors collected).
type Handler func(payload interface{}) error

// handlerEntry pairs a handler with a unique numeric ID and an optional name
// so it can be identified later for removal without comparing function values.
type handlerEntry struct {
	id      int
	handler Handler
	name    string
}

// EventBus is a simple typed event emitter with safe async handler support.
// Mirrors pi's core/event-bus.ts.
type EventBus struct {
	mu       sync.RWMutex
	nextID   int
	handlers map[string][]handlerEntry
}

// New creates a new EventBus instance.
func New() *EventBus {
	return &EventBus{handlers: make(map[string][]handlerEntry)}
}

// On registers a handler for the given event name and returns a token
// that can be used to unregister the handler later via Off.
func (eb *EventBus) On(event string, handler Handler) *HandlerToken {
	return eb.OnNamed(event, "", handler)
}

// OnNamed registers a handler with an explicit name for identification.
// The name is used by RemoveHandlersByName to remove all handlers with that name
// for a given event.
func (eb *EventBus) OnNamed(event, name string, handler Handler) *HandlerToken {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	id := eb.nextID
	eb.nextID++
	token := &HandlerToken{id: id, event: event, name: name}
	eb.handlers[event] = append(eb.handlers[event], handlerEntry{id: id, handler: handler, name: name})
	return token
}

// Off unregisters a handler using the token returned by On.
// If the token is nil or the handler was already removed, Off is a no-op.
func (eb *EventBus) Off(token *HandlerToken) {
	if token == nil {
		return
	}
	eb.mu.Lock()
	defer eb.mu.Unlock()
	entries := eb.handlers[token.event]
	for i, entry := range entries {
		if entry.id == token.id {
			eb.handlers[token.event] = append(entries[:i], entries[i+1:]...)
			return
		}
	}
}

// Emit fires an event to all registered handlers synchronously,
// collecting and returning all errors encountered.
func (eb *EventBus) Emit(event string, payload interface{}) []error {
	eb.mu.RLock()
	entries := make([]handlerEntry, len(eb.handlers[event]))
	copy(entries, eb.handlers[event])
	eb.mu.RUnlock()

	if len(entries) == 0 {
		return nil
	}
	var errs []error
	for _, entry := range entries {
		if err := entry.handler(payload); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// AsyncEmit fires an event to all registered handlers concurrently,
// returning a channel that will receive the collected errors.
// Non-blocking: returns immediately with a receive channel.
// Note: the returned channel MUST be read to prevent goroutine leaks.
// Use AsyncEmitWithTimeout if you cannot guarantee the channel will be read.
func (eb *EventBus) AsyncEmit(event string, payload interface{}) <-chan []error {
	return eb.AsyncEmitWithTimeout(event, payload, 30*time.Second)
}

// AsyncEmitWithTimeout is like AsyncEmit but the returned channel is
// auto-closed after the timeout to prevent goroutine leaks.
// If the handlers do not complete in time, nil errors are returned.
func (eb *EventBus) AsyncEmitWithTimeout(event string, payload interface{}, timeout time.Duration) <-chan []error {
	ch := make(chan []error, 1)
	go func() {
		done := make(chan []error, 1)
		go func() {
			eb.mu.RLock()
			entries := make([]handlerEntry, len(eb.handlers[event]))
			copy(entries, eb.handlers[event])
			eb.mu.RUnlock()

			if len(entries) == 0 {
				done <- nil
				return
			}
			var errs []error
			for _, entry := range entries {
				if err := entry.handler(payload); err != nil {
					errs = append(errs, err)
				}
			}
			done <- errs
		}()

		select {
		case errs := <-done:
			ch <- errs
		case <-time.After(timeout):
			ch <- []error{fmt.Errorf("async emit timed out after %v", timeout)}
		}
	}()
	return ch
}

// Events returns a copy of all registered event names.
func (eb *EventBus) Events() []string {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	events := make([]string, 0, len(eb.handlers))
	for event := range eb.handlers {
		events = append(events, event)
	}
	return events
}

// HandlerCount returns the number of handlers for the given event.
func (eb *EventBus) HandlerCount(event string) int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.handlers[event])
}

// Clear removes all handlers for all events.
func (eb *EventBus) Clear() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	for k := range eb.handlers {
		delete(eb.handlers, k)
	}
}

// RemoveAllForEvent removes all handlers for a specific event.
func (eb *EventBus) RemoveAllForEvent(event string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	delete(eb.handlers, event)
}

// RemoveHandlersByName removes all handlers with the given name from the
// specified event. Returns the number of handlers removed.
func (eb *EventBus) RemoveHandlersByName(event string, name string) int {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if name == "" {
		return 0
	}
	entries := eb.handlers[event]
	removed := 0
	filtered := entries[:0]
	for _, entry := range entries {
		if entry.name == name {
			removed++
		} else {
			filtered = append(filtered, entry)
		}
	}
	eb.handlers[event] = filtered
	return removed
}

// HandlerToken is returned by On and used by Off to identify a registered handler.
// This avoids the Go limitation that function values are not directly comparable.
type HandlerToken struct {
	id    int
	event string
	name  string
}

// Event returns the event name this token was registered for.
func (t *HandlerToken) Event() string {
	return t.event
}

// Name returns the handler name (if any) this token was registered with.
func (t *HandlerToken) Name() string {
	return t.name
}
