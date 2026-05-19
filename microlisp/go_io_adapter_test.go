package microlisp

import (
	"testing"
)

func TestIOAdapterStringReader(t *testing.T) {
	InitGlobalEnv()

	// Test reader-from-string + reader-read-all roundtrip
	result, err := EvalString(`(reader-read-all (reader-from-string "hello world"))`, globalEnv)
	if err != nil {
		t.Fatalf("reader-from-string + reader-read-all failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello world" {
		t.Fatalf("expected 'hello world', got: %v", result)
	}
}

func TestIOAdapterStringWriter(t *testing.T) {
	InitGlobalEnv()

	// Test writer-to-string + writer-get-string
	result, err := EvalString(`
		(let* ((w (writer-to-string))
		       (f (go:import "microlisp/io.NewBufferWriter")))
		  (go:call w "WriteString" "hello")
		  (writer-get-string w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("writer-to-string failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello" {
		t.Fatalf("expected 'hello', got: %v", result)
	}
}

func TestIOAdapterFileRoundtrip(t *testing.T) {
	InitGlobalEnv()

	// Test writer-to-file + reader-from-file roundtrip
	_, err := EvalString(`
		(let* ((w (writer-to-file "/tmp/lisp_io_adapter_test.txt")))
		  (go:call w "WriteString" "file content")
		  (writer-close w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("writer-to-file failed: %v", err)
	}

	result, err := EvalString(`(reader-read-all (reader-from-file "/tmp/lisp_io_adapter_test.txt"))`, globalEnv)
	if err != nil {
		t.Fatalf("reader-from-file + reader-read-all failed: %v", err)
	}
	if result.typ != VStr || result.str != "file content" {
		t.Fatalf("expected 'file content', got: %v", result)
	}

	// Cleanup
	EvalString(`(delete-file "/tmp/lisp_io_adapter_test.txt")`, globalEnv)
}

func TestIOAdapterCopyToFile(t *testing.T) {
	InitGlobalEnv()

	// Test io-copy-to-file
	_, err := EvalString(`(io-copy-to-file (reader-from-string "copied data") "/tmp/lisp_io_copy_test.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("io-copy-to-file failed: %v", err)
	}

	result, err := EvalString(`(read-file "/tmp/lisp_io_copy_test.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("read-file failed: %v", err)
	}
	if result.typ != VStr || result.str != "copied data" {
		t.Fatalf("expected 'copied data', got: %v", result)
	}

	EvalString(`(delete-file "/tmp/lisp_io_copy_test.txt")`, globalEnv)
}

func TestIOAdapterLimitString(t *testing.T) {
	InitGlobalEnv()

	result, err := EvalString(`(io-limit-string (reader-from-string "hello world") 5)`, globalEnv)
	if err != nil {
		t.Fatalf("io-limit-string failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello" {
		t.Fatalf("expected 'hello', got: %v", result)
	}
}

func TestIOAdapterNopCloser(t *testing.T) {
	InitGlobalEnv()

	// io-nop-closer should wrap a reader as ReadCloser
	result, err := EvalString(`(io-nop-closer (reader-from-string "test"))`, globalEnv)
	if err != nil {
		t.Fatalf("io-nop-closer failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got: %s", typeStr(result))
	}
}

func TestContextWithTimeout(t *testing.T) {
	InitGlobalEnv()

	// ctx-with-timeout returns (ctx cancel-fn)
	result, err := EvalString(`(ctx-with-timeout 5)`, globalEnv)
	if err != nil {
		t.Fatalf("ctx-with-timeout failed: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got: %s", typeStr(result))
	}
	ctx := result.car
	cancelFn := result.cdr.car
	if ctx.typ != VGoVal {
		t.Fatalf("expected VGoVal context, got: %s", typeStr(ctx))
	}
	if cancelFn.typ != VGoVal {
		t.Fatalf("expected VGoVal cancel-fn, got: %s", typeStr(cancelFn))
	}

	// ctx-done should return nil for a fresh context
	doneResult, err := EvalString(`(ctx-done (car (ctx-with-timeout 5)))`, globalEnv)
	if err != nil {
		t.Fatalf("ctx-done failed: %v", err)
	}
	if isTruthy(doneResult) {
		t.Fatalf("expected fresh context to not be done, got: %v", doneResult)
	}
}

func TestContextWithCancel(t *testing.T) {
	InitGlobalEnv()

	// ctx-with-cancel returns (ctx cancel-fn)
	result, err := EvalString(`(ctx-with-cancel)`, globalEnv)
	if err != nil {
		t.Fatalf("ctx-with-cancel failed: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got: %s", typeStr(result))
	}

	// Cancel and check done
	result2, err := EvalString(`
		(let* ((pair (ctx-with-cancel))
		       (ctx (car pair))
		       (cancel (cadr pair)))
		  (ctx-cancel cancel)
		  (ctx-done ctx))
	`, globalEnv)
	if err != nil {
		t.Fatalf("ctx-cancel + ctx-done failed: %v", err)
	}
	if !isTruthy(result2) {
		t.Fatalf("expected cancelled context to be done, got: %v", result2)
	}
}

func TestGoCallback(t *testing.T) {
	InitGlobalEnv()

	// Create a Go callback from a Lisp function
	result, err := EvalString(`
		(go:callback (lambda (x) (+ x 1)) "int32->int32")
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:callback failed: %v", err)
	}
	// Should return a VGoVal (Go function value)
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got: %s", typeStr(result))
	}
}

func TestHttpRequest(t *testing.T) {
	InitGlobalEnv()

	// Create an HTTP request object
	result, err := EvalString(`(http-request "GET" "https://httpbin.org/get")`, globalEnv)
	if err != nil {
		t.Fatalf("http-request failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got: %s", typeStr(result))
	}
}

func TestBinaryEncoding(t *testing.T) {
	InitGlobalEnv()

	// Test binary write + read roundtrip for uint32
	result, err := EvalString(`
		(let* ((data (binary-write-uint32 42 "big"))
		       (val (binary-read-uint32 data "big")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("binary uint32 roundtrip failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 42 {
		t.Fatalf("expected 42, got: %v", result)
	}

	// Test uint64
	result, err = EvalString(`
		(let* ((data (binary-write-uint64 12345 "little"))
		       (val (binary-read-uint64 data "little")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("binary uint64 roundtrip failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 12345 {
		t.Fatalf("expected 12345, got: %v", result)
	}
}

func TestFmtSprintf(t *testing.T) {
	InitGlobalEnv()

	result, err := EvalString(`(fmt-sprintf "Hello %s, you are %d years old" "World" 25)`, globalEnv)
	if err != nil {
		t.Fatalf("fmt-sprintf failed: %v", err)
	}
	if result.typ != VStr || result.str != "Hello World, you are 25 years old" {
		t.Fatalf("expected 'Hello World, you are 25 years old', got: %v", result)
	}
}

func TestIOAdapterWithGoStdlib(t *testing.T) {
	InitGlobalEnv()

	// Test using reader-from-string with a Go function that takes io.Reader
	// e.g. io.ReadAll is already wrapped, but let's test the adapter chain
	result, err := EvalString(`
		(let* ((r (reader-from-string "adapter test"))
		       (data (reader-read-all r)))
		  data)
	`, globalEnv)
	if err != nil {
		t.Fatalf("adapter chain failed: %v", err)
	}
	if result.typ != VStr || result.str != "adapter test" {
		t.Fatalf("expected 'adapter test', got: %v", result)
	}
}

func TestIOAdapterStringWriterDirect(t *testing.T) {
	InitGlobalEnv()

	// Test creating a string writer and writing to it via go:call
	result, err := EvalString(`
		(let* ((w (writer-to-string)))
		  (go:call w "WriteString" "hello ")
		  (go:call w "WriteString" "world")
		  (writer-get-string w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("string writer direct write failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello world" {
		t.Fatalf("expected 'hello world', got: %v", result)
	}
}

func TestIOAdapterWriterReset(t *testing.T) {
	InitGlobalEnv()

	result, err := EvalString(`
		(let* ((w (writer-to-string)))
		  (go:call w "WriteString" "before")
		  (writer-reset w)
		  (go:call w "WriteString" "after")
		  (writer-get-string w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("writer reset failed: %v", err)
	}
	if result.typ != VStr || result.str != "after" {
		t.Fatalf("expected 'after', got: %v", result)
	}
}
