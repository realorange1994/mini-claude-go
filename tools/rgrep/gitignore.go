package rgrep

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GitIgnorePattern represents a single .gitignore pattern.
type GitIgnorePattern struct {
	pattern   string // the parsed pattern
	negated   bool   // ! prefix
	dirOnly   bool   // trailing /
	rooted    bool   // leading /
	baseDir   string // directory where this .gitignore was found
	raw       string // original raw pattern
}

// GitIgnoreMatcher holds all loaded .gitignore patterns and answers
// "should this path be ignored?" queries.
type GitIgnoreMatcher struct {
	patterns []GitIgnorePattern
}

// NewGitIgnoreMatcher creates an empty matcher.
func NewGitIgnoreMatcher() *GitIgnoreMatcher {
	return &GitIgnoreMatcher{}
}

// LoadGitIgnore reads and parses a .gitignore file, adding its patterns.
// baseDir is the directory containing the .gitignore (used for rooted patterns).
func (g *GitIgnoreMatcher) LoadGitIgnore(path, baseDir string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		pat := parseGitIgnoreLine(line, baseDir)
		if pat != nil {
			g.patterns = append(g.patterns, *pat)
		}
	}
	return scanner.Err()
}

// LoadGitIgnoreFromDir loads .gitignore from the given directory if it exists.
func (g *GitIgnoreMatcher) LoadGitIgnoreFromDir(dir string) error {
	gitignorePath := filepath.Join(dir, ".gitignore")
	info, err := os.Stat(gitignorePath)
	if err != nil || info.IsDir() {
		return nil
	}
	return g.LoadGitIgnore(gitignorePath, dir)
}

// IsIgnored checks whether a path should be ignored.
// relPath is relative to the search root. isDir indicates if the path is a directory.
func (g *GitIgnoreMatcher) IsIgnored(relPath string, isDir bool) bool {
	// Normalize: forward slashes, no leading ./
	relPath = filepath.ToSlash(relPath)
	relPath = strings.TrimPrefix(relPath, "./")

	ignored := false
	for _, pat := range g.patterns {
		// Dir-only patterns only match directories
		if pat.dirOnly && !isDir {
			continue
		}

		if matchGitIgnorePattern(pat, relPath, isDir) {
			ignored = !pat.negated
		}
	}
	return ignored
}

// parseGitIgnoreLine parses a single .gitignore line into a pattern.
// Returns nil for blank lines and comments.
func parseGitIgnoreLine(line, baseDir string) *GitIgnorePattern {
	// Trim trailing whitespace (but not leading — leading space is significant)
	line = strings.TrimRight(line, " \t")
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}

	pat := &GitIgnorePattern{
		raw:     line,
		baseDir: baseDir,
	}

	// Handle negation
	if strings.HasPrefix(line, "!") {
		pat.negated = true
		line = line[1:]
	}

	// Handle trailing slash (dir-only)
	if strings.HasSuffix(line, "/") {
		pat.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	// Handle leading slash (rooted)
	if strings.HasPrefix(line, "/") {
		pat.rooted = true
		line = line[1:]
	}

	// Convert the gitignore glob into a Go-friendly pattern
	pat.pattern = convertGitIgnoreGlob(line, pat.rooted)
	return pat
}

// convertGitIgnoreGlob converts a gitignore glob pattern into a
// pattern suitable for filepath.Match or doublestar matching.
func convertGitIgnoreGlob(pattern string, rooted bool) string {
	// Gitignore patterns without a slash match anywhere in the path.
	// Patterns with a slash are relative to the .gitignore location.
	// We handle this in matchGitIgnorePattern instead of here.
	return pattern
}

// matchGitIgnorePattern checks if a path matches a gitignore pattern.
func matchGitIgnorePattern(pat GitIgnorePattern, relPath string, isDir bool) bool {
	pattern := pat.pattern

	// If the pattern contains a slash, it's relative to the baseDir.
	// If not, it can match any path component.
	hasSlash := strings.Contains(pattern, "/")

	if pat.rooted || hasSlash {
		// Rooted or slash-containing patterns match from the base directory
		return matchGlob(pattern, relPath)
	}

	// Pattern without slash: match against any path component
	// e.g., "*.o" matches "foo/bar.o"
	parts := strings.Split(relPath, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if matchGlob(pattern, parts[i]) {
			return true
		}
		// For directory patterns, also try matching the directory path prefix
		if isDir && i > 0 {
			dirPath := strings.Join(parts[:i+1], "/")
			if matchGlob(pattern, dirPath) {
				return true
			}
		}
	}
	return false
}

