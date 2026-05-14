package tools

import (
	"testing"
)

// ─── isBinaryExtension ──────────────────────────────────────────────────────

func TestIsBinaryExtensionExe(t *testing.T) {
	if !isBinaryExtension(".exe") {
		t.Error(".exe should be binary")
	}
}

func TestIsBinaryExtensionDll(t *testing.T) {
	if !isBinaryExtension(".dll") {
		t.Error(".dll should be binary")
	}
}

func TestIsBinaryExtensionZip(t *testing.T) {
	if !isBinaryExtension(".zip") {
		t.Error(".zip should be binary")
	}
}

func TestIsBinaryExtensionPng(t *testing.T) {
	if !isBinaryExtension(".png") {
		t.Error(".png should be binary")
	}
}

func TestIsBinaryExtensionPdf(t *testing.T) {
	if !isBinaryExtension(".pdf") {
		t.Error(".pdf should be binary")
	}
}

func TestIsBinaryExtensionGo(t *testing.T) {
	if isBinaryExtension(".go") {
		t.Error(".go should not be binary")
	}
}

func TestIsBinaryExtensionTxt(t *testing.T) {
	if isBinaryExtension(".txt") {
		t.Error(".txt should not be binary")
	}
}

func TestIsBinaryExtensionPy(t *testing.T) {
	if isBinaryExtension(".py") {
		t.Error(".py should not be binary")
	}
}

func TestIsBinaryExtensionMp3(t *testing.T) {
	if !isBinaryExtension(".mp3") {
		t.Error(".mp3 should be binary")
	}
}

func TestIsBinaryExtensionClass(t *testing.T) {
	if !isBinaryExtension(".class") {
		t.Error(".class should be binary")
	}
}

// ─── isDeviceFile ───────────────────────────────────────────────────────────

func TestIsDeviceFileZero(t *testing.T) {
	if !isDeviceFile("/dev/zero") {
		t.Error("/dev/zero should be device file")
	}
}

func TestIsDeviceFileRandom(t *testing.T) {
	if !isDeviceFile("/dev/random") {
		t.Error("/dev/random should be device file")
	}
}

func TestIsDeviceFileStdin(t *testing.T) {
	if !isDeviceFile("/dev/stdin") {
		t.Error("/dev/stdin should be device file")
	}
}

func TestIsDeviceFileProcFd(t *testing.T) {
	if !isDeviceFile("/proc/self/fd/0") {
		t.Error("/proc/self/fd/0 should be device file")
	}
}

func TestIsDeviceFileNormal(t *testing.T) {
	if isDeviceFile("/home/user/main.go") {
		t.Error("normal path should not be device file")
	}
}

func TestIsDeviceFileWindows(t *testing.T) {
	if isDeviceFile("C:\\Users\\file.txt") {
		t.Error("Windows path should not be device file")
	}
}

func TestIsDeviceFileWindowsBackslash(t *testing.T) {
	// The function normalizes backslashes to forward slashes
	if !isDeviceFile(`\dev\zero`) {
		t.Error("backslash /dev/zero should be detected after normalization")
	}
}

// ─── isBinaryMagic ──────────────────────────────────────────────────────────

func TestIsBinaryMagicPE(t *testing.T) {
	header := []byte{'M', 'Z', 0x00, 0x00}
	if !isBinaryMagic(header) {
		t.Error("PE/EXE header should be binary")
	}
}

func TestIsBinaryMagicELF(t *testing.T) {
	header := []byte{0x7f, 'E', 'L', 'F'}
	if !isBinaryMagic(header) {
		t.Error("ELF header should be binary")
	}
}

func TestIsBinaryMagicPDF(t *testing.T) {
	header := []byte{'%', 'P', 'D', 'F'}
	if !isBinaryMagic(header) {
		t.Error("PDF header should be binary")
	}
}

func TestIsBinaryMagicGZIP(t *testing.T) {
	header := []byte{0x1f, 0x8b, 0x00, 0x00}
	if !isBinaryMagic(header) {
		t.Error("GZIP header should be binary")
	}
}

func TestIsBinaryMagicJPEG(t *testing.T) {
	header := []byte{0xff, 0xd8, 0xff, 0xe0}
	if !isBinaryMagic(header) {
		t.Error("JPEG header should be binary")
	}
}

