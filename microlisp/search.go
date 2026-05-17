package microlisp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// builtinGoSearch performs efficient file+content search natively in Go,
// avoiding the O(N^2) string allocation overhead of the Lisp line-scanner.
// Only scans top-level files in the given directory (non-recursive),
// matching the old Lisp directory/glob behavior.
//
// Args: pattern path output-mode case-insensitive head-limit glob
func builtinGoSearch(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("go-search: need at least a pattern")
	}

	pattern := primaryValue(args[0]).str
	root := "."
	if len(args) > 1 {
		root = primaryValue(args[1]).str
	}

	outputMode := "content"
	if len(args) > 2 {
		outputMode = primaryValue(args[2]).str
	}

	caseInsensitive := false
	if len(args) > 3 && !isNil(primaryValue(args[3])) {
		caseInsensitive = true
	}

	headLimit := 100
	if len(args) > 4 {
		headLimit = int(primaryValue(args[4]).num)
		if headLimit <= 0 {
			headLimit = 100
		}
	}

	glob := ""
	if len(args) > 5 {
		v := primaryValue(args[5])
		if v.typ == VStr && v.str != "" {
			glob = v.str
		}
	}

	// If root is a regular file, search just that file
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		return searchSingleFile(root, pattern, outputMode, caseInsensitive, headLimit)
	}

	if _, err := os.Stat(root); os.IsNotExist(err) {
		return vstr(fmt.Sprintf("Error: no such directory: %s", root)), nil
	}

	searchPattern := pattern
	if caseInsensitive {
		searchPattern = strings.ToLower(pattern)
	}

	// Read directory entries (non-recursive, matching old Lisp behavior)
	entries, err := os.ReadDir(root)
	if err != nil {
		return vstr(fmt.Sprintf("Error: %v", err)), nil
	}

	var matches []string
	matchCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Apply glob filter
		if glob != "" {
			matched, _ := filepath.Match(glob, name)
			if !matched {
				continue
			}
		}

		fpath := filepath.Join(root, name)

		fileMatches, fileCount := scanFile(fpath, searchPattern, outputMode, caseInsensitive, headLimit-len(matches))
		matches = append(matches, fileMatches...)
		matchCount += fileCount

		if len(matches) >= headLimit {
			break
		}
	}

	var result string
	switch outputMode {
	case "count":
		result = fmt.Sprintf("%d", matchCount)
	default:
		result = strings.Join(matches, "\n")
	}
	return vstr(result), nil
}

// searchSingleFile searches one file and returns the result directly.
func searchSingleFile(fpath, pattern, outputMode string, caseInsensitive bool, headLimit int) (*Value, error) {
	searchPattern := pattern
	if caseInsensitive {
		searchPattern = strings.ToLower(pattern)
	}

	matches, count := scanFile(fpath, searchPattern, outputMode, caseInsensitive, headLimit)

	var result string
	switch outputMode {
	case "count":
		result = fmt.Sprintf("%d", count)
	default:
		result = strings.Join(matches, "\n")
	}
	return vstr(result), nil
}

// scanFile reads a file line-by-line using bufio.Scanner and searches for the pattern.
// Returns matching lines (for content/files_with_matches modes) and total match count.
func scanFile(fpath, searchPattern, outputMode string, caseInsensitive bool, remaining int) ([]string, int) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var matches []string
	lineNum := 0
	matchCount := 0
	fileAdded := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		searchLine := line
		if caseInsensitive {
			searchLine = strings.ToLower(line)
		}

		if strings.Contains(searchLine, searchPattern) {
			matchCount++

			switch outputMode {
			case "content":
				if remaining > 0 {
					matches = append(matches, fmt.Sprintf("%s\t%d\t%s", fpath, lineNum, line))
					remaining--
				}
			case "files_with_matches":
				if !fileAdded {
					matches = append(matches, fpath)
					fileAdded = true
				}
			}
		}
	}

	return matches, matchCount
}