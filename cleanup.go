package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CleanupManager handles periodic cleanup of stale session files.
// It removes old transcripts, session memory files, and plan files
// beyond a configurable cutoff (default: 30 days).
type CleanupManager struct {
	cutoffDays int
	projectDir string
}

// NewCleanupManager creates a cleanup manager with a 30-day default cutoff.
func NewCleanupManager(projectDir string) *CleanupManager {
	return &CleanupManager{
		cutoffDays: 30,
		projectDir: projectDir,
	}
}

// SetCutoff changes the retention period in days.
func (c *CleanupManager) SetCutoff(days int) {
	if days > 0 {
		c.cutoffDays = days
	}
}

// Run performs cleanup of stale files in .claude/ directories.
func (c *CleanupManager) Run() (int, error) {
	cutoff := time.Now().AddDate(0, 0, -c.cutoffDays)
	removed := 0

	dirs := []string{
		filepath.Join(c.projectDir, ".claude", "transcripts"),
		filepath.Join(c.projectDir, ".claude", "plans"),
		filepath.Join(c.projectDir, ".claude", "sessions"),
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // directory may not exist
		}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				path := filepath.Join(dir, e.Name())
				if err := os.Remove(path); err == nil {
					removed++
				}
			}
		}
	}

	// Clean stale .bak files (notebook backups)
	bakFiles, _ := filepath.Glob(filepath.Join(c.projectDir, "**", "*.bak"))
	for _, f := range bakFiles {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(f)
			removed++
		}
	}

	// Clean stale .tmp files (atomic write leftovers)
	tmpFiles, _ := filepath.Glob(filepath.Join(c.projectDir, "**", "*.tmp.*"))
	for _, f := range tmpFiles {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		// Temp files older than 1 day are stale
		if info.ModTime().Before(time.Now().AddDate(0, 0, -1)) {
			os.Remove(f)
			removed++
		}
	}

	return removed, nil
}

// handleCleanup handles the /cleanup slash command.
func handleCleanup(projectDir string, args []string) {
	cm := NewCleanupManager(projectDir)

	if len(args) > 0 {
		if days, err := fmt.Sscanf(args[0], "%d", &cm.cutoffDays); err != nil || days != 1 {
			fmt.Printf("Invalid cutoff: %s (expected number of days)\n", args[0])
			return
		}
	}

	removed, err := cm.Run()
	if err != nil {
		fmt.Printf("Cleanup error: %v\n", err)
		return
	}
	if removed == 0 {
		fmt.Println("No stale files found.")
	} else {
		fmt.Printf("Cleaned up %d stale files (cutoff: %d days).\n", removed, cm.cutoffDays)
	}
}

// isStaleFile checks if a file is older than the given cutoff.
func isStaleFile(path string, cutoff time.Time) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.ModTime().Before(cutoff)
}

// registerCleanupCommand adds /cleanup to the known commands list.
func init() {
	// This is handled in main.go's isKnownCmd check
}

// CleanupStaleTempFiles removes .tmp.* files older than 1 day in the project directory.
// Called automatically at startup to clean up leftover atomic write temp files.
func CleanupStaleTempFiles(projectDir string) int {
	tmpFiles, _ := filepath.Glob(filepath.Join(projectDir, "*.tmp.*"))
	removed := 0
	for _, f := range tmpFiles {
		if isStaleFile(f, time.Now().AddDate(0, 0, -1)) {
			if os.Remove(f) == nil {
				removed++
			}
		}
	}
	// Also check subdirectories (limited depth)
	subTmpFiles, _ := filepath.Glob(filepath.Join(projectDir, "*", "*.tmp.*"))
	for _, f := range subTmpFiles {
		if isStaleFile(f, time.Now().AddDate(0, 0, -1)) {
			if strings.Contains(f, ".claude") || strings.Contains(f, "node_modules") {
				continue // skip internal directories
			}
			if os.Remove(f) == nil {
				removed++
			}
		}
	}
	return removed
}
