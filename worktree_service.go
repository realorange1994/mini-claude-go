package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// ─── Git Worktree Service (MiMo-Code 3) ─────────────────────────────────────
//
// Full lifecycle management for git worktrees.
// Create, remove, reset, and inspect worktrees for isolated agent workspaces.
//
// MiMo-Code source: worktree/index.ts (614 lines)

// Worktree represents a git worktree.
type Worktree struct {
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	Head      string `json:"head"`
	Pristine  bool   `json:"pristine"`
}

// WorktreeService manages git worktrees.
type WorktreeService struct {
	mu          sync.Mutex
	projectDir  string
	worktreeDir string
	worktrees   map[string]*Worktree
}

// NewWorktreeService creates a new worktree service.
func NewWorktreeService(projectDir string) *WorktreeService {
	return &WorktreeService{
		projectDir:  projectDir,
		worktreeDir: filepath.Join(projectDir, ".claude", "worktrees"),
		worktrees:   make(map[string]*Worktree),
	}
}

// Create creates a new worktree with an auto-named branch.
func (s *WorktreeService) Create(slug string) (*Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate branch name
	branch := fmt.Sprintf("mimocode/%s", slug)

	// Create worktree directory
	worktreePath := filepath.Join(s.worktreeDir, slug)
	if err := os.MkdirAll(s.worktreeDir, 0755); err != nil {
		return nil, fmt.Errorf("create worktree dir: %w", err)
	}

	// Create worktree
	cmd := exec.Command("git", "worktree", "add", "-b", branch, worktreePath)
	cmd.Dir = s.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("create worktree: %s: %w", string(output), err)
	}

	worktree := &Worktree{
		Path:   worktreePath,
		Branch: branch,
	}

	s.worktrees[slug] = worktree
	return worktree, nil
}

// Remove removes a worktree.
func (s *WorktreeService) Remove(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	worktree, exists := s.worktrees[slug]
	if !exists {
		return fmt.Errorf("worktree not found: %s", slug)
	}

	// Remove worktree
	cmd := exec.Command("git", "worktree", "remove", worktree.Path, "--force")
	cmd.Dir = s.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove worktree: %s: %w", string(output), err)
	}

	// Clean up branch
	exec.Command("git", "branch", "-D", worktree.Branch).Run()

	delete(s.worktrees, slug)
	return nil
}

// Reset resets a worktree to the default branch.
func (s *WorktreeService) Reset(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	worktree, exists := s.worktrees[slug]
	if !exists {
		return fmt.Errorf("worktree not found: %s", slug)
	}

	// Get default branch
	defaultBranch, err := s.getDefaultBranch()
	if err != nil {
		return fmt.Errorf("get default branch: %w", err)
	}

	// Reset to default branch
	cmd := exec.Command("git", "checkout", defaultBranch)
	cmd.Dir = worktree.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("checkout: %s: %w", string(output), err)
	}

	// Hard reset
	cmd = exec.Command("git", "reset", "--hard", "HEAD")
	cmd.Dir = worktree.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("reset: %s: %w", string(output), err)
	}

	return nil
}

// Head returns the HEAD commit hash of a worktree.
func (s *WorktreeService) Head(slug string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	worktree, exists := s.worktrees[slug]
	if !exists {
		return "", fmt.Errorf("worktree not found: %s", slug)
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = worktree.Path
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get HEAD: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// IsPristine checks if a worktree has no uncommitted changes.
func (s *WorktreeService) IsPristine(slug string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	worktree, exists := s.worktrees[slug]
	if !exists {
		return false, fmt.Errorf("worktree not found: %s", slug)
	}

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = worktree.Path
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}

	return strings.TrimSpace(string(output)) == "", nil
}

// List returns all worktrees.
func (s *WorktreeService) List() []*Worktree {
	s.mu.Lock()
	defer s.mu.Unlock()

	var worktrees []*Worktree
	for _, w := range s.worktrees {
		worktrees = append(worktrees, w)
	}
	return worktrees
}

// Get returns a worktree by slug.
func (s *WorktreeService) Get(slug string) *Worktree {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.worktrees[slug]
}

// getDefaultBranch returns the default branch name.
func (s *WorktreeService) getDefaultBranch() (string, error) {
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	cmd.Dir = s.projectDir
	output, err := cmd.Output()
	if err != nil {
		// Fallback to main/master
		return "main", nil
	}

	branch := strings.TrimSpace(string(output))
	branch = strings.TrimPrefix(branch, "origin/")
	return branch, nil
}

// FormatWorktreeList formats a list of worktrees for display.
func FormatWorktreeList(worktrees []*Worktree) string {
	if len(worktrees) == 0 {
		return "No worktrees found."
	}

	var sb string
	sb += fmt.Sprintf("## Worktrees (%d found)\n\n", len(worktrees))

	for _, w := range worktrees {
		pristine := "✓"
		if !w.Pristine {
			pristine = "✗"
		}
		sb += fmt.Sprintf("- `%s` (%s) [%s] HEAD: %s\n", w.Path, w.Branch, pristine, w.Head[:8])
	}

	return sb
}
