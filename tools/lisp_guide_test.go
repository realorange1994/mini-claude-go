package tools

import (
	"context"
	"embed"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// ─── extractLispSuites Tests ────────────────────────────────────────────────

func TestExtractLispSuites(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{
			"basic suites",
			`(start-suite "Core Arithmetic")
(assert-equal 3 (+ 1 2))
(end-suite)
(start-suite "Special Forms")
(assert-equal 1 (if #t 1 2))`,
			[]string{"Core Arithmetic", "Special Forms"},
		},
		{
			"empty input",
			"",
			nil,
		},
		{
			"no suites",
			"(assert-equal 3 (+ 1 2))",
			nil,
		},
		{
			"single suite",
			`(start-suite "Only One")
(assert-true #t)`,
			[]string{"Only One"},
		},
		{
			"suite name with spaces and symbols",
			`(start-suite "Edge Cases: list-length / ldiff")`,
			[]string{"Edge Cases: list-length / ldiff"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractLispSuites(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d suites, want %d: %v", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("suite[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ─── extractLispDefs Tests ──────────────────────────────────────────────────

func TestExtractLispDefs(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string // sorted
	}{
		{
			"define functions",
			`(define (square x) (* x x))
(define (add a b) (+ a b))
(define (factorial n)
  (if (<= n 1) 1 (* n (factorial (- n 1)))))`,
			[]string{"add", "factorial", "square"},
		},
		{
			"defmacro",
			`(defmacro (my-if condition then else)
  (list 'if condition then else))`,
			[]string{"my-if"},
		},
		{
			"mixed define and defmacro",
			`(define (foo x) x)
(defmacro (bar x) x)
(define (baz x) x)`,
			[]string{"bar", "baz", "foo"},
		},
		{
			"empty input",
			"",
			nil,
		},
		{
			"no definitions",
			"(assert-equal 3 (+ 1 2))",
			nil,
		},
		{
			"duplicate names deduplicated",
			`(define (foo x) x)
(define (foo x) (+ x 1))`,
			[]string{"foo"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractLispDefs(tc.input)
			sort.Strings(got)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d defs, want %d: %v", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("def[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ─── autoSelectFiles Tests ──────────────────────────────────────────────────

func TestAutoSelectFilesContainsExpected(t *testing.T) {
	cases := []struct {
		question    string
		mustContain []string
	}{
		{"how to use go:import with time.Time", []string{"ffi_time.go", "go_ffi.go"}},
		{"how to use go:set-field with struct", []string{"go_ffi.go", "go_struct.go"}},
		{"channel select goroutine", []string{"go_channels.go"}},
		{"format ~A ~D ~S usage", []string{"format.go"}},
		{"list car cdr cons append", []string{"list_ops.go"}},
		{"defclass defmethod clos", []string{"clos.go", "advanced_clos.lisp"}},
		{"condition handler-case handler-bind restart-case", []string{"conditions.go", "handler_bind_test.go"}},
		{"hash-table gethash make-hash-table", []string{"data_structures.go"}},
		{"defmacro macroexpand", []string{"macroexpand.go"}},
		{"sequence mapcar filter reduce", []string{"seq_construct.go", "seq_search.go"}},
		{"number arithmetic bignum", []string{"arithmetic.go", "numbers.go"}},
		{"typep coerce type-of", []string{"type_system.go"}},
		{"stream read print io", []string{"streams.go", "io.go"}},
		{"package defpackage export", []string{"packages.go"}},
		{"x509 certificate tls", []string{"ffi_crypto_x509.go"}},
		{"json encoding", []string{"ffi_encoding_json.go"}},
		{"http url cookie", []string{"ffi_net_http.go"}},
		{"closure lambda let", []string{"closures.lisp"}},
		{"readtable reader", []string{"readtable-tests.lisp"}},
		{"bug crash panic regression", []string{"bug_regression_test.go"}},
		{"safety resource limit step limit", []string{"safety_test.go"}},
		{"ffi go:import struct reflect", []string{"go_ffi.go", "go_struct.go", "ffi_audit_test.go"}},
	}

	for _, tc := range cases {
		t.Run(tc.question, func(t *testing.T) {
			files := autoSelectFiles(tc.question)
			fileSet := make(map[string]bool)
			for _, f := range files {
				fileSet[f] = true
			}
			for _, want := range tc.mustContain {
				if !fileSet[want] {
					t.Errorf("autoSelectFiles(%q) did not include %q, got %v", tc.question, want, files)
				}
			}
		})
	}
}

func TestAutoSelectFilesAlwaysIncludesGoInterop(t *testing.T) {
	files := autoSelectFiles("any question")
	found := false
	for _, f := range files {
		if f == "go_interop.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("autoSelectFiles should always include go_interop.go, got %v", files)
	}
}

func TestAutoSelectFilesReturnsSorted(t *testing.T) {
	files := autoSelectFiles("ffi time channel")
	if !sort.StringsAreSorted(files) {
		t.Errorf("autoSelectFiles should return sorted files, got %v", files)
	}
}

func TestAutoSelectFilesNoDuplicates(t *testing.T) {
	files := autoSelectFiles("time ffi time.Time duration")
	seen := make(map[string]bool)
	for _, f := range files {
		if seen[f] {
			t.Errorf("duplicate file %q in autoSelectFiles result", f)
		}
		seen[f] = true
	}
}

func TestAutoSelectFilesDefaultFallback(t *testing.T) {
	files := autoSelectFiles("some random question with no keywords")
	// Should have default fallback files plus go_interop.go
	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
	}
	// go_interop.go is always added
	if !fileSet["go_interop.go"] {
		t.Error("default fallback should include go_interop.go")
	}
}

// ─── Interface Compliance Tests ─────────────────────────────────────────────

func TestLispGuideToolName(t *testing.T) {
	tool := &LispGuideTool{}
	if tool.Name() != "lisp_guide" {
		t.Errorf("expected 'lisp_guide', got %q", tool.Name())
	}
}

func TestLispGuideToolDescription(t *testing.T) {
	tool := &LispGuideTool{}
	desc := tool.Description()
	if !strings.Contains(desc, "Lisp") {
		t.Error("description should mention Lisp")
	}
	if !strings.Contains(desc, "FFI") {
		t.Error("description should mention FFI")
	}
}

func TestLispGuideToolInputSchema(t *testing.T) {
	tool := &LispGuideTool{}
	schema := tool.InputSchema()

	typ, ok := schema["type"]
	if !ok || typ != "object" {
		t.Errorf("schema type should be 'object', got %v", typ)
	}

	required, ok := schema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "question" {
		t.Errorf("schema required should be [question], got %v", required)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema properties should be a map")
	}

	if _, ok := props["question"]; !ok {
		t.Error("schema should have 'question' property")
	}
	if _, ok := props["source_files"]; !ok {
		t.Error("schema should have 'source_files' property")
	}
	if _, ok := props["max_tokens"]; !ok {
		t.Error("schema should have 'max_tokens' property")
	}
}

func TestLispGuideToolCheckPermissions(t *testing.T) {
	tool := &LispGuideTool{}
	result := tool.CheckPermissions(map[string]any{"question": "test"})
	if result.Behavior != PermissionPassthrough {
		t.Errorf("CheckPermissions should return Passthrough, got %v", result.Behavior)
	}
}

// ─── ExecuteContext Error Cases ─────────────────────────────────────────────

func TestLispGuideMissingQuestion(t *testing.T) {
	tool := &LispGuideTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{})
	if !result.IsError {
		t.Error("should error when question is missing")
	}
	if !strings.Contains(result.Output, "question is required") {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestLispGuideEmptyQuestion(t *testing.T) {
	tool := &LispGuideTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{"question": ""})
	if !result.IsError {
		t.Error("should error when question is empty")
	}
}

func TestLispGuideNilLLMCall(t *testing.T) {
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	tool := &LispGuideTool{SourceFS: sourceFS}
	// Explicitly provide the test file so auto-select finds a readable file
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "test question",
		"source_files": []string{"test.go"},
	})
	if !result.IsError {
		t.Error("should error when LLMCall is nil")
	}
	if !strings.Contains(result.Output, "LLM callback not configured") {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestLispGuideNoSourceFilesFound(t *testing.T) {
	// Tool with SourceFS set but TestDataFS nil — the explicitly provided
	// file doesn't exist in either FS
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	tool := &LispGuideTool{SourceFS: sourceFS}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "test",
		"source_files": []string{"nonexistent.go"},
	})
	if !result.IsError {
		t.Error("should error when no source files can be read")
	}
	if !strings.Contains(result.Output, "no source files") {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

// ─── Execute (non-context) ──────────────────────────────────────────────────

func TestLispGuideExecuteNoContext(t *testing.T) {
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	tool := &LispGuideTool{SourceFS: sourceFS}
	result := tool.Execute(map[string]any{
		"question":     "test",
		"source_files": []string{"test.go"},
	})
	if !result.IsError {
		t.Error("should error when LLMCall is nil")
	}
}

// ─── BuildIndex Tests with Mock FS ──────────────────────────────────────────

// mockGoFile creates a minimal embedded-style Go source file for testing.
var mockGoSource = `package microlisp

// Foo is an exported function for testing.
func Foo(a int, b string) error {
	return nil
}

// Bar returns a string.
func Bar() string { return "hello" }

// MyType is a test struct.
type MyType struct {
	Name string
	ID   int
}

// ConfigVar is a global variable.
var ConfigVar = "default"

// MaxRetries is a constant.
const MaxRetries = 3
`

// mockLispTest creates a minimal Lisp test file for testing.
var mockLispTest = `;; Test file for lisp_guide testing

(start-suite "Basic Tests")
(assert-equal 3 (+ 1 2))

(define (square x) (* x x))
(define (cube x) (* x x x))

(defmacro (my-when condition &rest body)
  (list 'if condition (cons 'progn body)))

(start-suite "Advanced Tests")
(assert-equal 9 (square 3))

(end-suite)
`

//go:embed lisp_guide_test_data
var testFS embed.FS

func TestBuildIndexWithMockFS(t *testing.T) {
	sourceFS, err := fs.Sub(testFS, "lisp_guide_test_data/go")
	if err != nil {
		t.Fatalf("failed to create source sub-FS: %v", err)
	}
	testdataFS, err := fs.Sub(testFS, "lisp_guide_test_data/lisp")
	if err != nil {
		t.Fatalf("failed to create testdata sub-FS: %v", err)
	}

	tool := &LispGuideTool{
		SourceFS:   sourceFS,
		TestDataFS: testdataFS,
		// Set up a mock LLM callback that captures the prompt
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			return "mock answer", nil
		},
	}

	// Trigger buildIndex via ExecuteContext
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "what is Foo",
		"source_files": []string{"test.go", "test.lisp"},
	})
	if result.IsError {
		t.Fatalf("ExecuteContext failed: %s", result.Output)
	}

	// Verify the index was built
	if tool.index == "" || strings.Contains(tool.index, "no embedded source files") {
		t.Errorf("buildIndex should have produced an index, got: %s", tool.index)
	}

	// Verify Go symbols are in the index
	if !strings.Contains(tool.index, "Foo") {
		t.Errorf("index should contain 'Foo', got: %s", tool.index)
	}
	if !strings.Contains(tool.index, "MyType") {
		t.Errorf("index should contain 'MyType', got: %s", tool.index)
	}
	if !strings.Contains(tool.index, "MaxRetries") {
		t.Errorf("index should contain 'MaxRetries', got: %s", tool.index)
	}

	// Verify Lisp suites are in the index
	if !strings.Contains(tool.index, "Basic Tests") {
		t.Errorf("index should contain 'Basic Tests' suite, got: %s", tool.index)
	}
	if !strings.Contains(tool.index, "Advanced Tests") {
		t.Errorf("index should contain 'Advanced Tests' suite, got: %s", tool.index)
	}

	// Verify Lisp defs are in the index
	if !strings.Contains(tool.index, "square") {
		t.Errorf("index should contain 'square' def, got: %s", tool.index)
	}
	if !strings.Contains(tool.index, "cube") {
		t.Errorf("index should contain 'cube' def, got: %s", tool.index)
	}
	if !strings.Contains(tool.index, "my-when") {
		t.Errorf("index should contain 'my-when' defmacro, got: %s", tool.index)
	}
}

func TestBuildIndexIdempotent(t *testing.T) {
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	testdataFS, _ := fs.Sub(testFS, "lisp_guide_test_data/lisp")

	tool := &LispGuideTool{
		SourceFS: sourceFS,
		TestDataFS: testdataFS,
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			return "mock answer", nil
		},
	}

	// First call builds index
	result1 := tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "test",
		"source_files": []string{"test.go"},
	})
	if result1.IsError {
		t.Fatalf("first call failed: %s", result1.Output)
	}
	firstIndex := tool.index

	// Second call should reuse cached index (sync.Once)
	result2 := tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "test2",
		"source_files": []string{"test.go"},
	})
	if result2.IsError {
		t.Fatalf("second call failed: %s", result2.Output)
	}

	if tool.index != firstIndex {
		t.Error("buildIndex should be idempotent via sync.Once")
	}
}

// ─── LLM Call Integration Tests ─────────────────────────────────────────────

func TestLispGuideLLMCallReceivesPrompt(t *testing.T) {
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	testdataFS, _ := fs.Sub(testFS, "lisp_guide_test_data/lisp")

	var receivedSystem, receivedUser string
	receivedTokens := 0

	tool := &LispGuideTool{
		SourceFS:   sourceFS,
		TestDataFS: testdataFS,
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			receivedSystem = systemPrompt
			receivedUser = userPrompt
			receivedTokens = maxTokens
			return "The answer is 42", nil
		},
	}

	result := tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "What is Foo?",
		"source_files": []string{"test.go"},
		"max_tokens":   2048,
	})
	if result.IsError {
		t.Fatalf("ExecuteContext failed: %s", result.Output)
	}
	if result.Output != "The answer is 42" {
		t.Errorf("expected 'The answer is 42', got %q", result.Output)
	}
	if receivedTokens != 2048 {
		t.Errorf("expected max_tokens=2048, got %d", receivedTokens)
	}
	// System prompt should mention Lisp/FFI
	if !strings.Contains(receivedSystem, "Lisp") {
		t.Error("system prompt should mention Lisp")
	}
	if !strings.Contains(receivedSystem, "FFI") {
		t.Error("system prompt should mention FFI")
	}
	// User prompt should contain the question
	if !strings.Contains(receivedUser, "What is Foo?") {
		t.Error("user prompt should contain the question")
	}
	// User prompt should contain source file content
	if !strings.Contains(receivedUser, "Foo") {
		t.Error("user prompt should contain the Go source content")
	}
}

