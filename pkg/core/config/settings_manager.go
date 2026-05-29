// Package config provides settings management aligned to pi's settings-manager.ts.
//
// Settings exist in two scopes: Global (user-wide) and Project (per-project).
// Project settings override Global settings. The merged view is accessible via Merged().
// Settings are persisted as JSON files on disk.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Scope determines where a setting is stored.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
)

// CompactionSettings holds compaction-specific settings.
// Aligned to TS CompactionSettings.
type CompactionSettings struct {
	Enabled       bool `json:"enabled,omitempty"`
	ReserveTokens int  `json:"reserveTokens,omitempty"`
	KeepRecentTokens int `json:"keepRecentTokens,omitempty"`
}

// BranchSummarySettings holds branch summary settings.
type BranchSummarySettings struct {
	ReserveTokens int  `json:"reserveTokens,omitempty"`
	SkipPrompt    bool `json:"skipPrompt,omitempty"`
}

// RetrySettings holds retry-specific settings.
type RetrySettings struct {
	Enabled     bool   `json:"enabled,omitempty"`
	MaxRetries  int    `json:"maxRetries,omitempty"`
	BaseDelayMs int    `json:"baseDelayMs,omitempty"`
	Provider    string `json:"provider,omitempty"`
}

// TerminalSettings holds terminal display settings.
type TerminalSettings struct {
	ShowImages            bool `json:"showImages,omitempty"`
	ImageWidthCells       int  `json:"imageWidthCells,omitempty"`
	ClearOnShrink         bool `json:"clearOnShrink,omitempty"`
	ShowTerminalProgress  bool `json:"showTerminalProgress,omitempty"`
}

// ImageSettings holds image handling settings.
type ImageSettings struct {
	AutoResize   bool `json:"autoResize,omitempty"`
	BlockImages  bool `json:"blockImages,omitempty"`
}

// ThinkingBudgetsSettings holds token budget per thinking level.
type ThinkingBudgetsSettings struct {
	Minimal int `json:"minimal,omitempty"`
	Low     int `json:"low,omitempty"`
	Medium  int `json:"medium,omitempty"`
	High    int `json:"high,omitempty"`
}

// Settings holds all configurable options for the coding agent.
// Aligned to TS Settings interface with the most impactful fields.
type Settings struct {
	// Model configuration
	Model         string `json:"model,omitempty"`
	MaxTokens     int    `json:"maxTokens,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`

	// API configuration
	APIKey   string `json:"apiKey,omitempty"`
	BaseURL  string `json:"baseUrl,omitempty"`
	Provider string `json:"provider,omitempty"` // "anthropic" | "bedrock" | "vertex"

	// Agent behavior
	MaxTurns        int    `json:"maxTurns,omitempty"`
	PermissionMode  string `json:"permissionMode,omitempty"` // "ask" | "auto" | "bypass" | "plan"
	CompactAfter    int    `json:"compactAfter,omitempty"`
	AutoCompact     bool   `json:"autoCompact,omitempty"`
	EnableStreaming  bool   `json:"enableStreaming,omitempty"`

	// Tool configuration
	EnabledTools  []string `json:"enabledTools,omitempty"`
	DisabledTools []string `json:"disabledTools,omitempty"`

	// Shell / execution
	Shell         string `json:"shell,omitempty"`
	CommandTimeout int   `json:"commandTimeout,omitempty"`

	// Output
	Verbose       bool   `json:"verbose,omitempty"`
	StreamOutput  bool   `json:"streamOutput,omitempty"`

	// Project-specific
	AppendSystemPrompt string   `json:"appendSystemPrompt,omitempty"`
	PromptGuidelines   []string `json:"promptGuidelines,omitempty"`

	// Context files to load (e.g. CLAUDE.md)
	ContextFiles []string `json:"contextFiles,omitempty"`

	// Custom system prompt (replaces default entirely)
	CustomSystemPrompt string `json:"customSystemPrompt,omitempty"`

	// Sub-struct settings
	Compaction       CompactionSettings       `json:"compaction,omitempty"`
	BranchSummary    BranchSummarySettings    `json:"branchSummary,omitempty"`
	Retry            RetrySettings            `json:"retry,omitempty"`
	Terminal         TerminalSettings         `json:"terminal,omitempty"`
	Image            ImageSettings            `json:"image,omitempty"`
	ThinkingBudgets  ThinkingBudgetsSettings  `json:"thinkingBudgets,omitempty"`

	// Misc
	TelemetryEnabled *bool `json:"telemetryEnabled,omitempty"`
	HTTPIdleTimeoutMs *int `json:"httpIdleTimeoutMs,omitempty"`
}

