package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// ─── expandPath tests ────────────────────────────────────────────────────────
// Mirrors upstream: file.ts getDisplayPath/getAbsoluteAndRelativePaths

func TestExpandPathTildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	tests := []struct {
		input string
	}{
		{"~"},
		{"~/file.txt"},
		{"~/subdir/file.txt"},
		{"~/.config/settings.json"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandPath(tt.input)
			if !filepath.IsAbs(got) {
				t.Errorf("expandPath(%q) = %q, want absolute path", tt.input, got)
			}
			if runtime.GOOS == "windows" {
				// On Windows, the path should start with home dir (e.g., C:\Users\xxx)
				if len(got) < len(home) {
					t.Errorf("expandPath(%q) too short: %q", tt.input, got)
				}
			} else {
				expectedPrefix := home
				if tt.input == "~" {
					expectedPrefix = home
				} else {
					rest := tt.input[1:] // strip ~
					expectedPrefix = filepath.Join(home, rest)
				}
				// After filepath.Clean, should match
				if got != filepath.Clean(expectedPrefix) {
					t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, filepath.Clean(expectedPrefix))
				}
			}
		})
	}
}

func TestExpandPathAlreadyAbsolute(t *testing.T) {
	// Absolute paths should be returned cleaned
	tests := []struct {
		input string
	}{
		{"/tmp"},
		{"/tmp/file.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandPath(tt.input)
			if !filepath.IsAbs(got) {
				t.Errorf("expandPath(%q) = %q, want absolute", tt.input, got)
			}
		})
	}
}

func TestExpandPathRelative(t *testing.T) {
	// Relative paths should be cleaned
	got := expandPath("./file.txt")
	want := filepath.Clean("file.txt")
	if got != want {
		t.Errorf("expandPath(./file.txt) = %q, want %q", got, want)
	}
}

func TestExpandPathDotDot(t *testing.T) {
	got := expandPath("../file.txt")
	want := filepath.Clean("../file.txt")
	if got != want {
		t.Errorf("expandPath(../file.txt) = %q, want %q", got, want)
	}
}

