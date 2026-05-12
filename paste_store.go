package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// PasteStore stores and retrieves content by hash.
// This allows the agent to reference previously shared content
// without re-transmitting it in full.
type PasteStore struct {
	dir string
}

// NewPasteStore creates a paste store in .claude/paste/
func NewPasteStore() *PasteStore {
	dir := filepath.Join(".claude", "paste")
	os.MkdirAll(dir, 0o755)
	return &PasteStore{dir: dir}
}

// Store saves content and returns its hash-based key.
func (p *PasteStore) Store(content string) string {
	hash := sha256.Sum256([]byte(content))
	key := hex.EncodeToString(hash[:])[:16] // Use first 16 chars of hex

	filePath := filepath.Join(p.dir, key)
	os.WriteFile(filePath, []byte(content), 0o644)

	return key
}

// Retrieve loads content by key.
func (p *PasteStore) Retrieve(key string) (string, error) {
	filePath := filepath.Join(p.dir, key)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("paste not found: %s", key)
	}
	return string(data), nil
}

// List returns all stored paste keys.
func (p *PasteStore) List() []string {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return nil
	}
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			keys = append(keys, e.Name())
		}
	}
	return keys
}

// Delete removes a paste by key.
func (p *PasteStore) Delete(key string) error {
	filePath := filepath.Join(p.dir, key)
	return os.Remove(filePath)
}
