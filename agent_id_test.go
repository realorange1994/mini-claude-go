package main

import (
	"testing"
)

func TestBuildMessagesForAgent_MainOnly(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// Add main agent message
	ctx.AddUserMessage("hello from main")
	ctx.AddAssistantText("response from main")

	// Add subagent message manually
	ctx.mu.Lock()
	ctx.entries = append(ctx.entries, conversationEntry{
		role:    "assistant",
		content: TextContent("response from subagent"),
		agentID: "agent-1",
	})
	ctx.mu.Unlock()

	// Build messages for main agent only
	msgs := ctx.BuildMessagesForAgent("main")
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages (user + main assistant), got %d", len(msgs))
	}
}

func TestBuildMessagesForAgent_All(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// Add main agent message
	ctx.AddUserMessage("hello from main")

	// Add subagent message manually
	ctx.mu.Lock()
	ctx.entries = append(ctx.entries, conversationEntry{
		role:    "assistant",
		content: TextContent("response from subagent"),
		agentID: "agent-1",
	})
	ctx.mu.Unlock()

	// Build messages for all agents
	msgs := ctx.BuildMessagesForAgent("*")
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

func TestBuildMessagesForAgent_SubagentOnly(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// Add main agent message
	ctx.AddUserMessage("hello from main")

	// Add subagent message manually
	ctx.mu.Lock()
	ctx.entries = append(ctx.entries, conversationEntry{
		role:    "assistant",
		content: TextContent("response from subagent"),
		agentID: "agent-1",
	})
	ctx.mu.Unlock()

	// Build messages for subagent only
	msgs := ctx.BuildMessagesForAgent("agent-1")
	if len(msgs) != 1 {
		t.Errorf("expected 1 message (subagent only), got %d", len(msgs))
	}
}

func TestConversationEntry_AgentID(t *testing.T) {
	entry := conversationEntry{
		role:    "assistant",
		content: TextContent("test"),
		agentID: "agent-1",
	}

	if entry.agentID != "agent-1" {
		t.Errorf("expected agentID 'agent-1', got '%s'", entry.agentID)
	}
}

func TestConversationEntry_DefaultAgentID(t *testing.T) {
	entry := conversationEntry{
		role:    "user",
		content: TextContent("test"),
	}

	// Default agentID should be empty (main agent)
	if entry.agentID != "" {
		t.Errorf("expected empty agentID, got '%s'", entry.agentID)
	}
}
