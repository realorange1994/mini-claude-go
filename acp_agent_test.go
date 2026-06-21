package main

import (
	"testing"
)

func TestACPAgent_New(t *testing.T) {
	agent := NewACPAgent(nil)
	if agent == nil {
		t.Error("expected non-nil agent")
	}
}

func TestACPAgent_HandleInitialize(t *testing.T) {
	agent := NewACPAgent(nil)

	req := &ACPRequest{
		Method: ACPInitialize,
	}

	result, err := agent.handleInitialize(req)
	if err != nil {
		t.Fatalf("handleInitialize failed: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map result")
	}
	if resultMap["serverName"] != "miniclaudecode" {
		t.Errorf("expected 'miniclaudecode', got %q", resultMap["serverName"])
	}
}

func TestACPAgent_HandleNewSession(t *testing.T) {
	agent := NewACPAgent(nil)

	req := &ACPRequest{
		Method: ACPNewSession,
		Params: []byte(`{"model":"claude-sonnet-4-20250514","mode":"auto"}`),
	}

	result, err := agent.handleNewSession(req)
	if err != nil {
		t.Fatalf("handleNewSession failed: %v", err)
	}

	session, ok := result.(*ACPSession)
	if !ok {
		t.Fatal("expected ACPSession result")
	}
	if session.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected 'claude-sonnet-4-20250514', got %q", session.Model)
	}
}

func TestACPAgent_HandleListSessions(t *testing.T) {
	agent := NewACPAgent(nil)

	// Add a session
	agent.sessions["test"] = &ACPSession{ID: "test", Model: "claude-sonnet-4-20250514"}

	req := &ACPRequest{
		Method: ACPListSessions,
	}

	result, err := agent.handleListSessions(req)
	if err != nil {
		t.Fatalf("handleListSessions failed: %v", err)
	}

	sessions, ok := result.([]*ACPSession)
	if !ok {
		t.Fatal("expected []*ACPSession result")
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
}

func TestACPAgent_HandleLoadSession(t *testing.T) {
	agent := NewACPAgent(nil)

	// Add a session
	agent.sessions["test"] = &ACPSession{ID: "test", Model: "claude-sonnet-4-20250514"}

	req := &ACPRequest{
		Method: ACPLoadSession,
		Params: []byte(`{"sessionId":"test"}`),
	}

	result, err := agent.handleLoadSession(req)
	if err != nil {
		t.Fatalf("handleLoadSession failed: %v", err)
	}

	session, ok := result.(*ACPSession)
	if !ok {
		t.Fatal("expected ACPSession result")
	}
	if session.ID != "test" {
		t.Errorf("expected 'test', got %q", session.ID)
	}
}

func TestACPAgent_HandleLoadSession_NotFound(t *testing.T) {
	agent := NewACPAgent(nil)

	req := &ACPRequest{
		Method: ACPLoadSession,
		Params: []byte(`{"sessionId":"nonexistent"}`),
	}

	_, err := agent.handleLoadSession(req)
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestACPAgent_HandleForkSession(t *testing.T) {
	agent := NewACPAgent(nil)

	// Add a session
	agent.sessions["original"] = &ACPSession{ID: "original", Model: "claude-sonnet-4-20250514"}

	req := &ACPRequest{
		Method: ACPForkSession,
		Params: []byte(`{"sessionId":"original"}`),
	}

	result, err := agent.handleForkSession(req)
	if err != nil {
		t.Fatalf("handleForkSession failed: %v", err)
	}

	session, ok := result.(*ACPSession)
	if !ok {
		t.Fatal("expected ACPSession result")
	}
	if session.ID == "original" {
		t.Error("expected different ID for forked session")
	}
}

func TestACPAgent_HandleSetModel(t *testing.T) {
	agent := NewACPAgent(nil)

	// Add a session
	agent.sessions["test"] = &ACPSession{ID: "test", Model: "claude-sonnet-4-20250514"}

	req := &ACPRequest{
		Method: ACPSetModel,
		Params: []byte(`{"sessionId":"test","model":"claude-opus-4-20250514"}`),
	}

	_, err := agent.handleSetModel(req)
	if err != nil {
		t.Fatalf("handleSetModel failed: %v", err)
	}

	if agent.sessions["test"].Model != "claude-opus-4-20250514" {
		t.Errorf("expected 'claude-opus-4-20250514', got %q", agent.sessions["test"].Model)
	}
}

func TestACPAgent_HandleSetMode(t *testing.T) {
	agent := NewACPAgent(nil)

	// Add a session
	agent.sessions["test"] = &ACPSession{ID: "test", Mode: "auto"}

	req := &ACPRequest{
		Method: ACPSetMode,
		Params: []byte(`{"sessionId":"test","mode":"bypass"}`),
	}

	_, err := agent.handleSetMode(req)
	if err != nil {
		t.Fatalf("handleSetMode failed: %v", err)
	}

	if agent.sessions["test"].Mode != "bypass" {
		t.Errorf("expected 'bypass', got %q", agent.sessions["test"].Mode)
	}
}

func TestACPAgent_Stop(t *testing.T) {
	agent := NewACPAgent(nil)
	agent.running = true

	agent.Stop()

	if agent.isRunning() {
		t.Error("expected agent to be stopped")
	}
}