func TestIsBinaryMagicPNG(t *testing.T) {
	header := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	if !isBinaryMagic(header) {
		t.Error("PNG header should be binary")
	}
}

func TestIsBinaryMagicGIF(t *testing.T) {
	header := []byte{'G', 'I', 'F', '8', '7', 'a'}
	if !isBinaryMagic(header) {
		t.Error("GIF header should be binary")
	}
}

func TestIsBinaryMagicZIP(t *testing.T) {
	header := []byte{'P', 'K', 0x03, 0x04}
	if !isBinaryMagic(header) {
		t.Error("ZIP header should be binary")
	}
}

func TestIsBinaryMagicWasm(t *testing.T) {
	header := []byte{0x00, 'a', 's', 'm'}
	if !isBinaryMagic(header) {
		t.Error("Wasm header should be binary")
	}
}

func TestIsBinaryMagicText(t *testing.T) {
	header := []byte("Hello, world!")
	if isBinaryMagic(header) {
		t.Error("plain text should not be detected as binary")
	}
}

func TestIsBinaryMagicEmpty(t *testing.T) {
	if isBinaryMagic([]byte{}) {
		t.Error("empty header should not be binary")
	}
}

func TestIsBinaryMagicShortHdr(t *testing.T) {
	if isBinaryMagic([]byte{0x00}) {
		t.Error("1-byte header should not match any 4-byte signature")
	}
}

func TestIsBinaryMagicLua(t *testing.T) {
	header := []byte{0x1b, 'L', 'u', 'a'}
	if !isBinaryMagic(header) {
		t.Error("Lua bytecode should be binary")
	}
}

func TestIsBinaryMagicClass(t *testing.T) {
	header := []byte{0xca, 0xfe, 0xba, 0xbe}
	if !isBinaryMagic(header) {
		t.Error("Java .class should be binary")
	}
}

func TestIsBinaryMagicMP3ID3(t *testing.T) {
	header := []byte{'I', 'D', '3'}
	if !isBinaryMagic(header) {
		t.Error("MP3 ID3v2 header should be binary")
	}
}

func TestIsBinaryMagicXZ(t *testing.T) {
	header := []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}
	if !isBinaryMagic(header) {
		t.Error("XZ header should be binary")
	}
}

func TestIsBinaryMagic7Z(t *testing.T) {
	header := []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}
	if !isBinaryMagic(header) {
		t.Error("7Z header should be binary")
	}
}

