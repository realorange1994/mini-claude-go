package rgrep

import (
	"bytes"
	"io"
	"os"
)

// IsBinaryFile checks if a file is binary by scanning for null bytes.
// It reads up to the first 8KB of the file for the check.
func IsBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	return IsBinaryReader(f)
}

// IsBinaryReader checks if a reader's content is binary by scanning
// for null bytes in the first 8KB.
func IsBinaryReader(r io.Reader) bool {
	buf := make([]byte, 8192)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return false
	}
	if n == 0 {
		return false
	}
	return bytes.IndexByte(buf[:n], 0) >= 0
}
