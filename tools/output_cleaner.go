package tools

import (
	"fmt"
	"regexp"
	"strings"
)

// Terminal escape sequence patterns (matching openclacky's OutputCleaner).
var (
	csiRegex       = regexp.MustCompile(`\x1b\[[\d;?]*[a-zA-Z@]`) // ESC[...]letter — colors, cursor, SGR
	oscRegex       = regexp.MustCompile(`\x1b\].*?(\a|\x1b\\)`)   // ESC]...BEL/ST — window title, etc.
	simpleEscRegex = regexp.MustCompile(`\x1b[=>\(\)].?`)         // ESC= / ESC>) — keypad modes
)

// StripTerminalCodes cleans raw terminal/PTY output for LLM consumption.
// It strips visual control sequences that convey no useful information:
//
//  1. CSI sequences  (ESC[...]letter) — colors, cursor positioning, SGR
//  2. OSC sequences  (ESC]...BEL/ST) — window titles, OSC hyperlinks
//  3. Simple 2-byte ESC (ESC= / ESC>) — keypad modes
//  4. \r-overwrites  — spinner/progress bars ("50%\r100%" → "100%")
//  5. Backspace erase (X\b pairs) — readline rubout
//  6. Leftover \r normalization
//
// This is lossy for full-screen apps (vim/top), but for line-based commands
// it yields clean, diff-friendly output. Inspired by openclacky's OutputCleaner.
func StripTerminalCodes(raw string) string {
	if raw == "" {
		return raw
	}

	s := raw

	// Step 1: Strip CSI sequences (colors, cursor movements, etc.)
	s = csiRegex.ReplaceAllString(s, "")

	// Step 2: Strip OSC sequences (window titles, OSC hyperlinks)
	s = oscRegex.ReplaceAllString(s, "")

	// Step 3: Strip simple ESC sequences (keypad modes)
	s = simpleEscRegex.ReplaceAllString(s, "")

	// Step 4: Collapse \r-overwrites within each line.
	// Split on \n, then for each segment keep only the portion after
	// the last \r (which is what would actually be visible on screen).
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if idx := strings.LastIndex(line, "\r"); idx >= 0 {
			lines[i] = line[idx+1:]
		}
	}
	s = strings.Join(lines, "\n")

	// Step 5: Drop backspace erase pairs (readline rubout).
	// Repeatedly remove "X\b" pairs until none remain.
	backspaceRegex := regexp.MustCompile(`[^\x08]\x08`)
	for backspaceRegex.MatchString(s) {
		s = backspaceRegex.ReplaceAllString(s, "")
	}

	// Step 6: Normalize any leftover isolated \r.
	s = strings.ReplaceAll(s, "\r", "")

	return s
}

// TruncateLongLines caps each line at maxLen characters.
// A single minified JSON line or base64 blob can consume the entire
// output budget (e.g., 50K chars on one line). Per-line truncation
// prevents this by shortening each line before the overall output cap.
// Inspired by openclacky's per-line truncation in the terminal tool.
func TruncateLongLines(raw string, maxLen int) string {
	if raw == "" || maxLen <= 0 {
		return raw
	}

	lines := strings.Split(raw, "\n")
	truncated := 0
	for i, line := range lines {
		if len(line) > maxLen {
			lines[i] = line[:maxLen] + " [... truncated]"
			truncated++
		}
	}
	if truncated > 0 {
		// Append notice at the end so the LLM knows lines were shortened
		return strings.Join(lines, "\n") + fmt.Sprintf("\n[%d lines truncated to %d chars]", truncated, maxLen)
	}
	return strings.Join(lines, "\n")
}
