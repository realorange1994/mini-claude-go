package rgrep

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// splitGlobPatterns splits a glob string on commas and whitespace,
// respecting brace groups. E.g. "*.ts, *.js" -> ["*.ts", "*.js"].
func splitGlobPatterns(glob string) []string {
	var parts []string
	var current strings.Builder
	inBrace := false
	for _, c := range glob {
		switch c {
		case '{':
			inBrace = true
			current.WriteRune(c)
		case '}':
			inBrace = false
			current.WriteRune(c)
		case ',', ' ':
			if !inBrace && current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(c)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// Search performs a full search using the given config.
// This is the main entry point for the rgrep package.
func Search(cfg SearchConfig) SearchResult {
	if cfg.Ctx == nil {
		cfg.Ctx = context.Background()
	}

	// Compile the regex pattern
	searchPattern := cfg.Pattern
	if cfg.FixedStrings {
		searchPattern = regexp.QuoteMeta(searchPattern)
	}
	if cfg.CaseInsensitive {
		searchPattern = "(?i)" + searchPattern
	}
	if cfg.Multiline {
		searchPattern = "(?s)" + searchPattern
	}

	re, err := regexp.Compile(searchPattern)
	if err != nil {
		return SearchResult{Err: fmt.Errorf("invalid regex: %v", err)}
	}

	// Determine if we're searching a single file or a directory
	info, err := os.Stat(cfg.Path)
	if err != nil {
		return SearchResult{Err: fmt.Errorf("path not found: %v", err)}
	}

	var entries []WalkEntry
	if info.Mode().IsRegular() {
		// Single file search
		entries = []WalkEntry{
			{Path: cfg.Path, RelPath: cfg.Path, Info: info},
		}
	} else {
		// Directory search — use the walker
		globs := splitGlobPatterns(cfg.Glob)
		walkOpts := WalkOptions{
			Root:             cfg.Path,
			MaxDepth:         cfg.MaxDepth,
			Globs:            globs,
			TypeFilter:       cfg.TypeFilter,
			RespectGitIgnore: true,
			Ctx:              cfg.Ctx,
			MaxFilesize:      cfg.MaxFilesize,
		}
		entries, err = WalkDir(walkOpts)
		if err != nil {
			return SearchResult{Err: err}
		}
	}

	filesSearched := len(entries)
	result := SearchResult{
		FilesSearched: filesSearched,
	}

	switch cfg.OutputMode {
	case OutputFilesWithMatch:
		result = searchFilesOnly(re, entries, cfg, filesSearched)
	case OutputCount:
		result = searchCount(re, entries, cfg, filesSearched)
	default:
		result = searchContent(re, entries, cfg, filesSearched)
	}

	return result
}

// searchFilesOnly returns just the file paths that contain matches.
func searchFilesOnly(re *regexp.Regexp, entries []WalkEntry, cfg SearchConfig, filesSearched int) SearchResult {
	var results []Result
	skipped := 0
	totalMatches := 0

	for _, entry := range entries {
		select {
		case <-cfg.Ctx.Done():
			return SearchResult{Err: cfg.Ctx.Err()}
		default:
		}

		f, err := os.Open(entry.Path)
		if err != nil {
			continue
		}

		// Binary detection: scan first 8KB for null bytes
		buf := make([]byte, 8192)
		n, _ := io.ReadFull(f, buf)
		if n > 0 && strings.Contains(string(buf[:min(n, 8192)]), "\x00") {
			f.Close()
			continue
		}

		// Read the rest of the file and check for match
		var rest []byte
		if n > 0 {
			rest, _ = io.ReadAll(f)
			rest = append(buf[:n], rest...)
		} else {
			rest, _ = io.ReadAll(f)
		}
		f.Close()

		if re.Match(rest) {
			totalMatches++
			if skipped < cfg.Offset {
				skipped++
				continue
			}
			relPath := makeRelative(cfg.Path, entry)
			results = append(results, Result{Path: relPath})
			if cfg.HeadLimit > 0 && len(results) >= cfg.HeadLimit {
				return SearchResult{
					Results:       results,
					FilesSearched: filesSearched,
					TotalMatches:  totalMatches,
					Truncated:     true,
				}
			}
		}
	}

	return SearchResult{
		Results:       results,
		FilesSearched: filesSearched,
		TotalMatches:  totalMatches,
	}
}

// searchCount returns per-file match counts.
func searchCount(re *regexp.Regexp, entries []WalkEntry, cfg SearchConfig, filesSearched int) SearchResult {
	var results []Result
	totalMatches := 0

	for _, entry := range entries {
		select {
		case <-cfg.Ctx.Done():
			return SearchResult{Err: cfg.Ctx.Err()}
		default:
		}

		data, err := os.ReadFile(entry.Path)
		if err != nil {
			continue
		}

		// Binary detection
		if strings.Contains(string(data), "\x00") {
			continue
		}

		var count int
		if cfg.Multiline {
			count = len(re.FindAllIndex(data, -1))
		} else {
			scanner := bufio.NewScanner(strings.NewReader(string(data)))
			scanner.Buffer(make([]byte, 64*1024), 1024*1024)
			for scanner.Scan() {
				if re.MatchString(scanner.Text()) {
					count++
				}
			}
		}

		if count > 0 {
			totalMatches += count
			relPath := makeRelative(cfg.Path, entry)
			results = append(results, Result{
				Path:    relPath,
				LineNum: count, // reuse LineNum for count
			})
		}
	}

	// Apply offset and head_limit
	if cfg.Offset > 0 && cfg.Offset < len(results) {
		results = results[cfg.Offset:]
	}
	if cfg.HeadLimit > 0 && len(results) > cfg.HeadLimit {
		results = results[:cfg.HeadLimit]
		return SearchResult{
			Results:       results,
			FilesSearched: filesSearched,
			TotalMatches:  totalMatches,
			Truncated:     true,
		}
	}

	return SearchResult{
		Results:       results,
		FilesSearched: filesSearched,
		TotalMatches:  totalMatches,
	}
}

// searchContent returns matching lines with context.
func searchContent(re *regexp.Regexp, entries []WalkEntry, cfg SearchConfig, filesSearched int) SearchResult {
	var results []Result
	skipped := 0
	totalMatches := 0
	ctxBefore := cfg.ContextBefore
	ctxAfter := cfg.ContextAfter

	for _, entry := range entries {
		select {
		case <-cfg.Ctx.Done():
			return SearchResult{Err: cfg.Ctx.Err()}
		default:
		}

		if cfg.Multiline {
			data, err := os.ReadFile(entry.Path)
			if err != nil {
				continue
			}
			// Binary detection
			if strings.Contains(string(data), "\x00") {
				continue
			}
			content := string(data)

			// Find all matches with positions
			matches := re.FindAllStringIndex(content, -1)
			if matches == nil {
				continue
			}
			relPath := makeRelative(cfg.Path, entry)

			for _, m := range matches {
				matchText := content[m[0]:m[1]]
				// Truncate long match text
				if len(matchText) > MaxGrepLineLen {
					matchText = matchText[:MaxGrepLineLen] + "..."
				}
				// Compute starting line number
				lineNum := strings.Count(content[:m[0]], "\n") + 1
				totalMatches++
				if skipped < cfg.Offset {
					skipped++
					continue
				}
				// Normalize match text: replace internal newlines with \n for display
				displayText := strings.ReplaceAll(matchText, "\n", "\\n")
				results = append(results, Result{
					Path:    relPath,
					LineNum: lineNum,
					Line:    displayText,
				})
				if cfg.HeadLimit > 0 && totalMatches-skipped >= cfg.HeadLimit {
					return SearchResult{
						Results:       results,
						FilesSearched: filesSearched,
						TotalMatches:  totalMatches,
						Truncated:     true,
					}
				}
			}
		} else {
			f, err := os.Open(entry.Path)
			if err != nil {
				continue
			}

			scanner := bufio.NewScanner(f)
			// 64KB buffer matching ripgrep's default
			scanner.Buffer(make([]byte, 64*1024), 1024*1024)

			// Read all lines for context support
			var lines []string
			binary := false
			for scanner.Scan() {
				line := scanner.Text()
				// Binary detection: check for null bytes
				if strings.Contains(line, "\x00") {
					binary = true
					break
				}
				// Truncate long lines
				if len(line) > MaxGrepLineLen {
					line = line[:MaxGrepLineLen] + "..."
				}
				lines = append(lines, line)
			}
			f.Close()

			if binary {
				continue
			}

			relPath := makeRelative(cfg.Path, entry)
			afterLeft := 0 // tracks remaining after-context lines to print

			for i, line := range lines {
				if re.MatchString(line) {
					totalMatches++
					if skipped < cfg.Offset {
						skipped++
						afterLeft = 0
						continue
					}

					// Print before-context
					if ctxBefore > 0 {
						start := max(0, i-ctxBefore)
						for j := start; j < i; j++ {
							results = append(results, Result{
								Path:    relPath,
								LineNum: j + 1,
								Line:    lines[j],
							})
						}
					}

					// Print the match line
					results = append(results, Result{
						Path:    relPath,
						LineNum: i + 1,
						Line:    line,
					})

					// Set after-context counter
					afterLeft = ctxAfter

					if cfg.HeadLimit > 0 && totalMatches-skipped >= cfg.HeadLimit {
						return SearchResult{
							Results:       results,
							FilesSearched: filesSearched,
							TotalMatches:  totalMatches,
							Truncated:     true,
						}
					}
				} else if afterLeft > 0 {
					// Print after-context line
					results = append(results, Result{
						Path:    relPath,
						LineNum: i + 1,
						Line:    line,
					})
					afterLeft--
				}
			}
		}
	}

	return SearchResult{
		Results:       results,
		FilesSearched: filesSearched,
		TotalMatches:  totalMatches,
	}
}

// MaxGrepLineLen is the maximum length of a line in content output.
const MaxGrepLineLen = 500

// TruncateLine truncates a line to MaxGrepLineLen characters.
func TruncateLine(line string) string {
	if len(line) <= MaxGrepLineLen {
		return line
	}
	return line[:MaxGrepLineLen] + "..."
}

// makeRelative returns a relative path for display.
func makeRelative(root string, entry WalkEntry) string {
	// If the entry already has a good relative path, use it
	if entry.RelPath != "" && entry.RelPath != "." {
		return filepathToSlash(entry.RelPath)
	}
	// Otherwise compute relative to root
	rel, err := filepath.Rel(root, entry.Path)
	if err != nil {
		return entry.Path
	}
	return filepathToSlash(rel)
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}