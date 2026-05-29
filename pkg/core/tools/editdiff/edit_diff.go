// Package editdiff provides fuzzy matching, multi-edit, BOM/line-ending handling,
// and diff generation for the Edit tool.
// Aligned to pi's tools/edit-diff.ts.
package editdiff

import (
	"fmt"
	"os"
	"strings"
	"unicode"
)

// normalizeForFuzzyMatch applies Unicode normalization to make LLM-produced text
// more likely to match the original file content.
// Normalizes:
//   - Smart quotes (U+2018, U+2019, U+201C, U+201D) -> ASCII ' and "
//   - Unicode dashes (en-dash U+2013, em-dash U+2014) -> ASCII -
//   - Special spaces (NBSP U+00A0, ideographic U+3000) -> regular space
//   - Strips trailing whitespace from each line
func NormalizeForFuzzyMatch(s string) string {
	// Normalize Unicode variants
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\u2018', '\u2019': // Left/right single quote
			b.WriteRune('\'')
		case '\u201C', '\u201D': // Left/right double quote
			b.WriteRune('"')
		case '\u2013', '\u2014': // En-dash, em-dash
			b.WriteRune('-')
		case '\u00A0', '\u3000': // NBSP, ideographic space
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}

	// Strip trailing whitespace from each line
	lines := strings.Split(b.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	return strings.Join(lines, "\n")
}

// stripBom detects and strips the UTF-8 BOM from the beginning of content.
// Returns (text, bom) where bom is the BOM string or empty.
func StripBom(content string) (string, string) {
	const utf8BOM = "\xEF\xBB\xBF"
	if strings.HasPrefix(content, utf8BOM) {
		return content[len(utf8BOM):], utf8BOM
	}
	return content, ""
}

