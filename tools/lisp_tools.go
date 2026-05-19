package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"miniclaudecode-go/microlisp"
)

// LispToolsTool provides a backup toolset that uses the Lisp interpreter
// for core file/text operations when Go-native tools are unavailable.
type LispToolsTool struct{}

// lispToolsLoaded tracks whether lispToolsLib has been loaded into globalEnv.
// ResetGlobalEnv clears all function definitions, so we need to reload.
// Call ResetLispToolsState() after any ResetGlobalEnv call.
var lispToolsLoaded bool
var lispToolsMu sync.Mutex

// ResetLispToolsState clears the library loaded flag so the library
// will be reloaded on the next ExecuteContext call. Call this after
// any microlisp.ResetGlobalEnv() to ensure stdlib functions are restored.
func ResetLispToolsState() {
	lispToolsMu.Lock()
	defer lispToolsMu.Unlock()
	lispToolsLoaded = false
}

// ensureLispToolsLoaded loads lispToolsLib if not already loaded.
// It is context-aware: if the context is cancelled while waiting for evalMu,
// it returns an error instead of blocking indefinitely.
func ensureLispToolsLoaded(ctx context.Context) error {
	lispToolsMu.Lock()
	if lispToolsLoaded {
		lispToolsMu.Unlock()
		return nil
	}
	lispToolsMu.Unlock()

	// Load in a goroutine so we can respect context cancellation.
	// SafeEvalString acquires evalMu; if held by another caller, this
	// would block indefinitely without the goroutine + select pattern.
	ch := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Errorf("panic loading lispToolsLib: %v", r)
			}
		}()
		_, err := microlisp.SafeEvalString(lispToolsLib)
		ch <- err
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("lisp_tools lib load timed out: %v", ctx.Err())
	case err := <-ch:
		if err == nil {
			lispToolsMu.Lock()
			lispToolsLoaded = true
			lispToolsMu.Unlock()
		}
		return err
	}
}

func (*LispToolsTool) Name() string { return "lisp_tools" }

func (*LispToolsTool) Description() string {
	return "Backup file/text toolset powered by the Lisp interpreter. " +
		"Provides read, write, edit, multi_edit, list, search, glob, mkdir, rm, mv, cp operations. " +
		"Use when Go-native tools (read_file, write_file, etc.) are unavailable. " +
		"All logic runs in the Lisp interpreter with resource limits. " +
		"Search uses substring matching (no regex)."
}

func (*LispToolsTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"read", "write", "edit", "multi_edit", "list", "search", "glob", "mkdir", "rm", "mv", "cp"},
				"description": "Operation to perform: read (read file), write (write file), edit (string replace), multi_edit (batch edits), list (directory listing), search (text search), glob (file matching), mkdir (create directory), rm (delete file), mv (move/rename), cp (copy file).",
			},
			"file_path": map[string]any{
				"type":        "string",
				"description": "File path for read, write, edit, multi_edit operations.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write (for write operation).",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Text to find and replace (for edit operation).",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Replacement text (for edit operation).",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace all occurrences of old_string (default: false, for edit).",
			},
			"edits": map[string]any{
				"type":        "array",
				"description": "Array of {old_string, new_string, replace_all?} for multi_edit operation.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Search root directory (required for list, search, glob, mkdir, rm; defaults to '.' for search and glob if omitted).",
			},
			"pattern": map[string]any{
				"type":        "string",
				"description": "Required for search (substring) and glob (shell wildcard pattern, e.g. '*.go').",
			},
			"recursive": map[string]any{
				"type":        "boolean",
				"description": "Recurse into subdirectories (for list and glob, default: false).",
			},
			"max_entries": map[string]any{
				"type":        "integer",
				"description": "Max entries to return (for list, default: 200).",
			},
			"show_hidden": map[string]any{
				"type":        "boolean",
				"description": "Include hidden/dot files (for list, default: false).",
			},
			"case_insensitive": map[string]any{
				"type":        "boolean",
				"description": "Case-insensitive search (for search, default: false).",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"enum":        []string{"content", "files_with_matches", "count"},
				"description": "Search output format (for search, default: content).",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "1-based line number to start reading (for read, default: 1).",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max lines to read (for read, default: 2000).",
			},
			"destination": map[string]any{
				"type":        "string",
				"description": "Destination path (for mv and cp operations).",
			},
			"head_limit": map[string]any{
				"type":        "integer",
				"description": "Max results to return (for glob and search, default: 100).",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "File name filter for search, e.g. '*.go'.",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *LispToolsTool) CheckPermissions(params map[string]any) PermissionResult {
	op, _ := params["operation"].(string)
	switch op {
	case "write", "edit", "multi_edit", "mkdir":
		path, _ := params["file_path"].(string)
		if path == "" {
			path, _ = params["path"].(string)
		}
		return CheckPathSafetyForAutoEdit(path)
	default:
		return PermissionResultPassthrough()
	}
}

