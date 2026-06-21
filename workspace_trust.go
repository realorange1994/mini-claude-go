package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ─── Workspace Trust System (MiMo-Code 1) ──────────────────────────────────
//
// Classifies workspace directories into trust levels.
// Prevents agent from operating in sensitive directories without consent.
//
// MiMo-Code source: project/workspace-trust.ts (67 lines)

// TrustLevel represents the trust level of a workspace.
type TrustLevel string

const (
	TrustTrusted   TrustLevel = "trusted"
	TrustUntrusted TrustLevel = "untrusted"
	TrustDangerous TrustLevel = "dangerous"
)

// WorkspaceTrust manages workspace trust levels.
type WorkspaceTrust struct {
	mu       sync.Mutex
	trusted  map[string]bool
	dataDir  string
}

// NewWorkspaceTrust creates a new workspace trust manager.
func NewWorkspaceTrust(dataDir string) *WorkspaceTrust {
	w := &WorkspaceTrust{
		trusted: make(map[string]bool),
		dataDir: dataDir,
	}
	w.load()
	return w
}

// GetTrustLevel returns the trust level for a directory.
func (w *WorkspaceTrust) GetTrustLevel(dir string) TrustLevel {
	w.mu.Lock()
	defer w.mu.Unlock()

	normalized := filepath.Clean(dir)

	// Check dangerous paths
	if w.isDangerous(normalized) {
		return TrustDangerous
	}

	// Check trusted paths
	if w.trusted[normalized] {
		return TrustTrusted
	}

	// Check if parent is trusted
	for trusted := range w.trusted {
		if strings.HasPrefix(normalized, trusted+string(filepath.Separator)) {
			return TrustTrusted
		}
	}

	return TrustUntrusted
}

// IsTrusted returns true if the directory is trusted.
func (w *WorkspaceTrust) IsTrusted(dir string) bool {
	return w.GetTrustLevel(dir) == TrustTrusted
}

// IsDangerous returns true if the directory is dangerous.
func (w *WorkspaceTrust) IsDangerous(dir string) bool {
	return w.GetTrustLevel(dir) == TrustDangerous
}

// MarkTrusted marks a directory as trusted.
func (w *WorkspaceTrust) MarkTrusted(dir string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	normalized := filepath.Clean(dir)
	if w.isDangerous(normalized) {
		return &os.PathError{Op: "trust", Path: dir, Err: os.ErrPermission}
	}

	w.trusted[normalized] = true
	w.save()
	return nil
}

// RevokeTrust revokes trust for a directory.
func (w *WorkspaceTrust) RevokeTrust(dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	normalized := filepath.Clean(dir)
	delete(w.trusted, normalized)
	w.save()
}

// isDangerous checks if a path is dangerous.
func (w *WorkspaceTrust) isDangerous(dir string) bool {
	// Home directory
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" && filepath.Clean(dir) == filepath.Clean(homeDir) {
		return true
	}

	// Filesystem roots
	if dir == "/" || dir == "C:\\" || dir == "C:" {
		return true
	}

	// System directories
	dangerousPaths := []string{
		"/etc", "/var", "/usr", "/bin", "/sbin",
		"C:\\Windows", "C:\\Program Files", "C:\\ProgramData",
	}
	for _, dangerous := range dangerousPaths {
		if strings.HasPrefix(dir, dangerous) {
			return true
		}
	}

	return false
}

// save persists trusted directories to disk.
func (w *WorkspaceTrust) save() {
	if w.dataDir == "" {
		return
	}

	path := filepath.Join(w.dataDir, "trusted-workspaces.json")
	os.MkdirAll(filepath.Dir(path), 0755)

	var dirs []string
	for dir := range w.trusted {
		dirs = append(dirs, dir)
	}

	data, _ := json.MarshalIndent(dirs, "", "  ")
	os.WriteFile(path, data, 0644)
}

// load loads trusted directories from disk.
func (w *WorkspaceTrust) load() {
	if w.dataDir == "" {
		return
	}

	path := filepath.Join(w.dataDir, "trusted-workspaces.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var dirs []string
	if json.Unmarshal(data, &dirs) == nil {
		for _, dir := range dirs {
			w.trusted[dir] = true
		}
	}
}

// FormatTrustStatus formats trust status for display.
func FormatTrustStatus(level TrustLevel, dir string) string {
	switch level {
	case TrustTrusted:
		return "✓ Trusted: " + dir
	case TrustUntrusted:
		return "? Untrusted: " + dir
	case TrustDangerous:
		return "✗ Dangerous: " + dir
	default:
		return "Unknown: " + dir
	}
}
