package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

var taggedIDCounter uint64

// ToTaggedID creates a tagged ID string of the form "tag_counter_randomHex".
// Upstream: toTaggedId() in taggedId.ts
func ToTaggedID(tag string) string {
	counter := atomic.AddUint64(&taggedIDCounter, 1)
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomHex := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("%s_%d_%s", tag, counter, randomHex)
}

// ParseTaggedID extracts the tag portion from a tagged ID.
// Returns the tag and true if valid, or empty string and false if invalid.
// Upstream: parsing logic from taggedId.ts
func ParseTaggedID(id string) (string, bool) {
	parts := strings.SplitN(id, "_", 3)
	if len(parts) < 2 {
		return "", false
	}
	return parts[0], true
}

// GetTaggedIDCounter extracts the counter portion from a tagged ID.
// Upstream: parsing logic from taggedId.ts
func GetTaggedIDCounter(id string) (uint64, bool) {
	parts := strings.SplitN(id, "_", 3)
	if len(parts) < 2 {
		return 0, false
	}
	counter, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return counter, true
}

// ValidateTaggedID checks if a string is a valid tagged ID with the given tag.
// Upstream: validation patterns from taggedId.ts
func ValidateTaggedID(id string, expectedTag string) bool {
	tag, ok := ParseTaggedID(id)
	if !ok {
		return false
	}
	return tag == expectedTag
}
