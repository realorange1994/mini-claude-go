package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── resolvePath ─────────────────────────────────────────────────────────────

func TestResolvePathRelative(t *testing.T) {
	result := resolvePath(filepath.FromSlash("/home/user/project"), "main.go")
	expected := filepath.Join(filepath.FromSlash("/home/user/project"), "main.go")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestResolvePathAbsolute(t *testing.T) {
	// Use a platform-specific absolute path
	absPath := filepath.Join(os.TempDir(), "file.go")
	result := resolvePath(filepath.FromSlash("/home/user/project"), absPath)
	if result != filepath.Clean(absPath) {
		t.Errorf("expected %q, got %q", filepath.Clean(absPath), result)
	}
}

func TestResolvePathParentRef(t *testing.T) {
	result := resolvePath(filepath.FromSlash("/home/user/project"), ".."+string(filepath.Separator)+"parent.go")
	expected := filepath.Clean(filepath.FromSlash("/home/user/parent.go"))
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// ─── expandPath ──────────────────────────────────────────────────────────────

func TestExpandPathAbsolute(t *testing.T) {
	absPath := filepath.Join(os.TempDir(), "path.go")
	result := expandPath(absPath)
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute path, got %q", result)
	}
}

func TestExpandPathRelative(t *testing.T) {
	// Should expand relative to current working directory
	result := expandPath("main.go")
	cwd, _ := os.Getwd()
	expected := filepath.Join(cwd, "main.go")
	if filepath.Clean(result) != filepath.Clean(expected) {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// ─── parseDuration ───────────────────────────────────────────────────────────

func TestParseDurationValid(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"1h", true},
		{"30m", true},
		{"2d", true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		result := parseDuration(tt.input)
		isZero := result.IsZero()
		if isZero == tt.want {
			t.Errorf("parseDuration(%q): expected non-zero=%v, got zero=%v", tt.input, tt.want, isZero)
		}
	}
}

func TestParseDurationDays(t *testing.T) {
	result := parseDuration("2d")
	if result.IsZero() {
		t.Error("expected non-zero result for '2d'")
	}
}

// ─── stripTrailingPunctuation ────────────────────────────────────────────────

func TestStripTrailingPunctuationNoTrailing(t *testing.T) {
	result := stripTrailingPunctuation("main.go")
	if result != "main.go" {
		t.Errorf("expected 'main.go', got %q", result)
	}
}

func TestStripTrailingPunctuationComma(t *testing.T) {
	result := stripTrailingPunctuation("path,")
	if result != "path" {
		t.Errorf("expected 'path', got %q", result)
	}
}

func TestStripTrailingPunctuationMultiple(t *testing.T) {
	result := stripTrailingPunctuation("path.")
	if result != "path" {
		t.Errorf("expected 'path', got %q", result)
	}
}

func TestStripTrailingPunctuationParen(t *testing.T) {
	result := stripTrailingPunctuation("path)")
	if result != "path" {
		t.Errorf("expected 'path', got %q", result)
	}
}

func TestStripTrailingPunctuationBracket(t *testing.T) {
	result := stripTrailingPunctuation("path]")
	if result != "path" {
		t.Errorf("expected 'path', got %q", result)
	}
}

func TestStripTrailingPunctuationBalancedParen(t *testing.T) {
	result := stripTrailingPunctuation("(path)")
	if result != "(path)" {
		t.Errorf("expected '(path)', got %q", result)
	}
}

// ─── stripQuotes ─────────────────────────────────────────────────────────────

func TestStripQuotesDoubleQuoted(t *testing.T) {
	result := stripQuotes(`"hello world"`)
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestStripQuotesSingleQuoted(t *testing.T) {
	result := stripQuotes(`'hello'`)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestStripQuotesNoQuotes(t *testing.T) {
	result := stripQuotes("naked")
	if result != "naked" {
		t.Errorf("expected 'naked', got %q", result)
	}
}

func TestStripQuotesEmpty(t *testing.T) {
	result := stripQuotes("")
	if result != "" {
		t.Errorf("expected '', got %q", result)
	}
}

func TestStripQuotesUnclosed(t *testing.T) {
	result := stripQuotes(`"unclosed`)
	if result != `"unclosed` {
		t.Errorf("expected '\"unclosed', got %q", result)
	}
}

// ─── parseFileTarget ─────────────────────────────────────────────────────────

func TestParseFileTargetSimple(t *testing.T) {
	target, lineStart, lineEnd := parseFileTarget("main.go")
	if target != "main.go" {
		t.Errorf("expected 'main.go', got %q", target)
	}
	if lineStart != 0 {
		t.Errorf("expected lineStart=0, got %d", lineStart)
	}
	if lineEnd != 0 {
		t.Errorf("expected lineEnd=0, got %d", lineEnd)
	}
}

func TestParseFileTargetLineRange(t *testing.T) {
	target, lineStart, lineEnd := parseFileTarget("main.go:10-50")
	if target != "main.go" {
		t.Errorf("expected 'main.go', got %q", target)
	}
	if lineStart != 10 {
		t.Errorf("expected lineStart=10, got %d", lineStart)
	}
	if lineEnd != 50 {
		t.Errorf("expected lineEnd=50, got %d", lineEnd)
	}
}

func TestParseFileTargetStartOnly(t *testing.T) {
	target, lineStart, lineEnd := parseFileTarget("main.go:10")
	if target != "main.go" {
		t.Errorf("expected 'main.go', got %q", target)
	}
	if lineStart != 10 {
		t.Errorf("expected lineStart=10, got %d", lineStart)
	}
	if lineEnd != 0 {
		t.Errorf("expected lineEnd=0, got %d", lineEnd)
	}
}

func TestParseFileTargetWithPath(t *testing.T) {
	target, lineStart, lineEnd := parseFileTarget("src/app.rs:5-20")
	if target != "src/app.rs" {
		t.Errorf("expected 'src/app.rs', got %q", target)
	}
	if lineStart != 5 {
		t.Errorf("expected lineStart=5, got %d", lineStart)
	}
	if lineEnd != 20 {
		t.Errorf("expected lineEnd=20, got %d", lineEnd)
	}
}

// ─── codeFenceLanguage ───────────────────────────────────────────────────────

func TestCodeFenceLanguageGo(t *testing.T) {
	if codeFenceLanguage("main.go") != "go" {
		t.Error("expected 'go' for .go file")
	}
}

func TestCodeFenceLanguageVarious(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"app.rs", "rust"},
		{"script.py", "python"},
		{"index.js", "javascript"},
		{"app.ts", "typescript"},
		{"app.tsx", "tsx"},
		{"config.json", "json"},
		{"readme.md", "markdown"},
		{"deploy.sh", "bash"},
		{"values.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"config.toml", "toml"},
		{"app.cpp", "cpp"},
		{"app.h", "c"},
		{"app.hpp", "cpp"},
		{"app.java", "java"},
		{"app.rb", "ruby"},
		{"app.php", "php"},
		{"query.sql", "sql"},
		{"page.html", "html"},
		{"style.css", "css"},
	}
	for _, tt := range tests {
		got := codeFenceLanguage(tt.path)
		if got != tt.want {
			t.Errorf("codeFenceLanguage(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestCodeFenceLanguageUnknown(t *testing.T) {
	if codeFenceLanguage("unknown.xyz") != "" {
		t.Error("expected empty string for unknown extension")
	}
	if codeFenceLanguage("noext") != "" {
		t.Error("expected empty string for file without extension")
	}
}

// ─── isBinaryLines ───────────────────────────────────────────────────────────

func TestIsBinaryLinesNullByte(t *testing.T) {
	if !isBinaryLines([]string{"\x00binary"}) {
		t.Error("should detect null byte as binary")
	}
}

func TestIsBinaryLinesNormal(t *testing.T) {
	if isBinaryLines([]string{"normal text", "more text"}) {
		t.Error("should not detect normal text as binary")
	}
}

func TestIsBinaryLinesEmpty(t *testing.T) {
	if isBinaryLines([]string{}) {
		t.Error("should not detect empty input as binary")
	}
}

// ─── isBinaryContent ─────────────────────────────────────────────────────────

func TestIsBinaryContentNull(t *testing.T) {
	if !isBinaryContent([]byte{0x00, 0x01, 0x02}) {
		t.Error("should detect null byte as binary")
	}
}

func TestIsBinaryContentNormal(t *testing.T) {
	if isBinaryContent([]byte("normal text content")) {
		t.Error("should not detect normal text as binary")
	}
}

// ─── ensurePathAllowed ───────────────────────────────────────────────────────

func TestEnsurePathAllowedSensitiveDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	result := ensurePathAllowed(filepath.Join(home, ".ssh", "id_rsa"), cwd)
	if result == "" {
		t.Error("should reject path in .ssh directory")
	}
}

func TestEnsurePathAllowedPathTraversal(t *testing.T) {
	cwd, _ := os.Getwd()
	outside := filepath.Join(filepath.Dir(cwd), "outside")
	result := ensurePathAllowed(outside, cwd)
	if result == "" {
		t.Error("should reject path outside cwd")
	}
}

func TestEnsurePathAllowedWithinCwd(t *testing.T) {
	cwd, _ := os.Getwd()
	result := ensurePathAllowed(filepath.Join(cwd, "main.go"), cwd)
	if result != "" {
		t.Errorf("should allow path within cwd, got: %q", result)
	}
}

// ─── buildFolderListing ──────────────────────────────────────────────────────

func TestBuildFolderListing(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub1", "sub2"), 0755)
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "sub1", "file2.txt"), []byte("world"), 0644)

	result := buildFolderListing(dir, dir, 200, 2)
	if result == "" {
		t.Error("expected non-empty folder listing")
	}
	if !containsInListing(result, "sub1") {
		t.Error("folder listing should contain 'sub1'")
	}
	if !containsInListing(result, "file1.txt") {
		t.Error("folder listing should contain 'file1.txt'")
	}
}

func TestBuildFolderListingEmpty(t *testing.T) {
	dir := t.TempDir()
	result := buildFolderListing(dir, dir, 200, 2)
	if result == "" {
		t.Error("expected non-empty folder listing for empty dir")
	}
}

func TestBuildFolderListingLimit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		os.WriteFile(filepath.Join(dir, "file"+string(rune('a'+i))+".txt"), []byte("x"), 0644)
	}

	result := buildFolderListing(dir, dir, 3, 2)
	if !containsInListing(result, "...") {
		t.Error("folder listing should contain truncation marker")
	}
}

func TestBuildFolderListingHidesHidden(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("secret"), 0644)

	result := buildFolderListing(dir, dir, 200, 2)
	if containsInListing(result, ".hidden") {
		t.Error("folder listing should not contain hidden files")
	}
}

