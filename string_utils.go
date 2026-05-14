package main

import (
	"fmt"
	"strings"
	"unicode"
)

// escapeRegExp escapes special regex characters in a string so it can be
// used as a literal pattern in a regex constructor.
// Ported from upstream stringUtils.ts escapeRegExp.
func escapeRegExp(s string) string {
	// Characters that need escaping in regex: ^ $ { } ( ) | [ ] \ . * + ?
	var result strings.Builder
	result.Grow(len(s) + strings.Count(s, "\\")*2)
	for _, ch := range s {
		switch ch {
		case '^', '$', '{', '}', '(', ')', '|', '[', ']', '\\', '.', '*', '+', '?':
			result.WriteRune('\\')
		}
		result.WriteRune(ch)
	}
	return result.String()
}

// capitalize uppercases the first character of a string, leaving the rest
// unchanged. Unlike lodash capitalize, this does NOT lowercase the rest.
// Ported from upstream stringUtils.ts capitalize.
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes[0]) + string(runes[1:])
}

// plural returns the singular or plural form of a word based on count.
// Ported from upstream stringUtils.ts plural.
func plural(n int, word string, pluralWord ...string) string {
	pw := word + "s"
	if len(pluralWord) > 0 {
		pw = pluralWord[0]
	}
	if n == 1 {
		return word
	}
	return pw
}

// firstLineOf returns the first line of a string without allocating a split array.
// Ported from upstream stringUtils.ts firstLineOf.
func firstLineOf(s string) string {
	nl := strings.IndexByte(s, '\n')
	if nl == -1 {
		return s
	}
	return s[:nl]
}

// countCharInString counts occurrences of a character in a string.
// Ported from upstream stringUtils.ts countCharInString.
func countCharInString(s string, char string, start ...int) int {
	offset := 0
	if len(start) > 0 {
		offset = start[0]
	}
	if offset >= len(s) {
		return 0
	}
	count := 0
	pos := offset
	for {
		idx := strings.Index(s[pos:], char)
		if idx == -1 {
			break
		}
		count++
		pos += idx + 1
	}
	return count
}

// normalizeFullWidthDigits converts full-width (zenkaku) digits to half-width digits.
// Useful for accepting input from Japanese/CJK IMEs.
// Ported from upstream stringUtils.ts normalizeFullWidthDigits.
func normalizeFullWidthDigits(s string) string {
	var result strings.Builder
	for _, r := range s {
		if r >= '\uFF10' && r <= '\uFF19' {
			result.WriteRune(r - '\uFF10' + '0')
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// normalizeFullWidthSpace converts full-width (zenkaku) space (U+3000) to half-width space (U+0020).
// Ported from upstream stringUtils.ts normalizeFullWidthSpace.
func normalizeFullWidthSpace(s string) string {
	return strings.ReplaceAll(s, "\u3000", " ")
}

// EndTruncatingAccumulator is a string accumulator that truncates from the end
// when a size limit is exceeded. Prevents crashes from large outputs.
// Ported from upstream stringUtils.ts EndTruncatingAccumulator.
type EndTruncatingAccumulator struct {
	maxSize          int
	content          strings.Builder
	isTruncated      bool
	totalBytesRecv   int
}

// NewEndTruncatingAccumulator creates a new accumulator with the given max size.
func NewEndTruncatingAccumulator(maxSize int) *EndTruncatingAccumulator {
	return &EndTruncatingAccumulator{maxSize: maxSize}
}

// Append adds data to the accumulator. If total size exceeds maxSize,
// the end is truncated to maintain the limit.
func (a *EndTruncatingAccumulator) Append(data string) {
	a.totalBytesRecv += len(data)
	currentLen := a.content.Len()

	// Already at capacity
	if a.isTruncated && currentLen >= a.maxSize {
		return
	}

	if currentLen+len(data) > a.maxSize {
		// Append only what fits
		remaining := a.maxSize - currentLen
		if remaining > 0 {
			a.content.WriteString(data[:remaining])
		}
		a.isTruncated = true
	} else {
		a.content.WriteString(data)
	}
}

// String returns the accumulated string. If truncated, appends a truncation note.
func (a *EndTruncatingAccumulator) String() string {
	content := a.content.String()
	if !a.isTruncated {
		return content
	}
	truncatedBytes := a.totalBytesRecv - a.maxSize
	truncatedKB := truncatedBytes / 1024
	return content + fmt.Sprintf("\n... [output truncated - %dKB removed]", truncatedKB)
}

// Clear resets all accumulated data.
func (a *EndTruncatingAccumulator) Clear() {
	a.content.Reset()
	a.isTruncated = false
	a.totalBytesRecv = 0
}

// Length returns the current size of accumulated data.
func (a *EndTruncatingAccumulator) Length() int {
	return a.content.Len()
}

// Truncated returns whether truncation has occurred.
func (a *EndTruncatingAccumulator) Truncated() bool {
	return a.isTruncated
}

// TotalBytes returns total bytes received before truncation.
func (a *EndTruncatingAccumulator) TotalBytes() int {
	return a.totalBytesRecv
}

// truncateToLines truncates text to a maximum number of lines, adding
// an ellipsis if truncated. Ported from upstream stringUtils.ts truncateToLines.
func truncateToLines(text string, maxLines int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}
	return strings.Join(lines[:maxLines], "\n") + "\u2026"
}

// safeJoinLines safely joins an array of strings with a delimiter,
// truncating if the result exceeds maxSize.
// Ported from upstream stringUtils.ts safeJoinLines.
func safeJoinLines(lines []string, delimiter string, maxSize int) string {
	if delimiter == "" {
		delimiter = ","
	}
	if maxSize == 0 {
		maxSize = 1 << 25 // 32MB default
	}
	truncationMarker := "...[truncated]"
	var result strings.Builder
	for _, line := range lines {
		sep := ""
		if result.Len() > 0 {
			sep = delimiter
		}
		addition := sep + line
		if result.Len()+len(addition) <= maxSize {
			result.WriteString(addition)
		} else {
			remaining := maxSize - result.Len() - len(sep) - len(truncationMarker)
			if remaining > 0 {
				result.WriteString(sep)
				if remaining < len(line) {
					result.WriteString(line[:remaining])
				} else {
					result.WriteString(line)
				}
				result.WriteString(truncationMarker)
			} else {
				result.WriteString(truncationMarker)
			}
			return result.String()
		}
	}
	return result.String()
}