func TestLispGuideMaxTokensDefault(t *testing.T) {
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	var receivedTokens int
	tool := &LispGuideTool{
		SourceFS: sourceFS,
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			receivedTokens = maxTokens
			return "ok", nil
		},
	}
	// Without max_tokens param, should default to 4096
	tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "test",
		"source_files": []string{"test.go"},
	})
	if receivedTokens != 4096 {
		t.Errorf("expected default max_tokens=4096, got %d", receivedTokens)
	}
}

// ─── Context Cancellation ───────────────────────────────────────────────────

func TestLispGuideContextCancellation(t *testing.T) {
	tool := &LispGuideTool{
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result := tool.ExecuteContext(ctx, map[string]any{"question": "test"})
	if !result.IsError {
		t.Error("should error when context is cancelled")
	}
}

// ─── Source File Truncation ─────────────────────────────────────────────────

func TestLispGuideFileTruncation(t *testing.T) {
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	var receivedUser string
	tool := &LispGuideTool{
		SourceFS: sourceFS,
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			receivedUser = userPrompt
			return "ok", nil
		},
	}
	tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "test",
		"source_files": []string{"test.go"},
	})
	// test.go is small so should not be truncated
	if strings.Contains(receivedUser, "(truncated") {
		t.Error("small file should not be truncated in prompt")
	}
}

