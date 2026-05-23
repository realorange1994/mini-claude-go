package tools

import (
	"testing"
)

// ─── WebFetchTool interface ─────────────────────────────────────────────────

func TestWebFetchToolName(t *testing.T) {
	tool := &WebFetchTool{}
	if tool.Name() != "web_fetch" {
		t.Errorf("expected 'web_fetch', got %q", tool.Name())
	}
}

func TestWebFetchToolSchema(t *testing.T) {
	tool := &WebFetchTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "url" {
		t.Errorf("expected required=[url], got %v", required)
	}
}

func TestWebFetchToolPermissionsFileURL(t *testing.T) {
	tool := &WebFetchTool{}
	result := tool.CheckPermissions(map[string]any{"url": "file:///etc/passwd"})
	if result.Behavior != PermissionAsk {
		t.Error("file:// URL should ask")
	}
}

func TestWebFetchToolPermissionsInternalURL(t *testing.T) {
	tool := &WebFetchTool{}
	result := tool.CheckPermissions(map[string]any{"url": "http://localhost:8080"})
	if result.Behavior != PermissionAsk {
		t.Error("localhost URL should ask")
	}
}

func TestWebFetchToolPermissionsPublicURL(t *testing.T) {
	tool := &WebFetchTool{}
	result := tool.CheckPermissions(map[string]any{"url": "https://example.com"})
	if result.Behavior != PermissionPassthrough {
		t.Error("public URL should passthrough")
	}
}

func TestWebFetchToolExecuteNoURL(t *testing.T) {
	tool := &WebFetchTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing url should return error")
	}
}

func TestWebFetchToolExecuteInvalidURL(t *testing.T) {
	tool := &WebFetchTool{}
	result := tool.Execute(map[string]any{"url": "not-a-url"})
	if !result.IsError {
		t.Error("invalid URL should return error")
	}
}

// ─── stripHTMLSimple ────────────────────────────────────────────────────────

func TestStripHTMLSimpleEmpty(t *testing.T) {
	if stripHTMLSimple("") != "" {
		t.Error("empty string should remain empty")
	}
}

func TestStripHTMLSimplePlainText(t *testing.T) {
	result := stripHTMLSimple("hello world")
	if result != "hello world" {
		t.Errorf("plain text should remain unchanged, got %q", result)
	}
}

func TestStripHTMLSimpleStripTags(t *testing.T) {
	result := stripHTMLSimple("<b>hello</b>")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestStripHTMLSimpleStripAllTags(t *testing.T) {
	result := stripHTMLSimple("<html><body><p>test</p></body></html>")
	if result == "" {
		t.Error("should have content after stripping")
	}
	if result != "test" {
		t.Errorf("expected 'test', got %q", result)
	}
}

func TestStripHTMLSimpleNestedTags(t *testing.T) {
	result := stripHTMLSimple("<div><p>nested</p><span>text</span></div>")
	if result == "" {
		t.Error("should have content after stripping")
	}
}

func TestStripHTMLSimpleUnclosedTag(t *testing.T) {
	result := stripHTMLSimple("before <b>bold after")
	if result == "" {
		t.Error("unclosed tag should still return text")
	}
}

// ─── extractHTMLTitle ───────────────────────────────────────────────────────

func TestExtractHTMLTitleWithTitle(t *testing.T) {
	result := extractHTMLTitle("<html><head><title>My Page</title></head><body></body></html>")
	if result != "My Page" {
		t.Errorf("expected 'My Page', got %q", result)
	}
}

func TestExtractHTMLTitleNoTitle(t *testing.T) {
	result := extractHTMLTitle("<html><body></body></html>")
	if result != "" {
		t.Errorf("no title should return empty, got %q", result)
	}
}

func TestExtractHTMLTitleEmptyTitle(t *testing.T) {
	result := extractHTMLTitle("<title></title>")
	if result != "" {
		t.Errorf("empty title should return empty, got %q", result)
	}
}

func TestExtractHTMLTitleWithSpaces(t *testing.T) {
	result := extractHTMLTitle("<title>  Hello World  </title>")
	if result != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", result)
	}
}

func TestExtractHTMLTitleFallbackToH1(t *testing.T) {
	result := extractHTMLTitle("<html><body><h1>Fallback Title</h1></body></html>")
	if result != "Fallback Title" {
		t.Errorf("expected fallback to H1, got %q", result)
	}
}

// ─── extractHTMLMeta ────────────────────────────────────────────────────────

func TestExtractHTMLMetaDescription(t *testing.T) {
	html := `<html><head><meta name="description" content="A test page"></head></html>`
	result := extractHTMLMeta(html, "description")
	if result != "A test page" {
		t.Errorf("expected 'A test page', got %q", result)
	}
}

func TestExtractHTMLMetaProperty(t *testing.T) {
	html := `<html><head><meta property="og:title" content="OG Title"></head></html>`
	result := extractHTMLMeta(html, "og:title")
	if result != "OG Title" {
		t.Errorf("expected 'OG Title', got %q", result)
	}
}

func TestExtractHTMLMetaNotFound(t *testing.T) {
	html := `<html><head></head></html>`
	result := extractHTMLMeta(html, "description")
	if result != "" {
		t.Errorf("no meta should return empty, got %q", result)
	}
}

// ─── stripTag ───────────────────────────────────────────────────────────────

func TestStripTagScript(t *testing.T) {
	result := stripTag("before<script>alert(1)</script>after", "script")
	if result == "" {
		t.Error("should have content after stripping")
	}
	if stripHTMLSimple(result) != "beforeafter" {
		t.Errorf("expected 'beforeafter', got %q", result)
	}
}

func TestStripTagStyle(t *testing.T) {
	result := stripTag("before<style>body{}</style>after", "style")
	if result == "" {
		t.Error("should have content after stripping")
	}
}

func TestStripTagNoTag(t *testing.T) {
	result := stripTag("hello world", "div")
	if result != "hello world" {
		t.Errorf("no tag should return unchanged, got %q", result)
	}
}

// ─── maxBodySize constant ───────────────────────────────────────────────────

func TestMaxBodySize(t *testing.T) {
	if maxBodySize != 1<<20 {
		t.Errorf("expected 1MB, got %d", maxBodySize)
	}
}
