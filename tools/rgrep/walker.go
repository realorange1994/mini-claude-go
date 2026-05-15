package rgrep

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Ignored directory names that are always skipped (VCS + common large dirs).
var defaultIgnoredDirs = map[string]bool{
	".git":          true,
	".svn":          true,
	".hg":           true,
	".bzr":          true,
	".jj":           true,
	".sl":           true,
	"node_modules":  true,
	"__pycache__":   true,
	".venv":         true,
	"venv":          true,
	".tox":          true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".ruff_cache":   true,
	".coverage":     true,
	"htmlcov":       true,
}

// WalkEntry represents a file found during directory traversal.
type WalkEntry struct {
	Path    string // absolute path
	RelPath string // relative to search root
	Info    os.FileInfo
}

// WalkOptions controls directory traversal.
type WalkOptions struct {
	Root          string   // root directory to walk
	MaxDepth      int      // max depth (0 = unlimited)
	Globs          []string // glob filters for filenames (multiple OR)
	TypeFilter    string   // language type filter
	Excludes      []string // additional exclude patterns
	RespectGitIgnore bool   // respect .gitignore
	Ctx              context.Context
	MaxFilesize      int64  // max file size in bytes (0 = unlimited)
}

// WalkDir traverses a directory tree and returns matching file entries.
// It respects .gitignore (if enabled), skips VCS/common dirs, and applies
// glob/type/exclude filters.
func WalkDir(opts WalkOptions) ([]WalkEntry, error) {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}

	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, err
	}

	// Load .gitignore if requested
	var gitignore *GitIgnoreMatcher
	if opts.RespectGitIgnore {
		gitignore = LoadGitIgnoreFromRepoRoot(root)
	}

	// Resolve type filter extensions
	var typeExts []string
	if opts.TypeFilter != "" {
		typeExts = ExtensionsForType(strings.ToLower(opts.TypeFilter))
	}

	var entries []WalkEntry
	rootDepth := strings.Count(filepath.Clean(root), string(filepath.Separator))

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip errors, keep walking
		}

		// Check context cancellation
		select {
		case <-opts.Ctx.Done():
			return opts.Ctx.Err()
		default:
		}

		rel, _ := filepath.Rel(root, path)

		if d.IsDir() {
			// Skip default ignored directories
			if defaultIgnoredDirs[d.Name()] {
				return filepath.SkipDir
			}

			// Skip .gitignore-ignored directories
			if gitignore != nil && gitignore.IsIgnored(rel, true) {
				return filepath.SkipDir
			}

			// Skip excluded directories
			for _, excl := range opts.Excludes {
				if matched, _ := filepath.Match(excl, d.Name()); matched {
					return filepath.SkipDir
				}
			}

			// Enforce max depth
			if opts.MaxDepth > 0 {
				curDepth := strings.Count(filepath.Clean(path), string(filepath.Separator)) - rootDepth
				if curDepth >= opts.MaxDepth {
					return filepath.SkipDir
				}
			}

			return nil
		}

		// It's a file

		// Skip .gitignore-ignored files
		if gitignore != nil && gitignore.IsIgnored(rel, false) {
			return nil
		}

		// Skip excluded files
		for _, excl := range opts.Excludes {
			if matched, _ := filepath.Match(excl, d.Name()); matched {
				return nil
			}
			if matched, _ := filepath.Match(excl, rel); matched {
				return nil
			}
		}

		// Apply glob filter(s)
		if len(opts.Globs) > 0 {
			matched := false
			for _, g := range opts.Globs {
				if m, _ := filepath.Match(g, d.Name()); m {
					matched = true
					break
				}
				if m, _ := filepath.Match(g, filepath.ToSlash(rel)); m {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		// Apply type filter
		if len(typeExts) > 0 {
			ext := strings.ToLower(filepath.Ext(path))
			found := false
			for _, e := range typeExts {
				if ext == e {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		}

		// Skip binary-ish extensions
		ext := strings.ToLower(filepath.Ext(path))
		if isBinaryExt(ext) {
			return nil
		}

		if info, err := d.Info(); err == nil {
			// Skip files exceeding max size
			if opts.MaxFilesize > 0 && info.Size() > opts.MaxFilesize {
				return nil
			}
			entries = append(entries, WalkEntry{
				Path:    path,
				RelPath: rel,
				Info:    info,
			})
		}
		return nil
	})

	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		return nil, err
	}

	return entries, nil
}

// isBinaryExt returns true for file extensions that are typically binary.
func isBinaryExt(ext string) bool {
	switch ext {
	case ".exe", ".dll", ".so", ".dylib", ".o", ".a", ".lib",
		".bin", ".dat", ".db", ".sqlite", ".sqlite3",
		".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".webp", ".tiff", ".tif",
		".zip", ".gz", ".tar", ".bz2", ".xz", ".7z", ".rar", ".lzma",
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".mp3", ".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv", ".wav", ".ogg",
		".pyc", ".pyo", ".pyd", ".class", ".jar", ".war",
		".woff", ".woff2", ".ttf", ".eot", ".otf",
		".wasm":
		return true
	}
	return false
}
