package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── Config Variable Tests ────────────────────────────────────────────────

func TestExpandConfigVariables_EnvVar(t *testing.T) {
	os.Setenv("TEST_API_KEY", "sk-test-123")
	defer os.Unsetenv("TEST_API_KEY")

	result := ExpandConfigVariables("key={env:TEST_API_KEY}", "")
	if result != "key=sk-test-123" {
		t.Errorf("expected 'key=sk-test-123', got %q", result)
	}
}

func TestExpandConfigVariables_EnvVarNotFound(t *testing.T) {
	result := ExpandConfigVariables("key={env:NONEXISTENT}", "")
	if result != "key={env:NONEXISTENT}" {
		t.Errorf("expected 'key={env:NONEXISTENT}', got %q", result)
	}
}

func TestExpandConfigVariables_FileVar(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "secret.txt")
	os.WriteFile(filePath, []byte("my-secret"), 0644)

	result := ExpandConfigVariables("secret={file:"+filePath+"}", "")
	if result != "secret=my-secret" {
		t.Errorf("expected 'secret=my-secret', got %q", result)
	}
}

func TestExpandConfigVariables_NoVariables(t *testing.T) {
	result := ExpandConfigVariables("plain text", "")
	if result != "plain text" {
		t.Errorf("expected 'plain text', got %q", result)
	}
}

func TestExpandConfigMap(t *testing.T) {
	os.Setenv("TEST_VAR", "value")
	defer os.Unsetenv("TEST_VAR")

	config := map[string]any{
		"key": "{env:TEST_VAR}",
		"num": 42,
	}

	result := ExpandConfigMap(config, "")
	if result["key"] != "value" {
		t.Errorf("expected 'value', got %v", result["key"])
	}
	if result["num"] != 42 {
		t.Errorf("expected 42, got %v", result["num"])
	}
}

// ─── Never-Ask Mode Tests ────────────────────────────────────────────────

func TestNeverAskConfig_Disabled(t *testing.T) {
	c := NewNeverAskConfig(false)
	if c.IsEnabled() {
		t.Error("expected disabled")
	}
}

func TestNeverAskConfig_Enabled(t *testing.T) {
	c := NewNeverAskConfig(true)
	if !c.IsEnabled() {
		t.Error("expected enabled")
	}
}

func TestNeverAskConfig_SetEnabled(t *testing.T) {
	c := NewNeverAskConfig(false)
	c.SetEnabled(true)
	if !c.IsEnabled() {
		t.Error("expected enabled after set")
	}
}

func TestBuildNeverAskPrompt(t *testing.T) {
	prompt := BuildNeverAskPrompt("Which option?", []string{"A", "B", "C"})
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestShouldNeverAsk(t *testing.T) {
	if ShouldNeverAsk(nil) {
		t.Error("expected false for nil config")
	}
	if ShouldNeverAsk(NewNeverAskConfig(false)) {
		t.Error("expected false when disabled")
	}
	if !ShouldNeverAsk(NewNeverAskConfig(true)) {
		t.Error("expected true when enabled")
	}
}

// ─── Plan Exit Tool Tests ────────────────────────────────────────────────

func TestPlanExitTool_Approved(t *testing.T) {
	tool := NewPlanExitTool()
	tool.SetPlanPath("/path/to/plan.md")

	result := tool.Execute(true)
	if !result.Approved {
		t.Error("expected approved")
	}
}

func TestPlanExitTool_Rejected(t *testing.T) {
	tool := NewPlanExitTool()
	tool.SetPlanPath("/path/to/plan.md")

	result := tool.Execute(false)
	if result.Approved {
		t.Error("expected rejected")
	}
}

func TestPlanExitTool_OnApprove(t *testing.T) {
	tool := NewPlanExitTool()
	called := false
	tool.SetOnApprove(func() { called = true })

	tool.Execute(true)
	if !called {
		t.Error("expected onApprove to be called")
	}
}

func TestBuildPlanExitPrompt(t *testing.T) {
	prompt := BuildPlanExitPrompt("/path/to/plan.md")
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
}

// ─── VCS Branch Watcher Tests ────────────────────────────────────────────

func TestVCSBranchWatcher_New(t *testing.T) {
	dir := t.TempDir()
	w := NewVCSBranchWatcher(dir)
	if w == nil {
		t.Error("expected non-nil watcher")
	}
}

func TestVCSBranchWatcher_GetCurrentBranch(t *testing.T) {
	dir := t.TempDir()
	w := NewVCSBranchWatcher(dir)

	branch := w.GetCurrentBranch()
	// Should return empty or actual branch
	if branch == "" {
		t.Log("no git repo, expected empty branch")
	}
}

func TestFormatBranchEvent(t *testing.T) {
	event := VCSBranchEvent{
		OldBranch: "main",
		NewBranch: "feature",
	}

	result := FormatBranchEvent(event)
	if result == "" {
		t.Error("expected non-empty result")
	}
}

// ─── Per-Section Budget Tests ────────────────────────────────────────────

func TestSectionBudget_Defined(t *testing.T) {
	if len(SectionBudget) == 0 {
		t.Error("expected non-empty section budgets")
	}
}

// ─── Compose Agent Tests ─────────────────────────────────────────────────

func TestComposeAgent_New(t *testing.T) {
	a := NewComposeAgent()
	if a == nil {
		t.Error("expected non-nil agent")
	}
}

func TestComposeAgent_GetMode(t *testing.T) {
	a := NewComposeAgent()
	if a.GetMode() != ComposeModeCompose {
		t.Errorf("expected compose mode, got %s", a.GetMode())
	}
}

func TestComposeAgent_SetMode(t *testing.T) {
	a := NewComposeAgent()
	a.SetMode(ComposeModeBuild)
	if a.GetMode() != ComposeModeBuild {
		t.Errorf("expected build mode, got %s", a.GetMode())
	}
}

func TestComposeAgent_GetSkills(t *testing.T) {
	a := NewComposeAgent()
	skills := a.GetSkills()
	if len(skills) == 0 {
		t.Error("expected non-empty skills")
	}
}

func TestBuildComposePrompt(t *testing.T) {
	prompt := BuildComposePrompt("test message")
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
}

// ─── Managed Config Tests ────────────────────────────────────────────────

func TestManagedConfig_New(t *testing.T) {
	c := NewManagedConfig()
	if c == nil {
		t.Error("expected non-nil config")
	}
}

func TestManagedConfig_Load(t *testing.T) {
	c := NewManagedConfig()
	err := c.Load()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestManagedConfig_Get(t *testing.T) {
	c := NewManagedConfig()
	c.Load()

	// No managed config, should return nil
	if c.Get("key") != nil {
		t.Error("expected nil for non-existent key")
	}
}

func TestManagedConfig_IsLoaded(t *testing.T) {
	c := NewManagedConfig()
	if c.IsLoaded() {
		t.Error("expected not loaded initially")
	}

	c.Load()
	if !c.IsLoaded() {
		t.Error("expected loaded after Load()")
	}
}

func TestManagedConfig_MergeWithConfig(t *testing.T) {
	c := NewManagedConfig()
	c.Load()

	userConfig := map[string]any{
		"key1": "value1",
	}

	result := c.MergeWithConfig(userConfig)
	if result["key1"] != "value1" {
		t.Errorf("expected 'value1', got %v", result["key1"])
	}
}
