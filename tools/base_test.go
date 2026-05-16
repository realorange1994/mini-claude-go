package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func TestValidateParamsAllPresent(t *testing.T) {
	tool := NewFileReadTool(nil)
	params := map[string]any{"file_path": "test.go"}
	if err := ValidateParams(tool, params); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateParamsMissingRequired(t *testing.T) {
	tool := NewFileReadTool(nil)
	params := map[string]any{}
	err := ValidateParams(tool, params)
	if err == nil {
		t.Error("expected error for missing required param 'path'")
	}
}

func TestValidateParamsNoRequired(t *testing.T) {
	// GlobTool has only "pattern" as required, "directory" is optional
	tool := &GlobTool{}
	params := map[string]any{"pattern": "*.go"}
	if err := ValidateParams(tool, params); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateParamsGrepMissingPattern(t *testing.T) {
	tool := &GrepTool{}
	params := map[string]any{"path": "."}
	err := ValidateParams(tool, params)
	if err == nil {
		t.Error("expected error for missing required param 'pattern'")
	}
}

func TestToolResultMetadataToCompactSummary(t *testing.T) {
	tests := []struct {
		name   string
		meta   ToolResultMetadata
		output string
		check  string
	}{
		{
			"exec success",
			ToolResultMetadata{ToolName: "exec", ExitCode: 0, DurationMs: 150, OutputLines: 47},
			"output text",
			"exec",
		},
		{
			"exec failure",
			ToolResultMetadata{ToolName: "exec", ExitCode: 1, DurationMs: 50, OutputLines: 5},
			"output text",
			"exec",
		},
		{
			"no tool name",
			ToolResultMetadata{ExitCode: 0, DurationMs: 10, OutputLines: 100},
			"output text",
			"lines",
		},
		{
			"long duration",
			ToolResultMetadata{ToolName: "exec", ExitCode: 0, DurationMs: 3500, OutputLines: 10},
			"output text",
			"3.5s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.meta.ToCompactSummary(tt.output)
			if !strings.Contains(got, tt.check) {
				t.Errorf("ToCompactSummary() = %q, expected to contain %q", got, tt.check)
			}
		})
	}
}

func TestToolResultMetadataToCompactSummaryOutputLines(t *testing.T) {
	// When OutputLines is 0, it should count from output
	meta := ToolResultMetadata{ToolName: "read_file", ExitCode: 0, DurationMs: 10, OutputLines: 0}
	got := meta.ToCompactSummary("line1\nline2\nline3")
	if !strings.Contains(got, "3 lines") {
		t.Errorf("expected '3 lines' in summary when OutputLines=0, got %q", got)
	}
}

func TestToolResultWithMetadata(t *testing.T) {
	meta := ToolResultMetadata{
		ToolName:   "exec",
		ExitCode:   0,
		DurationMs: 100,
	}
	result := ToolResult{
		Output:   "hello",
		IsError:  false,
		Metadata: meta,
	}

	if result.Metadata.ToolName != "exec" {
		t.Errorf("expected ToolName=exec, got %q", result.Metadata.ToolName)
	}
	if result.Metadata.ExitCode != 0 {
		t.Errorf("expected ExitCode=0, got %d", result.Metadata.ExitCode)
	}
}

func TestToolResultOK(t *testing.T) {
	r := ToolResultOK("success")
	if r.IsError {
		t.Error("expected IsError=false")
	}
	if r.Output != "success" {
		t.Errorf("expected output 'success', got %q", r.Output)
	}
}

func TestToolResultError(t *testing.T) {
	r := ToolResultError("failed")
	if !r.IsError {
		t.Error("expected IsError=true")
	}
	if r.Output != "failed" {
		t.Errorf("expected output 'failed', got %q", r.Output)
	}
}

func TestToolResultWithMetadataChain(t *testing.T) {
	r := ToolResultOK("ok").WithMetadata(NewToolResultMetadata("read_file", 0))
	if r.Metadata.ToolName != "read_file" {
		t.Errorf("expected ToolName=read_file, got %q", r.Metadata.ToolName)
	}
	if !r.Metadata.ExitCodeSet {
		t.Error("expected ExitCodeSet=true")
	}
}

func TestNewToolResultMetadata(t *testing.T) {
	m := NewToolResultMetadata("exec", 1)
	if m.ToolName != "exec" {
		t.Errorf("expected ToolName=exec, got %q", m.ToolName)
	}
	if m.ExitCode != 1 {
		t.Errorf("expected ExitCode=1, got %d", m.ExitCode)
	}
	if !m.ExitCodeSet {
		t.Error("expected ExitCodeSet=true")
	}
	if !m.IsError() {
		t.Error("expected IsError()=true for exit code 1")
	}
}

func TestToolResultMetadataExitCodeNotSet(t *testing.T) {
	m := ToolResultMetadata{ToolName: "exec"}
	if m.HasExitCode() {
		t.Error("expected HasExitCode()=false when not set")
	}
	if m.IsError() {
		t.Error("expected IsError()=false when exit code not set")
	}
}

func TestToolResultMetadataExitCodeZero(t *testing.T) {
	m := NewToolResultMetadata("exec", 0)
	if !m.HasExitCode() {
		t.Error("expected HasExitCode()=true for explicitly set 0")
	}
	if m.IsError() {
		t.Error("expected IsError()=false for exit code 0")
	}
}

// --- CheckFileStale content comparison tests ---

func TestCheckFileStale_CRLFContentMatch(t *testing.T) {
	// Verify that CheckFileStale correctly compares content when
	// timestamps differ but content is the same (CRLF vs normalized LF).
	dir := t.TempDir()
	fp := filepath.Join(dir, "crlf.txt")

	// Write a file with CRLF line endings
	crlfData := []byte("line1\r\nline2\r\nline3\r\n")
	if err := os.WriteFile(fp, crlfData, 0o644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()

	// Simulate a read_file call: store normalized content (LF) with old mtime
	content, _ := DecodeFileContent(crlfData)
	// content should be "line1\nline2\nline3\n" (LF normalized)
	registry.MarkFileReadWithParams(fp, -1, -1, content, false, false, true)

	// Now simulate a stale check where the mtime has changed.
	// We manually overwrite the stored mtime to force the content comparison path.
	normalized := canonicalPath(fp)
	registry.mu.Lock()
	info := registry.filesRead[normalized]
	info.mtime = info.mtime.Add(-10 * time.Second) // force mtime mismatch
	registry.filesRead[normalized] = info
	registry.mu.Unlock()

	// CheckFileStale should return empty (not stale) because the content
	// comparison decodes the current file and compares normalized content.
	msg := registry.CheckFileStale(fp)
	if msg != "" {
		t.Errorf("CheckFileStale() = %q, want empty (content is same, only CRLF normalized)", msg)
	}
}

func TestCheckFileStale_ReallyModified(t *testing.T) {
	// Verify that CheckFileStale correctly detects real file modifications.
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")

	if err := os.WriteFile(fp, []byte("original content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	registry.MarkFileReadWithParams(fp, -1, -1, "original content\n", false, false, true)

	// Actually modify the file
	if err := os.WriteFile(fp, []byte("modified content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Force mtime mismatch
	normalized := canonicalPath(fp)
	registry.mu.Lock()
	info := registry.filesRead[normalized]
	info.mtime = info.mtime.Add(-10 * time.Second)
	registry.filesRead[normalized] = info
	registry.mu.Unlock()

	msg := registry.CheckFileStale(fp)
	if msg == "" {
		t.Error("CheckFileStale() should return error for modified file")
	}
	if !strings.Contains(msg, "modified since read") {
		t.Errorf("CheckFileStale() = %q, should contain 'modified since read'", msg)
	}
}

func TestCheckFileStale_UTF16LEContentMatch(t *testing.T) {
	// Verify that CheckFileStale correctly compares content for UTF-16 LE files.
	dir := t.TempDir()
	fp := filepath.Join(dir, "utf16le.txt")

	// Write a UTF-16 LE file with BOM
	text := "Hello\nWorld\n"
	runes := []rune(text)
	u16s := utf16.Encode(runes)
	data := []byte{0xFF, 0xFE} // BOM
	for _, v := range u16s {
		data = append(data, byte(v), byte(v>>8))
	}
	if err := os.WriteFile(fp, data, 0o644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()

	// Simulate a read_file call: store decoded content
	content, _ := DecodeFileContent(data)
	registry.MarkFileReadWithParams(fp, -1, -1, content, false, false, true)

	// Force mtime mismatch
	normalized := canonicalPath(fp)
	registry.mu.Lock()
	info := registry.filesRead[normalized]
	info.mtime = info.mtime.Add(-10 * time.Second)
	registry.filesRead[normalized] = info
	registry.mu.Unlock()

	msg := registry.CheckFileStale(fp)
	if msg != "" {
		t.Errorf("CheckFileStale() = %q, want empty (UTF-16 LE content decoded to same string)", msg)
	}
}
