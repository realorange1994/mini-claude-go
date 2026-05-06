package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsBinaryMagic(t *testing.T) {
	tests := []struct {
		header   []byte
		expected bool
	}{
		{[]byte{0x7f, 'E', 'L', 'F'}, true},
		{[]byte{'M', 'Z'}, true},
		{[]byte{'%', 'P', 'D', 'F'}, true},
		{[]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, true},
		{[]byte{'G', 'I', 'F', '8', '7', 'a'}, true},
		{[]byte{'G', 'I', 'F', '8', '9', 'a'}, true},
		{[]byte{0xff, 0xd8, 0xff}, true},
		{[]byte{'P', 'K', 0x03, 0x04}, true},
		{[]byte{'P', 'K', 0x05, 0x06}, true},
		{[]byte{0x1f, 0x8b}, true},
		{[]byte{'B', 'Z'}, true},
		{[]byte{0xfd, '7', 'z', 'X', 'Z', 0x00}, true},
		{[]byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}, true},
		{[]byte{'I', 'D', '3'}, true},
		{[]byte{0xff, 0xfb}, true},
		{[]byte{'R', 'I', 'F', 'F', 0x00, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P'}, true},
		{[]byte{'R', 'I', 'F', 'F', 0x00, 0x00, 0x00, 0x00, 'W', 'A', 'V', 'E'}, true},
		{[]byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p'}, true},
		{[]byte{0xca, 0xfe, 0xba, 0xbe}, true},
		{[]byte{0x00, 'a', 's', 'm'}, true},
		{[]byte{0x0d, 0x0d, 0x0d, 0x0a}, true},
		{[]byte{0x1b, 'L', 'u', 'a'}, true},
		// Text-like bytes: should NOT match
		{[]byte{'#', '!', '/', 'b', 'i', 'n'}, false},
		{[]byte{'<', '?', 'x', 'm'}, false},
		{[]byte{'<', 'h', 't', 'm'}, false},
		{[]byte{0xef, 0xbb, 0xbf, 'p'}, false},
		{[]byte("hello world"), false},
	}

	for _, tc := range tests {
		result := isBinaryMagic(tc.header)
		if result != tc.expected {
			t.Errorf("isBinaryMagic(%v) = %v, want %v", tc.header, result, tc.expected)
		}
	}
}

func TestIsBinaryMagicShort(t *testing.T) {
	if isBinaryMagic([]byte{0x7f}) {
		t.Error("should return false for single byte")
	}
	if isBinaryMagic([]byte{0x7f, 'E', 'L'}) {
		t.Error("should return false for 3 bytes")
	}
	if isBinaryMagic(nil) {
		t.Error("should return false for nil")
	}
}

func TestReadFileBinaryMagicDetection(t *testing.T) {
	dir := t.TempDir()

	// PNG with wrong extension (renamed binary)
	pngPath := filepath.Join(dir, "fake.txt")
	os.WriteFile(pngPath, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00}, 0644)

	tool := &FileReadTool{}
	result := tool.Execute(map[string]any{"file_path": pngPath})
	if !result.IsError {
		t.Error("expected error for PNG magic bytes in fake .txt file")
	}
}

func TestReadFileBinaryMagicELF(t *testing.T) {
	dir := t.TempDir()
	elfPath := filepath.Join(dir, "binary.bin")
	os.WriteFile(elfPath, []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01}, 0644)

	tool := &FileReadTool{}
	result := tool.Execute(map[string]any{"file_path": elfPath})
	if !result.IsError {
		t.Error("expected error for ELF binary")
	}
}

func TestReadFileTextFile(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "main.go")
	os.WriteFile(txtPath, []byte("package main\n\nfunc main() {}\n"), 0644)

	tool := &FileReadTool{}
	result := tool.Execute(map[string]any{"file_path": txtPath})
	if result.IsError {
		t.Errorf("text file should be readable, got error: %s", result.Output)
	}
}
