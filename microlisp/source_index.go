package microlisp

import (
	"bufio"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// SourceEntry holds location info for a function.
type SourceEntry struct {
	Kind   string // "builtin", "stdlib", "special", "helper"
	File   string // Go source file or "stdlib.go"
	Start  int    // 1-based line number
	End    int    // 1-based line number
	Doc    string // brief description
	GoFunc string // Go function name (for builtins/helpers)
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

	// Scan special form case blocks in eval_core.go for line ranges
	scanSpecialForms()

	// Scan all non-builtin Go functions as helpers
	scanHelperFunctions()
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

// registerStdlibFunctions parses the stdlib string and extracts function names
// with Start/End line numbers (relative to the embedded stdlibContent string).
func registerStdlibFunctions() {
	// Match all define-like forms: (define (name ...), (define-macro (name ...),
	// (defsetf name ...), (defstruct name ...), (defgeneric name ...),
	// (define-condition name ...), (defconstant name ...), (defparameter name ...)
	defineRe := regexp.MustCompile(`\((?:define|define-macro|defsetf|defstruct|defgeneric|define-condition|defconstant|defparameter)\s+\(?(\S+)`)
	lines := strings.Split(stdlibContent, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if matches := defineRe.FindStringSubmatch(trimmed); len(matches) > 1 {
			// Strip trailing ) if present (e.g., "(define (internal-time-units-per-second)")
			name := strings.TrimRight(matches[1], ")")
			name = strings.ToLower(name)
			start := i + 1 // 1-based line in stdlibContent
			end := findSexpEndFromLines(lines, i)
			sourceIndex[name] = &SourceEntry{
				Kind: "stdlib",
				File: "stdlib.go",
				Start: start,
				End:   end,
				Doc:   fmt.Sprintf("Stdlib function: %s", matches[1]),
			}
		}
	}
}

// findSexpEndFromLines finds the closing paren of an s-expression starting at startLine.
// Returns 1-based line number.
func findSexpEndFromLines(lines []string, startLine int) int {
	depth := 0
	for i := startLine; i < len(lines); i++ {
		for _, ch := range lines[i] {
			if ch == '(' {
				depth++
			} else if ch == ')' {
				depth--
				if depth == 0 {
					return i + 1 // 1-based
				}
			}
		}
	}
	return len(lines)
}

// scanBuiltinFunctions scans microlisp/*.go files for builtin function definitions.
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

		// Read all lines first to avoid scanner state issues with findFuncEnd
		var lines []string
		f, err := os.Open(fpath)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		f.Close()

		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if matches := funcRe.FindStringSubmatch(trimmed); len(matches) > 1 {
				goFunc := matches[1]
				lispName, known := builtinMap[goFunc]
				if !known {
					continue // not a registered builtin
				}
				start := i + 1 // 1-based
				end := findFuncEndFromLines(lines, i)

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
	}
}

// findFuncEndFromLines finds the closing brace of a function starting at declLine.
func findFuncEndFromLines(lines []string, declLine int) int {
	depth := 0
	for i := declLine; i < len(lines); i++ {
		for _, ch := range lines[i] {
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
				if depth == 0 {
					return i + 1 // 1-based
				}
			}
		}
	}
	return len(lines)
}

// scanSpecialForms scans eval_core.go for case "FORMNAME": blocks and records
// line ranges for each special form in the index.
func scanSpecialForms() {
	dir := findMicrolispDir()
	if dir == "" {
		return
	}

	f, err := os.Open(filepath.Join(dir, "eval_core.go"))
	if err != nil {
		return
	}
	defer f.Close()

	// Read all lines first
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Match case "FORMNAME": or case "A", "B":
	// Capture all quoted names in the case label
	caseRe := regexp.MustCompile(`^\s+case\s+((?:"[A-Z][A-Z0-9\-!]*"\s*,?\s*)+):`)

	for i, line := range lines {
		if caseRe.MatchString(line) {
			// Extract all quoted names from the case label
			nameRe := regexp.MustCompile(`"([A-Z][A-Z0-9\-!]*)"`)
			allNames := nameRe.FindAllStringSubmatch(line, -1)
			if len(allNames) == 0 {
				continue
			}
			// Find the end of this case block
			startLine := i + 1 // 1-based
			endLine := findCaseBlockEnd(lines, i)

			for _, nameMatch := range allNames {
				formName := strings.ToLower(nameMatch[1])
				entry, ok := sourceIndex[formName]
				if !ok {
					continue // not a known special form
				}
				// Update to special form location in eval_core.go
				// This may overwrite a builtin entry if the function is in both lists
				entry.Kind = "special"
				entry.File = "eval_core.go"
				entry.Start = startLine
				entry.End = endLine
			}
		}
	}
}