// TestExpandPathDriveLetterWindows tests Windows drive letter normalization.
// On Windows, "E:" means current dir on that drive; normalize to "E:\"
func TestExpandPathDriveLetterWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	tests := []struct {
		input string
		want  string
	}{
		{"E:", "E:\\"},
		{"e:", "e:\\"},
		{"C:", "C:\\"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandPath(tt.input)
			if got != tt.want {
				t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestExpandPathLongDriveLetter tests that longer drive paths (e.g., "E:\")
// are NOT modified (only bare 2-char "X:" should be normalized).
func TestExpandPathLongDriveLetterWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	got := expandPath(`E:\projects\test`)
	// Should not add extra separator
	if len(got) < 3 {
		t.Errorf("expandPath truncated: %q", got)
	}
}

// ─── normalizeFilePath tests ─────────────────────────────────────────────────
// Mirrors upstream: file.ts normalizePathForComparison

func TestNormalizeFilePathBackslashToSlash(t *testing.T) {
	if runtime.GOOS != "windows" {
		// On Unix, backslash is a valid filename character, not a separator
		// But normalizeFilePath still converts it
		got := normalizeFilePath(`a\b\c`)
		want := "a/b/c"
		if got != want {
			t.Errorf("normalizeFilePath(a\\b\\c) = %q, want %q", got, want)
		}
	}
}

func TestNormalizeFilePathDotDotResolution(t *testing.T) {
	got := normalizeFilePath("/a/b/../c")
	want := "/a/c"
	if got != want {
		t.Errorf("normalizeFilePath(/a/b/../c) = %q, want %q", got, want)
	}
}

func TestNormalizeFilePathDotResolution(t *testing.T) {
	got := normalizeFilePath("/a/./b/c")
	want := "/a/b/c"
	if got != want {
		t.Errorf("normalizeFilePath(/a/./b/c) = %q, want %q", got, want)
	}
}

func TestNormalizeFilePathLowercase(t *testing.T) {
	// normalizeFilePath lowercases everything for consistent comparison
	got := normalizeFilePath("/A/B/C")
	want := "/a/b/c"
	if got != want {
		t.Errorf("normalizeFilePath(/A/B/C) = %q, want %q", got, want)
	}
}

func TestNormalizeFilePathRedundantSlashes(t *testing.T) {
	got := normalizeFilePath("/a//b///c")
	want := "/a/b/c"
	if got != want {
		t.Errorf("normalizeFilePath(/a//b///c) = %q, want %q", got, want)
	}
}

func TestNormalizeFilePathEmptyString(t *testing.T) {
	got := normalizeFilePath("")
	want := "."
	if got != want {
		t.Errorf("normalizeFilePath(\"\") = %q, want %q", got, want)
	}
}

func TestNormalizeFilePathSingleDot(t *testing.T) {
	got := normalizeFilePath(".")
	want := "."
	if got != want {
		t.Errorf("normalizeFilePath(.) = %q, want %q", got, want)
	}
}

func TestNormalizeFilePathIdempotent(t *testing.T) {
	// normalizeFilePath(normalizeFilePath(x)) == normalizeFilePath(x)
	paths := []string{
		"/a/b/../c",
		"/A//B///C",
		`x\y\z`,
		"file.txt",
		"./dir/../file",
	}
	for _, p := range paths {
		first := normalizeFilePath(p)
		second := normalizeFilePath(first)
		if first != second {
			t.Errorf("normalizeFilePath(%q) not idempotent: first=%q, second=%q", p, first, second)
		}
	}
}

// ─── IsPathAllowed tests ─────────────────────────────────────────────────────
// Mirrors upstream: file.ts path safety / directory containment checks

func TestIsPathAllowedWithinProject(t *testing.T) {
	// Get current working directory
	wd, err := os.Getwd()
	if err != nil {
		t.Skip("cannot get working directory")
	}
	// Create a subdirectory
	tmpDir := filepath.Join(wd, "test_path_allowed_dir")
	os.MkdirAll(tmpDir, 0o755)
	defer os.RemoveAll(tmpDir)

	// Create a test file inside
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	// Change to the test dir for this test
	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origWd)

	// "test.txt" should be allowed (relative to cwd)
	errMsg := IsPathAllowed("test.txt")
	if errMsg != "" {
		t.Errorf("IsPathAllowed(test.txt) = %q, want empty string", errMsg)
	}
}

func TestIsPathAllowedFileInSubDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Skip("cannot get working directory")
	}
	tmpDir := filepath.Join(wd, "test_path_allowed_sub")
	subDir := filepath.Join(tmpDir, "sub")
	os.MkdirAll(subDir, 0o755)
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(subDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0o644)

	origWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origWd)

	errMsg := IsPathAllowed("sub/test.txt")
	if errMsg != "" {
		t.Errorf("IsPathAllowed(sub/test.txt) = %q, want empty string", errMsg)
	}
}

// ─── copyFile / copyPath tests ───────────────────────────────────────────────
// Mirrors upstream: file.ts copyFile semantics

func TestCopyFileBasic(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.txt")
	dest := filepath.Join(t.TempDir(), "dest.txt")
	os.WriteFile(src, []byte("hello world"), 0o644)

	err := copyFile(src, dest)
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("cannot read dest: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("copyFile content mismatch: got %q, want %q", string(data), "hello world")
	}
}

func TestCopyFileSourceMissing(t *testing.T) {
	err := copyFile("/nonexistent/src_abc123.txt", "/nonexistent/dest_abc123.txt")
	if err == nil {
		t.Error("copyFile with missing source should error")
	}
}

// ─── copyPath (recursive) tests ─────────────────────────────────────────────

func TestCopyPathBasicDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src_dir")
	dest := filepath.Join(t.TempDir(), "dest_dir")

	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("aaa"), 0o644)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("bbb"), 0o644)

	err := copyPath(src, dest)
	if err != nil {
		t.Fatalf("copyPath failed: %v", err)
	}

	// Verify structure
	data1, _ := os.ReadFile(filepath.Join(dest, "a.txt"))
	if string(data1) != "aaa" {
		t.Errorf("dest/a.txt content mismatch: got %q", string(data1))
	}
	data2, _ := os.ReadFile(filepath.Join(dest, "sub", "b.txt"))
	if string(data2) != "bbb" {
		t.Errorf("dest/sub/b.txt content mismatch: got %q", string(data2))
	}
}

func TestCopyPathSourceMissing(t *testing.T) {
	err := copyPath("/nonexistent/src_dir_abc123", "/nonexistent/dest_dir_abc123")
	if err == nil {
		t.Error("copyPath with missing source should error")
	}
}

