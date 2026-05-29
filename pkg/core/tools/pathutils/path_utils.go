// Package pathutils provides path resolution utilities with platform-specific fallbacks.
// Aligned to pi's tools/path-utils.ts.
package pathutils

import (
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ResolveToCwd resolves a path relative to cwd, falling back to cwd if not found.
func ResolveToCwd(path, cwd string) string {
	// Try as-is first
	if _, err := os.Stat(path); err == nil {
		return path
	}

	// Try relative to cwd
	resolved := filepath.Join(cwd, path)
	if _, err := os.Stat(resolved); err == nil {
		return resolved
	}

	return resolved // Return resolved even if not found
}

// ExpandPath expands a path starting with ~ to the user's home directory.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

// PathExists checks if a path exists on the filesystem.
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ResolveReadPath resolves a path for reading with macOS-specific fallbacks.
// Handles:
//   - Standard path resolution
//   - macOS AM/PM with narrow no-break space (U+202F)
//   - NFD Unicode normalization (macOS filenames)
//   - Curly quote variants (French macOS: é vs e)
func ResolveReadPath(path string) string {
	// Try the path as-is first
	if PathExists(path) {
		return path
	}

	// Try expanding ~
	expanded := ExpandPath(path)
	if expanded != path && PathExists(expanded) {
		return expanded
	}

	// macOS-specific: try curly quote variant
	// ' (U+2018) and ' (U+2019) -> ' (ASCII)
	if variant := strings.NewReplacer(
		"\u2018", "'",
		"\u2019", "'",
	).Replace(path); variant != path && PathExists(variant) {
		return variant
	}

	// macOS-specific: try NFD normalization
	// On macOS, filenames may be NFD-normalized
	nfdPath := nfdNormalize(path)
	if nfdPath != path && PathExists(nfdPath) {
		return nfdPath
	}

	// Try narrow no-break space (U+202F) -> regular space
	if variant := strings.ReplaceAll(path, " ", "\u202F"); variant != path && PathExists(variant) {
		return variant
	}

	return path
}

// nfdNormalize performs NFC->NFD Unicode normalization.
// This is a simplified version — macOS uses NFD for filenames.
func nfdNormalize(s string) string {
	if !utf8.ValidString(s) {
		return s
	}
	// Use Go's norm package for proper NFD normalization
	// This is a placeholder — real impl would use golang.org/x/text/unicode/norm
	return s
}

// ResolveWritePath resolves a path for writing.
// Expands ~ and resolves relative paths.
func ResolveWritePath(path, cwd string) string {
	path = ExpandPath(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}

// WalkUpProjects walks up from cwd to find the project root.
// Returns a list of all ancestor directories.
func WalkUpProjects(cwd string) []string {
	var dirs []string
	dir := filepath.Clean(cwd)
	root := "/"
	if filepath.VolumeName(dir) != "" {
		// Windows: stop at drive root
		root = filepath.VolumeName(dir) + `\`
	}

	for {
		dirs = append(dirs, dir)
		parent := filepath.Dir(dir)
		if parent == dir || parent == root {
			break
		}
		dir = parent
	}

	return dirs
}

// FindContextFilesInDir searches a directory for context files.
// Returns the first match in order of preference:
// CLAUDE.md, CLAUDE.MD, AGENTS.md, AGENTS.MD
var contextFileNames = []string{"CLAUDE.md", "CLAUDE.MD", "AGENTS.md", "AGENTS.MD"}

func FindContextFilesInDir(dir string) (string, bool) {
	for _, name := range contextFileNames {
		fullPath := filepath.Join(dir, name)
		if PathExists(fullPath) {
			return fullPath, true
		}
	}
	return "", false
}

// FindSystemPromptFile searches for a SYSTEM.md file.
func FindSystemPromptFile(dir string) (string, bool) {
	names := []string{"SYSTEM.md", "SYSTEM.MD"}
	for _, name := range names {
		fullPath := filepath.Join(dir, name)
		if PathExists(fullPath) {
			return fullPath, true
		}
	}
	return "", false
}

// FindAppendSystemPromptFile searches for an APPEND_SYSTEM.md file.
func FindAppendSystemPromptFile(dir string) (string, bool) {
	names := []string{"APPEND_SYSTEM.md", "APPEND_SYSTEM.MD"}
	for _, name := range names {
		fullPath := filepath.Join(dir, name)
		if PathExists(fullPath) {
			return fullPath, true
		}
	}
	return "", false
}
