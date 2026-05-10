package tools

import (
	"testing"
)

// ─── ToolResult helpers ─────────────────────────────────────────────────────

func TestToolResultOKExtended(t *testing.T) {
	r := ToolResultOK("success")
	if r.Output != "success" {
		t.Errorf("expected 'success', got %q", r.Output)
	}
	if r.IsError {
		t.Error("should not be error")
	}
}

func TestToolResultErrorExtended(t *testing.T) {
	r := ToolResultError("fail")
	if !r.IsError {
		t.Error("should be error")
	}
	if r.Output != "fail" {
		t.Errorf("expected 'fail', got %q", r.Output)
	}
}

func TestToolResultWithMetadataExtended(t *testing.T) {
	r := ToolResultOK("data")
	r = r.WithMetadata(ToolResultMetadata{ToolName: "test"})
	if r.Metadata.ToolName != "test" {
		t.Errorf("expected tool name 'test', got %q", r.Metadata.ToolName)
	}
}

// ─── ToolResultMetadata ─────────────────────────────────────────────────────

func TestMetadataHasExitCode(t *testing.T) {
	m := NewToolResultMetadata("test", 0)
	if !m.HasExitCode() {
		t.Error("should have exit code set")
	}
}

func TestMetadataHasExitCodeDefault(t *testing.T) {
	m := ToolResultMetadata{}
	if m.HasExitCode() {
		t.Error("default metadata should not have exit code set")
	}
}

func TestMetadataIsError(t *testing.T) {
	m := NewToolResultMetadata("test", 1)
	if !m.IsError() {
		t.Error("non-zero exit code should be error")
	}
}

func TestMetadataIsErrorZero(t *testing.T) {
	m := NewToolResultMetadata("test", 0)
	if m.IsError() {
		t.Error("zero exit code should not be error")
	}
}

func TestMetadataIsErrorNotSet(t *testing.T) {
	m := ToolResultMetadata{}
	if m.IsError() {
		t.Error("unset exit code should not be error")
	}
}

// ─── NewToolResultMetadata ──────────────────────────────────────────────────

func TestNewToolResultMetadataExtended(t *testing.T) {
	m := NewToolResultMetadata("exec", 42)
	if m.ToolName != "exec" {
		t.Errorf("expected 'exec', got %q", m.ToolName)
	}
	if m.ExitCode != 42 {
		t.Errorf("expected 42, got %d", m.ExitCode)
	}
	if !m.ExitCodeSet {
		t.Error("ExitCodeSet should be true")
	}
}

// ─── ToCompactSummary ───────────────────────────────────────────────────────

func TestToCompactSummaryOK(t *testing.T) {
	m := NewToolResultMetadata("read_file", 0)
	summary := m.ToCompactSummary("line1\nline2")
	if summary == "" {
		t.Error("summary should not be empty")
	}
}

func TestToCompactSummaryError(t *testing.T) {
	m := NewToolResultMetadata("exec", 1)
	summary := m.ToCompactSummary("some output")
	if summary == "" {
		t.Error("summary should not be empty")
	}
}

func TestToCompactSummaryDetectsError(t *testing.T) {
	m := NewToolResultMetadata("read_file", 0)
	summary := m.ToCompactSummary("Error: file not found")
	if summary == "" {
		t.Error("summary should not be empty for Error: output")
	}
}

func TestToCompactSummaryNoToolName(t *testing.T) {
	m := ToolResultMetadata{ExitCodeSet: true, ExitCode: 0}
	summary := m.ToCompactSummary("output")
	if summary == "" {
		t.Error("summary should not be empty")
	}
}

func TestToCompactSummaryDurationMs(t *testing.T) {
	m := NewToolResultMetadata("exec", 0)
	m.DurationMs = 1500
	summary := m.ToCompactSummary("output")
	if summary == "" {
		t.Error("summary should not be empty")
	}
}

// ─── PermissionResult ───────────────────────────────────────────────────────

func TestPermissionResultAllow(t *testing.T) {
	r := PermissionResultAllow()
	if r.Behavior != PermissionAllow {
		t.Errorf("expected PermissionAllow, got %v", r.Behavior)
	}
}

func TestPermissionResultDeny(t *testing.T) {
	r := PermissionResultDeny("not allowed")
	if r.Behavior != PermissionDeny {
		t.Errorf("expected PermissionDeny, got %v", r.Behavior)
	}
	if !r.IsBypassImmune() {
		t.Error("deny should be bypass-immune")
	}
}

func TestPermissionResultAsk(t *testing.T) {
	r := PermissionResultAsk("confirm?", "safetyCheck")
	if r.Behavior != PermissionAsk {
		t.Errorf("expected PermissionAsk, got %v", r.Behavior)
	}
	if !r.IsBypassImmune() {
		t.Error("ask with safetyCheck should be bypass-immune")
	}
}

func TestPermissionResultAskNotClassifiable(t *testing.T) {
	r := PermissionResultAskNotClassifiable("dangerous", "safetyCheck")
	if r.Behavior != PermissionAsk {
		t.Errorf("expected PermissionAsk, got %v", r.Behavior)
	}
	if r.ClassifierApprovable {
		t.Error("not-classifiable should not be approvable")
	}
}

func TestPermissionResultPassthrough(t *testing.T) {
	r := PermissionResultPassthrough()
	if r.Behavior != PermissionPassthrough {
		t.Errorf("expected PermissionPassthrough, got %v", r.Behavior)
	}
	if r.IsBypassImmune() {
		t.Error("passthrough should not be bypass-immune")
	}
}

