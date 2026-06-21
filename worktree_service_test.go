package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorktreeService_New(t *testing.T) {
	dir := t.TempDir()
	s := NewWorktreeService(dir)
	if s == nil {
		t.Error("expected non-nil service")
	}
}

func TestWorktreeService_Create(t *testing.T) {
	// Initialize a git repo
	dir := t.TempDir()
	exec.Command("git", "init").Run()
	exec.Command("git", "config", "user.email", "test@test.com").Run()
	exec.Command("git", "config", "user.name", "Test").Run()

	// Create initial commit
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("test"), 0644)
	exec.Command("git", "add", ".").Run()
	exec.Command("git", "commit", "-m", "initial").Run()

	s := NewWorktreeService(dir)
	worktree, err := s.Create("test-worktree")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if worktree.Path == "" {
		t.Error("expected non-empty path")
	}
	if worktree.Branch != "mimocode/test-worktree" {
		t.Errorf("expected 'mimocode/test-worktree', got %q", worktree.Branch)
	}
}

func TestWorktreeService_List(t *testing.T) {
	dir := t.TempDir()
	s := NewWorktreeService(dir)

	worktrees := s.List()
	if len(worktrees) != 0 {
		t.Errorf("expected 0 worktrees, got %d", len(worktrees))
	}
}

func TestWorktreeService_Get(t *testing.T) {
	dir := t.TempDir()
	s := NewWorktreeService(dir)

	worktree := s.Get("nonexistent")
	if worktree != nil {
		t.Error("expected nil for nonexistent worktree")
	}
}

func TestWorktreeService_Remove_NotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewWorktreeService(dir)

	err := s.Remove("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent worktree")
	}
}

func TestWorktreeService_Reset_NotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewWorktreeService(dir)

	err := s.Reset("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent worktree")
	}
}

func TestWorktreeService_Head_NotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewWorktreeService(dir)

	_, err := s.Head("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent worktree")
	}
}

func TestWorktreeService_IsPristine_NotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewWorktreeService(dir)

	_, err := s.IsPristine("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent worktree")
	}
}

func TestFormatWorktreeList(t *testing.T) {
	worktrees := []*Worktree{
		{Path: "/path/to/worktree", Branch: "main", Head: "abc12345def", Pristine: true},
	}

	output := FormatWorktreeList(worktrees)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatWorktreeList_Empty(t *testing.T) {
	output := FormatWorktreeList(nil)
	if output != "No worktrees found." {
		t.Errorf("expected 'No worktrees found.', got %q", output)
	}
}
