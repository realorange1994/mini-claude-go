package tools

import (
	"strings"
	"testing"
)

func TestValidateParamsAllPresent(t *testing.T) {
	tool := &FileReadTool{}
	params := map[string]any{"path": "test.go"}
	if err := ValidateParams(tool, params); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateParamsMissingRequired(t *testing.T) {
	tool := &FileReadTool{}
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
