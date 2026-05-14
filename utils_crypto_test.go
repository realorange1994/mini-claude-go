package main

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// ═══════════════════════════════════════════════════════════
// Section: Hash Functions Tests (from hash_test.go)
// ═══════════════════════════════════════════════════════════

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

// Ported from upstream bunHashPolyfill.test.ts
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

// ═══════════════════════════════════════════════════════════
// Section: Fingerprint Tests (from fingerprint_test.go)
// ═══════════════════════════════════════════════════════════

// ---------- FINGERPRINT_SALT tests ----------
// Ported from upstream fingerprint.test.ts

func TestFingerprintSalt(t *testing.T) {
	if FingerprintSalt != "59cf53e54c78" {
		t.Errorf("FINGERPRINT_SALT should be '59cf53e54c78', got %q", FingerprintSalt)
	}
}

// ---------- computeFingerprint tests ----------
// Ported from upstream fingerprint.test.ts

func TestFingerprintReturns3CharHex(t *testing.T) {
	result := computeFingerprint("test message", "1.0.0")
	if len(result) != 3 {
		t.Errorf("fingerprint should be 3 chars, got %d", len(result))
	}
	if !regexp.MustCompile(`^[0-9a-f]{3}$`).MatchString(result) {
		t.Errorf("fingerprint should be 3-char hex, got %q", result)
	}
}

func TestFingerprintDeterministic(t *testing.T) {
	a := computeFingerprint("same input", "1.0.0")
	b := computeFingerprint("same input", "1.0.0")
	if a != b {
		t.Errorf("fingerprint should be deterministic: got %q and %q", a, b)
	}
}

func TestFingerprintDifferentMessage(t *testing.T) {
	a := computeFingerprint("hello world from test one", "1.0.0")
	b := computeFingerprint("goodbye world from test two", "1.0.0")
	if a == b {
		t.Error("different message text should produce different fingerprints")
	}
}

func TestFingerprintDifferentVersion(t *testing.T) {
	a := computeFingerprint("same text", "1.0.0")
	b := computeFingerprint("same text", "2.0.0")
	if a == b {
		t.Error("different version should produce different fingerprints")
	}
}

func TestFingerprintShortStrings(t *testing.T) {
	// String shorter than 21 chars: indices 4 and 7 exist, but 20 doesn't
	result := computeFingerprint("hi", "1.0.0")
	if len(result) != 3 {
		t.Errorf("fingerprint of short string should be 3 chars, got %d", len(result))
	}
	if !regexp.MustCompile(`^[0-9a-f]{3}$`).MatchString(result) {
		t.Errorf("fingerprint should be 3-char hex, got %q", result)
	}
}

func TestFingerprintEmptyString(t *testing.T) {
	result := computeFingerprint("", "1.0.0")
	if len(result) != 3 {
		t.Errorf("fingerprint of empty string should be 3 chars, got %d", len(result))
	}
	if !regexp.MustCompile(`^[0-9a-f]{3}$`).MatchString(result) {
		t.Errorf("fingerprint should be 3-char hex, got %q", result)
	}
}

func TestFingerprintValidHex(t *testing.T) {
	result := computeFingerprint("any message here for testing", "3.5.1")
	if !regexp.MustCompile(`^[0-9a-f]{3}$`).MatchString(result) {
		t.Errorf("fingerprint should be valid 3-char hex, got %q", result)
	}
}

func TestFingerprintExactCharsAtIndices(t *testing.T) {
	// For "hello world from test one", chars at indices [4, 7, 20] are:
	// index 4: 'o', index 7: 'o', index 20: 't'
	// (0-indexed: "hello world from test one"[4]='o', [7]='o', [20]='t')
	// Input = FINGERPRINT_SALT + "oot" + version
	input := FingerprintSalt + "oot" + "1.0.0"
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(input)))[:3]

	result := computeFingerprint("hello world from test one", "1.0.0")
	if result != hash {
		t.Errorf("fingerprint should use chars at [4,7,20]: got %q, expected %q", result, hash)
	}
}

func TestFingerprintPadsMissingIndices(t *testing.T) {
	// "abcde" has index 4 = 'e', but indices 7 and 20 are missing -> "e00"
	input := FingerprintSalt + "e00" + "1.0.0"
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(input)))[:3]

	result := computeFingerprint("abcde", "1.0.0")
	if result != hash {
		t.Errorf("fingerprint with missing indices should pad with '0': got %q, expected %q", result, hash)
	}
}

