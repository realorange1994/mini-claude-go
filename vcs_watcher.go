package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─── VCS Branch Watcher (MiMo-Code 2) ──────────────────────────────────────
//
// Monitors .git/HEAD for branch changes and publishes events.
// Keeps session metadata in sync with external git operations.
//
// MiMo-Code source: project/vcs.ts (176-189 lines)

// VCSBranchEvent represents a branch change event.
type VCSBranchEvent struct {
	OldBranch string
	NewBranch string
	Timestamp time.Time
}

// VCSBranchWatcher monitors git branch changes.
type VCSBranchWatcher struct {
	mu          sync.Mutex
	projectDir  string
	currentBranch string
	running     bool
	stopCh      chan struct{}
	callback    func(VCSBranchEvent)
}

// NewVCSBranchWatcher creates a new VCS branch watcher.
func NewVCSBranchWatcher(projectDir string) *VCSBranchWatcher {
	return &VCSBranchWatcher{
		projectDir: projectDir,
		stopCh:     make(chan struct{}),
	}
}

// Start starts watching for branch changes.
func (w *VCSBranchWatcher) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	// Get initial branch
	w.currentBranch = w.getCurrentBranch()

	// Start watcher goroutine
	go w.watch()
}

// Stop stops watching for branch changes.
func (w *VCSBranchWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		close(w.stopCh)
		w.running = false
	}
}

// SetCallback sets the callback for branch change events.
func (w *VCSBranchWatcher) SetCallback(fn func(VCSBranchEvent)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callback = fn
}

// GetCurrentBranch returns the current branch name.
func (w *VCSBranchWatcher) GetCurrentBranch() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.currentBranch
}

// watch monitors .git/HEAD for changes.
func (w *VCSBranchWatcher) watch() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.checkBranch()
		}
	}
}

// checkBranch checks if the branch has changed.
func (w *VCSBranchWatcher) checkBranch() {
	newBranch := w.getCurrentBranch()

	w.mu.Lock()
	oldBranch := w.currentBranch
	if newBranch != oldBranch && newBranch != "" {
		w.currentBranch = newBranch
		callback := w.callback
		w.mu.Unlock()

		if callback != nil {
			callback(VCSBranchEvent{
				OldBranch: oldBranch,
				NewBranch: newBranch,
				Timestamp: time.Now(),
			})
		}
	} else {
		w.mu.Unlock()
	}
}

// getCurrentBranch reads the current branch from .git/HEAD.
func (w *VCSBranchWatcher) getCurrentBranch() string {
	headPath := filepath.Join(w.projectDir, ".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}

	content := strings.TrimSpace(string(data))

	// If HEAD is a ref, extract branch name
	if strings.HasPrefix(content, "ref: refs/heads/") {
		return strings.TrimPrefix(content, "ref: refs/heads/")
	}

	// Detached HEAD
	return "detached"
}

// FormatBranchEvent formats a branch event for display.
func FormatBranchEvent(event VCSBranchEvent) string {
	return "Branch changed: " + event.OldBranch + " → " + event.NewBranch
}
