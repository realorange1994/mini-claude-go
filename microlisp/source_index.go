package microlisp

import (
	"bufio"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// SourceEntry holds location info for a function.
type SourceEntry struct {
	Kind   string // "builtin", "stdlib", "special", "helper", "go-ffi"
	File   string // Go source file or "stdlib.go" (absolute for go-ffi)
	Start  int    // 1-based line number
	End    int    // 1-based line number
	Doc    string // brief description
	GoFunc string // Go function name (for builtins/helpers/go-ffi)
	GoSrc  string // relative Go source path from GOROOT/src (for go-ffi)
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

	// Index Go FFI functions from GoPackageRegistry
	indexGoFFIFunctions()
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

// indexGoFFIFunctions scans GoPackageRegistry and registers each function
// in sourceIndex with Kind "go-ffi".
// Source paths are stored as relative paths (e.g. "math/sin.go") resolved
// at runtime via GOROOT/src/, so they work across platforms and toolchain versions.
func indexGoFFIFunctions() {
	for pkgName, pkg := range GoPackageRegistry {
		for symName, fnVal := range pkg {
			if fnVal.Kind() != reflect.Func {
				continue
			}
			fn := runtime.FuncForPC(fnVal.Pointer())
			if fn == nil {
				continue
			}
			file, line := fn.FileLine(fn.Entry())
			file = filepath.ToSlash(file)

			// Extract relative path from GOROOT/src/ by finding the "/src/"
			// separator in the compiled path. This works regardless of where
			// GOROOT actually lives (system install, toolchain in mod cache, etc.)
			// e.g. ".../go1.25.0/src/math/sin.go" → "math/sin.go"
			relFile := file
			if idx := strings.Index(file, "/src/"); idx >= 0 {
				relFile = file[idx+5:] // skip "/src/"
			}

			// Only index standard library files (those with a recognizable
			// relative path that starts with a known stdlib package)
			if relFile == file {
				continue // no "/src/" found, skip (vendor, third-party, etc.)
			}

			sig := fnVal.Type()
			params := make([]string, sig.NumIn())
			for i := 0; i < sig.NumIn(); i++ {
				params[i] = sig.In(i).String()
			}
			rets := make([]string, sig.NumOut())
			for i := 0; i < sig.NumOut(); i++ {
				rets[i] = sig.Out(i).String()
			}
			fullName := pkgName + "." + symName
			sourceIndex[strings.ToLower(fullName)] = &SourceEntry{
				Kind:   "go-ffi",
				File:   relFile, // relative path like "math/sin.go"
				Start:  line,
				End:    line,
				Doc:    fmt.Sprintf("Go stdlib: %s(%s) → %s", symName, strings.Join(params, ", "), strings.Join(rets, ", ")),
				GoFunc: fullName,
				GoSrc:  relFile, // same relative path for display
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

	case "go-ffi":
		if entry.GoFunc != "" {
			fmt.Fprintf(&b, "Go function: %s\n", entry.GoFunc)
		}
		if entry.GoSrc != "" {
			fmt.Fprintf(&b, "Source: %s\n", entry.GoSrc)
		}
		fmt.Fprintf(&b, "%s\n", entry.Doc)
		if entry.Start > 0 {
			fmt.Fprintf(&b, "Line: %d\n", entry.Start)
			// Resolve the relative path dynamically using runtime GOROOT
			resolved := resolveGoSrcPath(entry.File)
			if resolved != "" {
				fmt.Fprintf(&b, "\n%s", readAbsSourceSnippet(resolved, entry.Start, offset, limit))
			} else {
				fmt.Fprintf(&b, "\nGo source not found on this system. Function signature is available above.\n")
			}
		}
	}

	return b.String()
}

// GetDefine returns the function signature (parameter list, return types) for any function.
// Works for: builtins, stdlib functions, special forms, Go FFI functions.
func GetDefine(name string) string {
	name = strings.ToLower(name)

	// 1. Try exact match in source index
	entry, ok := sourceIndex[name]
	if !ok {
		// Try prefix match
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
			return fmt.Sprintf("No function definition found for: %q\n\nTry (source-list) to see all available functions.", name)
		}
	}

	var b strings.Builder
	switch entry.Kind {
	case "builtin":
		fmt.Fprintf(&b, "=== %s (builtin) ===\n\n", name)
		// Try builtinDocMap for signature
		if doc, ok := builtinDocMap[name]; ok {
			// Extract signature part (before em-dash)
			if idx := strings.Index(doc, "\u2014"); idx > 0 {
				fmt.Fprintf(&b, "%s\n\n%s\n", strings.TrimSpace(doc[:idx]), strings.TrimSpace(doc[idx+1:]))
			} else {
				fmt.Fprintf(&b, "%s\n", doc)
			}
		} else {
			// Try to extract from Go source: find the builtin func and show it accepts &[*Value] args
			if entry.GoFunc != "" {
				fmt.Fprintf(&b, "(%s &rest args) — Builtin function (Go: %s)\n\n", name, entry.GoFunc)
				fmt.Fprintf(&b, "Accepts any number of arguments, type-checked at call time.\n")
			} else {
				fmt.Fprintf(&b, "(%s &rest args) — Builtin function\n\n", name)
			}
		}
		fmt.Fprintf(&b, "Implementation: microlisp/%s (lines %d-%d)\n", entry.File, entry.Start, entry.End)

	case "special":
		fmt.Fprintf(&b, "=== %s (special form) ===\n\n", name)
		if doc, ok := builtinDocMap[name]; ok {
			fmt.Fprintf(&b, "%s\n", doc)
		} else {
			fmt.Fprintf(&b, "Special form: %s (handled directly by evaluator)\n", name)
		}
		fmt.Fprintf(&b, "Implementation: microlisp/eval_core.go")
		if entry.Start > 0 {
			fmt.Fprintf(&b, " (lines %d-%d)", entry.Start, entry.End)
		}
		fmt.Fprintf(&b, "\n\nNote: Special forms do not evaluate all arguments — control flow is built-in.\n")

	case "stdlib":
		fmt.Fprintf(&b, "=== %s (stdlib) ===\n\n", name)
		// Extract define/defun form from stdlib source
		defineForm := extractDefineForm(name)
		if defineForm != "" {
			fmt.Fprintf(&b, "%s\n", defineForm)
		} else {
			fmt.Fprintf(&b, "(%s ...) — Stdlib function\n", name)
		}
		fmt.Fprintf(&b, "Implementation: stdlib.go (lines %d-%d)\n", entry.Start, entry.End)

	case "go-ffi":
		fmt.Fprintf(&b, "=== %s (go-ffi) ===\n\n", name)
		if entry.GoFunc != "" {
			fmt.Fprintf(&b, "Go function: %s\n", entry.GoFunc)
		}
		if entry.GoSrc != "" {
			fmt.Fprintf(&b, "Source: %s:%d\n\n", entry.GoSrc, entry.Start)
		}
		// Extract signature from GoPackageRegistry
		sig := getFFISignature(name)
		if sig != "" {
			fmt.Fprintf(&b, "%s\n", sig)
		} else {
			fmt.Fprintf(&b, "%s\n", entry.Doc)
		}
		fmt.Fprintf(&b, "\nUsage in Lisp: ((ffi %q) arg1 arg2 ...)\n     or: ((go:import %q) arg1 arg2 ...)\n", entry.GoFunc, entry.GoFunc)

	case "helper":
		fmt.Fprintf(&b, "=== %s (helper) ===\n\n", name)
		if entry.GoFunc != "" {
			fmt.Fprintf(&b, "Go function: %s\n", entry.GoFunc)
		}
		fmt.Fprintf(&b, "Implementation: microlisp/%s (lines %d-%d)\n", entry.File, entry.Start, entry.End)
	}

	return b.String()
}

// extractDefineForm extracts the first line of a stdlib function definition
// from the embedded stdlib content, showing the function name and parameters.
func extractDefineForm(name string) string {
	// Match define forms: (define (name params...), (defun name (params...),
	// (define-macro (name params...), etc.
	pattern := fmt.Sprintf(`(?i)\((?:define|define-macro|defun|define-condition|defgeneric)\s+\(?\s*%s(?:\s|[\(\)\s])`, regexp.QuoteMeta(name))
	lines := strings.Split(stdlibContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if matched, _ := regexp.MatchString(pattern, trimmed); matched {
			// Extract just the signature part — up to the first body expression
			// Count parens to find where the arg list ends
			depth := 0
			inArgList := false
			var sb strings.Builder
			for i, ch := range trimmed {
				if ch == '(' {
					depth++
					if depth == 2 {
						inArgList = true
					}
				}
				if inArgList && depth == 1 && ch == ')' {
					// End of arg list, include this ) and stop
					sb.WriteRune(ch)
					// Check if there's more on this line after the arg list
					rest := strings.TrimSpace(trimmed[i+1:])
					if rest != "" && !strings.HasPrefix(rest, ")") {
						sb.WriteString(" ...")
					}
					break
				}
				if depth == 0 && ch == ')' {
					// Special case: (define name) with no args (constant)
					sb.WriteRune(ch)
					break
				}
				sb.WriteRune(ch)
			}
			if sb.Len() > 0 {
				return sb.String()
			}
			// Fallback: return the full first part
			if idx := strings.Index(trimmed, ")"); idx > 0 {
				// Check if this is just a simple define with no args
				inner := strings.TrimSpace(trimmed[7:idx]) // skip "(define "
				parts := strings.Fields(inner)
				if len(parts) == 1 {
					return trimmed[:idx+1] // just (define name)
				}
			}
			return trimmed
		}
	}
	return ""
}

// getFFISignature extracts the Go function signature from GoPackageRegistry
// for a given FFI function name (e.g. "math.sin").
func getFFISignature(name string) string {
	// name is like "math.sin" (lowercase)
	// Need to find the original case-sensitive pkg/sym name
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	lowerPkg := parts[0]
	lowerSym := parts[1]

	// Search GoPackageRegistry for matching (case-insensitive)
	for pkgName, pkg := range GoPackageRegistry {
		if strings.ToLower(pkgName) != lowerPkg {
			continue
		}
		for symName, fnVal := range pkg {
			if strings.ToLower(symName) != lowerSym {
				continue
			}
			if fnVal.Kind() != reflect.Func {
				return fmt.Sprintf("  %s.%s — not a function", pkgName, symName)
			}
			sig := fnVal.Type()
			params := make([]string, sig.NumIn())
			for i := 0; i < sig.NumIn(); i++ {
				params[i] = sig.In(i).String()
			}
			returns := make([]string, sig.NumOut())
			for i := 0; i < sig.NumOut(); i++ {
				returns[i] = sig.Out(i).String()
			}
			var isVariadic string
			if sig.IsVariadic() {
				isVariadic = " (variadic)"
			}
			return fmt.Sprintf("  func %s.%s(%s)%s → (%s)",
				pkgName, symName,
				strings.Join(params, ", "),
				isVariadic,
				strings.Join(returns, ", "))
		}
	}
	return ""
}

// resolveGoSrcPath resolves a relative Go stdlib source path (e.g. "math/sin.go")
// to an absolute path by searching multiple candidate locations:
//   1. runtime.GOROOT()/src/<relPath>  — standard Go install
//   2. The compiled path from FuncForPC — toolchain in mod cache
//   3. Common alternative locations
// Returns "" if the file cannot be found.
func resolveGoSrcPath(relPath string) string {
	goroot := filepath.ToSlash(runtime.GOROOT())

	// Candidate directories to search for Go stdlib source
	candidates := []string{
		goroot + "/src",
	}

	// Also try the compiled path from FuncForPC as a hint.
	// If we can find any FFI function's compiled path, extract the src/ prefix
	// to discover the actual toolchain source directory.
	for _, pkg := range GoPackageRegistry {
		for _, fnVal := range pkg {
			if fnVal.Kind() != reflect.Func {
				continue
			}
			fn := runtime.FuncForPC(fnVal.Pointer())
			if fn == nil {
				continue
			}
			compiledFile, _ := fn.FileLine(fn.Entry())
			compiledFile = filepath.ToSlash(compiledFile)
			if idx := strings.Index(compiledFile, "/src/"); idx >= 0 {
				srcDir := compiledFile[:idx+4] // include "/src/"
				// Deduplicate
				found := false
				for _, c := range candidates {
					if c == srcDir {
						found = true
						break
					}
				}
				if !found {
					candidates = append(candidates, srcDir)
				}
			}
			break // one hint is enough
		}
		break // one package is enough
	}

	// Search each candidate directory
	for _, srcDir := range candidates {
	 fullPath := filepath.Join(filepath.FromSlash(srcDir), filepath.FromSlash(relPath))
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}
	return ""
}

// readAbsSourceSnippet reads lines from an absolute-path Go source file (for go-ffi entries).
// It reads from startLine, showing up to `limit` lines (default 40 if limit==0).
func readAbsSourceSnippet(filename string, startLine, offset, limit int) string {
	if limit == 0 {
		limit = 40 // default: show 40 lines for Go stdlib functions
	}
	f, err := os.Open(filename)
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
		if lineNum >= startLine {
			snippetLine := lineNum - startLine
			if snippetLine < offset {
				continue
			}
			if shown >= limit {
				break
			}
			fmt.Fprintf(&b, "%4d  %s\n", lineNum, scanner.Text())
			shown++
		}
	}
	if shown >= limit {
		fmt.Fprintf(&b, "\n  ... more lines (use offset=%d, limit=%d to continue)\n",
			offset+shown, limit)
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