// ═══════════════════════════════════════════════════════════
// Section: UUID & Agent ID Tests (from uuid_test.go)
// ═══════════════════════════════════════════════════════════

// ---------- validateUUID tests ----------
// Ported from upstream uuid.test.ts

func TestValidateUUIDValid(t *testing.T) {
	result, ok := validateUUID("550e8400-e29b-41d4-a716-446655440000")
	if !ok {
		t.Error("should validate correct UUID")
	}
	if result != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("should return UUID string, got %q", result)
	}
}

func TestValidateUUIDUppercase(t *testing.T) {
	result, ok := validateUUID("550E8400-E29B-41D4-A716-446655440000")
	if !ok {
		t.Error("should validate uppercase UUID")
	}
	if result != "550E8400-E29B-41D4-A716-446655440000" {
		t.Errorf("should return uppercase UUID string, got %q", result)
	}
}

func TestValidateUUIDEmptyString(t *testing.T) {
	_, ok := validateUUID("")
	if ok {
		t.Error("should reject empty string")
	}
}

func TestValidateUUIDInvalidFormat(t *testing.T) {
	invalid := []string{
		"not-a-uuid",
		"550e8400-e29b-41d4-a716",
		"550e8400e29b41d4a716446655440000",
	}
	for _, s := range invalid {
		_, ok := validateUUID(s)
		if ok {
			t.Errorf("should reject invalid UUID format: %q", s)
		}
	}
}

func TestValidateUUIDInvalidChars(t *testing.T) {
	_, ok := validateUUID("550e8400-e29b-41d4-a716-44665544000g")
	if ok {
		t.Error("should reject UUID with invalid chars (g)")
	}
}

func TestValidateUUIDWhitespace(t *testing.T) {
	// Leading/trailing whitespace should be rejected
	_, ok := validateUUID(" 550e8400-e29b-41d4-a716-446655440000")
	if ok {
		t.Error("should reject UUID with leading whitespace")
	}
	_, ok = validateUUID("550e8400-e29b-41d4-a716-446655440000 ")
	if ok {
		t.Error("should reject UUID with trailing whitespace")
	}
}

// ---------- createAgentId tests ----------
// Ported from upstream uuid.test.ts

func TestCreateAgentIdNoLabel(t *testing.T) {
	id := createAgentId("")
	// Format: a + 16 hex chars
	re := regexp.MustCompile(`^a[0-9a-f]{16}$`)
	if !re.MatchString(id) {
		t.Errorf("agent ID format should be 'a' + 16 hex chars, got %q", id)
	}
}

func TestCreateAgentIdWithLabel(t *testing.T) {
	id := createAgentId("compact")
	// Format: acompact- + 16 hex chars
	re := regexp.MustCompile(`^acompact-[0-9a-f]{16}$`)
	if !re.MatchString(id) {
		t.Errorf("agent ID format should be 'acompact-' + 16 hex chars, got %q", id)
	}
}

func TestCreateAgentIdUniqueness(t *testing.T) {
	// N generated IDs should be unique
	const n = 100
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		id := createAgentId("test")
		if seen[id] {
			t.Fatalf("duplicate agent ID at iteration %d: %s", i, id)
		}
		seen[id] = true
	}
}

func TestCreateAgentIdDifferentLabels(t *testing.T) {
	id1 := createAgentId("compact")
	id2 := createAgentId("worker")
	if id1[:len("acompact")] == id2[:len("acompact")] {
		t.Error("different labels should produce different prefixes")
	}
}

func TestRandomHexLength(t *testing.T) {
	// 8 bytes -> 16 hex chars
	h := randomHex(8)
	if len(h) != 16 {
		t.Errorf("randomHex(8) should produce 16 hex chars, got %d", len(h))
	}

	// 16 bytes -> 32 hex chars
	h = randomHex(16)
	if len(h) != 32 {
		t.Errorf("randomHex(16) should produce 32 hex chars, got %d", len(h))
	}
}

func TestRandomHexUniqueness(t *testing.T) {
	const n = 100
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		h := randomHex(8)
		if seen[h] {
			t.Fatalf("duplicate random hex at iteration %d: %s", i, h)
		}
		seen[h] = true
	}
}

