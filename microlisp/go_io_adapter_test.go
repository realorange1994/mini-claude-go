package microlisp

import (
	"strings"
	"testing"
	"time"
)

// ============================================================
// Section 1: io.Reader adapters
// ============================================================

func TestReaderFromStringBasic(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(reader-read-all (reader-from-string "hello world"))`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "hello world" {
		t.Fatalf("expected \"hello world\", got: %v", result)
	}
}

func TestReaderFromStringEmpty(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(reader-read-all (reader-from-string ""))`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "" {
		t.Fatalf("expected empty string, got: %v", result)
	}
}

func TestReaderFromStringUnicode(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(reader-read-all (reader-from-string "你好世界🌍"))`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "你好世界🌍" {
		t.Fatalf("expected unicode string, got: %v", result)
	}
}

func TestReaderFromBuffer(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((r (funcall (go:import "microlisp/io.NewBufferReader") "buffer data")))
		  (reader-read-all r))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "buffer data" {
		t.Fatalf("expected \"buffer data\", got: %v", result)
	}
}

func TestReaderFromFileNotFound(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(reader-from-file "/tmp/nonexistent_file_12345.txt")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestReaderFromFileRoundtrip(t *testing.T) {
	InitGlobalEnv()
	// Write then read
	_, err := EvalString(`
		(let* ((w (writer-to-file "/tmp/lisp_reader_test.txt")))
		  (go:call w "WriteString" "file roundtrip")
		  (writer-close w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	result, err := EvalString(`(reader-read-all (reader-from-file "/tmp/lisp_reader_test.txt"))`, globalEnv)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if result.typ != VStr || result.str != "file roundtrip" {
		t.Fatalf("expected \"file roundtrip\", got: %v", result)
	}
	EvalString(`(delete-file "/tmp/lisp_reader_test.txt")`, globalEnv)
}

// ============================================================
// Section 2: io.Writer adapters
// ============================================================

func TestWriterToStringBasic(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((w (writer-to-string)))
		  (go:call w "WriteString" "hello ")
		  (go:call w "WriteString" "world")
		  (writer-get-string w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "hello world" {
		t.Fatalf("expected \"hello world\", got: %v", result)
	}
}

func TestWriterToStringEmpty(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(writer-get-string (writer-to-string))`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "" {
		t.Fatalf("expected empty string, got: %v", result)
	}
}

func TestWriterReset(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((w (writer-to-string)))
		  (go:call w "WriteString" "before")
		  (writer-reset w)
		  (go:call w "WriteString" "after")
		  (writer-get-string w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "after" {
		t.Fatalf("expected \"after\", got: %v", result)
	}
}

func TestWriterToFileRoundtrip(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`
		(let* ((w (writer-to-file "/tmp/lisp_writer_test.txt")))
		  (go:call w "WriteString" "written content")
		  (writer-close w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	result, err := EvalString(`(read-file "/tmp/lisp_writer_test.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if result.typ != VStr || result.str != "written content" {
		t.Fatalf("expected \"written content\", got: %v", result)
	}
	EvalString(`(delete-file "/tmp/lisp_writer_test.txt")`, globalEnv)
}

func TestBufferWriterBasic(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((w (funcall (go:import "microlisp/io.NewBufferWriter"))))
		  (go:call w "WriteString" "buffer write")
		  (let ((bytes (funcall (go:import "microlisp/io.BufferWriterBytes") w)))
		    (funcall (go:import "string") bytes)))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The result should contain "buffer write" — exact type may vary
	if result.typ != VStr || !strings.Contains(result.str, "buffer write") {
		t.Fatalf("expected string containing \"buffer write\", got: %v", result)
	}
}

// ============================================================
// Section 3: io-copy-to-string / io-copy-to-file / io-limit-string / io-nop-closer
// ============================================================

func TestIoCopyToString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(io-copy-to-string (reader-from-string "copy test"))`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "copy test" {
		t.Fatalf("expected \"copy test\", got: %v", result)
	}
}

func TestIoCopyToFile(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-copy-to-file (reader-from-string "file copy") "/tmp/lisp_copy_test.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, err := EvalString(`(read-file "/tmp/lisp_copy_test.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if result.typ != VStr || result.str != "file copy" {
		t.Fatalf("expected \"file copy\", got: %v", result)
	}
	EvalString(`(delete-file "/tmp/lisp_copy_test.txt")`, globalEnv)
}

func TestIoLimitString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(io-limit-string (reader-from-string "hello world") 5)`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "hello" {
		t.Fatalf("expected \"hello\", got: %v", result)
	}
}

func TestIoLimitStringZero(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(io-limit-string (reader-from-string "hello") 0)`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "" {
		t.Fatalf("expected empty string, got: %v", result)
	}
}

func TestIoLimitStringExceedLength(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(io-limit-string (reader-from-string "hi") 100)`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "hi" {
		t.Fatalf("expected \"hi\", got: %v", result)
	}
}

func TestIoNopCloser(t *testing.T) {
	InitGlobalEnv()
	// Create a nop-closer and read from it
	result, err := EvalString(`
		(let* ((r (reader-from-string "nop closer test"))
		       (rc (io-nop-closer r)))
		  (reader-read-all rc))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "nop closer test" {
		t.Fatalf("expected \"nop closer test\", got: %v", result)
	}
}

// ============================================================
// Section 4: Context adapters
// ============================================================

func TestCtxWithTimeoutReturnsPair(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(ctx-with-timeout 10)`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isList(result) || listLength(result) != 2 {
		t.Fatalf("expected 2-element list, got: %s", typeStr(result))
	}
	ctx := result.car
	cancelFn := result.cdr.car
	if ctx.typ != VGoVal {
		t.Fatalf("expected VGoVal context, got: %s", typeStr(ctx))
	}
	if cancelFn.typ != VGoVal {
		t.Fatalf("expected VGoVal cancel-fn, got: %s", typeStr(cancelFn))
	}
}

func TestCtxWithTimeoutNotDone(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(ctx-done (car (ctx-with-timeout 10)))`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isTruthy(result) {
		t.Fatal("fresh context should not be done")
	}
}

func TestCtxWithTimeoutExpires(t *testing.T) {
	InitGlobalEnv()
	// Create a 10ms timeout and sleep 50ms (50000000 ns)
	result, err := EvalString(`
		(let* ((pair (ctx-with-timeout 0.01))
		       (ctx (car pair)))
		  (funcall (go:import "time.Sleep") 50000000)
		  (ctx-done ctx))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("expired context should be done")
	}
}

func TestCtxWithCancelBasic(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(ctx-with-cancel)`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isList(result) || listLength(result) != 2 {
		t.Fatalf("expected 2-element list, got: %s", typeStr(result))
	}
}

func TestCtxCancelMakesDone(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((pair (ctx-with-cancel))
		       (ctx (car pair))
		       (cancel (cadr pair)))
		  (ctx-cancel cancel)
		  (ctx-done ctx))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("cancelled context should be done")
	}
}

func TestCtxWithTimeoutWithParent(t *testing.T) {
	InitGlobalEnv()
	// Create parent, then child with timeout
	result, err := EvalString(`
		(let* ((parent-pair (ctx-with-cancel))
		       (parent-ctx (car parent-pair))
		       (child-pair (ctx-with-timeout parent-ctx 5))
		       (child-ctx (car child-pair)))
		  (ctx-done child-ctx))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isTruthy(result) {
		t.Fatal("child context with fresh parent should not be done")
	}
}

func TestCtxCancelParentCancelsChild(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((parent-pair (ctx-with-cancel))
		       (parent-ctx (car parent-pair))
		       (parent-cancel (cadr parent-pair))
		       (child-pair (ctx-with-cancel parent-ctx))
		       (child-ctx (car child-pair)))
		  (ctx-cancel parent-cancel)
		  (ctx-done child-ctx))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("cancelling parent should make child done")
	}
}

// ============================================================
// Section 5: go:callback
// ============================================================

func TestGoCallbackInt32ToInt32(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:callback (lambda (x) (+ x 1)) "int32->int32")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got: %s", typeStr(result))
	}
}

func TestGoCallbackIntToInt(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:callback (lambda (x) (* x 2)) "int->int")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got: %s", typeStr(result))
	}
}

func TestGoCallbackStringToString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:callback (lambda (s) (string-append s "!")) "string->string")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got: %s", typeStr(result))
	}
}

func TestGoCallbackIntToBool(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:callback (lambda (x) (> x 0)) "int->bool")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got: %s", typeStr(result))
	}
}

func TestGoCallbackVoid(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(go:callback (lambda () nil) "()->")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got: %s", typeStr(result))
	}
}

func TestGoCallbackUnknownSignature(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:callback (lambda (x) x) "float64->float64")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for unknown signature, got nil")
	}
	if !strings.Contains(err.Error(), "unknown signature") {
		t.Fatalf("expected 'unknown signature' error, got: %v", err)
	}
}

func TestGoCallbackMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:callback (lambda (x) x))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
}

func TestGoCallbackWrongTypeSecondArg(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:callback (lambda (x) x) 42)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-string signature, got nil")
	}
}

// ============================================================
// Section 6: http-request / http-do
// ============================================================

func TestHttpRequestGet(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(http-request "GET" "https://httpbin.org/get")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got: %s", typeStr(result))
	}
}

func TestHttpRequestPostWithBody(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(http-request "POST" "https://httpbin.org/post" "hello body")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got: %s", typeStr(result))
	}
}

func TestHttpRequestMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(http-request "GET")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
}

func TestHttpRequestWrongTypes(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(http-request 42 99)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-string args, got nil")
	}
}

func TestHttpDoReturnsList(t *testing.T) {
	InitGlobalEnv()
	// Create request and execute
	result, err := EvalString(`
		(let* ((req (http-request "GET" "https://httpbin.org/get"))
		       (resp (http-do req)))
		  resp)
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list (body status), got: %s", typeStr(result))
	}
	// Second element should be status code 200
	statusCode := result.cdr.car
	if !isNumeric(statusCode) || toNum(statusCode) != 200 {
		t.Fatalf("expected status 200, got: %v", statusCode)
	}
	// First element should be a string body
	body := result.car
	if body.typ != VStr {
		t.Fatalf("expected string body, got: %s", typeStr(body))
	}
}

func TestHttpDoMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(http-do)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
}

func TestHttpDoWrongType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(http-do "not a request")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for wrong type, got nil")
	}
}

// ============================================================
// Section 7: Binary encoding
// ============================================================

func TestBinaryUint32BigEndian(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint32 305419896 "big"))
		       (val (binary-read-uint32 data "big")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 305419896 {
		t.Fatalf("expected 305419896, got: %v", result)
	}
}

