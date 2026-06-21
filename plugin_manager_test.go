package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginManager_New(t *testing.T) {
	dir := t.TempDir()
	m := NewPluginManager(dir, true)
	if m == nil {
		t.Error("expected non-nil manager")
	}
	if !m.IsEnabled() {
		t.Error("expected enabled")
	}
}

func TestPluginManager_Disabled(t *testing.T) {
	dir := t.TempDir()
	m := NewPluginManager(dir, false)
	if m.IsEnabled() {
		t.Error("expected disabled")
	}
}

func TestPluginManager_Load_NoDir(t *testing.T) {
	dir := t.TempDir()
	m := NewPluginManager(filepath.Join(dir, "nonexistent"), true)

	err := m.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
}

func TestPluginManager_Load_WithPlugins(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	// Create a mock plugin
	pluginPath := filepath.Join(pluginDir, "test-plugin")
	os.MkdirAll(pluginPath, 0755)
	manifest := `{"name": "test-plugin", "version": "1.0.0", "description": "A test plugin"}`
	os.WriteFile(filepath.Join(pluginPath, "plugin.json"), []byte(manifest), 0644)

	m := NewPluginManager(pluginDir, true)
	m.Load()

	plugin := m.GetPlugin("test-plugin")
	if plugin == nil {
		t.Error("expected plugin to be loaded")
	}
	if plugin.Name != "test-plugin" {
		t.Errorf("expected 'test-plugin', got %q", plugin.Name)
	}
}

func TestPluginManager_Register(t *testing.T) {
	dir := t.TempDir()
	m := NewPluginManager(dir, true)

	// Add a mock plugin
	m.plugins["test"] = &Plugin{
		Name:    "test",
		Enabled: true,
		Hooks:   make(map[PluginHookPoint]HookFunc),
	}

	err := m.Register("test", PluginHookPreStop, func(ctx *PluginHookContext) (*PluginHookResult, error) {
		return &PluginHookResult{Continue: true}, nil
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
}

func TestPluginManager_Register_NotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewPluginManager(dir, true)

	err := m.Register("nonexistent", PluginHookPreStop, func(ctx *PluginHookContext) (*PluginHookResult, error) {
		return nil, nil
	})
	if err == nil {
		t.Error("expected error for nonexistent plugin")
	}
}

func TestPluginManager_Execute(t *testing.T) {
	dir := t.TempDir()
	m := NewPluginManager(dir, true)

	executed := false
	m.plugins["test"] = &Plugin{
		Name:    "test",
		Enabled: true,
		Hooks: map[PluginHookPoint]HookFunc{
			PluginHookPreStop: func(ctx *PluginHookContext) (*PluginHookResult, error) {
				executed = true
				return &PluginHookResult{Continue: true}, nil
			},
		},
	}
	m.hooks[PluginHookPreStop] = []*Plugin{m.plugins["test"]}

	ctx := &PluginHookContext{Hook: PluginHookPreStop}
	results, err := m.Execute(PluginHookPreStop, ctx)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !executed {
		t.Error("expected hook to be executed")
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestPluginManager_Execute_Disabled(t *testing.T) {
	dir := t.TempDir()
	m := NewPluginManager(dir, true)

	executed := false
	m.plugins["test"] = &Plugin{
		Name:    "test",
		Enabled: false,
		Hooks: map[PluginHookPoint]HookFunc{
			PluginHookPreStop: func(ctx *PluginHookContext) (*PluginHookResult, error) {
				executed = true
				return nil, nil
			},
		},
	}
	m.hooks[PluginHookPreStop] = []*Plugin{m.plugins["test"]}

	ctx := &PluginHookContext{Hook: PluginHookPreStop}
	m.Execute(PluginHookPreStop, ctx)

	if executed {
		t.Error("expected hook to NOT be executed when disabled")
	}
}

func TestPluginManager_ListPlugins(t *testing.T) {
	dir := t.TempDir()
	m := NewPluginManager(dir, true)

	m.plugins["test1"] = &Plugin{Name: "test1"}
	m.plugins["test2"] = &Plugin{Name: "test2"}

	plugins := m.ListPlugins()
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(plugins))
	}
}

func TestPluginManager_EnableDisable(t *testing.T) {
	dir := t.TempDir()
	m := NewPluginManager(dir, true)

	m.plugins["test"] = &Plugin{Name: "test", Enabled: true}

	m.Disable("test")
	if m.GetPlugin("test").Enabled {
		t.Error("expected disabled")
	}

	m.Enable("test")
	if !m.GetPlugin("test").Enabled {
		t.Error("expected enabled")
	}
}

func TestPluginManager_Uninstall(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	pluginPath := filepath.Join(pluginDir, "test-plugin")
	os.MkdirAll(pluginPath, 0755)

	m := NewPluginManager(pluginDir, true)
	m.plugins["test-plugin"] = &Plugin{Name: "test-plugin", Path: pluginPath}

	err := m.Uninstall("test-plugin")
	if err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	if m.GetPlugin("test-plugin") != nil {
		t.Error("expected plugin to be removed")
	}
}

func TestFormatPluginList(t *testing.T) {
	plugins := []*Plugin{
		{Name: "test", Version: "1.0.0", Description: "A test plugin", Enabled: true},
	}

	output := FormatPluginList(plugins)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatPluginList_Empty(t *testing.T) {
	output := FormatPluginList(nil)
	if output != "No plugins loaded." {
		t.Errorf("expected 'No plugins loaded.', got %q", output)
	}
}
