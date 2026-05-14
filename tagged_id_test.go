package main

import (
	"strings"
	"testing"
)

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
