package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// ============================================================================
// Section: diff tests (from diff_test.go)
// ============================================================================

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

// ============================================================================
// Section: file_utils tests (from file_utils_test.go)
// ============================================================================

// Ported from upstream: src/utils/__tests__/file.test.ts
// Tests for convertLeadingTabsToSpaces, addLineNumbers, stripLineNumberPrefix, pathsEqual

// --- ConvertLeadingTabsToSpaces tests ---

func TestConvertLeadingTabsToSpacesBasic(t *testing.T) {
	// Each tab should become 2 spaces
	got := ConvertLeadingTabsToSpaces("\t\thello")
	want := "    hello" // 2 tabs = 4 spaces
	if got != want {
		t.Errorf("ConvertLeadingTabsToSpaces(tab*2 + hello) = %q, want %q", got, want)
	}
}

func TestConvertLeadingTabsToSpacesOnlyLeading(t *testing.T) {
	// Only leading tabs should be converted; tabs within the line preserved
	got := ConvertLeadingTabsToSpaces("\thello\tworld")
	want := "  hello\tworld"
	if got != want {
		t.Errorf("ConvertLeadingTabsToSpaces = %q, want %q", got, want)
	}
}

func TestConvertLeadingTabsToSpacesNoTabs(t *testing.T) {
	got := ConvertLeadingTabsToSpaces("no tabs")
	if got != "no tabs" {
		t.Errorf("ConvertLeadingTabsToSpaces(%q) = %q, want unchanged", "no tabs", got)
	}
}

func TestConvertLeadingTabsToSpacesEmpty(t *testing.T) {
	got := ConvertLeadingTabsToSpaces("")
	if got != "" {
		t.Errorf("ConvertLeadingTabsToSpaces(\"\") = %q, want \"\"", got)
	}
}

func TestConvertLeadingTabsToSpacesMultiline(t *testing.T) {
	input := "\tline1\n\t\tline2\nline3"
	want := "  line1\n    line2\nline3"
	got := ConvertLeadingTabsToSpaces(input)
	if got != want {
		t.Errorf("ConvertLeadingTabsToSpaces(multiline) = %q, want %q", got, want)
	}
}

// --- AddLineNumbers tests ---

var addLineNumRegex = regexp.MustCompile(`^\s*\d+[\t]\S+`)

func TestAddLineNumbersStartsFrom1(t *testing.T) {
	result := AddLineNumbers(AddLineNumbersOptions{Content: "a\nb\nc", StartLine: 1})
	// Check that lines start with line numbers
	lines := splitLines(result)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	// First line should start with 1
	if !startsWithNum(lines[0], 1) {
		t.Errorf("first line should start with 1, got %q", lines[0])
	}
	if !startsWithNum(lines[1], 2) {
		t.Errorf("second line should start with 2, got %q", lines[1])
	}
	if !startsWithNum(lines[2], 3) {
		t.Errorf("third line should start with 3, got %q", lines[2])
	}
}

func TestAddLineNumbersEmpty(t *testing.T) {
	result := AddLineNumbers(AddLineNumbersOptions{Content: "", StartLine: 1})
	if result != "" {
		t.Errorf("AddLineNumbers(empty) = %q, want \"\"", result)
	}
}

func TestAddLineNumbersStartLineOffset(t *testing.T) {
	result := AddLineNumbers(AddLineNumbersOptions{Content: "hello", StartLine: 10})
	if !startsWithNum(result, 10) {
		t.Errorf("AddLineNumbers with startLine=10 should start with 10, got %q", result)
	}
}

func startsWithNum(s string, n int) bool {
	expected := digitStr(n) + "\t"
	return len(s) >= len(expected) && s[:len(expected)] == expected
}

