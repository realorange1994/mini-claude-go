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

	if _, err := os.Stat(root); os.IsNotExist(err) {
		return vstr(fmt.Sprintf("Error: no such directory: %s", root)), nil
	}

	searchPattern := pattern
	if caseInsensitive {
		searchPattern = strings.ToLower(pattern)
	}

	var matches []string
	matchCount := 0

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			return nil
		}

		// Apply glob filter
		if glob != "" {
			matched, _ := filepath.Match(glob, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		// Open and scan file
		f, err := os.Open(path)
		if err != nil {
			return nil // skip unreadable files
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		// Increase buffer size for long lines
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		lineNum := 0
		fileHasMatch := false

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			searchLine := line
			if caseInsensitive {
				searchLine = strings.ToLower(line)
			}

			if strings.Contains(searchLine, searchPattern) {
				fileHasMatch = true
				matchCount++

				if outputMode == "content" && len(matches) < headLimit {
					matches = append(matches, fmt.Sprintf("%s\t%d\t%s", path, lineNum, line))
				}
			}
		}

		if fileHasMatch && outputMode == "files_with_matches" && len(matches) < headLimit {
			matches = append(matches, path)
		}
		return nil
	})

	if err != nil {
		return vstr(fmt.Sprintf("Error: %v", err)), nil
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
