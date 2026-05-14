package main

import (
	"regexp"
	"testing"
)

// ---------- djb2Hash tests ----------
// Ported from upstream hash.test.ts

func TestDjb2HashReturnsNonZero(t *testing.T) {
	result := djb2Hash("hello")
	if result == 0 {
		t.Error("djb2Hash('hello') should not return 0")
	}
}

func TestDjb2HashEmptyString(t *testing.T) {
	result := djb2Hash("")
	if result != 0 {
		t.Errorf("djb2Hash('') should return 0, got %d", result)
	}
}

func TestDjb2HashDeterministic(t *testing.T) {
	a := djb2Hash("test")
	b := djb2Hash("test")
	if a != b {
		t.Errorf("djb2Hash should be deterministic: got %d and %d for 'test'", a, b)
	}
}

func TestDjb2HashDifferentStrings(t *testing.T) {
	a := djb2Hash("abc")
	b := djb2Hash("def")
	if a == b {
		t.Error("djb2Hash('abc') and djb2Hash('def') should produce different hashes")
	}
}

func TestDjb2HashIsInt32(t *testing.T) {
	result := djb2Hash("some long string to hash")
	// Go int32 is always in valid range by definition
	if result > 2147483647 || result < -2147483648 {
		t.Errorf("djb2Hash result out of int32 range: %d", result)
	}
}

func TestDjb2HashKnownAnswer(t *testing.T) {
	// Upstream TS test: djb2Hash("hello") === 99162322
	// The TS implementation uses charCodeAt (UTF-16), Go uses byte values.
	// For ASCII-only strings, the algorithm is identical.
	result := djb2Hash("hello")
	if result != 99162322 {
		t.Errorf("djb2Hash('hello') = %d, want 99162322", result)
	}
}

// ---------- hashContent tests ----------

func TestHashContentReturnsString(t *testing.T) {
	result := hashContent("hello")
	if result == "" {
		t.Error("hashContent('hello') should return non-empty string")
	}
}

func TestHashContentDeterministic(t *testing.T) {
	a := hashContent("test")
	b := hashContent("test")
	if a != b {
		t.Errorf("hashContent should be deterministic: got %q and %q", a, b)
	}
}

func TestHashContentDifferentStrings(t *testing.T) {
	a := hashContent("abc")
	b := hashContent("def")
	if a == b {
		t.Error("hashContent('abc') and hashContent('def') should produce different hashes")
	}
}

func TestHashContentEmptyString(t *testing.T) {
	result := hashContent("")
	// SHA-256 of empty string is a known 64-char hex string
	if !regexp.MustCompile(`^[0-9a-f]+$`).MatchString(result) {
		t.Errorf("hashContent('') should return numeric hex string, got %q", result)
	}
}

func TestHashContentNumericFormat(t *testing.T) {
	result := hashContent("hello")
	if !regexp.MustCompile(`^[0-9a-f]+$`).MatchString(result) {
		t.Errorf("hashContent('hello') should return hex string, got %q", result)
	}
}

func TestHashContentLength(t *testing.T) {
	result := hashContent("hello")
	// SHA-256 produces 32 bytes = 64 hex chars
	if len(result) != 64 {
		t.Errorf("hashContent should produce 64-char hex string, got %d chars", len(result))
	}
}

// ---------- hashPair tests ----------

func TestHashPairReturnsString(t *testing.T) {
	result := hashPair("a", "b")
	if result == "" {
		t.Error("hashPair('a','b') should return non-empty string")
	}
}

func TestHashPairDeterministic(t *testing.T) {
	a := hashPair("a", "b")
	b := hashPair("a", "b")
	if a != b {
		t.Errorf("hashPair should be deterministic: got %q and %q", a, b)
	}
}

func TestHashPairOrderMatters(t *testing.T) {
	a := hashPair("a", "b")
	b := hashPair("b", "a")
	if a == b {
		t.Error("hashPair('a','b') and hashPair('b','a') should produce different hashes")
	}
}

