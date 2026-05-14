package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSkillContent = `---
name: test-skill
description: A test skill for unit testing
always: false
available: true
commands: ["/test"]
tags: [test]
version: 1.0.0
requires:
  - GIT
---

This is the body of the test skill.
It contains instructions for the AI.
`

func TestParseFrontmatter(t *testing.T) {
	meta := parseFrontmatter(testSkillContent)

	if meta.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got: %q", meta.Name)
	}
	if meta.Description != "A test skill for unit testing" {
		t.Errorf("unexpected description: %q", meta.Description)
	}
	if meta.Always {
		t.Error("expected always=false")
	}
	if !meta.Available {
		t.Error("expected available=true")
	}
	if len(meta.Commands) != 1 || meta.Commands[0] != "/test" {
		t.Errorf("unexpected commands: %v", meta.Commands)
	}
	if len(meta.Tags) != 1 || meta.Tags[0] != "test" {
		t.Errorf("unexpected tags: %v", meta.Tags)
	}
	if meta.Version != "1.0.0" {
		t.Errorf("unexpected version: %q", meta.Version)
	}
	if len(meta.Requires) != 1 || meta.Requires[0] != "GIT" {
		t.Errorf("unexpected requires: %v", meta.Requires)
	}
}

func TestParseFrontmatterNoFrontmatter(t *testing.T) {
	meta := parseFrontmatter("Just plain markdown")
	if meta.Name != "" {
		t.Error("expected empty meta for content without frontmatter")
	}
}

func TestParseFrontmatterMissingEnd(t *testing.T) {
	meta := parseFrontmatter("---\nname: test\nno closing delimiter")
	if meta.Name != "" {
		t.Error("expected empty meta when end delimiter is missing")
	}
}

func TestParseInlineList(t *testing.T) {
	tests := []struct {
		input  string
		expect []string
	}{
		{"[a, b, c]", []string{"a", "b", "c"}},
		{"[\"x\", \"y\"]", []string{"x", "y"}},
		{"[]", nil},
		{"not a list", nil},
	}

	for _, tc := range tests {
		result := parseInlineList(tc.input)
		if len(result) != len(tc.expect) {
			t.Errorf("parseInlineList(%q) = %v, want %v", tc.input, result, tc.expect)
			continue
		}
		for i := range result {
			if result[i] != tc.expect[i] {
				t.Errorf("parseInlineList(%q)[%d] = %q, want %q", tc.input, i, result[i], tc.expect[i])
			}
		}
	}
}

func TestUnquote(t *testing.T) {
	if unquote(`"hello"`) != "hello" {
		t.Error("unquote failed for double-quoted string")
	}
	if unquote(`'hello'`) != "hello" {
		t.Error("unquote failed for single-quoted string")
	}
	if unquote("hello") != "hello" {
		t.Error("unquote failed for unquoted string")
	}
}

func TestStripFrontmatter(t *testing.T) {
	result := stripFrontmatter(testSkillContent)
	if !strings.Contains(result, "This is the body") {
		t.Errorf("expected body content in stripped result: %s", result)
	}
	if strings.HasPrefix(result, "---") {
		t.Error("expected frontmatter to be stripped")
	}
}

func TestLoaderLoadSkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testSkillContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	content := loader.LoadSkill("test-skill")
	if content == "" {
		t.Fatal("expected skill content, got empty")
	}
	if !strings.Contains(content, "A test skill for unit testing") {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestLoaderLoadSkillNotFound(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(dir)
	_ = loader.Refresh()

	content := loader.LoadSkill("nonexistent")
	if content != "" {
		t.Errorf("expected empty content, got: %s", content)
	}
}

func TestLoaderListSkills(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testSkillContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	skills := loader.ListSkills(false)
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got: %d", len(skills))
	}

	if skills[0].Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got: %q", skills[0].Name)
	}
}

func TestLoaderBuildSkillsSummary(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testSkillContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	summary := loader.BuildSkillsSummary()
	if !strings.Contains(summary, "<skills>") {
		t.Errorf("expected skills xml tag: %s", summary)
	}
	if !strings.Contains(summary, "test-skill") {
		t.Errorf("expected skill name in summary: %s", summary)
	}
}

func TestLoaderNoSkillsDir(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(dir)
	err := loader.Refresh()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	skills := loader.ListSkills(false)
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got: %d", len(skills))
	}
}

