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

// ─── Ported from upstream truncate.test.ts: CJK/emoji boundary tests ───────

func TestTruncateToWidthCJKExactBoundaryUpstream(t *testing.T) {
	// Upstream: truncateToWidth("你好世界", 4) => "你…" (CJK chars take 2 width each)
	result := truncateToWidth("你好世界", 4)
	if !strings.HasSuffix(result, "\u2026") {
		t.Errorf("expected truncation at CJK boundary, got %q", result)
	}
	// After truncation, width should be <= 4
	if stringWidth(result) > 4 {
		t.Errorf("truncated CJK string width %d exceeds max 4, got %q", stringWidth(result), result)
	}
}

func TestTruncateToWidthCJKPreservesChars(t *testing.T) {
	// Upstream: truncateToWidth("你好世界", 6) => "你好…" (preserves full CJK chars)
	result := truncateToWidth("你好世界", 6)
	if !strings.HasSuffix(result, "\u2026") {
		t.Errorf("expected truncation, got %q", result)
	}
	if stringWidth(result) > 6 {
		t.Errorf("truncated result width %d exceeds max 6, got %q", stringWidth(result), result)
	}
}

func TestTruncateToWidthCJKPassThrough(t *testing.T) {
	// Upstream: truncateToWidth("你好", 4) => "你好" (within limit, no truncation)
	result := truncateToWidth("你好", 4)
	if result != "你好" {
		t.Errorf("expected CJK pass-through, got %q", result)
	}
}

func TestTruncateToWidthMixedASCIICJK(t *testing.T) {
	// Upstream: truncateToWidth("hello你好", 8) => "hello你…"
	result := truncateToWidth("hello你好", 8)
	if !strings.HasSuffix(result, "\u2026") {
		t.Errorf("expected mixed truncation, got %q", result)
	}
	if stringWidth(result) > 8 {
		t.Errorf("mixed result width %d exceeds max 8, got %q", stringWidth(result), result)
	}
}

func TestTruncateToWidthMixedASCIICJKExact(t *testing.T) {
	// Upstream: truncateToWidth("hello你好", 9) => "hello你好" (exact fit)
	result := truncateToWidth("hello你好", 9)
	if result != "hello你好" {
		t.Errorf("expected exact fit pass-through, got %q", result)
	}
}

func TestTruncateStartToWidthCJKBoundaryUpstream(t *testing.T) {
	// Upstream: truncateStartToWidth("你好世界", 4) => "…界"
	result := truncateStartToWidth("你好世界", 4)
	if !strings.HasPrefix(result, "\u2026") {
		t.Errorf("expected leading ellipsis, got %q", result)
	}
	if stringWidth(result) > 4 {
		t.Errorf("truncated start result width %d exceeds max 4, got %q", stringWidth(result), result)
	}
}

func TestTruncateStartToWidthCJKPreservesChars(t *testing.T) {
	// Upstream: truncateStartToWidth("你好世界", 6) => "…世界"
	result := truncateStartToWidth("你好世界", 6)
	if !strings.HasPrefix(result, "\u2026") {
		t.Errorf("expected leading ellipsis, got %q", result)
	}
	if stringWidth(result) > 6 {
		t.Errorf("truncated start result width %d exceeds max 6, got %q", stringWidth(result), result)
	}
}

func TestTruncateToWidthNoEllipsisCJK(t *testing.T) {
	// Upstream: truncateToWidthNoEllipsis("你好世界", 4) => "你好"
	result := truncateToWidthNoEllipsis("你好世界", 4)
	if strings.Contains(result, "\u2026") {
		t.Errorf("should not contain ellipsis, got %q", result)
	}
	if stringWidth(result) > 4 {
		t.Errorf("result width %d exceeds max 4, got %q", stringWidth(result), result)
	}
}

func TestTruncatePathMiddleShortPath(t *testing.T) {
	// Upstream: truncatePathMiddle("src/index.ts", 50) => unchanged
	result := truncatePathMiddle("src/index.ts", 50)
	if result != "src/index.ts" {
		t.Errorf("expected unchanged for short path, got %q", result)
	}
}

func TestTruncatePathMiddleLongPath(t *testing.T) {
	// Upstream: truncation should preserve filename at end
	path := "src/components/deeply/nested/folder/MyComponent.tsx"
	result := truncatePathMiddle(path, 30)
	if !strings.Contains(result, "\u2026") {
		t.Errorf("expected middle ellipsis, got %q", result)
	}
	if !strings.Contains(result, "MyComponent.tsx") {
		t.Errorf("expected filename preserved at end, got %q", result)
	}
}

func TestTruncatePathMiddleMaxLength1(t *testing.T) {
	// Upstream: truncatePathMiddle("/a/b", 1) => "…"
	result := truncatePathMiddle("/a/b", 1)
	if result != "\u2026" {
		t.Errorf("expected ellipsis for maxLength 1, got %q", result)
	}
}

