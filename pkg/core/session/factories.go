// Package session provides static factory methods and session management utilities.
// Aligned to pi's session-manager.ts static factories and utilities.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionInfo holds metadata about a session file.
type SessionInfo struct {
	Path          string
	ID            string
	Cwd           string
	Name          string // from session_info entries
	ParentSession string // path to parent session (if forked)
	Created       time.Time
	Modified      time.Time
	MessageCount  int
	FirstMessage  string
	AllMessages   string // concatenated text of user+assistant messages
}

// GetDefaultSessionDir returns the default session directory for a cwd.
// Aligned to pi's getDefaultSessionDir(cwd, agentDir).
// Structure: ~/.miniclaude/sessions/--<encoded-cwd>--/
func GetDefaultSessionDir(cwd string, agentDir string) string {
	resolvedCwd := filepath.Clean(cwd)
	resolvedAgent := filepath.Clean(agentDir)

	// Encode cwd by replacing path separators and colons with dashes
	safePath := strings.ReplaceAll(resolvedCwd, string(filepath.Separator), "-")
	safePath = strings.ReplaceAll(safePath, ":", "-")
	safePath = strings.TrimLeft(safePath, "-.")
	if safePath == "" {
		safePath = "root"
	}
	safePath = "--" + safePath + "--"

	sessionDir := filepath.Join(resolvedAgent, "sessions", safePath)
	os.MkdirAll(sessionDir, 0755)
	return sessionDir
}

// FindMostRecentSession finds the most recent .jsonl session file in a directory.
func FindMostRecentSession(sessionDir string) (string, error) {
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return "", err
	}

	var bestPath string
	var bestTime time.Time

	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}

		fullPath := filepath.Join(sessionDir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}

		// Validate it's a proper session file
		if !isValidSessionFile(fullPath) {
			continue
		}

		if info.ModTime().After(bestTime) {
			bestTime = info.ModTime()
			bestPath = fullPath
		}
	}

	if bestPath == "" {
		return "", fmt.Errorf("no valid sessions found")
	}
	return bestPath, nil
}

// isValidSessionFile checks if a .jsonl file has a valid session header.
func isValidSessionFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	// Read first line
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	firstLine := strings.TrimSpace(string(buf[:n]))
	if firstLine == "" {
		return false
	}

	var header struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal([]byte(firstLine), &header); err != nil {
		return false
	}
	return header.Type == "session" && header.ID != ""
}

// LoadEntriesFromFile reads a JSONL session file and returns parsed entries.
// Exposed public version of the internal loadEntriesFromFile.
func LoadEntriesFromFile(path string) ([]FileEntry, error) {
	return loadEntriesFromFile(path)
}

// Create creates a new session with optional explicit session directory.
// Aligned to pi's SessionManager.create(cwd, sessionDir).
func Create(cwd string, sessionDir string) (*SessionManager, error) {
	if sessionDir == "" {
		home, _ := os.UserHomeDir()
		agentDir := filepath.Join(home, ".miniclaude")
		sessionDir = GetDefaultSessionDir(cwd, agentDir)
	}

	sm := NewSessionManager(cwd, sessionDir, "", true)
	return sm, nil
}

// Open opens a specific session file.
// Aligned to pi's SessionManager.open(path, sessionDir, cwdOverride).
func Open(path string, cwdOverride string, sessionDir string) (*SessionManager, error) {
	resolvedPath := filepath.Clean(path)
	if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("session file not found: %s", path)
	}

	// Load entries
	entries, err := loadEntriesFromFile(resolvedPath)
	if err != nil || len(entries) == 0 {
		return nil, fmt.Errorf("failed to load session: %v", err)
	}

	header, _ := entries[0].(SessionHeader)
	cwd := header.Cwd
	if cwd == "" {
		cwd = cwdOverride
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	sDir := sessionDir
	if sDir == "" {
		sDir = filepath.Dir(resolvedPath)
	}

	sm := NewSessionManager(cwd, sDir, resolvedPath, true)
	return sm, nil
}

// ContinueRecent opens the most recent session for a cwd, or creates a new one.
// Aligned to pi's SessionManager.continueRecent(cwd, sessionDir).
func ContinueRecent(cwd string, agentDir string, sessionDir string) (*SessionManager, error) {
	if sessionDir == "" {
		sessionDir = GetDefaultSessionDir(cwd, agentDir)
	}

	// Find most recent session
	sessionPath, err := FindMostRecentSession(sessionDir)
	if err != nil || sessionPath == "" {
		// No session found, create new
		return Create(cwd, sessionDir)
	}

	return Open(sessionPath, cwd, sessionDir)
}

// InMemory creates a non-persisted session.
// Aligned to pi's SessionManager.inMemory(cwd).
func InMemory(cwd string) *SessionManager {
	sm := NewSessionManager(cwd, "", "", false)
	return sm
}