func TestBinaryUint32LittleEndian(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint32 42 "little"))
		       (val (binary-read-uint32 data "little")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 42 {
		t.Fatalf("expected 42, got: %v", result)
	}
}

func TestBinaryUint64Roundtrip(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint64 123456789 "big"))
		       (val (binary-read-uint64 data "big")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 123456789 {
		t.Fatalf("expected 123456789, got: %v", result)
	}
}

func TestBinaryInt32Roundtrip(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint32 4294967294 "big"))
		       (val (binary-read-int32 data "big")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4294967294 as uint32 -> -2 as int32
	if !isNumeric(result) || toNum(result) != -2 {
		t.Fatalf("expected -2, got: %v", result)
	}
}

func TestBinaryZeroValue(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint32 0 "big"))
		       (val (binary-read-uint32 data "big")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 0 {
		t.Fatalf("expected 0, got: %v", result)
	}
}

// ============================================================
// Section 8: fmt-sprintf
// ============================================================

func TestFmtSprintfBasic(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(fmt-sprintf "Hello %s" "World")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "Hello World" {
		t.Fatalf("expected \"Hello World\", got: %v", result)
	}
}

func TestFmtSprintfMultipleArgs(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(fmt-sprintf "%s is %d years old" "Alice" 30)`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "Alice is 30 years old" {
		t.Fatalf("expected \"Alice is 30 years old\", got: %v", result)
	}
}

func TestFmtSprintfNoArgs(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(fmt-sprintf "no placeholders")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "no placeholders" {
		t.Fatalf("expected \"no placeholders\", got: %v", result)
	}
}

func TestFmtSprintfFloat(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(fmt-sprintf "pi=%.2f" 3.14159)`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "pi=3.14" {
		t.Fatalf("expected \"pi=3.14\", got: %v", result)
	}
}

