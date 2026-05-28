package resourceloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ContextFile represents a loaded context file with its path and content.
type ContextFile struct {
	Path    string
	Content string
}

// ResourceLoader loads project resources like CLAUDE.md, AGENTS.md, skills, etc.
// Aligned to TS resource-loader.ts.
type ResourceLoader struct {
	Cwd      string
	AgentDir string
}

// New creates a new resource loader.
func New(cwd, agentDir string) *ResourceLoader {
	return &ResourceLoader{
		Cwd:      cwd,
		AgentDir: agentDir,
	}
}

// contextFileCandidates are the files checked when looking for project context.
var contextFileCandidates = []string{
	"AGENTS.md",
	"AGENTS.MD",
	"CLAUDE.md",
	"CLAUDE.MD",
}

// LoadContextFileFromDir checks for context files in a directory.
// Returns the first found file's content, or nil if none found.
func (rl *ResourceLoader) LoadContextFileFromDir(dir string) *ContextFile {
	for _, name := range contextFileCandidates {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err == nil {
			return &ContextFile{
				Path:    path,
				Content: string(data),
			}
		}
	}
	return nil
}

// LoadProjectContextFiles loads all context files from the project hierarchy.
// Returns files in order: global agent dir (first), root dirs, ..., cwd (last).
// This means cwd files override root files, which override global files.
func (rl *ResourceLoader) LoadProjectContextFiles() []ContextFile {
	var files []ContextFile

	// Load from global agent dir first
	if rl.AgentDir != "" {
		if cf := rl.LoadContextFileFromDir(rl.AgentDir); cf != nil {
			files = append(files, *cf)
		}
	}

	// Walk from cwd up to root, collecting context files
	currentDir := rl.Cwd
	for {
		// Resolve to absolute path to avoid infinite loops
		absDir, err := filepath.EvalSymlinks(currentDir)
		if err != nil {
			absDir = currentDir
		}

		// Stop at root
		parent := filepath.Dir(absDir)
		if parent == absDir {
			break
		}

		if cf := rl.LoadContextFileFromDir(absDir); cf != nil {
			files = append(files, *cf)
		}

		currentDir = parent
		if currentDir == "" || currentDir == "." {
			break
		}
	}

	return files
}

// SystemPromptFileCandidates are the files checked for system prompt.
var systemPromptFileCandidates = []string{
	"SYSTEM.md",
	"SYSTEM.MD",
}

// DiscoverSystemPromptFile looks for a system prompt file.
// Checks in order: <cwd>/.<config>/SYSTEM.md, <agentDir>/SYSTEM.md
func (rl *ResourceLoader) DiscoverSystemPromptFile() *ContextFile {
	// Check agent dir first
	if rl.AgentDir != "" {
		for _, name := range systemPromptFileCandidates {
			path := filepath.Join(rl.AgentDir, name)
			data, err := os.ReadFile(path)
			if err == nil {
				return &ContextFile{
					Path:    path,
					Content: string(data),
				}
			}
		}
	}

	return nil
}

// AppendSystemPromptFileCandidates are the files checked for append system prompt.
var appendSystemPromptFileCandidates = []string{
	"APPEND_SYSTEM.md",
	"APPEND_SYSTEM.MD",
}

// DiscoverAppendSystemPromptFile looks for an append system prompt file.
func (rl *ResourceLoader) DiscoverAppendSystemPromptFile() *ContextFile {
	// Check agent dir
	if rl.AgentDir != "" {
		for _, name := range appendSystemPromptFileCandidates {
			path := filepath.Join(rl.AgentDir, name)
			data, err := os.ReadFile(path)
			if err == nil {
				return &ContextFile{
					Path:    path,
					Content: string(data),
				}
			}
		}
	}

	return nil
}

// ResolvePromptInput checks if input is a file path and loads it, or returns input as-is.
func ResolvePromptInput(input, description string) (string, error) {
	// Check if it looks like a file path
	if strings.HasPrefix(input, "./") || strings.HasPrefix(input, "/") ||
		(strings.HasSuffix(input, ".md") && len(input) < 200) {
		data, err := os.ReadFile(input)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt file %s: %w", input, err)
		}
		return string(data), nil
	}
	// Return as-is
	return input, nil
}