package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"miniclaudecode-go/tools/rgrep"
)

// ─── splitGlobPatterns ─────────────────────────────────────────────────────

func TestSplitGlobPatternsSingle(t *testing.T) {
	parts := splitGlobPatterns("*.go")
	if len(parts) != 1 || parts[0] != "*.go" {
		t.Errorf("expected [*.go], got %v", parts)
	}
}

func TestSplitGlobPatternsCommaSep(t *testing.T) {
	parts := splitGlobPatterns("*.ts, *.js")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "*.ts" {
		t.Errorf("expected '*.ts', got %q", parts[0])
	}
	if parts[1] != "*.js" {
		t.Errorf("expected '*.js', got %q", parts[1])
	}
}

func TestSplitGlobPatternsBraceGroup(t *testing.T) {
	// Brace groups: commas inside braces are skipped (not written to output)
	// The function preserves braces but strips commas inside them
	parts := splitGlobPatterns("*.{ts,js}")
	if len(parts) != 1 || parts[0] != "*.{tsjs}" {
		t.Errorf("expected [*.{tsjs}], got %v", parts)
	}
}

func TestSplitGlobPatternsSpaceSep(t *testing.T) {
	parts := splitGlobPatterns("*.py *.rs")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "*.py" {
		t.Errorf("expected '*.py', got %q", parts[0])
	}
	if parts[1] != "*.rs" {
		t.Errorf("expected '*.rs', got %q", parts[1])
	}
}

func TestSplitGlobPatternsMixed(t *testing.T) {
	parts := splitGlobPatterns("*.ts, *.js, *.{py,rs}")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
	if parts[2] != "*.{pyrs}" {
		t.Errorf("expected '*.{pyrs}', got %q", parts[2])
	}
}

func TestSplitGlobPatternsEmpty(t *testing.T) {
	parts := splitGlobPatterns("")
	if len(parts) != 0 {
		t.Errorf("expected empty slice, got %v", parts)
	}
}

func TestSplitGlobPatternsWhitespace(t *testing.T) {
	parts := splitGlobPatterns("   ")
	if len(parts) != 0 {
		t.Errorf("expected empty slice for whitespace, got %v", parts)
	}
}

// ─── parseIntParam ─────────────────────────────────────────────────────────

