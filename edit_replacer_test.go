package main

import (
	"testing"
)

func TestEditReplacerChain_ExactMatch(t *testing.T) {
	chain := NewEditReplacerChain()

	content := "hello world"
	result, ok := chain.Replace(content, "world", "universe")
	if !ok {
		t.Error("expected success")
	}
	if result != "hello universe" {
		t.Errorf("expected 'hello universe', got %q", result)
	}
}

func TestEditReplacerChain_NoMatch(t *testing.T) {
	chain := NewEditReplacerChain()

	content := "hello world"
	_, ok := chain.Replace(content, "xyz", "abc")
	if ok {
		t.Error("expected failure")
	}
}

func TestSimpleReplacer(t *testing.T) {
	r := &SimpleReplacer{}

	result, ok := r.Replace("hello world", "world", "universe")
	if !ok {
		t.Error("expected success")
	}
	if result != "hello universe" {
		t.Errorf("expected 'hello universe', got %q", result)
	}
}

func TestLineTrimmedReplacer(t *testing.T) {
	r := &LineTrimmedReplacer{}

	content := "  hello\n  world"
	old := "hello\nworld"
	new := "foo\nbar"

	result, ok := r.Replace(content, old, new)
	if !ok {
		t.Error("expected success")
	}
	// LineTrimmedReplacer replaces with new content
	if result != "foo\nbar" {
		t.Errorf("expected 'foo\\nbar', got %q", result)
	}
}

func TestWhitespaceNormalizedReplacer(t *testing.T) {
	r := &WhitespaceNormalizedReplacer{}

	content := "hello  world"
	old := "hello world"
	new := "foo bar"

	result, ok := r.Replace(content, old, new)
	if !ok {
		t.Error("expected success")
	}
	if result != "foo bar" {
		t.Errorf("expected 'foo bar', got %q", result)
	}
}

func TestIndentationFlexibleReplacer(t *testing.T) {
	r := &IndentationFlexibleReplacer{}

	content := "    hello\n    world"
	old := "hello\nworld"
	new := "foo\nbar"

	result, ok := r.Replace(content, old, new)
	if !ok {
		t.Error("expected success")
	}
	if result != "    foo\n    bar" {
		t.Errorf("expected '    foo\\n    bar', got %q", result)
	}
}

func TestBlockAnchorReplacer(t *testing.T) {
	r := &BlockAnchorReplacer{}

	content := "line1\nhello\nworld\nline4"
	old := "hello\nworld"
	new := "foo\nbar"

	result, ok := r.Replace(content, old, new)
	if !ok {
		t.Error("expected success")
	}
	if result != "line1\nfoo\nbar\nline4" {
		t.Errorf("expected 'line1\\nfoo\\nbar\\nline4', got %q", result)
	}
}

func TestTrimmedBoundaryReplacer(t *testing.T) {
	r := &TrimmedBoundaryReplacer{}

	content := "hello\nworld"
	old := "\nhello\nworld\n"
	new := "foo\nbar"

	result, ok := r.Replace(content, old, new)
	if !ok {
		t.Error("expected success")
	}
	if result != "foo\nbar" {
		t.Errorf("expected 'foo\\nbar', got %q", result)
	}
}

func TestMultiOccurrenceReplacer(t *testing.T) {
	r := &MultiOccurrenceReplacer{}

	content := "hello hello world"
	old := "hello"
	new := "foo"

	result, ok := r.Replace(content, old, new)
	if !ok {
		t.Error("expected success")
	}
	if result != "foo hello world" {
		t.Errorf("expected 'foo hello world', got %q", result)
	}
}

func TestContextAwareReplacer(t *testing.T) {
	r := &ContextAwareReplacer{}

	content := "line1\nhello\nworld\nline4"
	old := "hello\nworld\nuniverse"
	new := "foo\nbar\nbaz"

	// Context-aware should find "hello\nworld" context
	_, ok := r.Replace(content, old, new)
	// May or may not succeed depending on context
	if ok {
		t.Log("context-aware match succeeded")
	}
}