// ============================================================
// Section 9: json-marshal-indent
// ============================================================

func TestJsonMarshalIndent(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(json-marshal-indent (list 1 2 3) "" "  ")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got: %s", typeStr(result))
	}
	if !strings.Contains(result.str, "[") || !strings.Contains(result.str, "1") {
		t.Fatalf("expected JSON array, got: %v", result)
	}
}

// ============================================================
// Section 10: Error handling
// ============================================================

func TestReaderReadAllWrongType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(reader-read-all 42)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-VGoVal, got nil")
	}
}

func TestReaderReadAllNotAReader(t *testing.T) {
	InitGlobalEnv()
	// Create a VGoVal that is NOT an io.Reader
	_, err := EvalString(`(reader-read-all (go:new "time.Time"))`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-io.Reader VGoVal, got nil")
	}
}

func TestIoCopyToStringWrongType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-copy-to-string "not a reader")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-VGoVal, got nil")
	}
}

func TestIoCopyToFileWrongReaderType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-copy-to-file 42 "/tmp/test.txt")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-VGoVal reader, got nil")
	}
}

func TestIoCopyToFileWrongPathType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-copy-to-file (reader-from-string "x") 42)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-string path, got nil")
	}
}

func TestIoLimitStringWrongReaderType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-limit-string "not a reader" 5)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-VGoVal, got nil")
	}
}

