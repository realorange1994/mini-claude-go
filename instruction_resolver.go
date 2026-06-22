package main

import (
	"os"
	"path/filepath"
	"strings"
)

// ─── Instruction Resolution (MiMo-Code 1) ──────────────────────────────────
//
// Hierarchical instruction file resolution that walks up the directory tree.
// Discovers AGENTS.md, CLAUDE.md, and CONTEXT.md files.
//
// MiMo-Code source: session/instruction.ts (276 lines)

// InstructionFile represents a discovered instruction file.
type InstructionFile struct {
	Path     string
	Type     string // "AGENTS", "CLAUDE", "CONTEXT"
	Content  string
	Depth    int // distance from project root
}

// InstructionResolver resolves instruction files hierarchically.
type InstructionResolver struct {
	projectDir string
	cache      map[string]*InstructionFile
}

// NewInstructionResolver creates a new instruction resolver.
func NewInstructionResolver(projectDir string) *InstructionResolver {
	return &InstructionResolver{
		projectDir: projectDir,
		cache:      make(map[string]*InstructionFile),
	}
}

// Resolve discovers instruction files starting from the given directory.
func (r *InstructionResolver) Resolve(startDir string) []*InstructionFile {
	var files []*InstructionFile
	seen := make(map[string]bool)

	dir := startDir
	for {
		// Check for instruction files
		for _, name := range []string{"AGENTS.md", "CLAUDE.md", "CONTEXT.md"} {
			path := filepath.Join(dir, name)
			if seen[path] {
				continue
			}

			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			content := string(data)
			if len(strings.TrimSpace(content)) == 0 {
				continue
			}

			files = append(files, &InstructionFile{
				Path:    path,
				Type:    strings.TrimSuffix(name, ".md"),
				Content: content,
			})
			seen[path] = true
		}

		// Walk up
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return files
}

// ResolveWithFallback resolves instruction files with fallback to CLAUDE.md.
// If AGENTS.md is sparse (< 500 chars), also loads CLAUDE.md as fallback.
func (r *InstructionResolver) ResolveWithFallback(startDir string) []*InstructionFile {
	files := r.Resolve(startDir)

	// Check if AGENTS.md is sparse
	for _, f := range files {
		if f.Type == "AGENTS" && len(f.Content) < 500 {
			// Also load CLAUDE.md if not already loaded
			claudePath := filepath.Join(filepath.Dir(f.Path), "CLAUDE.md")
			found := false
			for _, ff := range files {
				if ff.Path == claudePath {
					found = true
					break
				}
			}
			if !found {
				data, err := os.ReadFile(claudePath)
				if err == nil {
					files = append(files, &InstructionFile{
						Path:    claudePath,
						Type:    "CLAUDE",
						Content: string(data),
					})
				}
			}
		}
	}

	return files
}

// FormatInstructionFiles formats instruction files for display.
func FormatInstructionFiles(files []*InstructionFile) string {
	if len(files) == 0 {
		return "No instruction files found."
	}

	var sb string
	sb += "## Instruction Files\n\n"
	for _, f := range files {
		sb += "- **" + f.Type + "**: " + f.Path + "\n"
	}
	return sb
}
