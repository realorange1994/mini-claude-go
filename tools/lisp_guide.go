package tools

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// LLMSimpleCall is a callback provided by the main package that wraps
// a single-turn LLM API call. The tools package cannot import
// anthropic-sdk-go directly (it's in main), so we use a function type.
type LLMSimpleCall func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error)

// LispGuideTool uses the built-in LLM to answer Lisp/FFI questions with source code context.
// The agent asks a question -> the tool reads relevant microlisp source files ->
// calls the LLM directly -> returns a precise answer. Much more efficient than the agent
// exploring the codebase itself.
type LispGuideTool struct {
	LLMCall      LLMSimpleCall
	SourceFS     fs.FS // embedded microlisp/*.go source tree
	TestSourceFS fs.FS // embedded microlisp/*_test.go test source files
	TestDataFS   fs.FS // embedded microlisp/testdata/*.lisp test files

	index   string
	indexMu sync.Once
}

func (t *LispGuideTool) Name() string { return "lisp_guide" }

func (t *LispGuideTool) Description() string {
	return "Ask the LLM about Lisp/FFI syntax and usage. " +
		"The tool reads relevant microlisp source code and Common Lisp test examples, " +
		"feeds them to the LLM, and returns a precise answer with code examples. " +
		"Much more efficient than exploring the codebase yourself. " +
		"Example: question=\"how to use go:set-field with time.Time\". " +
		"You can optionally specify source_files (e.g. \"go_ffi.go\" or \"core.lisp\"), " +
		"or leave it empty for auto-selection."
}

func (t *LispGuideTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "Your Lisp/FFI question. Be specific.",
			},
			"source_files": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional: files to read — Go sources (e.g. \"go_ffi.go\") or Lisp tests (e.g. \"core.lisp\", \"hash_tables.lisp\"). Auto-selected if empty.",
			},
			"max_tokens": map[string]any{
				"type":        "integer",
				"description": "Max tokens for the LLM response. Default: 4096.",
			},
		},
		"required": []string{"question"},
	}
}

func (t *LispGuideTool) CheckPermissions(params map[string]any) PermissionResult {
	return PermissionResultPassthrough()
}

// indexGoFiles scans a Go source FS using go/ast and appends symbol info to sb.
// The prefix label (e.g. "test:") is prepended to each file entry to distinguish test files.
func indexGoFiles(sb *strings.Builder, fsys fs.FS, prefix string) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return
	}
	fset := token.NewFileSet()
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			continue
		}
		f, err := parser.ParseFile(fset, name, data, parser.ParseComments)
		if err != nil {
			continue
		}
		var funcs, types, consts, vars []string
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				sig := formatFuncSig(fset, d)
				if sig != "" {
					funcs = append(funcs, sig)
				}
			case *ast.GenDecl:
				switch d.Tok {
				case token.TYPE:
					for _, spec := range d.Specs {
						if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
							types = append(types, formatTypeSig(fset, ts))
						}
					}
				case token.CONST:
					for _, spec := range d.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							for _, n := range vs.Names {
								if n.IsExported() {
									consts = append(consts, n.Name)
								}
							}
						}
					}
				case token.VAR:
					for _, spec := range d.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							for _, n := range vs.Names {
								if n.IsExported() {
									typ := ""
									if vs.Type != nil {
										typ = " " + nodeToString(fset, vs.Type)
									}
									vars = append(vars, n.Name+typ)
								}
							}
						}
					}
				}
			}
		}
		if len(funcs)+len(types)+len(consts)+len(vars) == 0 {
			continue
		}
		sb.WriteString(prefix + name + ":")
		if len(funcs) > 0 {
			sb.WriteString(" fn=[" + strings.Join(funcs, ", ") + "]")
		}
		if len(types) > 0 {
			sb.WriteString(" type=[" + strings.Join(types, ", ") + "]")
		}
		if len(consts) > 0 {
			sb.WriteString(" const=[" + strings.Join(consts, ", ") + "]")
		}
		if len(vars) > 0 {
			sb.WriteString(" var=[" + strings.Join(vars, ", ") + "]")
		}
		sb.WriteString("\n")
	}
}

