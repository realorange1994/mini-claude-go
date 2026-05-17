package microlisp

import (
	"bufio"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SourceEntry holds location info for a function.
type SourceEntry struct {
	Kind     string // "builtin", "stdlib", "special", "macro"
	File     string // Go source file or "stdlib.lisp"
	Start    int    // 1-based line number
	End      int    // 1-based line number (0 for stdlib/special)
	Doc      string // brief description
	GoFunc   string // Go function name (for builtins)
}

var sourceIndex map[string]*SourceEntry

func init() {
	sourceIndex = make(map[string]*SourceEntry)

	// Initialize stdlibContent first (needed by registerStdlibFunctions below)
	data, err := stdlibFS.ReadFile("stdlib.go")
	if err == nil {
		stdlibContent = extractStdlibString(string(data))
	}

	// Register special forms
	for _, name := range specialOpNames {
		sourceIndex[strings.ToLower(name)] = &SourceEntry{
			Kind: "special",
			File: "eval_core.go",
			Doc:  fmt.Sprintf("Special form: %s", name),
		}
	}

	// Register stdlib Lisp functions
	registerStdlibFunctions()

	// Scan builtin_register.go to get lisp-name -> go-func mappings,
	// then find source for each go-func in the Go source files.
	builtinMap := loadBuiltinRegistry()
	scanBuiltinFunctions(builtinMap)
}

// loadBuiltinRegistry parses builtin_register.go to extract {lisp-name, go-func} pairs.
func loadBuiltinRegistry() map[string]string {
	// Returns map: goFuncName -> lispName, e.g. "builtinAdd" -> "+"
	result := make(map[string]string)

	dir := findMicrolispDir()
	if dir == "" {
		return result
	}

	f, err := os.Open(filepath.Join(dir, "builtin_register.go"))
	if err != nil {
		return result
	}
	defer f.Close()

	// Match: {"lisp-name", builtinFunc},
	re := regexp.MustCompile(`^\s*\{"([^"]+)",\s*(builtin[A-Za-z0-9_]+)\},`)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := re.FindStringSubmatch(line); len(matches) > 1 {
			lispName := matches[1]
			goFunc := matches[2]
			result[goFunc] = lispName
		}
	}
	return result
}

// RegisterStdlibFunctions parses the stdlib string and extracts function names.
func registerStdlibFunctions() {
	// Extract (define (name ...) and (define-macro (name ...) from stdlib
	defineRe := regexp.MustCompile(`\((?:define|define-macro)\s+\(?(\S+)`)
	for _, line := range strings.Split(stdlibContent, "\n") {
		line = strings.TrimSpace(line)
		if matches := defineRe.FindStringSubmatch(line); len(matches) > 1 {
			name := strings.ToLower(matches[1])
			sourceIndex[name] = &SourceEntry{
				Kind: "stdlib",
				File: "stdlib.go",
				Doc:  fmt.Sprintf("Stdlib function: %s", matches[1]),
			}
		}
	}
}

// ScanBuiltinFunctions scans microlisp/*.go files for builtin function definitions.
func scanBuiltinFunctions(builtinMap map[string]string) {
	dir := findMicrolispDir()
	if dir == "" {
		return
	}

	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return
	}

	funcRe := regexp.MustCompile(`^func\s+(builtin[A-Za-z0-9_]+)\s*\(`)

	for _, fpath := range goFiles {
		base := filepath.Base(fpath)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}

		f, err := os.Open(fpath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimSpace(scanner.Text())
			if matches := funcRe.FindStringSubmatch(line); len(matches) > 1 {
				goFunc := matches[1]
				lispName, known := builtinMap[goFunc]
				if !known {
					continue // not a registered builtin
				}
				start := lineNum
				end := findFuncEnd(scanner, lineNum)

				sourceIndex[strings.ToLower(lispName)] = &SourceEntry{
					Kind:   "builtin",
					File:   base,
					Start:  start,
					End:    end,
					Doc:    fmt.Sprintf("Builtin: %s (Go: %s)", lispName, goFunc),
					GoFunc: goFunc,
				}
			}
		}
		f.Close()
	}
}

// findFuncEnd reads lines from scanner until the function's closing brace.
// Assumes we're starting from the function declaration line.
func findFuncEnd(scanner *bufio.Scanner, startLine int) int {
	depth := 1 // we're inside the function body already
	lineNum := startLine
	for scanner.Scan() {
		lineNum++
		for _, ch := range scanner.Text() {
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
				if depth == 0 {
					return lineNum
				}
			}
		}
	}
	return lineNum
}

// GetSource returns the source code for a named function or special form.
// Returns empty string if not found.
func GetSource(name string) string {
	name = strings.ToLower(name)

	// Look for exact match first
	entry, ok := sourceIndex[name]
	if !ok {
		// Try prefix match — find the first function containing name
		best := ""
		for k := range sourceIndex {
			if strings.Contains(k, name) && (best == "" || len(k) < len(best)) {
				best = k
			}
		}
		if best != "" {
			entry = sourceIndex[best]
			name = best
		} else {
			return fmt.Sprintf("No source found for: %s\n\nTry: (source-list) to see all available functions, or search with a partial name.", name)
		}
	}

	// Build the response
	var b strings.Builder
	fmt.Fprintf(&b, "=== %s (%s) ===\n", name, entry.Kind)
	fmt.Fprintf(&b, "File: %s\n", entry.File)

	switch entry.Kind {
	case "builtin":
		if entry.GoFunc != "" {
			fmt.Fprintf(&b, "Go function: %s\n", entry.GoFunc)
		}
		if entry.Start > 0 {
			fmt.Fprintf(&b, "Lines: %d-%d\n", entry.Start, entry.End)
		}
		fmt.Fprintf(&b, "\n%s", readSourceSnippet(entry.File, entry.Start, entry.End))

	case "stdlib":
		fmt.Fprintf(&b, "\n%s", readStdlibSnippet(name))

	case "special":
		fmt.Fprintf(&b, "\n%s", entry.Doc)
		fmt.Fprintf(&b, "\nSpecial forms are handled directly by the evaluator (eval_core.go).")
	}

	return b.String()
}

