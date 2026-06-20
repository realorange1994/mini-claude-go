package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Session Revert with Snapshot Restoration (MiMo-Code 3A) ────────────────
//
// Full session revert system with git snapshot restoration.
// Supports reverting file changes from specific agent steps.
//
// MiMo-Code source: session/revert.ts (161 lines)

// RevertState holds the state of a revert operation.
type RevertState struct {
	mu           sync.Mutex
	SessionID    string `json:"session_id"`
	SnapshotDir  string `json:"snapshot_dir"`
	RevertPoint  int    `json:"revert_point"`
	RevertTime   time.Time `json:"revert_time"`
	OriginalFiles map[string][]byte `json:"original_files"`
	Changes      []RevertChange `json:"changes"`
}

// RevertChange represents a file change during revert.
type RevertChange struct {
	Path        string
	Original    []byte
	Reverted    []byte
	Timestamp   time.Time
}

// RevertManager manages session reverts.
type RevertManager struct {
	mu          sync.Mutex
	snapshotDir string
	reverts     map[string]*RevertState // sessionID -> revert state
}

// NewRevertManager creates a new revert manager.
func NewRevertManager(snapshotDir string) *RevertManager {
	return &RevertManager{
		snapshotDir: snapshotDir,
		reverts:     make(map[string]*RevertState),
	}
}

// RevertPoint represents a point in the session to revert to.
type RevertPoint struct {
	SessionID   string
	MessageIdx  int
	Timestamp   time.Time
	FilesChanged []string
}

// CreateSnapshot creates a snapshot of the current file state.
func (m *RevertManager) CreateSnapshot(sessionID string, files map[string][]byte) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure snapshot directory exists
	os.MkdirAll(m.snapshotDir, 0755)

	// Generate snapshot ID
	snapshotID := fmt.Sprintf("snap-%s-%s", sessionID, time.Now().Format("20060102-150405"))
	snapshotPath := filepath.Join(m.snapshotDir, snapshotID)

	// Save file contents
	os.MkdirAll(snapshotPath, 0755)
	for path, content := range files {
		filePath := filepath.Join(snapshotPath, filepath.Base(path))
		os.WriteFile(filePath, content, 0644)
	}

	return snapshotID
}

// RevertToFile reverts a specific file to its snapshot state.
func (m *RevertManager) RevertToFile(sessionID, filePath string, snapshotID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	snapshotPath := filepath.Join(m.snapshotDir, snapshotID, filepath.Base(filePath))
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}

	// Write original content back
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	// Track revert
	state := m.getOrCreateState(sessionID)
	state.Changes = append(state.Changes, RevertChange{
		Path:      filePath,
		Reverted:  data,
		Timestamp: time.Now(),
	})

	return nil
}

// RevertSession reverts all files in a session to a snapshot.
func (m *RevertManager) RevertSession(sessionID string, snapshotID string, files []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreateState(sessionID)
	state.RevertPoint = len(state.Changes)
	state.RevertTime = time.Now()

	for _, filePath := range files {
		snapshotPath := filepath.Join(m.snapshotDir, snapshotID, filepath.Base(filePath))
		data, err := os.ReadFile(snapshotPath)
		if err != nil {
			continue
		}

		// Save original content
		original, _ := os.ReadFile(filePath)
		state.OriginalFiles[filePath] = original

		// Write snapshot content
		os.WriteFile(filePath, data, 0644)

		state.Changes = append(state.Changes, RevertChange{
			Path:      filePath,
			Original:  original,
			Reverted:  data,
			Timestamp: time.Now(),
		})
	}

	m.reverts[sessionID] = state
	return nil
}

// UndoRevert undoes the last revert operation.
func (m *RevertManager) UndoRevert(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.reverts[sessionID]
	if !exists {
		return fmt.Errorf("no revert state for session %s", sessionID)
	}

	// Restore original files
	for path, content := range state.OriginalFiles {
		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("restore file %s: %w", path, err)
		}
	}

	delete(m.reverts, sessionID)
	return nil
}

// GetRevertState returns the current revert state for a session.
func (m *RevertManager) GetRevertState(sessionID string) *RevertState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reverts[sessionID]
}

// HasRevertState checks if a session has a pending revert.
func (m *RevertManager) HasRevertState(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.reverts[sessionID]
	return exists
}

// getOrCreateState gets or creates a revert state for a session.
func (m *RevertManager) getOrCreateState(sessionID string) *RevertState {
	if state, exists := m.reverts[sessionID]; exists {
		return state
	}
	state := &RevertState{
		SessionID:     sessionID,
		SnapshotDir:   m.snapshotDir,
		OriginalFiles: make(map[string][]byte),
	}
	m.reverts[sessionID] = state
	return state
}

// SaveRevertMetadata saves revert metadata to disk.
func (m *RevertManager) SaveRevertMetadata(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.reverts[sessionID]
	if !exists {
		return nil
	}

	metadataPath := filepath.Join(m.snapshotDir, sessionID+"-revert.json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metadataPath, data, 0644)
}

// LoadRevertMetadata loads revert metadata from disk.
func (m *RevertManager) LoadRevertMetadata(sessionID string) (*RevertState, error) {
	metadataPath := filepath.Join(m.snapshotDir, sessionID+"-revert.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}

	var state RevertState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}