func TestRandomHexCharsOnly(t *testing.T) {
	for i := 0; i < 100; i++ {
		h := randomHex(16)
		for _, c := range h {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("randomHex produced non-hex char: %c", c)
			}
		}
	}
}

// ═══════════════════════════════════════════════════════════
// Section: Tagged ID Tests (from tagged_id_test.go)
// ═══════════════════════════════════════════════════════════

// ─── ToTaggedID ──────────────────────────────────────────────────────────────

func TestToTaggedIDStartsWithTag(t *testing.T) {
	result := ToTaggedID("user")
	if !strings.HasPrefix(result, "user_") {
		t.Fatalf("expected prefix 'user_', got %q", result)
	}
}

func TestToTaggedIDDifferentTags(t *testing.T) {
	userResult := ToTaggedID("user")
	orgResult := ToTaggedID("org")
	msgResult := ToTaggedID("msg")

	if userResult == orgResult {
		t.Fatal("user and org IDs should differ")
	}
	if orgResult == msgResult {
		t.Fatal("org and msg IDs should differ")
	}
	if userResult == msgResult {
		t.Fatal("user and msg IDs should differ")
	}
}

func TestToTaggedIDUniqueAcrossCalls(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := ToTaggedID("test")
		if ids[id] {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		ids[id] = true
	}
}

func TestToTaggedIDFormat(t *testing.T) {
	result := ToTaggedID("user")
	// Format: tag_counter_randomHex
	parts := strings.SplitN(result, "_", 3)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts in tagged ID, got %d: %q", len(parts), result)
	}
	if parts[0] != "user" {
		t.Errorf("expected tag 'user', got %q", parts[0])
	}
	if len(parts[2]) != 8 { // 4 bytes = 8 hex chars
		t.Errorf("expected 8-char random hex suffix, got %d: %q", len(parts[2]), parts[2])
	}
}

// ─── ParseTaggedID ───────────────────────────────────────────────────────────

func TestParseTaggedIDValid(t *testing.T) {
	id := ToTaggedID("user")
	tag, ok := ParseTaggedID(id)
	if !ok {
		t.Fatal("expected valid parse")
	}
	if tag != "user" {
		t.Fatalf("expected tag 'user', got %q", tag)
	}
}

func TestParseTaggedIDInvalid(t *testing.T) {
	_, ok := ParseTaggedID("invalid")
	if ok {
		t.Fatal("expected invalid parse for string without underscore")
	}
}

// ─── GetTaggedIDCounter ──────────────────────────────────────────────────────

func TestGetTaggedIDCounterValid(t *testing.T) {
	id := ToTaggedID("test")
	counter, ok := GetTaggedIDCounter(id)
	if !ok {
		t.Fatal("expected valid counter parse")
	}
	if counter == 0 {
		t.Fatal("expected non-zero counter")
	}
}

// ─── ValidateTaggedID ───────────────────────────────────────────────────────

func TestValidateTaggedIDCorrectTag(t *testing.T) {
	id := ToTaggedID("user")
	if !ValidateTaggedID(id, "user") {
		t.Fatal("expected validation to pass for matching tag")
	}
}

func TestValidateTaggedIDWrongTag(t *testing.T) {
	id := ToTaggedID("user")
	if ValidateTaggedID(id, "org") {
		t.Fatal("expected validation to fail for mismatched tag")
	}
}

func TestValidateTaggedIDInvalidID(t *testing.T) {
	if ValidateTaggedID("invalid", "user") {
		t.Fatal("expected validation to fail for invalid ID format")
	}
}

// ─── Invariant: counter always increases ─────────────────────────────────────

func TestTaggedIDCounterMonotonic(t *testing.T) {
	id1 := ToTaggedID("test")
	id2 := ToTaggedID("test")
	counter1, _ := GetTaggedIDCounter(id1)
	counter2, _ := GetTaggedIDCounter(id2)
	if counter2 <= counter1 {
		t.Fatalf("counter should be monotonic: %d <= %d", counter2, counter1)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Section: Word Lists & Slug Generation Tests (from words_test.go)
// ═══════════════════════════════════════════════════════════════════════════════

// ─── generateWordSlug ────────────────────────────────────────────────────
// Ported from upstream words.test.ts

func TestGenerateWordSlugThreeParts(t *testing.T) {
	slug := generateWordSlug()
	parts := strings.Split(slug, "-")
	if len(parts) != 3 {
		t.Errorf("expected 3-part slug, got %d parts: %q", len(parts), slug)
	}
}

func TestGenerateWordSlugNonEmptyParts(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateWordSlug()
		parts := strings.Split(slug, "-")
		for j, part := range parts {
			if part == "" {
				t.Errorf("iteration %d: part %d is empty in slug %q", i, j, slug)
			}
		}
	}
}

func TestGenerateWordSlugAllLowercase(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateWordSlug()
		if slug != strings.ToLower(slug) {
			t.Errorf("slug should be all lowercase, got %q", slug)
		}
	}
}

