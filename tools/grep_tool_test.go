package tools

import (
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
		"pattern":      "hello",
		"output_mode":  "invalid_mode",
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
			"pattern":      "nonexistent_xyz",
			"output_mode":  mode,
			"path":         t.TempDir(),
		})
		if result.IsError {
			t.Errorf("valid mode '%s' should not error, got: %s", mode, result.Output)
		}
	}
}
