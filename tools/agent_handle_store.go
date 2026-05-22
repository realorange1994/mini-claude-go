package tools

import (
	"sync"
)

// AgentHandle represents a running or completed sub-agent that can be
// addressed by name.
type AgentHandle struct {
	Name   string `json:"name"`
	TaskID string `json:"task_id"`
	Status string `json:"status"` // "running", "completed", "failed"
	Result string `json:"result,omitempty"`
	Done   chan struct{}
}

// AgentHandleStore tracks named agents for routing via send_message.
// It provides a lightweight name -> handle mapping on top of the task store.
type AgentHandleStore struct {
	mu     sync.RWMutex
	agents map[string]*AgentHandle // name -> handle
}

// NewAgentHandleStore creates an empty agent handle store.
func NewAgentHandleStore() *AgentHandleStore {
	return &AgentHandleStore{agents: make(map[string]*AgentHandle)}
}

// Register adds or updates a named agent handle.
func (s *AgentHandleStore) Register(name string, handle *AgentHandle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[name] = handle
}

// Lookup returns the handle for a given name, or (nil, false) if not found.
func (s *AgentHandleStore) Lookup(name string) (*AgentHandle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.agents[name]
	return h, ok
}

// List returns all registered agent handles.
func (s *AgentHandleStore) List() []*AgentHandle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*AgentHandle, 0, len(s.agents))
	for _, h := range s.agents {
		result = append(result, h)
	}
	return result
}

// Complete updates the status and result of a named agent.
func (s *AgentHandleStore) Complete(name, result string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h, ok := s.agents[name]; ok {
		if h.Status != "completed" && h.Status != "failed" {
			h.Status = "completed"
			h.Result = result
			close(h.Done)
		}
	}
}

// Fail updates the status of a named agent to indicate failure.
func (s *AgentHandleStore) Fail(name, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h, ok := s.agents[name]; ok {
		if h.Status != "completed" && h.Status != "failed" {
			h.Status = "failed"
			h.Result = errMsg
			close(h.Done)
		}
	}
}

// Count returns the number of registered agents.
func (s *AgentHandleStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.agents)
}