// buildIndex scans Go sources with go/ast and Lisp tests with regex,
// producing a compact symbol index for the LLM.
func (t *LispGuideTool) buildIndex() {
	t.indexMu.Do(func() {
		var sb strings.Builder

		// === Go source files ===
		if t.SourceFS != nil {
			indexGoFiles(&sb, t.SourceFS, "")
		}

		// === Go test source files ===
		if t.TestSourceFS != nil {
			indexGoFiles(&sb, t.TestSourceFS, "test:")
		}

		// === Lisp test files ===
		if t.TestDataFS != nil {
			entries, err := fs.ReadDir(t.TestDataFS, ".")
			if err == nil {
				sb.WriteString("\n--- Lisp Test Examples ---\n")
				var lispNames []string
				for _, e := range entries {
					if !e.IsDir() && strings.HasSuffix(e.Name(), ".lisp") {
						lispNames = append(lispNames, e.Name())
					}
				}
				sort.Strings(lispNames)
				for _, name := range lispNames {
					data, err := fs.ReadFile(t.TestDataFS, name)
					if err != nil {
						continue
					}
					content := string(data)
					suites := extractLispSuites(content)
					defs := extractLispDefs(content)
					if len(suites) == 0 && len(defs) == 0 {
						continue
					}
					sb.WriteString(name + ":")
					if len(suites) > 0 {
						sb.WriteString(" suites=[" + strings.Join(suites, ", ") + "]")
					}
					if len(defs) > 0 {
						sb.WriteString(" defs=[" + strings.Join(defs, ", ") + "]")
					}
					sb.WriteString("\n")
				}
			}
		}

		if sb.Len() == 0 {
			t.index = "(no embedded source files)"
			return
		}
		t.index = sb.String()
	})
}

// extractLispSuites finds (start-suite "name") patterns.
var reStartSuite = regexp.MustCompile(`\(start-suite\s+"([^"]+)"\)`)

func extractLispSuites(content string) []string {
	matches := reStartSuite.FindAllStringSubmatch(content, -1)
	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = m[1]
	}
	return result
}

// extractLispDefs finds (define (funcName ...)) and (defmacro name ...) patterns.
var reDefineFunc = regexp.MustCompile(`\(define\s+\(([\w-]+)`)
var reDefmacro = regexp.MustCompile(`\(defmacro\s+\(([\w-]+)`)

func extractLispDefs(content string) []string {
	var names []string
	for _, m := range reDefineFunc.FindAllStringSubmatch(content, -1) {
		names = append(names, m[1])
	}
	for _, m := range reDefmacro.FindAllStringSubmatch(content, -1) {
		names = append(names, m[1])
	}
	// Deduplicate
	seen := make(map[string]bool)
	var result []string
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			result = append(result, n)
		}
	}
	return result
}

// formatFuncSig returns a compact function signature like "Foo(a int, b string) error"
func formatFuncSig(fset *token.FileSet, d *ast.FuncDecl) string {
	if !d.Name.IsExported() {
		return ""
	}
	var sb strings.Builder
	if d.Recv != nil {
		sb.WriteString("(")
		for i, field := range d.Recv.List {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(nodeToString(fset, field.Type))
		}
		sb.WriteString(").")
	}
	sb.WriteString(d.Name.Name)
	sb.WriteString("(")
	for i, field := range d.Type.Params.List {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(nodeToString(fset, field.Type))
	}
	sb.WriteString(")")
	if d.Type.Results != nil {
		if len(d.Type.Results.List) == 1 {
			sb.WriteString(" " + nodeToString(fset, d.Type.Results.List[0].Type))
		} else if len(d.Type.Results.List) > 1 {
			sb.WriteString(" (")
			for i, field := range d.Type.Results.List {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(nodeToString(fset, field.Type))
			}
			sb.WriteString(")")
		}
	}
	return sb.String()
}

// formatTypeSig returns a compact type declaration like "Foo struct{...}" or "Bar int"
func formatTypeSig(fset *token.FileSet, ts *ast.TypeSpec) string {
	name := ts.Name.Name
	kind := nodeToString(fset, ts.Type)
	// Abbreviate struct/interface bodies
	if strings.HasPrefix(kind, "struct{") || strings.HasPrefix(kind, "interface{") {
		kind = kind[:strings.Index(kind, "{")+1] + "}"
	}
	return name + " " + kind
}

// nodeToString renders an AST node back to source text using go/printer.
func nodeToString(fset *token.FileSet, node ast.Node) string {
	var buf strings.Builder
	if err := printer.Fprint(&buf, fset, node); err != nil {
		return "?"
	}
	return buf.String()
}