// matchGlob does glob matching with support for **, ?, and char ranges.
// This is a simplified implementation that handles the most common patterns.
func matchGlob(pattern, name string) bool {
	// Fast path: exact match
	if pattern == name {
		return true
	}

	// Use filepath.Match for simple patterns (no **)
	if !strings.Contains(pattern, "**") {
		matched, err := filepath.Match(pattern, name)
		if err == nil && matched {
			return true
		}
		// Also try matching against just the last component
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			matched, err = filepath.Match(pattern, name[idx+1:])
			if err == nil && matched {
				return true
			}
		}
		return false
	}

	// Handle ** patterns by splitting on "**"
	return matchDoublestar(pattern, name)
}

// matchDoublestar handles ** glob patterns.
// ** matches zero or more path components.
func matchDoublestar(pattern, name string) bool {
	// Split pattern on "**"
	parts := strings.SplitN(pattern, "**", 2)
	if len(parts) != 2 {
		return false
	}

	prefix := parts[0]
	suffix := parts[1]

	// Remove trailing/leading slashes from parts
	prefix = strings.TrimSuffix(prefix, "/")
	suffix = strings.TrimPrefix(suffix, "/")

	if prefix == "" && suffix == "" {
		return true
	}

	// Try all possible splits of name where ** could expand
	nameParts := strings.Split(name, "/")
	for i := 0; i <= len(nameParts); i++ {
		var prefixName, suffixName string
		if i > 0 {
			prefixName = strings.Join(nameParts[:i], "/")
		}
		if i < len(nameParts) {
			suffixName = strings.Join(nameParts[i:], "/")
		}

		prefixOK := prefix == ""
		if prefix != "" {
			m, err := filepath.Match(prefix, prefixName)
			prefixOK = err == nil && m
		}

		if !prefixOK {
			continue
		}

		suffixOK := suffix == ""
		if suffix != "" {
			if strings.Contains(suffix, "**") {
				suffixOK = matchDoublestar(suffix, suffixName)
			} else {
				m, err := filepath.Match(suffix, suffixName)
				suffixOK = err == nil && m
			}
		}

		if prefixOK && suffixOK {
			return true
		}
	}

	return false
}

// LoadGitIgnoreFromRepoRoot loads .gitignore from the git repo root
// (the directory containing .git/).
func LoadGitIgnoreFromRepoRoot(searchRoot string) *GitIgnoreMatcher {
	matcher := NewGitIgnoreMatcher()

	// Walk up from searchRoot to find .git directory
	dir := searchRoot
	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			// Found repo root. Load .gitignore from root and all parent dirs
			// up to searchRoot.
			loadGitIgnoreChain(matcher, searchRoot, dir)
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root, no git repo found
			break
		}
		dir = parent
	}

	return matcher
}

// loadGitIgnoreChain loads .gitignore files from searchRoot up to repoRoot.
// Git applies .gitignore patterns from all levels, with deeper .gitignore
// files taking precedence (later patterns override earlier ones).
func loadGitIgnoreChain(matcher *GitIgnoreMatcher, searchRoot, repoRoot string) {
	// Collect directories from searchRoot up to repoRoot
	var dirs []string
	dir := searchRoot
	for {
		dirs = append(dirs, dir)
		if dir == repoRoot {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Load from repo root first (lowest priority), then deeper dirs (higher priority)
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = matcher.LoadGitIgnoreFromDir(dirs[i])
	}
}

// FindGitRepoRoot walks up from dir to find the .git directory.
// Returns the repo root or empty string if not found.
func FindGitRepoRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if info, err := os.Stat(filepath.Join(abs, ".git")); err == nil && info.IsDir() {
			return abs
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return ""
		}
		abs = parent
	}
}

// FormatGitIgnoreDebug returns a debug string showing loaded patterns.
func (g *GitIgnoreMatcher) FormatGitIgnoreDebug() string {
	if len(g.patterns) == 0 {
		return "no .gitignore patterns loaded"
	}
	return fmt.Sprintf("%d .gitignore patterns loaded", len(g.patterns))
}
