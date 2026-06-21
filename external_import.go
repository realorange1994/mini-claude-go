package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─── External Session Import (MiMo-Code 4) ─────────────────────────────────
//
// Imports session history from Claude Code, OpenAI Codex, and other CLI agents.
// Enables cross-tool continuity.
//
// MiMo-Code source: session/claude-import.ts (381 lines), session/codex-import.ts (416 lines)

// ExternalSource represents the source of an external session.
type ExternalSource string

const (
	SourceClaudeCode ExternalSource = "claude-code"
	SourceCodex      ExternalSource = "codex"
	SourceOpenCode   ExternalSource = "opencode"
)

// ExternalSession represents an imported external session.
type ExternalSession struct {
	ID        string          `json:"id"`
	Source    ExternalSource  `json:"source"`
	Title     string          `json:"title"`
	StartTime time.Time       `json:"start_time"`
	EndTime   time.Time       `json:"end_time"`
	Messages  []ExternalMessage `json:"messages"`
	Metadata  map[string]any  `json:"metadata"`
}

// ExternalMessage represents a message from an external session.
type ExternalMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Model     string `json:"model,omitempty"`
}

// ExternalImportConfig holds import configuration.
type ExternalImportConfig struct {
	Enabled       bool   `json:"enabled"`
	ClaudeCodeDir string `json:"claude_code_dir"`
	CodexDir      string `json:"codex_dir"`
	OpenCodeDir   string `json:"opencode_dir"`
}

// ExternalImportService manages external session imports.
type ExternalImportService struct {
	mu       sync.Mutex
	config   ExternalImportConfig
	imported map[string]time.Time // session ID -> last import time
}

// NewExternalImportService creates a new import service.
func NewExternalImportService(config ExternalImportConfig) *ExternalImportService {
	return &ExternalImportService{
		config:   config,
		imported: make(map[string]time.Time),
	}
}

// ScanClaudeCode scans Claude Code sessions directory.
func (s *ExternalImportService) ScanClaudeCode() ([]ExternalSession, error) {
	if s.config.ClaudeCodeDir == "" {
		homeDir, _ := os.UserHomeDir()
		if homeDir != "" {
			s.config.ClaudeCodeDir = filepath.Join(homeDir, ".claude", "projects")
		}
	}

	if _, err := os.Stat(s.config.ClaudeCodeDir); os.IsNotExist(err) {
		return nil, nil
	}

	var sessions []ExternalSession

	// Walk through project directories
	entries, err := os.ReadDir(s.config.ClaudeCodeDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectDir := filepath.Join(s.config.ClaudeCodeDir, entry.Name())
		sessionFiles, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
		if err != nil {
			continue
		}

		for _, sessionFile := range sessionFiles {
			session, err := s.parseClaudeCodeSession(sessionFile)
			if err != nil {
				continue
			}
			sessions = append(sessions, *session)
		}
	}

	return sessions, nil
}

// parseClaudeCodeSession parses a Claude Code JSONL session file.
func (s *ExternalImportService) parseClaudeCodeSession(filePath string) (*ExternalSession, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	session := &ExternalSession{
		ID:       filepath.Base(filePath),
		Source:   SourceClaudeCode,
		Messages: make([]ExternalMessage, 0),
		Metadata: make(map[string]any),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// Parse message
		if role, ok := entry["role"].(string); ok {
			msg := ExternalMessage{
				Role: role,
			}

			if content, ok := entry["content"].(string); ok {
				msg.Content = content
			} else if content, ok := entry["content"].([]any); ok {
				// Handle array content
				var parts []string
				for _, part := range content {
					if p, ok := part.(map[string]any); ok {
						if text, ok := p["text"].(string); ok {
							parts = append(parts, text)
						}
					}
				}
				msg.Content = strings.Join(parts, "\n")
			}

			if model, ok := entry["model"].(string); ok {
				msg.Model = model
			}

			if ts, ok := entry["timestamp"].(string); ok {
				if t, err := time.Parse(time.RFC3339, ts); err == nil {
					msg.Timestamp = t
				}
			}

			session.Messages = append(session.Messages, msg)
		}

		// Extract metadata
		if title, ok := entry["title"].(string); ok && session.Title == "" {
			session.Title = title
		}
	}

	if len(session.Messages) > 0 {
		session.StartTime = session.Messages[0].Timestamp
		session.EndTime = session.Messages[len(session.Messages)-1].Timestamp
	}

	return session, nil
}