func (t *LispGuideTool) ExecuteContext(ctx context.Context, params map[string]any) (result ToolResult) {
	defer func() {
		if r := recover(); r != nil {
			result = ToolResult{Output: fmt.Sprintf("Error: lisp_guide panic: %v", r), IsError: true}
		}
	}()

	question, _ := params["question"].(string)
	if question == "" {
		return ToolResult{Output: "Error: question is required", IsError: true}
	}

	maxTokens := 4096
	if v, ok := params["max_tokens"].(float64); ok && v > 0 {
		maxTokens = int(v)
	} else if v, ok := params["max_tokens"].(int); ok && v > 0 {
		maxTokens = v
	}

	// Build symbol index on first call
	t.buildIndex()

	// Determine which source files to read
	var sourceFiles []string
	if raw, ok := params["source_files"]; ok {
		switch v := raw.(type) {
		case []string:
			sourceFiles = v
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					sourceFiles = append(sourceFiles, s)
				}
			}
		}
	}

	if len(sourceFiles) == 0 {
		sourceFiles = autoSelectFiles(question)
	}

	// Read source files — try SourceFS first, then TestSourceFS, then TestDataFS
	var sourceContext []string
	var anyRead bool
	for _, f := range sourceFiles {
		data, err := fs.ReadFile(t.SourceFS, f)
		if err != nil && t.TestSourceFS != nil {
			data, err = fs.ReadFile(t.TestSourceFS, f)
		}
		if err != nil && t.TestDataFS != nil {
			data, err = fs.ReadFile(t.TestDataFS, f)
		}
		if err != nil {
			sourceContext = append(sourceContext, fmt.Sprintf("--- %s (not found: %v) ---", f, err))
			continue
		}
		anyRead = true
		content := string(data)
		lines := strings.Split(content, "\n")
		if len(lines) > 2000 {
			content = strings.Join(lines[:2000], "\n") + "\n... (truncated, full file has " + fmt.Sprintf("%d", len(lines)) + " lines)"
		}
		sourceContext = append(sourceContext, fmt.Sprintf("--- %s ---\n%s", f, content))
	}

	if !anyRead {
		return ToolResult{Output: "Error: no source files could be read from embedded FS.", IsError: true}
	}

	// Build the prompt
	systemPrompt := `You are a Lisp/FFI expert helping an AI agent use a Common Lisp interpreter with Go FFI.
The interpreter is embedded in a Go application (miniClaudeCode-go). The microlisp sub-package implements:
- Standard CL operations (lists, strings, hash tables, arrays, CLOS, conditions, macros, format)
- Go FFI: go:import, go:new, go:field, go:set-field, go:call, go:channel, go:send, go:recv, go:select, go:spawn
- VGoVal type: opaque Go values that preserve Go type info for reflection
- reflectToLisp/lispToReflectSafe: bidirectional conversion between Go and Lisp values

Answer the agent's question concisely and accurately. Provide code examples when helpful.
Focus on exact syntax. Do NOT guess — use the source code below as your reference.
If the source code doesn't cover the question, say so explicitly.
Respond with the answer text only — do not wrap in XML tags or JSON.

At the very end of your response, if you think other source files would help, suggest them as:
"Try reading: filename1.go, filename2.go" or "Try reading: filename1.lisp, filename2.lisp"`

	userPrompt := fmt.Sprintf("Symbol index for ALL embedded files (Go sources + Lisp test examples):\n\n%s\n\nFull source code I've read:\n\n%s\n\nQuestion: %s",
		t.index, strings.Join(sourceContext, "\n\n"), question)

	// Call the LLM directly
	if t.LLMCall == nil {
		return ToolResult{Output: "Error: LLM callback not configured", IsError: true}
	}

	answer, err := t.LLMCall(ctx, systemPrompt, userPrompt, maxTokens)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: LLM call failed: %v", err), IsError: true}
	}

	return ToolResult{Output: answer}
}

func (t *LispGuideTool) Execute(params map[string]any) ToolResult {
	return t.ExecuteContext(context.Background(), params)
}

