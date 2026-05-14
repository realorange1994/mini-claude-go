package main

import (
	"crypto/rand"
	"fmt"
	"regexp"
)

// uuidRegex matches standard UUID format: 8-4-4-4-12 hex digits.
var uuidRegex = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// validateUUID checks if a string is a valid UUID and returns the string if valid,
// or an empty string if not. Ported from upstream TypeScript validateUuid().
func validateUUID(maybeUUID string) (string, bool) {
	if maybeUUID == "" {
		return "", false
	}
	if uuidRegex.MatchString(maybeUUID) {
		return maybeUUID, true
	}
	return "", false
}

// createAgentId generates a new agent ID with prefix for consistency with task IDs.
// Format: a{label-}{16 hex chars}
// Example: acompact-a3f2c1b4d5e6f7a8, aa3f2c1b4d5e6f7a8
// Ported from upstream TypeScript createAgentId().
func createAgentId(label string) string {
	suffix := randomHex(8) // 8 bytes = 16 hex chars
	if label != "" {
		return fmt.Sprintf("a%s-%s", label, suffix)
	}
	return fmt.Sprintf("a%s", suffix)
}

// randomHex generates a random hex string of the given byte length.
func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%0*x", n*2, b)
}
