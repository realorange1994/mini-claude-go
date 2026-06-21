package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// ─── Plugin System (MiMo-Code 4) ───────────────────────────────────────────
//
// Extensible plugin system with hooks and matcher aggregation.
// Enables third-party extensibility without modifying core code.
//
// MiMo-Code source: plugin/index.ts (581 lines)

// PluginHookPoint represents a hook point in the system.
type PluginHookPoint string

const (
	PluginHookPreStop   PluginHookPoint = "preStop"
	PluginHookPostStop  PluginHookPoint = "postStop"
	PluginHookConfig    PluginHookPoint = "config"
	PluginHookEvent     PluginHookPoint = "event"
	PluginHookPreTool   PluginHookPoint = "preTool"
	PluginHookPostTool  PluginHookPoint = "postTool"
)

// Plugin represents a loaded plugin.
type Plugin struct {
	Name        string                      `json:"name"`
	Version     string                      `json:"version"`
	Description string                      `json:"description"`
	Path        string                      `json:"path"`
	Enabled     bool                        `json:"enabled"`
	Config      map[string]any              `json:"config"`
	Hooks       map[PluginHookPoint]HookFunc `json:"-"`
}

// HookFunc is a function that handles a hook.
type HookFunc func(ctx *PluginHookContext) (*PluginHookResult, error)

// PluginHookContext provides context to hook functions.
type PluginHookContext struct {
	Hook      PluginHookPoint
	Plugin    *Plugin
	SessionID string
	AgentID   string
	Data      map[string]any
}

// PluginHookResult represents the result of a hook execution.
type PluginHookResult struct {
	Continue bool         `json:"continue"`
	Reason   string       `json:"reason,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// PluginManager manages plugins.
type PluginManager struct {
	mu          sync.Mutex
	plugins     map[string]*Plugin
	pluginDir   string
	hooks       map[PluginHookPoint][]*Plugin
	enabled     bool
}

// NewPluginManager creates a new plugin manager.
func NewPluginManager(pluginDir string, enabled bool) *PluginManager {
	return &PluginManager{
		plugins:   make(map[string]*Plugin),
		pluginDir: pluginDir,
		hooks:     make(map[PluginHookPoint][]*Plugin),
		enabled:   enabled,
	}
}

// Load loads plugins from the plugin directory.
func (m *PluginManager) Load() error {
	if !m.enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pluginDir == "" {
		return nil
	}

	entries, err := os.ReadDir(m.pluginDir)
	if err != nil {
		return nil // No plugin directory
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginPath := filepath.Join(m.pluginDir, entry.Name())
		manifestPath := filepath.Join(pluginPath, "plugin.json")

		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		plugin, err := m.loadPlugin(manifestPath, pluginPath)
		if err != nil {
			continue
		}

		m.plugins[plugin.Name] = plugin
	}

	return nil
}

// loadPlugin loads a plugin from a manifest file.
func (m *PluginManager) loadPlugin(manifestPath, pluginPath string) (*Plugin, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	var plugin Plugin
	if err := json.Unmarshal(data, &plugin); err != nil {
		return nil, err
	}

	plugin.Path = pluginPath
	plugin.Enabled = true
	plugin.Hooks = make(map[PluginHookPoint]HookFunc)

	return &plugin, nil
}

// Register registers a plugin hook.
func (m *PluginManager) Register(pluginName string, hook PluginHookPoint, fn HookFunc) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[pluginName]
	if !exists {
		return fmt.Errorf("plugin not found: %s", pluginName)
	}

	plugin.Hooks[hook] = fn
	m.hooks[hook] = append(m.hooks[hook], plugin)

	return nil
}

// Execute executes all hooks for a given hook point.
func (m *PluginManager) Execute(hook PluginHookPoint, ctx *PluginHookContext) ([]*PluginHookResult, error) {
	if !m.enabled {
		return nil, nil
	}

	m.mu.Lock()
	plugins := m.hooks[hook]
	m.mu.Unlock()

	var results []*PluginHookResult
	for _, plugin := range plugins {
		if !plugin.Enabled {
			continue
		}

		fn, exists := plugin.Hooks[hook]
		if !exists {
			continue
		}

		ctx.Plugin = plugin
		result, err := fn(ctx)
		if err != nil {
			continue
		}

		if result != nil {
			results = append(results, result)
		}
	}

	return results, nil
}

// ExecuteQuiet executes hooks without returning results.
func (m *PluginManager) ExecuteQuiet(hook PluginHookPoint, ctx *PluginHookContext) {
	m.Execute(hook, ctx)
}

// GetPlugin returns a plugin by name.
func (m *PluginManager) GetPlugin(name string) *Plugin {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.plugins[name]
}

// ListPlugins returns all loaded plugins.
func (m *PluginManager) ListPlugins() []*Plugin {
	m.mu.Lock()
	defer m.mu.Unlock()

	var plugins []*Plugin
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// Enable enables a plugin.
func (m *PluginManager) Enable(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin not found: %s", name)
	}

	plugin.Enabled = true
	return nil
}

// Disable disables a plugin.
func (m *PluginManager) Disable(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin not found: %s", name)
	}

	plugin.Enabled = false
	return nil
}

// Install installs a plugin from a URL or path.
func (m *PluginManager) Install(source string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if it's a local path
	if _, err := os.Stat(source); err == nil {
		return m.installLocal(source)
	}

	// Assume it's a git URL
	return m.installGit(source)
}

// installLocal installs a plugin from a local directory.
func (m *PluginManager) installLocal(source string) error {
	manifestPath := filepath.Join(source, "plugin.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return fmt.Errorf("no plugin.json found in %s", source)
	}

	// Copy to plugin directory
	dest := filepath.Join(m.pluginDir, filepath.Base(source))
	if err := copyDir(source, dest); err != nil {
		return fmt.Errorf("copy plugin: %w", err)
	}

	return nil
}

// installGit installs a plugin from a git repository.
func (m *PluginManager) installGit(url string) error {
	// Extract repo name from URL
	parts := strings.Split(url, "/")
	repoName := strings.TrimSuffix(parts[len(parts)-1], ".git")

	dest := filepath.Join(m.pluginDir, repoName)

	cmd := exec.Command("git", "clone", url, dest)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %s: %w", string(output), err)
	}

	return nil
}

// Uninstall removes a plugin.
func (m *PluginManager) Uninstall(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin not found: %s", name)
	}

	// Remove plugin directory
	if err := os.RemoveAll(plugin.Path); err != nil {
		return fmt.Errorf("remove plugin: %w", err)
	}

	delete(m.plugins, name)
	return nil
}

// IsEnabled returns true if the plugin system is enabled.
func (m *PluginManager) IsEnabled() bool {
	return m.enabled
}

// FormatPluginList formats a list of plugins for display.
func FormatPluginList(plugins []*Plugin) string {
	if len(plugins) == 0 {
		return "No plugins loaded."
	}

	var sb string
	sb += fmt.Sprintf("## Plugins (%d loaded)\n\n", len(plugins))

	for _, p := range plugins {
		status := "✓"
		if !p.Enabled {
			status = "✗"
		}
		sb += fmt.Sprintf("- [%s] **%s** v%s: %s\n", status, p.Name, p.Version, p.Description)
	}

	return sb
}

// copyDir copies a directory recursively.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(src, path)
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(destPath, data, info.Mode())
	})
}
