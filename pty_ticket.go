package main

import (
	"sync"
	"time"
)

// ─── PTY Ticket System (MiMo-Code 5) ───────────────────────────────────────
//
// One-time-use ticket system for PTY WebSocket connections.
// Prevents unauthorized or replayed connection attempts.
//
// MiMo-Code source: server/pty-ticket.ts (42 lines)

// PTYTicket represents a one-time-use connection ticket.
type PTYTicket struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Consumed  bool      `json:"consumed"`
}

// PTYTicketManager manages PTY connection tickets.
type PTYTicketManager struct {
	mu           sync.Mutex
	tickets      map[string]*PTYTicket
	ttl          time.Duration
	gcInterval   time.Duration
	stopGC       chan struct{}
}

// NewPTYTicketManager creates a new ticket manager.
func NewPTYTicketManager() *PTYTicketManager {
	m := &PTYTicketManager{
		tickets:    make(map[string]*PTYTicket),
		ttl:        60 * time.Second,
		gcInterval: 30 * time.Second,
		stopGC:     make(chan struct{}),
	}
	go m.runGC()
	return m
}

// Issue issues a new ticket.
func (m *PTYTicketManager) Issue(sessionID string) *PTYTicket {
	m.mu.Lock()
	defer m.mu.Unlock()

	ticket := &PTYTicket{
		ID:        generateTicketID(),
		SessionID: sessionID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(m.ttl),
	}

	m.tickets[ticket.ID] = ticket
	return ticket
}

// Consume consumes a ticket (single-use).
func (m *PTYTicketManager) Consume(ticketID string) (*PTYTicket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ticket, exists := m.tickets[ticketID]
	if !exists {
		return nil, ErrTicketNotFound
	}

	if ticket.Consumed {
		return nil, ErrTicketConsumed
	}

	if time.Now().After(ticket.ExpiresAt) {
		return nil, ErrTicketExpired
	}

	ticket.Consumed = true
	return ticket, nil
}

// Revoke revokes a ticket.
func (m *PTYTicketManager) Revoke(ticketID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tickets, ticketID)
}

// runGC garbage collects expired tickets.
func (m *PTYTicketManager) runGC() {
	ticker := time.NewTicker(m.gcInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.mu.Lock()
			now := time.Now()
			for id, ticket := range m.tickets {
				if now.After(ticket.ExpiresAt) || ticket.Consumed {
					delete(m.tickets, id)
				}
			}
			m.mu.Unlock()
		case <-m.stopGC:
			return
		}
	}
}

// Stop stops the garbage collector.
func (m *PTYTicketManager) Stop() {
	close(m.stopGC)
}

// Count returns the number of active tickets.
func (m *PTYTicketManager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.tickets)
}

// generateTicketID generates a unique ticket ID.
func generateTicketID() string {
	return time.Now().Format("20060102150405.000000000")
}

// Ticket errors.
var (
	ErrTicketNotFound = &TicketError{"ticket not found"}
	ErrTicketConsumed = &TicketError{"ticket already consumed"}
	ErrTicketExpired  = &TicketError{"ticket expired"}
)

type TicketError struct {
	msg string
}

func (e *TicketError) Error() string {
	return e.msg
}

// FormatTicketStatus formats a ticket for display.
func FormatTicketStatus(ticket *PTYTicket) string {
	if ticket == nil {
		return "No ticket."
	}
	if ticket.Consumed {
		return "Ticket consumed."
	}
	if time.Now().After(ticket.ExpiresAt) {
		return "Ticket expired."
	}
	return "Ticket active."
}
