package main

import (
	"strings"
	"testing"
)

// TestDiffIdenticalContent returns empty diff when old and new are the same.
func TestDiffIdenticalContent(t *testing.T) {
	result := StructuredDiff("same content", "same content", "test.txt")
	if result != "" {
		t.Errorf("expected empty diff for identical content, got:\n%s", result)
	}
}

// TestDiffEmptyOldContent (new file) produces diff with only + lines.
func TestDiffEmptyOldContent(t *testing.T) {
	result := StructuredDiff("", "new content", "test.txt")
	if result == "" {
		t.Fatal("expected non-empty diff when old content is empty")
	}
	if !strings.Contains(result, "+new content") {
		t.Errorf("expected + line with new content, got:\n%s", result)
	}
	// Check there are no content removal lines (lines starting with "-" but not "---")
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "---") {
			t.Errorf("expected no content removal lines when old content is empty, found: %s", trimmed)
		}
	}
}

// TestDiffEmptyNewContent (deleted file) produces diff with only - lines.
func TestDiffEmptyNewContent(t *testing.T) {
	result := StructuredDiff("line1\nline2\nline3", "", "test.txt")
	if result == "" {
		t.Fatal("expected non-empty diff when new content is empty")
	}
	// Check there are no content addition lines (lines starting with "+" but not "+++")
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "+") && !strings.HasPrefix(trimmed, "+++") {
			t.Errorf("expected no content addition lines when new content is empty, found: %s", trimmed)
		}
	}
}

// TestDiffSpecialCharactersAmpersands handles ampersands correctly.
func TestDiffSpecialCharactersAmpersands(t *testing.T) {
	result := StructuredDiff("a & b", "a & c", "test.txt")
	if result == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(result, "&") {
		t.Errorf("expected ampersands preserved in output, got:\n%s", result)
	}
}

// TestDiffSpecialCharactersDollarSigns handles dollar signs correctly.
func TestDiffSpecialCharactersDollarSigns(t *testing.T) {
	result := StructuredDiff("price: $5", "price: $10", "test.txt")
	if result == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(result, "$") {
		t.Errorf("expected dollar signs preserved in output, got:\n%s", result)
	}
}

// TestDiffSpecialCharacters verifies various special characters survive round-trip.
func TestDiffSpecialCharacters(t *testing.T) {
	specialChars := []string{
		"hello <world>",
		"path\\to\\file",
		"quotes 'single' and \"double\"",
		"backtick `code`",
		"star * glob ?",
	}
	for _, old := range specialChars {
		new := old + "_changed"
		result := StructuredDiff(old, new, "special.txt")
		if result == "" {
			t.Errorf("expected diff for %q -> %q", old, new)
			continue
		}
		if !strings.Contains(result, old) {
			t.Errorf("original content %q not found in diff output:\n%s", old, result)
		}
		if !strings.Contains(result, new) {
			t.Errorf("new content %q not found in diff output:\n%s", new, result)
		}
	}
}

// TestDiffUnicodeContent handles unicode characters.
func TestDiffUnicodeContent(t *testing.T) {
	unicodeStrings := []struct {
		old, new string
	}{
		{"Hello \u4e16\u754c", "Hello \u4e16\u754c!"},
		{"\u00e9\u00e0\u00fc\u00f6", "\u00e9\u00e0\u00fc\u00f6\u00df"},
		{"\U0001f600", "\U0001f600\U0001f601"},
		{"\u2014em dash", "\u2014em dash!"},
	}
	for _, tc := range unicodeStrings {
		result := StructuredDiff(tc.old, tc.new, "unicode.txt")
		if result == "" {
			t.Errorf("expected diff for unicode change: %q -> %q", tc.old, tc.new)
			continue
		}
		// Should contain the new unicode string
		if !strings.Contains(result, tc.new) {
			t.Errorf("new unicode content %q not found in diff:\n%s", tc.new, result)
		}
	}
}

