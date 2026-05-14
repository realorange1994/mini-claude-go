package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// ─── ContainsPathTraversal ───────────────────────────────────────────────────

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
}

func TestContainsPathTraversalBackslash(t *testing.T) {
	if !ContainsPathTraversal(`foo\..\bar`) {
		t.Fatal("expected true for backslash path traversal")
	}
}

// ─── NormalizePathForConfigKey ───────────────────────────────────────────────

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

// ─── ToRelativePath ──────────────────────────────────────────────────────────

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

// ─── GetDirectoryForPath ─────────────────────────────────────────────────────

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

// ─── Invariant: NormalizePathForConfigKey idempotent ─────────────────────────

func TestNormalizePathForConfigKeyIdempotent(t *testing.T) {
	input := `foo\bar\./baz\..`
	first := NormalizePathForConfigKey(input)
	second := NormalizePathForConfigKey(first)
	if first != second {
		t.Fatalf("normalization not idempotent: %q != %q", first, second)
	}
}

// ─── Invariant: ContainsPathTraversal symmetrical with path separators ───────

func TestContainsPathTraversalBothSeparators(t *testing.T) {
	forward := ContainsPathTraversal("foo/../bar")
	backward := ContainsPathTraversal(`foo\..\bar`)
	if forward != backward {
		t.Fatalf("path traversal detection should be consistent across separators: /=%v, \\=%v", forward, backward)
	}
}