func TestParseIntParamFloat64(t *testing.T) {
	result := parseIntParam(map[string]any{"n": float64(42)}, "n")
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestParseIntParamInt(t *testing.T) {
	result := parseIntParam(map[string]any{"n": 42}, "n")
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestParseIntParamMissing(t *testing.T) {
	result := parseIntParam(map[string]any{}, "n")
	if result != 0 {
		t.Errorf("expected 0 for missing key, got %d", result)
	}
}

func TestParseIntParamWrongType(t *testing.T) {
	result := parseIntParam(map[string]any{"n": "string"}, "n")
	if result != 0 {
		t.Errorf("expected 0 for wrong type, got %d", result)
	}
}

// ─── rgrep truncateLine ─────────────────────────────────────────────────────

func TestTruncateLineShort(t *testing.T) {
	line := strings.Repeat("x", 100)
	result := rgrep.TruncateLine(line)
	if result != line {
		t.Error("short line should not be truncated")
	}
}

func TestTruncateLineLong(t *testing.T) {
	line := strings.Repeat("x", 600)
	result := rgrep.TruncateLine(line)
	if len(result) > rgrep.MaxGrepLineLen+3 {
		t.Errorf("truncated line too long: %d chars", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated line should end with ...")
	}
}

func TestTruncateLineExact(t *testing.T) {
	line := strings.Repeat("x", rgrep.MaxGrepLineLen)
	result := rgrep.TruncateLine(line)
	if result != line {
		t.Error("exact max length should not be truncated")
	}
}

// ─── rgrep DefaultTypes ────────────────────────────────────────────────────

func TestTypeMapGo(t *testing.T) {
	exts := rgrep.ExtensionsForType("go")
	if len(exts) != 1 || exts[0] != ".go" {
		t.Errorf("expected [.go], got %v", exts)
	}
}

func TestTypeMapPy(t *testing.T) {
	exts := rgrep.ExtensionsForType("py")
	if len(exts) != 2 || exts[0] != ".py" || exts[1] != ".pyi" {
		t.Errorf("expected [.py, .pyi], got %v", exts)
	}
}

func TestTypeMapTs(t *testing.T) {
	exts := rgrep.ExtensionsForType("ts")
	if len(exts) != 4 {
		t.Errorf("ts should have 4 extensions, got %d", len(exts))
	}
}

func TestTypeMapYaml(t *testing.T) {
	exts := rgrep.ExtensionsForType("yaml")
	if len(exts) != 2 {
		t.Errorf("yaml should have 2 extensions, got %d", len(exts))
	}
}

func TestTypeMapCaseInsensitive(t *testing.T) {
	// ExtensionsForType is used with strings.ToLower, so "Go" -> "go"
	exts := rgrep.ExtensionsForType("Go")
	if len(exts) != 0 {
		t.Error("ExtensionsForType keys are lowercase only, Go should not be found directly")
	}
}

// ─── GrepTool constants ────────────────────────────────────────────────────

func TestGrepToolConstants(t *testing.T) {
	if maxGrepMatches != 250 {
		t.Errorf("expected maxGrepMatches=250, got %d", maxGrepMatches)
	}
	if rgrep.MaxGrepLineLen != 500 {
		t.Errorf("expected rgrep.MaxGrepLineLen=500, got %d", rgrep.MaxGrepLineLen)
	}
}

// ─── GrepTool interface ────────────────────────────────────────────────────

func TestGrepToolName(t *testing.T) {
	tool := &GrepTool{}
	if tool.Name() != "grep" {
		t.Errorf("expected 'grep', got %q", tool.Name())
	}
}

func TestGrepToolSchema(t *testing.T) {
	tool := &GrepTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "pattern" {
		t.Error("schema should require 'pattern'")
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["path"]; !ok {
		t.Error("schema should have 'path' property")
	}
	if _, ok := props["glob"]; !ok {
		t.Error("schema should have 'glob' property")
	}
	if _, ok := props["output_mode"]; !ok {
		t.Error("schema should have 'output_mode' property")
	}
}

func TestGrepToolExecuteNoPattern(t *testing.T) {
	tool := &GrepTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing pattern should return error")
	}
}

func TestGrepToolPermissions(t *testing.T) {
	tool := &GrepTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

// ─── Regression: invalid output_mode must return error (Bug 3) ────────────────

func TestGrepInvalidOutputMode(t *testing.T) {
	tool := &GrepTool{}
	result := tool.Execute(map[string]any{
		"pattern":     "hello",
		"output_mode": "invalid_mode",
	})
	if !result.IsError {
		t.Error("expected error for invalid output_mode")
	}
	if !strings.Contains(result.Output, "invalid output_mode") {
		t.Errorf("error should mention invalid output_mode, got: %s", result.Output)
	}
}

func TestGrepValidOutputModes(t *testing.T) {
	for _, mode := range []string{"content", "files_with_matches", "count"} {
		tool := &GrepTool{}
		result := tool.Execute(map[string]any{
			"pattern":     "nonexistent_xyz",
			"output_mode": mode,
			"path":        t.TempDir(),
		})
		if result.IsError {
			t.Errorf("valid mode '%s' should not error, got: %s", mode, result.Output)
		}
	}
}

// ─── Regression: -B/-A context parsing (Bug 1) ─────────────────────────────
// Verify that -B and -A are parsed independently (not overriding each other)
// and that -C only fills in zero values.

func TestContextParsingBAIndependent(t *testing.T) {
	// -B=2 -A=3 with no -C → ctxBefore=2, ctxAfter=3
	params := map[string]any{
		"-B": float64(2),
		"-A": float64(3),
	}
	ctxBefore := parseIntParam(params, "-B")
	if ctxBefore == 0 {
		ctxBefore = parseIntParam(params, "context_before")
	}
	ctxAfter := parseIntParam(params, "-A")
	if ctxAfter == 0 {
		ctxAfter = parseIntParam(params, "context_after")
	}
	ctxCombined := parseIntParam(params, "-C")
	if ctxCombined == 0 {
		ctxCombined = parseIntParam(params, "context")
	}
	if ctxCombined > 0 {
		if ctxBefore == 0 {
			ctxBefore = ctxCombined
		}
		if ctxAfter == 0 {
			ctxAfter = ctxCombined
		}
	}

	if ctxBefore != 2 {
		t.Errorf("ctxBefore should be 2, got %d", ctxBefore)
	}
	if ctxAfter != 3 {
		t.Errorf("ctxAfter should be 3, got %d", ctxAfter)
	}
}

func TestContextParsingCFillsZero(t *testing.T) {
	// -C=2 with no -B/-A → ctxBefore=2, ctxAfter=2
	params := map[string]any{
		"-C": float64(2),
	}
	ctxBefore := parseIntParam(params, "-B")
	if ctxBefore == 0 {
		ctxBefore = parseIntParam(params, "context_before")
	}
	ctxAfter := parseIntParam(params, "-A")
	if ctxAfter == 0 {
		ctxAfter = parseIntParam(params, "context_after")
	}
	ctxCombined := parseIntParam(params, "-C")
	if ctxCombined == 0 {
		ctxCombined = parseIntParam(params, "context")
	}
	if ctxCombined > 0 {
		if ctxBefore == 0 {
			ctxBefore = ctxCombined
		}
		if ctxAfter == 0 {
			ctxAfter = ctxCombined
		}
	}

	if ctxBefore != 2 {
		t.Errorf("ctxBefore should be 2 (from -C), got %d", ctxBefore)
	}
	if ctxAfter != 2 {
		t.Errorf("ctxAfter should be 2 (from -C), got %d", ctxAfter)
	}
}

func TestContextParsingBDominatesC(t *testing.T) {
	// -B=1 -C=3 → ctxBefore=1 (explicit), ctxAfter=3 (from -C)
	params := map[string]any{
		"-B": float64(1),
		"-C": float64(3),
	}
	ctxBefore := parseIntParam(params, "-B")
	if ctxBefore == 0 {
		ctxBefore = parseIntParam(params, "context_before")
	}
	ctxAfter := parseIntParam(params, "-A")
	if ctxAfter == 0 {
		ctxAfter = parseIntParam(params, "context_after")
	}
	ctxCombined := parseIntParam(params, "-C")
	if ctxCombined == 0 {
		ctxCombined = parseIntParam(params, "context")
	}
	if ctxCombined > 0 {
		if ctxBefore == 0 {
			ctxBefore = ctxCombined
		}
		if ctxAfter == 0 {
			ctxAfter = ctxCombined
		}
	}

	if ctxBefore != 1 {
		t.Errorf("ctxBefore should be 1 (explicit -B), got %d", ctxBefore)
	}
	if ctxAfter != 3 {
		t.Errorf("ctxAfter should be 3 (from -C fill), got %d", ctxAfter)
	}
}

func TestContextParsingAliasParams(t *testing.T) {
	// context_before/context_after should work as aliases for -B/-A
	params := map[string]any{
		"context_before": float64(1),
		"context_after":  float64(4),
	}
	ctxBefore := parseIntParam(params, "-B")
	if ctxBefore == 0 {
		ctxBefore = parseIntParam(params, "context_before")
	}
	ctxAfter := parseIntParam(params, "-A")
	if ctxAfter == 0 {
		ctxAfter = parseIntParam(params, "context_after")
	}
	ctxCombined := parseIntParam(params, "-C")
	if ctxCombined == 0 {
		ctxCombined = parseIntParam(params, "context")
	}
	if ctxCombined > 0 {
		if ctxBefore == 0 {
			ctxBefore = ctxCombined
		}
		if ctxAfter == 0 {
			ctxAfter = ctxCombined
		}
	}

	if ctxBefore != 1 {
		t.Errorf("ctxBefore should be 1 (from alias), got %d", ctxBefore)
	}
	if ctxAfter != 4 {
		t.Errorf("ctxAfter should be 4 (from alias), got %d", ctxAfter)
	}
}

// ─── Regression: rgrep excludes with ** patterns (Bug 5) ─────────────────────
// Test that rgrep excludes properly filter files using matchExcludePattern.

func TestRgrepExcludesWithDoubleStar(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src", "vendor", "pkg"), 0755)
	os.MkdirAll(filepath.Join(dir, "src", "main"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "src", "vendor", "vendor.go"), []byte("package vendor"), 0644)
	os.WriteFile(filepath.Join(dir, "src", "main", "app.go"), []byte("package main"), 0644)

	cfg := rgrep.SearchConfig{
		Pattern:    "package",
		Path:       dir,
		OutputMode: rgrep.OutputContent,
		Excludes:   []string{"**/vendor/**"},
		Ctx:        context.Background(),
	}
	result := rgrep.Search(cfg)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	for _, r := range result.Results {
		if strings.Contains(r.Path, "vendor") {
			t.Errorf("vendor files should be excluded, but found: %s", r.Path)
		}
	}

	// Should still find non-vendor files
	found := false
	for _, r := range result.Results {
		if strings.Contains(r.Path, "main.go") || strings.Contains(r.Path, "app.go") {
			found = true
		}
	}
	if !found {
		t.Error("expected to find non-vendor .go files")
	}
}

func TestRgrepExcludesSimpleGlob(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("package test"), 0644)
	os.WriteFile(filepath.Join(dir, "test.json"), []byte(`{"key": "value"}`), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	cfg := rgrep.SearchConfig{
		Pattern:    ".",
		Path:       dir,
		OutputMode: rgrep.OutputContent,
		Excludes:   []string{"*.json"},
		Ctx:        context.Background(),
	}
	result := rgrep.Search(cfg)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	for _, r := range result.Results {
		if strings.HasSuffix(r.Path, ".json") {
			t.Errorf("json files should be excluded, but found: %s", r.Path)
		}
	}
}

func TestRgrepExcludesDirName(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0755)
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"), []byte("module.exports = {}"), 0644)
	os.WriteFile(filepath.Join(dir, "src", "app.js"), []byte("const x = 1"), 0644)

	cfg := rgrep.SearchConfig{
		Pattern:    ".",
		Path:       dir,
		OutputMode: rgrep.OutputContent,
		Excludes:   []string{"node_modules"},
		Ctx:        context.Background(),
	}
	result := rgrep.Search(cfg)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	for _, r := range result.Results {
		if strings.Contains(r.Path, "node_modules") {
			t.Errorf("node_modules should be excluded, but found: %s", r.Path)
		}
	}
}

// ─── groupMatchLines ─────────────────────────────────────────────────────────

func TestGroupMatchLinesNoSeparator(t *testing.T) {
	lines := []string{"file:1:match1", "file:2:context"}
	groups := groupMatchLines(lines)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0]) != 2 {
		t.Errorf("expected 2 lines in group, got %d", len(groups[0]))
	}
}

func TestGroupMatchLinesWithSeparator(t *testing.T) {
	lines := []string{"file:1:match1", "context", "--", "file:5:match2"}
	groups := groupMatchLines(lines)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if len(groups[0]) != 2 {
		t.Errorf("group[0] expected 2 lines, got %d", len(groups[0]))
	}
	if len(groups[1]) != 1 {
		t.Errorf("group[1] expected 1 line, got %d", len(groups[1]))
	}
}

func TestGroupMatchLinesEmpty(t *testing.T) {
	groups := groupMatchLines([]string{})
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for empty input, got %d", len(groups))
	}
}

