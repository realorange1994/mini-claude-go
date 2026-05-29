// Package renderutils provides shared rendering utilities for tool output.
// Aligned to pi's tools/render-utils.ts.
package renderutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// ShortenPath replaces the user's home directory with ~ in a path.
func ShortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// SafeString casts an interface{} to string safely.
func SafeString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ReplaceTabs replaces tab characters with 4 spaces.
func ReplaceTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

// NormalizeDisplayText normalizes text for display:
//   - Replaces tabs with spaces
//   - Strips ANSI escape sequences
//   - Strips null bytes (binary content indicator)
//   - Replaces \r\n with \n
func NormalizeDisplayText(s string) string {
	// Replace tabs
	s = ReplaceTabs(s)

	// Strip ANSI escape sequences
	s = StripAnsi(s)

	// Strip null bytes (indicates binary content)
	s = strings.ReplaceAll(s, "\x00", "")

	// Normalize line endings
	s = strings.ReplaceAll(s, "\r\n", "\n")

	return s
}

// StripAnsi removes ANSI escape sequences from a string.
func StripAnsi(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			// Start of escape sequence
			i++
			if i < len(s) && s[i] == '[' {
				// CSI sequence: skip until terminator
				i++
				for i < len(s) {
					if s[i] >= 0x40 && s[i] <= 0x7e {
						i++
						break
					}
					i++
				}
			} else if i < len(s) && (s[i] == ']' || s[i] == 'P' || s[i] == '_' || s[i] == '^') {
				// OSC or other sequence: skip until BEL or ST
				for i < len(s) {
					if s[i] == '\x07' { // BEL
						i++
						break
					}
					if i+1 < len(s) && s[i] == '\x1b' && s[i+1] == '\\' { // ST
						i += 2
						break
					}
					i++
				}
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// GetTextOutput extracts text content from tool result content blocks.
// Returns the text content and optionally image blocks.
type TextImageContent struct {
	Text   string
	Images []ImageData
}

// ImageData represents an inline image in tool output.
type ImageData struct {
	Base64   string
	MimeType string
}

// SanitizeForDisplay sanitizes text for safe display.
// Strips control characters except newlines and tabs.
func SanitizeForDisplay(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' || r == '\r' {
			b.WriteRune(r)
		} else if unicode.IsControl(r) {
			continue
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// InvalidArgText returns a formatted error message for invalid tool arguments.
func InvalidArgText(argName string, details string) string {
	return fmt.Sprintf("Invalid argument '%s': %s", argName, details)
}

// FormatFileLabel creates a display label for a file path.
func FormatFileLabel(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if dir == "." {
		return base
	}
	return filepath.Join(ShortenPath(dir), base)
}

// IsBinaryContent detects if content appears to be binary.
// Uses the same heuristic as git: if the first 8000 bytes contain a NUL byte,
// the content is considered binary.
func IsBinaryContent(data []byte) bool {
	checkLen := len(data)
	if checkLen > 8000 {
		checkLen = 8000
	}
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// TruncateOutput truncates output to maxLines, appending a truncation notice.
func TruncateOutput(output string, maxLines int) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}

	// Remove trailing empty from split
	if output[len(output)-1] == '\n' {
		lines = lines[:len(lines)-1]
	}

	if len(lines) <= maxLines {
		return output
	}

	result := strings.Join(lines[:maxLines], "\n")
	result += fmt.Sprintf("\n\n... (%d more lines)", len(lines)-maxLines)
	return result
}