func (t *LispToolsTool) ExecuteContext(ctx context.Context, params map[string]any) (result ToolResult) {
	defer func() {
		if r := recover(); r != nil {
			result = ToolResult{Output: fmt.Sprintf("Error: lisp_tools panic: %v", r), IsError: true}
		}
	}()

	select {
	case <-ctx.Done():
		return ToolResult{Output: fmt.Sprintf("Error: lisp_tools timed out: %v", ctx.Err()), IsError: true}
	default:
	}

	// Load helper library (idempotent: loaded once, reloaded after ResetGlobalEnv)
	if err := ensureLispToolsLoaded(ctx); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: failed to load lisp_tools library: %v", err), IsError: true}
	}

	op, _ := params["operation"].(string)
	if op == "" {
		return ToolResult{Output: "Error: operation is required", IsError: true}
	}

	switch op {
	case "read":
		return t.doRead(ctx, params)
	case "write":
		return t.doWrite(ctx, params)
	case "edit":
		return t.doEdit(ctx, params)
	case "multi_edit":
		return t.doMultiEdit(ctx, params)
	case "list":
		return t.doList(ctx, params)
	case "search":
		return t.doSearch(ctx, params)
	case "glob":
		return t.doGlob(ctx, params)
	case "mkdir":
		return t.doMkdir(ctx, params)
	case "rm":
		return t.doRm(ctx, params)
	case "mv":
		return t.doMv(ctx, params)
	case "cp":
		return t.doCp(ctx, params)
	default:
		return ToolResult{Output: fmt.Sprintf("Error: unknown operation: %s", op), IsError: true}
	}
}

func (t *LispToolsTool) Execute(params map[string]any) ToolResult {
	return t.ExecuteContext(context.Background(), params)
}

// -------- Operation implementations --------

func (t *LispToolsTool) doRead(ctx context.Context, params map[string]any) ToolResult {
	path, _ := params["file_path"].(string)
	if path == "" {
		return ToolResult{Output: "Error: file_path is required for read", IsError: true}
	}
	offset := 1
	if v, ok := paramInt(params["offset"]); ok {
		offset = v
	}
	limit := 2000
	if v, ok := paramInt(params["limit"]); ok {
		limit = v
	}
	expr := fmt.Sprintf(`(lisp-read-file %s %d %d)`, lispStr(path), offset, limit)
	return t.evalCapture(ctx, expr)
}

func (t *LispToolsTool) doWrite(ctx context.Context, params map[string]any) ToolResult {
	path, _ := params["file_path"].(string)
	if path == "" {
		return ToolResult{Output: "Error: file_path is required for write", IsError: true}
	}
	content, hasContent := params["content"]
	if !hasContent {
		return ToolResult{Output: "Error: content is required for write", IsError: true}
	}
	contentStr, _ := content.(string)
	expr := fmt.Sprintf(`(lisp-write-file %s %s)`, lispStr(path), lispStr(contentStr))
	return t.evalVoid(ctx, expr)
}

func (t *LispToolsTool) doEdit(ctx context.Context, params map[string]any) ToolResult {
	path, _ := params["file_path"].(string)
	if path == "" {
		return ToolResult{Output: "Error: file_path is required for edit", IsError: true}
	}
	oldStr, _ := params["old_string"].(string)
	newStr, _ := params["new_string"].(string)
	if oldStr == "" {
		return ToolResult{Output: "Error: old_string is required for edit", IsError: true}
	}
	replaceAll := "nil"
	if v, ok := params["replace_all"].(bool); ok && v {
		replaceAll = "t"
	}
	expr := fmt.Sprintf(`(lisp-edit-file %s %s %s %s)`, lispStr(path), lispStr(oldStr), lispStr(newStr), replaceAll)
	return t.evalVoid(ctx, expr)
}

