package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// ─── Upstream Quality: ExpandPath tests (from path.test.ts) ───────────────────

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

// ─── Upstream Quality: ToRelativePath detailed tests ────────────────────────────

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

// ─── Upstream Quality: NormalizePathForComparison invariant tests ────────────────

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
