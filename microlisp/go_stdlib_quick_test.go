package microlisp

import (
	"strings"
	"testing"
)

func TestGoStdlibStringWrappers(t *testing.T) {
	InitGlobalEnv()

	// Test string-contains
	result, err := EvalString(`(string-contains "hello world" "world")`, globalEnv)
	if err != nil {
		t.Fatalf("string-contains failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatalf("expected t from string-contains, got: %v", result)
	}

	// Test string-split
	result, err = EvalString(`(string-split "a,b,c" ",")`, globalEnv)
	if err != nil {
		t.Fatalf("string-split failed: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list from string-split, got: %s", typeStr(result))
	}

	// Test string-join
	result, err = EvalString(`(string-join '("a" "b" "c") ",")`, globalEnv)
	if err != nil {
		t.Fatalf("string-join failed: %v", err)
	}
	if result.typ != VStr || result.str != "a,b,c" {
		t.Fatalf("expected 'a,b,c', got: %v", result)
	}

	// Test string-to-upper
	result, err = EvalString(`(string-to-upper "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("string-to-upper failed: %v", err)
	}
	if result.typ != VStr || result.str != "HELLO" {
		t.Fatalf("expected 'HELLO', got: %v", result)
	}

	// Test string-trim
	result, err = EvalString(`(string-trim "  hello  ")`, globalEnv)
	if err != nil {
		t.Fatalf("string-trim failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello" {
		t.Fatalf("expected 'hello', got: %v", result)
	}

	// Test string-replace
	result, err = EvalString(`(string-replace "hello" "l" "r")`, globalEnv)
	if err != nil {
		t.Fatalf("string-replace failed: %v", err)
	}
	if result.typ != VStr || result.str != "herro" {
		t.Fatalf("expected 'herro', got: %v", result)
	}

	// Test string-to-lower
	result, err = EvalString(`(string-to-lower "HELLO")`, globalEnv)
	if err != nil {
		t.Fatalf("string-to-lower failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello" {
		t.Fatalf("expected 'hello', got: %v", result)
	}

	// Test string-repeat
	result, err = EvalString(`(string-repeat "ab" 3)`, globalEnv)
	if err != nil {
		t.Fatalf("string-repeat failed: %v", err)
	}
	if result.typ != VStr || result.str != "ababab" {
		t.Fatalf("expected 'ababab', got: %v", result)
	}

	// Test string-starts-with
	result, err = EvalString(`(string-starts-with "hello" "he")`, globalEnv)
	if err != nil {
		t.Fatalf("string-starts-with failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatalf("expected t from string-starts-with, got: %v", result)
	}

	// Test string-ends-with
	result, err = EvalString(`(string-ends-with "hello" "llo")`, globalEnv)
	if err != nil {
		t.Fatalf("string-ends-with failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatalf("expected t from string-ends-with, got: %v", result)
	}
}

func TestGoStdlibPathWrappers(t *testing.T) {
	InitGlobalEnv()

	// Test path-base
	result, err := EvalString(`(path-base "/foo/bar/baz.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("path-base failed: %v", err)
	}
	if result.typ != VStr || result.str != "baz.txt" {
		t.Fatalf("expected 'baz.txt', got: %v", result)
	}

	// Test path-ext
	result, err = EvalString(`(path-ext "foo.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("path-ext failed: %v", err)
	}
	if result.typ != VStr || result.str != ".txt" {
		t.Fatalf("expected '.txt', got: %v", result)
	}

	// Test path-dir (platform-dependent separators)
	result, err = EvalString(`(path-dir "/foo/bar/baz.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("path-dir failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from path-dir, got: %s", typeStr(result))
	}
	// filepath.Dir returns platform-specific separators
	if !strings.Contains(result.str, "foo") {
		t.Fatalf("expected path containing 'foo', got: %v", result)
	}

	// Test path-join
	result, err = EvalString(`(path-join "foo" "bar" "baz.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("path-join failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from path-join, got: %s", typeStr(result))
	}
	if !strings.Contains(result.str, "foo") || !strings.Contains(result.str, "baz.txt") {
		t.Fatalf("expected joined path, got: %v", result)
	}

	// Test path-clean
	result, err = EvalString(`(path-clean "foo/../bar")`, globalEnv)
	if err != nil {
		t.Fatalf("path-clean failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from path-clean, got: %s", typeStr(result))
	}

	// Test path-is-absolute (platform-dependent)
	result, err = EvalString(`(path-is-absolute "C:\\foo")`, globalEnv)
	if err != nil {
		t.Fatalf("path-is-absolute failed: %v", err)
	}
	// On Windows, C:\foo is absolute; on Unix it would be relative
	// Just verify it returns a boolean without error
	if result.typ != VBool {
		t.Fatalf("expected boolean from path-is-absolute, got: %s", typeStr(result))
	}
}

func TestGoStdlibEnvWrappers(t *testing.T) {
	InitGlobalEnv()

	// Test setenv/getenv roundtrip
	_, err := EvalString(`(setenv "LISP_TEST_VAR" "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("setenv failed: %v", err)
	}
	result, err := EvalString(`(getenv "LISP_TEST_VAR")`, globalEnv)
	if err != nil {
		t.Fatalf("getenv failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello" {
		t.Fatalf("expected 'hello', got: %v", result)
	}

	// Test unsetenv
	_, err = EvalString(`(unsetenv "LISP_TEST_VAR")`, globalEnv)
	if err != nil {
		t.Fatalf("unsetenv failed: %v", err)
	}

	// Test current-dir
	result, err = EvalString(`(current-dir)`, globalEnv)
	if err != nil {
		t.Fatalf("current-dir failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from current-dir, got: %s", typeStr(result))
	}
}

func TestGoStdlibRegexWrappers(t *testing.T) {
	InitGlobalEnv()

	// Test regex-match
	result, err := EvalString(`(regex-match "go+" "gooo")`, globalEnv)
	if err != nil {
		t.Fatalf("regex-match failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatalf("expected t from regex-match, got: %v", result)
	}

	// Test regex-replace
	result, err = EvalString(`(regex-replace "go+" "gooo dog" "cat")`, globalEnv)
	if err != nil {
		t.Fatalf("regex-replace failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from regex-replace, got: %s", typeStr(result))
	}

	// Test regex-split
	result, err = EvalString(`(regex-split "," "a,b,c")`, globalEnv)
	if err != nil {
		t.Fatalf("regex-split failed: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list from regex-split, got: %s", typeStr(result))
	}

	// Test regex-find-all
	result, err = EvalString(`(regex-find-all "\\d+" "abc123def456")`, globalEnv)
	if err != nil {
		t.Fatalf("regex-find-all failed: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list from regex-find-all, got: %s", typeStr(result))
	}
}

func TestGoStdlibMiscWrappers(t *testing.T) {
	InitGlobalEnv()

	// Test pid
	result, err := EvalString(`(pid)`, globalEnv)
	if err != nil {
		t.Fatalf("pid failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected number from pid, got: %s", typeStr(result))
	}

	// Test go-version
	result, err = EvalString(`(go-version)`, globalEnv)
	if err != nil {
		t.Fatalf("go-version failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from go-version, got: %s", typeStr(result))
	}

	// Test num-cpus
	result, err = EvalString(`(num-cpus)`, globalEnv)
	if err != nil {
		t.Fatalf("num-cpus failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected number from num-cpus, got: %s", typeStr(result))
	}

	// Test go-os
	result, err = EvalString(`(go-os)`, globalEnv)
	if err != nil {
		t.Fatalf("go-os failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from go-os, got: %s", typeStr(result))
	}

	// Test hostname
	result, err = EvalString(`(hostname)`, globalEnv)
	if err != nil {
		t.Fatalf("hostname failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from hostname, got: %s", typeStr(result))
	}
}

func TestGoStdlibEncodingWrappers(t *testing.T) {
	InitGlobalEnv()

	// Test base64 encode/decode roundtrip
	result, err := EvalString(`(base64-encode "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("base64-encode failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from base64-encode, got: %s", typeStr(result))
	}

	result2, err := EvalString(`(base64-decode (base64-encode "hello world"))`, globalEnv)
	if err != nil {
		t.Fatalf("base64-decode failed: %v", err)
	}
	if result2.typ != VStr {
		t.Fatalf("expected string from base64-decode, got: %s", typeStr(result2))
	}

	// Test url-encode/url-decode roundtrip
	result, err = EvalString(`(url-encode "hello world")`, globalEnv)
	if err != nil {
		t.Fatalf("url-encode failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from url-encode, got: %s", typeStr(result))
	}

	result2, err = EvalString(`(url-decode (url-encode "hello world"))`, globalEnv)
	if err != nil {
		t.Fatalf("url-decode failed: %v", err)
	}
	if result2.typ != VStr || result2.str != "hello world" {
		t.Fatalf("expected 'hello world' from url-decode, got: %v", result2)
	}
}

func TestGoStdlibCryptoWrappers(t *testing.T) {
	InitGlobalEnv()

	// Test md5
	result, err := EvalString(`(md5 "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("md5 failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from md5, got: %s", typeStr(result))
	}

	// Test sha256
	result, err = EvalString(`(sha256 "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("sha256 failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from sha256, got: %s", typeStr(result))
	}
}

func TestGoStdlibTimeWrappers(t *testing.T) {
	InitGlobalEnv()

	// Test current-timestamp
	result, err := EvalString(`(current-timestamp)`, globalEnv)
	if err != nil {
		t.Fatalf("current-timestamp failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from current-timestamp, got: %s", typeStr(result))
	}

	// Test format-time
	result, err = EvalString(`(format-time "2006-01-02")`, globalEnv)
	if err != nil {
		t.Fatalf("format-time failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from format-time, got: %s", typeStr(result))
	}
}

func TestGoStdlibFileIOWrappers(t *testing.T) {
	InitGlobalEnv()

	// Test file-exists-p on a file that should exist
	result, err := EvalString(`(file-exists-p "go_stdlib_wrapper.go")`, globalEnv)
	if err != nil {
		t.Fatalf("file-exists-p failed: %v", err)
	}
	// Result should be truthy or nil — just verify no error
	t.Logf("file-exists-p result: %v type=%s", result, typeStr(result))

	// Test write-file / read-file roundtrip
	_, err = EvalString(`(write-file "/tmp/lisp_test_io.txt" "hello lisp")`, globalEnv)
	if err != nil {
		t.Fatalf("write-file failed: %v", err)
	}
	result, err = EvalString(`(read-file "/tmp/lisp_test_io.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("read-file failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello lisp" {
		t.Fatalf("expected 'hello lisp', got: %v", result)
	}

	// Test delete-file
	_, err = EvalString(`(delete-file "/tmp/lisp_test_io.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("delete-file failed: %v", err)
	}

	// Verify file is gone
	result, err = EvalString(`(file-exists-p "/tmp/lisp_test_io.txt")`, globalEnv)
	if err != nil {
		t.Fatalf("file-exists-p after delete failed: %v", err)
	}
	if isTruthy(result) {
		t.Fatalf("expected file to be deleted, but file-exists-p returned t")
	}
}

func TestGoStdlibJSONWrappers(t *testing.T) {
	InitGlobalEnv()

	// Test json-encode of simple types
	result, err := EvalString(`(json-encode nil)`, globalEnv)
	if err != nil {
		t.Fatalf("json-encode nil failed: %v", err)
	}
	if result.typ != VStr || result.str != "null" {
		t.Fatalf("expected 'null', got: %v", result)
	}

	result, err = EvalString(`(json-encode t)`, globalEnv)
	if err != nil {
		t.Fatalf("json-encode t failed: %v", err)
	}
	if result.typ != VStr || result.str != "true" {
		t.Fatalf("expected 'true', got: %v", result)
	}

	result, err = EvalString(`(json-encode 42)`, globalEnv)
	if err != nil {
		t.Fatalf("json-encode 42 failed: %v", err)
	}
	if result.typ != VStr || result.str != "42" {
		t.Fatalf("expected '42', got: %v", result)
	}

	result, err = EvalString(`(json-encode "hello")`, globalEnv)
	if err != nil {
		t.Fatalf("json-encode string failed: %v", err)
	}
	if result.typ != VStr || result.str != `"hello"` {
		t.Fatalf("expected '\"hello\"', got: %v", result)
	}

	// Test json-encode of list
	result, err = EvalString(`(json-encode '(1 2 3))`, globalEnv)
	if err != nil {
		t.Fatalf("json-encode list failed: %v", err)
	}
	if result.typ != VStr || result.str != "[1,2,3]" {
		t.Fatalf("expected '[1,2,3]', got: %v", result)
	}

	// Test json-valid-p
	result, err = EvalString(`(json-valid-p "{\"key\":1}")`, globalEnv)
	if err != nil {
		t.Fatalf("json-valid-p failed: %v", err)
	}
	if !isTruthy(result) {
		t.Fatalf("expected t from json-valid-p, got: %v", result)
	}
}