// TestDiffDeterministic verifies same inputs produce same diff content
// (after normalizing temp paths and git warnings).
// NOTE: On Windows, git diff output includes temp paths that differ per invocation,
// so we normalize the diff body (removing headers) and compare only the change lines.
func TestDiffDeterministic(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nline2_changed\nline3\n"

	// Extract only the +/- change lines (skip headers and warnings)
	extractChanges := func(s string) []string {
		s = stripGitWarnings(s)
		lines := strings.Split(s, "\n")
		var changes []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "---") {
				changes = append(changes, trimmed)
			}
			if strings.HasPrefix(trimmed, "+") && !strings.HasPrefix(trimmed, "+++") {
				changes = append(changes, trimmed)
			}
		}
		return changes
	}

	c1 := extractChanges(StructuredDiff(old, new, "deterministic.txt"))
	c2 := extractChanges(StructuredDiff(old, new, "deterministic.txt"))
	c3 := extractChanges(StructuredDiff(old, new, "deterministic.txt"))

	if len(c1) == 0 {
		t.Fatal("expected at least one change line")
	}
	for i := range c1 {
		if c1[i] != c2[i] || c2[i] != c3[i] {
			t.Errorf("change lines not deterministic: c1=%v, c2=%v, c3=%v", c1, c2, c3)
		}
	}
}

// TestDiffSingleLineChange produces a diff with one - and one + line.
func TestDiffSingleLineChange(t *testing.T) {
	old := "line1\nline2\nline3"
	new := "line1\nline2_changed\nline3"

	result := StructuredDiff(old, new, "single.txt")
	if result == "" {
		t.Fatal("expected non-empty diff for single line change")
	}
	if !strings.Contains(result, "-line2") {
		t.Errorf("expected -line2 in diff, got:\n%s", result)
	}
	if !strings.Contains(result, "+line2_changed") {
		t.Errorf("expected +line2_changed in diff, got:\n%s", result)
	}
}

// TestDiffMultiLineChange handles changes spanning multiple lines.
func TestDiffMultiLineChange(t *testing.T) {
	old := "header\nold_a\nold_b\nold_c\nfooter"
	new := "header\nnew_a\nnew_b\nnew_c\nfooter"

	result := StructuredDiff(old, new, "multi.txt")
	if result == "" {
		t.Fatal("expected non-empty diff for multi-line change")
	}
	for _, removed := range []string{"old_a", "old_b", "old_c"} {
		if !strings.Contains(result, "-"+removed) {
			t.Errorf("expected -%s in diff, got:\n%s", removed, result)
		}
	}
	for _, added := range []string{"new_a", "new_b", "new_c"} {
		if !strings.Contains(result, "+"+added) {
			t.Errorf("expected +%s in diff, got:\n%s", added, result)
		}
	}
}

// TestDiffLargeContent handles large content diffs without errors.
func TestDiffLargeContent(t *testing.T) {
	var oldBuilder, newBuilder strings.Builder
	for i := 0; i < 1000; i++ {
		digit := string(rune('0' + i%10))
		oldBuilder.WriteString("line_" + digit + "\n")
		if i%2 == 0 {
			newBuilder.WriteString("line_" + digit + "\n")
		} else {
			newBuilder.WriteString("modified_line_" + digit + "\n")
		}
	}
	old := oldBuilder.String()
	new := newBuilder.String()

	result := StructuredDiff(old, new, "large.txt")
	if result == "" {
		t.Fatal("expected non-empty diff for large content")
	}
	// Should contain changes (every odd line was modified)
	if !strings.Contains(result, "-line_") {
		t.Error("expected -line_ removals in large diff")
	}
	if !strings.Contains(result, "+modified_line_") {
		t.Error("expected +modified_line_ additions in large diff")
	}
}

// TestDiffIdenticalContentNoChangeLines verifies identical content produces no +/- lines.
func TestDiffIdenticalContentNoChangeLines(t *testing.T) {
	// Even for multi-line identical content
	multiLine := "alpha\nbeta\ngamma\ndelta\n"
	result := StructuredDiff(multiLine, multiLine, "identical.txt")
	// On Windows, git may emit warnings to stderr that get included;
	// the actual diff body should be empty.
	body := stripGitWarnings(result)
	if body != "" {
		t.Errorf("expected empty diff body for identical multi-line content, got:\n%s", body)
	}
}

