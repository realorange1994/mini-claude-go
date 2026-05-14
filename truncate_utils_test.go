package main

import (
	"strings"
	"testing"
)

// ─── stringWidth ─────────────────────────────────────────────────────────
// Ported from upstream truncate.test.ts (indirectly via truncateToWidth)

func TestStringWidthASCII(t *testing.T) {
	if stringWidth("hello") != 5 {
		t.Errorf("expected width 5, got %d", stringWidth("hello"))
	}
}

func TestStringWidthCJK(t *testing.T) {
	if stringWidth("你好") != 4 { // 2 CJK chars x 2 columns
		t.Errorf("expected width 4, got %d", stringWidth("你好"))
	}
}

func TestStringWidthEmpty(t *testing.T) {
	if stringWidth("") != 0 {
		t.Errorf("expected width 0, got %d", stringWidth(""))
	}
}

func TestStringWidthMixed(t *testing.T) {
	// "hi你好" = 2 + 4 = 6
	if stringWidth("hi\u4f60\u597d") != 6 {
		t.Errorf("expected width 6, got %d", stringWidth("hi\u4f60\u597d"))
	}
}

// ─── truncateToWidth ────────────────────────────────────────────────────
// Ported from upstream truncate.test.ts

func TestTruncateToWidthShortEnough(t *testing.T) {
	result := truncateToWidth("hello", 10)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncateToWidthTruncates(t *testing.T) {
	result := truncateToWidth("hello world", 8)
	if !strings.HasSuffix(result, "\u2026") {
		t.Errorf("expected ellipsis, got %q", result)
	}
	if result != "hello w\u2026" {
		t.Errorf("expected 'hello w…', got %q", result)
	}
}

func TestTruncateToWidthExactWidth(t *testing.T) {
	result := truncateToWidth("hello", 5)
	if result != "hello" {
		t.Errorf("expected 'hello' (exact fit), got %q", result)
	}
}

func TestTruncateToWidthCJK(t *testing.T) {
	result := truncateToWidth("你好世界", 5)
	if !strings.HasSuffix(result, "\u2026") {
		t.Errorf("expected ellipsis, got %q", result)
	}
}

func TestTruncateToWidthZeroWidth(t *testing.T) {
	if truncateToWidth("abc", 0) != "\u2026" {
		t.Errorf("expected ellipsis for width 0, got %q", truncateToWidth("abc", 0))
	}
}

func TestTruncateToWidthOneChar(t *testing.T) {
	if truncateToWidth("abc", 1) != "\u2026" {
		t.Errorf("expected ellipsis for width 1, got %q", truncateToWidth("abc", 1))
	}
}

func TestTruncateToWidthEmptyString(t *testing.T) {
	if truncateToWidth("", 5) != "" {
		t.Errorf("expected '' for empty input, got %q", truncateToWidth("", 5))
	}
}

func TestTruncateToWidthMixedCJK(t *testing.T) {
	// "ab你好cd" has width 2+4+2=8, truncate to 5
	result := truncateToWidth("ab\u4f60\u597dcd", 5)
	if !strings.HasSuffix(result, "\u2026") {
		t.Errorf("expected ellipsis, got %q", result)
	}
}

// ─── truncateStartToWidth ────────────────────────────────────────────────
// Ported from upstream truncate.test.ts

func TestTruncateStartToWidthShortEnough(t *testing.T) {
	result := truncateStartToWidth("hello", 10)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncateStartToWidthTruncates(t *testing.T) {
	result := truncateStartToWidth("hello world", 8)
	if !strings.HasPrefix(result, "\u2026") {
		t.Errorf("expected leading ellipsis, got %q", result)
	}
	if result != "\u2026o world" {
		t.Errorf("expected '…o world', got %q", result)
	}
}

func TestTruncateStartToWidthExactWidth(t *testing.T) {
	result := truncateStartToWidth("hello", 5)
	if result != "hello" {
		t.Errorf("expected 'hello' (exact fit), got %q", result)
	}
}

func TestTruncateStartToWidthCJK(t *testing.T) {
	result := truncateStartToWidth("你好世界", 5)
	if !strings.HasPrefix(result, "\u2026") {
		t.Errorf("expected leading ellipsis, got %q", result)
	}
}

func TestTruncateStartToWidthEmpty(t *testing.T) {
	result := truncateStartToWidth("", 5)
	if result != "" {
		t.Errorf("expected '' for empty input, got %q", result)
	}
}

// ─── truncateToWidthNoEllipsis ───────────────────────────────────────────
// Ported from upstream truncate.test.ts

func TestTruncateToWidthNoEllipsisShortEnough(t *testing.T) {
	result := truncateToWidthNoEllipsis("hello", 10)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncateToWidthNoEllipsisTruncates(t *testing.T) {
	result := truncateToWidthNoEllipsis("hello world", 5)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
	if strings.Contains(result, "\u2026") {
		t.Errorf("should not contain ellipsis, got %q", result)
	}
}

func TestTruncateToWidthNoEllipsisZero(t *testing.T) {
	result := truncateToWidthNoEllipsis("abc", 0)
	if result != "" {
		t.Errorf("expected '' for width 0, got %q", result)
	}
}

func TestTruncateToWidthNoEllipsisEmpty(t *testing.T) {
	result := truncateToWidthNoEllipsis("", 5)
	if result != "" {
		t.Errorf("expected '' for empty input, got %q", result)
	}
}

// ─── truncatePathMiddle ──────────────────────────────────────────────────
// Ported from upstream truncate.test.ts

func TestTruncatePathMiddleShortEnough(t *testing.T) {
	result := truncatePathMiddle("/usr/local/bin/file.txt", 100)
	if result != "/usr/local/bin/file.txt" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestTruncatePathMiddleTruncates(t *testing.T) {
	result := truncatePathMiddle("/very/long/path/to/some/deep/directory/file.txt", 30)
	if !strings.Contains(result, "\u2026") {
		t.Errorf("expected middle ellipsis, got %q", result)
	}
	if !strings.Contains(result, "file.txt") {
		t.Errorf("expected filename preserved, got %q", result)
	}
}

func TestTruncatePathMiddleFilenameOnly(t *testing.T) {
	result := truncatePathMiddle("verylongfilename.txt", 15)
	// Should still truncate, possibly from start
	if stringWidth(result) > 15 {
		t.Errorf("result wider than maxWidth: %q (width %d)", result, stringWidth(result))
	}
}

func TestTruncatePathMiddleEmpty(t *testing.T) {
	result := truncatePathMiddle("", 10)
	if result != "" {
		t.Errorf("expected '' for empty path, got %q", result)
	}
}

// ─── truncateStringPorted ────────────────────────────────────────────────
// Ported from upstream truncate.test.ts

func TestTruncateStringPortedBasic(t *testing.T) {
	result := truncateStringPorted("hello world", 8)
	if !strings.HasSuffix(result, "\u2026") {
		t.Errorf("expected ellipsis, got %q", result)
	}
}

func TestTruncateStringPortedShortEnough(t *testing.T) {
	result := truncateStringPorted("hi", 10)
	if result != "hi" {
		t.Errorf("expected 'hi', got %q", result)
	}
}

func TestTruncateStringPortedSingleLine(t *testing.T) {
	result := truncateStringPorted("hello\nworld", 20, true)
	if strings.Contains(result, "\n") {
		t.Errorf("singleLine should remove newline, got %q", result)
	}
	if !strings.Contains(result, "\u2026") {
		t.Errorf("singleLine should add ellipsis, got %q", result)
	}
}

func TestTruncateStringPortedSingleLineShortFirstLine(t *testing.T) {
	result := truncateStringPorted("hi\nlonger content here", 10, true)
	if strings.Contains(result, "\n") {
		t.Errorf("singleLine should remove newline, got %q", result)
	}
}

// ─── wrapText ────────────────────────────────────────────────────────────
// Ported from upstream truncate.test.ts

func TestWrapTextBasic(t *testing.T) {
	result := wrapText("hello world", 6)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(result))
	}
	if result[0] != "hello " {
		t.Errorf("expected 'hello ', got %q", result[0])
	}
	if result[1] != "world" {
		t.Errorf("expected 'world', got %q", result[1])
	}
}

func TestWrapTextShortEnough(t *testing.T) {
	result := wrapText("hi", 10)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d", len(result))
	}
	if result[0] != "hi" {
		t.Errorf("expected 'hi', got %q", result[0])
	}
}

func TestWrapTextEmpty(t *testing.T) {
	result := wrapText("", 10)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

// ─── Invariants ──────────────────────────────────────────────────────────

func TestTruncateToWidthThenStartToWidthInvariant(t *testing.T) {
	// truncateToWidth(x, w) should have width <= w
	// truncateStartToWidth(x, w) should have width <= w
	for _, s := range []string{"hello world", "你好世界", "mix你好ed"} {
		for _, w := range []int{1, 3, 5, 10, 20} {
			result1 := truncateToWidth(s, w)
			result2 := truncateStartToWidth(s, w)
			if stringWidth(result1) > w {
				t.Errorf("truncateToWidth(%q, %d) = %q (width %d) exceeds max", s, w, result1, stringWidth(result1))
			}
			if stringWidth(result2) > w {
				t.Errorf("truncateStartToWidth(%q, %d) = %q (width %d) exceeds max", s, w, result2, stringWidth(result2))
			}
		}
	}
}

func TestTruncateToWidthNoEllipsisWidthInvariant(t *testing.T) {
	for _, s := range []string{"hello world", "你好世界"} {
		for _, w := range []int{0, 1, 3, 5, 10} {
			result := truncateToWidthNoEllipsis(s, w)
			if stringWidth(result) > w {
				t.Errorf("truncateToWidthNoEllipsis(%q, %d) = %q (width %d) exceeds max", s, w, result, stringWidth(result))
			}
		}
	}
}

func TestTruncateToWidthIdempotent(t *testing.T) {
	// Truncating an already-truncated string should not change it
	input := "hello world this is a long string"
	truncated := truncateToWidth(input, 10)
	doubleTruncated := truncateToWidth(truncated, 10)
	if truncated != doubleTruncated {
		t.Errorf("idempotency: %q != %q", truncated, doubleTruncated)
	}
}

func TestTruncateStartToWidthIdempotent(t *testing.T) {
	input := "hello world this is a long string"
	truncated := truncateStartToWidth(input, 10)
	doubleTruncated := truncateStartToWidth(truncated, 10)
	if truncated != doubleTruncated {
		t.Errorf("idempotency: %q != %q", truncated, doubleTruncated)
	}
}