// ScanCodex scans OpenAI Codex sessions directory.
func (s *ExternalImportService) ScanCodex() ([]ExternalSession, error) {
	if s.config.CodexDir == "" {
		homeDir, _ := os.UserHomeDir()
		if homeDir != "" {
			s.config.CodexDir = filepath.Join(homeDir, ".codex", "sessions")
		}
	}

	if _, err := os.Stat(s.config.CodexDir); os.IsNotExist(err) {
		return nil, nil
	}

	var sessions []ExternalSession

	entries, err := os.ReadDir(s.config.CodexDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		filePath := filepath.Join(s.config.CodexDir, entry.Name())
		session, err := s.parseCodexSession(filePath)
		if err != nil {
			continue
		}
		sessions = append(sessions, *session)
	}

	return sessions, nil
}

// parseCodexSession parses a Codex JSONL session file.
func (s *ExternalImportService) parseCodexSession(filePath string) (*ExternalSession, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	session := &ExternalSession{
		ID:       filepath.Base(filePath),
		Source:   SourceCodex,
		Messages: make([]ExternalMessage, 0),
		Metadata: make(map[string]any),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if role, ok := entry["role"].(string); ok {
			msg := ExternalMessage{
				Role: role,
			}

			if content, ok := entry["content"].(string); ok {
				msg.Content = content
			}

			if model, ok := entry["model"].(string); ok {
				msg.Model = model
			}

			session.Messages = append(session.Messages, msg)
		}
	}

	return session, nil
}

// ImportSession imports an external session into the local system.
func (s *ExternalImportService) ImportSession(session ExternalSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already imported
	if lastImport, exists := s.imported[session.ID]; exists {
		if session.EndTime.Before(lastImport) || session.EndTime.Equal(lastImport) {
			return nil // Already up to date
		}
	}

	// Mark as imported
	s.imported[session.ID] = time.Now()

	return nil
}

// ScanAll scans all configured external sources.
func (s *ExternalImportService) ScanAll() ([]ExternalSession, error) {
	var allSessions []ExternalSession

	claudeSessions, err := s.ScanClaudeCode()
	if err == nil {
		allSessions = append(allSessions, claudeSessions...)
	}

	codexSessions, err := s.ScanCodex()
	if err == nil {
		allSessions = append(allSessions, codexSessions...)
	}

	return allSessions, nil
}

// GetImportedCount returns the number of imported sessions.
func (s *ExternalImportService) GetImportedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.imported)
}

// IsImported checks if a session has been imported.
func (s *ExternalImportService) IsImported(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.imported[sessionID]
	return exists
}

// FormatImportSummary formats an import summary for display.
func FormatImportSummary(sessions []ExternalSession) string {
	if len(sessions) == 0 {
		return "No external sessions found."
	}

	var sb string
	sb += fmt.Sprintf("## External Sessions (%d found)\n\n", len(sessions))

	bySource := make(map[ExternalSource]int)
	for _, s := range sessions {
		bySource[s.Source]++
	}

	for source, count := range bySource {
		sb += fmt.Sprintf("- %s: %d sessions\n", source, count)
	}

	sb += "\n### Recent Sessions\n\n"
	for i, s := range sessions {
		if i >= 5 {
			break
		}
		title := s.Title
		if title == "" {
			title = "Untitled"
		}
		sb += fmt.Sprintf("- [%s] %s (%d messages)\n", s.Source, title, len(s.Messages))
	}

	return sb
}