// SettingsManager manages global and project-scoped settings.
type SettingsManager struct {
	mu      sync.RWMutex
	global  *Settings
	project *Settings
	// Paths
	globalDir  string // ~/.miniclaude/
	projectDir string // .miniclaude/ in project root
}

// NewSettingsManager creates a settings manager.
// globalDir defaults to ~/.miniclaude/; projectDir defaults to <cwd>/.miniclaude/.
func NewSettingsManager(globalDir, projectDir string) *SettingsManager {
	if globalDir == "" {
		home, _ := os.UserHomeDir()
		globalDir = filepath.Join(home, ".miniclaude")
	}
	if projectDir == "" {
		cwd, _ := os.Getwd()
		projectDir = filepath.Join(cwd, ".miniclaude")
	}
	return &SettingsManager{
		global:     &Settings{},
		project:    &Settings{},
		globalDir:  globalDir,
		projectDir: projectDir,
	}
}

// Load reads settings from disk for both scopes.
func (sm *SettingsManager) Load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if err := sm.loadScope(sm.globalPath(), &sm.global); err != nil {
		return fmt.Errorf("load global settings: %w", err)
	}
	if err := sm.loadScope(sm.projectPath(), &sm.project); err != nil {
		// Project settings are optional — don't fail if missing
		sm.project = &Settings{}
	}
	return nil
}

// Save persists the given scope to disk.
func (sm *SettingsManager) Save(scope Scope) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var s *Settings
	var path string
	switch scope {
	case ScopeGlobal:
		s = sm.global
		path = sm.globalPath()
	case ScopeProject:
		s = sm.project
		path = sm.projectPath()
	default:
		return fmt.Errorf("unknown scope: %s", scope)
	}
	return sm.saveScope(path, s)
}

// Merged returns the effective settings with project overriding global.
func (sm *SettingsManager) Merged() Settings {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return mergeSettings(sm.global, sm.project)
}

// GetGlobal returns a copy of global settings.
func (sm *SettingsManager) GetGlobal() Settings {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return *sm.global
}

// GetProject returns a copy of project settings.
func (sm *SettingsManager) GetProject() Settings {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return *sm.project
}

// SetGlobal sets a global setting value.
func (sm *SettingsManager) SetGlobal(s Settings) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.global = &s
}

// SetProject sets a project setting value.
func (sm *SettingsManager) SetProject(s Settings) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.project = &s
}

// UpdateGlobal applies partial updates to global settings.
func (sm *SettingsManager) UpdateGlobal(fn func(s *Settings)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	fn(sm.global)
}

// UpdateProject applies partial updates to project settings.
func (sm *SettingsManager) UpdateProject(fn func(s *Settings)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	fn(sm.project)
}

// GlobalDir returns the global settings directory path.
func (sm *SettingsManager) GlobalDir() string { return sm.globalDir }

// ProjectDir returns the project settings directory path.
func (sm *SettingsManager) ProjectDir() string { return sm.projectDir }

// --- Internal ---

func (sm *SettingsManager) globalPath() string {
	return filepath.Join(sm.globalDir, "settings.json")
}

func (sm *SettingsManager) projectPath() string {
	return filepath.Join(sm.projectDir, "settings.json")
}

func (sm *SettingsManager) loadScope(path string, target **Settings) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	*target = &s
	return nil
}

