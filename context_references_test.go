package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── ParseContextReferences ──────────────────────────────────────────────────

func TestParseContextReferencesEmpty(t *testing.T) {
	refs := ParseContextReferences("")
	if refs != nil {
		t.Errorf("expected nil for empty input, got %v", refs)
	}
}

func TestParseContextReferencesFileRef(t *testing.T) {
	refs := ParseContextReferences("look at @file:main.go")
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refs))
	}
	if refs[0].Kind != "file" {
		t.Errorf("expected kind 'file', got %q", refs[0].Kind)
	}
	if refs[0].Target != "main.go" {
		t.Errorf("expected target 'main.go', got %q", refs[0].Target)
	}
}

func TestParseContextReferencesFolderRef(t *testing.T) {
	refs := ParseContextReferences("check @folder:src/")
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refs))
	}
	if refs[0].Kind != "folder" {
		t.Errorf("expected kind 'folder', got %q", refs[0].Kind)
	}
}

func TestParseContextReferencesDiffRef(t *testing.T) {
	refs := ParseContextReferences("show @diff")
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refs))
	}
	if refs[0].Kind != "diff" {
		t.Errorf("expected kind 'diff', got %q", refs[0].Kind)
	}
}

func TestParseContextReferencesStagedRef(t *testing.T) {
	refs := ParseContextReferences("show @staged")
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refs))
	}
	if refs[0].Kind != "staged" {
		t.Errorf("expected kind 'staged', got %q", refs[0].Kind)
	}
}

func TestParseContextReferencesGitRef(t *testing.T) {
	refs := ParseContextReferences("show @git:5")
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refs))
	}
	if refs[0].Kind != "git" {
		t.Errorf("expected kind 'git', got %q", refs[0].Kind)
	}
	if refs[0].Target != "5" {
		t.Errorf("expected target '5', got %q", refs[0].Target)
	}
}

func TestParseContextReferencesURLRef(t *testing.T) {
	refs := ParseContextReferences("see @url:https://example.com")
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refs))
	}
	if refs[0].Kind != "url" {
		t.Errorf("expected kind 'url', got %q", refs[0].Kind)
	}
}

func TestParseContextReferencesSkipsEmail(t *testing.T) {
	refs := ParseContextReferences("email user@domain.com here")
	if len(refs) != 0 {
		t.Errorf("expected 0 references (email exclusion), got %d", len(refs))
	}
}

func TestParseContextReferencesMultiple(t *testing.T) {
	refs := ParseContextReferences("look at @file:a.go and @file:b.go")
	if len(refs) != 2 {
		t.Fatalf("expected 2 references, got %d", len(refs))
	}
}

func TestParseContextReferencesQuotedPath(t *testing.T) {
	refs := ParseContextReferences(`@file:"path with spaces.go"`)
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refs))
	}
	if refs[0].Target != "path with spaces.go" {
		t.Errorf("expected target 'path with spaces.go', got %q", refs[0].Target)
	}
}

// ─── isWordChar ──────────────────────────────────────────────────────────────

func TestIsWordChar(t *testing.T) {
	tests := []struct {
		b    byte
		want bool
	}{
		{'a', true},
		{'Z', true},
		{'5', true},
		{'_', true},
		{'/', true},
		{'@', false},
		{' ', false},
		{'.', false},
	}
	for _, tt := range tests {
		got := isWordChar(tt.b)
		if got != tt.want {
			t.Errorf("isWordChar(%q) = %v, want %v", tt.b, got, tt.want)
		}
	}
}

// ─── stripTrailingPunctuation ────────────────────────────────────────────────