// detectLineEnding detects the dominant line ending in content.
// Returns "\r\n" or "\n".
func DetectLineEnding(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// normalizeToLF converts all line endings to \n for matching.
func NormalizeToLF(content string) string {
	return strings.ReplaceAll(content, "\r\n", "\n")
}

// restoreLineEndings converts \n back to the original line ending.
func RestoreLineEndings(content, ending string) string {
	if ending == "\r\n" {
		return strings.ReplaceAll(content, "\n", "\r\n")
	}
	return content
}

// Edit represents a single edit operation.
type Edit struct {
	OldText string
	NewText string
}

// MatchResult holds the result of finding text in content.
type MatchResult struct {
	Found          bool
	Index          int
	MatchLength    int
	UsedFuzzyMatch bool
	ContentForReplacement string // normalized content if fuzzy matched
}

// FuzzyFindText tries to find oldText in content.
// Tries exact match first, then fuzzy match.
func FuzzyFindText(content, oldText string) MatchResult {
	// Exact match first
	idx := strings.Index(content, oldText)
	if idx != -1 {
		return MatchResult{Found: true, Index: idx, MatchLength: len(oldText), UsedFuzzyMatch: false}
	}

	// Fuzzy match
	normContent := NormalizeForFuzzyMatch(content)
	normOld := NormalizeForFuzzyMatch(oldText)
	idx = strings.Index(normContent, normOld)
	if idx != -1 {
		return MatchResult{
			Found:          true,
			Index:          idx,
			MatchLength:    len(normOld),
			UsedFuzzyMatch: true,
			ContentForReplacement: normContent,
		}
	}

	return MatchResult{Found: false}
}

// CountOccurrences counts how many times text appears in content.
func CountOccurrences(content, text string) int {
	if text == "" {
		return 0
	}
	count := 0
	idx := 0
	for {
		pos := strings.Index(content[idx:], text)
		if pos == -1 {
			break
		}
		count++
		idx += pos + len(text)
	}
	return count
}

// MatchedEdit holds a validated match with its position.
type MatchedEdit struct {
	Edit
	MatchIndex  int
	MatchLength int
	UsedFuzzy   bool
}

// ApplyEditsToNormalizedContent applies multiple edits to content.
// Edits are matched against the original content, not incrementally.
// Returns the modified content or an error.
func ApplyEditsToNormalizedContent(content string, edits []Edit, path string) (string, error) {
	// Validate no empty oldText
	for i, e := range edits {
		if e.OldText == "" {
			return "", fmt.Errorf("edit %d: oldText must not be empty", i+1)
		}
	}

	// Find all matches against original content
	matched := make([]MatchedEdit, len(edits))
	for i, e := range edits {
		result := FuzzyFindText(content, e.OldText)
		if !result.Found {
			return "", fmt.Errorf("could not find the exact text in %s. The text must match exactly (fuzzy matching is attempted for Unicode variants)", path)
		}

		// Check uniqueness
		occurrences := CountOccurrences(content, result.ContentForReplacement)
		if occurrences > 1 {
			return "", fmt.Errorf("found %d occurrences of the text in %s. The text must be unique. Please provide more context to make it unique", occurrences, path)
		}

		matched[i] = MatchedEdit{
			Edit:        e,
			MatchIndex:  result.Index,
			MatchLength: result.MatchLength,
			UsedFuzzy:   result.UsedFuzzyMatch,
		}
	}

	// Sort by position for overlap check
	sorted := make([]MatchedEdit, len(matched))
	copy(sorted, matched)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].MatchIndex < sorted[i].MatchIndex {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Check for overlapping edits
	for i := 1; i < len(sorted); i++ {
		prev := sorted[i-1]
		curr := sorted[i]
		if prev.MatchIndex+prev.MatchLength > curr.MatchIndex {
			return "", fmt.Errorf("edit at position %d overlaps with edit at position %d in %s", i, i+1, path)
		}
	}

	// Apply edits in reverse order to keep earlier offsets stable
	newContent := content
	for i := len(matched) - 1; i >= 0; i-- {
		m := matched[i]
		newContent = newContent[:m.MatchIndex] + m.NewText + newContent[m.MatchIndex+m.MatchLength:]
	}

	// No-change check
	if newContent == content {
		return "", fmt.Errorf("no changes made to %s. The new text is identical to the old text", path)
	}

	return newContent, nil
}

// GenerateDiffString produces a readable diff showing changed lines.
func GenerateDiffString(oldContent, newContent string) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// Find first changed line
	firstChanged := 0
	for firstChanged < len(oldLines) && firstChanged < len(newLines) {
		if oldLines[firstChanged] != newLines[firstChanged] {
			break
		}
		firstChanged++
	}

	if firstChanged >= len(oldLines) && firstChanged >= len(newLines) {
		return "" // identical
	}

	// Find last changed line (from end)
	lastOld := len(oldLines) - 1
	lastNew := len(newLines) - 1
	for lastOld >= firstChanged && lastNew >= firstChanged {
		if oldLines[lastOld] != newLines[lastNew] {
			break
		}
		lastOld--
		lastNew--
	}

	// Show context: 3 lines before and after
	ctxStart := firstChanged - 3
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEndOld := lastOld + 3
	if ctxEndOld > len(oldLines) {
		ctxEndOld = len(oldLines)
	}
	ctxEndNew := lastNew + 3
	if ctxEndNew > len(newLines) {
		ctxEndNew = len(newLines)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- changed at line %d\n", ctxStart+1))

	// Old lines (with - prefix)
	for i := ctxStart; i < ctxEndOld; i++ {
		b.WriteString("- ")
		b.WriteString(oldLines[i])
		b.WriteString("\n")
	}

	// New lines (with + prefix)
	for i := ctxStart; i < ctxEndNew; i++ {
		b.WriteString("+ ")
		b.WriteString(newLines[i])
		b.WriteString("\n")
	}

	return b.String()
}

// GenerateUnifiedPatch produces a standard unified diff patch.
func GenerateUnifiedPatch(path, oldContent, newContent string) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// Find first/last changed lines
	firstChanged := 0
	for firstChanged < len(oldLines) && firstChanged < len(newLines) {
		if oldLines[firstChanged] != newLines[firstChanged] {
			break
		}
		firstChanged++
	}

	lastOld := len(oldLines) - 1
	lastNew := len(newLines) - 1
	for lastOld >= firstChanged && lastNew >= firstChanged {
		if oldLines[lastOld] != newLines[lastNew] {
			break
		}
		lastOld--
		lastNew--
	}

	// Context
	ctxStart := firstChanged - 3
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEndOld := lastOld + 3
	if ctxEndOld > len(oldLines) {
		ctxEndOld = len(oldLines)
	}
	ctxEndNew := lastNew + 3
	if ctxEndNew > len(newLines) {
		ctxEndNew = len(newLines)
	}

	oldCount := len(oldLines) // number of old lines (for hunk header)
	_ = oldCount
	newCount := len(newLines) // number of new lines (for hunk header)
	_ = newCount

	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- a/%s\n", path))
	b.WriteString(fmt.Sprintf("+++ b/%s\n", path))

	// Hunk header
	oldStart := ctxStart + 1
	oldLen := ctxEndOld - ctxStart
	newStart := ctxStart + 1
	newLen := ctxEndNew - ctxStart
	b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldLen, newStart, newLen))

	// Context lines before changes
	maxCtx := ctxEndNew
	if ctxEndOld > maxCtx {
		maxCtx = ctxEndOld
	}

	// Unified diff: walk both old and new with shared context
	oi, ni := ctxStart, ctxStart
	for oi < ctxEndOld || ni < ctxEndNew {
		if oi < ctxEndOld && ni < ctxEndNew && oldLines[oi] == newLines[ni] {
			b.WriteString(" " + oldLines[oi] + "\n")
			oi++
			ni++
		} else {
			if oi < ctxEndOld {
				b.WriteString("-" + oldLines[oi] + "\n")
				oi++
			}
			if ni < ctxEndNew {
				b.WriteString("+" + newLines[ni] + "\n")
				ni++
			}
		}
	}

	return b.String()
}