// findCaseBlockEnd finds the line where a switch case block ends.
// In Go, case blocks extend until the next case/default or the closing brace.
func findCaseBlockEnd(lines []string, caseLine int) int {
	// Track brace depth relative to the switch
	switchDepth := 0
	// Find the opening brace of the switch (should be on or after the switch line)
	for i := caseLine; i >= 0; i-- {
		for _, ch := range lines[i] {
			if ch == '{' {
				switchDepth++
			} else if ch == '}' {
				switchDepth--
			}
		}
	}
	// switchDepth now represents how many braces were opened before the caseLine
	// The switch itself opened one brace, so inside the switch depth >= 1

	for i := caseLine + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Count braces on this line before checking
		for _, ch := range line {
			if ch == '{' {
				switchDepth++
			} else if ch == '}' {
				switchDepth--
				if switchDepth == 0 {
					// Closing brace of the switch statement
					return i + 1 // 1-based
				}
			}
		}

		// Check if this line starts a new case or default at the switch level
		// In Go, case labels inside a switch are at one indent level inside the switch
		if strings.HasPrefix(trimmed, "case ") || strings.HasPrefix(trimmed, "default:") {
			// This is a new case block — the previous one ended on the line before this
			return i // 1-based (i is 0-based, so line i is 1-based i+1, but we want the line before)
		}
	}
	return len(lines)
}

// scanHelperFunctions scans all microlisp/*.go files for non-builtin Go functions
// and registers them as "helper" entries in the source index.
func scanHelperFunctions() {
	dir := findMicrolispDir()
	if dir == "" {
		return
	}

	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return
	}

	// Match any function: func <name>(...
	// Skip test files and main.go
	funcRe := regexp.MustCompile(`^func\s+([A-Za-z][A-Za-z0-9_]*)\s*\(`)

	for _, fpath := range goFiles {
		base := filepath.Base(fpath)
		if strings.HasSuffix(base, "_test.go") || base == "main.go" {
			continue
		}

		// Read all lines first to avoid scanner state issues
		var lines []string
		f, err := os.Open(fpath)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		f.Close()

		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if matches := funcRe.FindStringSubmatch(trimmed); len(matches) > 1 {
				goFunc := matches[1]
				// Skip builtins (already indexed)
				if strings.HasPrefix(goFunc, "builtin") {
					continue
				}
				// Skip already-indexed names (stdlib, special forms)
				lower := strings.ToLower(goFunc)
				if _, exists := sourceIndex[lower]; exists {
					continue
				}
				start := i + 1 // 1-based
				end := findFuncEndFromLines(lines, i)
				sourceIndex[lower] = &SourceEntry{
					Kind:   "helper",
					File:   base,
					Start:  start,
					End:    end,
					Doc:    fmt.Sprintf("Helper function: %s", goFunc),
					GoFunc: goFunc,
				}
			}
		}
	}
}

// GetSource returns the source code for a named function or special form.
// offset/limit control how many lines of source to show (0 means unlimited).
func GetSource(name string, offset, limit int) string {
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
	case "builtin", "helper":
		if entry.GoFunc != "" {
			fmt.Fprintf(&b, "Go function: %s\n", entry.GoFunc)
		}
		if entry.Start > 0 {
			fmt.Fprintf(&b, "Lines: %d-%d", entry.Start, entry.End)
			totalLines := entry.End - entry.Start + 1
			if limit > 0 && totalLines > limit {
				fmt.Fprintf(&b, " (showing %d lines, use offset/limit to see more)", limit)
			}
			fmt.Fprintf(&b, "\n")
		}
		fmt.Fprintf(&b, "\n%s", readSourceSnippet(entry.File, entry.Start, entry.End, offset, limit))

	case "stdlib":
		if entry.Start > 0 {
			fmt.Fprintf(&b, "Lines: %d-%d", entry.Start, entry.End)
			totalLines := entry.End - entry.Start + 1
			if limit > 0 && totalLines > limit {
				fmt.Fprintf(&b, " (showing %d lines, use offset/limit to see more)", limit)
			}
			fmt.Fprintf(&b, "\n")
		}
		fmt.Fprintf(&b, "\n%s", readStdlibSnippet(name, offset, limit))

	case "special":
		fmt.Fprintf(&b, "\n%s", entry.Doc)
		if entry.Start > 0 {
			fmt.Fprintf(&b, "\nLines: %d-%d", entry.Start, entry.End)
			totalLines := entry.End - entry.Start + 1
			if limit > 0 && totalLines > limit {
				fmt.Fprintf(&b, " (showing %d lines, use offset/limit to see more)", limit)
			}
			fmt.Fprintf(&b, "\n")
			fmt.Fprintf(&b, "\n%s", readSourceSnippet(entry.File, entry.Start, entry.End, offset, limit))
		} else {
			fmt.Fprintf(&b, "\nSpecial forms are handled directly by the evaluator (eval_core.go).")
		}
	}

	return b.String()
}