func (t *LispToolsTool) doMultiEdit(ctx context.Context, params map[string]any) ToolResult {
	path, _ := params["file_path"].(string)
	if path == "" {
		return ToolResult{Output: "Error: file_path is required for multi_edit", IsError: true}
	}
	editsVal, ok := params["edits"]
	if !ok {
		return ToolResult{Output: "Error: edits array is required for multi_edit", IsError: true}
	}
	editsArr, ok := editsVal.([]any)
	if !ok || len(editsArr) == 0 {
		return ToolResult{Output: "Error: edits must be a non-empty array", IsError: true}
	}
	var editItems []string
	for _, e := range editsArr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		oldS, _ := m["old_string"].(string)
		newS, _ := m["new_string"].(string)
		ra := "nil"
		if v, ok := m["replace_all"].(bool); ok && v {
			ra = "t"
		}
		editItems = append(editItems, fmt.Sprintf(`(list %s %s %s)`, lispStr(oldS), lispStr(newS), ra))
	}
	editsList := fmt.Sprintf(`(list %s)`, strings.Join(editItems, " "))
	expr := fmt.Sprintf(`(lisp-multi-edit %s %s)`, lispStr(path), editsList)
	return t.evalVoid(ctx, expr)
}

func (t *LispToolsTool) doList(ctx context.Context, params map[string]any) ToolResult {
	path, _ := params["path"].(string)
	if path == "" {
		path = "."
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: no such directory: %s", path), IsError: true}
	}
	recursive := "nil"
	if v, ok := params["recursive"].(bool); ok && v {
		recursive = "t"
	}
	maxEntries := 200
	if v, ok := paramInt(params["max_entries"]); ok {
		maxEntries = v
	}
	showHidden := "nil"
	if v, ok := params["show_hidden"].(bool); ok && v {
		showHidden = "t"
	}
	expr := fmt.Sprintf(`(lisp-list-dir %s %s %d %s)`, lispStr(path), recursive, maxEntries, showHidden)
	return t.evalCapture(ctx, expr)
}

func (t *LispToolsTool) doSearch(ctx context.Context, params map[string]any) ToolResult {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return ToolResult{Output: "Error: pattern is required for search", IsError: true}
	}
	path, _ := params["path"].(string)
	if path == "" {
		path = "."
	}
	outputMode := "content"
	if v, ok := params["output_mode"].(string); ok && v != "" {
		outputMode = v
	}
	caseInsensitive := "nil"
	if v, ok := params["case_insensitive"].(bool); ok && v {
		caseInsensitive = "t"
	}
	headLimit := 250
	if v, ok := paramInt(params["head_limit"]); ok {
		headLimit = v
	}
	globFilter := lispStr("")
	if v, ok := params["glob"].(string); ok && v != "" {
		globFilter = lispStr(v)
	}
	expr := fmt.Sprintf(`(lisp-search %s %s %s %s %d %s)`,
		lispStr(pattern), lispStr(path), lispStr(outputMode), caseInsensitive, headLimit, globFilter)
	return t.evalCapture(ctx, expr)
}

func (t *LispToolsTool) doGlob(ctx context.Context, params map[string]any) ToolResult {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return ToolResult{Output: "Error: pattern is required for glob", IsError: true}
	}
	path, _ := params["path"].(string)
	if path == "" {
		path = "."
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: no such directory: %s", path), IsError: true}
	}
	headLimit := 100
	if v, ok := paramInt(params["head_limit"]); ok {
		headLimit = v
	}
	expr := fmt.Sprintf(`(lisp-glob %s %s %d)`, lispStr(pattern), lispStr(path), headLimit)
	return t.evalCapture(ctx, expr)
}

func (t *LispToolsTool) doMkdir(ctx context.Context, params map[string]any) ToolResult {
	path, _ := params["path"].(string)
	if path == "" {
		return ToolResult{Output: "Error: path is required for mkdir", IsError: true}
	}
	recursive := "nil"
	if v, ok := params["recursive"].(bool); ok && v {
		recursive = "t"
	}
	expr := fmt.Sprintf(`(lisp-mkdir %s %s)`, lispStr(path), recursive)
	return t.evalVoid(ctx, expr)
}

func (t *LispToolsTool) doRm(ctx context.Context, params map[string]any) ToolResult {
	path, _ := params["path"].(string)
	if path == "" {
		path, _ = params["file_path"].(string)
	}
	if path == "" {
		return ToolResult{Output: "Error: path is required for rm", IsError: true}
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: no such file or directory: %s", path), IsError: true}
	}
	expr := fmt.Sprintf(`(lisp-rm %s)`, lispStr(path))
	return t.evalVoid(ctx, expr)
}

