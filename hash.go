package main

import (
	"crypto/sha256"
	"fmt"
)

// djb2Hash computes a DJB2 hash of a string, returning a signed 32-bit integer.
// Deterministic across runtimes. Ported from upstream TypeScript hash.ts.
func djb2Hash(str string) int32 {
	var hash int32 = 0
	for i := 0; i < len(str); i++ {
		hash = (hash<<5 - hash) + int32(str[i])
	}
	return hash
}

// hashContent hashes arbitrary content for change detection using SHA-256.
// Returns a hex string. Ported from upstream hashContent().
func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

// hashPair hashes two strings disambiguating ("ts","code") vs ("tsc","ode").
// Uses a null separator to ensure different splits produce different hashes.
// Ported from upstream hashPair().
func hashPair(a string, b string) string {
	h := sha256.New()
	h.Write([]byte(a))
	h.Write([]byte{0}) // null separator
	h.Write([]byte(b))
	return fmt.Sprintf("%x", h.Sum(nil))
}
