package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TruncatedResultSaver saves truncated tool output to disk and returns
// a recallable path. This matches DeepSeek-Reasonix's
// truncated-result-saver pattern: when tool output exceeds the truncation
// limit, the full content is saved to .claude/truncated-results/ so the
// agent can recall it later with read_file instead of re-running the tool.
//
// This has two benefits:
//  1. Reduces context window pollution from massive outputs
//  2. Enables the LLM to selectively recall only the parts it needs
type TruncatedResultSaver struct {
	projectDir string
	maxAge     time.Duration // default 30 days
}

// NewTruncatedResultSaver creates a saver for the given project directory.
func NewTruncatedResultSaver(projectDir string) *TruncatedResultSaver {
	return &TruncatedResultSaver{
		projectDir: projectDir,
		maxAge:     30 * 24 * time.Hour,
	}
}

// Save writes the full tool output to disk and returns a message string
// indicating where the content was saved. If saving fails, returns empty string.
func (s *TruncatedResultSaver) Save(toolName, content string) string {
	if s.projectDir == "" {
		return ""
	}

	dir := filepath.Join(s.projectDir, ".claude", "truncated-results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}

	timestamp := time.Now().Format("20060102-150405")
	uid := shortUUID8()
	filename := fmt.Sprintf("%s-%s-%s.txt", timestamp, uid, sanitizeToolName(toolName))
	fullPath := filepath.Join(dir, filename)

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return ""
	}

	// Return relative path from project dir
	relPath := filepath.Join(".claude", "truncated-results", filename)
	return fmt.Sprintf("Full output saved to %s (use read_file to recall)", relPath)
}

// CleanupOld removes truncated result files older than maxAge.
func (s *TruncatedResultSaver) CleanupOld() {
	if s.projectDir == "" {
		return
	}

	dir := filepath.Join(s.projectDir, ".claude", "truncated-results")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-s.maxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// sanitizeToolName makes a tool name safe for use in a filename.
func sanitizeToolName(name string) string {
	r := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		" ", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"?", "_",
		"*", "_",
		"\"", "_",
	)
	return r.Replace(name)
}

// shortUUID8 returns an 8-character random hex string.
func shortUUID8() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