func TestIsBinaryMagicWebP(t *testing.T) {
	header := []byte{'R', 'I', 'F', 'F', 0x00, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P'}
	if !isBinaryMagic(header) {
		t.Error("WebP header should be binary")
	}
}

func TestIsBinaryMagicMP4(t *testing.T) {
	header := []byte{0x00, 0x00, 0x00, 0x20, 'f', 't', 'y', 'p'}
	if !isBinaryMagic(header) {
		t.Error("MP4 ftyp header should be binary")
	}
}

// ─── isUncPath ──────────────────────────────────────────────────────────────

func TestIsUncPathForwardSlash(t *testing.T) {
	if !isUncPath("//server/share") {
		t.Error("//server/share should be UNC")
	}
}

func TestIsUncPathBackslash(t *testing.T) {
	if !isUncPath(`\\server\share`) {
		t.Error("\\\\server\\share should be UNC")
	}
}

func TestIsUncPathNormal(t *testing.T) {
	if isUncPath("/home/user/file.txt") {
		t.Error("normal path should not be UNC")
	}
}

func TestIsUncPathWindowsDrive(t *testing.T) {
	if isUncPath("C:\\Users\\file.txt") {
		t.Error("Windows drive path should not be UNC")
	}
}

// ─── expandPath ─────────────────────────────────────────────────────────────

func TestExpandPathTilde(t *testing.T) {
	result := expandPath("~/file.txt")
	if result == "~/file.txt" {
		t.Error("tilde should be expanded")
	}
}

func TestExpandPathWindowsDrive(t *testing.T) {
	result := expandPath("E:")
	if !stringsContains(result, ":\\") && !stringsContains(result, ":/") {
		t.Errorf("bare drive letter should get separator, got %q", result)
	}
}

func TestExpandPathUnixOnWindows(t *testing.T) {
	result := expandPath("/e/workspace")
	// Should convert /e/ to E:\ on Windows
	if stringsContains(result, "E:") || stringsContains(result, "e:") {
		t.Logf("Unix path converted: %q", result)
	}
}

func TestExpandPathNormal(t *testing.T) {
	result := expandPath("/home/user/file.txt")
	if result == "" {
		t.Error("normal path should not be empty")
	}
}

// ─── FileReadTool interface ─────────────────────────────────────────────────

func TestFileReadToolName(t *testing.T) {
	tool := &FileReadTool{}
	if tool.Name() != "read_file" {
		t.Errorf("expected 'read_file', got %q", tool.Name())
	}
}

func TestFileReadToolSchema(t *testing.T) {
	tool := &FileReadTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "file_path" {
		t.Errorf("expected required=[file_path], got %v", required)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["file_path"]; !ok {
		t.Error("schema should have file_path property")
	}
	if _, ok := props["offset"]; !ok {
		t.Error("schema should have offset property")
	}
	if _, ok := props["limit"]; !ok {
		t.Error("schema should have limit property")
	}
}

func TestFileReadToolPermissions(t *testing.T) {
	tool := &FileReadTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestFileReadToolExecuteNoPath(t *testing.T) {
	tool := &FileReadTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing file_path should return error")
	}
}

func TestFileReadToolExecuteUNC(t *testing.T) {
	tool := &FileReadTool{}
	result := tool.Execute(map[string]any{"file_path": `\\server\share\file.txt`})
	if !result.IsError {
		t.Error("UNC path should return error")
	}
}

// ─── NewFileReadTool ────────────────────────────────────────────────────────

func TestNewFileReadTool(t *testing.T) {
	r := NewRegistry()
	tool := NewFileReadTool(r)
	if tool == nil {
		t.Fatal("NewFileReadTool should not return nil")
	}
}

// ─── FileUnchangedStub ──────────────────────────────────────────────────────

func TestFileUnchangedStub(t *testing.T) {
	if FileUnchangedStub != "File unchanged since last read." {
		t.Errorf("unexpected stub value: %q", FileUnchangedStub)
	}
}

// ─── maxFileSize ────────────────────────────────────────────────────────────

func TestMaxFileSize(t *testing.T) {
	if maxFileSize != 256*1024 {
		t.Errorf("expected 256KB, got %d", maxFileSize)
	}
}

// ─── Upstream Quality: isBinaryExtension completeness invariant ──────────────

func TestIsBinaryExtensionCompleteness(t *testing.T) {
	// All common binary formats should be detected — invariant from upstream
	requiredExts := []string{
		".exe", ".dll", ".so", ".dylib", ".com",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar",
		".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp", ".svgz",
		".mp3", ".mp4", ".wav", ".ogg", ".avi", ".flac",
		".pyc", ".class", ".jar", ".o", ".obj",
		".pdf", ".docx", ".xlsx", ".pptx",
		".bin", ".db", ".sqlite",
		".woff", ".woff2", ".ttf", ".eot",
	}
	for _, ext := range requiredExts {
		if !isBinaryExtension(ext) {
			t.Errorf("isBinaryExtension(%q) should be true — missing from binary extension list", ext)
		}
	}
}

// ─── Upstream Quality: isBinaryExtension no overlap with text ─────────────────

func TestIsBinaryExtensionNoTextOverlap(t *testing.T) {
	// Common text formats must NOT be flagged as binary
	textExts := []string{
		".go", ".py", ".js", ".ts", ".rs", ".java", ".c", ".cpp",
		".txt", ".md", ".json", ".yaml", ".yml", ".toml",
		".html", ".css", ".scss", ".xml", ".csv",
		".sh", ".bash", ".zsh", ".fish",
		".gitignore", ".dockerignore",
		".env", ".ini", ".cfg", ".conf",
		".sql", ".rb", ".php", ".pl",
		".swift", ".kt", ".scala",
		".lua", ".r", ".m",
	}
	for _, ext := range textExts {
		if isBinaryExtension(ext) {
			t.Errorf("isBinaryExtension(%q) should be false — text extension incorrectly flagged as binary", ext)
		}
	}
}

// ─── Upstream Quality: isBinaryMagic completeness invariant ──────────────────

func TestIsBinaryMagicCompleteness(t *testing.T) {
	// All major binary magic signatures should be detected
	tests := []struct {
		name   string
		header []byte
	}{
		{"PE/EXE", []byte{'M', 'Z', 0x00, 0x00}},
		{"ELF", []byte{0x7f, 'E', 'L', 'F', 0x00, 0x00}},
		{"PDF", []byte{'%', 'P', 'D', 'F', '-', '1', '.', '4'}},
		{"GZIP", []byte{0x1f, 0x8b, 0x08, 0x00}},
		{"JPEG", []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10}},
		{"PNG", []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}},
		{"GIF87a", []byte{'G', 'I', 'F', '8', '7', 'a'}},
		{"GIF89a", []byte{'G', 'I', 'F', '8', '9', 'a'}},
		{"ZIP", []byte{'P', 'K', 0x03, 0x04}},
		{"Wasm", []byte{0x00, 'a', 's', 'm', 0x01, 0x00}},
		{"Java class", []byte{0xca, 0xfe, 0xba, 0xbe}},
		{"Lua bytecode", []byte{0x1b, 'L', 'u', 'a'}},
		{"XZ", []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}},
		{"7Z", []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}},
		{"WebP", []byte{'R', 'I', 'F', 'F', 0x00, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P'}},
		{"MP4", []byte{0x00, 0x00, 0x00, 0x20, 'f', 't', 'y', 'p'}},
		{"BZIP2", []byte{'B', 'Z', 'h', 0x31}},
		{"MP3 ID3v2", []byte{'I', 'D', '3', 0x03, 0x00}},
		{"MP3 sync", []byte{0xff, 0xfb, 0x90, 0x00}},
		{"Python pyc", []byte{0x0d, 0x0d, 0x0d, 0x0a}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !isBinaryMagic(tt.header) {
				t.Errorf("isBinaryMagic should detect %s signature", tt.name)
			}
		})
	}
}