func (sm *SettingsManager) saveScope(path string, s *Settings) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// mergeSettings overlays project settings on top of global settings.
// Non-zero values in project take precedence over global.
func mergeSettings(global, project *Settings) Settings {
	result := *global // start with global

	// Overlay project non-zero values
	if project.Model != "" {
		result.Model = project.Model
	}
	if project.MaxTokens != 0 {
		result.MaxTokens = project.MaxTokens
	}
	if project.Temperature != nil {
		result.Temperature = project.Temperature
	}
	if project.APIKey != "" {
		result.APIKey = project.APIKey
	}
	if project.BaseURL != "" {
		result.BaseURL = project.BaseURL
	}
	if project.Provider != "" {
		result.Provider = project.Provider
	}
	if project.MaxTurns != 0 {
		result.MaxTurns = project.MaxTurns
	}
	if project.PermissionMode != "" {
		result.PermissionMode = project.PermissionMode
	}
	if project.CompactAfter != 0 {
		result.CompactAfter = project.CompactAfter
	}
	// Bool: project explicitly set true overrides
	if project.AutoCompact {
		result.AutoCompact = project.AutoCompact
	}
	if project.EnableStreaming {
		result.EnableStreaming = project.EnableStreaming
	}
	if len(project.EnabledTools) > 0 {
		result.EnabledTools = project.EnabledTools
	}
	if len(project.DisabledTools) > 0 {
		result.DisabledTools = project.DisabledTools
	}
	if project.Shell != "" {
		result.Shell = project.Shell
	}
	if project.CommandTimeout != 0 {
		result.CommandTimeout = project.CommandTimeout
	}
	if project.Verbose {
		result.Verbose = project.Verbose
	}
	if project.StreamOutput {
		result.StreamOutput = project.StreamOutput
	}
	if project.AppendSystemPrompt != "" {
		result.AppendSystemPrompt = project.AppendSystemPrompt
	}
	if len(project.PromptGuidelines) > 0 {
		result.PromptGuidelines = project.PromptGuidelines
	}
	if len(project.ContextFiles) > 0 {
		result.ContextFiles = project.ContextFiles
	}
	if project.CustomSystemPrompt != "" {
		result.CustomSystemPrompt = project.CustomSystemPrompt
	}
	// Sub-struct settings: merge field by field
	if project.Compaction.Enabled {
		result.Compaction.Enabled = project.Compaction.Enabled
	}
	if project.Compaction.ReserveTokens != 0 {
		result.Compaction.ReserveTokens = project.Compaction.ReserveTokens
	}
	if project.Compaction.KeepRecentTokens != 0 {
		result.Compaction.KeepRecentTokens = project.Compaction.KeepRecentTokens
	}
	if project.BranchSummary.ReserveTokens != 0 {
		result.BranchSummary.ReserveTokens = project.BranchSummary.ReserveTokens
	}
	if project.BranchSummary.SkipPrompt {
		result.BranchSummary.SkipPrompt = project.BranchSummary.SkipPrompt
	}
	if project.Retry.Enabled {
		result.Retry.Enabled = project.Retry.Enabled
	}
	if project.Retry.MaxRetries != 0 {
		result.Retry.MaxRetries = project.Retry.MaxRetries
	}
	if project.Retry.BaseDelayMs != 0 {
		result.Retry.BaseDelayMs = project.Retry.BaseDelayMs
	}
	if project.Retry.Provider != "" {
		result.Retry.Provider = project.Retry.Provider
	}
	if project.Terminal.ShowImages {
		result.Terminal.ShowImages = project.Terminal.ShowImages
	}
	if project.Terminal.ImageWidthCells != 0 {
		result.Terminal.ImageWidthCells = project.Terminal.ImageWidthCells
	}
	if project.Terminal.ClearOnShrink {
		result.Terminal.ClearOnShrink = project.Terminal.ClearOnShrink
	}
	if project.Terminal.ShowTerminalProgress {
		result.Terminal.ShowTerminalProgress = project.Terminal.ShowTerminalProgress
	}
	if project.Image.AutoResize {
		result.Image.AutoResize = project.Image.AutoResize
	}
	if project.Image.BlockImages {
		result.Image.BlockImages = project.Image.BlockImages
	}
	if project.ThinkingBudgets.Minimal != 0 {
		result.ThinkingBudgets.Minimal = project.ThinkingBudgets.Minimal
	}
	if project.ThinkingBudgets.Low != 0 {
		result.ThinkingBudgets.Low = project.ThinkingBudgets.Low
	}
	if project.ThinkingBudgets.Medium != 0 {
		result.ThinkingBudgets.Medium = project.ThinkingBudgets.Medium
	}
	if project.ThinkingBudgets.High != 0 {
		result.ThinkingBudgets.High = project.ThinkingBudgets.High
	}
	if project.HTTPIdleTimeoutMs != nil {
		result.HTTPIdleTimeoutMs = project.HTTPIdleTimeoutMs
	}

	// Misc
	if project.TelemetryEnabled != nil {
		result.TelemetryEnabled = project.TelemetryEnabled
	}

	return result
}