func TestStripTrailingPunctuation(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"main.go", "main.go"},
		{"path,", "path"},
		{"path.", "path"},
		{"path)", "path"},
		{"path]", "path"},
		{"(path)", "(path)"},
	}
	for _, tt := range tests {
		got := stripTrailingPunctuation(tt.input)
		if got != tt.want {
			t.Errorf("stripTrailingPunctuation(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ─── stripQuotes ─────────────────────────────────────────────────────────────

func TestStripQuotes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"hello"`, "hello"},
		{`'world'`, "world"},
		{"naked", "naked"},
		{`"unclosed`, `"unclosed`},
		{`""`, ""},
	}
	for _, tt := range tests {
		got := stripQuotes(tt.input)
		if got != tt.want {
			t.Errorf("stripQuotes(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ─── parseFileTarget ─────────────────────────────────────────────────────────

func TestParseFileTarget(t *testing.T) {
	tests := []struct {
		input     string
		target    string
		lineStart int
		lineEnd   int
	}{
		{"main.go", "main.go", 0, 0},
		{"main.go:10", "main.go", 10, 0},
		{"main.go:10-50", "main.go", 10, 50},
		{"src/app.rs:5-20", "src/app.rs", 5, 20},
	}
	for _, tt := range tests {
		target, lineStart, lineEnd := parseFileTarget(tt.input)
		if target != tt.target || lineStart != tt.lineStart || lineEnd != tt.lineEnd {
			t.Errorf("parseFileTarget(%q) = (%q, %d, %d), want (%q, %d, %d)",
				tt.input, target, lineStart, lineEnd, tt.target, tt.lineStart, tt.lineEnd)
		}
	}
}

// ─── codeFenceLanguage ───────────────────────────────────────────────────────

func TestCodeFenceLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.rs", "rust"},
		{"script.py", "python"},
		{"index.js", "javascript"},
		{"app.ts", "typescript"},
		{"config.json", "json"},
		{"readme.md", "markdown"},
		{"deploy.sh", "bash"},
		{"values.yaml", "yaml"},
		{"unknown.xyz", ""},
	}
	for _, tt := range tests {
		got := codeFenceLanguage(tt.path)
		if got != tt.want {
			t.Errorf("codeFenceLanguage(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// ─── isBinaryLines ───────────────────────────────────────────────────────────

func TestIsBinaryLines(t *testing.T) {
	if !isBinaryLines([]string{"\x00binary"}) {
		t.Error("should detect null byte as binary")
	}
	if isBinaryLines([]string{"normal text"}) {
		t.Error("should not detect normal text as binary")
	}
}

func TestIsBinaryContent(t *testing.T) {
	if !isBinaryContent([]byte{0x00, 0x01, 0x02}) {
		t.Error("should detect null byte as binary")
	}
	if isBinaryContent([]byte("normal text")) {
		t.Error("should not detect normal text as binary")
	}
}

// ─── extractHTMLContent ──────────────────────────────────────────────────────

func TestExtractHTMLContent(t *testing.T) {
	html := `<html><body><p>Hello World</p></body></html>`
	result := extractHTMLContent(html)
	if !strings.Contains(result, "Hello World") {
		t.Errorf("expected 'Hello World' in extracted content, got %q", result)
	}
}

func TestExtractHTMLContentStripsScript(t *testing.T) {
	html := `<html><body><script>alert('xss')</script><p>Safe</p></body></html>`
	result := extractHTMLContent(html)
	if strings.Contains(result, "alert") {
		t.Error("script content should be stripped")
	}
	if !strings.Contains(result, "Safe") {
		t.Error("non-script content should be preserved")
	}
}

func TestExtractHTMLContentDecodesEntities(t *testing.T) {
	html := `<p>&amp; &lt; &gt;</p>`
	result := extractHTMLContent(html)
	if !strings.Contains(result, "& < >") {
		t.Errorf("expected decoded entities, got %q", result)
	}
}

// ─── extractHTMLTitle ────────────────────────────────────────────────────────

func TestExtractHTMLTitle(t *testing.T) {
	html := `<html><head><title>Test Page</title></head><body></body></html>`
	title := extractHTMLTitle(html)
	if title != "Test Page" {
		t.Errorf("expected 'Test Page', got %q", title)
	}
}

func TestExtractHTMLTitleMissing(t *testing.T) {
	html := `<html><body>No title</body></html>`
	title := extractHTMLTitle(html)
	if title != "" {
		t.Errorf("expected empty string for missing title, got %q", title)
	}
}

// ─── resolvePath ─────────────────────────────────────────────────────────────

func TestResolvePath(t *testing.T) {
	// Absolute path (platform-specific)
	cwd, _ := os.Getwd()
	absPath := filepath.Join(cwd, "abs", "path.go")
	result := resolvePath(cwd, absPath)
	if result != absPath {
		t.Errorf("expected absolute path preserved, got %q", result)
	}
}

// ─── removeReferenceTokens ───────────────────────────────────────────────────

func TestRemoveReferenceTokens(t *testing.T) {
	msg := "look at @file:main.go and @file:other.go"
	refs := ParseContextReferences(msg)
	result := removeReferenceTokens(msg, refs)
	if strings.Contains(result, "@file:") {
		t.Errorf("references should be removed, got %q", result)
	}
}

func TestRemoveReferenceTokensNoRefs(t *testing.T) {
	msg := "no references here"
	result := removeReferenceTokens(msg, nil)
	if result != msg {
		t.Errorf("expected unchanged message, got %q", result)
	}
}