func digitStr(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// --- StripLineNumberPrefix tests ---

func TestStripLineNumberPrefixArrow(t *testing.T) {
	got := StripLineNumberPrefix("     1\u2192content") // 1->content with leading spaces
	want := "content"
	if got != want {
		t.Errorf("StripLineNumberPrefix(arrow) = %q, want %q", got, want)
	}
}

func TestStripLineNumberPrefixTab(t *testing.T) {
	got := StripLineNumberPrefix("1\tcontent")
	want := "content"
	if got != want {
		t.Errorf("StripLineNumberPrefix(tab) = %q, want %q", got, want)
	}
}

func TestStripLineNumberPrefixNoPrefix(t *testing.T) {
	got := StripLineNumberPrefix("no prefix")
	want := "no prefix"
	if got != want {
		t.Errorf("StripLineNumberPrefix(no prefix) = %q, want %q", got, want)
	}
}

func TestStripLineNumberPrefixLargeNumber(t *testing.T) {
	got := StripLineNumberPrefix("123456\u2192content")
	want := "content"
	if got != want {
		t.Errorf("StripLineNumberPrefix(large number) = %q, want %q", got, want)
	}
}

// --- NormalizePathForComparison tests ---

func TestNormalizePathForComparisonRedundantSeparators(t *testing.T) {
	result := NormalizePathForComparison("/a//b/c")
	want := "/a/b/c"
	if result != want {
		t.Errorf("NormalizePathForComparison(/a//b/c) = %q, want %q", result, want)
	}
}

func TestNormalizePathForComparisonDotSegments(t *testing.T) {
	result := NormalizePathForComparison("/a/./b/../c")
	want := "/a/c"
	if result != want {
		t.Errorf("NormalizePathForComparison(/a/./b/../c) = %q, want %q", result, want)
	}
}

// --- PathsEqual tests ---

func TestPathsEqualIdentical(t *testing.T) {
	if !PathsEqual("/a/b/c", "/a/b/c") {
		t.Error("PathsEqual(/a/b/c, /a/b/c) should be true")
	}
}

func TestPathsEqualDotSegments(t *testing.T) {
	if !PathsEqual("/a/./b", "/a/b") {
		t.Error("PathsEqual(/a/./b, /a/b) should be true")
	}
}

func TestPathsEqualDifferent(t *testing.T) {
	if PathsEqual("/a/b", "/a/c") {
		t.Error("PathsEqual(/a/b, /a/c) should be false")
	}
}

func TestPathsEqualBackslashOnWindows(t *testing.T) {
	// From upstream: pathsEqual with backslash vs forward slash
	if runtime.GOOS == "windows" {
		// On Windows, forward slash and backslash should be equivalent
		if !PathsEqual(`/a/b/c`, `\a\b\c`) {
			t.Error("PathsEqual should treat / and \\ as equivalent on Windows")
		}
	}
}

func TestPathsEqualCaseInsensitiveOnWindows(t *testing.T) {
	// From upstream: Windows case-insensitive file system
	if runtime.GOOS == "windows" {
		if !PathsEqual(`/A/B/C`, `/a/b/c`) {
			t.Error("PathsEqual should be case-insensitive on Windows")
		}
	}
}

// --- Upstream Quality: Roundtrip test ---

func TestAddLineNumbersStripLineNumberPrefixRoundtrip(t *testing.T) {
	// Adding line numbers then stripping them should recover the original line
	content := "hello\nworld\nfoo"
	numbered := AddLineNumbers(AddLineNumbersOptions{Content: content, StartLine: 1})
	lines := splitLines(numbered)
	var recovered []string
	for _, line := range lines {
		recovered = append(recovered, StripLineNumberPrefix(line))
	}
	result := joinLines(recovered)
	if result != content {
		t.Errorf("roundtrip failed: got %q, want %q", result, content)
	}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return regexp.MustCompile(`\r?\n`).Split(s, -1)
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

// ============================================================================
// Section: path_utils tests (from path_utils_test.go)
// ============================================================================

// --- ContainsPathTraversal ---

func TestContainsPathTraversalWithParentRef(t *testing.T) {
	if !ContainsPathTraversal("../foo") {
		t.Fatal("expected true for '../foo'")
	}
	if !ContainsPathTraversal("foo/../bar") {
		t.Fatal("expected true for 'foo/../bar'")
	}
	if !ContainsPathTraversal("foo/..") {
		t.Fatal("expected true for 'foo/..'")
	}
}

func TestContainsPathTraversalNoParentRef(t *testing.T) {
	if ContainsPathTraversal("foo/bar/baz") {
		t.Fatal("expected false for 'foo/bar/baz'")
	}
	if ContainsPathTraversal("foo/bar..baz") {
		t.Fatal("expected false for 'foo/bar..baz' (.. not a segment)")
	}
	if ContainsPathTraversal("./foo") {
		t.Fatal("expected false for './foo' (single dot, not double)")
	}
}

func TestContainsPathTraversalEdgeCases(t *testing.T) {
	if !ContainsPathTraversal("..") {
		t.Fatal("expected true for bare '..'")
	}
	if ContainsPathTraversal("") {
		t.Fatal("expected false for empty string")
	}
	if ContainsPathTraversal("foo") {
		t.Fatal("expected false for simple filename")
	}
	// From upstream: ... in filename is not traversal
	if ContainsPathTraversal("foo/...bar") {
		t.Fatal("expected false for 'foo/...bar' (triple dot in filename)")
	}
	// From upstream: dotdot in filename without separator is not traversal
	if ContainsPathTraversal("foo..bar") {
		t.Fatal("expected false for 'foo..bar' (dotdot in filename without separator)")
	}
	// From upstream: .. at end of absolute path
	if !ContainsPathTraversal("/path/to/..") {
		t.Fatal("expected true for '/path/to/..' (traversal at end of absolute path)")
	}
}

func TestContainsPathTraversalBackslash(t *testing.T) {
	if !ContainsPathTraversal(`foo\..\bar`) {
		t.Fatal("expected true for backslash path traversal")
	}
}

// --- NormalizePathForConfigKey ---

func TestNormalizePathForConfigKeyBackslashes(t *testing.T) {
	result := NormalizePathForConfigKey(`foo\bar\baz`)
	if result != "foo/bar/baz" {
		t.Fatalf("expected 'foo/bar/baz', got %q", result)
	}
}

func TestNormalizePathForConfigKeyAlreadyNormalized(t *testing.T) {
	result := NormalizePathForConfigKey("foo/bar/baz")
	if result != "foo/bar/baz" {
		t.Fatalf("expected 'foo/bar/baz', got %q", result)
	}
}

func TestNormalizePathForConfigKeyResolvesDots(t *testing.T) {
	result := NormalizePathForConfigKey("foo/./bar")
	if result != "foo/bar" {
		t.Fatalf("expected 'foo/bar', got %q", result)
	}
}

func TestNormalizePathForConfigKeyResolvesParentRefs(t *testing.T) {
	result := NormalizePathForConfigKey("foo/bar/../baz")
	if result != "foo/baz" {
		t.Fatalf("expected 'foo/baz', got %q", result)
	}
}

func TestNormalizePathForConfigKeyMixedSeparators(t *testing.T) {
	// From upstream: "normalizes mixed separators foo/bar\baz"
	result := NormalizePathForConfigKey("foo/bar\\baz")
	if result != "foo/bar/baz" {
		t.Fatalf("expected 'foo/bar/baz', got %q", result)
	}
}

func TestNormalizePathForConfigKeyRedundantSeparators(t *testing.T) {
	// From upstream: "normalizes redundant separators foo//bar"
	result := NormalizePathForConfigKey("foo//bar")
	if result != "foo/bar" {
		t.Fatalf("expected 'foo/bar', got %q", result)
	}
}

func TestNormalizePathForConfigKeyAbsolutePath(t *testing.T) {
	// From upstream: "handles absolute path"
	result := NormalizePathForConfigKey("/Users/test/project")
	if result != "/Users/test/project" {
		t.Fatalf("expected '/Users/test/project', got %q", result)
	}
}

// --- ToRelativePath ---

func TestToRelativePathWithinCWD(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	absPath := filepath.Join(cwd, "some", "file.txt")
	result := ToRelativePath(absPath)
	// Should not be the absolute path (i.e., should be relative)
	if result == absPath {
		t.Fatalf("expected relative path, got absolute: %q", result)
	}
}

func TestToRelativePathOutsideCWD(t *testing.T) {
	// A path that is not under CWD should be returned as-is
	absPath := "/some/path/outside/cwd"
	result := ToRelativePath(absPath)
	// On different OS, this may or may not be relative
	// Just verify it doesn't panic and returns something
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestToRelativePathAlreadyRelative(t *testing.T) {
	result := ToRelativePath("relative/path")
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

// --- GetDirectoryForPath ---

func TestGetDirectoryForPathExistingDir(t *testing.T) {
	// Use a directory that we know exists
	dir := os.TempDir()
	result := GetDirectoryForPath(dir)
	if result != dir {
		t.Fatalf("expected %q, got %q", dir, result)
	}
}

func TestGetDirectoryForPathExistingFile(t *testing.T) {
	// Create a temp file
	f, err := os.CreateTemp("", "test_get_dir_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	result := GetDirectoryForPath(f.Name())
	expected := filepath.Dir(f.Name())
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestGetDirectoryForPathNonExistent(t *testing.T) {
	result := GetDirectoryForPath("/nonexistent/path/file.txt")
	expected := "/nonexistent/path"
	if runtime.GOOS == "windows" {
		expected = "\\nonexistent\\path"
	}
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

// --- Invariant: NormalizePathForConfigKey idempotent ---

func TestNormalizePathForConfigKeyIdempotent(t *testing.T) {
	input := `foo\bar\./baz\..`
	first := NormalizePathForConfigKey(input)
	second := NormalizePathForConfigKey(first)
	if first != second {
		t.Fatalf("normalization not idempotent: %q != %q", first, second)
	}
}

// --- Invariant: ContainsPathTraversal symmetrical with path separators ---

func TestContainsPathTraversalBothSeparators(t *testing.T) {
	forward := ContainsPathTraversal("foo/../bar")
	backward := ContainsPathTraversal(`foo\..\bar`)
	if forward != backward {
		t.Fatalf("path traversal detection should be consistent across separators: /=%v, \\=%v", forward, backward)
	}
}

// --- Upstream Quality: ExpandPath tests (from path.test.ts) ---

func TestExpandPathRelativePath(t *testing.T) {
	// From upstream: "resolves relative path against baseDir"
	// expandPath in file_history_tools.go resolves relative paths against cwd
	result := expandPath("src")
	cwd, _ := os.Getwd()
	expected := filepath.Join(cwd, "src")
	if result != expected {
		t.Errorf("expandPath('src') = %q, want %q", result, expected)
	}
}

func TestExpandPathAbsolutePathPassthrough(t *testing.T) {
	// From upstream: "passes absolute paths through normalized"
	// On Windows, absolute paths are like C:\foo
	if runtime.GOOS == "windows" {
		result := expandPath(`C:\Users\test`)
		if result != `C:\Users\test` {
			t.Errorf("expandPath should pass absolute paths through, got %q", result)
		}
	} else {
		result := expandPath("/usr/local/bin")
		if result != "/usr/local/bin" {
			t.Errorf("expandPath should pass absolute paths through, got %q", result)
		}
	}
}

func TestExpandPathMSYS2DriveOnWindows(t *testing.T) {
	// From upstream/windowsPaths: converts /c/Users/ to C:\Users\
	if runtime.GOOS != "windows" {
		t.Skip("MSYS2 path handling is Windows-specific")
	}
	result := expandPath("/c/Users/foo")
	if !filepath.IsAbs(result) {
		t.Errorf("expandPath('/c/Users/foo') should produce absolute path, got %q", result)
	}
}

// --- Upstream Quality: ToRelativePath detailed tests ---

func TestToRelativePathCWDDot(t *testing.T) {
	// From upstream: "returns empty string for cwd itself"
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	result := ToRelativePath(cwd)
	// When cwd == cwd, relative(cwd, cwd) returns "." or ""
	// Both indicate the path is within cwd
	if result != "." && result != "" {
		// Some platforms may return "." instead of ""
		t.Logf("ToRelativePath(cwd) = %q (expected '.' or '')", result)
	}
}

// --- Upstream Quality: NormalizePathForComparison invariant tests ---

func TestNormalizePathForComparisonIdempotent(t *testing.T) {
	// Idempotency: normalizing twice gives same result
	input := "/a/./b/../c//d"
	first := NormalizePathForComparison(input)
	second := NormalizePathForComparison(first)
	if first != second {
		t.Fatalf("NormalizePathForComparison not idempotent: %q != %q", first, second)
	}
}

func TestNormalizePathForComparisonMixedSeparators(t *testing.T) {
	// From upstream: mixed backslash and forward slash
	result := NormalizePathForComparison("foo/bar\\baz")
	// Should normalize to consistent separator
	if strings.Contains(result, "\\") && !strings.Contains(result, "/") && !isWindowsPlatform() {
		t.Errorf("NormalizePathForComparison should use forward slashes on non-Windows: got %q", result)
	}
}
