package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ─── Archive Management (extracted from compact.go) ─────────────────────────

// archiveRounds saves rounds to an archive file.
func archiveRounds(archiveDir string, rounds []apiRound) (string, error) {
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return "", fmt.Errorf("create archive dir: %w", err)
	}

	// Flatten rounds to messages
	var messages []CompactionMessage
	for _, round := range rounds {
		messages = append(messages, round.messages...)
	}

	// Generate archive filename
	archivePath := filepath.Join(archiveDir, fmt.Sprintf("archive-%s.json", timestampStr()))

	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal archive: %w", err)
	}

	if err := os.WriteFile(archivePath, data, 0644); err != nil {
		return "", fmt.Errorf("write archive: %w", err)
	}

	return archivePath, nil
}

// LoadArchive loads messages from an archive file.
func LoadArchive(path string) ([]CompactionMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}

	var messages []CompactionMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("unmarshal archive: %w", err)
	}

	return messages, nil
}

// ListArchives lists archive files in a directory.
func ListArchives(archiveDir string) ([]os.FileInfo, error) {
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return nil, err
	}

	var archives []os.FileInfo
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			info, err := entry.Info()
			if err == nil {
				archives = append(archives, info)
			}
		}
	}

	return archives, nil
}

// timestampStr returns a timestamp string for filenames.
func timestampStr() string {
	return fmt.Sprintf("%d", time.Now().UnixMilli())
}