// ForkFrom forks a session from a source session file into a new session.
// Aligned to pi's SessionManager.forkFrom(sourcePath, targetCwd, sessionDir).
func ForkFrom(sourcePath string, targetCwd string, sessionDir string) (*SessionManager, error) {
	// Load source entries
	srcEntries, err := loadEntriesFromFile(sourcePath)
	if err != nil || len(srcEntries) == 0 {
		return nil, fmt.Errorf("failed to load source session: %v", err)
	}

	// Create target session dir
	if sessionDir == "" {
		home, _ := os.UserHomeDir()
		agentDir := filepath.Join(home, ".miniclaude")
		sessionDir = GetDefaultSessionDir(targetCwd, agentDir)
	}

	os.MkdirAll(sessionDir, 0755)

	// Generate new session ID and timestamp
	newID := generateID()
	ts := nowISO()

	// Create new file
	fileTs := strings.ReplaceAll(ts, ":", "-")
	fileTs = strings.ReplaceAll(fileTs, ".", "-")
	targetPath := filepath.Join(sessionDir, fmt.Sprintf("%s_%s.jsonl", fileTs, newID))

	// Write header with parentSession reference
	header := SessionHeader{
		Type:          "session",
		Version:       CurrentSessionVersion,
		ID:            newID,
		Timestamp:     ts,
		Cwd:           targetCwd,
		ParentSession: sourcePath,
	}

	f, err := os.Create(targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create forked session file: %v", err)
	}

	// Write header
	hdrLine, _ := json.Marshal(header)
	f.Write(append(hdrLine, '\n'))

	// Copy all non-header entries
	for _, e := range srcEntries {
		if _, ok := e.(SessionHeader); ok {
			continue
		}
		line, _ := json.Marshal(e)
		f.Write(append(line, '\n'))
	}
	f.Close()

	// Create session manager pointing to new file
	sm := NewSessionManager(targetCwd, sessionDir, targetPath, true)
	return sm, nil
}

// CreateBranchedSession extracts a linear path from root to leafId into a new session file.
// Aligned to pi's SessionManager.createBranchedSession(leafId).
func (sm *SessionManager) CreateBranchedSession(leafID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get branch path
	branch := sm.GetBranch(leafID)
	if len(branch) == 0 {
		return fmt.Errorf("no entries found for leaf %s", leafID)
	}

	// Filter out LabelEntry items
	var filtered []FileEntry
	for _, e := range branch {
		if _, ok := e.(LabelEntry); !ok {
			filtered = append(filtered, e)
		}
	}

	// Generate new session
	newID := generateID()
	ts := nowISO()
	origFile := sm.sessionFile

	// Create new file
	fileTs := strings.ReplaceAll(ts, ":", "-")
	fileTs = strings.ReplaceAll(fileTs, ".", "-")
	newFile := filepath.Join(sm.sessionDir, fmt.Sprintf("%s_%s.jsonl", fileTs, newID))

	f, err := os.Create(newFile)
	if err != nil {
		return fmt.Errorf("failed to create branched session file: %v", err)
	}

	// Write header with parentSession pointing to original
	header := SessionHeader{
		Type:          "session",
		Version:       CurrentSessionVersion,
		ID:            newID,
		Timestamp:     ts,
		Cwd:           sm.cwd,
		ParentSession: origFile,
	}

	hdrLine, _ := json.Marshal(header)
	f.Write(append(hdrLine, '\n'))

	// Write filtered entries
	for _, e := range filtered {
		line, _ := json.Marshal(e)
		f.Write(append(line, '\n'))
	}
	f.Close()

	// Replace session state
	sm.sessionFile = newFile
	sm.sessionID = newID
	sm.fileEntries = filtered
	sm.buildIndex()
	sm.flushed = true

	// Persist the file (rewrite)
	sm.rewriteFile()

	return nil
}

// GetChildren returns all entries whose parentId matches the given ID.
func (sm *SessionManager) GetChildren(parentID string) []FileEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var children []FileEntry
	for _, e := range sm.byID {
		if entryParentID(e) == parentID {
			children = append(children, e)
		}
	}
	return children
}

// SessionListProgress is a callback for reporting session loading progress.
type SessionListProgress func(loaded int, total int)

// List sessions for a directory, sorted by modified descending.
func List(sessionDir string, onProgress SessionListProgress) ([]SessionInfo, error) {
	files, err := filepath.Glob(filepath.Join(sessionDir, "*.jsonl"))
	if err != nil {
		return nil, err
	}

	var infos []SessionInfo
	for i, f := range files {
		info, err := BuildSessionInfo(f)
		if err != nil {
			continue
		}
		infos = append(infos, info)
		if onProgress != nil {
			onProgress(i+1, len(files))
		}
	}

	// Sort by modified descending
	for i := 0; i < len(infos)-1; i++ {
		for j := 0; j < len(infos)-i-1; j++ {
			if infos[j].Modified.Before(infos[j+1].Modified) {
				infos[j], infos[j+1] = infos[j+1], infos[j]
			}
		}
	}

	return infos, nil
}

// BuildSessionInfo builds a SessionInfo from a single .jsonl file.
func BuildSessionInfo(path string) (SessionInfo, error) {
	entries, err := loadEntriesFromFile(path)
	if err != nil || len(entries) == 0 {
		return SessionInfo{}, fmt.Errorf("failed to load session: %v", err)
	}

	header, _ := entries[0].(SessionHeader)

	var name string
	var msgCount int
	var firstMsg string
	var allMsgs strings.Builder

	for _, e := range entries {
		switch entry := e.(type) {
		case SessionInfoEntry:
			name = entry.Name
		case SessionMessageEntry:
			msgCount++
			var msgData map[string]interface{}
			if json.Unmarshal(entry.Message, &msgData) == nil {
				role, _ := msgData["role"].(string)
				content, _ := msgData["content"].(string)
				if role == "user" && firstMsg == "" {
					firstMsg = content
				}
				if role == "user" || role == "assistant" {
					if allMsgs.Len() > 0 {
						allMsgs.WriteString("\n")
					}
					allMsgs.WriteString(content)
				}
			}
		}
	}

	fileInfo, err := os.Stat(path)
	modTime := time.Now()
	if err == nil {
		modTime = fileInfo.ModTime()
	}

	return SessionInfo{
		Path:          path,
		ID:            header.ID,
		Cwd:           header.Cwd,
		Name:          name,
		ParentSession: header.ParentSession,
		Created:       parseTime(header.Timestamp),
		Modified:      modTime,
		MessageCount:  msgCount,
		FirstMessage:  firstMsg,
		AllMessages:   allMsgs.String(),
	}, nil
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now()
	}
	return t
}