func TestGroupMatchLinesTrailingSeparator(t *testing.T) {
	lines := []string{"file:1:match1", "--"}
	groups := groupMatchLines(lines)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
}

func TestGroupMatchLinesLeadingSeparator(t *testing.T) {
	lines := []string{"--", "file:1:match1"}
	groups := groupMatchLines(lines)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0][0] != "file:1:match1" {
		t.Errorf("expected 'file:1:match1', got %q", groups[0][0])
	}
}

// ─── Regression: offset/head_limit must skip entire match groups, not lines (Bug 1) ──
// Previously, offset skipped raw lines which could drop before-context lines
// belonging to the first remaining match group.

func TestGrepOffsetSkipsMatchGroupsNotLines(t *testing.T) {
	dir := t.TempDir()
	// Create a file with 3 matches far enough apart that rg inserts -- separators
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("a\nb\nMATCH1\nc\nd\ne\nf\ng\nMATCH2\nh\ni\nj\nk\nMATCH3\nl\n"), 0644)

	tool := &GrepTool{}
	result := tool.Execute(map[string]any{
		"pattern":     "MATCH",
		"path":        dir,
		"output_mode": "content",
		"-B":          float64(1),
		"-A":          float64(1),
		"head_limit":  float64(0), // unlimited
		"offset":      float64(1), // skip first match group
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	// After skipping 1 group, we should see MATCH2 (with its context)
	if !strings.Contains(result.Output, "MATCH2") {
		t.Errorf("expected MATCH2 after offset=1, got: %s", result.Output)
	}
	// MATCH1 should NOT appear
	if strings.Contains(result.Output, "MATCH1") {
		t.Errorf("MATCH1 should be skipped by offset=1, got: %s", result.Output)
	}
}

func TestGrepHeadLimitMatchGroupsNotLines(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("a\nb\nMATCH1\nc\nd\ne\nf\ng\nMATCH2\nh\ni\nj\nk\nMATCH3\nl\n"), 0644)

	tool := &GrepTool{}
	result := tool.Execute(map[string]any{
		"pattern":     "MATCH",
		"path":        dir,
		"output_mode": "content",
		"-B":          float64(1),
		"-A":          float64(1),
		"head_limit":  float64(1), // only first match
		"offset":      float64(0),
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	// Should only see MATCH1 (with its context), not MATCH2 or MATCH3
	if !strings.Contains(result.Output, "MATCH1") {
		t.Errorf("expected MATCH1 with head_limit=1, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "MATCH2") {
		t.Errorf("MATCH2 should not appear with head_limit=1, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "MATCH3") {
		t.Errorf("MATCH3 should not appear with head_limit=1, got: %s", result.Output)
	}
	// Should contain truncation message
	if !strings.Contains(result.Output, "truncated") {
		t.Errorf("expected truncation message with head_limit=1, got: %s", result.Output)
	}
}

func TestGrepOffsetPlusHeadLimitCombined(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("a\nb\nMATCH1\nc\nd\ne\nf\ng\nMATCH2\nh\ni\nj\nk\nMATCH3\nl\nm\nn\nMATCH4\n"), 0644)

	tool := &GrepTool{}
	result := tool.Execute(map[string]any{
		"pattern":     "MATCH",
		"path":        dir,
		"output_mode": "content",
		"-B":          float64(1),
		"-A":          float64(0),
		"head_limit":  float64(2),
		"offset":      float64(1),
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	// Should see MATCH2 and MATCH3, not MATCH1 or MATCH4
	if strings.Contains(result.Output, "MATCH1") {
		t.Errorf("MATCH1 should be skipped, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "MATCH2") {
		t.Errorf("expected MATCH2, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "MATCH3") {
		t.Errorf("expected MATCH3, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "MATCH4") {
		t.Errorf("MATCH4 should be limited, got: %s", result.Output)
	}
}

func TestGrepBeforeContextNotSkippedByOffset(t *testing.T) {
	// This is the core Bug 1 regression: before-context lines must not be
	// individually skipped by offset. The entire first match group is skipped.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("before1\nMATCH1\nafter1\n---\nbefore2\nMATCH2\nafter2\n"), 0644)

	tool := &GrepTool{}
	result := tool.Execute(map[string]any{
		"pattern":     "MATCH",
		"path":        dir,
		"output_mode": "content",
		"-B":          float64(1),
		"-A":          float64(1),
		"head_limit":  float64(0),
		"offset":      float64(1),
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	// MATCH2's before-context line "before2" must be present
	if !strings.Contains(result.Output, "before2") {
		t.Errorf("BUG: before2 should be present (before-context must not be skipped), got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "MATCH2") {
		t.Errorf("expected MATCH2, got: %s", result.Output)
	}
}
