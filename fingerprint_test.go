package main

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"testing"
)

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
