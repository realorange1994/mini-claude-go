package main

import (
	"crypto/sha256"
	"fmt"
)

// FingerprintSalt is the hardcoded salt for fingerprint validation.
// Must match the backend's expected value.
const FingerprintSalt = "59cf53e54c78"

// computeFingerprint computes a 3-character fingerprint for Claude Code attribution.
// Algorithm: SHA256(SALT + msg[4] + msg[7] + msg[20] + version)[:3]
// IMPORTANT: Do not change this without careful coordination with API providers.
// Ported from upstream TypeScript fingerprint.ts.
func computeFingerprint(messageText string, version string) string {
	// Extract chars at indices [4, 7, 20], use "0" if index not found
	indices := []int{4, 7, 20}
	chars := ""
	for _, i := range indices {
		if i < len(messageText) {
			chars += string(messageText[i])
		} else {
			chars += "0"
		}
	}

	input := FingerprintSalt + chars + version

	// SHA256 hash, return first 3 hex chars
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)[:3]
}