// readSourceSnippet reads lines from a microlisp source file.
// offset: skip first N lines of the snippet (0 = start from beginning).
// limit: max lines to show (0 = unlimited, show all).
func readSourceSnippet(filename string, start, end, offset, limit int) string {
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
	shown := 0
	for scanner.Scan() {
		lineNum++
		if lineNum >= start && lineNum <= end {
			// Apply offset: skip first N lines of the snippet
			snippetLine := lineNum - start
			if snippetLine < offset {
				continue
			}
			// Apply limit: stop after showing limit lines
			if limit > 0 && shown >= limit {
				break
			}
			fmt.Fprintf(&b, "%4d  %s\n", lineNum, scanner.Text())
			shown++
		}
		if lineNum > end {
			break
		}
	}
	// If truncated, add continuation hint
	totalLines := end - start + 1
	if limit > 0 && shown >= limit && shown < totalLines-offset {
		fmt.Fprintf(&b, "\n  ... %d more lines (use offset=%d, limit=%d to continue)\n",
			totalLines-offset-shown, offset+shown, limit)
	}
	return b.String()
}

// readStdlibSnippet extracts a stdlib function definition from the embedded stdlib string.
// offset: skip first N lines of the definition (0 = start from beginning).
// limit: max lines to show (0 = unlimited, show all).
func readStdlibSnippet(name string, offset, limit int) string {
	entry, ok := sourceIndex[name]
	if !ok || entry.Start == 0 {
		// Fallback: use regex-based extraction (no line numbers known)
		return readStdlibSnippetRegex(name, offset, limit)
	}

	lines := strings.Split(stdlibContent, "\n")
	start := entry.Start - 1 // convert to 0-based
	end := entry.End         // already 1-based, exclusive upper bound for slice

	var b strings.Builder
	shown := 0
	for i := start; i < end && i < len(lines); i++ {
		snippetLine := i - start
		if snippetLine < offset {
			continue
		}
		if limit > 0 && shown >= limit {
			break
		}
		fmt.Fprintf(&b, "%4d  %s\n", i+1, lines[i])
		shown++
	}
	// If truncated, add continuation hint
	totalLines := end - start
	if limit > 0 && shown >= limit && shown < totalLines-offset {
		fmt.Fprintf(&b, "\n  ... %d more lines (use offset=%d, limit=%d to continue)\n",
			totalLines-offset-shown, offset+shown, limit)
	}
	return b.String()
}

// readStdlibSnippetRegex is the fallback regex-based extraction for stdlib functions
// that don't have line numbers recorded.
func readStdlibSnippetRegex(name string, offset, limit int) string {
	// Match all define-like forms that contain the function name
	// Use [ \t()\n] instead of \b to handle names with special chars like *
	pattern := fmt.Sprintf(`(?i)\((?:define|define-macro|defsetf|defstruct|defgeneric|define-condition|defconstant|defparameter)\s+\(?\s*%s(?:[ \t()\n]|$)`, regexp.QuoteMeta(name))
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
					shown := 0
					for k := start; k <= j; k++ {
						snippetLine := k - start
						if snippetLine < offset {
							continue
						}
						if limit > 0 && shown >= limit {
							break
						}
						fmt.Fprintf(&b, "%4d  %s\n", k+1, lines[k])
						shown++
					}
					// If truncated, add continuation hint
					totalLines := j - start + 1
					if limit > 0 && shown >= limit && shown < totalLines-offset {
						fmt.Fprintf(&b, "\n  ... %d more lines (use offset=%d, limit=%d to continue)\n",
							totalLines-offset-shown, offset+shown, limit)
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
	builtins, stdlibCount, special, helpers := 0, 0, 0, 0
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
			case "helper":
				helpers++
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
	fmt.Fprintf(&b, "microlisp source index (%d total entries, %d match %q):\n", len(sourceIndex), totalMatched, query)
	fmt.Fprintf(&b, "  Builtins: %d | Stdlib: %d | Special forms: %d | Helpers: %d\n", builtins, stdlibCount, special, helpers)
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
	// 1. Try relative to cwd
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

	// 2. Try relative to executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		for _, rel := range []string{"..", filepath.Join("..", "..")} {
			c := filepath.Join(exeDir, rel, "microlisp")
			if info, err := os.Stat(c); err == nil && info.IsDir() {
				if _, err := os.Stat(filepath.Join(c, "eval.go")); err == nil {
					abs, _ := filepath.Abs(c)
					return abs
				}
			}
		}
	}

	// 3. Use runtime.Caller to find this source file's directory
	if _, file, _, ok := runtime.Caller(0); ok {
		dir := filepath.Dir(file)
		if _, err := os.Stat(filepath.Join(dir, "eval.go")); err == nil {
			return dir
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