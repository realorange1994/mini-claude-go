package main

import (
	"math"
	"strings"
	"unicode"
)

// ─── Multi-Strategy Edit Replacer (MiMo-Code 1) ───────────────────────────
//
// Cascading replacers for edit tool to reduce edit failures.
// Tries 9 strategies when exact match fails.
//
// MiMo-Code source: tool/edit.ts (187-685 lines)

// EditReplacer represents a string replacement strategy.
type EditReplacer interface {
	Name() string
	Replace(content, oldString, newString string) (string, bool)
}

// EditReplacerChain chains multiple replacers.
type EditReplacerChain struct {
	replacers []EditReplacer
}

// NewEditReplacerChain creates a new replacer chain with default strategies.
func NewEditReplacerChain() *EditReplacerChain {
	return &EditReplacerChain{
		replacers: []EditReplacer{
			&SimpleReplacer{},
			&LineTrimmedReplacer{},
			&WhitespaceNormalizedReplacer{},
			&IndentationFlexibleReplacer{},
			&EscapeNormalizedReplacer{},
			&TrimmedBoundaryReplacer{},
			&BlockAnchorReplacer{},
			&ContextAwareReplacer{},
			&MultiOccurrenceReplacer{},
		},
	}
}

// Replace tries each replacer in order until one succeeds.
func (c *EditReplacerChain) Replace(content, oldString, newString string) (string, bool) {
	// Try exact match first
	if strings.Contains(content, oldString) {
		return strings.Replace(content, oldString, newString, 1), true
	}

	// Try each replacer
	for _, r := range c.replacers {
		if result, ok := r.Replace(content, oldString, newString); ok {
			return result, true
		}
	}

	return content, false
}

// ─── Simple Replacer ─────────────────────────────────────────────────────

// SimpleReplacer does exact string replacement.
type SimpleReplacer struct{}

func (r *SimpleReplacer) Name() string { return "simple" }
func (r *SimpleReplacer) Replace(content, oldString, newString string) (string, bool) {
	if strings.Contains(content, oldString) {
		return strings.Replace(content, oldString, newString, 1), true
	}
	return content, false
}

// ─── Line Trimmed Replacer ───────────────────────────────────────────────

// LineTrimmedReplacer ignores leading/trailing whitespace per line.
type LineTrimmedReplacer struct{}

func (r *LineTrimmedReplacer) Name() string { return "line-trimmed" }
func (r *LineTrimmedReplacer) Replace(content, oldString, newString string) (string, bool) {
	oldLines := strings.Split(oldString, "\n")
	newLines := strings.Split(newString, "\n")

	// Build trimmed pattern
	var trimmedOld []string
	for _, line := range oldLines {
		trimmedOld = append(trimmedOld, strings.TrimSpace(line))
	}

	contentLines := strings.Split(content, "\n")
	for i := 0; i <= len(contentLines)-len(oldLines); i++ {
		match := true
		for j, oldLine := range trimmedOld {
			if strings.TrimSpace(contentLines[i+j]) != oldLine {
				match = false
				break
			}
		}
		if match {
			// Replace preserving original indentation
			result := make([]string, len(contentLines))
			copy(result, contentLines)
			for j, newLine := range newLines {
				result[i+j] = newLine
			}
			return strings.Join(result, "\n"), true
		}
	}

	return content, false
}

// ─── Whitespace Normalized Replacer ─────────────────────────────────────

// WhitespaceNormalizedReplacer normalizes all whitespace to single spaces.
type WhitespaceNormalizedReplacer struct{}

func (r *WhitespaceNormalizedReplacer) Name() string { return "whitespace-normalized" }
func (r *WhitespaceNormalizedReplacer) Replace(content, oldString, newString string) (string, bool) {
	normalizeWS := func(s string) string {
		return strings.Join(strings.Fields(s), " ")
	}

	normalizedOld := normalizeWS(oldString)
	normalizedContent := normalizeWS(content)

	if strings.Contains(normalizedContent, normalizedOld) {
		// Find the position in normalized content
		idx := strings.Index(normalizedContent, normalizedOld)
		if idx < 0 {
			return content, false
		}

		// Map back to original content position
		origIdx := findOriginalIndex(content, idx)
		if origIdx < 0 {
			return content, false
		}

		// Find end position
		origEnd := findOriginalIndex(content, idx+len(normalizedOld))
		if origEnd < 0 {
			return content, false
		}

		return content[:origIdx] + newString + content[origEnd:], true
	}

	return content, false
}

