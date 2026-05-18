package microlisp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// -------- Cross-Reference Index --------
//
// Builds a call-graph from microlisp source files (Go builtins + stdlib Lisp)
// to answer "who calls X?" (callers) and "what does X call?" (callees).
//
// For Go builtins: scans function bodies for calls to other builtinXxx funcs
// and for references to Lisp function names in string literals / symbol tables.
// For stdlib Lisp: scans s-expressions for symbol references.

// XRefEntry describes one reference from a caller to a callee.
type XRefEntry struct {
	Caller    string // function that contains the reference
	Callee    string // function being referenced
	File      string // source file containing the reference
	Line      int    // 1-based line number
	Kind      string // "builtin", "stdlib", "special", "helper"
	Snippet   string // the source line text (trimmed)
}

var xrefIndex []XRefEntry // flat list of all references
var xrefByCallee map[string][]int // callee name -> indices into xrefIndex
var xrefByCaller map[string][]int // caller name -> indices into xrefIndex
var xrefBuilt bool

// BuildXRefIndex constructs the cross-reference index.
// Called lazily on first xref query.
func BuildXRefIndex() {
	if xrefBuilt {
		return
	}
	xrefBuilt = true
	xrefIndex = nil
	xrefByCallee = make(map[string][]int)
	xrefByCaller = make(map[string][]int)

	dir := findMicrolispDir()
	if dir == "" {
		return
	}

	// 1. Scan Go source files for builtin/helper function cross-references
	scanGoXRefs(dir)

	// 2. Scan stdlib Lisp source for function cross-references
	scanStdlibXRefs()

	// 3. Build index maps
	for i, entry := range xrefIndex {
		xrefByCallee[strings.ToLower(entry.Callee)] = append(xrefByCallee[strings.ToLower(entry.Callee)], i)
		xrefByCaller[strings.ToLower(entry.Caller)] = append(xrefByCaller[strings.ToLower(entry.Caller)], i)
	}
}

// scanGoXRefs scans all microlisp/*.go files for cross-references between
// Go functions. It detects:
//   - Direct calls: builtinXxx(...) → maps to the Lisp name of builtinXxx
//   - Symbol references: strings like "car", "cdr" in symbol table entries
func scanGoXRefs(dir string) {
	goFiles, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return
	}

	// Build reverse map: Go func name -> Lisp name
	goToLisp := make(map[string]string)
	for name, entry := range sourceIndex {
		if entry.GoFunc != "" {
			goToLisp[strings.ToLower(entry.GoFunc)] = name
		}
	}

	// Regex to find calls to builtinXxx or other known Go funcs
	callRe := regexp.MustCompile(`(builtin[A-Za-z0-9_]+)\s*\(`)

	for _, fpath := range goFiles {
		base := filepath.Base(fpath)
		if strings.HasSuffix(base, "_test.go") || base == "main.go" {
			continue
		}

		var lines []string
		f, err := os.Open(fpath)
		if err != nil {
			continue
		}
		scanner := scanFileLines(f)
		f.Close()
		lines = scanner

		// Find which Go function we're currently inside
		funcRe := regexp.MustCompile(`^func\s+([A-Za-z][A-Za-z0-9_]*)\s*\(`)
		currentFunc := ""

		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Check if we're entering a new function
			if matches := funcRe.FindStringSubmatch(trimmed); len(matches) > 1 {
				currentFunc = matches[1]
			}
			if currentFunc == "" {
				continue
			}

			// Find calls to other builtin/helper functions
			for _, callMatch := range callRe.FindAllStringSubmatch(trimmed, -1) {
				calledGoFunc := callMatch[1]
				if calledGoFunc == currentFunc {
					continue // skip self-references
				}
				calledLispName := goToLisp[strings.ToLower(calledGoFunc)]
				if calledLispName == "" {
					// Not a registered builtin — try as helper
					calledLispName = strings.ToLower(calledGoFunc)
					if _, ok := sourceIndex[calledLispName]; !ok {
						continue // unknown function, skip
					}
				}

				// Determine caller's Lisp name
				callerLispName := goToLisp[strings.ToLower(currentFunc)]
				if callerLispName == "" {
					callerLispName = strings.ToLower(currentFunc)
				}

				entry := sourceIndex[callerLispName]
				kind := "helper"
				if entry != nil {
					kind = entry.Kind
				}

				xrefIndex = append(xrefIndex, XRefEntry{
					Caller:  callerLispName,
					Callee:  calledLispName,
					File:    base,
					Line:    i + 1,
					Kind:    kind,
					Snippet: trimmed,
				})
			}
		}
	}
}

