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
	EncodingUTF16BE = "utf-16be"
)

// LineEnding constants.
const (
	LineEndingLF   = "LF"
	LineEndingCRLF = "CRLF"
)

// FileMetadata holds encoding and line-ending information detected from a file.
type FileMetadata struct {
	Encoding    string // "utf-8", "utf-16le", or "utf-16be"
	LineEndings string // "LF" or "CRLF"
	HasBOM      bool   // true if the original file had a BOM (UTF-8 or UTF-16)
}

// DecodeFileContent reads raw file bytes, detects encoding via BOM, decodes
// to a Go string, and normalizes CRLF to LF. Returns the decoded content
// and metadata about the original encoding and line endings.
//
// This matches upstream's readFileSyncWithMetadata() behavior:
//   - UTF-16 LE BOM (FF FE) → decode via utf16 LE, encoding="utf-16le"
//   - UTF-16 BE BOM (FE FF) → decode via utf16 BE, encoding="utf-16be"
//   - UTF-16 LE without BOM → heuristic detection (null byte pattern), encoding="utf-16le"
//   - UTF-16 BE without BOM → heuristic detection (null byte pattern), encoding="utf-16be"
//   - UTF-8 BOM (EF BB BF) → strip BOM, encoding="utf-8", hasBOM=true
//   - Otherwise → treat as UTF-8, encoding="utf-8"
//   - CRLF → normalize to LF for internal use, lineEndings="CRLF"
func DecodeFileContent(data []byte) (content string, meta FileMetadata) {
	meta = FileMetadata{
		Encoding:    EncodingUTF8,
		LineEndings: LineEndingLF,
	}

	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		// UTF-16 LE BOM detected
		meta.Encoding = EncodingUTF16LE
		meta.HasBOM = true
		u16s := bytesToUint16LE(data[2:])
		content = string(utf16.Decode(u16s))
	} else if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		// UTF-16 BE BOM detected
		meta.Encoding = EncodingUTF16BE
		meta.HasBOM = true
		u16s := bytesToUint16BE(data[2:])
		content = string(utf16.Decode(u16s))
	} else if detectUTF16LEWithoutBOM(data) {
		// UTF-16 LE without BOM (heuristic)
		meta.Encoding = EncodingUTF16LE
		u16s := bytesToUint16LE(data)
		content = string(utf16.Decode(u16s))
	} else if detectUTF16BEWithoutBOM(data) {
		// UTF-16 BE without BOM (heuristic)
		meta.Encoding = EncodingUTF16BE
		u16s := bytesToUint16BE(data)
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

	// Strip UTF-8 BOM but record that it existed (matching official behavior)
	if strings.HasPrefix(content, "\xEF\xBB\xBF") {
		content = content[3:]
		meta.HasBOM = true
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
	switch meta.Encoding {
	case EncodingUTF16LE:
		return encodeUTF16LE(content)
	case EncodingUTF16BE:
		return encodeUTF16BE(content)
	}

	// Restore UTF-8 BOM if the original file had one
	if meta.HasBOM {
		return append([]byte{0xEF, 0xBB, 0xBF}, []byte(content)...)
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

// detectUTF16LEWithoutBOM uses heuristics to detect UTF-16 LE files that lack a BOM.
// UTF-16 LE has a distinctive pattern: ASCII characters appear as pairs where the
// second byte is 0x00 (e.g., 'H' = 0x48 0x00, 'e' = 0x65 0x00).
// Returns true if the data strongly resembles UTF-16 LE encoding.
func detectUTF16LEWithoutBOM(data []byte) bool {
	if len(data) < 4 || len(data)%2 != 0 {
		return false
	}

	// Count how many 2-byte pairs have the pattern: non-zero byte followed by 0x00
	// This is the signature of ASCII/low-Unicode chars in UTF-16 LE
	nullSecond := 0
	totalPairs := len(data) / 2
	for i := 0; i < totalPairs; i++ {
		lo := data[2*i]
		hi := data[2*i+1]
		// ASCII range: lo is printable ASCII (0x20-0x7E) or control chars (0x0A, 0x0D),
		// hi is 0x00
		if hi == 0x00 && (lo >= 0x20 && lo <= 0x7E || lo == 0x0A || lo == 0x0D || lo == 0x09) {
			nullSecond++
		}
	}

	// If more than 70% of pairs match the UTF-16 LE ASCII pattern, it's very likely UTF-16 LE
	// This threshold avoids false positives on pure ASCII (where every byte is < 0x80)
	// because in pure ASCII, the "hi" bytes would also be ASCII chars, not 0x00
	if totalPairs > 0 && float64(nullSecond)/float64(totalPairs) > 0.70 {
		return true
	}

	return false
}

// detectUTF16BEWithoutBOM uses heuristics to detect UTF-16 BE files that lack a BOM.
// UTF-16 BE has a distinctive pattern: ASCII characters appear as pairs where the
// first byte is 0x00 (e.g., 'H' = 0x00 0x48, 'e' = 0x00 0x65).
func detectUTF16BEWithoutBOM(data []byte) bool {
	if len(data) < 4 || len(data)%2 != 0 {
		return false
	}

	nullFirst := 0
	totalPairs := len(data) / 2
	for i := 0; i < totalPairs; i++ {
		hi := data[2*i]
		lo := data[2*i+1]
		if hi == 0x00 && (lo >= 0x20 && lo <= 0x7E || lo == 0x0A || lo == 0x0D || lo == 0x09) {
			nullFirst++
		}
	}

	if totalPairs > 0 && float64(nullFirst)/float64(totalPairs) > 0.70 {
		return true
	}

	return false
}

// DetectCharset detects the charset of file content using multiple strategies:
// 1. BOM detection (UTF-16 LE/BE, UTF-8 BOM)
// 2. Heuristic UTF-16 LE/BE detection (null-byte patterns)
// 3. charset.DetermineEncoding (byte-pattern analysis from golang.org/x/net/html/charset)
// 4. UTF-8 validity check
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

	// Heuristic UTF-16 LE/BE detection (without BOM)
	if detectUTF16LEWithoutBOM(data) {
		return "utf-16le", false
	}
	if detectUTF16BEWithoutBOM(data) {
		return "utf-16be", false
	}

	// Use charset.DetermineEncoding for byte-pattern analysis
	e, name, certain := charset.DetermineEncoding(data, hint)
	if e != nil && name != "" {
		return name, certain
	}

	// Check if content is valid UTF-8
	if bytes.HasPrefix(data, []byte("<?xml")) || bytes.HasPrefix(data, []byte("<!DOCTYPE")) {
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
	switch encName {
	case "utf-16le":
		u16s := bytesToUint16LE(stripBOM(data, 2))
		return string(utf16.Decode(u16s)), nil
	case "utf-16be":
		u16s := bytesToUint16BE(stripBOM(data, 2))
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
	switch encName {
	case "utf-16le":
		return encodeUTF16LE(content), nil
	case "utf-16be":
		return encodeUTF16BE(content), nil
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
	case "utf-8", "utf-16le", "utf-16be":
		return true
	}
	enc, err := htmlindex.Get(encName)
	return enc != nil && err == nil
}

// IsLikelyNonUTF8 checks if file content is likely not UTF-8 encoded.
// Used by existing tools to detect when they should suggest the file_encoding tool.
// Returns the detected encoding name if non-UTF-8, or empty string if likely UTF-8
// or if the encoding is handled by the existing tools (UTF-16 LE/BE, with or without BOM).
func IsLikelyNonUTF8(data []byte) string {
	// UTF-16 LE with BOM is fully supported by existing tools
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		return ""
	}
	// UTF-16 BE with BOM is fully supported by existing tools
	if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		return ""
	}

	// UTF-16 LE without BOM is now supported by DecodeFileContent (heuristic detection)
	if detectUTF16LEWithoutBOM(data) {
		return ""
	}
	// UTF-16 BE without BOM is now supported by DecodeFileContent (heuristic detection)
	if detectUTF16BEWithoutBOM(data) {
		return ""
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

// stripBOM removes a BOM from the beginning of data only if it matches the expected BOM bytes.
func stripBOM(data []byte, bomSize int) []byte {
	switch bomSize {
	case 2:
		// UTF-16 LE BOM: FF FE, or UTF-16 BE BOM: FE FF
		if len(data) >= 2 && ((data[0] == 0xFF && data[1] == 0xFE) || (data[0] == 0xFE && data[1] == 0xFF)) {
			return data[2:]
		}
	case 3:
		// UTF-8 BOM: EF BB BF
		if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
			return data[3:]
		}
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

// bytesToUint16BE converts a big-endian byte slice to []uint16.
func bytesToUint16BE(b []byte) []uint16 {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u16s := make([]uint16, len(b)/2)
	for i := range u16s {
		u16s[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
	}
	return u16s
}

// encodeUTF16BE encodes a Go string as UTF-16 BE with BOM prefix.
func encodeUTF16BE(s string) []byte {
	runes := []rune(s)
	u16s := utf16.Encode(runes)
	out := make([]byte, 2+2*len(u16s))
	out[0] = 0xFE // BOM high byte
	out[1] = 0xFF // BOM low byte
	for i, v := range u16s {
		out[2+2*i] = byte(v >> 8)   // high byte
		out[2+2*i+1] = byte(v)      // low byte
	}
	return out
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