func (t *LispToolsTool) doMv(ctx context.Context, params map[string]any) ToolResult {
	src, _ := params["file_path"].(string)
	if src == "" {
		src, _ = params["path"].(string)
	}
	dst, _ := params["destination"].(string)
	if src == "" || dst == "" {
		return ToolResult{Output: "Error: file_path and destination are required for mv", IsError: true}
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: no such file or directory: %s", src), IsError: true}
	}
	expr := fmt.Sprintf(`(lisp-mv %s %s)`, lispStr(src), lispStr(dst))
	return t.evalVoid(ctx, expr)
}

func (t *LispToolsTool) doCp(ctx context.Context, params map[string]any) ToolResult {
	src, _ := params["file_path"].(string)
	if src == "" {
		src, _ = params["path"].(string)
	}
	dst, _ := params["destination"].(string)
	if src == "" || dst == "" {
		return ToolResult{Output: "Error: file_path and destination are required for cp", IsError: true}
	}
	expr := fmt.Sprintf(`(lisp-cp %s %s)`, lispStr(src), lispStr(dst))
	return t.evalVoid(ctx, expr)
}

// -------- Unquote helper --------

// unquoteLispString strips outer quotes from a Lisp string literal
// returned by SafeEvalWithLimits ToString(), e.g. "\"ok\"" -> "ok".
func unquoteLispString(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		inner := s[1 : len(s)-1]
		// Unescape common Lisp string escapes
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner
	}
	return s
}

// paramInt extracts an int from a map value (handles both int and float64 from JSON).
func paramInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	}
	return 0, false
}

// -------- Eval helpers --------

func (t *LispToolsTool) evalCapture(ctx context.Context, expr string) ToolResult {
	limits := microlisp.DefaultLimits()
	// Wire context cancellation into CancelChan so stepCheck() aborts
	// mid-evaluation when ctx.Done() fires, releasing evalMu promptly.
	cancelChan := microlisp.NewCancelChannel()
	limits.CancelChan = cancelChan

	ch := make(chan evalResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
			}
		}()
		captured, ret, err := microlisp.SafeEvalStringCaptureWithLimits(expr, limits)
		if err != nil {
			ch <- evalResult{"", err}
			return
		}
		output := captured
		if ret != "" && ret != "NIL" {
			// Strip outer quotes from Lisp string literals
			ret = unquoteLispString(ret)
			if output != "" {
				output += "\n" + ret
			} else {
				output = ret
			}
		}
		ch <- evalResult{output, nil}
	}()
	select {
	case <-ctx.Done():
		close(cancelChan) // Trigger stepCheck() abort to release evalMu
		return ToolResult{Output: "Error: lisp_tools timed out", IsError: true}
	case r := <-ch:
		if r.err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v", r.err), IsError: true}
		}
		result := r.output
		if result == "" {
			result = "NIL"
		}
		// Check if the Lisp function returned an error string
		if strings.HasPrefix(result, "Error:") {
			return ToolResult{Output: result, IsError: true}
		}
		return ToolResult{Output: result}
	}
}

func (t *LispToolsTool) evalVoid(ctx context.Context, expr string) ToolResult {
	limits := microlisp.DefaultLimits()
	// Wire context cancellation into CancelChan so stepCheck() aborts
	// mid-evaluation when ctx.Done() fires, releasing evalMu promptly.
	cancelChan := microlisp.NewCancelChannel()
	limits.CancelChan = cancelChan

	ch := make(chan evalResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
			}
		}()
		result, err := microlisp.SafeEvalWithLimits(expr, limits)
		ch <- evalResult{result, err}
	}()
	select {
	case <-ctx.Done():
		close(cancelChan) // Trigger stepCheck() abort to release evalMu
		return ToolResult{Output: "Error: lisp_tools timed out", IsError: true}
	case r := <-ch:
		if r.err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v", r.err), IsError: true}
		}
		result := r.output
		if result == "" {
			result = "ok"
		} else {
			// Strip outer quotes from Lisp string literals returned by SafeEvalWithLimits
			result = unquoteLispString(result)
		}
		// Check if the Lisp function returned an error string
		if strings.HasPrefix(result, "Error:") {
			return ToolResult{Output: result, IsError: true}
		}
		return ToolResult{Output: result}
	}
}

// -------- Lisp string escaping --------

