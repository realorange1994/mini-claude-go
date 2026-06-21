package main

import (
	"fmt"
	"sync"
	"time"
)

// ─── Bash Interactive Service (MiMo-Code 7) ────────────────────────────────
//
// Deferred request/reply mechanism for interactive bash commands.
// Handles commands that need user input (sudo password, database prompt).
//
// MiMo-Code source: tool/bash-interactive.ts (183 lines)

// InteractiveRequest represents a request for user input.
type InteractiveRequest struct {
	ID          string    `json:"id"`
	Command     string    `json:"command"`
	Cwd         string    `json:"cwd"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Resolved    bool      `json:"resolved"`
	Response    string    `json:"response,omitempty"`
}

// BashInteractiveService manages interactive bash requests.
type BashInteractiveService struct {
	mu       sync.Mutex
	requests map[string]*InteractiveRequest
}

// NewBashInteractiveService creates a new bash interactive service.
func NewBashInteractiveService() *BashInteractiveService {
	return &BashInteractiveService{
		requests: make(map[string]*InteractiveRequest),
	}
}

// Request creates a new interactive request and waits for response.
func (s *BashInteractiveService) Request(command, cwd, description string) (string, error) {
	s.mu.Lock()

	req := &InteractiveRequest{
		ID:          fmt.Sprintf("req-%s", time.Now().Format("20060102-150405.000")),
		Command:     command,
		Cwd:         cwd,
		Description: description,
		Timestamp:   time.Now(),
	}

	s.requests[req.ID] = req
	s.mu.Unlock()

	// Wait for response (with timeout)
	timeout := 5 * time.Minute
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		s.mu.Lock()
		if req.Resolved {
			response := req.Response
			delete(s.requests, req.ID)
			s.mu.Unlock()
			return response, nil
		}
		s.mu.Unlock()

		time.Sleep(100 * time.Millisecond)
	}

	// Timeout
	s.mu.Lock()
	delete(s.requests, req.ID)
	s.mu.Unlock()
	return "", fmt.Errorf("timeout waiting for interactive input")
}

// Reply provides a response to an interactive request.
func (s *BashInteractiveService) Reply(requestID, response string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, exists := s.requests[requestID]
	if !exists {
		return fmt.Errorf("request not found: %s", requestID)
	}

	req.Resolved = true
	req.Response = response
	return nil
}

// List returns all pending interactive requests.
func (s *BashInteractiveService) List() []*InteractiveRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	var pending []*InteractiveRequest
	for _, req := range s.requests {
		if !req.Resolved {
			pending = append(pending, req)
		}
	}
	return pending
}

// Cancel cancels a pending request.
func (s *BashInteractiveService) Cancel(requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, exists := s.requests[requestID]
	if !exists {
		return fmt.Errorf("request not found: %s", requestID)
	}

	req.Resolved = true
	req.Response = ""
	return nil
}
