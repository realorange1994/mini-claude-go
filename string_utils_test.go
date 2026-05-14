package main

import (
	"strings"
	"testing"
)

// ─── escapeRegExp ────────────────────────────────────────────────────────
// Ported from upstream stringUtils.test.ts

func TestEscapeRegExpBasic(t *testing.T) {
	result := escapeRegExp("hello")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestEscapeRegExpSpecialChars(t *testing.T) {
	result := escapeRegExp("^$")
	if result != `\^\$` {
		t.Errorf("expected escaped, got %q", result)
	}
}

func TestEscapeRegExpAllSpecials(t *testing.T) {
	input := `^${}()|[]\.*+?`
	expected := `\^\$\{\}\(\)\|\[\]\\\.\*\+\?`
	result := escapeRegExp(input)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestEscapeRegExpNoSpecialChars(t *testing.T) {
	result := escapeRegExp("abc123")
	if result != "abc123" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestEscapeRegExpMixed(t *testing.T) {
	result := escapeRegExp("price: $5.00")
	expected := `price: \$5\.00`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// Idempotency: escaping an already-escaped string should double-escape the backslash
func TestEscapeRegExpDoubleEscape(t *testing.T) {
	first := escapeRegExp("a.b")
	second := escapeRegExp(first)
	// First escape: "a.b" -> "a\\.b" (5 chars: a, \, ., b)
	// Second escape: each \ and . in "a\.b" gets escaped again
	// So: a, \\, \\, ., b = "a\\\\.b" (6 chars)
	expected := "a" + "\\" + "\\" + "\\" + "." + "b"
	if second != expected {
		t.Errorf("double escape: expected %q, got %q", expected, second)
	}
}

// ─── capitalize ──────────────────────────────────────────────────────────
// Ported from upstream stringUtils.test.ts

func TestCapitalizeBasic(t *testing.T) {
	if capitalize("hello") != "Hello" {
		t.Errorf("expected 'Hello', got %q", capitalize("hello"))
	}
}

func TestCapitalizeEmptyString(t *testing.T) {
	if capitalize("") != "" {
		t.Errorf("expected '', got %q", capitalize(""))
	}
}

func TestCapitalizeSingleChar(t *testing.T) {
	if capitalize("a") != "A" {
		t.Errorf("expected 'A', got %q", capitalize("a"))
	}
}

func TestCapitalizeAlreadyCapitalized(t *testing.T) {
	if capitalize("Hello") != "Hello" {
		t.Errorf("expected 'Hello', got %q", capitalize("Hello"))
	}
}

func TestCapitalizeOnlyFirstChar(t *testing.T) {
	// Unlike lodash, this should NOT lowercase remaining chars
	if capitalize("hELLO") != "HELLO" {
		t.Errorf("expected 'HELLO' (no lowercasing rest), got %q", capitalize("hELLO"))
	}
}

func TestCapitalizeUnicode(t *testing.T) {
	if capitalize("hello") != "Hello" {
		t.Errorf("expected 'Hello', got %q", capitalize("hello"))
	}
}

// ─── plural ──────────────────────────────────────────────────────────────
// Ported from upstream stringUtils.test.ts

func TestPluralSingular(t *testing.T) {
	if plural(1, "cat") != "cat" {
		t.Errorf("expected 'cat', got %q", plural(1, "cat"))
	}
}

func TestPluralPlural(t *testing.T) {
	if plural(2, "cat") != "cats" {
		t.Errorf("expected 'cats', got %q", plural(2, "cat"))
	}
}

func TestPluralZero(t *testing.T) {
	if plural(0, "cat") != "cats" {
		t.Errorf("expected 'cats' for zero, got %q", plural(0, "cat"))
	}
}

func TestPluralCustomPlural(t *testing.T) {
	if plural(2, "person", "people") != "people" {
		t.Errorf("expected 'people', got %q", plural(2, "person", "people"))
	}
}

func TestPluralCustomPluralSingular(t *testing.T) {
	if plural(1, "person", "people") != "person" {
		t.Errorf("expected 'person', got %q", plural(1, "person", "people"))
	}
}

func TestPluralNegative(t *testing.T) {
	if plural(-1, "cat") != "cats" {
		t.Errorf("expected 'cats' for negative, got %q", plural(-1, "cat"))
	}
}

// ─── firstLineOf ─────────────────────────────────────────────────────────
// Ported from upstream stringUtils.test.ts

func TestFirstLineOfSingleLine(t *testing.T) {
	if firstLineOf("hello") != "hello" {
		t.Errorf("expected 'hello', got %q", firstLineOf("hello"))
	}
}

func TestFirstLineOfMultiLine(t *testing.T) {
	if firstLineOf("hello\nworld") != "hello" {
		t.Errorf("expected 'hello', got %q", firstLineOf("hello\nworld"))
	}
}

func TestFirstLineOfEmptyString(t *testing.T) {
	if firstLineOf("") != "" {
		t.Errorf("expected '', got %q", firstLineOf(""))
	}
}

func TestFirstLineOfTrailingNewline(t *testing.T) {
	if firstLineOf("hello\n") != "hello" {
		t.Errorf("expected 'hello', got %q", firstLineOf("hello\n"))
	}
}

func TestFirstLineOfThreeLines(t *testing.T) {
	if firstLineOf("one\ntwo\nthree") != "one" {
		t.Errorf("expected 'one', got %q", firstLineOf("one\ntwo\nthree"))
	}
}

// Upstream: returns empty string for leading newline
func TestFirstLineOfLeadingNewline(t *testing.T) {
	if firstLineOf("\nline2") != "" {
		t.Errorf("expected '', got %q", firstLineOf("\nline2"))
	}
}

// ─── countCharInString ──────────────────────────────────────────────────
// Ported from upstream stringUtils.test.ts

func TestCountCharInStringBasic(t *testing.T) {
	if countCharInString("hello", "l") != 2 {
		t.Errorf("expected 2, got %d", countCharInString("hello", "l"))
	}
}

func TestCountCharInStringNotFound(t *testing.T) {
	if countCharInString("hello", "z") != 0 {
		t.Errorf("expected 0, got %d", countCharInString("hello", "z"))
	}
}

func TestCountCharInStringEmpty(t *testing.T) {
	if countCharInString("", "a") != 0 {
		t.Errorf("expected 0, got %d", countCharInString("", "a"))
	}
}

func TestCountCharInStringWithOffset(t *testing.T) {
	if countCharInString("hello", "l", 3) != 1 {
		t.Errorf("expected 1 with offset, got %d", countCharInString("hello", "l", 3))
	}
}

func TestCountCharInStringAllSame(t *testing.T) {
	if countCharInString("aaa", "a") != 3 {
		t.Errorf("expected 3, got %d", countCharInString("aaa", "a"))
	}
}

func TestCountCharInStringOffsetBeyond(t *testing.T) {
	if countCharInString("hi", "h", 10) != 0 {
		t.Errorf("expected 0 when offset beyond string, got %d", countCharInString("hi", "h", 10))
	}
}

// Upstream: counts from start offset (aabaa, 'a', offset 2)
func TestCountCharInStringUpstreamOffset(t *testing.T) {
	if countCharInString("aabaa", "a", 2) != 2 {
		t.Errorf("expected 2 for 'aabaa' from offset 2, got %d", countCharInString("aabaa", "a", 2))
	}
}

// Upstream: "hello world" count "l" = 3
func TestCountCharInStringUpstream(t *testing.T) {
	if countCharInString("hello world", "l") != 3 {
		t.Errorf("expected 3 for 'hello world' count 'l', got %d", countCharInString("hello world", "l"))
	}
}

// ─── normalizeFullWidthDigits ────────────────────────────────────────────
// Ported from upstream stringUtils.test.ts

func TestNormalizeFullWidthDigitsBasic(t *testing.T) {
	result := normalizeFullWidthDigits("\uFF10\uFF11\uFF12") // ０１２
	if result != "012" {
		t.Errorf("expected '012', got %q", result)
	}
}

func TestNormalizeFullWidthDigitsMixed(t *testing.T) {
	result := normalizeFullWidthDigits("abc\uFF13def") // ３
	if result != "abc3def" {
		t.Errorf("expected 'abc3def', got %q", result)
	}
}

func TestNormalizeFullWidthDigitsNoFullWidth(t *testing.T) {
	result := normalizeFullWidthDigits("0123456789")
	if result != "0123456789" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestNormalizeFullWidthDigitsEmpty(t *testing.T) {
	result := normalizeFullWidthDigits("")
	if result != "" {
		t.Errorf("expected '', got %q", result)
	}
}

func TestNormalizeFullWidthDigitsIdempotent(t *testing.T) {
	input := "0123456789"
	result := normalizeFullWidthDigits(normalizeFullWidthDigits(input))
	if result != input {
		t.Errorf("idempotency: expected %q, got %q", input, result)
	}
}

// ─── normalizeFullWidthSpace ─────────────────────────────────────────────
// Ported from upstream stringUtils.test.ts

func TestNormalizeFullWidthSpaceBasic(t *testing.T) {
	result := normalizeFullWidthSpace("hello\u3000world") // ideographic space
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestNormalizeFullWidthSpaceMultiple(t *testing.T) {
	result := normalizeFullWidthSpace("\u3000\u3000")
	if result != "  " {
		t.Errorf("expected '  ', got %q", result)
	}
}

func TestNormalizeFullWidthSpaceNoFullWidth(t *testing.T) {
	result := normalizeFullWidthSpace("hello world")
	if result != "hello world" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestNormalizeFullWidthSpaceEmpty(t *testing.T) {
	result := normalizeFullWidthSpace("")
	if result != "" {
		t.Errorf("expected '', got %q", result)
	}
}

// ─── truncateToLines ─────────────────────────────────────────────────────
// Ported from upstream stringUtils.test.ts

func TestTruncateToLinesShortEnough(t *testing.T) {
	result := truncateToLines("a\nb\nc", 5)
	if result != "a\nb\nc" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestTruncateToLinesTruncates(t *testing.T) {
	result := truncateToLines("a\nb\nc\nd\ne", 3)
	if !strings.HasSuffix(result, "\u2026") {
		t.Errorf("expected ellipsis suffix, got %q", result)
	}
	if !strings.HasPrefix(result, "a\nb\nc") {
		t.Errorf("expected first 3 lines, got %q", result)
	}
}

func TestTruncateToLinesExactLines(t *testing.T) {
	result := truncateToLines("a\nb\nc", 3)
	if result != "a\nb\nc" {
		t.Errorf("expected unchanged (exact fit), got %q", result)
	}
}

func TestTruncateToLinesSingleLine(t *testing.T) {
	result := truncateToLines("hello", 1)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncateToLinesZeroMaxLines(t *testing.T) {
	result := truncateToLines("a\nb", 0)
	if result != "\u2026" {
		t.Errorf("expected just ellipsis, got %q", result)
	}
}

// Upstream: preserves intentional empty lines between content
func TestSafeJoinLinesPreservesIntentionalEmptyLines(t *testing.T) {
	result := safeJoinLines([]string{"first", "", "second", "", "third"}, "\n", 100)
	if result != "first\n\nsecond\n\nthird" {
		t.Errorf("expected intentional empty lines preserved, got %q", result)
	}
}

// Upstream: handles string with only newlines (using \n delimiter to match upstream)
func TestSafeJoinLinesOnlyNewlines(t *testing.T) {
	result := safeJoinLines([]string{"\n", "\n"}, "\n", 100)
	if result != "\n\n\n" {
		t.Errorf("expected \\n\\n\\n, got %q", result)
	}
}

// Upstream: single empty line
func TestSafeJoinLinesSingleEmpty(t *testing.T) {
	result := safeJoinLines([]string{""}, ",", 100)
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}
// Ported from upstream stringUtils.test.ts

func TestSafeJoinLinesBasic(t *testing.T) {
	result := safeJoinLines([]string{"a", "b", "c"}, ", ", 100)
	if result != "a, b, c" {
		t.Errorf("expected 'a, b, c', got %q", result)
	}
}

func TestSafeJoinLinesSingleElement(t *testing.T) {
	result := safeJoinLines([]string{"only"}, ", ", 100)
	if result != "only" {
		t.Errorf("expected 'only', got %q", result)
	}
}

func TestSafeJoinLinesEmpty(t *testing.T) {
	result := safeJoinLines(nil, ", ", 100)
	if result != "" {
		t.Errorf("expected '', got %q", result)
	}
}

func TestSafeJoinLinesTruncates(t *testing.T) {
	result := safeJoinLines([]string{"a", "b", "c"}, ", ", 5)
	if !strings.Contains(result, "truncated") {
		t.Errorf("expected truncation marker, got %q", result)
	}
}

// ─── EndTruncatingAccumulator ────────────────────────────────────────────
// Ported from upstream stringUtils.test.ts

func TestEndTruncatingAccumulatorUnderLimit(t *testing.T) {
	acc := NewEndTruncatingAccumulator(100)
	acc.Append("hello")
	if acc.Truncated() {
		t.Error("should not be truncated")
	}
	if acc.String() != "hello" {
		t.Errorf("expected 'hello', got %q", acc.String())
	}
}

func TestEndTruncatingAccumulatorAtLimit(t *testing.T) {
	acc := NewEndTruncatingAccumulator(5)
	acc.Append("hello")
	if acc.Truncated() {
		t.Error("should not be truncated at exactly limit")
	}
	if acc.String() != "hello" {
		t.Errorf("expected 'hello', got %q", acc.String())
	}
}

func TestEndTruncatingAccumulatorOverLimit(t *testing.T) {
	acc := NewEndTruncatingAccumulator(5)
	acc.Append("hello world")
	if !acc.Truncated() {
		t.Error("should be truncated when over limit")
	}
	// 11 total - 5 maxSize = 6 bytes removed, 6/1024 = 0KB
	if acc.String() != "hello\n... [output truncated - 0KB removed]" {
		t.Errorf("expected truncated output, got %q", acc.String())
	}
}

func TestEndTruncatingAccumulatorMultipleAppends(t *testing.T) {
	acc := NewEndTruncatingAccumulator(10)
	acc.Append("12345")
	acc.Append("67890")
	if acc.Truncated() {
		t.Error("should not be truncated (10 bytes exact)")
	}
}

func TestEndTruncatingAccumulatorMultipleAppendsOver(t *testing.T) {
	acc := NewEndTruncatingAccumulator(7)
	acc.Append("12345")
	acc.Append("67890")
	if !acc.Truncated() {
		t.Error("should be truncated")
	}
	// Should contain first 7 bytes
	if !strings.HasPrefix(acc.String(), "1234567") {
		t.Errorf("expected prefix '1234567', got %q", acc.String())
	}
}

func TestEndTruncatingAccumulatorClear(t *testing.T) {
	acc := NewEndTruncatingAccumulator(5)
	acc.Append("hello world")
	acc.Clear()
	if acc.Truncated() {
		t.Error("should not be truncated after clear")
	}
	if acc.Length() != 0 {
		t.Errorf("expected 0 length after clear, got %d", acc.Length())
	}
	if acc.String() != "" {
		t.Errorf("expected '' after clear, got %q", acc.String())
	}
}

func TestEndTruncatingAccumulatorTotalBytes(t *testing.T) {
	acc := NewEndTruncatingAccumulator(5)
	acc.Append("hello world")
	if acc.TotalBytes() != 11 {
		t.Errorf("expected 11 total bytes, got %d", acc.TotalBytes())
	}
}

// Upstream: stops accepting data once truncated and full
func TestEndTruncatingAccumulatorStopsAfterFull(t *testing.T) {
	acc := NewEndTruncatingAccumulator(5)
	acc.Append("12345")
	acc.Append("67890")
	if acc.Length() != 5 {
		t.Errorf("expected length 5 after first truncation, got %d", acc.Length())
	}
	acc.Append("more")
	if acc.Length() != 5 {
		t.Errorf("expected length still 5 after additional append, got %d", acc.Length())
	}
}

// Upstream: truncate flag set after exceeding maxSize with large single append
func TestEndTruncatingAccumulatorTruncateFlagOnLargeAppend(t *testing.T) {
	acc := NewEndTruncatingAccumulator(10)
	acc.Append("12345678901234567890")
	if !acc.Truncated() {
		t.Error("should be truncated after large append")
	}
	if acc.Length() != 10 {
		t.Errorf("expected length 10 (maxSize), got %d", acc.Length())
	}
}