func TestTruncatePathMiddleMaxLength0(t *testing.T) {
	// Upstream: truncatePathMiddle("src/index.ts", 0) => "…"
	result := truncatePathMiddle("src/index.ts", 0)
	if result != "\u2026" {
		t.Errorf("expected ellipsis for maxLength 0, got %q", result)
	}
}

func TestTruncatePathMiddleNoSlashes(t *testing.T) {
	// Upstream: truncatePathMiddle("verylongfilename.ts", 10)
	result := truncatePathMiddle("verylongfilename.ts", 10)
	if stringWidth(result) > 10 {
		t.Errorf("result width exceeds max, got %q (width %d)", result, stringWidth(result))
	}
}

func TestTruncateStringPortedSingleLineNoTruncation(t *testing.T) {
	// Upstream: truncate("first\nsecond", 50, false) => unchanged
	result := truncateStringPorted("first\nsecond", 50, false)
	if result != "first\nsecond" {
		t.Errorf("expected unchanged when not singleLine and within limit, got %q", result)
	}
}

func TestWrapTextWidth1(t *testing.T) {
	// Upstream: wrapText("abc", 1) => ["a", "b", "c"]
	result := wrapText("abc", 1)
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(result))
	}
	if result[0] != "a" {
		t.Errorf("expected 'a', got %q", result[0])
	}
	if result[1] != "b" {
		t.Errorf("expected 'b', got %q", result[1])
	}
	if result[2] != "c" {
		t.Errorf("expected 'c', got %q", result[2])
	}
}

// ─── Roundtrip / width invariants ──────────────────────────────────────────

func TestTruncateToWidthRoundtripInvariant(t *testing.T) {
	// truncateToWidth(x, w) produces output y where truncateToWidth(y, w) = y
	// (idempotency). Also, stringWidth(y) <= w.
	testStrings := []string{
		"hello world this is a very long string indeed",
		"你好世界这是中文测试字符串",
		"hello你好world世界mixed",
		"",
	}
	testWidths := []int{1, 3, 5, 10, 20, 50}

	for _, s := range testStrings {
		for _, w := range testWidths {
			y := truncateToWidth(s, w)
			if stringWidth(y) > w {
				t.Errorf("truncateToWidth(%q, %d) = %q (width %d > %d)", s, w, y, stringWidth(y), w)
			}
			yy := truncateToWidth(y, w)
			if y != yy {
				t.Errorf("idempotency broken: truncateToWidth(%q, %d) = %q, then %q", s, w, y, yy)
			}
		}
	}
}

func TestTruncateStartToWidthRoundtripInvariant(t *testing.T) {
	testStrings := []string{
		"hello world this is a very long string indeed",
		"你好世界这是中文测试字符串",
	}
	testWidths := []int{1, 3, 5, 10, 20}

	for _, s := range testStrings {
		for _, w := range testWidths {
			y := truncateStartToWidth(s, w)
			if stringWidth(y) > w {
				t.Errorf("truncateStartToWidth(%q, %d) = %q (width %d > %d)", s, w, y, stringWidth(y), w)
			}
			yy := truncateStartToWidth(y, w)
			if y != yy {
				t.Errorf("idempotency broken: truncateStartToWidth(%q, %d) = %q, then %q", s, w, y, yy)
			}
		}
	}
}

func TestTruncateToWidthNoEllipsisRoundtripInvariant(t *testing.T) {
	testStrings := []string{
		"hello world this is a very long string indeed",
		"你好世界这是中文测试字符串",
	}
	testWidths := []int{0, 1, 3, 5, 10, 20}

	for _, s := range testStrings {
		for _, w := range testWidths {
			y := truncateToWidthNoEllipsis(s, w)
			if stringWidth(y) > w {
				t.Errorf("truncateToWidthNoEllipsis(%q, %d) = %q (width %d > %d)", s, w, y, stringWidth(y), w)
			}
			yy := truncateToWidthNoEllipsis(y, w)
			if y != yy {
				t.Errorf("idempotency broken: truncateToWidthNoEllipsis(%q, %d) = %q, then %q", s, w, y, yy)
			}
		}
	}
}

func TestTruncatePathMiddleWidthInvariant(t *testing.T) {
	paths := []string{
		"/usr/local/bin/file.txt",
		"src/components/deeply/nested/folder/MyComponent.tsx",
		"verylongfilename.ts",
	}
	testWidths := []int{1, 5, 10, 20, 30}

	for _, path := range paths {
		for _, w := range testWidths {
			result := truncatePathMiddle(path, w)
			if stringWidth(result) > w {
				t.Errorf("truncatePathMiddle(%q, %d) = %q (width %d > %d)", path, w, result, stringWidth(result), w)
			}
		}
	}
}

// ─── wrapText width invariant ──────────────────────────────────────────────

func TestWrapTextWidthInvariant(t *testing.T) {
	text := "hello world this is a longer string for wrapping tests"
	for _, w := range []int{3, 5, 10, 20} {
		lines := wrapText(text, w)
		for i, line := range lines {
			if stringWidth(line) > w {
				t.Errorf("wrapText width=%d, line %d too wide: %q (width %d)", w, i, line, stringWidth(line))
			}
		}
	}
}