// readSourceSnippet reads lines from a microlisp source file.
func readSourceSnippet(filename string, start, end int) string {
	dir := findMicrolispDir()
	if dir == "" {
		return "(source directory not available)\n"
	}

	f, err := os.Open(filepath.Join(dir, filename))
	if err != nil {
		return fmt.Sprintf("(cannot open %s: %v)\n", filename, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var b strings.Builder
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum >= start && lineNum <= end {
			fmt.Fprintf(&b, "%4d  %s\n", lineNum, scanner.Text())
		}
		if lineNum > end {
			break
		}
	}
	return b.String()
}

// readStdlibSnippet extracts a stdlib function definition from the embedded stdlib string.
func readStdlibSnippet(name string) string {
	// Find the function in stdlib
	// Match (define (name ...) or (define name ...)
	pattern := fmt.Sprintf(`(?i)\(define\s+\(?%s\b`, regexp.QuoteMeta(name))
	re := regexp.MustCompile(pattern)

	lines := strings.Split(stdlibContent, "\n")
	for i, line := range lines {
		if re.MatchString(line) {
			// Found the start, now find the end by counting parens
			start := i
			depth := 0
			for j := i; j < len(lines); j++ {
				for _, ch := range lines[j] {
					if ch == '(' {
						depth++
					} else if ch == ')' {
						depth--
					}
				}
				if depth == 0 {
					// Found the end
					var b strings.Builder
					for k := start; k <= j; k++ {
						fmt.Fprintf(&b, "%4d  %s\n", k+1, lines[k])
					}
					return b.String()
				}
			}
		}
	}
	return "(function definition not found in stdlib)\n"
}

// SourceList returns a summary of all indexed functions with pagination.
// query: substring filter (empty = all)
// offset: skip first N entries (1-based, default 1)
// limit: max entries to return (default 50)
func SourceList(query string, offset, limit int) string {
	const defaultLimit = 50

	if limit <= 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Collect and sort matching entries
	var matched []string
	builtins, stdlibCount, special := 0, 0, 0
	for name, entry := range sourceIndex {
		if query == "" || strings.Contains(name, strings.ToLower(query)) {
			matched = append(matched, name)
			switch entry.Kind {
			case "builtin":
				builtins++
			case "stdlib":
				stdlibCount++
			case "special":
				special++
			}
		}
	}

	if len(matched) == 0 {
		return fmt.Sprintf("No functions found matching: %q\nUse (source-list) with empty query to see all functions.", query)
	}

	// Sort for stable pagination
	sort.Strings(matched)

	totalMatched := len(matched)

	// Apply offset and limit
	if offset >= totalMatched {
		return fmt.Sprintf("Offset %d exceeds total matches (%d). Use offset=0 to start from beginning.", offset, totalMatched)
	}
	end := offset + limit
	if end > totalMatched {
		end = totalMatched
	}
	page := matched[offset:end]

	var b strings.Builder
	fmt.Fprintf(&b, "microlisp function index (%d total functions, %d match %q):\n", len(sourceIndex), totalMatched, query)
	fmt.Fprintf(&b, "  Builtins: %d | Stdlib: %d | Special forms: %d\n", builtins, stdlibCount, special)
	fmt.Fprintf(&b, "  Showing entries %d-%d of %d (use offset/limit to page)\n\n", offset+1, end, totalMatched)
	fmt.Fprintf(&b, "Usage: (source \"name\") to view source of a specific function.\n\n")

	for _, name := range page {
		entry := sourceIndex[name]
		fmt.Fprintf(&b, "  %-30s %-10s %s\n", name, entry.Kind, entry.File)
	}

	if end < totalMatched {
		fmt.Fprintf(&b, "\n  ... %d more entries (use offset=%d to continue)\n", totalMatched-end, end)
	}

	return b.String()
}

// findMicrolispDir locates the microlisp source directory.
func findMicrolispDir() string {
	// Try: relative to cwd, then relative to executable
	candidates := []string{
		"microlisp",
		filepath.Join("..", "microlisp"),
		filepath.Join("..", "..", "microlisp"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(c, "eval.go")); err == nil {
				abs, _ := filepath.Abs(c)
				return abs
			}
		}
	}
	return ""
}

// ---- Embed stdlib for reference ----

//go:embed stdlib.go
var stdlibFS embed.FS

// stdlibContent is the embedded stdlib source string.
var stdlibContent string

// extractStdlibString extracts the initLib string constant from stdlib.go source.
func extractStdlibString(src string) string {
	re := regexp.MustCompile(`(?s)var initLib\s*=\s*` + "`" + `(.*)` + "`")
	if matches := re.FindStringSubmatch(src); len(matches) > 1 {
		return matches[1]
	}
	return ""
}
