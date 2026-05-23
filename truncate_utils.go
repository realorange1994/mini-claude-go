package main

import (
	"strings"
	"unicode"
)

// stringWidth computes the display width of a string in terminal columns.
// CJK and most emoji count as 2 cells. ASCII counts as 1 cell.
// Zero-width characters (combining marks, ZWJ) count as 0.
// Simplified port of @anthropic/ink stringWidth behavior.
func stringWidth(s string) int {
	width := 0
	for _, r := range s {
		width += runeDisplayWidth(r)
	}
	return width
}

// runeDisplayWidth returns the display width of a single rune.
func runeDisplayWidth(r rune) int {
	// Control characters, combining marks, ZWJ, BOM, etc. are zero-width
	if r < 0x20 || (r >= 0x7F && r <= 0x9F) {
		return 0
	}
	if isCombiningMark(r) {
		return 0
	}

	// CJK Unified Ideographs, CJK Extensions, CJK Compatibility
	if (r >= 0x2E80 && r <= 0x9FFF) ||
		(r >= 0xAC00 && r <= 0xD7A3) || // Hangul Syllables
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0xFE10 && r <= 0xFE1F) || // Vertical Forms
		(r >= 0xFE30 && r <= 0xFE4F) || // CJK Compatibility Forms
		(r >= 0xFF01 && r <= 0xFF60) || // Fullwidth ASCII (excludes halfwidth katakana)
		(r >= 0xFFE0 && r <= 0xFFE6) || // Fullwidth symbols
		(r >= 0x20000 && r <= 0x2FFFD) || // CJK Extension B
		(r >= 0x30000 && r <= 0x3FFFD) {
		return 2
	}

	// Most emoji in the BMP range (U+2600 - U+27BF, U+1F000+)
	if r >= 0x2600 && r <= 0x27BF {
		return 2
	}
	if r >= 0x1F300 && r <= 0x1FAD6 {
		return 2
	}
	if r >= 0x1F000 && r <= 0x1F0FF {
		return 2
	}
	if r >= 0x1F600 && r <= 0x1F6FF {
		return 2
	}
	if r >= 0x1F900 && r <= 0x1F9FF {
		return 2
	}
	if r >= 0x1FA00 && r <= 0x1FAFF {
		return 2
	}
	if r >= 0x1FAE0 && r <= 0x1FAE8 {
		return 2
	}

	// Default: 1
	return 1
}

// isCombiningMark checks if a rune is a combining character (zero-width).
func isCombiningMark(r rune) bool {
	// Unicode Category Mn (Nonspacing Mark) and Me (Enclosing Mark)
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r)
}

// truncateToWidth truncates a string to fit within a maximum display width,
// appending '…' when truncation occurs.
// Ported from upstream truncate.ts truncateToWidth.
func truncateToWidth(text string, maxWidth int) string {
	if stringWidth(text) <= maxWidth {
		return text
	}
	if maxWidth <= 1 {
		return "\u2026" // …
	}

	var result strings.Builder
	var width int

	for _, ch := range text {
		chWidth := runeDisplayWidth(ch)
		if width+chWidth > maxWidth-1 {
			break
		}
		result.WriteRune(ch)
		width += chWidth
	}
	return result.String() + "\u2026"
}

// truncateStartToWidth truncates from the start of a string, keeping the
// tail end, and prepending '…' when truncation occurs.
// Ported from upstream truncate.ts truncateStartToWidth.
func truncateStartToWidth(text string, maxWidth int) string {
	if stringWidth(text) <= maxWidth {
		return text
	}
	if maxWidth <= 1 {
		return "\u2026"
	}

	runes := []rune(text)
	runewidths := make([]int, len(runes))
	for i, r := range runes {
		runewidths[i] = runeDisplayWidth(r)
	}

	var width int
	startIdx := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		if width+runewidths[i] > maxWidth-1 {
			break
		}
		width += runewidths[i]
		startIdx = i
	}

	return "\u2026" + string(runes[startIdx:])
}

// truncateToWidthNoEllipsis truncates a string to fit within a maximum
// display width, without appending an ellipsis.
// Ported from upstream truncate.ts truncateToWidthNoEllipsis.
func truncateToWidthNoEllipsis(text string, maxWidth int) string {
	if stringWidth(text) <= maxWidth {
		return text
	}
	if maxWidth <= 0 {
		return ""
	}

	var result strings.Builder
	var width int

	for _, ch := range text {
		chWidth := runeDisplayWidth(ch)
		if width+chWidth > maxWidth {
			break
		}
		result.WriteRune(ch)
		width += chWidth
	}
	return result.String()
}

// truncatePathMiddle truncates a file path in the middle to preserve
// both directory context and filename.
// Ported from upstream truncate.ts truncatePathMiddle.
func truncatePathMiddle(path string, maxLength int) string {
	if stringWidth(path) <= maxLength {
		return path
	}

	if maxLength <= 0 {
		return "\u2026"
	}

	if maxLength < 5 {
		return truncateToWidth(path, maxLength)
	}

	// Find filename (last segment after '/')
	lastSlash := strings.LastIndex(path, "/")
	var filename string
	var directory string
	if lastSlash >= 0 {
		filename = path[lastSlash:] // Include the leading slash
		directory = path[:lastSlash]
	} else {
		filename = path
		directory = ""
	}

	filenameWidth := stringWidth(filename)

	if filenameWidth >= maxLength-1 {
		return truncateStartToWidth(path, maxLength)
	}

	availableForDir := maxLength - 1 - filenameWidth
	if availableForDir <= 0 {
		return truncateStartToWidth(filename, maxLength)
	}

	truncatedDir := truncateToWidthNoEllipsis(directory, availableForDir)
	return truncatedDir + "\u2026" + filename
}

// truncateStringPorted truncates a string to fit within a maximum display width.
// If singleLine is true, also truncates at the first newline.
// Ported from upstream truncate.ts truncate.
func truncateStringPorted(str string, maxWidth int, singleLine ...bool) string {
	sl := false
	if len(singleLine) > 0 {
		sl = singleLine[0]
	}

	if sl {
		firstNewline := strings.Index(str, "\n")
		if firstNewline != -1 {
			result := str[:firstNewline]
			if stringWidth(result)+1 > maxWidth {
				return truncateToWidth(result, maxWidth)
			}
			return result + "\u2026"
		}
	}

	if stringWidth(str) <= maxWidth {
		return str
	}
	return truncateToWidth(str, maxWidth)
}

// wrapText wraps text into lines of the given display width.
// Ported from upstream truncate.ts wrapText.
func wrapText(text string, width int) []string {
	if text == "" {
		return nil
	}

	var lines []string
	var result strings.Builder
	var currentWidth int

	for _, ch := range text {
		chWidth := runeDisplayWidth(ch)
		if currentWidth+chWidth <= width {
			result.WriteRune(ch)
			currentWidth += chWidth
		} else {
			lines = append(lines, result.String())
			result.Reset()
			result.WriteRune(ch)
			currentWidth = chWidth
		}
	}

	if result.Len() > 0 {
		lines = append(lines, result.String())
	}
	return lines
}
