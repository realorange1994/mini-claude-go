package main

import (
	"strings"
	"testing"
)

// ─── sliceAnsi ───────────────────────────────────────────────────────────
// Ported from upstream sliceAnsi.test.ts

func TestSliceAnsiPlainString(t *testing.T) {
	result := sliceAnsi("hello world", 0, 5)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestSliceAnsiPlainStringStart(t *testing.T) {
	result := sliceAnsi("hello world", 6)
	if result != "world" {
		t.Errorf("expected 'world', got %q", result)
	}
}

func TestSliceAnsiEntireString(t *testing.T) {
	result := sliceAnsi("hello", 0)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestSliceAnsiEmptyString(t *testing.T) {
	result := sliceAnsi("", 0)
	if result != "" {
		t.Errorf("expected '', got %q", result)
	}
}

func TestSliceAnsiSingleChar(t *testing.T) {
	result := sliceAnsi("a", 0, 1)
	if result != "a" {
		t.Errorf("expected 'a', got %q", result)
	}
}

func TestSliceAnsiWithColor(t *testing.T) {
	// Red "hello" + normal " world"
	colored := "\x1b[31mhello\x1b[0m world"
	result := sliceAnsi(colored, 0, 5)
	// Should include the color code since it's before position 5
	if !strings.Contains(result, "\x1b[31m") {
		t.Errorf("expected color code preserved, got %q", result)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("expected 'hello' preserved, got %q", result)
	}
}

func TestSliceAnsiWithColorEnd(t *testing.T) {
	// Red "hello" + reset + normal " world"
	colored := "\x1b[31mhello\x1b[0m world"
	result := sliceAnsi(colored, 6)
	// Starting after the reset, so the result should be "world"
	// but may include inherited active codes from before start position
	plainResult := stripAnsiCodes(result)
	if plainResult != "world" {
		t.Errorf("expected visible text 'world', got %q (full: %q)", plainResult, result)
	}
}

func TestSliceAnsiWithMultipleColors(t *testing.T) {
	// Red "he" + green "llo" + normal " world"
	colored := "\x1b[31mhe\x1b[32mllo\x1b[0m world"
	result := sliceAnsi(colored, 0, 3)
	if !strings.Contains(result, "\x1b[31m") {
		t.Errorf("expected red code preserved, got %q", result)
	}
}

func TestSliceAnsiStartAfterColorCode(t *testing.T) {
	// Red "hello" + normal " world"
	colored := "\x1b[31mhello\x1b[0m world"
	result := sliceAnsi(colored, 2, 5)
	// Should include the red code since the slice starts within the colored section
	if !strings.Contains(result, "\x1b[31m") {
		t.Errorf("expected red code preserved in partial slice, got %q", result)
	}
	if !strings.Contains(result, "llo") {
		t.Errorf("expected 'llo' preserved, got %q", result)
	}
}

func TestSliceAnsiZeroStartZeroEnd(t *testing.T) {
	result := sliceAnsi("hello", 0, 0)
	if result != "" {
		t.Errorf("expected '' for 0:0 slice, got %q", result)
	}
}

// ─── stripAnsiCodes ──────────────────────────────────────────────────────

func TestStripAnsiCodesBasic(t *testing.T) {
	result := stripAnsiCodes("\x1b[31mhello\x1b[0m world")
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestStripAnsiCodesNoCodes(t *testing.T) {
	result := stripAnsiCodes("plain text")
	if result != "plain text" {
		t.Errorf("expected 'plain text', got %q", result)
	}
}

func TestStripAnsiCodesEmpty(t *testing.T) {
	result := stripAnsiCodes("")
	if result != "" {
		t.Errorf("expected '', got %q", result)
	}
}

func TestStripAnsiCodesMultiple(t *testing.T) {
	result := stripAnsiCodes("\x1b[1m\x1b[31mhello\x1b[0m")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

// ─── containsAnsiCode ────────────────────────────────────────────────────

func TestContainsAnsiCodeTrue(t *testing.T) {
	if !containsAnsiCode("\x1b[31mhello") {
		t.Error("expected true for string with ANSI codes")
	}
}

func TestContainsAnsiCodeFalse(t *testing.T) {
	if containsAnsiCode("plain text") {
		t.Error("expected false for string without ANSI codes")
	}
}

// ─── ansiCodeCount ───────────────────────────────────────────────────────

func TestAnsiCodeCountZero(t *testing.T) {
	if ansiCodeCount("hello") != 0 {
		t.Errorf("expected 0 codes, got %d", ansiCodeCount("hello"))
	}
}

func TestAnsiCodeCountOne(t *testing.T) {
	if ansiCodeCount("\x1b[31mhello") != 1 {
		t.Errorf("expected 1 code, got %d", ansiCodeCount("\x1b[31mhello"))
	}
}

func TestAnsiCodeCountMultiple(t *testing.T) {
	if ansiCodeCount("\x1b[1m\x1b[31mhello\x1b[0m") != 3 {
		t.Errorf("expected 3 codes, got %d", ansiCodeCount("\x1b[1m\x1b[31mhello\x1b[0m"))
	}
}

// ─── Invariants ──────────────────────────────────────────────────────────

func TestSliceAnsiRoundtrip(t *testing.T) {
	// sliceAnsi(x, 0) should return the plain text of x (modulo ANSI codes)
	colored := "\x1b[31mhello\x1b[0m world"
	result := sliceAnsi(colored, 0)
	plainResult := stripAnsiCodes(result)
	if plainResult != "hello world" {
		t.Errorf("roundtrip: expected 'hello world', got %q", plainResult)
	}
}

func TestSliceAnsiPreservesAnsiCodes(t *testing.T) {
	// sliceAnsi should never strip ANSI codes from within the sliced range
	colored := "\x1b[31mhello\x1b[0m"
	result := sliceAnsi(colored, 0, 5)
	if !strings.Contains(result, "\x1b[31m") {
		t.Errorf("ANSI code should be preserved in sliced output, got %q", result)
	}
}

func TestSliceAnsiWidthConsistent(t *testing.T) {
	// The display width of the sliced result (ignoring ANSI codes) should
	// match the slice boundaries
	colored := "\x1b[31mhello\x1b[0m world"
	result := sliceAnsi(colored, 0, 5)
	plainResult := stripAnsiCodes(result)
	if len(plainResult) != 5 {
		t.Errorf("expected 5 visible chars, got %d: %q", len(plainResult), plainResult)
	}
}

func TestSliceAnsiStartLessThanEnd(t *testing.T) {
	// If start >= end, should return empty
	result := sliceAnsi("hello", 3, 3)
	if result != "" {
		t.Errorf("expected '' for start==end, got %q", result)
	}
}

func TestSliceAnsiStartBeyondLength(t *testing.T) {
	result := sliceAnsi("hi", 10)
	if result != "" {
		t.Errorf("expected '' for start beyond length, got %q", result)
	}
}

func TestStripAnsiCodesIdempotent(t *testing.T) {
	input := "\x1b[31mhello\x1b[0m"
	first := stripAnsiCodes(input)
	second := stripAnsiCodes(first)
	if first != second {
		t.Errorf("idempotency: %q != %q", first, second)
	}
}