package main

import (
	"testing"
)

func TestInboxService_New(t *testing.T) {
	s := NewInboxService("")
	if s == nil {
		t.Error("expected non-nil service")
	}
}

func TestInboxService_Send(t *testing.T) {
	s := NewInboxService("")

	msg := s.Send("agent-1", "agent-2", "hello")
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.FromAgent != "agent-1" {
		t.Errorf("expected 'agent-1', got %q", msg.FromAgent)
	}
	if msg.ToAgent != "agent-2" {
		t.Errorf("expected 'agent-2', got %q", msg.ToAgent)
	}
	if msg.Content != "hello" {
		t.Errorf("expected 'hello', got %q", msg.Content)
	}
}

func TestInboxService_Drain(t *testing.T) {
	s := NewInboxService("")

	s.Send("agent-1", "agent-2", "msg1")
	s.Send("agent-1", "agent-2", "msg2")
	s.Send("agent-1", "agent-2", "msg3")

	messages := s.Drain("agent-2", 0)
	if len(messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(messages))
	}

	// After drain, inbox should be empty
	if s.Count("agent-2") != 0 {
		t.Errorf("expected 0 messages after drain, got %d", s.Count("agent-2"))
	}
}

func TestInboxService_Drain_Limit(t *testing.T) {
	s := NewInboxService("")

	s.Send("agent-1", "agent-2", "msg1")
	s.Send("agent-1", "agent-2", "msg2")
	s.Send("agent-1", "agent-2", "msg3")

	messages := s.Drain("agent-2", 2)
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	// Should have 1 remaining
	if s.Count("agent-2") != 1 {
		t.Errorf("expected 1 message remaining, got %d", s.Count("agent-2"))
	}
}

func TestInboxService_Peek(t *testing.T) {
	s := NewInboxService("")

	s.Send("agent-1", "agent-2", "msg1")

	messages := s.Peek("agent-2")
	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}

	// Peek should not remove messages
	if s.Count("agent-2") != 1 {
		t.Errorf("expected 1 message after peek, got %d", s.Count("agent-2"))
	}
}

func TestInboxService_Count(t *testing.T) {
	s := NewInboxService("")

	if s.Count("agent-1") != 0 {
		t.Error("expected 0 messages initially")
	}

	s.Send("agent-2", "agent-1", "msg1")

	if s.Count("agent-1") != 1 {
		t.Errorf("expected 1 message, got %d", s.Count("agent-1"))
	}
}

func TestInboxService_Clear(t *testing.T) {
	s := NewInboxService("")

	s.Send("agent-1", "agent-2", "msg1")
	s.Clear("agent-2")

	if s.Count("agent-2") != 0 {
		t.Errorf("expected 0 messages after clear, got %d", s.Count("agent-2"))
	}
}

func TestInboxService_ClearAll(t *testing.T) {
	s := NewInboxService("")

	s.Send("agent-1", "agent-2", "msg1")
	s.Send("agent-2", "agent-1", "msg2")
	s.ClearAll()

	if s.Count("agent-1") != 0 {
		t.Errorf("expected 0 messages for agent-1, got %d", s.Count("agent-1"))
	}
	if s.Count("agent-2") != 0 {
		t.Errorf("expected 0 messages for agent-2, got %d", s.Count("agent-2"))
	}
}

func TestFormatInbox(t *testing.T) {
	messages := []*InboxMessage{
		{FromAgent: "agent-1", Content: "hello"},
	}

	output := FormatInbox(messages)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatInbox_Empty(t *testing.T) {
	output := FormatInbox(nil)
	if output != "No messages in inbox." {
		t.Errorf("expected 'No messages in inbox.', got %q", output)
	}
}

func TestFormatInboxForAgent(t *testing.T) {
	messages := []*InboxMessage{
		{FromAgent: "agent-1", Content: "hello"},
	}

	output := FormatInboxForAgent(messages)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatInboxForAgent_Empty(t *testing.T) {
	output := FormatInboxForAgent(nil)
	if output != "" {
		t.Errorf("expected empty output, got %q", output)
	}
}
