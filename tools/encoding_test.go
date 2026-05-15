package tools

import (
	"testing"
	"unicode/utf16"
)

func TestStripBOM(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		bomSize  int
		expected []byte
	}{
		{"UTF-8 no BOM", []byte("你好世界"), 3, []byte("你好世界")},
		{"UTF-8 with BOM", append([]byte{0xEF, 0xBB, 0xBF}, []byte("你好世界")...), 3, []byte("你好世界")},
		{"UTF-16 LE with BOM", []byte{0xFF, 0xFE, 0x48, 0x00}, 2, []byte{0x48, 0x00}},
		{"UTF-16 LE no BOM", []byte{0x48, 0x00, 0x65, 0x00}, 2, []byte{0x48, 0x00, 0x65, 0x00}},
		{"UTF-16 BE with BOM", []byte{0xFE, 0xFF, 0x00, 0x48}, 2, []byte{0x00, 0x48}},
		{"UTF-16 BE no BOM", []byte{0x00, 0x48, 0x00, 0x65}, 2, []byte{0x00, 0x48, 0x00, 0x65}},
		{"Empty data", []byte{}, 3, []byte{}},
		{"Short data", []byte{0x48}, 3, []byte{0x48}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripBOM(tt.data, tt.bomSize)
			if string(result) != string(tt.expected) {
				t.Errorf("stripBOM() = %x, want %x", result, tt.expected)
			}
		})
	}
}

func TestDecodeWithEncoding(t *testing.T) {
	// UTF-8 without BOM — must NOT lose first character
	data := []byte("你好世界")
	decoded, err := DecodeWithEncoding(data, "utf-8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded != "你好世界" {
		t.Errorf("DecodeWithEncoding utf-8 no BOM = %q, want %q", decoded, "你好世界")
	}

	// UTF-8 with BOM — must strip BOM and decode correctly
	bomData := append([]byte{0xEF, 0xBB, 0xBF}, []byte("你好世界")...)
	decoded2, err2 := DecodeWithEncoding(bomData, "utf-8")
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if decoded2 != "你好世界" {
		t.Errorf("DecodeWithEncoding utf-8 with BOM = %q, want %q", decoded2, "你好世界")
	}
}

func TestDecodeFileContent_UTF8NoBOM(t *testing.T) {
	data := []byte("你好世界\n第二行\n")
	content, meta := DecodeFileContent(data)
	if meta.Encoding != "utf-8" {
		t.Errorf("encoding = %q, want %q", meta.Encoding, "utf-8")
	}
	if meta.HasBOM {
		t.Errorf("HasBOM = true, want false")
	}
	if content != "你好世界\n第二行\n" {
		t.Errorf("content = %q, want %q", content, "你好世界\n第二行\n")
	}
}

func TestDecodeFileContent_UTF8WithBOM(t *testing.T) {
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("你好世界\n")...)
	content, meta := DecodeFileContent(data)
	if meta.Encoding != "utf-8" {
		t.Errorf("encoding = %q, want %q", meta.Encoding, "utf-8")
	}
	if !meta.HasBOM {
		t.Errorf("HasBOM = false, want true")
	}
	if content != "你好世界\n" {
		t.Errorf("content = %q, want %q", content, "你好世界\n")
	}
	// Verify round-trip preserves BOM
	encoded := EncodeFileContent(content, meta)
	if len(encoded) < 3 || encoded[0] != 0xEF || encoded[1] != 0xBB || encoded[2] != 0xBF {
		t.Errorf("round-trip: BOM not preserved, first 3 bytes: %x", encoded[:min(3, len(encoded))])
	}
}

func TestDecodeFileContent_UTF16LE(t *testing.T) {
	text := "Hello\n你好\n"
	runes := []rune(text)
	u16s := utf16.Encode(runes)

	// With BOM
	leBOM := []byte{0xFF, 0xFE}
	for _, v := range u16s {
		leBOM = append(leBOM, byte(v), byte(v>>8))
	}
	content, meta := DecodeFileContent(leBOM)
	if meta.Encoding != "utf-16le" {
		t.Errorf("encoding = %q, want %q", meta.Encoding, "utf-16le")
	}
	if content != text {
		t.Errorf("content = %q, want %q", content, text)
	}

	// Without BOM (heuristic detection)
	leNoBOM := leBOM[2:]
	content2, meta2 := DecodeFileContent(leNoBOM)
	if meta2.Encoding != "utf-16le" {
		t.Errorf("encoding = %q, want %q", meta2.Encoding, "utf-16le")
	}
	if content2 != text {
		t.Errorf("content = %q, want %q", content2, text)
	}
}

func TestDecodeFileContent_UTF16BE(t *testing.T) {
	text := "Hello\n你好\n"
	runes := []rune(text)
	u16s := utf16.Encode(runes)

	// With BOM
	beBOM := []byte{0xFE, 0xFF}
	for _, v := range u16s {
		beBOM = append(beBOM, byte(v>>8), byte(v))
	}
	content, meta := DecodeFileContent(beBOM)
	if meta.Encoding != "utf-16be" {
		t.Errorf("encoding = %q, want %q", meta.Encoding, "utf-16be")
	}
	if content != text {
		t.Errorf("content = %q, want %q", content, text)
	}

	// Without BOM (heuristic detection)
	beNoBOM := beBOM[2:]
	content2, meta2 := DecodeFileContent(beNoBOM)
	if meta2.Encoding != "utf-16be" {
		t.Errorf("encoding = %q, want %q", meta2.Encoding, "utf-16be")
	}
	if content2 != text {
		t.Errorf("content = %q, want %q", content2, text)
	}
}

func TestDetectUTF16LEWithoutBOM(t *testing.T) {
	text := "Hello World\nTest line\n"
	runes := []rune(text)
	u16s := utf16.Encode(runes)
	data := make([]byte, 0, len(u16s)*2)
	for _, v := range u16s {
		data = append(data, byte(v), byte(v>>8))
	}
	if !detectUTF16LEWithoutBOM(data) {
		t.Error("detectUTF16LEWithoutBOM should return true for UTF-16 LE data")
	}
	// Plain ASCII should not be detected as UTF-16 LE
	if detectUTF16LEWithoutBOM([]byte("Hello World\n")) {
		t.Error("detectUTF16LEWithoutBOM should return false for plain ASCII")
	}
}

func TestDetectUTF16BEWithoutBOM(t *testing.T) {
	text := "Hello World\nTest line\n"
	runes := []rune(text)
	u16s := utf16.Encode(runes)
	data := make([]byte, 0, len(u16s)*2)
	for _, v := range u16s {
		data = append(data, byte(v>>8), byte(v))
	}
	if !detectUTF16BEWithoutBOM(data) {
		t.Error("detectUTF16BEWithoutBOM should return true for UTF-16 BE data")
	}
	// Plain ASCII should not be detected as UTF-16 BE
	if detectUTF16BEWithoutBOM([]byte("Hello World\n")) {
		t.Error("detectUTF16BEWithoutBOM should return false for plain ASCII")
	}
}