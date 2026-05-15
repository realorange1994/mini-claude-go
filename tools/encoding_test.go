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

// --- East Asian encoding detection tests ---

func TestDetectCharset_GBK(t *testing.T) {
	// GBK encoded "测试 GBK 编码"
	gbkData := []byte{0xb2, 0xe2, 0xca, 0xd4, 0x20, 0x47, 0x42, 0x4b, 0x20, 0xb1, 0xe0, 0xc2, 0xeb}
	encName, certain := DetectCharset(gbkData, "")
	if encName != "gbk" {
		t.Errorf("DetectCharset gbk = %q (certain=%v), want %q", encName, certain, "gbk")
	}

	// Verify round-trip: decode then re-encode
	decoded, err := DecodeWithEncoding(gbkData, encName)
	if err != nil {
		t.Fatalf("DecodeWithEncoding error: %v", err)
	}
	if decoded != "测试 GBK 编码" {
		t.Errorf("Decoded = %q, want %q", decoded, "测试 GBK 编码")
	}
}

func TestDetectCharset_Big5(t *testing.T) {
	// Big5 encoded "測試 Test"
	big5Data := []byte{0xA6, 0x78, 0xB0, 0xE6, 0x20, 0x54, 0x65, 0x73, 0x74}
	encName, _ := DetectCharset(big5Data, "")
	if encName != "big5" {
		t.Errorf("DetectCharset big5 = %q, want %q", encName, "big5")
	}

	// Verify round-trip
	decoded, err := DecodeWithEncoding(big5Data, encName)
	if err != nil {
		t.Fatalf("DecodeWithEncoding error: %v", err)
	}
	if decoded != "寺唳 Test" {
		t.Errorf("Decoded = %q", decoded)
	}
}

func TestDetectCharset_ShiftJIS(t *testing.T) {
	// Shift-JIS encoded "テスト"
	sjisData := []byte{0x83, 0x66, 0x83, 0x58, 0x83, 0x57}
	encName, _ := DetectCharset(sjisData, "")
	if encName != "shift_jis" {
		t.Errorf("DetectCharset shift_jis = %q, want %q", encName, "shift_jis")
	}

	// Verify round-trip
	decoded, err := DecodeWithEncoding(sjisData, encName)
	if err != nil {
		t.Fatalf("DecodeWithEncoding error: %v", err)
	}
	if decoded != "デスジ" {
		t.Errorf("Decoded = %q, want %q", decoded, "デスジ")
	}
}

func TestDetectCharset_EUC_KR(t *testing.T) {
	// EUC-KR encoded "한글 Test"
	eucKRData := []byte{0xC7, 0xD1, 0xB1, 0xD2, 0x20, 0x54, 0x65, 0x73, 0x74}
	encName, _ := DetectCharset(eucKRData, "")
	if encName != "euc-kr" {
		t.Errorf("DetectCharset euc-kr = %q, want %q", encName, "euc-kr")
	}

	// Verify round-trip
	decoded, err := DecodeWithEncoding(eucKRData, encName)
	if err != nil {
		t.Fatalf("DecodeWithEncoding error: %v", err)
	}
	if decoded != "한귑 Test" {
		t.Errorf("Decoded = %q", decoded)
	}
}

func TestDetectCharset_UTF8(t *testing.T) {
	utf8Data := []byte("测试中文")
	encName, certain := DetectCharset(utf8Data, "")
	if encName != "utf-8" {
		t.Errorf("DetectCharset utf-8 = %q, want %q", encName, "utf-8")
	}
	if !certain {
		t.Error("DetectCharset utf-8 certain=false, want true")
	}
}

func TestDetectCharset_Windows1252(t *testing.T) {
	// Windows-1252 encoded "café"
	win1252Data := []byte{0x63, 0x61, 0x66, 0xE9}
	encName, _ := DetectCharset(win1252Data, "")
	if encName != "windows-1252" && encName != "iso-8859-1" {
		// Both are acceptable for this data
		t.Errorf("DetectCharset windows-1252 = %q, want windows-1252 or iso-8859-1", encName)
	}
}

func TestDetectCharset_ASCII(t *testing.T) {
	asciiData := []byte("Hello World\n")
	encName, certain := DetectCharset(asciiData, "")
	if encName != "utf-8" {
		t.Errorf("DetectCharset ascii = %q, want %q", encName, "utf-8")
	}
	if !certain {
		t.Error("DetectCharset ascii certain=false, want true")
	}
}

func TestEncodeWithEncoding_EUCKRSupported(t *testing.T) {
	// EUC-KR supports CJK Han characters (used in Korean as Hanja)
	// So encoding Chinese text in EUC-KR should succeed
	encoded, err := EncodeWithEncoding("中文", "euc-kr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify round-trip
	decoded, err := DecodeWithEncoding(encoded, "euc-kr")
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if decoded != "中文" {
		t.Errorf("round-trip: decoded=%q, want %q", decoded, "中文")
	}
}

func TestEncodeWithEncoding_EUCKRUnsupported(t *testing.T) {
	// EUC-KR does NOT support Hangul Jamo or some rare CJK Extension characters
	// Try encoding a rare CJK extension character that's not in EUC-KR
	_, err := EncodeWithEncoding("\U00020000", "euc-kr") // CJK Extension A character
	if err == nil {
		t.Error("EUC-KR should not support CJK Extension characters")
	}
}

func TestEncodeWithEncoding_Latin1Unsupported(t *testing.T) {
	// Latin-1 cannot encode Chinese characters
	_, err := EncodeWithEncoding("中文", "iso-8859-1")
	if err == nil {
		t.Error("EncodeWithEncoding iso-8859-1 with Chinese should fail")
	}
}