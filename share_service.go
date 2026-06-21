package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// ─── Session Sharing (MiMo-Code 7) ─────────────────────────────────────────
//
// Shares session state to a cloud service with real-time incremental sync.
// Enables collaborative debugging and session replay sharing.
//
// MiMo-Code source: share/session.ts (57 lines), share/share-next.ts (381 lines)

// ShareConfig holds sharing configuration.
type ShareConfig struct {
	Enabled   bool   `json:"enabled"`
	APIURL    string `json:"api_url"`
	APIKey    string `json:"api_key"`
	AutoShare bool   `json:"auto_share"`
}

// ShareState represents the share state of a session.
type ShareState struct {
	SessionID   string    `json:"session_id"`
	ShareID     string    `json:"share_id"`
	ShareURL    string    `json:"share_url"`
	SharedAt    time.Time `json:"shared_at"`
	LastSyncAt  time.Time `json:"last_sync_at"`
	IsShared    bool      `json:"is_shared"`
}

// ShareService manages session sharing.
type ShareService struct {
	mu      sync.Mutex
	config  ShareConfig
	states  map[string]*ShareState // sessionID -> share state
	client  *http.Client
}

// NewShareService creates a new share service.
func NewShareService(config ShareConfig) *ShareService {
	return &ShareService{
		config: config,
		states: make(map[string]*ShareState),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Share shares a session to the cloud.
func (s *ShareService) Share(sessionID string, metadata map[string]any) (*ShareState, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("sharing is not enabled")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already shared
	if state, exists := s.states[sessionID]; exists && state.IsShared {
		return state, nil
	}

	// Create share via API
	reqBody := map[string]any{
		"session_id": sessionID,
		"metadata":   metadata,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", s.config.APIURL+"/share", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.config.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("share request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("share failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		ShareID  string `json:"share_id"`
		ShareURL string `json:"share_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	state := &ShareState{
		SessionID:  sessionID,
		ShareID:    result.ShareID,
		ShareURL:   result.ShareURL,
		SharedAt:   time.Now(),
		LastSyncAt: time.Now(),
		IsShared:   true,
	}

	s.states[sessionID] = state
	return state, nil
}

// Unshare removes a session from sharing.
func (s *ShareService) Unshare(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.states[sessionID]
	if !exists || !state.IsShared {
		return nil
	}

	// Delete share via API
	req, err := http.NewRequest("DELETE", s.config.APIURL+"/share/"+state.ShareID, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.config.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("unshare request: %w", err)
	}
	defer resp.Body.Close()

	state.IsShared = false
	return nil
}

// Sync syncs session data to the cloud.
func (s *ShareService) Sync(sessionID string, data map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.states[sessionID]
	if !exists || !state.IsShared {
		return nil
	}

	jsonBody, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	req, err := http.NewRequest("PATCH", s.config.APIURL+"/share/"+state.ShareID, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.config.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sync request: %w", err)
	}
	defer resp.Body.Close()

	state.LastSyncAt = time.Now()
	return nil
}

// GetShareState returns the share state for a session.
func (s *ShareService) GetShareState(sessionID string) *ShareState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.states[sessionID]
}

// IsShared checks if a session is shared.
func (s *ShareService) IsShared(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, exists := s.states[sessionID]
	return exists && state.IsShared
}

// GetShareURL returns the share URL for a session.
func (s *ShareService) GetShareURL(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state, exists := s.states[sessionID]; exists {
		return state.ShareURL
	}
	return ""
}

// FormatShareState formats a share state for display.
func FormatShareState(state *ShareState) string {
	if state == nil {
		return "Session not shared."
	}

	var sb string
	sb += "## Session Share\n\n"
	sb += fmt.Sprintf("- **Session ID**: %s\n", state.SessionID)
	sb += fmt.Sprintf("- **Share ID**: %s\n", state.ShareID)
	sb += fmt.Sprintf("- **URL**: %s\n", state.ShareURL)
	sb += fmt.Sprintf("- **Shared At**: %s\n", state.SharedAt.Format(time.RFC3339))
	sb += fmt.Sprintf("- **Last Sync**: %s\n", state.LastSyncAt.Format(time.RFC3339))

	return sb
}