// ─── Panic Recovery ─────────────────────────────────────────────────────────

func TestLispGuidePanicRecovery(t *testing.T) {
	tool := &LispGuideTool{
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			panic("test panic")
		},
	}
	result := tool.ExecuteContext(context.Background(), map[string]any{"question": "test"})
	if !result.IsError {
		t.Error("should recover from panic and return error")
	}
	if !strings.Contains(result.Output, "panic") {
		t.Errorf("should mention panic in error, got: %s", result.Output)
	}
}

// ─── Source File Read Errors ────────────────────────────────────────────────

func TestLispGuideSourceFileNotFound(t *testing.T) {
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	testdataFS, _ := fs.Sub(testFS, "lisp_guide_test_data/lisp")
	tool := &LispGuideTool{
		SourceFS:   sourceFS,
		TestDataFS: testdataFS,
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			return "ok", nil
		},
	}
	// Explicitly request a file that doesn't exist in either FS
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "test",
		"source_files": []string{"nonexistent_file.go"},
	})
	if !result.IsError {
		t.Error("should error when no readable files found")
	}
	if !strings.Contains(result.Output, "no source files") {
		t.Errorf("should mention no source files, got: %s", result.Output)
	}
}

// ─── Cross-FS File Reading ─────────────────────────────────────────────────

