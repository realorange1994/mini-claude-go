package main

import (
	"regexp"
	"testing"
)

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
