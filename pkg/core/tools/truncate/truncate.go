// Package truncate provides structured truncation utilities for tool output.
// Aligned to pi's tools/truncate.ts.
package truncate

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// DefaultMaxLines is the default maximum number of lines to keep.
	DefaultMaxLines = 2000

	// DefaultMaxBytes is the default maximum number of bytes (50KB).
	DefaultMaxBytes = 50 * 1024

	// GrepMaxLineLength is the max chars per grep match line.
	GrepMaxLineLength = 500
)

// TruncationResult holds the result of a truncation operation.
type TruncationResult struct {
	Content            string      // The truncated content
	Truncated          bool        // Whether truncation occurred
	TruncatedBy        string      // "lines", "bytes", or "" if not truncated
	TotalLines         int         // Total lines in original content
	TotalBytes         int         // Total bytes in original content
	OutputLines        int         // Lines in truncated output
	OutputBytes        int         // Bytes in truncated output
	LastLinePartial    bool        // Whether the last line was partially truncated (tail only)
	FirstLineExceeds   bool        // Whether the first line exceeded byte limit (head only)
	MaxLines           int         // The max lines limit applied
	MaxBytes           int         // The max bytes limit applied
}

// TruncationOptions configures truncation behavior.
type TruncationOptions struct {
	MaxLines int // Maximum number of lines (default: 2000)
	MaxBytes int // Maximum number of bytes (default: 50KB)
}

// FormatSize formats a byte count as a human-readable string.
func FormatSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
}

// splitLinesForCounting splits content into lines for counting.
// Handles trailing newline by removing the trailing empty element.
func splitLinesForCounting(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if len(content) > 0 && content[len(content)-1] == '\n' {
		// Remove trailing empty element from split
		lines = lines[:len(lines)-1]
	}
	return lines
}

// lineBytes returns the byte length of a line including the newline.
func lineBytes(line string) int {
	return len(line) + 1 // +1 for \n
}

// TruncateHead keeps the first N lines/bytes of content.
// Never returns partial lines. If the first line exceeds maxBytes,
// returns empty content with FirstLineExceeds set to true.
func TruncateHead(content string, opts TruncationOptions) TruncationResult {
	maxLines := opts.MaxLines
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	totalBytes := len(content)
	lines := splitLinesForCounting(content)
	totalLines := len(lines)

	// Check if no truncation needed
	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content:    content,
			Truncated:  false,
			TotalLines: totalLines,
			TotalBytes: totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
			MaxLines:   maxLines,
			MaxBytes:   maxBytes,
		}
	}

	// First line check: if first line alone exceeds maxBytes
	if len(lines) > 0 && lineBytes(lines[0]) > maxBytes {
		return TruncationResult{
			Content:          "",
			Truncated:        true,
			TruncatedBy:      "bytes",
			TotalLines:       totalLines,
			TotalBytes:       totalBytes,
			OutputLines:      0,
			OutputBytes:      0,
			FirstLineExceeds: true,
			MaxLines:         maxLines,
			MaxBytes:         maxBytes,
		}
	}

	// Collect lines forward
	var collected []string
	collectedBytes := 0
	truncatedBy := ""

	for i, line := range lines {
		lb := lineBytes(line)
		if i >= maxLines {
			truncatedBy = "lines"
			break
		}
		if collectedBytes+lb > maxBytes {
			truncatedBy = "bytes"
			break
		}
		collected = append(collected, line)
		collectedBytes += lb
	}

	output := strings.Join(collected, "\n")
	if len(collected) > 0 {
		output += "\n"
	}

	return TruncationResult{
		Content:        output,
		Truncated:      true,
		TruncatedBy:    truncatedBy,
		TotalLines:     totalLines,
		TotalBytes:     totalBytes,
		OutputLines:    len(collected),
		OutputBytes:    len(output),
		MaxLines:       maxLines,
		MaxBytes:       maxBytes,
	}
}

// truncateStringToBytesFromEnd truncates a string from the end, respecting UTF-8 boundaries.
func truncateStringToBytesFromEnd(str string, maxBytes int) string {
	if len(str) <= maxBytes {
		return str
	}

	// Find a valid UTF-8 boundary
	start := len(str) - maxBytes
	for start < len(str) {
		_, size := utf8.DecodeRuneInString(str[start:])
		if size > 0 && (str[start]&0xC0) != 0x80 {
			// Found a valid rune start (not a continuation byte)
			break
		}
		start++
	}

	return str[start:]
}

// TruncateTail keeps the last N lines/bytes of content.
// If the last line exceeds maxBytes and no other lines fit,
// returns a partial last line with LastLinePartial set to true.
func TruncateTail(content string, opts TruncationOptions) TruncationResult {
	maxLines := opts.MaxLines
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	totalBytes := len(content)
	lines := splitLinesForCounting(content)
	totalLines := len(lines)

	// Check if no truncation needed
	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content:     content,
			Truncated:   false,
			TotalLines:  totalLines,
			TotalBytes:  totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
			MaxLines:    maxLines,
			MaxBytes:    maxBytes,
		}
	}

	// Collect lines backward
	var collected []string
	collectedBytes := 0
	truncatedBy := ""

	for i := len(lines) - 1; i >= 0; i-- {
		lb := lineBytes(lines[i])
		if len(collected) >= maxLines {
			truncatedBy = "lines"
			break
		}
		if collectedBytes+lb > maxBytes {
			truncatedBy = "bytes"
			break
		}
		collected = append(collected, lines[i])
		collectedBytes += lb
	}

	// Reverse to original order
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}

	// Partial line edge case: if nothing fit and last line exceeds maxBytes
	if len(collected) == 0 && len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		partial := truncateStringToBytesFromEnd(lastLine, maxBytes)
		return TruncationResult{
			Content:         partial + "\n",
			Truncated:       true,
			TruncatedBy:     "bytes",
			TotalLines:      totalLines,
			TotalBytes:      totalBytes,
			OutputLines:     1,
			OutputBytes:     len(partial) + 1,
			LastLinePartial: true,
			MaxLines:        maxLines,
			MaxBytes:        maxBytes,
		}
	}

	output := strings.Join(collected, "\n")
	if len(collected) > 0 {
		output += "\n"
	}

	return TruncationResult{
		Content:     output,
		Truncated:   true,
		TruncatedBy: truncatedBy,
		TotalLines:  totalLines,
		TotalBytes:  totalBytes,
		OutputLines: len(collected),
		OutputBytes: len(output),
		MaxLines:    maxLines,
		MaxBytes:    maxBytes,
	}
}

// TruncateLineResult holds the result of truncating a single line.
type TruncateLineResult struct {
	Text         string
	WasTruncated bool
}

// TruncateLine truncates a single line to maxChars.
// Appends "... [truncated]" if truncated.
func TruncateLine(line string, maxChars ...int) TruncateLineResult {
	max := GrepMaxLineLength
	if len(maxChars) > 0 && maxChars[0] > 0 {
		max = maxChars[0]
	}

	if len(line) <= max {
		return TruncateLineResult{Text: line, WasTruncated: false}
	}

	return TruncateLineResult{
		Text:         line[:max] + "... [truncated]",
		WasTruncated: true,
	}
}