// ─── globMatch ───────────────────────────────────────────────────────────────

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "main.rs", false},
		{"**/*.go", "src/main.go", true},
		{"**/*.go", "main.go", true},
	}
	for _, tt := range tests {
		got := globMatch(tt.pattern, tt.path)
		if got != tt.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func containsInListing(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && (s == sub || (len(s) > len(sub) && (s[:len(sub)] == sub || s[len(s)-len(sub):] == sub || containsInListing(s[1:], sub))))
}

// ─── Regression: file_history_search format error (Bug 4) ────────────────────
// Previously, when mode param was nil, the error message used params["mode"]
// directly producing "No %!s(<nil>) results for ...".
// Now modeStr is computed before the error check with a default of "changed".

func TestFileHistorySearchNoModeInErrorMessage(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("original content"), 0644)

	sh := NewSnapshotHistory(dir)
	sh.TakeSnapshot(fp)

	tool := &FileHistorySearchTool{History: sh}
	// Search for text that doesn't exist in any version
	result := tool.Execute(map[string]any{
		"path":  fp,
		"query": "nonexistent_text_xyz",
		// Note: no "mode" param provided
	})
	// Should return an error-like message (no results) but NOT with %!s(<nil>)
	if result.Output == "" {
		t.Fatal("expected non-empty output for no results")
	}
	if strings.Contains(result.Output, "%!s") {
		t.Errorf("error message contains format error: %s", result.Output)
	}
	if strings.Contains(result.Output, "<nil>") {
		t.Errorf("error message contains <nil>: %s", result.Output)
	}
	// Should mention "changed" as the default mode
	if !strings.Contains(result.Output, "changed") {
		t.Errorf("expected default mode 'changed' in message, got: %s", result.Output)
	}
}