// EditResult holds the result of an edit operation.
type EditResult struct {
	Content         string // old content (before edit)
	NewContent      string // new content (after edit)
	Diff            string
	Patch           string
	FirstChangedLine int
	EditsApplied    int
}

// ApplyEditToFile reads, edits, and writes a file.
// Handles BOM, line endings, fuzzy matching, and diff generation.
func ApplyEditToFile(path string, edits []Edit) (*EditResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)

	// Strip BOM
	text, bom := StripBom(content)

	// Detect and normalize line endings
	originalEnding := DetectLineEnding(text)
	normalized := NormalizeToLF(text)

	// Normalize edit strings to LF
	for i := range edits {
		edits[i].OldText = NormalizeToLF(edits[i].OldText)
		edits[i].NewText = NormalizeToLF(edits[i].NewText)
	}

	// Apply edits
	newContent, err := ApplyEditsToNormalizedContent(normalized, edits, path)
	if err != nil {
		return nil, err
	}

	// Restore line endings and BOM
	finalContent := bom + RestoreLineEndings(newContent, originalEnding)

	// Generate diff and patch
	diff := GenerateDiffString(normalized, newContent)
	patch := GenerateUnifiedPatch(path, normalized, newContent)

	// Find first changed line
	firstChangedLine := 1
	oldLines := strings.Split(normalized, "\n")
	newLines := strings.Split(newContent, "\n")
	for i := 0; i < len(oldLines) && i < len(newLines); i++ {
		if oldLines[i] != newLines[i] {
			firstChangedLine = i + 1
			break
		}
	}

	// Write file
	if err := os.WriteFile(path, []byte(finalContent), 0666); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	return &EditResult{
		Content:          content,
		NewContent:       finalContent,
		Diff:             diff,
		Patch:            patch,
		FirstChangedLine: firstChangedLine,
		EditsApplied:     len(edits),
	}, nil
}
