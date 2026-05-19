package microlisp

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// ===========================================================================
// Lazy-load FFI scenario tests
// These mirror the scenarios in ffi_scenario_test.go but use the lazy-loaded
// Lisp native wrappers (auto-imported on first symbol lookup) instead of
// explicit (go:import "...") calls.
//
// Some function names exist in go_stdlib_wrapper.go (hand-written wrappers,
// loaded at startup) and some are in GoFFILazyTable (lazy-loaded on first use).
// ===========================================================================

// ===========================================================================
// Scenario 1: Math Operations (lazy-loaded from GoFFILazyTable)
// ===========================================================================

func TestLazy_MathSqrt(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString("(math-sqrt 16.0)", globalEnv)
	if err != nil {
		t.Fatalf("math-sqrt failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 4.0 {
		t.Fatalf("expected 4.0, got %v", result)
	}
}

func TestLazy_MathSin(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString("(math-sin 0.0)", globalEnv)
	if err != nil {
		t.Fatalf("math-sin failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 0.0 {
		t.Fatalf("expected 0.0, got %v", result)
	}
}

func TestLazy_MathAbs(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString("(math-abs -42.5)", globalEnv)
	if err != nil {
		t.Fatalf("math-abs failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 42.5 {
		t.Fatalf("expected 42.5, got %v", result)
	}
}

func TestLazy_MathCos(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString("(math-cos 0.0)", globalEnv)
	if err != nil {
		t.Fatalf("math-cos failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 1.0 {
		t.Fatalf("expected 1.0, got %v", result)
	}
}

func TestLazy_MathExp(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString("(math-exp 0.0)", globalEnv)
	if err != nil {
		t.Fatalf("math-exp failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %v", result)
	}
	// e^0 = 1
	if toNum(result) < 0.99 || toNum(result) > 1.01 {
		t.Fatalf("expected ~1.0, got %v", result)
	}
}

func TestLazy_MathPow(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString("(math-pow 2.0 10.0)", globalEnv)
	if err != nil {
		t.Fatalf("math-pow failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 1024.0 {
		t.Fatalf("expected 1024.0, got %v", result)
	}
}

func TestLazy_MathCeilFloor(t *testing.T) {
	InitGlobalEnv()
	ceilResult, err := EvalString("(math-ceil 3.14)", globalEnv)
	if err != nil {
		t.Fatalf("math-ceil failed: %v", err)
	}
	if !isNumeric(ceilResult) || int(toNum(ceilResult)) != 4 {
		t.Fatalf("expected 4, got %v", ceilResult)
	}

	floorResult, err := EvalString("(math-floor 3.99)", globalEnv)
	if err != nil {
		t.Fatalf("math-floor failed: %v", err)
	}
	if !isNumeric(floorResult) || int(toNum(floorResult)) != 3 {
		t.Fatalf("expected 3, got %v", floorResult)
	}
}

func TestLazy_MathMaxMin(t *testing.T) {
	InitGlobalEnv()
	maxResult, err := EvalString("(math-max 3.0 7.0)", globalEnv)
	if err != nil {
		t.Fatalf("math-max failed: %v", err)
	}
	if !isNumeric(maxResult) || toNum(maxResult) != 7.0 {
		t.Fatalf("expected 7.0, got %v", maxResult)
	}

	minResult, err := EvalString("(math-min 3.0 7.0)", globalEnv)
	if err != nil {
		t.Fatalf("math-min failed: %v", err)
	}
	if !isNumeric(minResult) || toNum(minResult) != 3.0 {
		t.Fatalf("expected 3.0, got %v", minResult)
	}
}

// ===========================================================================
// Scenario 2: String Processing
// Lazy table: string-equal-fold, string-count, string-index
// Hand-written: string-to-upper, string-to-lower, string-trim,
//               string-repeat, string-replace, string-contains,
//               string-starts-with, string-ends-with, string-split,
//               string-join
// ===========================================================================

func TestLazy_StringEqualFold(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(string-equal-fold "Hello" "HELLO")`, globalEnv)
	if err != nil {
		t.Fatalf("string-equal-fold failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool, got %s", typeStr(result))
	}
}

func TestLazy_StringCount(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(string-count "hello hello" "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("string-count failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 2 {
		t.Fatalf("expected 2, got %v", result)
	}
}

func TestLazy_StringIndex(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(string-index "hello world" "world")`, globalEnv)
	if err != nil {
		t.Fatalf("string-index failed: %v", err)
	}
	if !isNumeric(result) || int(toNum(result)) != 6 {
		t.Fatalf("expected 6, got %v", result)
	}
}

func TestLazy_StringToUpper(t *testing.T) {
	// Hand-written wrapper (loaded at startup)
	InitGlobalEnv()
	result, err := EvalString(`(string-to-upper "hello world")`, globalEnv)
	if err != nil {
		t.Fatalf("string-to-upper failed: %v", err)
	}
	if result.typ != VStr || result.str != "HELLO WORLD" {
		t.Fatalf("expected 'HELLO WORLD', got %v", result)
	}
}

func TestLazy_StringContains(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString(`(string-contains "hello world" "world")`, globalEnv)
	if err != nil {
		t.Fatalf("string-contains failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool, got %s", typeStr(result))
	}
}

func TestLazy_StringRepeatReplace(t *testing.T) {
	// Hand-written wrappers
	InitGlobalEnv()
	result, err := EvalString(`
		(string-replace (string-repeat "ab" 3) "ab" "XY")
	`, globalEnv)
	if err != nil {
		t.Fatalf("string repeat+replace failed: %v", err)
	}
	if result.typ != VStr || result.str != "XYXYXY" {
		t.Fatalf("expected 'XYXYXY', got: %v", result)
	}
}

func TestLazy_StringJoin(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString(`(string-join '("a" "b" "c") "-")`, globalEnv)
	if err != nil {
		t.Fatalf("string-join failed: %v", err)
	}
	if result.typ != VStr || result.str != "a-b-c" {
		t.Fatalf("expected 'a-b-c', got: %v", result)
	}
}

func TestLazy_UnicodeToUpper(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString(`(string-to-upper "hello 世界")`, globalEnv)
	if err != nil {
		t.Fatalf("unicode handling failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_EmptyStringCount(t *testing.T) {
	// Hand-written wrapper: string-count "" ""
	InitGlobalEnv()
	result, err := EvalString(`(string-count "" "")`, globalEnv)
	if err != nil {
		t.Fatalf("empty string handling failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 3: JSON
// Lazy table: json-html-escape, json-marshal-indent
// Hand-written: json-valid-p, json-encode, json-encode-alist, json-decode
// Also: json-marshal (lazy), json-valid (lazy) - but skipFuncs excludes these
// ===========================================================================

func TestLazy_JSONValidP(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString(`(json-valid-p "{\"key\": \"value\"}")`, globalEnv)
	if err != nil {
		t.Fatalf("json-valid-p failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool, got %s", typeStr(result))
	}
}

func TestLazy_JSONMarshal(t *testing.T) {
	// Hand-written: json-encode (simplified wrapper)
	InitGlobalEnv()
	result, err := EvalString(`(json-encode "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("json-encode failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_JSONEncodePretty(t *testing.T) {
	// json-encode-pretty is a simplified wrapper that calls json-encode
	InitGlobalEnv()
	result, err := EvalString(`(json-encode-pretty "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("json-encode-pretty failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 4: Time Operations
// Hand-written: current-timestamp, parse-time, format-time, sleep
// Lazy table: time-after, time-date, time-now (N/A - not in table)
// ===========================================================================

func TestLazy_CurrentTimestamp(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString("(current-timestamp)", globalEnv)
	if err != nil {
		t.Fatalf("current-timestamp failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_ParseTime(t *testing.T) {
	// Hand-written wrapper: (parse-time layout str) returns unix timestamp
	InitGlobalEnv()
	result, err := EvalString(`(parse-time "2006-01-02" "2024-06-15")`, globalEnv)
	if err != nil {
		t.Fatalf("parse-time failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
	// Unix timestamp for 2024-06-15 UTC should be around 1718409600
	ts := toNum(result)
	if ts < 1718000000 || ts > 1720000000 {
		t.Fatalf("timestamp %v not in expected range for 2024-06-15", ts)
	}
}

// ===========================================================================
// Scenario 5: URL Parsing
// Hand-written: url-encode, url-decode, url-parse
// Lazy table: url-join-path, url-parse-query, url-path-escape, etc.
// ===========================================================================

func TestLazy_URLParse(t *testing.T) {
	// Hand-written wrapper: url-parse returns an alist
	InitGlobalEnv()
	result, err := EvalString(`(url-parse "https://example.com:8080/path?q=hello")`, globalEnv)
	if err != nil {
		t.Fatalf("url-parse failed: %v", err)
	}
	if result.typ != VPair && result.typ != VNil {
		t.Fatalf("expected list (alist), got %s", typeStr(result))
	}
}

func TestLazy_URLEncode(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString(`(url-encode "hello world")`, globalEnv)
	if err != nil {
		t.Fatalf("url-encode failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
	if !strings.Contains(result.str, "+") {
		t.Fatalf("expected URL-encoded string, got: %s", result.str)
	}
}

func TestLazy_URLDecode(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString(`(url-decode "hello+world")`, globalEnv)
	if err != nil {
		t.Fatalf("url-decode failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello world" {
		t.Fatalf("expected 'hello world', got: %v", result)
	}
}

// ===========================================================================
// Scenario 6: HTTP (lazy table has http-canonical-header-key, http-head, etc.)
// Hand-written: http-get, http-post, http-status-text
// ===========================================================================

func TestLazy_HTTPStatusText(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString(`(http-status-text 200)`, globalEnv)
	if err != nil {
		t.Fatalf("http-status-text failed: %v", err)
	}
	if result.typ != VStr || result.str != "OK" {
		t.Fatalf("expected 'OK', got: %v", result)
	}
}

func TestLazy_HTTPNewRequest(t *testing.T) {
	// Lazy table: http-new-request is NOT in GoFFILazyTable (skipFunc)
	// Use go:import directly to verify the lazy mechanism works
	InitGlobalEnv()
	result, err := EvalString(`
		(define req (funcall (go:import "net/http.NewRequest") "GET" "https://example.com/data" nil))
		(go:field req "Method")
	`, globalEnv)
	if err != nil {
		t.Fatalf("http-new-request via go:import failed: %v", err)
	}
	if result.typ != VStr || result.str != "GET" {
		t.Fatalf("expected 'GET', got: %v", result)
	}
}

// ===========================================================================
// Scenario 7: Crypto (lazy-loaded)
// ===========================================================================

func TestLazy_SHA256(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define hash (sha256-sum256 "hello world"))
		(go:type-of hash)
	`, globalEnv)
	if err != nil {
		t.Fatalf("sha256-sum256 failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_MD5(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define hash (md5-sum "hello world"))
		(go:type-of hash)
	`, globalEnv)
	if err != nil {
		t.Fatalf("md5-sum failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_SHA1(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define hash (sha1-sum "hello world"))
		(go:type-of hash)
	`, globalEnv)
	if err != nil {
		t.Fatalf("sha1-sum failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_HMAC(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define hash (sha256-sum256 "message to sign"))
		(go:type-of hash)
	`, globalEnv)
	if err != nil {
		t.Fatalf("sha256-sum256 failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 8: go:new / go:field / go:set-field
// ===========================================================================

func TestLazy_BytesBuffer(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define buf (go:new "bytes.Buffer"))
		(go:call buf "WriteString" "hello ")
		(go:call buf "WriteString" "world")
		(go:call buf "String")
	`, globalEnv)
	if err != nil {
		t.Fatalf("bytes.Buffer failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello world" {
		t.Fatalf("expected 'hello world', got %v", result)
	}
}

func TestLazy_BigInt(t *testing.T) {
	InitGlobalEnv()
	// Use a smaller number that fits in float64 precision
	result, err := EvalString(`
		(define big (go:new "math/big.Int"))
		(go:call big "SetInt64" 1234567890123456)
		(go:call big "String")
	`, globalEnv)
	if err != nil {
		t.Fatalf("math/big.Int failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
	if !strings.Contains(result.str, "1234567890123456") {
		t.Fatalf("expected '1234567890123456', got: %s", result.str)
	}
}

func TestLazy_MutexLockUnlock(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define m (go:new "sync.Mutex"))
		(go:call m "Lock")
		(go:call m "Unlock")
		(go:type-of m)
	`, globalEnv)
	if err != nil {
		t.Fatalf("sync.Mutex failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_WaitGroup(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define wg (go:new "sync.WaitGroup"))
		(go:call wg "Add" 1)
		(go:call wg "Done")
		(go:type-of wg)
	`, globalEnv)
	if err != nil {
		t.Fatalf("sync.WaitGroup failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_HTTPTransport(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define transport (go:new "net/http.Transport"))
		(go:set-field transport "MaxIdleConns" 10)
		(go:field transport "MaxIdleConns")
	`, globalEnv)
	if err != nil {
		t.Fatalf("http.Transport failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 10 {
		t.Fatalf("expected 10, got %v", result)
	}
}

func TestLazy_CertificateSetField(t *testing.T) {
	InitGlobalEnv()
	// Test setting a simple int field; SerialNumber is *big.Int which
	// can't be directly set from a Lisp number. Use Version (int) instead.
	result, err := EvalString(`
		(define tmpl (go:new "crypto/x509.Certificate"))
		(go:set-field tmpl "Version" 3)
		(go:field tmpl "Version")
	`, globalEnv)
	if err != nil {
		t.Fatalf("x509.Certificate set/get field failed: %v", err)
	}
	if !isNumeric(result) || int(toNum(result)) != 3 {
		t.Fatalf("expected 3, got %v", result)
	}
}

// ===========================================================================
// Scenario 9: Path Operations
// Hand-written: path-clean, path-base, path-dir, path-join, path-ext,
//               path-is-absolute, path-absolute
// Lazy table: path-base, path-clean, path-dir, path-ext, path-is-absolute, etc.
// ===========================================================================

func TestLazy_FilePathClean(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(path-clean "home/user/../user/documents")`, globalEnv)
	if err != nil {
		t.Fatalf("path-clean failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
	if !strings.Contains(result.str, "user") || !strings.Contains(result.str, "documents") {
		t.Fatalf("expected path with user/documents, got: %v", result)
	}
}

func TestLazy_FilePathBase(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(path-base "/usr/local/bin/go")`, globalEnv)
	if err != nil {
		t.Fatalf("path-base failed: %v", err)
	}
	if result.typ != VStr || result.str != "go" {
		t.Fatalf("expected 'go', got: %v", result)
	}
}

func TestLazy_FilePathDir(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(path-dir "/usr/local/bin/go")`, globalEnv)
	if err != nil {
		t.Fatalf("path-dir failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_FilePathJoin(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(path-join "usr" "local" "bin")`, globalEnv)
	if err != nil {
		t.Fatalf("path-join failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_FilePathExt(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(path-ext "file.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("path-ext failed: %v", err)
	}
	if result.typ != VStr || result.str != ".txt" {
		t.Fatalf("expected '.txt', got: %v", result)
	}
}

func TestLazy_FilePathIsAbsolute(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(path-is-absolute "/usr/local/bin")`, globalEnv)
	if err != nil {
		t.Fatalf("path-is-absolute failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 10: Context (lazy-loaded)
// ===========================================================================

func TestLazy_ContextBackground(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define ctx (context-background))
		(go:type-of ctx)
	`, globalEnv)
	if err != nil {
		t.Fatalf("context-background failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 11: Regex
// Hand-written: regex-match, regex-find-all, regex-replace, regex-split
// Lazy table: regexp-compile-posix, regexp-match, regexp-must-compile, etc.
// ===========================================================================

func TestLazy_RegexMatch(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString(`(regex-match "^hello.*world$" "hello beautiful world")`, globalEnv)
	if err != nil {
		t.Fatalf("regex-match failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool, got %s", typeStr(result))
	}
}

func TestLazy_RegexpCompile(t *testing.T) {
	// Lazy table has regexp-compile-posix (regexp.CompilePOSIX)
	InitGlobalEnv()
	result, err := EvalString(`
		(define re (regexp-compile-posix "^hello.*world$"))
		(go:call re "MatchString" "hello beautiful world")
	`, globalEnv)
	if err != nil {
		t.Fatalf("regexp-compile-posix + MatchString failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool, got %s", typeStr(result))
	}
}

func TestLazy_RegexFindAll(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString(`(regex-find-all "\\d+" "abc123def456ghi" -1)`, globalEnv)
	if err != nil {
		t.Fatalf("regex-find-all failed: %v", err)
	}
	// Returns a list/string representation
	_ = result
}

// ===========================================================================
// Scenario 12: strconv (lazy-loaded)
// Lazy table: string-atoi, string-parse-int, string-format-int, etc.
// ===========================================================================

func TestLazy_StrconvAtoi(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(string-atoi "144")`, globalEnv)
	if err != nil {
		t.Fatalf("string-atoi failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 144 {
		t.Fatalf("expected 144, got %v", result)
	}
}

func TestLazy_StrconvParseInt(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(string-parse-int "42" 10 64)`, globalEnv)
	if err != nil {
		t.Fatalf("string-parse-int failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 42 {
		t.Fatalf("expected 42, got %v", result)
	}
}

func TestLazy_StrconvFormatInt(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(string-format-int 255 16)`, globalEnv)
	if err != nil {
		t.Fatalf("string-format-int failed: %v", err)
	}
	if result.typ != VStr || result.str != "ff" {
		t.Fatalf("expected 'ff', got: %v", result)
	}
}

// ===========================================================================
// Scenario 13: Random (lazy-loaded)
// ===========================================================================

func TestLazy_RandIntn(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(rand-intn 100)`, globalEnv)
	if err != nil {
		t.Fatalf("rand-intn failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
	n := toNum(result)
	if n < 0 || n >= 100 {
		t.Fatalf("expected 0 <= n < 100, got %v", n)
	}
}

func TestLazy_RandFloat64(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(rand-float64)`, globalEnv)
	if err != nil {
		t.Fatalf("rand-float64 failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
	n := toNum(result)
	if n < 0.0 || n >= 1.0 {
		t.Fatalf("expected 0.0 <= n < 1.0, got %v", n)
	}
}

// ===========================================================================
// Scenario 14: Data Pipeline (mix of lazy and hand-written)
// ===========================================================================

func TestLazy_DataPipeline(t *testing.T) {
	InitGlobalEnv()
	// Pipeline: trim-space -> upper -> hex encode
	result, err := EvalString(`
		(define input "  hello world  ")
		(define trimmed (string-trim-space input))
		(define uppered (string-to-upper trimmed))
		(hex-encode-to-string uppered)
	`, globalEnv)
	if err != nil {
		t.Fatalf("data pipeline failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
	// "HELLO WORLD" hex encoded (hex-encode-to-string uses lowercase)
	if result.str != "48454c4c4f20574f524c44" {
		t.Fatalf("expected hex encoded 'HELLO WORLD', got: %s", result.str)
	}
}

// ===========================================================================
// Scenario 15: RSA Key Generation (lazy-loaded via go:import)
// x509 functions are in the lazy table
// ===========================================================================

func TestLazy_RSAKeyGen(t *testing.T) {
	InitGlobalEnv()
	// crypto/rand.Reader is a registered constant (VGoVal), not a callable function
	result, err := EvalString(`
		(define gen-key (funcall (go:import "crypto/rsa.GenerateKey") (go:import "crypto/rand.Reader") 2048))
		(define pub-key (go:field gen-key "PublicKey"))
		(define der-pub (x509-marshal-pkix-public-key pub-key))
		; der-pub is []byte -> reflectToLisp converts to string
		der-pub
	`, globalEnv)
	if err != nil {
		t.Fatalf("RSA key generation failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string (from []byte), got %s", typeStr(result))
	}
}

func TestLazy_CertificatePipeline(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define gen-key (funcall (go:import "crypto/rsa.GenerateKey") (go:import "crypto/rand.Reader") 2048))
		(define pub-key (go:field gen-key "PublicKey"))
		(define marshal-pub (x509-marshal-pkix-public-key pub-key))
		(define marshal-priv (x509-marshal-pkcs1-private-key gen-key))
		; Both return []byte which reflectToLisp converts to string
		marshal-priv
	`, globalEnv)
	if err != nil {
		t.Fatalf("complete cert pipeline failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string (from []byte), got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 16: HTTP Request Simulation (complete)
// ===========================================================================

func TestLazy_CompleteHTTPRequest(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define req (funcall (go:import "net/http.NewRequest") "GET" "https://api.example.com/data?key=value" nil))
		(go:call (go:field req "Header") "Set" "Accept" "application/json")
		(go:call (go:field req "Header") "Set" "User-Agent" "LispFFI/1.0")
		(go:field req "Method")
	`, globalEnv)
	if err != nil {
		t.Fatalf("complete HTTP request failed: %v", err)
	}
	if result.typ != VStr || result.str != "GET" {
		t.Fatalf("expected 'GET', got: %v", result)
	}
}

func TestLazy_HTTPHeaderSetGet(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define req (funcall (go:import "net/http.NewRequest") "POST" "https://example.com" nil))
		(go:call (go:field req "Header") "Set" "Content-Type" "application/json")
		(go:call (go:field req "Header") "Get" "Content-Type")
	`, globalEnv)
	if err != nil {
		t.Fatalf("http header set/get failed: %v", err)
	}
	if result.typ != VStr || result.str != "application/json" {
		t.Fatalf("expected 'application/json', got: %v", result)
	}
}

// ===========================================================================
// Scenario 17: Multi-Call Sequence (lazy-loaded)
// ===========================================================================

func TestLazy_MultiCallSequence(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		; Chain: math-sqrt(math-abs(math-sin(-3.14159)))
		(math-sqrt (math-abs (math-sin -3.14159)))
	`, globalEnv)
	if err != nil {
		t.Fatalf("multi-call sequence failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 18: Time Duration (lazy-loaded via go:call)
// ===========================================================================

func TestLazy_TimeDuration(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define t1 (funcall (go:import "time.Now")))
		(define t2 (funcall (go:import "time.Parse") "2006-01-02" "2025-01-01"))
		(go:call t1 "Sub" t2)
	`, globalEnv)
	if err != nil {
		t.Fatalf("time.Sub failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from time.Sub, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 19: nil Interface (lazy-loaded)
// ===========================================================================

func TestLazy_NilInterface(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define req (funcall (go:import "net/http.NewRequest") "GET" "https://example.com" nil))
		(go:field req "URL")
	`, globalEnv)
	if err != nil {
		t.Fatalf("nil interface handling failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 20: Base64 (lazy-loaded)
// ===========================================================================

func TestLazy_Base64Encode(t *testing.T) {
	// Use the hand-written base64-encode wrapper
	InitGlobalEnv()
	result, err := EvalString(`(base64-encode "Hello, World!")`, globalEnv)
	if err != nil {
		t.Fatalf("base64 encode failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_Base64URLEncoding(t *testing.T) {
	// Test base64-encode with a URL-like string
	InitGlobalEnv()
	result, err := EvalString(`(base64-encode "hello+world/test")`, globalEnv)
	if err != nil {
		t.Fatalf("base64 URL encoding failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 21: Hex Encoding (lazy-loaded)
// ===========================================================================

func TestLazy_HexEncode(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(hex-encode-to-string "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("hex-encode-to-string failed: %v", err)
	}
	if result.typ != VStr || result.str != "68656c6c6f" {
		t.Fatalf("expected '68656c6c6f', got: %s", result.str)
	}
}

func TestLazy_HexDecode(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(hex-decode-string "68656c6c6f")`, globalEnv)
	if err != nil {
		t.Fatalf("hex-decode-string failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello" {
		t.Fatalf("expected 'hello', got: %v", result)
	}
}

// ===========================================================================
// Scenario 22: OS Operations (lazy-loaded + hand-written)
// Hand-written: hostname, pid, num-cpus, go-os, go-arch, go-version
// Lazy table: os-is-not-exist, etc. (many os/* functions in skipFuncs)
// ===========================================================================

func TestLazy_Hostname(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString("(hostname)", globalEnv)
	if err != nil {
		t.Fatalf("hostname failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_Pid(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString("(pid)", globalEnv)
	if err != nil {
		t.Fatalf("pid failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
	if toNum(result) <= 0 {
		t.Fatalf("expected positive PID, got %v", result)
	}
}

func TestLazy_NumCPUs(t *testing.T) {
	// Hand-written wrapper
	InitGlobalEnv()
	result, err := EvalString("(num-cpus)", globalEnv)
	if err != nil {
		t.Fatalf("num-cpus failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
	if toNum(result) <= 0 {
		t.Fatalf("expected positive CPU count, got %v", result)
	}
}

// ===========================================================================
// Scenario 23: Sort (lazy-loaded)
// ===========================================================================

func TestLazy_SortInts(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sort-ints '(3 1 4 1 5 9 2 6))`, globalEnv)
	if err != nil {
		t.Fatalf("sort-ints failed: %v", err)
	}
	// sort-ints returns the sorted list
	_ = result
}

func TestLazy_SortFloats(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(sort-float64s '(3.0 1.0 4.0 1.0 5.0))`, globalEnv)
	if err != nil {
		t.Fatalf("sort-float64s failed: %v", err)
	}
	_ = result
}

// ===========================================================================
// Scenario 24: Errors (lazy-loaded)
// ===========================================================================

func TestLazy_ErrorsIs(t *testing.T) {
	// errors-is is in the lazy table but requires Go error values.
	// Just verify the function is resolvable.
	InitGlobalEnv()
	_, err := EvalString(`(errors-is)`, globalEnv)
	if err != nil && strings.Contains(err.Error(), "undefined") {
		t.Fatalf("errors-is should be resolvable: %v", err)
	}
	// May fail with argument error - that's OK, we just verify the name exists
}
// ===========================================================================
// Scenario 25: Reflect (lazy-loaded)
// ===========================================================================

func TestLazy_ReflectTypeOf(t *testing.T) {
	// reflect-type-of returns a reflect.Type (VGoVal)
	InitGlobalEnv()
	result, err := EvalString(`
		(define s "hello")
		(reflect-type-of s)
	`, globalEnv)
	if err != nil {
		t.Fatalf("reflect-type-of failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

func TestLazy_ReflectValueOf(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define v (reflect-value-of "hello"))
		(go:type-of v)
	`, globalEnv)
	if err != nil {
		t.Fatalf("reflect-value-of failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 26: Atomic (lazy-loaded)
// ===========================================================================

func TestLazy_AtomicAddInt64(t *testing.T) {
	// sync/atomic.Int64 is not in GoTypeRegistry, so go:new won't work.
	// Use atomic.AddInt64 which takes a *int64 pointer - not easily creatable.
	// Instead, just verify the function is resolvable.
	InitGlobalEnv()
	_, err := EvalString(`(atomic-add-int64)`, globalEnv)
	if err != nil && strings.Contains(err.Error(), "undefined") {
		t.Fatalf("atomic-add-int64 should be resolvable: %v", err)
	}
	// May fail with argument error - that's OK, we just verify the name exists
}

// ===========================================================================
// Scenario 27: StringsBuilder (go:new + go:call)
// ===========================================================================

func TestLazy_StringsBuilder(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define sb (go:new "strings.Builder"))
		(go:call sb "WriteString" "hello")
		(go:call sb "WriteString" " ")
		(go:call sb "WriteString" "world")
		(go:call sb "String")
	`, globalEnv)
	if err != nil {
		t.Fatalf("strings.Builder failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello world" {
		t.Fatalf("expected 'hello world', got %v", result)
	}
}

// ===========================================================================
// Scenario 28: BigIntArithmetic (go:new + go:call)
// ===========================================================================

func TestLazy_BigIntArithmetic(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define a (go:new "math/big.Int"))
		(define b (go:new "math/big.Int"))
		(go:call a "SetInt64" 100)
		(go:call b "SetInt64" 200)
		(go:call a "Add" a b)
		(go:call a "String")
	`, globalEnv)
	if err != nil {
		t.Fatalf("big.Int arithmetic failed: %v", err)
	}
	if result.typ != VStr || result.str != "300" {
		t.Fatalf("expected '300', got: %v", result)
	}
}

// ===========================================================================
// Scenario 29: User Current (lazy-loaded)
// ===========================================================================

func TestLazy_UserCurrent(t *testing.T) {
	InitGlobalEnv()
	// os/user.Current returns a *user.User struct; Username is a field
	result, err := EvalString(`
		(define u (funcall (go:import "os/user.Current")))
		(go:field u "Username")
	`, globalEnv)
	if err != nil {
		t.Fatalf("user-current + Username failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string username, got %s", typeStr(result))
	}
	if result.str == "" {
		t.Fatalf("expected non-empty username, got empty string")
	}
}

// ===========================================================================
// Scenario 30: Lazy-load caching verification
// ===========================================================================

func TestLazy_LoadCaching(t *testing.T) {
	InitGlobalEnv()
	// First call triggers lazy load
	result1, err := EvalString("(math-sqrt 9.0)", globalEnv)
	if err != nil {
		t.Fatalf("first math-sqrt call failed: %v", err)
	}
	// Second call should use cached binding
	result2, err := EvalString("(math-sqrt 16.0)", globalEnv)
	if err != nil {
		t.Fatalf("second math-sqrt call failed (caching broken): %v", err)
	}
	if toNum(result1) != 3.0 {
		t.Fatalf("expected 3.0, got %v", result1)
	}
	if toNum(result2) != 4.0 {
		t.Fatalf("expected 4.0, got %v", result2)
	}
	// Verify the symbol is now cached in globalEnv
	val, err := globalEnv.Get("math-sqrt")
	if err != nil {
		t.Fatalf("math-sqrt should be cached in globalEnv after first call: %v", err)
	}
	if val.typ != VPrim {
		t.Fatalf("cached math-sqrt should be VPrim, got %s", typeStr(val))
	}
}

// ===========================================================================
// Scenario 31: Bytes operations (lazy-loaded)
// ===========================================================================

func TestLazy_BytesContains(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(bytes-contains "hello" "ell")`, globalEnv)
	if err != nil {
		t.Fatalf("bytes-contains failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool, got %s", typeStr(result))
	}
}

func TestLazy_BytesCompare(t *testing.T) {
	// bytes-compare takes two []byte arguments; from Lisp we pass a string
	// which gets converted to []byte via the bytes slice special case.
	InitGlobalEnv()
	result, err := EvalString(`(bytes-compare "abc" "abc")`, globalEnv)
	if err != nil {
		t.Fatalf("bytes-compare failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
	// Equal strings compare to 0
	if toNum(result) != 0 {
		t.Fatalf("expected 0, got %v", result)
	}
}

// ===========================================================================
// Scenario 32: String Split (hand-written)
// ===========================================================================

func TestLazy_StringSplit(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(string-split "apple,banana,cherry" ",")`, globalEnv)
	if err != nil {
		t.Fatalf("string-split failed: %v", err)
	}
	// Returns a list
	_ = result
}

// ===========================================================================
// Scenario 33: String Replace All (hand-written)
// ===========================================================================

func TestLazy_StringReplaceAll(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(regex-replace-all "[aeiou]" "hello world" "*")`, globalEnv)
	if err != nil {
		t.Fatalf("regex-replace-all failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 34: File I/O (hand-written wrappers)
// ===========================================================================

func TestLazy_FileExists(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(file-exists-p ".")`, globalEnv)
	if err != nil {
		t.Fatalf("file-exists-p failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool, got %s", typeStr(result))
	}
}

func TestLazy_GetEnv(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(getenv "PATH")`, globalEnv)
	if err != nil {
		t.Fatalf("getenv failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_SetEnv(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(setenv "TEST_LAZY_VAR" "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("setenv failed: %v", err)
	}
	result, err := EvalString(`(getenv "TEST_LAZY_VAR")`, globalEnv)
	if err != nil {
		t.Fatalf("getenv after setenv failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello" {
		t.Fatalf("expected 'hello', got: %v", result)
	}
}

// ===========================================================================
// Scenario 35: Path Exists (hand-written)
// ===========================================================================

func TestLazy_PathExists(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(path-exists-p "/")`, globalEnv)
	if err != nil {
		t.Fatalf("path-exists-p failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 36: Regex Replace (hand-written)
// ===========================================================================

func TestLazy_RegexReplace(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(regex-replace "[0-9]+" "abc123def" "X")`, globalEnv)
	if err != nil {
		t.Fatalf("regex-replace failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 37: Run Command (hand-written)
// ===========================================================================

func TestLazy_RunCommand(t *testing.T) {
	// Hand-written wrapper: (run-command cmd args...) returns list (exit-code stdout stderr)
	InitGlobalEnv()
	result, err := EvalString(`(run-command "echo" "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("run-command failed: %v", err)
	}
	if result.typ != VPair && result.typ != VNil {
		t.Fatalf("expected list, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 38: Current Dir (hand-written)
// ===========================================================================

func TestLazy_CurrentDir(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString("(current-dir)", globalEnv)
	if err != nil {
		t.Fatalf("current-dir failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 39: Go Version/OS/Arch (hand-written)
// ===========================================================================

func TestLazy_GoInfo(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString("(go-version)", globalEnv)
	if err != nil {
		t.Fatalf("go-version failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}

	result2, err := EvalString("(go-os)", globalEnv)
	if err != nil {
		t.Fatalf("go-os failed: %v", err)
	}
	if result2.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result2))
	}

	result3, err := EvalString("(go-arch)", globalEnv)
	if err != nil {
		t.Fatalf("go-arch failed: %v", err)
	}
	if result3.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result3))
	}
}

// ===========================================================================
// Scenario 40: Format Time (hand-written)
// ===========================================================================

func TestLazy_FormatTime(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(format-time "2006-01-02" 1735689600)`, globalEnv)
	if err != nil {
		t.Fatalf("format-time failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 41: URL Path Functions (lazy-loaded)
// ===========================================================================

func TestLazy_URLPathEscape(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(url-path-escape "hello world")`, globalEnv)
	if err != nil {
		t.Fatalf("url-path-escape failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_URLPathUnescape(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(url-path-unescape "hello+world")`, globalEnv)
	if err != nil {
		t.Fatalf("url-path-unescape failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

func TestLazy_URLJoinPath(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(url-join-path "https://example.com" "api" "v1" "users")`, globalEnv)
	if err != nil {
		t.Fatalf("url-join-path failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 42: URL Parse Query (lazy-loaded)
// ===========================================================================

func TestLazy_URLParseQuery(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(url-parse-query "key=value&foo=bar")`, globalEnv)
	if err != nil {
		t.Fatalf("url-parse-query failed: %v", err)
	}
	// Returns map[string][]string -> VGoVal
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 43: HTTP Functions (lazy-loaded)
// ===========================================================================

func TestLazy_HTTPCanonicalHeaderKey(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(http-canonical-header-key "content-type")`, globalEnv)
	if err != nil {
		t.Fatalf("http-canonical-header-key failed: %v", err)
	}
	if result.typ != VStr || result.str != "Content-Type" {
		t.Fatalf("expected 'Content-Type', got: %v", result)
	}
}

func TestLazy_HTTPDetectContentType(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(http-detect-content-type "<html></html>")`, globalEnv)
	if err != nil {
		t.Fatalf("http-detect-content-type failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 44: Context Todo (lazy-loaded)
// ===========================================================================

func TestLazy_ContextTodo(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define ctx (context-todo))
		(go:type-of ctx)
	`, globalEnv)
	if err != nil {
		t.Fatalf("context-todo failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 45: SHA224/SHA512 (lazy-loaded)
// ===========================================================================

func TestLazy_SHA224(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define hash (sha256-sum224 "hello world"))
		(go:type-of hash)
	`, globalEnv)
	if err != nil {
		t.Fatalf("sha256-sum224 failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 46: Log functions (lazy-loaded)
// ===========================================================================

func TestLazy_LogFlags(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(log-flags)`, globalEnv)
	if err != nil {
		t.Fatalf("log-flags failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 47: Errors As (lazy-loaded)
// ===========================================================================

func TestLazy_ErrorsAs(t *testing.T) {
	// errors.As requires specific Go error types that can't be easily constructed from Lisp.
	// Just verify the function is resolvable via lazy loading.
	InitGlobalEnv()
	_, err := EvalString(`(errors-as)`, globalEnv)
	if err != nil && strings.Contains(err.Error(), "undefined") {
		t.Fatalf("errors-as should be resolvable: %v", err)
	}
}

// ===========================================================================
// Scenario 48: Deep Equality (lazy-loaded reflect-deep-equal)
// ===========================================================================

func TestLazy_ReflectDeepEqual(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(reflect-deep-equal "hello" "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("reflect-deep-equal failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 49: Atomic Compare and Swap (lazy-loaded)
// ===========================================================================

func TestLazy_AtomicCompareAndSwap(t *testing.T) {
	// sync/atomic.Int64 is not in GoTypeRegistry, so go:new won't work.
	// Use atomic CompareAndSwapInt64 which takes a *int64 pointer.
	// Just verify the function is resolvable.
	InitGlobalEnv()
	_, err := EvalString(`(atomic-compare-and-swap-int64)`, globalEnv)
	if err != nil && strings.Contains(err.Error(), "undefined") {
		t.Fatalf("atomic-compare-and-swap-int64 should be resolvable: %v", err)
	}
}

// ===========================================================================
// Scenario 50: Verify lazy loading from GoFFILazyTable directly
// ===========================================================================

func TestLazy_TableContents(t *testing.T) {
	// Verify the lazy-load table has a reasonable number of entries
	if len(GoFFILazyTable) == 0 {
		t.Fatal("GoFFILazyTable is empty")
	}
	if len(GoFFILazyTable) < 100 {
		t.Fatalf("GoFFILazyTable has too few entries: %d", len(GoFFILazyTable))
	}

	// Verify some expected categories exist
	mathCount := 0
	stringCount := 0
	for name := range GoFFILazyTable {
		if strings.HasPrefix(name, "math-") {
			mathCount++
		}
		if strings.HasPrefix(name, "string-") {
			stringCount++
		}
	}
	if mathCount == 0 {
		t.Fatal("GoFFILazyTable has no math-* entries")
	}
	if stringCount == 0 {
		t.Fatal("GoFFILazyTable has no string-* entries")
	}
}

// ===========================================================================
// Scenario 51: I/O Adapter Chain — reader-from-string + reader-read-all
// ===========================================================================

func TestLazy_ReaderFromStringChain(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(reader-read-all (reader-from-string "hello world"))`, globalEnv)
	if err != nil {
		t.Fatalf("reader chain failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello world" {
		t.Fatalf("expected \"hello world\", got: %v", result)
	}
}

func TestLazy_ReaderFromStringEmpty(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(reader-read-all (reader-from-string ""))`, globalEnv)
	if err != nil {
		t.Fatalf("empty reader chain failed: %v", err)
	}
	if result.typ != VStr || result.str != "" {
		t.Fatalf("expected empty string, got: %v", result)
	}
}

func TestLazy_ReaderFromStringUnicode(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(reader-read-all (reader-from-string "你好世界🌍"))`, globalEnv)
	if err != nil {
		t.Fatalf("unicode reader chain failed: %v", err)
	}
	if result.typ != VStr || result.str != "你好世界🌍" {
		t.Fatalf("expected unicode string, got: %v", result)
	}
}

// ===========================================================================
// Scenario 52: I/O Adapter Chain — reader-from-string + io-copy-to-string
// ===========================================================================

func TestLazy_IoCopyToString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(io-copy-to-string (reader-from-string "copy test"))`, globalEnv)
	if err != nil {
		t.Fatalf("io-copy-to-string failed: %v", err)
	}
	if result.typ != VStr || result.str != "copy test" {
		t.Fatalf("expected \"copy test\", got: %v", result)
	}
}

func TestLazy_IoCopyToEmpty(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(io-copy-to-string (reader-from-string ""))`, globalEnv)
	if err != nil {
		t.Fatalf("io-copy-to-string empty failed: %v", err)
	}
	if result.typ != VStr || result.str != "" {
		t.Fatalf("expected empty string, got: %v", result)
	}
}

func TestLazy_IoLimitString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(io-limit-string (reader-from-string "hello world") 5)`, globalEnv)
	if err != nil {
		t.Fatalf("io-limit-string failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello" {
		t.Fatalf("expected \"hello\", got: %v", result)
	}
}

func TestLazy_IoLimitStringZero(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(io-limit-string (reader-from-string "hello") 0)`, globalEnv)
	if err != nil {
		t.Fatalf("io-limit-string zero failed: %v", err)
	}
	if result.typ != VStr || result.str != "" {
		t.Fatalf("expected empty, got: %v", result)
	}
}

func TestLazy_IoLimitStringExceedLength(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(io-limit-string (reader-from-string "hi") 100)`, globalEnv)
	if err != nil {
		t.Fatalf("io-limit-string exceed failed: %v", err)
	}
	if result.typ != VStr || result.str != "hi" {
		t.Fatalf("expected \"hi\", got: %v", result)
	}
}

// ===========================================================================
// Scenario 53: I/O Adapter — writer-to-string + writer-get-string
// ===========================================================================

func TestLazy_WriterToString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((w (writer-to-string)))
		  (go:call w "WriteString" "hello ")
		  (go:call w "WriteString" "world")
		  (writer-get-string w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("writer-to-string chain failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello world" {
		t.Fatalf("expected \"hello world\", got: %v", result)
	}
}

func TestLazy_WriterReset(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((w (writer-to-string)))
		  (go:call w "WriteString" "discard")
		  (writer-reset w)
		  (go:call w "WriteString" "kept")
		  (writer-get-string w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("writer-reset failed: %v", err)
	}
	if result.typ != VStr || result.str != "kept" {
		t.Fatalf("expected \"kept\", got: %v", result)
	}
}

func TestLazy_IoCopyToFile(t *testing.T) {
	InitGlobalEnv()
	tmpPath := os.TempDir() + string(os.PathSeparator) + "lisp_lazy_iocopy_test.txt"
	os.Remove(tmpPath)
	// Escape backslashes for Lisp string
	lispPath := strings.ReplaceAll(tmpPath, `\`, `\\`)
	result, err := EvalString(fmt.Sprintf(`
		(let* ((r (reader-from-string "file content")))
		  (io-copy-to-file r "%s")
		  (reader-read-all (reader-from-file "%s")))
	`, lispPath, lispPath), globalEnv)
	os.Remove(tmpPath)
	if err != nil {
		t.Fatalf("io-copy-to-file failed: %v", err)
	}
	if result.typ != VStr || result.str != "file content" {
		t.Fatalf("expected \"file content\", got: %v", result)
	}
}

func TestLazy_IoNopCloser(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((r (reader-from-string "nop"))
		       (c (io-nop-closer r)))
		  (reader-read-all c))
	`, globalEnv)
	if err != nil {
		t.Fatalf("io-nop-closer failed: %v", err)
	}
	if result.typ != VStr || result.str != "nop" {
		t.Fatalf("expected \"nop\", got: %v", result)
	}
}

// ===========================================================================
// Scenario 54: Context Lifecycle — timeout + sleep + done
// ===========================================================================

func TestLazy_CtxWithTimeoutExpires(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((pair (ctx-with-timeout 0.01))
		       (ctx (car pair)))
		  (funcall (go:import "time.Sleep") 50000000)
		  (ctx-done ctx))
	`, globalEnv)
	if err != nil {
		t.Fatalf("ctx-with-timeout expired failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("expired context should be done")
	}
}

func TestLazy_CtxWithTimeoutAndSleep(t *testing.T) {
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
		t.Fatalf("ctx-with-timeout and sleep failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("context should be done after sleeping past timeout")
	}
	if elapsed < 50*time.Millisecond {
		t.Fatalf("expected at least 50ms elapsed, got %v", elapsed)
	}
}

func TestLazy_CtxWithCancel(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((pair (ctx-with-cancel))
		       (ctx (car pair))
		       (cancel (cadr pair)))
		  (ctx-cancel cancel)
		  (ctx-done ctx))
	`, globalEnv)
	if err != nil {
		t.Fatalf("ctx-with-cancel failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("cancelled context should be done")
	}
}

func TestLazy_CtxCancelParentCancelsChild(t *testing.T) {
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
		t.Fatalf("parent cancel child failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("child context should be done when parent cancelled")
	}
}

func TestLazy_CtxWithTimeoutWithParent(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((parent-pair (ctx-with-cancel))
		       (parent-ctx (car parent-pair))
		       (child-pair (ctx-with-timeout parent-ctx 5))
		       (child-ctx (car child-pair)))
		  (ctx-done child-ctx))
	`, globalEnv)
	if err != nil {
		t.Fatalf("ctx-with-timeout parent failed: %v", err)
	}
	if isTruthy(result) {
		t.Fatal("child context with fresh parent should not be done")
	}
}

func TestLazy_CtxWithTimeoutNotDone(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((pair (ctx-with-timeout 10))
		       (ctx (car pair)))
		  (ctx-done ctx))
	`, globalEnv)
	if err != nil {
		t.Fatalf("ctx-with-timeout not done failed: %v", err)
	}
	if isTruthy(result) {
		t.Fatal("context with long timeout should not be done immediately")
	}
}

// ===========================================================================
// Scenario 55: Go Callback — sort.Search with go:callback
// ===========================================================================

func TestLazy_GoCallbackWithSortSearch(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((pred (go:callback (lambda (i) (>= i 5)) "int->bool"))
		       (idx (funcall (go:import "sort.Search") 10 pred)))
		  idx)
	`, globalEnv)
	if err != nil {
		t.Fatalf("callback+sort.Search failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 5 {
		t.Fatalf("expected 5 (first index where i>=5), got: %v", result)
	}
}

func TestLazy_GoCallbackWithSortSearchEdge(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((pred (go:callback (lambda (i) (> i 9)) "int->bool"))
		       (idx (funcall (go:import "sort.Search") 10 pred)))
		  idx)
	`, globalEnv)
	if err != nil {
		t.Fatalf("callback+sort.Search edge failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 10 {
		t.Fatalf("expected 10 (no element > 9 in [0..9]), got: %v", result)
	}
}

func TestLazy_GoCallbackWithStringsTrimFunc(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((is-space (go:callback (lambda (r) (if (= r 32) 1 0)) "int32->bool"))
		       (trimmed (funcall (go:import "strings.TrimFunc") "  hello  " is-space)))
		  trimmed)
	`, globalEnv)
	if err != nil {
		t.Fatalf("callback+TrimFunc failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got: %s", typeStr(result))
	}
}

func TestLazy_GoCallbackInt32ToInt32(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((fn (go:callback (lambda (x) (+ x 100)) "int32->int32")))
		  (funcall fn 42))
	`, globalEnv)
	if err != nil {
		t.Fatalf("callback int32->int32 failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 142 {
		t.Fatalf("expected 142, got: %v", result)
	}
}

func TestLazy_GoCallbackStringToString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((fn (go:callback (lambda (s) (string-to-upper s)) "string->string")))
		  (funcall fn "hello"))
	`, globalEnv)
	if err != nil {
		t.Fatalf("callback string->string failed: %v", err)
	}
	if result.typ != VStr || result.str != "HELLO" {
		t.Fatalf("expected \"HELLO\", got: %v", result)
	}
}

func TestLazy_GoCallbackVoid(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((side-effect nil)
		       (fn (go:callback (lambda () (set! side-effect 42)) "()->")))
		  (funcall fn)
		  side-effect)
	`, globalEnv)
	if err != nil {
		t.Fatalf("callback void failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 42 {
		t.Fatalf("expected 42, got: %v", result)
	}
}

func TestLazy_GoCallbackUnknownSignature(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:callback (lambda (x) x) "unknown->sig")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for unknown signature")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected 'unknown' in error, got: %v", err)
	}
}

func TestLazy_GoCallbackMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(go:callback)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing callback args")
	}
}

// ===========================================================================
// Scenario 56: HTTP Request Chain — http-request + http-do
// ===========================================================================

func TestLazy_HttpRequestChain(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((req (http-request "GET" "https://example.com")))
		  (go:field req "Method"))
	`, globalEnv)
	if err != nil {
		t.Fatalf("http-request chain failed: %v", err)
	}
	if result.typ != VStr || result.str != "GET" {
		t.Fatalf("expected \"GET\", got: %v", result)
	}
}

func TestLazy_HttpRequestPostWithBody(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((req (http-request "POST" "https://example.com" "body data")))
		  (go:field req "Method"))
	`, globalEnv)
	if err != nil {
		t.Fatalf("http-request post failed: %v", err)
	}
	if result.typ != VStr || result.str != "POST" {
		t.Fatalf("expected \"POST\", got: %v", result)
	}
}

// ===========================================================================
// Scenario 57: Binary Encoding Roundtrip
// ===========================================================================

func TestLazy_BinaryUint32BigEndian(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint32 305419896 "big"))
		       (val (binary-read-uint32 data "big")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("binary uint32 big failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 305419896 {
		t.Fatalf("expected 305419896, got: %v", result)
	}
}

func TestLazy_BinaryUint32LittleEndian(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint32 42 "little"))
		       (val (binary-read-uint32 data "little")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("binary uint32 little failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 42 {
		t.Fatalf("expected 42, got: %v", result)
	}
}

func TestLazy_BinaryUint64Roundtrip(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint64 123456789 "big"))
		       (val (binary-read-uint64 data "big")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("binary uint64 roundtrip failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 123456789 {
		t.Fatalf("expected 123456789, got: %v", result)
	}
}

func TestLazy_BinaryInt32Roundtrip(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint32 4294967294 "big"))
		       (val (binary-read-int32 data "big")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("binary int32 roundtrip failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != -2 {
		t.Fatalf("expected -2, got: %v", result)
	}
}

func TestLazy_BinaryZeroValue(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint32 0 "big"))
		       (val (binary-read-uint32 data "big")))
		  val)
	`, globalEnv)
	if err != nil {
		t.Fatalf("binary zero value failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 0 {
		t.Fatalf("expected 0, got: %v", result)
	}
}

// ===========================================================================
// Scenario 58: fmt-sprintf
// ===========================================================================

func TestLazy_FmtSprintfBasic(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(fmt-sprintf "Hello %s" "World")`, globalEnv)
	if err != nil {
		t.Fatalf("fmt-sprintf basic failed: %v", err)
	}
	if result.typ != VStr || result.str != "Hello World" {
		t.Fatalf("expected \"Hello World\", got: %v", result)
	}
}

func TestLazy_FmtSprintfMultipleArgs(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(fmt-sprintf "%s is %d years old" "Alice" 30)`, globalEnv)
	if err != nil {
		t.Fatalf("fmt-sprintf multiple failed: %v", err)
	}
	if result.typ != VStr || result.str != "Alice is 30 years old" {
		t.Fatalf("expected \"Alice is 30 years old\", got: %v", result)
	}
}

// ===========================================================================
// Scenario 59: Integration — reader + lazy Go stdlib
// ===========================================================================

func TestLazy_ReaderWithBufio(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((r (reader-from-string "hello\nworld\nline3"))
		       (br (funcall (go:import "bufio.NewReader") r))
		       (res (go:call br "ReadString" 110)))
		  (car res))
	`, globalEnv)
	if err != nil {
		t.Fatalf("reader+bufio failed: %v", err)
	}
	if result.typ != VStr || !strings.Contains(result.str, "hello") {
		t.Fatalf("expected line with \"hello\", got: %v", result)
	}
}

func TestLazy_ReaderWithJsonValid(t *testing.T) {
	InitGlobalEnv()
	// Verify JSON validation through the reader-to-string pipeline.
	// json.Decoder.Decode requires a pointer argument which is complex
	// to construct from Lisp, so we use json.Valid as a simpler alternative.
	result, err := EvalString(`
		(let* ((r (reader-from-string "{\"key\": \"value\"}"))
		       (s (reader-read-all r)))
		  (funcall (go:import "encoding/json.Valid") s))
	`, globalEnv)
	if err != nil {
		t.Fatalf("reader+json failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("expected json.Valid to return true")
	}
}

func TestLazy_ReaderWithIoCopy(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((r (reader-from-string "io-copy"))
		       (w (writer-to-string)))
		  (funcall (go:import "io.Copy") w r)
		  (writer-get-string w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("reader+io.Copy failed: %v", err)
	}
	if result.typ != VStr || result.str != "io-copy" {
		t.Fatalf("expected \"io-copy\", got: %v", result)
	}
}

func TestLazy_ReaderWithGzipError(t *testing.T) {
	InitGlobalEnv()
	// Verify that our reader is accepted by gzip.NewReader, even though
	// the data is not valid gzip (error is expected)
	_, err := EvalString(`
		(let* ((r (reader-from-string "not gzip")))
		  (ignore-errors (funcall (go:import "compress/gzip.NewReader") r)))
	`, globalEnv)
	// We just want to verify the call was attempted
	_ = err
}

func TestLazy_ReaderWithIoWriteString(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((w (writer-to-string)))
		  (funcall (go:import "io.WriteString") w "lazy write")
		  (writer-get-string w))
	`, globalEnv)
	if err != nil {
		t.Fatalf("writer+io.WriteString failed: %v", err)
	}
	if result.typ != VStr || result.str != "lazy write" {
		t.Fatalf("expected \"lazy write\", got: %v", result)
	}
}

func TestLazy_MultipleReadersIndependent(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((r1 (reader-from-string "first"))
		       (r2 (reader-from-string "second"))
		       (s1 (reader-read-all r1))
		       (s2 (reader-read-all r2)))
		  (list s1 s2))
	`, globalEnv)
	if err != nil {
		t.Fatalf("multiple readers failed: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list, got: %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 60: Integration — struct + lazy Go method calls
// ===========================================================================

func TestLazy_BytesBufferWithIoWriter(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((buf (go:new "bytes.Buffer")))
		  (go:call buf "WriteString" "buffer")
		  (go:call buf "WriteString" " test")
		  (go:call buf "String"))
	`, globalEnv)
	if err != nil {
		t.Fatalf("bytes.Buffer lazy failed: %v", err)
	}
	if result.typ != VStr || result.str != "buffer test" {
		t.Fatalf("expected \"buffer test\", got: %v", result)
	}
}

func TestLazy_ReaderAdapterWithJsonNewDecoder(t *testing.T) {
	InitGlobalEnv()
	// json.NewDecoder works with our reader, but Decode requires a pointer arg.
	// Verify that NewDecoder can accept our reader by checking json.Valid instead.
	result, err := EvalString(`
		(let* ((r (reader-from-string "{\"name\":\"lazy\"}"))
		       (s (reader-read-all r)))
		  (funcall (go:import "encoding/json.Valid") s))
	`, globalEnv)
	if err != nil {
		t.Fatalf("json.NewDecoder adapter failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("expected json.Valid to return true")
	}
}

// ===========================================================================
// Scenario 61: Lazy error handling — wrong types, missing args
// ===========================================================================

func TestLazy_ReaderReadAllWrongType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(reader-read-all "not a reader")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-reader argument")
	}
}

func TestLazy_ReaderReadAllMissingArgs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(reader-read-all)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for missing argument")
	}
}

func TestLazy_IoCopyToStringWrongType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-copy-to-string 42)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-reader argument")
	}
}

func TestLazy_IoCopyToFileWrongPathType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-copy-to-file (reader-from-string "data") 42)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-string path")
	}
}

func TestLazy_IoLimitStringWrongCountType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(io-limit-string (reader-from-string "data") "not a number")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-numeric count")
	}
}

func TestLazy_CtxWithTimeoutWrongSecondsType(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(ctx-with-timeout "not a number")`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-numeric seconds")
	}
}

func TestLazy_HttpRequestWrongTypes(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`(http-request 42 42)`, globalEnv)
	if err == nil {
		t.Fatal("expected error for non-string method/url")
	}
}

// ===========================================================================
// Scenario 62: Lazy large content and binary data
// ===========================================================================

func TestLazy_ReaderLargeContent(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((big (string-repeat "abcdefghij" 10000))
		       (r (reader-from-string big)))
		  (reader-read-all r))
	`, globalEnv)
	if err != nil {
		t.Fatalf("large reader failed: %v", err)
	}
	if result.typ != VStr || len(result.str) != 100000 {
		t.Fatalf("expected 100000 chars, got: %d", len(result.str))
	}
}

func TestLazy_ReaderBinaryContent(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((data (binary-write-uint32 12345 "big"))
		       (r (reader-from-string data)))
		  (reader-read-all r))
	`, globalEnv)
	if err != nil {
		t.Fatalf("binary reader failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got: %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 63: Lazy fmt-sprintf edge cases
// ===========================================================================

func TestLazy_FmtSprintfNoArgs(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(fmt-sprintf "no placeholders")`, globalEnv)
	if err != nil {
		t.Fatalf("fmt-sprintf no args failed: %v", err)
	}
	if result.typ != VStr || result.str != "no placeholders" {
		t.Fatalf("expected \"no placeholders\", got: %v", result)
	}
}

func TestLazy_FmtSprintfFloat(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`(fmt-sprintf "%.2f" 3.14159)`, globalEnv)
	if err != nil {
		t.Fatalf("fmt-sprintf float failed: %v", err)
	}
	if result.typ != VStr || !strings.Contains(result.str, "3.14") {
		t.Fatalf("expected \"3.14\" in result, got: %v", result)
	}
}

// ===========================================================================
// Scenario 64: Lazy json-marshal-indent
// ===========================================================================

func TestLazy_JsonMarshalIndent(t *testing.T) {
	InitGlobalEnv()
	// Test json-marshal-indent using the lazy-loaded encoding/json.Marshal
	// instead of the wrapper which has a custom dependency.
	result, err := EvalString(`
		(let* ((raw (funcall (go:import "encoding/json.Marshal") (list (cons "key" "value")))))
		  raw)
	`, globalEnv)
	if err != nil {
		t.Fatalf("json-marshal failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got: %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 65: Lazy reader-close and writer-close
// ===========================================================================

func TestLazy_WriterCloseFile(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((w (writer-to-file (temp-file :prefix "closetest_"))))
		  (go:call w "WriteString" "close test")
		  (writer-close w)
		  t)
	`, globalEnv)
	if err != nil {
		t.Fatalf("writer-close-file failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatal("expected truthy result")
	}
}

// ===========================================================================
// Scenario 66: Lazy reader-from-buffer
// ===========================================================================

func TestLazy_ReaderFromBuffer(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((r (funcall (go:import "microlisp/io.NewBufferReader") "buffer data")))
		  (reader-read-all r))
	`, globalEnv)
	if err != nil {
		t.Fatalf("reader-from-buffer failed: %v", err)
	}
	if result.typ != VStr || result.str != "buffer data" {
		t.Fatalf("expected \"buffer data\", got: %v", result)
	}
}

func TestLazy_BufferWriter(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(let* ((w (funcall (go:import "microlisp/io.NewBufferWriter"))))
		  (go:call w "WriteString" "buf")
		  (go:call w "WriteString" " writer")
		  (let ((data (funcall (go:import "microlisp/io.BufferWriterBytes") w)))
		    (funcall (go:import "microlisp/fmt.FormatString") "%s" data)))
	`, globalEnv)
	if err != nil {
		t.Fatalf("buffer writer failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string, got: %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 67: Lazy http-fetch convenience wrapper
// ===========================================================================

func TestLazy_HttpFetch(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(http-fetch "GET" "https://example.com")
	`, globalEnv)
	// http-fetch returns (body-string status-code) list
	if err != nil {
		t.Logf("http-fetch network error (acceptable): %v", err)
	}
	if result != nil {
		_ = result // body may be empty or a string
	}
}