// scanStdlibXRefs scans the embedded stdlib Lisp source for function references.
// It detects calls like (car x), (string-append a b), etc.
func scanStdlibXRefs() {
	if stdlibContent == "" {
		return
	}

	lines := strings.Split(stdlibContent, "\n")

	// Regex to find function call positions: (symbol-name ...)
	// This catches most function calls in Lisp code
	callRe := regexp.MustCompile(`\(([A-Za-z0-9_\-!?><=+*/%][A-Za-z0-9_\-!?><=+*/%.]*)\s`)

	// Also catch quoted symbols: 'symbol-name
	symbolRe := regexp.MustCompile(`'([A-Za-z0-9_\-!?><=+*/%][A-Za-z0-9_\-!?><=+*/%.]*)`)

	// Determine which stdlib function we're inside
	defineRe := regexp.MustCompile(`\((?:define|define-macro|defsetf|defstruct|defgeneric|define-condition|defconstant|defparameter)\s+\(?(\S+)`)
	currentFunc := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if we're entering a new define
		if matches := defineRe.FindStringSubmatch(trimmed); len(matches) > 1 {
			name := strings.TrimRight(matches[1], ")")
			currentFunc = strings.ToLower(name)
		}

		if currentFunc == "" {
			continue
		}

		// Find function calls in this line
		for _, callMatch := range callRe.FindAllStringSubmatch(trimmed, -1) {
			calledName := strings.ToLower(callMatch[1])
			// Skip self-references, special forms, and non-indexed names
			if calledName == currentFunc {
				continue
			}
			// Only index references to known functions
			if _, ok := sourceIndex[calledName]; !ok {
				// Could be a local variable or unknown function
				continue
			}

			xrefIndex = append(xrefIndex, XRefEntry{
				Caller:  currentFunc,
				Callee:  calledName,
				File:    "stdlib.go",
				Line:    i + 1,
				Kind:    "stdlib",
				Snippet: trimmed,
			})
		}

		// Find quoted symbol references (for funcall, apply, etc.)
		for _, symMatch := range symbolRe.FindAllStringSubmatch(trimmed, -1) {
			symName := strings.ToLower(symMatch[1])
			if symName == currentFunc {
				continue
			}
			if _, ok := sourceIndex[symName]; !ok {
				continue
			}
			// Avoid duplicates with callRe matches
			xrefIndex = append(xrefIndex, XRefEntry{
				Caller:  currentFunc,
				Callee:  symName,
				File:    "stdlib.go",
				Line:    i + 1,
				Kind:    "stdlib",
				Snippet: trimmed,
			})
		}
	}
}