// TestDiffFilePathInHeader verifies that the diff output includes header lines.
func TestDiffFilePathInHeader(t *testing.T) {
	result := StructuredDiff("old", "new", "my/file.txt")
	if result == "" {
		t.Fatal("expected non-empty diff")
	}
	// The diff output should contain --- and +++ header lines
	if !strings.Contains(result, "---") {
		t.Errorf("expected --- header in diff output, got:\n%s", result)
	}
	if !strings.Contains(result, "+++") {
		t.Errorf("expected +++ header in diff output, got:\n%s", result)
	}
}

// TestDiffLineAdded tests when new content has more lines than old.
func TestDiffLineAdded(t *testing.T) {
	old := "line1\nline2"
	new := "line1\nline2\nline3\nline4"

	result := StructuredDiff(old, new, "added.txt")
	if result == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(result, "+line3") {
		t.Errorf("expected +line3 in diff, got:\n%s", result)
	}
	if !strings.Contains(result, "+line4") {
		t.Errorf("expected +line4 in diff, got:\n%s", result)
	}
}

// TestDiffLineRemoved tests when new content has fewer lines than old.
func TestDiffLineRemoved(t *testing.T) {
	old := "line1\nline2\nline3\nline4"
	new := "line1\nline2"

	result := StructuredDiff(old, new, "removed.txt")
	if result == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(result, "-line3") {
		t.Errorf("expected -line3 in diff, got:\n%s", result)
	}
	if !strings.Contains(result, "-line4") {
		t.Errorf("expected -line4 in diff, got:\n%s", result)
	}
}

// TestDiffEmptyBothContents returns empty diff when both are empty.
func TestDiffEmptyBothContents(t *testing.T) {
	result := StructuredDiff("", "", "empty.txt")
	if result != "" {
		t.Errorf("expected empty diff when both contents are empty, got:\n%s", result)
	}
}

// TestDiffNewlinesPreserved verifies trailing newline handling.
func TestDiffNewlinesPreserved(t *testing.T) {
	old := "line1\nline2\n"
	new := "line1\nline2\n"

	result := StructuredDiff(old, new, "newline.txt")
	// On Windows git may emit LF->CRLF warnings; the diff body should be empty.
	body := stripGitWarnings(result)
	if body != "" {
		t.Errorf("expected empty diff body for identical content with trailing newline, got:\n%s", body)
	}
}

// stripGitWarnings removes git warning lines from diff output.
func stripGitWarnings(s string) string {
	lines := strings.Split(s, "\n")
	var filtered []string
	for _, line := range lines {
		if !strings.HasPrefix(line, "warning:") {
			filtered = append(filtered, line)
		}
	}
	result := strings.Join(filtered, "\n")
	return strings.TrimSpace(result)
}

// normalizeDiffBody removes git warnings, temp paths, and header lines
// to leave only the meaningful diff content for comparison.
func normalizeDiffBody(s string) string {
	s = stripGitWarnings(s)
	lines := strings.Split(s, "\n")
	var filtered []string
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			continue
		}
		if strings.HasPrefix(line, "index ") {
			continue
		}
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

// TestDiffIdenticalNoChangeLinesInvariant verifies that diffing identical content
// never produces +/- change lines in the output body.
func TestDiffIdenticalNoChangeLinesInvariant(t *testing.T) {
	contents := []string{
		"single line",
		"multi\nline\ncontent",
		"trailing newline\n",
		"",
	}
	for _, content := range contents {
		result := StructuredDiff(content, content, "invariant.txt")
		body := normalizeDiffBody(result)
		lines := strings.Split(body, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "+") {
				t.Errorf("identical content %q should not produce change lines, found: %q in body:\n%s", content, line, body)
			}
		}
	}
}
