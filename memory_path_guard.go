package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ─── Memory Path Guard (MiMo-Code 5) ───────────────────────────────────────
//
// Enforces strict write-path authority over the memory tree.
// Prevents memory corruption in multi-agent scenarios.
//
// MiMo-Code source: tool/memory-path-guard.ts (162 lines)

// MemoryPathGuard enforces memory write permissions.
type MemoryPathGuard struct {
	projectDir     string
	allowedPaths   map[string]bool
	reservedPaths  map[string]bool
}

// NewMemoryPathGuard creates a new memory path guard.
func NewMemoryPathGuard(projectDir string) *MemoryPathGuard {
	return &MemoryPathGuard{
		projectDir: projectDir,
		allowedPaths: map[string]bool{
			"MEMORY.md":           true,
			"checkpoint.md":       true,
			"notes.md":            true,
			"checkpoint.json":     true,
		},
		reservedPaths: map[string]bool{
			"tasks/": true,
		},
	}
}

// IsMemoryPath checks if a path is within the memory tree.
func (g *MemoryPathGuard) IsMemoryPath(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, ".claude/memory/") ||
		strings.Contains(normalized, ".claude/session_memory.md") ||
		strings.Contains(normalized, ".claude/checkpoints/")
}

// IsCheckpointWriterAllowed checks if a path is allowed for checkpoint writer.
func (g *MemoryPathGuard) IsCheckpointWriterAllowed(path string) bool {
	if !g.IsMemoryPath(path) {
		return false
	}

	normalized := filepath.ToSlash(path)
	base := filepath.Base(path)

	// Check allowed paths
	if g.allowedPaths[base] {
		return true
	}

	// Check task progress files
	if strings.Contains(normalized, "/tasks/") && strings.HasSuffix(base, ".md") {
		return true
	}

	return false
}

// IsReservedForCheckpointWriter checks if a path is reserved for checkpoint writer.
func (g *MemoryPathGuard) IsReservedForCheckpointWriter(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "/tasks/")
}

// IsMainAgentAllowed checks if a path is allowed for main agent.
func (g *MemoryPathGuard) IsMainAgentAllowed(path string) bool {
	if !g.IsMemoryPath(path) {
		return true // Non-memory paths are allowed
	}

	// Main agent cannot write to tasks directory
	if g.IsReservedForCheckpointWriter(path) {
		return false
	}

	return true
}

// IsSubagentAllowed checks if a path is allowed for a subagent.
func (g *MemoryPathGuard) IsSubagentAllowed(path string, agentID string) bool {
	if !g.IsMemoryPath(path) {
		return true
	}

	// Subagent can only write to its own task directory
	normalized := filepath.ToSlash(path)
	if strings.Contains(normalized, "/tasks/") {
		// Check if it's the agent's own task
		expectedPrefix := fmt.Sprintf("/tasks/%s/", agentID)
		return strings.Contains(normalized, expectedPrefix)
	}

	return false
}

// ValidateMemoryWrite validates a memory write operation.
func (g *MemoryPathGuard) ValidateMemoryWrite(path string, agentType string, agentID string) error {
	if !g.IsMemoryPath(path) {
		return nil // Not a memory path, allow
	}

	switch agentType {
	case "checkpoint-writer":
		if !g.IsCheckpointWriterAllowed(path) {
			return fmt.Errorf("checkpoint writer not allowed to write: %s", path)
		}
	case "main":
		if !g.IsMainAgentAllowed(path) {
			return fmt.Errorf("main agent not allowed to write: %s", path)
		}
	case "subagent":
		if !g.IsSubagentAllowed(path, agentID) {
			return fmt.Errorf("subagent %s not allowed to write: %s", agentID, path)
		}
	}

	return nil
}

// FormatMemoryPathError formats an error message for memory path violations.
func FormatMemoryPathError(path string, agentType string, agentID string) string {
	switch agentType {
	case "checkpoint-writer":
		return fmt.Sprintf("Memory path %s is not in checkpoint writer allowlist. Allowed: MEMORY.md, checkpoint.md, notes.md, tasks/<TID>/*.md", path)
	case "main":
		return fmt.Sprintf("Memory path %s is reserved for checkpoint writer. Main agent cannot write to tasks/", path)
	case "subagent":
		return fmt.Sprintf("Memory path %s is not in subagent %s's task directory", path, agentID)
	default:
		return fmt.Sprintf("Memory path %s is not allowed for %s", path, agentType)
	}
}
