package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Inter-Agent Inbox (MiMo-Code 5) ───────────────────────────────────────
//
// Persistent inter-agent messaging system.
// Allows agents to send messages to each other.
//
// MiMo-Code source: inbox/inbox.ts (223 lines)

// InboxMessage represents a message in the inbox.
type InboxMessage struct {
	ID         string    `json:"id"`
	FromAgent  string    `json:"from_agent"`
	ToAgent    string    `json:"to_agent"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
	Read       bool      `json:"read"`
}

// InboxService manages inter-agent messaging.
type InboxService struct {
	mu        sync.Mutex
	inboxDir  string
	messages  map[string][]*InboxMessage // agentID -> messages
}

// NewInboxService creates a new inbox service.
func NewInboxService(inboxDir string) *InboxService {
	return &InboxService{
		inboxDir: inboxDir,
		messages: make(map[string][]*InboxMessage),
	}
}

// Send sends a message from one agent to another.
func (s *InboxService) Send(fromAgent, toAgent, content string) *InboxMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := &InboxMessage{
		ID:        fmt.Sprintf("msg-%s", time.Now().Format("20060102-150405.000")),
		FromAgent: fromAgent,
		ToAgent:   toAgent,
		Content:   content,
		Timestamp: time.Now(),
	}

	s.messages[toAgent] = append(s.messages[toAgent], msg)

	// Persist to disk
	s.persistMessage(msg)

	return msg
}

// Drain drains messages for an agent (returns and marks as read).
func (s *InboxService) Drain(agentID string, limit int) []*InboxMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages := s.messages[agentID]
	if len(messages) == 0 {
		return nil
	}

	// Apply limit
	if limit > 0 && len(messages) > limit {
		messages = messages[:limit]
	}

	// Mark as read
	for _, msg := range messages {
		msg.Read = true
	}

	// Remove from inbox
	s.messages[agentID] = s.messages[agentID][len(messages):]

	return messages
}

// Peek returns messages for an agent without marking as read.
func (s *InboxService) Peek(agentID string) []*InboxMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages := s.messages[agentID]
	result := make([]*InboxMessage, len(messages))
	copy(result, messages)
	return result
}

// Count returns the number of unread messages for an agent.
func (s *InboxService) Count(agentID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.messages[agentID])
}

// Clear clears all messages for an agent.
func (s *InboxService) Clear(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.messages, agentID)
}

// ClearAll clears all messages.
func (s *InboxService) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = make(map[string][]*InboxMessage)
}

// persistMessage persists a message to disk.
func (s *InboxService) persistMessage(msg *InboxMessage) {
	if s.inboxDir == "" {
		return
	}

	os.MkdirAll(s.inboxDir, 0755)

	path := filepath.Join(s.inboxDir, msg.ID+".json")
	data, _ := json.MarshalIndent(msg, "", "  ")
	os.WriteFile(path, data, 0644)
}

// LoadMessages loads messages from disk.
func (s *InboxService) LoadMessages() error {
	if s.inboxDir == "" {
		return nil
	}

	entries, err := os.ReadDir(s.inboxDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(s.inboxDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var msg InboxMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if !msg.Read {
			s.messages[msg.ToAgent] = append(s.messages[msg.ToAgent], &msg)
		}
	}

	return nil
}

// FormatInbox formats inbox messages for display.
func FormatInbox(messages []*InboxMessage) string {
	if len(messages) == 0 {
		return "No messages in inbox."
	}

	var sb string
	sb += fmt.Sprintf("## Inbox (%d messages)\n\n", len(messages))

	for _, msg := range messages {
		sb += fmt.Sprintf("### From: %s\n", msg.FromAgent)
		sb += fmt.Sprintf("_%s_\n\n", msg.Timestamp.Format(time.RFC3339))
		sb += msg.Content + "\n\n"
	}

	return sb
}

// FormatInboxForAgent formats inbox messages as an XML block for agent injection.
func FormatInboxForAgent(messages []*InboxMessage) string {
	if len(messages) == 0 {
		return ""
	}

	var sb string
	sb += "<inbox>\n"
	for _, msg := range messages {
		sb += fmt.Sprintf("  <message from=\"%s\" sent_at=\"%s\">\n", msg.FromAgent, msg.Timestamp.Format(time.RFC3339))
		sb += "    " + msg.Content + "\n"
		sb += "  </message>\n"
	}
	sb += "</inbox>"

	return sb
}
