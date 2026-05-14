package main

import (
	"regexp"
	"runtime"
	"testing"
)

// Ported from upstream: src/utils/__tests__/file.test.ts
// Tests for convertLeadingTabsToSpaces, addLineNumbers, stripLineNumberPrefix, pathsEqual

// ─── ConvertLeadingTabsToSpaces tests ─────────────────────────────────────

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

// ─── AddLineNumbers tests ─────────────────────────────────────────────────

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

// ─── StripLineNumberPrefix tests ─────────────────────────────────────────

func TestStripLineNumberPrefixArrow(t *testing.T) {
	got := StripLineNumberPrefix("     1\u2192content") // 1→content with leading spaces
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

// ─── NormalizePathForComparison tests ────────────────────────────────────

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

// ─── PathsEqual tests ───────────────────────────────────────────────────

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

// ─── Upstream Quality: Roundtrip test ───────────────────────────────────

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
