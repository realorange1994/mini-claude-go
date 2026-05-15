package tools

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf16"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/htmlindex"
)

// File encoding constants matching upstream Claude Code behavior.
const (
	EncodingUTF8    = "utf-8"
	EncodingUTF16LE = "utf-16le"
)

// LineEnding constants.
const (
	LineEndingLF   = "LF"
	LineEndingCRLF = "CRLF"
)

// FileMetadata holds encoding and line-ending information detected from a file.
type FileMetadata struct {
	Encoding    string // "utf-8" or "utf-16le" (BOM-based, matching upstream)
	LineEndings string // "LF" or "CRLF"
}

// DecodeFileContent reads raw file bytes, detects encoding via BOM, decodes
// to a Go string, and normalizes CRLF to LF. Returns the decoded content
// and metadata about the original encoding and line endings.
//
// This matches upstream's readFileSyncWithMetadata() behavior:
//   - UTF-16 LE BOM (FF FE) → decode via utf16, encoding="utf-16le"
//   - UTF-8 BOM (EF BB BF) → strip BOM, encoding="utf-8"
//   - Otherwise → treat as UTF-8, encoding="utf-8"
//   - CRLF → normalize to LF for internal use, lineEndings="CRLF"
func DecodeFileContent(data []byte) (content string, meta FileMetadata) {
	meta = FileMetadata{
		Encoding:    EncodingUTF8,
		LineEndings: LineEndingLF,
	}

	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		// UTF-16 LE BOM detected — decode to UTF-8 string
		meta.Encoding = EncodingUTF16LE
		u16s := bytesToUint16LE(data[2:])
		content = string(utf16.Decode(u16s))
	} else {
		content = string(data)
	}

	// Detect line endings before normalizing
	if strings.Contains(content, "\r\n") {
		meta.LineEndings = LineEndingCRLF
	}

	// Normalize CRLF to LF for internal processing
	content = strings.ReplaceAll(content, "\r\n", "\n")

	// Strip UTF-8 BOM (matching official Claude Code behavior)
	if strings.HasPrefix(content, "\xEF\xBB\xBF") {
		content = content[3:]
	}

	return content, meta
}

// EncodeFileContent encodes a Go string back to file bytes, preserving the
// original encoding and line endings detected by DecodeFileContent.
//
// This matches upstream's writeTextContent() behavior:
//   - If encoding is UTF-16 LE → restore CRLF (if needed), then encode as UTF-16 LE with BOM
//   - If lineEndings is CRLF → restore CRLF before writing
//   - Otherwise → write as UTF-8
func EncodeFileContent(content string, meta FileMetadata) []byte {
	// Restore line endings first (content is internally LF-only)
	if meta.LineEndings == LineEndingCRLF {
		content = restoreCRLF(content)
	}

	// Encode based on detected encoding
	if meta.Encoding == EncodingUTF16LE {
		return encodeUTF16LE(content)
	}

	return []byte(content)
}

// ReadFileWithMetadata is a convenience function that reads a file and returns
// decoded content with encoding metadata. Used by file_read, file_edit,
// file_write, and multi_edit tools.
func ReadFileWithMetadata(data []byte) (string, FileMetadata) {
	return DecodeFileContent(data)
}

// --- Arbitrary encoding support (golang.org/x/text) ---

// DetectCharset detects the charset of file content using multiple strategies:
// 1. BOM detection (UTF-16 LE, UTF-8 BOM)
// 2. charset.DetermineEncoding (byte-pattern analysis from golang.org/x/net/html/charset)
// 3. UTF-8 validity check
//
// Returns the detected encoding name (lowercase, e.g. "gbk", "utf-8", "shift_jis")
// and whether the detection is considered certain.
func DetectCharset(data []byte, hint string) (encName string, certain bool) {
	// BOM detection first (reliable)
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		return "utf-16le", true
	}
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return "utf-8", true
	}
	if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		return "utf-16be", true
	}

	// Use charset.DetermineEncoding for byte-pattern analysis
	e, name, certain := charset.DetermineEncoding(data, hint)
	if e != nil && name != "" {
		return name, certain
	}

	// Check if content is valid UTF-8
	if bytes.HasPrefix(data, []byte("<?xml")) || bytes.HasPrefix(data, []byte("<!DOCTYPE")) {
		// HTML/XML files often have charset declarations
		// charset.DetermineEncoding already handles this, but as fallback:
		return "utf-8", false
	}

	// Default: assume UTF-8 if it passes validity check
	if isValidUTF8(data) {
		return "utf-8", true
	}

	// Not valid UTF-8 — likely some other encoding, but we can't determine which
	return "unknown", false
}

