package tools

import (
	"fmt"
	"path/filepath"
	"time"
)

// SidechainTranscript manages transcript recording for sub-agents.
// It records the sub-agent's conversation separately from the parent,
// while maintaining a link back to the parent's entry via ParentUUID.
type SidechainTranscript struct {
	ParentUUID string // UUID of the parent agent's entry
	Path       string // path to sidechain transcript file
	writer     writerIface
}

// writerIface is a minimal interface to avoid import cycles with the transcript package.
type writerIface interface {
	WriteUser(content string) error
	WriteAssistant(content, model string) error
	WriteToolUse(toolID, toolName string, args map[string]any) error
	WriteToolResult(toolID, toolName, result string) error
	WriteError(err string) error
	WriteSystem(content string) error
	FilePath() string
}

// NewSidechainTranscript creates a sidechain transcript for a sub-agent.
// The transcript file is named sidechain-<agentName>-<timestamp>.jsonl
// and placed in the sessionDir.
func NewSidechainTranscript(parentUUID, sessionDir, agentName string) *SidechainTranscript {
	filename := fmt.Sprintf("sidechain-%s-%s.jsonl", agentName, time.Now().Format("20060102-150405"))
	path := filepath.Join(sessionDir, filename)
	return &SidechainTranscript{
		ParentUUID: parentUUID,
		Path:       path,
	}
}

// InitWriter initializes the transcript writer.
// Must be called before Write* methods. The factory function creates the
// actual writer (typically transcript.NewWriter).
func (s *SidechainTranscript) InitWriter(factory func(sessionID, filePath string) writerIface) {
	if s.writer == nil {
		sessionID := "sidechain-" + time.Now().Format("20060102-150405")
		s.writer = factory(sessionID, s.Path)
	}
}

// Writer returns the underlying writer interface, or nil if not initialized.
func (s *SidechainTranscript) Writer() writerIface {
	return s.writer
}

// RecordToolUse records a tool use in the sidechain transcript.
func (s *SidechainTranscript) RecordToolUse(toolID, toolName string, args map[string]any) error {
	if s.writer == nil {
		return nil // silently skip if not initialized
	}
	return s.writer.WriteToolUse(toolID, toolName, args)
}

// RecordToolResult records a tool result in the sidechain transcript.
func (s *SidechainTranscript) RecordToolResult(toolID, toolName, result string) error {
	if s.writer == nil {
		return nil
	}
	return s.writer.WriteToolResult(toolID, toolName, result)
}

// RecordSystem records a system message in the sidechain transcript.
func (s *SidechainTranscript) RecordSystem(content string) error {
	if s.writer == nil {
		return nil
	}
	return s.writer.WriteSystem(content)
}

// RecordError records an error in the sidechain transcript.
func (s *SidechainTranscript) RecordError(err string) error {
	if s.writer == nil {
		return nil
	}
	return s.writer.WriteError(err)
}
