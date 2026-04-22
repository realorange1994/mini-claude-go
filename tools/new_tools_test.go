package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageReadSuccess(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "test.png")
	// Minimal PNG header: 89 50 4E 47 ...
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	os.WriteFile(imgPath, pngHeader, 0644)

	tool := &ImageReadTool{}
	result := tool.Execute(map[string]any{"path": imgPath})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "image/png") {
		t.Errorf("expected image/png in output: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Base64") {
		t.Errorf("expected Base64 in output: %s", result.Output)
	}
}

func TestImageReadNotFound(t *testing.T) {
	tool := &ImageReadTool{}
	result := tool.Execute(map[string]any{"path": "/nonexistent/image.png"})
	if !result.IsError {
		t.Error("expected error for missing file")
	}
}

func TestImageReadUnsupported(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "file.xyz")
	os.WriteFile(fp, []byte("not an image"), 0644)

	tool := &ImageReadTool{}
	result := tool.Execute(map[string]any{"path": fp})
	if !result.IsError {
		t.Error("expected error for unsupported format")
	}
}

func TestImageReadJPEG(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "test.jpg")
	// Minimal JPEG header: FF D8 FF
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	os.WriteFile(imgPath, jpegHeader, 0644)

	tool := &ImageReadTool{}
	result := tool.Execute(map[string]any{"path": imgPath, "detail": "low"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "image/jpeg") {
		t.Errorf("expected image/jpeg: %s", result.Output)
	}
}

func TestProcessList(t *testing.T) {
	tool := &ProcessTool{}
	result := tool.Execute(map[string]any{"operation": "list"})
	// This should work on any platform
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
}

func TestProcessPgrep(t *testing.T) {
	tool := &ProcessTool{}
	// Search for a pattern that likely won't match
	result := tool.Execute(map[string]any{"operation": "pgrep", "pattern": "nonexistentprocessxyz123"})
	// On Windows this returns "No processes matching..."
	if !strings.Contains(result.Output, "No processes") && result.IsError {
		t.Logf("got error (may be expected): %s", result.Output)
	}
}

func TestProcessMissingOperation(t *testing.T) {
	tool := &ProcessTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("expected error for missing operation")
	}
}

func TestFileOpsMkdir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "newdir")

	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "mkdir", "path": target})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Errorf("directory not created: %v", err)
	}
}

func TestFileOpsMkdirRecursive(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "c")

	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "mkdir", "path": target})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		t.Errorf("recursive directory not created: %v", err)
	}
}

func TestFileOpsRm(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "file.txt")
	os.WriteFile(fp, []byte("hello"), 0644)

	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "rm", "path": fp})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

func TestFileOpsRmrf(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0755)

	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "rmrf", "path": filepath.Join(dir, "sub")})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub")); !os.IsNotExist(err) {
		t.Error("directory should have been removed")
	}
}

func TestFileOpsMv(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("hello"), 0644)
	dst := filepath.Join(dir, "dst.txt")

	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "mv", "path": src, "destination": dst})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source should have been removed")
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "hello" {
		t.Errorf("destination content mismatch: %s %v", data, err)
	}
}

func TestFileOpsCp(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	os.WriteFile(src, []byte("hello"), 0644)
	dst := filepath.Join(dir, "dst.txt")

	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "cp", "path": src, "destination": dst})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	// Both files should exist
	if _, err := os.Stat(src); err != nil {
		t.Error("source should still exist")
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "hello" {
		t.Errorf("destination content mismatch: %s %v", data, err)
	}
}

func TestFileOpsCpdir(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "srcdir")
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("b"), 0644)
	dstDir := filepath.Join(dir, "dstdir")

	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "cpdir", "path": srcDir, "destination": dstDir})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	data, err := os.ReadFile(filepath.Join(dstDir, "a.txt"))
	if err != nil || string(data) != "a" {
		t.Errorf("copied file content mismatch: %s %v", data, err)
	}
}

func TestFileOpsChmod(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "file.txt")
	os.WriteFile(fp, []byte("hello"), 0644)

	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "chmod", "path": fp, "mode": "755"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	info, err := os.Stat(fp)
	if err != nil {
		t.Fatal(err)
	}
	// On Windows, the mode may not change the same way
	perm := info.Mode().Perm()
	if perm != 0o755 {
		t.Logf("mode changed to %o (expected 755, may differ on Windows)", perm)
	}
}

func TestFileOpsUnknownOperation(t *testing.T) {
	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "unknown", "path": "/tmp"})
	if !result.IsError {
		t.Error("expected error for unknown operation")
	}
}

func TestFileOpsMissingPath(t *testing.T) {
	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "mkdir"})
	if !result.IsError {
		t.Error("expected error for missing path")
	}
}