// autoSelectFiles picks relevant files based on question keywords.
func autoSelectFiles(question string) []string {
	q := strings.ToLower(question)
	selected := make(map[string]bool)

	mappings := []struct {
		keywords []string
		files    []string
	}{
		{[]string{"ffi", "go:", "go:import", "go:call", "go:field", "go:set-field", "struct", "reflect", "goval", "interop"},
			[]string{"go_ffi.go", "go_struct.go", "ffi_audit_test.go", "go_stdlib_quick_test.go"}},
		{[]string{"time", "time.time", "duration", "now", "location", "timezone"},
			[]string{"ffi_time.go", "time.go", "go_ffi.go"}},
		{[]string{"channel", "select", "spawn", "goroutine", "concurrent", "go:send", "go:recv"},
			[]string{"go_channels.go"}},
		{[]string{"format", "~a", "~d", "~s", "~%", "~{", "format-nil", "format-t"},
			[]string{"format.go", "format-tests.lisp"}},
		{[]string{"list", "car", "cdr", "cons", "append", "nth", "member", "assoc"},
			[]string{"list_ops.go", "list.lisp"}},
		{[]string{"string", "char", "substring", "string-append", "string-length"},
			[]string{"strings.go", "strings.lisp"}},
		{[]string{"array", "make-array", "aref", "vector"},
			[]string{"data_structures.go", "array-edge-cases.lisp"}},
		{[]string{"clos", "defclass", "defmethod", "defgeneric", "slot-value", "make-instance", "object"},
			[]string{"clos.go", "advanced_clos.lisp"}},
		{[]string{"condition", "error", "handler-case", "handler-bind", "restart-case", "signal", "warn"},
			[]string{"conditions.go", "conditions-edge-cases.lisp", "handler_bind_test.go"}},
		{[]string{"hash", "hash-table", "gethash", "make-hash-table"},
			[]string{"data_structures.go", "hash_tables.lisp"}},
		{[]string{"macro", "defmacro", "define-macro", "macroexpand", "backquote", "quasiquote"},
			[]string{"macroexpand.go", "macros.lisp"}},
		{[]string{"sequence", "mapcar", "filter", "reduce", "remove-if", "count-if"},
			[]string{"seq_construct.go", "seq_search.go", "sequences.lisp"}},
		{[]string{"number", "arithmetic", "math", "integer", "float", "rational", "bignum"},
			[]string{"arithmetic.go", "numbers.go", "core.lisp"}},
		{[]string{"type", "type-of", "typep", "subtypep", "deftype", "coerce"},
			[]string{"type_system.go", "type-tests.lisp", "coerce.lisp"}},
		{[]string{"stream", "read", "print", "io", "file", "read-line"},
			[]string{"streams.go", "io.go", "io-stream-tests.lisp"}},
		{[]string{"package", "defpackage", "in-package", "import", "export"},
			[]string{"packages.go", "packages-advanced.lisp"}},
		{[]string{"loop", "dotimes", "dolist", "do", "iterate"},
			[]string{"iteration.go", "loop-iteration-edge-cases.lisp"}},
		{[]string{"x509", "certificate", "tls", "ssl"},
			[]string{"ffi_crypto_x509.go", "ffi_crypto_tls.go"}},
		{[]string{"json", "xml", "encoding"},
			[]string{"ffi_encoding_json.go", "ffi_encoding_xml.go", "ffi_encoding.go"}},
		{[]string{"sql", "database"},
			[]string{"ffi_database_sql.go", "ffi_database_sql_driver.go"}},
		{[]string{"http", "net", "url", "cookie"},
			[]string{"ffi_net_http.go", "ffi_net_url.go", "ffi_net.go"}},
		{[]string{"rsa", "ecdsa", "ed25519", "aes", "sha", "cipher"},
			[]string{"ffi_crypto_rsa.go", "ffi_crypto_aes.go", "ffi_crypto_sha256.go"}},
		{[]string{"regexp", "regex", "pattern", "match"},
			[]string{"ffi_regexp.go"}},
		{[]string{"closure", "lambda", "let", "lexenv"},
			[]string{"closures.lisp", "closures-edge-cases.lisp"}},
		{[]string{"control flow", "cond", "case", "when", "unless", "if"},
			[]string{"control-flow-edge-cases.lisp", "case-tests.lisp"}},
		{[]string{"destructure", "destructuring"},
			[]string{"destructure-tests.lisp"}},
		{[]string{"readtable", "reader"},
			[]string{"readtable-tests.lisp"}},
		{[]string{"bug", "issue", "fix", "crash", "panic"},
			[]string{"bug_regression_test.go", "bug_report_test.go", "bugs_new_test.go"}},
		{[]string{"safety", "resource limit", "step limit"},
			[]string{"safety_test.go"}},
	}

	for _, m := range mappings {
		for _, kw := range m.keywords {
			if strings.Contains(q, kw) {
				for _, f := range m.files {
					selected[f] = true
				}
				break
			}
		}
	}

	// Default fallback
	if len(selected) == 0 {
		for _, f := range []string{"go_ffi.go", "go_struct.go", "ffi_time.go", "core.lisp", "stdlib.lisp"} {
			selected[f] = true
		}
	}

	selected["go_interop.go"] = true

	files := make([]string, 0, len(selected))
	for f := range selected {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// CallLLMWithTimeout wraps an LLMSimpleCall with a timeout.
func CallLLMWithTimeout(ctx context.Context, fn LLMSimpleCall, systemPrompt, userPrompt string, maxTokens int, timeout time.Duration) (string, error) {
	type callResult struct {
		answer string
		err    error
	}

	resultCh := make(chan callResult, 1)
	go func() {
		answer, err := fn(ctx, systemPrompt, userPrompt, maxTokens)
		resultCh <- callResult{answer, err}
	}()

	select {
	case <-ctx.Done():
		return "", fmt.Errorf("LLM call cancelled: %v", ctx.Err())
	case <-time.After(timeout):
		return "", fmt.Errorf("LLM call timed out after %v", timeout)
	case r := <-resultCh:
		return r.answer, r.err
	}
}
