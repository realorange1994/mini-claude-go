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