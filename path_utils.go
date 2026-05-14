package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ContainsPathTraversal checks if a path contains parent-directory references (..).
// Returns true if the path contains ".." as a path segment (not just part of a filename).
// Upstream: containsPathTraversal() in path.ts
func ContainsPathTraversal(path string) bool {
	// Check for .. preceded or followed by a path separator
	// This catches: ../foo, foo/../bar, foo/.., .., foo\..\bar
	pathTraversalRe := regexp.MustCompile(`(^|[/\\])\.\.([/\\]|$)`)
	return pathTraversalRe.MatchString(path)
}

// NormalizePathForConfigKey normalizes a path for use as a configuration key.
// Resolves dot segments, converts backslashes to forward slashes, and normalizes separators.
// Upstream: normalizePathForConfigKey() in path.ts
func NormalizePathForConfigKey(path string) string {
	// Convert backslashes to forward slashes
	result := strings.ReplaceAll(path, "\\", "/")
	// Use filepath.Clean to resolve . and .. segments, then convert back
	cleaned := filepath.Clean(result)
	// filepath.Clean may use backslashes on Windows; convert back
	return strings.ReplaceAll(cleaned, "\\", "/")
}

// ToRelativePath returns a relative path from the base directory if the path
// is inside it, or the absolute path otherwise.
// Upstream: toRelativePath() in path.ts
func ToRelativePath(absPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return absPath
	}
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return absPath
	}
	// If the relative path goes above cwd, return the absolute path instead
	if strings.HasPrefix(rel, "..") {
		return absPath
	}
	return rel
}

// GetDirectoryForPath returns the directory containing the given path.
// If the path is a directory, returns it unchanged.
// If the path is a file, returns its parent directory.
// If the path does not exist, returns its parent directory.
// Upstream: getDirectoryForPath() in path.ts
func GetDirectoryForPath(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		// Path doesn't exist; return parent directory
		return filepath.Dir(path)
	}
	if info.IsDir() {
		return path
	}
	return filepath.Dir(path)
}