// scanFileLines reads all lines from a file into a string slice.
func scanFileLines(f *os.File) []string {
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

// GetXRef returns cross-reference information for a named function.
// Shows both callers (who calls this function) and callees (what this function calls).
// contextLines controls how many surrounding lines to show for each reference.
func GetXRef(name string, contextLines int) string {
	BuildXRefIndex()

	name = strings.ToLower(name)

	// Resolve name (support partial matches like GetSource does)
	resolvedName := name
	if _, ok := sourceIndex[name]; !ok {
		best := ""
		for k := range sourceIndex {
			if strings.Contains(k, name) && (best == "" || len(k) < len(best)) {
				best = k
			}
		}
		if best != "" {
			resolvedName = best
		} else {
			return fmt.Sprintf("No cross-reference found for: %s\n\nTry: operation=source-list to see all available functions.", name)
		}
	}

	var b strings.Builder

	// Header with function info
	entry := sourceIndex[resolvedName]
	fmt.Fprintf(&b, "=== Cross-Reference: %s ===\n", resolvedName)
	fmt.Fprintf(&b, "Type: %s | File: %s | Lines: %d-%d\n\n", entry.Kind, entry.File, entry.Start, entry.End)

	// Callers: who calls this function?
	callerIdxs := xrefByCallee[resolvedName]
	if len(callerIdxs) > 0 {
		fmt.Fprintf(&b, "--- Callers (functions that call %s): %d ---\n", resolvedName, len(callerIdxs))
		for _, idx := range callerIdxs {
			ref := xrefIndex[idx]
			fmt.Fprintf(&b, "  %s (%s, %s:%d)\n", ref.Caller, ref.Kind, ref.File, ref.Line)
			if contextLines > 0 {
				fmt.Fprintf(&b, "    %s\n", ref.Snippet)
				if contextLines > 1 {
					showContext(&b, ref.File, ref.Line, contextLines)
				}
			}
		}
	} else {
		fmt.Fprintf(&b, "--- Callers: none ---\n")
	}

	fmt.Fprintf(&b, "\n")

	// Callees: what does this function call?
	calleeIdxs := xrefByCaller[resolvedName]
	if len(calleeIdxs) > 0 {
		// Deduplicate callees (same callee may appear multiple times)
		seen := make(map[string]bool)
		fmt.Fprintf(&b, "--- Callees (functions called by %s): %d ---\n", resolvedName, len(calleeIdxs))
		for _, idx := range calleeIdxs {
			ref := xrefIndex[idx]
			if seen[ref.Callee] {
				continue
			}
			seen[ref.Callee] = true
			calleeEntry := sourceIndex[ref.Callee]
			calleeInfo := ""
			if calleeEntry != nil {
				calleeInfo = fmt.Sprintf(" (%s, %s:%d-%d)", calleeEntry.Kind, calleeEntry.File, calleeEntry.Start, calleeEntry.End)
			}
			// Count how many times this callee is called
			count := 0
			for _, idx2 := range calleeIdxs {
				if xrefIndex[idx2].Callee == ref.Callee {
					count++
				}
			}
			callCount := ""
			if count > 1 {
				callCount = fmt.Sprintf(" [called %d times]", count)
			}
			fmt.Fprintf(&b, "  %s%s%s\n", ref.Callee, calleeInfo, callCount)
		}

		// Show call sites with context
		if contextLines > 0 && len(calleeIdxs) > 0 {
			fmt.Fprintf(&b, "\n  Call sites:\n")
			for _, idx := range calleeIdxs {
				ref := xrefIndex[idx]
				fmt.Fprintf(&b, "  %s:%d → %s\n", ref.File, ref.Line, ref.Callee)
				fmt.Fprintf(&b, "    %s\n", ref.Snippet)
				if contextLines > 1 {
					showContext(&b, ref.File, ref.Line, contextLines)
				}
			}
		}
	} else {
		fmt.Fprintf(&b, "--- Callees: none ---\n")
	}

	return b.String()
}

// XRefList returns a summary of all cross-reference relationships.
// query: filter substring (empty = all)
// offset/limit: pagination
func XRefList(query string, offset, limit int) string {
	BuildXRefIndex()

	const defaultLimit = 50
	if limit <= 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}

	// Collect unique caller-callee pairs
	type pair struct {
		caller string
		callee string
		count  int
	}
	pairMap := make(map[string]*pair) // key: "caller->callee"
	for _, ref := range xrefIndex {
		key := ref.Caller + "->" + ref.Callee
		if p, ok := pairMap[key]; ok {
			p.count++
		} else {
			pairMap[key] = &pair{ref.Caller, ref.Callee, 1}
		}
	}

	// Filter and sort
	var pairs []*pair
	for _, p := range pairMap {
		if query == "" ||
			strings.Contains(p.caller, strings.ToLower(query)) ||
			strings.Contains(p.callee, strings.ToLower(query)) {
			pairs = append(pairs, p)
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].caller != pairs[j].caller {
			return pairs[i].caller < pairs[j].caller
		}
		return pairs[i].callee < pairs[j].callee
	})

	if len(pairs) == 0 {
		return fmt.Sprintf("No cross-references found matching: %q\nUse operation=xref-list with empty query to see all.", query)
	}

	total := len(pairs)
	if offset >= total {
		return fmt.Sprintf("Offset %d exceeds total (%d). Use offset=0.", offset, total)
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := pairs[offset:end]

	var b strings.Builder
	fmt.Fprintf(&b, "Cross-Reference Index (%d call relationships, %d match %q):\n", total, len(pairs), query)
	fmt.Fprintf(&b, "Showing entries %d-%d of %d\n\n", offset+1, end, total)
	fmt.Fprintf(&b, "Usage: operation=xref with expression=<function-name> for detailed callers/callees.\n\n")

	for _, p := range page {
		fmt.Fprintf(&b, "  %s → %s", p.caller, p.callee)
		if p.count > 1 {
			fmt.Fprintf(&b, " [%d calls]", p.count)
		}
		fmt.Fprintf(&b, "\n")
	}

	if end < total {
		fmt.Fprintf(&b, "\n  ... %d more entries (use offset=%d to continue)\n", total-end, end)
	}

	return b.String()
}

// showContext prints surrounding lines for a reference at the given file and line.
func showContext(b *strings.Builder, filename string, lineNum int, contextLines int) {
	if contextLines <= 1 {
		return // already showing the line itself
	}

	// For stdlib.go, use the embedded stdlib content
	if filename == "stdlib.go" {
		lines := strings.Split(stdlibContent, "\n")
		start := lineNum - contextLines
		if start < 0 {
			start = 0
		}
		end := lineNum + contextLines
		if end > len(lines) {
			end = len(lines)
		}
		for i := start; i < end; i++ {
			if i+1 == lineNum {
				continue // already shown as Snippet
			}
			marker := "    "
			if i+1 == lineNum {
				marker = "  >>"
			}
			fmt.Fprintf(b, "%s %4d  %s\n", marker, i+1, lines[i])
		}
		return
	}

	// For Go source files, read from disk
	dir := findMicrolispDir()
	if dir == "" {
		return
	}
	f, err := os.Open(filepath.Join(dir, filename))
	if err != nil {
		return
	}
	defer f.Close()

	var allLines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	start := lineNum - contextLines
	if start < 1 {
		start = 1
	}
	end := lineNum + contextLines
	if end > len(allLines) {
		end = len(allLines)
	}

	for i := start - 1; i < end; i++ {
		if i+1 == lineNum {
			continue // already shown as Snippet
		}
		marker := "    "
		fmt.Fprintf(b, "%s %4d  %s\n", marker, i+1, allLines[i])
	}
}