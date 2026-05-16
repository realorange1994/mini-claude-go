package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// ─── normalizeQuotes ────────────────────────────────────────────────────────

func TestNormalizeQuotesCurlyDouble(t *testing.T) {
	input := "\u201Chello\u201D"
	result := normalizeQuotes(input)
	if result != `"hello"` {
		t.Errorf("expected '\"hello\"', got %q", result)
	}
}

func TestNormalizeQuotesCurlySingle(t *testing.T) {
	input := "\u2018hello\u2019"
	result := normalizeQuotes(input)
	if result != "'hello'" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestNormalizeQuotesStraight(t *testing.T) {
	input := `"hello"`
	result := normalizeQuotes(input)
	if result != `"hello"` {
		t.Errorf("straight quotes should be unchanged, got %q", result)
	}
}

func TestNormalizeQuotesMixed(t *testing.T) {
	input := "\u201Cfoo\u201D and \u2018bar\u2019"
	result := normalizeQuotes(input)
	if result != `"foo" and 'bar'` {
		t.Errorf("expected '\"foo\" and \\'bar\\'', got %q", result)
	}
}

// ─── curlyToStraightDouble ──────────────────────────────────────────────────

func TestCurlyToStraightDouble(t *testing.T) {
	result := curlyToStraightDouble("\u201Chello\u201D")
	if result != `"hello"` {
		t.Errorf("expected '\"hello\"', got %q", result)
	}
}

func TestCurlyToStraightDoubleNoCurly(t *testing.T) {
	result := curlyToStraightDouble(`"hello"`)
	if result != `"hello"` {
		t.Errorf("straight quotes unchanged, got %q", result)
	}
}

// ─── curlyToStraightSingle ──────────────────────────────────────────────────

func TestCurlyToStraightSingle(t *testing.T) {
	result := curlyToStraightSingle("\u2018hello\u2019")
	if result != "'hello'" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

// ─── straightToCurlyDouble ─────────────────────────────────────────────────

func TestStraightToCurlyDouble(t *testing.T) {
	result := straightToCurlyDouble(`"hello"`)
	if result != "\u201Chello\u201D" {
		t.Errorf("expected curly double quotes, got %q", result)
	}
}

func TestStraightToCurlyDoubleNested(t *testing.T) {
	result := straightToCurlyDouble(`say "hello" now`)
	if result != "say \u201Chello\u201D now" {
		t.Errorf("expected curly in context, got %q", result)
	}
}

// ─── straightToCurlySingle ─────────────────────────────────────────────────

func TestStraightToCurlySingleContraction(t *testing.T) {
	// "don't" — apostrophe between letters should be right curly
	result := straightToCurlySingle("don't")
	if !strings.Contains(result, "\u2019") {
		t.Errorf("contraction should use right curly quote, got %q", result)
	}
}

func TestStraightToCurlySingleOpening(t *testing.T) {
	// Single quote at start of string should be left curly
	result := straightToCurlySingle("'hello")
	if !strings.HasPrefix(result, "\u2018") {
		t.Errorf("opening single quote should be left curly, got %q", result)
	}
}

// ─── isOpeningQuoteContext ──────────────────────────────────────────────────

func TestIsOpeningQuoteContextSpace(t *testing.T) {
	if !isOpeningQuoteContext(' ') {
		t.Error("space should be opening context")
	}
}

func TestIsOpeningQuoteContextParen(t *testing.T) {
	if !isOpeningQuoteContext('(') {
		t.Error("paren should be opening context")
	}
}

func TestIsOpeningQuoteContextLetter(t *testing.T) {
	if isOpeningQuoteContext('a') {
		t.Error("letter should not be opening context")
	}
}

func TestIsOpeningQuoteContextNewline(t *testing.T) {
	if !isOpeningQuoteContext('\n') {
		t.Error("newline should be opening context")
	}
}

// ─── isLetter ───────────────────────────────────────────────────────────────

func TestIsLetterLower(t *testing.T) {
	if !isLetter('a') || !isLetter('z') {
		t.Error("lowercase letters should be letters")
	}
}

func TestIsLetterUpper(t *testing.T) {
	if !isLetter('A') || !isLetter('Z') {
		t.Error("uppercase letters should be letters")
	}
}

func TestIsLetterDigit(t *testing.T) {
	if isLetter('5') {
		t.Error("digit should not be letter")
	}
}

func TestIsLetterSymbol(t *testing.T) {
	if isLetter('-') {
		t.Error("symbol should not be letter")
	}
}

// ─── stripTrailingWhitespace ────────────────────────────────────────────────

func TestStripTrailingWhitespaceSpaces(t *testing.T) {
	result := stripTrailingWhitespace("hello   \nworld  ")
	if result != "hello\nworld" {
		t.Errorf("expected 'hello\\nworld', got %q", result)
	}
}

func TestStripTrailingWhitespaceTabs(t *testing.T) {
	result := stripTrailingWhitespace("line1\t\t\nline2\t")
	if result != "line1\nline2" {
		t.Errorf("expected 'line1\\nline2', got %q", result)
	}
}

func TestStripTrailingWhitespaceNone(t *testing.T) {
	input := "hello\nworld"
	result := stripTrailingWhitespace(input)
	if result != input {
		t.Errorf("no trailing whitespace should be unchanged, got %q", result)
	}
}

func TestStripTrailingWhitespaceEmpty(t *testing.T) {
	result := stripTrailingWhitespace("")
	if result != "" {
		t.Errorf("empty string should remain empty, got %q", result)
	}
}

// ─── restoreCRLF ────────────────────────────────────────────────────────────

func TestRestoreCRLFBasic(t *testing.T) {
	result := restoreCRLF("line1\nline2\n")
	if result != "line1\r\nline2\r\n" {
		t.Errorf("expected CRLF, got %q", result)
	}
}

func TestRestoreCRLFAlreadyCRLFEdit(t *testing.T) {
	input := "line1\r\nline2\r\n"
	result := restoreCRLF(input)
	if result != input {
		t.Errorf("already CRLF should be unchanged, got %q", result)
	}
}

func TestRestoreCRLFEmptyEdit(t *testing.T) {
	if restoreCRLF("") != "" {
		t.Error("empty string should remain empty")
	}
}

// ─── applyReplacement ───────────────────────────────────────────────────────

func TestApplyReplacementSingle(t *testing.T) {
	result := applyReplacement("hello world", "world", "there", false)
	if result != "hello there" {
		t.Errorf("expected 'hello there', got %q", result)
	}
}

func TestApplyReplacementAll(t *testing.T) {
	result := applyReplacement("aaa", "a", "b", true)
	if result != "bbb" {
		t.Errorf("expected 'bbb', got %q", result)
	}
}

func TestApplyReplacementNotAll(t *testing.T) {
	result := applyReplacement("aaa", "a", "b", false)
	if result != "baa" {
		t.Errorf("expected 'baa', got %q", result)
	}
}

func TestApplyReplacementNoMatch(t *testing.T) {
	result := applyReplacement("hello", "xyz", "abc", false)
	if result != "hello" {
		t.Errorf("no match should be unchanged, got %q", result)
	}
}

// ─── preserveQuoteStyle ────────────────────────────────────────────────────

func TestPreserveQuoteStyleNoNorm(t *testing.T) {
	// When oldStr == oldStrNorm (no normalization), return newStr as-is
	result := preserveQuoteStyle("content", "old", "new", "old")
	if result != "new" {
		t.Errorf("expected 'new', got %q", result)
	}
}

func TestPreserveQuoteStyleWithNorm(t *testing.T) {
	// When oldStr != oldStrNorm, should try to match quote style
	content := "\u201Chello\u201D"
	oldStr := `"hello"` // straight quotes (what user typed)
	oldStrNorm := `"hello"`
	newStr := `"world"`
	result := preserveQuoteStyle(content, oldStr, newStr, oldStrNorm)
	// oldStr == oldStrNorm here, so returns newStr as-is
	if result != `"world"` {
		t.Errorf("expected '\"world\"', got %q", result)
	}
}

// ─── encodeUTF16LE / bytesToUint16LE ────────────────────────────────────────

func TestEncodeUTF16LE(t *testing.T) {
	input := "AB"
	result := encodeUTF16LE(input)
	// BOM + 'A' + 'B' in UTF-16 LE
	if len(result) != 6 { // 2 BOM + 2*2 chars
		t.Errorf("expected 6 bytes, got %d", len(result))
	}
	if result[0] != 0xFF || result[1] != 0xFE {
		t.Error("should start with UTF-16 LE BOM")
	}
}

func TestBytesToUint16LE(t *testing.T) {
	// Encode "AB" as UTF-16 LE (no BOM)
	b := []byte{0x41, 0x00, 0x42, 0x00} // A=0x0041, B=0x0042 in LE
	result := bytesToUint16LE(b)
	if len(result) != 2 {
		t.Fatalf("expected 2 uint16s, got %d", len(result))
	}
	if result[0] != 0x0041 || result[1] != 0x0042 {
		t.Errorf("expected [0x41, 0x42], got %v", result)
	}
}

func TestBytesToUint16LEOddLength(t *testing.T) {
	b := []byte{0x41, 0x00, 0x42} // odd trailing byte
	result := bytesToUint16LE(b)
	if len(result) != 1 {
		t.Errorf("odd length should drop trailing byte, got %d uint16s", len(result))
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	input := "Hello, 世界!"
	encoded := encodeUTF16LE(input)
	// Skip BOM
	decoded := string(utf16.Decode(bytesToUint16LE(encoded[2:])))
	if decoded != input {
		t.Errorf("round trip failed: expected %q, got %q", input, decoded)
	}
}

// ─── FileEditTool interface ─────────────────────────────────────────────────

func TestFileEditToolName(t *testing.T) {
	tool := &FileEditTool{}
	if tool.Name() != "edit_file" {
		t.Errorf("expected 'edit_file', got %q", tool.Name())
	}
}

func TestFileEditToolSchema(t *testing.T) {
	tool := &FileEditTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 3 {
		t.Errorf("expected 3 required params, got %d", len(required))
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["file_path"]; !ok {
		t.Error("schema should have file_path property")
	}
	if _, ok := props["old_string"]; !ok {
		t.Error("schema should have old_string property")
	}
	if _, ok := props["new_string"]; !ok {
		t.Error("schema should have new_string property")
	}
	if _, ok := props["replace_all"]; !ok {
		t.Error("schema should have replace_all property")
	}
}

func TestFileEditToolCheckPermissionsEmpty(t *testing.T) {
	tool := &FileEditTool{}
	result := tool.CheckPermissions(map[string]any{})
	if result.Behavior != PermissionPassthrough {
		t.Error("empty path should passthrough")
	}
}

func TestFileEditToolCheckPermissionsSafe(t *testing.T) {
	tool := &FileEditTool{}
	result := tool.CheckPermissions(map[string]any{"file_path": "main.go"})
	if result.Behavior != PermissionPassthrough {
		t.Error("safe path should passthrough")
	}
}

func TestFileEditToolCheckPermissionsDangerous(t *testing.T) {
	tool := &FileEditTool{}
	result := tool.CheckPermissions(map[string]any{"file_path": ".bashrc"})
	if result.Behavior != PermissionAsk {
		t.Errorf("dangerous path should ask, got %v", result.Behavior)
	}
}

func TestFileEditToolExecuteNoPath(t *testing.T) {
	tool := &FileEditTool{}
	result := tool.Execute(map[string]any{"old_string": "a", "new_string": "b"})
	if !result.IsError {
		t.Error("missing file_path should return error")
	}
}

func TestFileEditToolExecuteSameStrings(t *testing.T) {
	tool := &FileEditTool{}
	result := tool.Execute(map[string]any{
		"file_path":  "/tmp/test.txt",
		"old_string": "same",
		"new_string": "same",
	})
	if !result.IsError {
		t.Error("identical old/new strings should return error")
	}
}

func TestFileEditToolExecuteUNC(t *testing.T) {
	tool := &FileEditTool{}
	result := tool.Execute(map[string]any{
		"file_path":  `\\server\share\file.txt`,
		"old_string": "a",
		"new_string": "b",
	})
	if !result.IsError {
		t.Error("UNC path should return error")
	}
}

// ─── NewFileEditTool ────────────────────────────────────────────────────────

func TestNewFileEditTool(t *testing.T) {
	r := NewRegistry()
	tool := NewFileEditTool(r)
	if tool == nil {
		t.Fatal("NewFileEditTool should not return nil")
	}
	if tool.Name() != "edit_file" {
		t.Errorf("expected 'edit_file', got %q", tool.Name())
	}
}

// ─── edit_file empty old_string error message ────────────────────────────────

func TestFileEditEmptyOldStringExistingFile(t *testing.T) {
	// When old_string="" and file exists with content, the error message
	// should clearly explain the situation, not just say "file already exists"
	dir := t.TempDir()
	fp := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(fp, []byte("some content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	// Mark file as read so the stale check passes
	registry.MarkFileRead(fp)

	tool := NewFileEditTool(registry)
	result := tool.Execute(map[string]any{
		"file_path":  fp,
		"old_string": "",
		"new_string": "new content",
	})
	if !result.IsError {
		t.Error("empty old_string on existing file with content should return error")
	}
	// The error message should mention the file path and explain the issue clearly
	if !strings.Contains(result.Output, "cannot create new file") {
		t.Errorf("error should mention 'cannot create new file', got: %q", result.Output)
	}
	if !strings.Contains(result.Output, "already exists") {
		t.Errorf("error should mention 'already exists', got: %q", result.Output)
	}
	if !strings.Contains(result.Output, "non-empty old_string") {
		t.Errorf("error should suggest using non-empty old_string, got: %q", result.Output)
	}
}

func TestFileEditEmptyOldStringNewFile(t *testing.T) {
	// When old_string="" and file doesn't exist, it should create the file
	dir := t.TempDir()
	fp := filepath.Join(dir, "newfile.txt")

	registry := NewRegistry()
	tool := NewFileEditTool(registry)
	result := tool.Execute(map[string]any{
		"file_path":  fp,
		"old_string": "",
		"new_string": "hello world",
	})
	if result.IsError {
		t.Errorf("empty old_string on nonexistent file should create file, got: %q", result.Output)
	}
	if !strings.Contains(result.Output, "Successfully created") {
		t.Errorf("should say 'Successfully created', got: %q", result.Output)
	}

	// Verify file was created with correct content
	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Errorf("file content = %q, want 'hello world'", string(data))
	}
}

func TestFileEditEmptyOldStringEmptyFile(t *testing.T) {
	// When old_string="" and file exists but is empty, it should write to it
	dir := t.TempDir()
	fp := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(fp, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	registry.MarkFileRead(fp)

	tool := NewFileEditTool(registry)
	result := tool.Execute(map[string]any{
		"file_path":  fp,
		"old_string": "",
		"new_string": "content",
	})
	if result.IsError {
		t.Errorf("empty old_string on empty existing file should succeed, got: %q", result.Output)
	}
}