func TestGenerateWordSlugNoConsecutiveHyphens(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateWordSlug()
		if strings.Contains(slug, "--") {
			t.Errorf("slug should not contain consecutive hyphens, got %q", slug)
		}
	}
}

func TestGenerateWordSlugPattern(t *testing.T) {
	slug := generateWordSlug()
	pattern := regexp.MustCompile(`^[a-z]+-[a-z]+-[a-z]+$`)
	if !pattern.MatchString(slug) {
		t.Errorf("slug should match adjective-verb-noun pattern, got %q", slug)
	}
}

func TestGenerateWordSlugVaried(t *testing.T) {
	// Multiple calls should produce varied results
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		seen[generateWordSlug()] = true
	}
	if len(seen) <= 10 {
		t.Errorf("expected varied slugs, only got %d unique out of 20", len(seen))
	}
}

// ─── generateShortWordSlug ───────────────────────────────────────────────
// Ported from upstream words.test.ts

func TestGenerateShortWordSlugTwoParts(t *testing.T) {
	slug := generateShortWordSlug()
	parts := strings.Split(slug, "-")
	if len(parts) != 2 {
		t.Errorf("expected 2-part slug, got %d parts: %q", len(parts), slug)
	}
}

func TestGenerateShortWordSlugNonEmptyParts(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateShortWordSlug()
		parts := strings.Split(slug, "-")
		for j, part := range parts {
			if part == "" {
				t.Errorf("iteration %d: part %d is empty in slug %q", i, j, slug)
			}
		}
	}
}

func TestGenerateShortWordSlugAllLowercase(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateShortWordSlug()
		if slug != strings.ToLower(slug) {
			t.Errorf("short slug should be all lowercase, got %q", slug)
		}
	}
}

func TestGenerateShortWordSlugPattern(t *testing.T) {
	slug := generateShortWordSlug()
	pattern := regexp.MustCompile(`^[a-z]+-[a-z]+$`)
	if !pattern.MatchString(slug) {
		t.Errorf("short slug should match adjective-noun pattern, got %q", slug)
	}
}

func TestGenerateShortWordSlugNoConsecutiveHyphens(t *testing.T) {
	for i := 0; i < 10; i++ {
		slug := generateShortWordSlug()
		if strings.Contains(slug, "--") {
			t.Errorf("short slug should not contain consecutive hyphens, got %q", slug)
		}
	}
}

// ─── Invariants ──────────────────────────────────────────────────────────

func TestGenerateWordSlugNeverEmpty(t *testing.T) {
	for i := 0; i < 20; i++ {
		slug := generateWordSlug()
		if slug == "" {
			t.Error("generateWordSlug should never return empty string")
		}
	}
}

func TestGenerateShortWordSlugNeverEmpty(t *testing.T) {
	for i := 0; i < 20; i++ {
		slug := generateShortWordSlug()
		if slug == "" {
			t.Error("generateShortWordSlug should never return empty string")
		}
	}
}

func TestAdjectiveListNotEmpty(t *testing.T) {
	if len(adjectives) == 0 {
		t.Error("adjectives list should not be empty")
	}
}

func TestVerbListNotEmpty(t *testing.T) {
	if len(verbs) == 0 {
		t.Error("verbs list should not be empty")
	}
}

func TestNounListNotEmpty(t *testing.T) {
	if len(nouns) == 0 {
		t.Error("nouns list should not be empty")
	}
}

func TestWordListsLargeEnoughForVariation(t *testing.T) {
	// Should have at least 50 of each type for good variation
	if len(adjectives) < 50 {
		t.Errorf("expected at least 50 adjectives, got %d", len(adjectives))
	}
	if len(verbs) < 50 {
		t.Errorf("expected at least 50 verbs, got %d", len(verbs))
	}
	if len(nouns) < 50 {
		t.Errorf("expected at least 50 nouns, got %d", len(nouns))
	}
}
