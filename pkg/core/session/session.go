package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Session represents a single agent session
type Session struct {
	Id           string
	Path         string
	RootId       string // root session ID for the tree
	ParentId     string // parent session ID (empty for root)
	Model        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	messageCount int
	mu           sync.RWMutex
}

// MessageEntry represents a message in the session JSONL
type MessageEntry struct {
	Type      json.RawMessage `json:"type"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// SessionManager manages all sessions (mirrors pi's SessionManager)
type SessionManager struct {
	sessionsDir string
	sessions    map[string]*Session
	mu          sync.RWMutex
	activeId    string
}

// NewSessionManager creates a session manager
func NewSessionManager(sessionsDir string) (*SessionManager, error) {
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return nil, fmt.Errorf("session: create sessions dir: %w", err)
	}
	sm := &SessionManager{
		sessionsDir: sessionsDir,
		sessions:    make(map[string]*Session),
	}
	// Load existing sessions
	if err := sm.loadSessions(); err != nil {
		return nil, err
	}
	return sm, nil
}

// NewSession creates a new session
func (sm *SessionManager) NewSession(model, cwd string) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sessionId := sm.generateId()
	timestamp := time.Now()

	s := &Session{
		Id:        sessionId,
		RootId:    sessionId,
		ParentId:  "",
		Model:     model,
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
	}

	// Create session directory
	s.Path = filepath.Join(sm.sessionsDir, sessionId)
	if err := os.MkdirAll(s.Path, 0755); err != nil {
		return nil, fmt.Errorf("session: create session dir: %w", err)
	}

	// Create metadata file
	meta := map[string]interface{}{
		"id":        s.Id,
		"rootId":    s.RootId,
		"parentId":  s.ParentId,
		"model":     s.Model,
		"createdAt": s.CreatedAt.Unix(),
		"cwd":       cwd,
	}
	if err := sm.writeMetadata(s, meta); err != nil {
		return nil, err
	}

	sm.sessions[sessionId] = s
	sm.activeId = sessionId
	return s, nil
}

// Fork creates a branched session from a parent session
func (sm *SessionManager) Fork(parentId, message string) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	parent, ok := sm.sessions[parentId]
	if !ok {
		return nil, fmt.Errorf("parent session not found: %s", parentId)
	}

	sessionId := sm.generateId()
	timestamp := time.Now()

	s := &Session{
		Id:        sessionId,
		RootId:    parent.RootId,
		ParentId:  parentId,
		Model:     parent.Model,
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
	}

	s.Path = filepath.Join(sm.sessionsDir, s.RootId, sessionId)
	if err := os.MkdirAll(s.Path, 0755); err != nil {
		return nil, fmt.Errorf("session: create forked session dir: %w", err)
	}

	// Write fork metadata
	meta := map[string]interface{}{
		"id":        s.Id,
		"rootId":    s.RootId,
		"parentId":  s.ParentId,
		"model":     s.Model,
		"forkMsg":   message,
		"createdAt": s.CreatedAt.Unix(),
	}
	if err := sm.writeMetadata(s, meta); err != nil {
		return nil, err
	}

	sm.sessions[sessionId] = s
	sm.activeId = sessionId
	return s, nil
}

// AppendMessage appends a message to the session's JSONL file
func (sm *SessionManager) AppendMessage(sessionId string, entryType string, data interface{}) error {
	sm.mu.Lock()
	session, ok := sm.sessions[sessionId]
	sm.mu.Unlock()

	if !ok {
		return fmt.Errorf("session not found: %s", sessionId)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("session: marshal message: %w", err)
	}

	entry := MessageEntry{
		Type:      json.RawMessage(fmt.Sprintf(`"%s"`, entryType)),
		Timestamp: time.Now().Unix(),
		Data:      jsonData,
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("session: marshal entry: %w", err)
	}

	f, err := os.OpenFile(sm.getMessagesPath(session), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("session: open messages file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("session: write message: %w", err)
	}

	session.messageCount++
	session.UpdatedAt = time.Now()
	return nil
}

// GetMessages returns all messages from a session
func (sm *SessionManager) GetMessages(sessionId string) ([]MessageEntry, error) {
	sm.mu.RLock()
	session, ok := sm.sessions[sessionId]
	sm.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionId)
	}

	path := sm.getMessagesPath(session)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []MessageEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry MessageEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// SwitchSession sets the active session
func (sm *SessionManager) SwitchSession(sessionId string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if _, ok := sm.sessions[sessionId]; !ok {
		return fmt.Errorf("session not found: %s", sessionId)
	}
	sm.activeId = sessionId
	return nil
}

// GetActiveSession returns the currently active session
func (sm *SessionManager) GetActiveSession() *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[sm.activeId]
}

// GetSession returns a session by ID
func (sm *SessionManager) GetSession(id string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[id]
}

// ListSessions returns all sessions
func (sm *SessionManager) ListSessions() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	sessions := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// GetSessionBranches returns all branches from a root session
func (sm *SessionManager) GetSessionBranches(rootId string) []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var branches []*Session
	for _, s := range sm.sessions {
		if s.RootId == rootId {
			branches = append(branches, s)
		}
	}
	return branches
}

func (sm *SessionManager) generateId() string {
	return fmt.Sprintf("sess_%d_%d", time.Now().UnixNano(), len(sm.sessions))
}

func (sm *SessionManager) getMessagesPath(s *Session) string {
	return filepath.Join(s.Path, "messages.jsonl")
}

func (sm *SessionManager) writeMetadata(s *Session, meta map[string]interface{}) error {
	metaPath := filepath.Join(s.Path, "meta.json")
	f, err := os.Create(metaPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(meta)
}

func (sm *SessionManager) loadSessions() error {
	entries, err := os.ReadDir(sm.sessionsDir)
	if err != nil {
		return nil // no sessions dir yet
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Each session directory
		metaPath := filepath.Join(sm.sessionsDir, entry.Name(), "meta.json")
		f, err := os.Open(metaPath)
		if err != nil {
			continue
		}
		var meta map[string]interface{}
		if err := json.NewDecoder(f).Decode(&meta); err != nil {
			f.Close()
			continue
		}
		f.Close()

		s := &Session{
			Id:   entry.Name(),
			Path: filepath.Join(sm.sessionsDir, entry.Name()),
		}
		if v, ok := meta["rootId"].(string); ok {
			s.RootId = v
		}
		if v, ok := meta["parentId"].(string); ok {
			s.ParentId = v
		}
		if v, ok := meta["model"].(string); ok {
			s.Model = v
		}
		if v, ok := meta["createdAt"].(float64); ok {
			s.CreatedAt = time.Unix(int64(v), 0)
			s.UpdatedAt = s.CreatedAt
		}
		sm.sessions[s.Id] = s
	}
	return nil
}