// findOriginalIndex maps normalized index back to original string.
func findOriginalIndex(original string, normalizedIdx int) int {
	count := 0
	for i, r := range original {
		if !unicode.IsSpace(r) || (i > 0 && !unicode.IsSpace(rune(original[i-1]))) {
			count++
		}
		if count > normalizedIdx {
			return i
		}
	}
	return len(original)
}

// ─── Indentation Flexible Replacer ──────────────────────────────────────

// IndentationFlexibleReplacer strips common indentation before matching.
type IndentationFlexibleReplacer struct{}

func (r *IndentationFlexibleReplacer) Name() string { return "indentation-flexible" }
func (r *IndentationFlexibleReplacer) Replace(content, oldString, newString string) (string, bool) {
	// Find common indentation in old string
	oldLines := strings.Split(oldString, "\n")
	minIndent := math.MaxInt32
	for _, line := range oldLines {
		trimmed := strings.TrimLeft(line, " \t")
		if len(trimmed) > 0 {
			indent := len(line) - len(trimmed)
			if indent < minIndent {
				minIndent = indent
			}
		}
	}

	// Strip common indentation
	var strippedOld []string
	for _, line := range oldLines {
		if len(line) >= minIndent {
			strippedOld = append(strippedOld, line[minIndent:])
		} else {
			strippedOld = append(strippedOld, line)
		}
	}
	strippedOldStr := strings.Join(strippedOld, "\n")

	// Try to find stripped pattern in content
	contentLines := strings.Split(content, "\n")
	for i := 0; i <= len(contentLines)-len(oldLines); i++ {
		block := strings.Join(contentLines[i:i+len(oldLines)], "\n")
		blockStripped := stripCommonIndent(block)

		if blockStripped == strippedOldStr {
			// Found match, preserve indentation
			indent := detectIndent(contentLines[i])
			newLines := strings.Split(newString, "\n")
			var indentedNew []string
			for _, line := range newLines {
				indentedNew = append(indentedNew, indent+line)
			}
			result := make([]string, len(contentLines))
			copy(result, contentLines)
			for j, line := range indentedNew {
				result[i+j] = line
			}
			return strings.Join(result, "\n"), true
		}
	}

	return content, false
}