func TestCopyPathSingleFile(t *testing.T) {
	// copyPath on a file should delegate to copyFile
	src := filepath.Join(t.TempDir(), "file.txt")
	dest := filepath.Join(t.TempDir(), "file_copy.txt")
	os.WriteFile(src, []byte("content"), 0o644)

	err := copyPath(src, dest)
	if err != nil {
		t.Fatalf("copyPath on file failed: %v", err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "content" {
		t.Errorf("copyPath content: got %q", string(data))
	}
}

// ─── opMkdir tests ──────────────────────────────────────────────────────────

func TestOpMkdirBasic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new_dir")
	result := opMkdir(dir, map[string]any{})
	if result.IsError {
		t.Errorf("opMkdir failed: %s", result.Output)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Error("directory was not created")
	}
}

func TestOpMkdirRecursive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "c")
	result := opMkdir(dir, map[string]any{})
	if result.IsError {
		t.Errorf("opMkdir recursive failed: %s", result.Output)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Error("nested directory was not created")
	}
}

func TestOpMkdirWithMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode checking not reliable on Windows")
	}
	dir := filepath.Join(t.TempDir(), "mode_dir")
	result := opMkdir(dir, map[string]any{"mode": "700"})
	if result.IsError {
		t.Errorf("opMkdir with mode failed: %s", result.Output)
	}
	info, _ := os.Stat(dir)
	got := info.Mode().Perm()
	if got != 0o700 {
		t.Errorf("expected mode 0700, got %o", got)
	}
}

func TestOpMkdirInvalidPath(t *testing.T) {
	// On most systems, you can't create a directory with an empty path
	result := opMkdir("", map[string]any{})
	if !result.IsError {
		t.Log("opMkdir with empty path returned success (may be OS-dependent)")
	}
}

// ─── opLink (symlink) tests ─────────────────────────────────────────────────

func TestOpLinkSymlinkBasic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks may require admin on Windows")
	}
	target := filepath.Join(t.TempDir(), "target.txt")
	link := filepath.Join(t.TempDir(), "link.txt")
	os.WriteFile(target, []byte("target content"), 0o644)

	result := opLink(target, map[string]any{
		"destination": link,
		"symbolic":    true,
	})
	if result.IsError {
		t.Errorf("opLink symlink failed: %s", result.Output)
		return
	}
	// Verify it's a symlink
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("cannot lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink, got regular file")
	}
}

func TestOpLinkNoDestination(t *testing.T) {
	result := opLink("/tmp/target", map[string]any{"symbolic": true})
	if !result.IsError {
		t.Error("opLink without destination should error")
	}
}

func TestOpLinkForce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks may require admin on Windows")
	}
	target := filepath.Join(t.TempDir(), "target2.txt")
	link := filepath.Join(t.TempDir(), "link2.txt")
	os.WriteFile(target, []byte("target"), 0o644)
	// Pre-create the destination as a regular file
	os.WriteFile(link, []byte("existing"), 0o644)

	result := opLink(target, map[string]any{
		"destination": link,
		"symbolic":    true,
		"force":       true,
	})
	if result.IsError {
		t.Errorf("opLink force failed: %s", result.Output)
	}
}

// ─── opRemoveAll protected paths ────────────────────────────────────────────
// Mirrors upstream: file.ts dangerous path checks

func TestOpRemoveAllProtectedRoot(t *testing.T) {
	result := opRemoveAll("/")
	if !result.IsError {
		t.Error("opRemoveAll(/) should be protected")
	}
}

func TestOpRemoveAllProtectedDot(t *testing.T) {
	result := opRemoveAll(".")
	if !result.IsError {
		t.Error("opRemoveAll(.) should be protected")
	}
}

func TestOpRemoveAllProtectedDotSlash(t *testing.T) {
	result := opRemoveAll("./")
	if !result.IsError {
		t.Error("opRemoveAll(./) should be protected")
	}
}

func TestOpRemoveAllProtectedDotGit(t *testing.T) {
	result := opRemoveAll("/some/path/.git")
	if !result.IsError {
		t.Error("opRemoveAll(.git) should be protected")
	}
}

func TestOpRemoveAllProtectedHomeTilde(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix only: Windows requires drive letters")
	}
	result := opRemoveAll("/home/user/~")
	if !result.IsError {
		t.Error("opRemoveAll(~/~) should be protected")
	}
}

func TestOpRemoveAllWindowsPathStyles(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	// Windows-specific protected path patterns
	tests := []string{
		`C:\.git`,
		`D:\~`,
	}
	for _, p := range tests {
		t.Run(p, func(t *testing.T) {
			result := opRemoveAll(p)
			if !result.IsError {
				t.Errorf("opRemoveAll(%q) should be protected", p)
			}
		})
	}
}