// ─── Upstream Quality: isDeviceFile edge cases ───────────────────────────────

func TestIsDeviceFileEdgeCases(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Exact matches
		{"/dev/zero", true},
		{"/dev/random", true},
		{"/dev/urandom", true},
		{"/dev/full", true},
		{"/dev/null", false}, // /dev/null is NOT blocked — it returns EOF
		// Path suffix matches (e.g., inside container)
		{"some/path/dev/zero", true},
		// /proc patterns
		{"/proc/self/fd/0", true},
		{"/proc/123/fd/1", true},
		// Case insensitive
		{"/DEV/ZERO", true},
		// Normal paths
		{"/home/user/main.go", false},
		{"/tmp/output.txt", false},
		// Windows paths
		{"C:\\Users\\file.txt", false},
	}
	for _, tt := range tests {
		got := isDeviceFile(tt.path)
		if got != tt.expected {
			t.Errorf("isDeviceFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

// ─── Upstream Quality: expandPath idempotency ────────────────────────────────

func TestExpandPathIdempotent(t *testing.T) {
	// expandPath(expandPath(p)) should produce the same result for absolute paths
	paths := []string{
		"/home/user/file.txt",
		"C:\\Users\\file.txt",
	}
	for _, p := range paths {
		first := expandPath(p)
		second := expandPath(first)
		if second != first {
			t.Errorf("expandPath not idempotent for %q: first=%q, second=%q", p, first, second)
		}
	}
}

// ─── Upstream Quality: isUncPath edge cases ──────────────────────────────────

func TestIsUncPathEdgeCases(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"//server/share", true},
		{`\\server\share`, true},
		{"/home/user/file", false},
		{"C:\\Users\\file", false},
		{"https://example.com", false}, // URL should not be UNC
		{"//", true},                   // bare UNC prefix
	}
	for _, tt := range tests {
		got := isUncPath(tt.path)
		if got != tt.expected {
			t.Errorf("isUncPath(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

// ─── Upstream Quality: normalizeQuotes idempotency ───────────────────────────

func TestNormalizeQuotesIdempotent(t *testing.T) {
	// normalizeQuotes(normalizeQuotes(x)) == normalizeQuotes(x) — idempotency invariant
	inputs := []string{
		"hello world",
		`"straight quotes"`,
		"\u201Ccurly\u201D",
		"mixed \u201Cfoo\u201D and \u2018bar\u2019",
	}
	for _, in := range inputs {
		first := normalizeQuotes(in)
		second := normalizeQuotes(first)
		if second != first {
			t.Errorf("normalizeQuotes not idempotent for %q: first=%q, second=%q", in, first, second)
		}
	}
}

// helper
func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && len(sub) > 0 && containsAny(s, sub)))
}

func containsAny(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}