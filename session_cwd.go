package main

import (
	"os"
	"path/filepath"
	"sync"
)

// ─── Session-Scoped CWD (MiMo-Code 3) ──────────────────────────────────────
//
// Provides a change_directory tool that lets the agent switch its working
// directory per-session. All subsequent file operations resolve relative
// paths from the new directory.
//
// MiMo-Code source: tool/change-directory.ts (91 lines), tool/session-cwd.ts (35 lines)

// SessionCwd manages per-session working directories.
type SessionCwd struct {
	mu      sync.Mutex
	paths   map[string]string // sessionID -> cwd
	projectDir string
}

// NewSessionCwd creates a new session CWD manager.
func NewSessionCwd(projectDir string) *SessionCwd {
	return &SessionCwd{
		paths:      make(map[string]string),
		projectDir: projectDir,
	}
}

// Get returns the current working directory for a session.
func (s *SessionCwd) Get(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cwd, exists := s.paths[sessionID]; exists {
		return cwd
	}
	return s.projectDir
}

// Set sets the working directory for a session.
func (s *SessionCwd) Set(sessionID string, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Resolve relative paths
	if !filepath.IsAbs(path) {
		currentCwd := s.paths[sessionID]
		if currentCwd == "" {
			currentCwd = s.projectDir
		}
		path = filepath.Join(currentCwd, path)
	}

	// Verify directory exists
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &os.PathError{Op: "chdir", Path: path, Err: os.ErrInvalid}
	}

	s.paths[sessionID] = path
	return nil
}

// Clear resets the working directory for a session to project root.
func (s *SessionCwd) Clear(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.paths, sessionID)
}

// ResolvePath resolves a path relative to the session's CWD.
func (s *SessionCwd) ResolvePath(sessionID string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}

	cwd := s.Get(sessionID)
	return filepath.Join(cwd, path)
}

// ChangeDirectoryTool provides the change_directory tool.
type ChangeDirectoryTool struct {
	cwd *SessionCwd
}

// NewChangeDirectoryTool creates a new change_directory tool.
func NewChangeDirectoryTool(cwd *SessionCwd) *ChangeDirectoryTool {
	return &ChangeDirectoryTool{cwd: cwd}
}

// Execute changes the working directory.
func (t *ChangeDirectoryTool) Execute(sessionID string, path string) error {
	if path == "~" || path == "" {
		t.cwd.Clear(sessionID)
		return nil
	}
	return t.cwd.Set(sessionID, path)
}