// stripCommonIndent removes common leading whitespace from a block.
func stripCommonIndent(block string) string {
	lines := strings.Split(block, "\n")
	minIndent := math.MaxInt32
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if len(trimmed) > 0 {
			indent := len(line) - len(trimmed)
			if indent < minIndent {
				minIndent = indent
			}
		}
	}

	var result []string
	for _, line := range lines {
		if len(line) >= minIndent {
			result = append(result, line[minIndent:])
		} else {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

// detectIndent detects the indentation of a line.
func detectIndent(line string) string {
	var indent strings.Builder
	for _, r := range line {
		if r == ' ' || r == '\t' {
			indent.WriteRune(r)
		} else {
			break
		}
	}
	return indent.String()
}

// ─── Escape Normalized Replacer ─────────────────────────────────────────

// EscapeNormalizedReplacer normalizes escape sequences.
type EscapeNormalizedReplacer struct{}

func (r *EscapeNormalizedReplacer) Name() string { return "escape-normalized" }
func (r *EscapeNormalizedReplacer) Replace(content, oldString, newString string) (string, bool) {
	normalizeEscapes := func(s string) string {
		s = strings.ReplaceAll(s, "\\n", "\n")
		s = strings.ReplaceAll(s, "\\t", "\t")
		s = strings.ReplaceAll(s, "\\r", "\r")
		return s
	}

	normalizedOld := normalizeEscapes(oldString)
	normalizedContent := normalizeEscapes(content)

	if strings.Contains(normalizedContent, normalizedOld) {
		// Find in normalized and replace in original
		idx := strings.Index(normalizedContent, normalizedOld)
		if idx >= 0 {
			// Simple approach: replace in original
			return strings.Replace(content, oldString, newString, 1), true
		}
	}

	return content, false
}

// ─── Trimmed Boundary Replacer ──────────────────────────────────────────

// TrimmedBoundaryReplacer trims empty lines at start/end of old string.
type TrimmedBoundaryReplacer struct{}

func (r *TrimmedBoundaryReplacer) Name() string { return "trimmed-boundary" }
func (r *TrimmedBoundaryReplacer) Replace(content, oldString, newString string) (string, bool) {
	// Trim empty lines at boundaries
	oldLines := strings.Split(oldString, "\n")
	startTrim := 0
	for i, line := range oldLines {
		if strings.TrimSpace(line) != "" {
			startTrim = i
			break
		}
	}

	endTrim := len(oldLines)
	for i := len(oldLines) - 1; i >= 0; i-- {
		if strings.TrimSpace(oldLines[i]) != "" {
			endTrim = i + 1
			break
		}
	}

	if startTrim > 0 || endTrim < len(oldLines) {
		trimmedOld := strings.Join(oldLines[startTrim:endTrim], "\n")
		if strings.Contains(content, trimmedOld) {
			return strings.Replace(content, trimmedOld, newString, 1), true
		}
	}

	return content, false
}

// ─── Block Anchor Replacer ──────────────────────────────────────────────

// BlockAnchorReplacer matches first/last lines as anchors.
type BlockAnchorReplacer struct{}

func (r *BlockAnchorReplacer) Name() string { return "block-anchor" }
func (r *BlockAnchorReplacer) Replace(content, oldString, newString string) (string, bool) {
	oldLines := strings.Split(oldString, "\n")
	if len(oldLines) < 2 {
		return content, false
	}

	firstLine := strings.TrimSpace(oldLines[0])
	lastLine := strings.TrimSpace(oldLines[len(oldLines)-1])

	contentLines := strings.Split(content, "\n")

	// Find first line anchor
	for i, line := range contentLines {
		if strings.TrimSpace(line) == firstLine {
			// Check if last line matches at expected position
			expectedLastIdx := i + len(oldLines) - 1
			if expectedLastIdx < len(contentLines) && strings.TrimSpace(contentLines[expectedLastIdx]) == lastLine {
				// Match found, replace block
				result := make([]string, len(contentLines))
				copy(result, contentLines)
				newLines := strings.Split(newString, "\n")
				for j, newLine := range newLines {
					if i+j < len(result) {
						result[i+j] = newLine
					}
				}
				return strings.Join(result, "\n"), true
			}
		}
	}

	return content, false
}

// ─── Context Aware Replacer ─────────────────────────────────────────────

// ContextAwareReplacer uses surrounding context for matching.
type ContextAwareReplacer struct{}

func (r *ContextAwareReplacer) Name() string { return "context-aware" }
func (r *ContextAwareReplacer) Replace(content, oldString, newString string) (string, bool) {
	// Try to find unique context around the match
	oldLines := strings.Split(oldString, "\n")
	if len(oldLines) < 3 {
		return content, false
	}

	// Use first 2 lines as context
	contextLines := oldLines[:2]
	contextStr := strings.Join(contextLines, "\n")

	contentLines := strings.Split(content, "\n")
	for i := 0; i <= len(contentLines)-len(oldLines); i++ {
		block := strings.Join(contentLines[i:i+2], "\n")
		if block == contextStr {
			// Context matches, check remaining lines
			match := true
			for j := 2; j < len(oldLines); j++ {
				if i+j >= len(contentLines) || strings.TrimSpace(contentLines[i+j]) != strings.TrimSpace(oldLines[j]) {
					match = false
					break
				}
			}
			if match {
				result := make([]string, len(contentLines))
				copy(result, contentLines)
				newLines := strings.Split(newString, "\n")
				for j, newLine := range newLines {
					if i+j < len(result) {
						result[i+j] = newLine
					}
				}
				return strings.Join(result, "\n"), true
			}
		}
	}

	return content, false
}

// ─── Multi Occurrence Replacer ──────────────────────────────────────────

// MultiOccurrenceReplacer handles multiple occurrences.
type MultiOccurrenceReplacer struct{}

func (r *MultiOccurrenceReplacer) Name() string { return "multi-occurrence" }
func (r *MultiOccurrenceReplacer) Replace(content, oldString, newString string) (string, bool) {
	count := strings.Count(content, oldString)
	if count > 1 {
		// Replace first occurrence
		return strings.Replace(content, oldString, newString, 1), true
	}
	return content, false
}