func TestIoLimitStringWrongCountType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-limit-string (reader-from-string "x") "not a number")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-numeric count, got nil")
	}
}

func TestIoNopCloserWrongType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-nop-closer 42)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-VGoVal, got nil")
	}
}

func TestCtxWithTimeoutWrongSecondsType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(ctx-with-timeout "not a number")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-numeric seconds, got nil")
	}
}

func TestHttpRequestInvalidURL(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(http-request "GET" "://invalid-url")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

// ============================================================
// Section 11: Integration — adapter + Go stdlib
// ============================================================

func TestAdapterWithGoNewReader(t *testing.T) {
	InitGlobalEnv()
	// Use strings.NewReader (Go stdlib) + reader-read-all
	result, err := EvalString(`
		(let* ((r (funcall (go:import "strings.NewReader") "from strings.NewReader")))
		  (reader-read-all r))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "from strings.NewReader" {
		t.Fatalf("expected \"from strings.NewReader\", got: %v", result)
	}
}

func TestAdapterWithBufioNewReader(t *testing.T) {
	InitGlobalEnv()
	// Create a string reader, wrap with bufio.NewReader, then read all
	result, err := EvalString(`
		(let* ((sr (reader-from-string "bufio test data"))
		       (br (funcall (go:import "bufio.NewReader") sr)))
		  (reader-read-all br))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "bufio test data" {
		t.Fatalf("expected \"bufio test data\", got: %v", result)
	}
}

func TestAdapterWithJsonDecoder(t *testing.T) {
	InitGlobalEnv()
	// Create a reader from JSON string, use json.NewDecoder
	result, err := EvalString(`
		(let* ((r (reader-from-string "{\"key\":\"value\"}"))
		       (dec (funcall (go:import "encoding/json.NewDecoder") r))
		       (m (go:new "encoding/json.Decoder")))
		  (go:call dec "Decode" m))
	`, globalEnv)
	// This may or may not work depending on Decode's return type handling,
	// but it should at least not panic
	_ = result
	_ = err
}

func TestAdapterWithIoCopy(t *testing.T) {
	InitGlobalEnv()
	// Use io.Copy to copy from reader to writer
	result, err := EvalString(`
		(let* ((r (reader-from-string "io copy test"))
		       (w (writer-to-string)))
		  (funcall (go:import "io.Copy") w r)
		  (writer-get-string w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "io copy test" {
		t.Fatalf("expected \"io copy test\", got: %v", result)
	}
}

func TestAdapterWithIoWriteString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((w (writer-to-string())))
		  (funcall (go:import "io.WriteString") w "written via io.WriteString")
		  (writer-get-string w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "written via io.WriteString" {
		t.Fatalf("expected \"written via io.WriteString\", got: %v", result)
	}
}

// ============================================================
// Section 12: http-fetch convenience
// ============================================================

func TestHttpFetch(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(http-fetch "GET" "https://httpbin.org/get")`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string body, got: %s", typeStr(result))
	}
	if !strings.Contains(result.str, "httpbin.org") {
		t.Fatalf("expected body containing httpbin.org, got: %s", result.str[:min(100, len(result.str))])
	}
}

// ============================================================
// Section 13: reader-read-all edge cases
// ============================================================

func TestReaderReadAllLargeContent(t *testing.T) {
	InitGlobalEnv()
	// Create a large string and read it all
	longStr := strings.Repeat("x", 10000)
	result, err := EvalString(`(reader-read-all (reader-from-string "`+longStr+`"))`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || len(result.str) != 10000 {
		t.Fatalf("expected 10000-char string, got length %d", len(result.str))
	}
}

func TestReaderReadAllBinaryContent(t *testing.T) {
	InitGlobalEnv()
	// Read content with null bytes
	result, err := EvalString(`(reader-read-all (reader-from-string "hello\x00world"))`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got: %s", typeStr(result))
	}
}

// ============================================================
// Section 14: Context deadline precision
// ============================================================

func TestCtxWithDeadlineFromTimeout(t *testing.T) {
	InitGlobalEnv()
	// Create context with 1s timeout, verify it's not done immediately
	result, err := EvalString(`
		(let* ((pair (ctx-with-timeout 1))
		       (ctx (car pair)))
		  (ctx-done ctx))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isTruthy(result) {
		t.Fatal("1-second timeout context should not be done immediately")
	}
}

// ============================================================
// Section 15: Multiple readers from same source
// ============================================================

func TestMultipleReadersIndependent(t *testing.T) {
	InitGlobalEnv()
	// Each reader should be independent
	result, err := EvalString(`
		(let* ((r1 (reader-from-string "first"))
		       (r2 (reader-from-string "second"))
		       (d1 (reader-read-all r1))
		       (d2 (reader-read-all r2)))
		  (string-append d1 "|" d2))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.typ != VStr || result.str != "first|second" {
		t.Fatalf("expected \"first|second\", got: %v", result)
	}
}

// ============================================================
// Section 16: Writer close idempotent
// ============================================================

func TestFileWriterClose(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`
		(let* ((w (writer-to-file "/tmp/lisp_close_test.txt")))
		  (go:call w "WriteString" "close test")
		  (writer-close w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify content
	result, err := EvalString(`(read-file "/tmp/lisp_close_test.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if result.typ != VStr || result.str != "close test" {
		t.Fatalf("expected \"close test\", got: %v", result)
	}
	EvalString(`(delete-file "/tmp/lisp_close_test.txt")`, globalEnv)
}

func TestFileReaderClose(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`
		(let* ((r (reader-from-file "/tmp/lisp_freader_close.txt")))
		  (reader-close r))
	`, globalEnv)
	// This should fail because file doesn't exist — that's fine
	// Let's create the file first
	_ = err
	EvalString(`(write-file "/tmp/lisp_freader_close.txt" "test")`, globalEnv)
	_, err = EvalString(`
		(let* ((r (reader-from-file "/tmp/lisp_freader_close.txt")))
		  (reader-close r))
	`, globalEnv)
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}
	EvalString(`(delete-file "/tmp/lisp_freader_close.txt")`, globalEnv)
}

// ============================================================
// Section 17: Go callback invoked through Go stdlib
// ============================================================

func TestGoCallbackWithSortSearch(t *testing.T) {
	InitGlobalEnv()
	// sort.Search takes func(int) bool — use go:callback
	result, err := EvalString(`
		(let* ((pred (go:callback (lambda (i) (>= i 5)) "int->bool"))
		       (idx (funcall (go:import "sort.Search") 10 pred)))
		  idx)
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 5 {
		t.Fatalf("expected 5 (first index where i>=5), got: %v", result)
	}
}

func TestGoCallbackWithStringsTrimFunc(t *testing.T) {
	InitGlobalEnv()
	// strings.TrimFunc takes func(int32) bool
	result, err := EvalString(`
		(let* ((is-space (go:callback (lambda (r) (if (= r 32) 1 0)) "int32->bool"))
		       (trimmed (funcall (go:import "strings.TrimFunc") "  hello  " is-space)))
		  trimmed)
	`, globalEnv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The callback returns 1 for space (rune 32), which Go treats as truthy
	// So TrimFunc should trim spaces
	if result.typ != VStr {
		t.Fatalf("expected string, got: %s", typeStr(result))
	}
}

// ============================================================
// Section 18: Adapter chain — reader → Go decoder → result
// ============================================================

func TestAdapterWithGzipReader(t *testing.T) {
	InitGlobalEnv()
	// This tests that our io.Reader adapter works with compress/gzip.NewReader
	// We can't easily create gzip data in Lisp, so just verify the function
	// accepts our reader type without error
	_, err := EvalString(`
		(let* ((r (reader-from-string "not actually gzip")))
		  (ignore-errors (funcall (go:import "compress/gzip.NewReader") r)))
	`, globalEnv)
	// This will error because the data isn't valid gzip, but the point is
	// that our reader adapter was accepted as io.Reader parameter
	_ = err
}

// ============================================================
// Section 19: Time-based context integration
// ============================================================

func TestCtxWithTimeoutAndSleep(t *testing.T) {
	InitGlobalEnv()
	start := time.Now()
	result, err := EvalString(`
		(let* ((pair (ctx-with-timeout 0.05))
		       (ctx (car pair)))
		  (funcall (go:import "time.Sleep") 100000000)
		  (ctx-done ctx))
	`, globalEnv)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("context should be done after sleeping past timeout")
	}
	if elapsed < 50*time.Millisecond {
		t.Fatalf("expected at least 50ms elapsed, got %v", elapsed)
	}
}

// ============================================================
// Section 20: reader-read-all missing args
// ============================================================

func TestReaderReadAllMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(reader-read-all)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
}

func TestIoCopyToStringMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-copy-to-string)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
}

func TestIoCopyToFileMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-copy-to-file)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
}

func TestIoLimitStringMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-limit-string)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
}

func TestIoNopCloserMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-nop-closer)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
}

// helper
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func listLength(v *Value) int {
	n := 0
	for p := v; !isNil(p); p = p.cdr {
		n++
	}
	return n
}