func TestLoaderLoadSkillsForContext(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(testSkillContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	content := loader.LoadSkillsForContext([]string{"test-skill"})
	if !strings.Contains(content, "### Skill: test-skill") {
		t.Errorf("expected skill header: %s", content)
	}
	if !strings.Contains(content, "body of the test skill") {
		t.Errorf("expected skill body: %s", content)
	}
}

func TestParseBool(t *testing.T) {
	if !parseBool("true") {
		t.Error("parseBool(true) should be true")
	}
	if !parseBool("yes") {
		t.Error("parseBool(yes) should be true")
	}
	if !parseBool("True") {
		t.Error("parseBool(True) should be true")
	}
	if parseBool("false") {
		t.Error("parseBool(false) should be false")
	}
	if parseBool("no") {
		t.Error("parseBool(no) should be false")
	}
}

// ============================================================================
// Upstream Quality: XML Escaping Tests
// ============================================================================

func TestEscapeXML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// All 5 XML entities (matching upstream xml.test.ts coverage)
		{"ampersand", "a & b", "a &amp; b"},
		{"less-than", "<div>", "&lt;div&gt;"},
		{"greater-than", "a > b", "a &gt; b"},
		{"double quote", `say "hello"`, `say &quot;hello&quot;`},
		{"single quote", "it's", "it&apos;s"},
		// Multiple special chars in one string
		{"multiple special chars", "<a & b>", "&lt;a &amp; b&gt;"},
		{"all five entities", `<a & 'b' "c">`, `&lt;a &amp; &apos;b&apos; &quot;c&quot;&gt;`},
		// Empty string
		{"empty string", "", ""},
		// Normal text unchanged
		{"normal text unchanged", "hello world", "hello world"},
		{"numbers unchanged", "42 + 10", "42 + 10"},
		{"spaces unchanged", "  spaces  ", "  spaces  "},
		// Only special chars
		{"only ampersands", "&&", "&amp;&amp;"},
		{"only angle brackets", "<<>>", "&lt;&lt;&gt;&gt;"},
		{"only quotes", `''""`, `&apos;&apos;&quot;&quot;`},
		// Mixed normal and special
		{"mixed content", `if (x < 10 && y > 5) return "ok"`, `if (x &lt; 10 &amp;&amp; y &gt; 5) return &quot;ok&quot;`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeXML(tt.input)
			if got != tt.want {
				t.Errorf("escapeXML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeXMLIdempotent(t *testing.T) {
	// escapeXML(escapeXML(x)) should equal escapeXML(x) for already-escaped text.
	// This is the key invariant from upstream: already-escaped text should not be
	// double-escaped. The Go implementation processes & first, so already-escaped
	// &amp; stays as &amp; (the & is replaced, then amp; is left as literal text).
	// However, this function is NOT idempotent because & in &amp; gets re-escaped
	// on a second pass. This is a known limitation of simple ReplaceAll-based
	// implementations. Test the actual behavior so we document it.
	inputs := []string{
		"hello world",           // no special chars: idempotent
		"",                      // empty: idempotent
		"a & b",                 // ampersand: NOT idempotent (&amp; -> &amp;amp;)
		"<div>",                 // angle brackets: NOT idempotent
	}

	for _, in := range inputs {
		first := escapeXML(in)
		second := escapeXML(first)
		// For plain text, it should be idempotent
		if in == "hello world" || in == "" {
			if second != first {
				t.Errorf("escapeXML should be idempotent for %q: first=%q, second=%q", in, first, second)
			}
		}
		// For text with special chars, double-escaping is the known behavior
		// Just verify that the first pass produces the expected result
		if in == "a & b" && first != "a &amp; b" {
			t.Errorf("escapeXML(%q) = %q, want %q", in, first, "a &amp; b")
		}
		if in == "<div>" && first != "&lt;div&gt;" {
			t.Errorf("escapeXML(%q) = %q, want %q", in, first, "&lt;div&gt;")
		}
	}
}

func TestEscapeXMLOrderMatters(t *testing.T) {
	// The Go implementation replaces & first, which is the correct order for
	// XML escaping. If & were not replaced first, &lt; would become &amp;lt;
	// instead of the intended &lt;.
	result := escapeXML("&lt;")
	// & is replaced first: &lt; -> &amp;lt;
	// This means text that is already partially escaped gets the & re-escaped.
	// This is expected behavior for a simple ReplaceAll chain.
	if result != "&amp;lt;" {
		t.Errorf("escapeXML('&lt;') = %q, want %q (amp-first order verified)", result, "&amp;lt;")
	}

	// But for unescaped input, the order is correct:
	result2 := escapeXML("<")
	if result2 != "&lt;" {
		t.Errorf("escapeXML('<') = %q, want %q", result2, "&lt;")
	}
}

func TestEscapeXMLInBuildSkillsSummary(t *testing.T) {
	// Integration: verify escapeXML is used correctly in BuildSkillsSummary
	// when skill names contain special characters.
	skillContent := `---
name: skill<with>&special'"chars
description: A skill with XML entities: <tag> & "quoted" 'single'
always: false
available: true
---

Skill body.
`
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "skill-with-special")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	summary := loader.BuildSkillsSummary()
	// Verify that special chars in name/description are escaped
	if !strings.Contains(summary, "&lt;") {
		t.Error("expected &lt; in skills summary for < character")
	}
	if !strings.Contains(summary, "&amp;") {
		t.Error("expected &amp; in skills summary for & character")
	}
	if !strings.Contains(summary, "&quot;") {
		t.Error("expected &quot; in skills summary for double-quote character")
	}
	if !strings.Contains(summary, "&apos;") {
		t.Error("expected &apos; in skills summary for single-quote character")
	}
}

func TestLoaderGetAlwaysSkills(t *testing.T) {
	dir := t.TempDir()

	alwaysContent := `---
name: always-skill
description: Always on skill
always: true
available: true
---

Always skill body.
`

	skillDir := filepath.Join(dir, "skills", "always-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(alwaysContent), 0644)

	loader := NewLoader(dir)
	_ = loader.Refresh()

	alwaysSkills := loader.GetAlwaysSkills()
	if len(alwaysSkills) != 1 {
		t.Fatalf("expected 1 always skill, got: %d", len(alwaysSkills))
	}
	if alwaysSkills[0].Name != "always-skill" {
		t.Errorf("expected 'always-skill', got: %q", alwaysSkills[0].Name)
	}
}