func TestFileHistorySearchExplicitModeInErrorMessage(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test2.txt")
	os.WriteFile(fp, []byte("some content"), 0644)

	sh := NewSnapshotHistory(dir)
	sh.TakeSnapshot(fp)

	tool := &FileHistorySearchTool{History: sh}
	// Search with explicit "added" mode
	result := tool.Execute(map[string]any{
		"path":  fp,
		"query": "nonexistent_text_xyz",
		"mode":  "added",
	})
	if result.Output == "" {
		t.Fatal("expected non-empty output")
	}
	if strings.Contains(result.Output, "%!s") || strings.Contains(result.Output, "<nil>") {
		t.Errorf("error message has format issue: %s", result.Output)
	}
	if !strings.Contains(result.Output, "added") {
		t.Errorf("expected mode 'added' in message, got: %s", result.Output)
	}
}

// ─── Regression: file_history_diff friendly error for non-existent versions (Bug 5) ────
// Previously: "last1 is out of range (only 1 versions)" — exposes internals.
// Now: "version 'last1' does not exist — this file only has 1 version(s)"

func TestFileHistoryDiffFriendlyErrorForMissingVersion(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("content"), 0644)

	sh := NewSnapshotHistory(dir)
	sh.TakeSnapshot(fp)

	tool := &FileHistoryDiffTool{History: sh}
	result := tool.Execute(map[string]any{
		"path": fp,
		"from": "last2",
	})
	if !result.IsError {
		t.Error("expected error for non-existent version")
	}
	// Should NOT contain raw "out of range" technical language
	if strings.Contains(result.Output, "out of range") {
		t.Errorf("error should not contain technical 'out of range', got: %s", result.Output)
	}
	// Should contain user-friendly "does not exist"
	if !strings.Contains(result.Output, "does not exist") {
		t.Errorf("error should contain 'does not exist', got: %s", result.Output)
	}
}