func TestIsBypassImmuneAllow(t *testing.T) {
	r := PermissionResult{Behavior: PermissionAllow}
	if r.IsBypassImmune() {
		t.Error("allow should not be bypass-immune")
	}
}

func TestIsBypassImmuneAskTool(t *testing.T) {
	r := PermissionResult{Behavior: PermissionAsk, DecisionReason: "tool"}
	if r.IsBypassImmune() {
		t.Error("ask with tool reason should not be bypass-immune")
	}
}

// ─── ValidateParams ─────────────────────────────────────────────────────────

func TestValidateParamsRequired(t *testing.T) {
	tool := &MemoryAddTool{}
	err := ValidateParams(tool, map[string]any{})
	if err == nil {
		t.Error("missing required params should return error")
	}
}

func TestValidateParamsEnumValid(t *testing.T) {
	tool := &MemoryAddTool{}
	err := ValidateParams(tool, map[string]any{
		"category": "preference",
		"content":  "test",
	})
	if err != nil {
		t.Errorf("valid enum should not error: %v", err)
	}
}

func TestValidateParamsEnumInvalid(t *testing.T) {
	tool := &MemoryAddTool{}
	err := ValidateParams(tool, map[string]any{
		"category": "invalid_category",
		"content":  "test",
	})
	if err == nil {
		t.Error("invalid enum value should return error")
	}
}

func TestValidateParamsNotRequired(t *testing.T) {
	// Tool with no required params should pass
	tool := &FileOpsTool{}
	err := ValidateParams(tool, map[string]any{})
	if err != nil {
		t.Errorf("no required params should not error: %v", err)
	}
}

// ─── InternalToUpstreamName ─────────────────────────────────────────────────

func TestInternalToUpstreamRead(t *testing.T) {
	if InternalToUpstreamName(FileReadToolName) != "Read" {
		t.Error("read_file should map to Read")
	}
}

func TestInternalToUpstreamWrite(t *testing.T) {
	if InternalToUpstreamName(FileWriteToolName) != "Write" {
		t.Error("write_file should map to Write")
	}
}

func TestInternalToUpstreamEdit(t *testing.T) {
	if InternalToUpstreamName(FileEditToolName) != "Edit" {
		t.Error("edit_file should map to Edit")
	}
}

func TestInternalToUpstreamBash(t *testing.T) {
	if InternalToUpstreamName(ExecToolName) != "Bash" {
		t.Error("exec should map to Bash")
	}
}

func TestInternalToUpstreamNoAlias(t *testing.T) {
	if InternalToUpstreamName("custom_tool") != "custom_tool" {
		t.Error("no alias should return unchanged")
	}
}

// ─── normalizeFilePath ──────────────────────────────────────────────────────

func TestNormalizeFilePath(t *testing.T) {
	// Should lowercase and normalize slashes
	result := normalizeFilePath("E:\\Workspace\\main.go")
	if result != "e:/workspace/main.go" {
		t.Errorf("expected 'e:/workspace/main.go', got %q", result)
	}
}

func TestNormalizeFilePathDots(t *testing.T) {
	result := normalizeFilePath("/home/user/../other/file.go")
	if result != "/home/other/file.go" {
		t.Errorf("expected '/home/other/file.go', got %q", result)
	}
}

// ─── RestoreCRLF ────────────────────────────────────────────────────────────

func TestRestoreCRLF(t *testing.T) {
	input := "line1\nline2\n"
	result := RestoreCRLF(input)
	if result != "line1\r\nline2\r\n" {
		t.Errorf("expected 'line1\\r\\nline2\\r\\n', got %q", result)
	}
}

func TestRestoreCRLFAlreadyCRLF(t *testing.T) {
	input := "line1\r\nline2\r\n"
	result := RestoreCRLF(input)
	if result != "line1\r\nline2\r\n" {
		t.Errorf("already CRLF should remain unchanged, got %q", result)
	}
}

func TestRestoreCRLFEmpty(t *testing.T) {
	if RestoreCRLF("") != "" {
		t.Error("empty string should remain empty")
	}
}

func TestRestoreCRLFNoNewline(t *testing.T) {
	if RestoreCRLF("hello") != "hello" {
		t.Error("text without newlines should remain unchanged")
	}
}

// ─── Registry basic tests ───────────────────────────────────────────────────

func TestRegistryNew(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry should not return nil")
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tool := &MemoryAddTool{}
	r.Register(tool)
	got, ok := r.Get("memory_add")
	if !ok {
		t.Fatal("should find registered tool")
	}
	if got.Name() != "memory_add" {
		t.Errorf("expected 'memory_add', got %q", got.Name())
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("should not find missing tool")
	}
}

func TestRegistryAllTools(t *testing.T) {
	r := NewRegistry()
	r.Register(&MemoryAddTool{})
	r.Register(&MemorySearchTool{})
	tools := r.AllTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

func TestRegistryAPISchemas(t *testing.T) {
	r := NewRegistry()
	r.Register(&MemoryAddTool{})
	schemas := r.APISchemas()
	if len(schemas) != 1 {
		t.Errorf("expected 1 schema, got %d", len(schemas))
	}
	if schemas[0]["name"] != "memory_add" {
		t.Errorf("expected name 'memory_add', got %v", schemas[0]["name"])
	}
}

// ─── ExecuteWithContext ─────────────────────────────────────────────────────

func TestExecuteWithContextFallback(t *testing.T) {
	// MemoryAddTool doesn't implement ContextTool interface, so should fallback to Execute
	tool := &MemoryAddTool{OnAdd: func(category, content, source string) {}}
	result := ExecuteWithContext(nil, tool, map[string]any{
		"category": "state",
		"content":  "test",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
}