// lispStr returns a Lisp string literal with proper escaping.
func lispStr(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// -------- Embedded Lisp helper library --------
// NOTE: microlisp reader upcases all symbols. All identifiers must be lowercase
// in source (reader upcases them). No named-let (let loop) — use define for
// recursion. No let* — use nested let. No &optional — use fixed params.

const lispToolsLib = `
;; ===== Lisp Tools Helper Library =====

;; String join helper
(define (string-join lst sep)
  (if (null lst) ""
      (if (null (cdr lst)) (car lst)
          (string-append (car lst) sep (string-join (cdr lst) sep)))))

;; Split string by delimiter (recursive helper)
(define (split-string str delim)
  (let ((dlen (string-length delim)))
    (if (= dlen 0) (list str)
        (split-string-loop str delim dlen '()))))

(define (split-string-loop s delim dlen acc)
  (let ((pos (string-find delim s)))
    (if (not pos)
        (reverse (cons s acc))
        (split-string-loop (substring s (+ pos dlen)) delim dlen
                           (cons (substring s 0 pos) acc)))))

;; Count substring occurrences
(define (count-substring str sub)
  (let ((slen (string-length sub)))
    (if (= slen 0) 0
        (count-substring-loop str sub slen 0))))

(define (count-substring-loop s sub slen count)
  (let ((pos (string-find sub s)))
    (if (not pos) count
        (count-substring-loop (substring s (+ pos slen)) sub slen (+ count 1)))))

;; Replace first N occurrences (N=-1 means all)
(define (replace-first-n str old new n)
  (let ((olen (string-length old)))
    (if (= olen 0) str
        (replace-first-n-loop str old new n olen "" 0))))

(define (replace-first-n-loop s old new n olen result count)
  (if (and (> n 0) (>= count n))
      (string-append result s)
      (let ((pos (string-find old s)))
        (if (not pos)
            (string-append result s)
            (replace-first-n-loop (substring s (+ pos olen)) old new n olen
                                  (string-append result (substring s 0 pos) new)
                                  (+ count 1))))))

;; Read entire file as string
;; NOTE: read-line returns a list (line eof-flag). At EOF this is ("" #f).
;; We MUST extract the actual flag with (car (cdr ...)) since (cdr result)
;; returns a cons cell which is always truthy in Lisp.
(define (read-file-fully path)
  (let ((stream (open path :direction :input)))
    (read-file-fully-loop stream '())))

(define (read-file-fully-loop stream lines)
  (let ((result (read-line stream nil)))
    (if (car (cdr result))
        (read-file-fully-loop stream (cons (car result) lines))
        (if (equal (car result) "")
            (progn (close stream) (string-join (reverse lines) "\n"))
            (progn (close stream) (string-join (reverse (cons (car result) lines)) "\n"))))))

;; Read file lines as list
(define (read-file-lines path)
  (let ((stream (open path :direction :input)))
    (read-file-lines-loop stream '())))

(define (read-file-lines-loop stream lines)
  (let ((result (read-line stream nil)))
    (if (car (cdr result))
        (read-file-lines-loop stream (cons (car result) lines))
        (if (equal (car result) "")
            (progn (close stream) (reverse lines))
            (progn (close stream) (reverse (cons (car result) lines)))))))

;; Format lines with line numbers
(define (format-lines lines start)
  (format-lines-loop lines start '()))

(define (format-lines-loop lst num acc)
  (if (null lst)
      (string-join (reverse acc) "\n")
      (format-lines-loop (cdr lst) (+ num 1)
                         (cons (format nil "~A\t~A" num (car lst)) acc))))

;; Path safety check
(define (safe-path? path)
  (let ((lp (string-downcase path)))
    (if (string-find "/dev/" lp) (error "unsafe path: device file")
        (if (string-find "\\dev\\" lp) (error "unsafe path: device file")
            (if (string-find "/proc/" lp) (error "unsafe path: proc filesystem")
                (if (string-find "\\proc\\" lp) (error "unsafe path: proc filesystem")
                    path))))))

;; ===== Core Tool Functions =====

;; Read file with optional offset and limit
(define (lisp-read-file path offset limit)
  (handler-case
    (let ((safe (safe-path? path)))
      (let ((lines (read-file-lines safe)))
        (let ((total (length lines)))
          (let ((start (max 1 offset)))
            (let ((end (min total (+ (- start 1) limit))))
              (let ((sliced (subseq lines (- start 1) end)))
                (format-lines sliced start)))))))
    (condition (c) (format nil "Error: ~A" c))))

;; Write content to file, creating parent dirs
(define (lisp-write-file path content)
  (handler-case
    (progn
      (ensure-directories-exist (safe-path? path))
      (let ((stream (open path :direction :output :if-exists :supersede)))
        (write-string content stream)
        (close stream))
      "ok")
    (condition (c) (format nil "Error: ~A" c))))

;; Edit file: exact string replacement
(define (lisp-edit-file path old-str new-str replace-all)
  (handler-case
    (let ((safe (safe-path? path)))
      (let ((content (read-file-fully safe)))
        (let ((count (count-substring content old-str)))
          (if (= count 0)
              (error "old_string not found in file")
              (if (and (not replace-all) (> count 1))
                  (error "old_string found multiple times; set replace_all=true to replace all")
                  (progn
                    (lisp-write-file path
                      (replace-first-n content old-str new-str (if replace-all -1 1)))
                    "ok"))))))
    (condition (c) (format nil "Error: ~A" c))))

;; Multi-edit: apply batch edits
(define (lisp-multi-edit path edits)
  (handler-case
    (let ((original (read-file-fully (safe-path? path))))
      (lisp-multi-edit-loop path original edits))
    (condition (c) (format nil "Error: ~A" c))))

(define (lisp-multi-edit-loop path content remaining)
  (if (null remaining)
      (progn (lisp-write-file path content) "ok")
      (let ((edit (car remaining)))
        (let ((old-str (first edit)))
          (let ((new-str (second edit)))
            (let ((ra (third edit)))
              (let ((count (count-substring content old-str)))
                (if (= count 0)
                    (error (format nil "old_string not found: ~A..." (substring old-str 0 (min 40 (string-length old-str)))))
                    (if (and (not ra) (> count 1))
                        (error (format nil "old_string found ~D times; set replace_all=true" count))
                        (lisp-multi-edit-loop path
                          (replace-first-n content old-str new-str (if ra -1 1))
                          (cdr remaining)))))))))))

;; Directory listing
(define (lisp-list-dir path recursive max-entries show-hidden)
  (handler-case
    (let ((entries (path-glob (path-join (safe-path? path) "*"))))
      (let ((filtered (if show-hidden entries
                          (remove-if (lambda (p)
                                       (let ((name (path-base p)))
                                         (and (> (string-length name) 0)
                                              (char= (char name 0) #\.))))
                                     entries))))
        (let ((limited (subseq filtered 0 (min (length filtered) max-entries))))
          (string-join limited "\n"))))
    (condition (c) (format nil "Error: ~A" c))))

;; Text search (substring matching, no regex)
;; Delegates to go-search -> rgrep engine for efficient file walking, binary detection,
;; and line scanning. Respects .gitignore, skips VCS/binary files.
(define (lisp-search pattern path output-mode case-insensitive head-limit glob-filter)
  (handler-case
    (let ((safe (safe-path? path)))
      (let ((glob (if (and (stringp glob-filter) (> (string-length glob-filter) 0))
                      glob-filter
                      "")))
        (go-search pattern safe output-mode case-insensitive head-limit glob)))
    (condition (c) (format nil "Error: ~A" c))))

;; Glob file matching
(define (lisp-glob pattern path head-limit)
  (handler-case
    (let ((full-pattern (path-join (safe-path? path) pattern)))
      (let ((results (path-glob full-pattern)))
        (let ((limited (subseq results 0 (min (length results) head-limit))))
          (string-join limited "\n"))))
    (condition (c) (format nil "Error: ~A" c))))

;; Create directory
;; ensure-directories-exist creates all intermediate dirs when path ends with /.
;; For both recursive and non-recursive, we create all parent dirs plus the target.
(define (lisp-mkdir path recursive)
  (handler-case
    (progn
      (ensure-directories-exist (string-append (safe-path? path) "/"))
      "ok")
    (condition (c) (format nil "Error: ~A" c))))

;; Delete file
(define (lisp-rm path)
  (handler-case
    (progn (delete-file (safe-path? path)) "ok")
    (condition (c) (format nil "Error: ~A" c))))

;; Move/rename file
(define (lisp-mv src dst)
  (handler-case
    (progn (rename-file (safe-path? src) (safe-path? dst)) "ok")
    (condition (c) (format nil "Error: ~A" c))))

;; Copy file
(define (lisp-cp src dst)
  (handler-case
    (let ((content (read-file-fully (safe-path? src))))
      (lisp-write-file (safe-path? dst) content)
      "ok")
    (condition (c) (format nil "Error: ~A" c))))
`