func TestLispGuideReadFromTestDataFS(t *testing.T) {
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	testdataFS, _ := fs.Sub(testFS, "lisp_guide_test_data/lisp")
	tool := &LispGuideTool{
		SourceFS:   sourceFS,
		TestDataFS: testdataFS,
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			return "ok", nil
		},
	}
	// test.lisp is only in TestDataFS, not SourceFS
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "test",
		"source_files": []string{"test.lisp"},
	})
	if result.IsError {
		t.Fatalf("should read test.lisp from TestDataFS: %s", result.Output)
	}
}

// ─── TestSourceFS (Go *_test.go files) ────────────────────────────────────────

func TestLispGuideReadFromTestSourceFS(t *testing.T) {
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	testSourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go_test")
	testdataFS, _ := fs.Sub(testFS, "lisp_guide_test_data/lisp")
	tool := &LispGuideTool{
		SourceFS:     sourceFS,
		TestSourceFS: testSourceFS,
		TestDataFS:   testdataFS,
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			return "ok", nil
		},
	}
	// test_test.go is only in TestSourceFS
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "test helper",
		"source_files": []string{"test_test.go"},
	})
	if result.IsError {
		t.Fatalf("should read test_test.go from TestSourceFS: %s", result.Output)
	}
}

func TestLispGuideTestSourceFSIndexed(t *testing.T) {
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	testSourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go_test")
	testdataFS, _ := fs.Sub(testFS, "lisp_guide_test_data/lisp")
	tool := &LispGuideTool{
		SourceFS:     sourceFS,
		TestSourceFS: testSourceFS,
		TestDataFS:   testdataFS,
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			return "ok", nil
		},
	}
	// Trigger buildIndex
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "test",
		"source_files": []string{"test.go"},
	})
	if result.IsError {
		t.Fatalf("ExecuteContext failed: %s", result.Output)
	}
	// Index should contain test: prefix for test file entries
	if !strings.Contains(tool.index, "test:test_test.go") {
		t.Errorf("index should contain 'test:test_test.go' entry, got: %s", tool.index)
	}
	// Index should contain exported symbols from test file
	if !strings.Contains(tool.index, "HelperFunc") {
		t.Errorf("index should contain 'HelperFunc' from test file, got: %s", tool.index)
	}
	if !strings.Contains(tool.index, "MockStruct") {
		t.Errorf("index should contain 'MockStruct' from test file, got: %s", tool.index)
	}
	if !strings.Contains(tool.index, "SetupMock") {
		t.Errorf("index should contain 'SetupMock' from test file, got: %s", tool.index)
	}
}

func TestLispGuideTestSourceFSPromptContainsTestCode(t *testing.T) {
	sourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go")
	testSourceFS, _ := fs.Sub(testFS, "lisp_guide_test_data/go_test")
	var receivedUser string
	tool := &LispGuideTool{
		SourceFS:     sourceFS,
		TestSourceFS: testSourceFS,
		LLMCall: func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
			receivedUser = userPrompt
			return "ok", nil
		},
	}
	tool.ExecuteContext(context.Background(), map[string]any{
		"question":     "how to use TestHelper",
		"source_files": []string{"test_test.go"},
	})
	if !strings.Contains(receivedUser, "HelperFunc") {
		t.Error("user prompt should contain HelperFunc from test_test.go")
	}
}
