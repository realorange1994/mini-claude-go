package tools

import "testing"

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
