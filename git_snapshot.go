package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─── Git-Based Snapshot System (MiMo-Code 6) ───────────────────────────────
//
// Maintains a separate git repository that tracks the working tree state
// independently of the user's git. Uses git plumbing commands for fast,
// space-efficient snapshot/restore with proper diff support.
//
// MiMo-Code source: snapshot/index.ts (777 lines)

// Snapshot represents a point-in-time snapshot of the working tree.
type Snapshot struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash"` // git tree hash
	Timestamp time.Time `json:"timestamp"`
	Files     []string  `json:"files"`
	Message   string    `json:"message"`
}

// SnapshotManager manages git-based snapshots.
type SnapshotManager struct {
	mu          sync.Mutex
	snapshotDir string
	repoDir     string // .git directory for snapshots
	workDir     string // working directory being tracked
}

// NewSnapshotManager creates a new snapshot manager.
func NewSnapshotManager(workDir string) *SnapshotManager {
	snapshotDir := filepath.Join(workDir, ".claude", "snapshots")
	return &SnapshotManager{
		snapshotDir: snapshotDir,
		repoDir:     filepath.Join(snapshotDir, ".git"),
		workDir:     workDir,
	}
}

// Init initializes the snapshot repository.
func (m *SnapshotManager) Init() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create snapshot directory
	if err := os.MkdirAll(m.snapshotDir, 0755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}

	// Initialize git repo if not exists
	gitDir := filepath.Join(m.snapshotDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := m.runGitInDir(m.snapshotDir, "init"); err != nil {
			return fmt.Errorf("init git repo: %w", err)
		}
	}

	return nil
}

// Track adds files to the snapshot index.
func (m *SnapshotManager) Track(files []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, file := range files {
		fullPath := filepath.Join(m.workDir, file)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		// Copy file to snapshot dir
		snapPath := filepath.Join(m.snapshotDir, file)
		os.MkdirAll(filepath.Dir(snapPath), 0755)

		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		os.WriteFile(snapPath, data, 0644)

		// Add to git index
		m.runGitInDir(m.snapshotDir, "add", file)
	}

	return nil
}

// Create creates a new snapshot.
func (m *SnapshotManager) Create(message string) (*Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Commit current state
	if err := m.runGitInDir(m.snapshotDir, "commit", "-m", message, "--allow-empty"); err != nil {
		return nil, fmt.Errorf("commit snapshot: %w", err)
	}

	// Get commit hash
	hash, err := m.runGitOutput(m.snapshotDir, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("get hash: %w", err)
	}

	// Get tracked files
	files, err := m.runGitOutput(m.snapshotDir, "ls-files")
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}

	snapshot := &Snapshot{
		ID:        fmt.Sprintf("snap-%s", time.Now().Format("20060102-150405")),
		Hash:      strings.TrimSpace(hash),
		Timestamp: time.Now(),
		Files:     strings.Split(strings.TrimSpace(files), "\n"),
		Message:   message,
	}

	return snapshot, nil
}

// Diff computes the diff between two snapshots.
func (m *SnapshotManager) Diff(fromHash, toHash string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	diff, err := m.runGitOutput(m.snapshotDir, "diff", fromHash, toHash)
	if err != nil {
		return "", fmt.Errorf("compute diff: %w", err)
	}

	return diff, nil
}

// Restore restores files from a snapshot.
func (m *SnapshotManager) Restore(hash string, files []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, file := range files {
		// Get file content from snapshot
		content, err := m.runGitOutput(m.snapshotDir, "show", hash+":"+file)
		if err != nil {
			continue
		}

		// Write to working directory
		fullPath := filepath.Join(m.workDir, file)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, []byte(content), 0644)
	}

	return nil
}

// Revert reverts changes from a specific snapshot.
func (m *SnapshotManager) Revert(hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get files changed in this snapshot
	diff, err := m.runGitOutput(m.snapshotDir, "diff", hash+"~1", hash, "--name-only")
	if err != nil {
		return fmt.Errorf("get changed files: %w", err)
	}

	files := strings.Split(strings.TrimSpace(diff), "\n")
	for _, file := range files {
		if file == "" {
			continue
		}

		// Get previous content
		content, err := m.runGitOutput(m.snapshotDir, "show", hash+"~1:"+file)
		if err != nil {
			// File was added in this snapshot, delete it
			os.Remove(filepath.Join(m.workDir, file))
			continue
		}

		// Restore previous content
		fullPath := filepath.Join(m.workDir, file)
		os.WriteFile(fullPath, []byte(content), 0644)
	}

	return nil
}

// List returns all snapshots.
func (m *SnapshotManager) List() ([]Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	log, err := m.runGitOutput(m.snapshotDir, "log", "--oneline", "--format=%H %s")
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	var snapshots []Snapshot
	for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		snapshots = append(snapshots, Snapshot{
			Hash:    parts[0],
			Message: parts[1],
		})
	}

	return snapshots, nil
}

// Cleanup removes old snapshots.
func (m *SnapshotManager) Cleanup(keepDays int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Run git gc with pruning
	if err := m.runGitInDir(m.snapshotDir, "gc", fmt.Sprintf("--prune=%d.days", keepDays)); err != nil {
		return 0, fmt.Errorf("gc: %w", err)
	}

	return 0, nil
}

// runGit runs a git command in the snapshot directory.
func (m *SnapshotManager) runGit(args ...string) error {
	return m.runGitInDir(m.snapshotDir, args...)
}

// runGitInDir runs a git command in the specified directory.
func (m *SnapshotManager) runGitInDir(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), string(output), err)
	}
	return nil
}

// runGitOutput runs a git command and returns its output.
func (m *SnapshotManager) runGitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}

// FormatSnapshotList formats a list of snapshots for display.
func FormatSnapshotList(snapshots []Snapshot) string {
	if len(snapshots) == 0 {
		return "No snapshots found."
	}

	var sb string
	sb += fmt.Sprintf("## Snapshots (%d found)\n\n", len(snapshots))

	for i, s := range snapshots {
		if i >= 10 {
			sb += fmt.Sprintf("\n... and %d more\n", len(snapshots)-10)
			break
		}
		sb += fmt.Sprintf("- `%s`: %s\n", s.Hash[:8], s.Message)
	}

	return sb
}
