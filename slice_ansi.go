package main

import (
	"fmt"
	"regexp"
	"strconv"
)

// sliceAnsi slices a string containing ANSI escape codes at display-cell
// boundaries, preserving ANSI codes within the slice and properly closing
// any active styles at the slice end.
// Ported from upstream sliceAnsi.ts sliceAnsi.
func sliceAnsi(str string, start int, end ...int) string {
	endIdx := len([]rune(str))
	if len(end) > 0 {
		endIdx = end[0]
	}
	if endIdx < start {
		endIdx = start
	}

	// Parse the string into display runes and ANSI codes
	segments := parseAnsiSegments(str)

	var result string
	var position int
	include := false
	var activeCodes []ansiCode

	for _, seg := range segments {
		width := seg.displayWidth

		// Break after trailing zero-width marks
		if endIdx >= 0 && position >= endIdx {
			if seg.typ == segmentAnsi || width > 0 || !include {
				break
			}
		}

		if seg.typ == segmentAnsi {
			activeCodes = append(activeCodes, seg.code)
			if include {
				result += seg.raw
			}
		} else {
			if !include && position >= start {
				if start > 0 && width == 0 {
					continue
				}
				include = true
				// Emit all active start codes
				result = activeCodesToStartCodes(activeCodes)
			}

			if include {
				result += seg.value
			}

			position += width
		}
	}

	// Close any remaining active codes
	if include {
		result += closeAnsiCodes(activeCodes)
	}

	return result
}

// Segment type constants
const (
	segmentChar = iota
	segmentAnsi
)

// ansiCode represents a parsed ANSI escape code.
type ansiCode struct {
	params []int
	cmd    string
	raw    string
}

type ansiSegment struct {
	typ          int // segmentChar or segmentAnsi
	value        string
	displayWidth int
	code         ansiCode
	raw          string
}

// ansiRegex matches ANSI SGR escape sequences.
var ansiRegex = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

func parseAnsiSegments(str string) []ansiSegment {
	var segments []ansiSegment
	remaining := str

	for {
		match := ansiRegex.FindStringSubmatchIndex(remaining)
		if match == nil {
			// No more ANSI codes - remaining text is plain characters
			if len(remaining) > 0 {
				for _, ch := range remaining {
					segments = append(segments, ansiSegment{
						typ:          segmentChar,
						value:        string(ch),
						displayWidth: runeDisplayWidth(ch),
						raw:          string(ch),
					})
				}
			}
			break
		}

		// Add plain text before this ANSI code
		if match[0] > 0 {
			for _, ch := range remaining[:match[0]] {
				segments = append(segments, ansiSegment{
					typ:          segmentChar,
					value:        string(ch),
					displayWidth: runeDisplayWidth(ch),
					raw:          string(ch),
				})
			}
		}

		// Parse the ANSI code
		raw := remaining[match[0]:match[1]]
		code := parseAnsiCode(raw)
		segments = append(segments, ansiSegment{
			typ:          segmentAnsi,
			code:         code,
			raw:          raw,
			displayWidth: 0,
		})

		remaining = remaining[match[1]:]
	}

	return segments
}

func parseAnsiCode(raw string) ansiCode {
	// Extract params from \x1b[...m
	inner := raw[2 : len(raw)-1] // strip \x1b[ and m
	var params []int
	if inner == "" {
		params = []int{0}
	} else {
		for _, part := range regexp.MustCompile(`;`).Split(inner, -1) {
			if n, err := strconv.Atoi(part); err == nil {
				params = append(params, n)
			}
		}
		if len(params) == 0 {
			params = []int{0}
		}
	}
	return ansiCode{params: params, raw: raw}
}

// activeCodesToStartCodes returns a string with only the "start" (open) codes
// from the given list, suitable for prefixing a slice.
func activeCodesToStartCodes(codes []ansiCode) string {
	var result string
	for _, code := range codes {
		result += code.raw
	}
	return result
}

// closeAnsiCodes returns the reset sequences for currently active codes.
func closeAnsiCodes(codes []ansiCode) string {
	if len(codes) == 0 {
		return ""
	}
	// Collect active SGR parameters to generate appropriate resets
	var resetCodes []string
	for _, code := range codes {
		if len(code.params) == 0 || code.params[0] == 0 {
			// Reset all
			continue
		}
		for _, p := range code.params {
			if p == 0 {
				// Reset all
				continue
			}
			// Map param to reset code
			switch {
			case p == 1:
				resetCodes = append(resetCodes, "\x1b[22m") // normal intensity
			case p == 2:
				resetCodes = append(resetCodes, "\x1b[22m")
			case p == 3:
				resetCodes = append(resetCodes, "\x1b[23m") // no italic
			case p == 4:
				resetCodes = append(resetCodes, "\x1b[24m") // no underline
			case p == 5:
				resetCodes = append(resetCodes, "\x1b[25m") // no blink
			case p == 7:
				resetCodes = append(resetCodes, "\x1b[27m") // no reverse
			case p == 8:
				resetCodes = append(resetCodes, "\x1b[28m") // no conceal
			case p == 9:
				resetCodes = append(resetCodes, "\x1b[29m") // no strikethrough
			case p >= 30 && p <= 37:
				resetCodes = append(resetCodes, "\x1b[39m") // default foreground
			case p == 38:
				resetCodes = append(resetCodes, "\x1b[39m")
			case p == 39:
				resetCodes = append(resetCodes, "\x1b[39m")
			case p >= 40 && p <= 47:
				resetCodes = append(resetCodes, "\x1b[49m") // default background
			case p == 48:
				resetCodes = append(resetCodes, "\x1b[49m")
			case p == 49:
				resetCodes = append(resetCodes, "\x1b[49m")
			case p == 90:
				resetCodes = append(resetCodes, "\x1b[39m") // bright fg default
			}
		}
	}
	if len(resetCodes) == 0 {
		return ""
	}
	return resetCodes[len(resetCodes)-1]
}

// undoAnsiCodes generates reset sequences for the given codes.
// Used to close active styles at slice end.
func undoAnsiCodes(codes []ansiCode) string {
	return closeAnsiCodes(codes)
}

// stripAnsiCodes removes all ANSI escape codes from a string, returning
// only the visible text.
func stripAnsiCodes(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// ansiCodeCount counts the number of ANSI escape sequences in a string.
func ansiCodeCount(s string) int {
	return len(ansiRegex.FindAllString(s, -1))
}

// containsAnsiCode checks if a string contains ANSI escape codes.
func containsAnsiCode(s string) bool {
	return ansiRegex.MatchString(s)
}

// Helper to get ANSI reset code for a foreground color parameter
func fgResetForParam(p int) string {
	if p >= 30 && p <= 37 || p == 38 || p == 39 {
		return "\x1b[39m"
	}
	if p >= 90 && p <= 97 {
		return "\x1b[39m"
	}
	return ""
}

// Helper to get ANSI reset code for a background color parameter
func bgResetForParam(p int) string {
	if p >= 40 && p <= 47 || p == 48 || p == 49 {
		return "\x1b[49m"
	}
	if p >= 100 && p <= 107 {
		return "\x1b[49m"
	}
	return ""
}

// _ unused import suppressors for helper functions
var _ = fgResetForParam
var _ = bgResetForParam
var _ = fmt.Sprintf // keep for potential future use