func TestHashPairDisambiguatesDifferentSplits(t *testing.T) {
	// Upstream invariant: ("ts","code") != ("tsc","ode")
	a := hashPair("ts", "code")
	b := hashPair("tsc", "ode")
	if a == b {
		t.Error("hashPair('ts','code') and hashPair('tsc','ode') should produce different hashes")
	}
}

func TestHashPairEmptyStrings(t *testing.T) {
	// All empty combinations should produce valid hex strings
	emptyEmpty := hashPair("", "")
	emptyA := hashPair("", "a")
	aEmpty := hashPair("a", "")

	for _, result := range []string{emptyEmpty, emptyA, aEmpty} {
		if !regexp.MustCompile(`^[0-9a-f]+$`).MatchString(result) {
			t.Errorf("hashPair with empty strings should return hex string, got %q", result)
		}
	}

	// hashPair("", "a") != hashPair("a", "") -- order matters even with empty strings
	if emptyA == aEmpty {
		t.Error("hashPair('', 'a') and hashPair('a', '') should differ")
	}
}

func TestHashPairLength(t *testing.T) {
	result := hashPair("a", "b")
	// SHA-256 produces 64 hex chars
	if len(result) != 64 {
		t.Errorf("hashPair should produce 64-char hex string, got %d chars", len(result))
	}
}

// ─── Ported from upstream bunHashPolyfill.test.ts ─────────────────────────
// The upstream tests a 32-bit FNV-1a polyfill. Go uses djb2Hash (32-bit)
// and fnvHash (64-bit). These tests port upstream invariant patterns.

// Upstream: various input types produce valid hashes (unicode, null bytes)
func TestDjb2HashUnicodeInputs(t *testing.T) {
	inputs := []string{"", "a", "hello world", "unicode: 你好", "\x00\xff"}
	for _, input := range inputs {
		h := djb2Hash(input)
		// Go int32 is always in valid range, just verify it's computed
		_ = h // hash of any string should not crash
	}
}

// Upstream: different seeds produce different hashes (Go djb2Hash has no seed,
// but we can verify fnvHash produces different results for different inputs)
func TestFnvHashCollisionResistance(t *testing.T) {
	var seen = make(map[uint64]string)
	inputs := []string{
		"hello", "world", "test", "abc", "def", "123", "",
		"a", "b", "c", "long string with many characters",
		"unicode: 你好世界", "path/to/file.go", "another/path",
	}
	for _, input := range inputs {
		h := fnvHash(input)
		if existing, ok := seen[h]; ok && existing != input {
			t.Errorf("fnv hash collision: %q and %q both hash to %d", existing, input, h)
		}
		seen[h] = input
	}
}

// Upstream: idempotency — same input always produces same output
func TestHashContentIdempotent(t *testing.T) {
	const iterations = 100
	first := hashContent("idempotency test")
	for i := 0; i < iterations; i++ {
		h := hashContent("idempotency test")
		if h != first {
			t.Errorf("hashContent not idempotent at iteration %d", i)
			break
		}
	}
}

// Upstream: fnvHash always produces a uint64 in valid range
func TestFnvHashUint64Range(t *testing.T) {
	inputs := []string{"", "a", "hello world", "unicode: 你好", "\x00\xff", "long input string"}
	for _, input := range inputs {
		h := fnvHash(input)
		// uint64 is always >= 0 in Go, just verify it doesn't crash
		if h == 0 && input != "" {
			t.Errorf("fnvHash(%q) = 0 unexpectedly", input)
		}
	}
}

// Upstream: hashContent of empty string produces known SHA-256 value
func TestHashContentEmptyStringKnownAnswer(t *testing.T) {
	result := hashContent("")
	// SHA-256 of empty string is well-known: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if result != expected {
		t.Errorf("hashContent('') = %q, want %q", result, expected)
	}
}

// Upstream: hashPair produces same result as sha256(a + null + b)
func TestHashPairEqualsSha256WithNullSeparator(t *testing.T) {
	// hashPair("test","content") computes sha256("test\x00content")
	hp := hashPair("test", "content")
	// Same underlying bytes should produce same hash
	hc := hashContent("test\x00content")
	if hp != hc {
		t.Errorf("hashPair('test','content') should equal hashContent('test\\x00content'): got %q vs %q", hp, hc)
	}
}