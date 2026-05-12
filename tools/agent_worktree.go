package tools

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// WorktreeConfig specifies worktree isolation settings for a sub-agent.
type WorktreeConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Name    string `json:"name,omitempty"` // worktree name (auto-generated if empty)
	Keep    bool   `json:"keep,omitempty"` // keep worktree after agent completes
}

// WorktreeResult holds the result of setting up a worktree.
type WorktreeResult struct {
	Path    string
	Cleanup func() error
}

// SetupWorktree creates a git worktree for isolated agent execution.
// Returns the worktree path and a cleanup function.
// If cfg.Enabled is false, returns an empty path with a no-op cleanup.
func SetupWorktree(cfg WorktreeConfig) (worktreePath string, cleanup func() error, err error) {
	if !cfg.Enabled {
		return "", func() error { return nil }, nil
	}

	// Generate worktree name
	name := cfg.Name
	if name == "" {
		name = fmt.Sprintf("agent-%s", uuidV4Short())
	}

	// Create worktree directory path
	worktreeDir := filepath.Join(".claude", "worktrees", name)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(worktreeDir), 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create worktree directory: %w", err)
	}

	// Create worktree: git worktree add <path>
	cmd := exec.Command("git", "worktree", "add", worktreeDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}

	// Cleanup function
	cleanup = func() error {
		if cfg.Keep {
			return nil
		}
		// git worktree remove --force <path>
		cmd := exec.Command("git", "worktree", "remove", "--force", worktreeDir)
		return cmd.Run()
	}

	return worktreeDir, cleanup, nil
}

// uuidV4Short returns a short hex string suitable for unique naming.
func uuidV4Short() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return fmt.Sprintf("%08x", b)
}