// DecodeWithEncoding decodes file bytes using a named encoding (e.g. "gbk", "shift_jis").
// Returns the decoded UTF-8 string.
func DecodeWithEncoding(data []byte, encName string) (string, error) {
	// Handle BOM-based encodings with our existing logic
	switch encName {
	case "utf-16le":
		u16s := bytesToUint16LE(stripBOM(data, 2))
		return string(utf16.Decode(u16s)), nil
	case "utf-8":
		content := string(stripBOM(data, 3))
		return content, nil
	}

	// Use golang.org/x/text for arbitrary encodings
	enc, err := htmlindex.Get(encName)
	if err != nil {
		return "", fmtEncodingError(encName)
	}

	decoder := enc.NewDecoder()
	decoded, err := decoder.Bytes(data)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

// EncodeWithEncoding encodes a UTF-8 Go string to bytes using a named encoding.
func EncodeWithEncoding(content string, encName string) ([]byte, error) {
	// Handle BOM-based encodings with our existing logic
	switch encName {
	case "utf-16le":
		return encodeUTF16LE(content), nil
	case "utf-8":
		return []byte(content), nil
	}

	// Use golang.org/x/text for arbitrary encodings
	enc, err := htmlindex.Get(encName)
	if err != nil {
		return nil, fmtEncodingError(encName)
	}

	encoder := enc.NewEncoder()
	encoded, err := encoder.Bytes([]byte(content))
	if err != nil {
		return nil, err
	}

	return encoded, nil
}

// IsSupportedEncoding checks if an encoding name is supported by golang.org/x/text.
func IsSupportedEncoding(encName string) bool {
	switch encName {
	case "utf-8", "utf-16le":
		return true
	}
	enc, err := htmlindex.Get(encName)
	return enc != nil && err == nil
}

// IsLikelyNonUTF8 checks if file content is likely not UTF-8 encoded.
// Used by existing tools to detect when they should suggest the file_encoding tool.
// Returns the detected encoding name if non-UTF-8, or empty string if likely UTF-8.
func IsLikelyNonUTF8(data []byte) string {
	// BOM-based detection is handled by DecodeFileContent already
	// (UTF-16 LE is supported by existing tools)
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		return "" // UTF-16 LE is supported
	}

	// Check if content is valid UTF-8
	if isValidUTF8(data) {
		return "" // Valid UTF-8, no issue
	}

	// Not valid UTF-8 — try to detect what encoding it might be
	encName, _ := DetectCharset(data, "")
	if encName != "utf-8" && encName != "unknown" && encName != "" {
		return encName
	}

	// Can't determine encoding, but it's definitely not UTF-8
	return "unknown"
}

// isValidUTF8 checks if data is valid UTF-8 by looking for invalid sequences.
// Uses a simple heuristic: if the data contains bytes that are not valid UTF-8
// start bytes or continuation bytes, it's likely not UTF-8.
func isValidUTF8(data []byte) bool {
	// Quick check: look for common non-UTF-8 patterns
	// Bytes 0x80-0xBF are continuation bytes and must follow a multi-byte start
	// Bytes 0xC0-0xC1 are overlong encodings (invalid in UTF-8)
	// Bytes 0xF5-0xFF are invalid UTF-8 start bytes
	for i := 0; i < len(data); i++ {
		b := data[i]
		if b >= 0xF5 && b <= 0xFF {
			return false // Invalid UTF-8 start byte
		}
		if b == 0xC0 || b == 0xC1 {
			return false // Overlong encoding
		}
	}
	// More thorough: check that the entire content is valid UTF-8
	// by scanning for proper multi-byte sequences
	i := 0
	for i < len(data) {
		b := data[i]
		if b < 0x80 {
			i++
			continue // ASCII byte, always valid
		}
		// Multi-byte sequence
		seqLen := 0
		if b >= 0xC2 && b <= 0xDF {
			seqLen = 2
		} else if b >= 0xE0 && b <= 0xEF {
			seqLen = 3
		} else if b >= 0xF0 && b <= 0xF4 {
			seqLen = 4
		} else {
			return false // Invalid start byte
		}
		if i+seqLen > len(data) {
			return false // Truncated sequence
		}
		for j := 1; j < seqLen; j++ {
			if data[i+j] < 0x80 || data[i+j] > 0xBF {
				return false // Invalid continuation byte
			}
		}
		i += seqLen
	}
	return true
}

// stripBOM removes a BOM of the specified size from the beginning of data.
func stripBOM(data []byte, bomSize int) []byte {
	if len(data) >= bomSize {
		return data[bomSize:]
	}
	return data
}

// fmtEncodingError returns a user-friendly error for unsupported encoding names.
func fmtEncodingError(encName string) error {
	return fmtError("unsupported encoding: %s. Supported encodings include: utf-8, utf-16le, gbk, gb18030, big5, shift_jis, euc-jp, euc-kr, iso-8859-1 (latin-1), windows-1252, and more. See https://www.iana.org/assignments/character-sets for the full list", encName)
}

// fmtError is a simple error formatter.
func fmtError(format string, args ...any) error {
	return &encodingError{msg: format, args: args}
}

type encodingError struct {
	msg  string
	args []any
}

func (e *encodingError) Error() string {
	return fmt.Sprintf(e.msg, e.args...)
}

// bytesToUint16LE converts a little-endian byte slice to []uint16.
func bytesToUint16LE(b []byte) []uint16 {
	if len(b)%2 != 0 {
		b = b[:len(b)-1] // drop trailing odd byte
	}
	u16s := make([]uint16, len(b)/2)
	for i := range u16s {
		u16s[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return u16s
}

// encodeUTF16LE encodes a Go string as UTF-16 LE with BOM prefix.
// Used to preserve the original file encoding when writing back.
func encodeUTF16LE(s string) []byte {
	runes := []rune(s)
	u16s := utf16.Encode(runes)
	// BOM + UTF-16 LE (little-endian): 2 bytes per uint16
	out := make([]byte, 2+2*len(u16s))
	out[0] = 0xFF // BOM low byte
	out[1] = 0xFE // BOM high byte
	for i, v := range u16s {
		out[2+2*i] = byte(v)        // low byte
		out[2+2*i+1] = byte(v >> 8) // high byte
	}
	return out
}

// restoreCRLF replaces \n with \r\n only where not already preceded by \r.
func restoreCRLF(s string) string {
	var b strings.Builder
	b.Grow(len(s) + len(s)/10)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			b.WriteString("\r\n")
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}