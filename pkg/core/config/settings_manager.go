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
	Enabled        bool   `json:"enabled,omitempty"`
	MaxRetries     int    `json:"maxRetries,omitempty"`
	BaseDelayMs    int    `json:"baseDelayMs,omitempty"`
	Provider       string `json:"provider,omitempty"`
	TimeoutMs      int    `json:"timeoutMs,omitempty"`
	MaxRetryDelayMs int   `json:"maxRetryDelayMs,omitempty"`
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

// MarkdownSettings holds markdown rendering settings.
type MarkdownSettings struct {
	CodeBlockIndent string `json:"codeBlockIndent,omitempty"` // default "  "
}

// WarningSettings holds warning configuration.
type WarningSettings struct {
	AnthropicExtraUsage bool `json:"anthropicExtraUsage,omitempty"` // default true
}

// PackageSource represents a package source (string or structured).
type PackageSource struct {
	Source     string   `json:"source"`
	Extensions []string `json:"extensions,omitempty"`
	Skills     []string `json:"skills,omitempty"`
	Prompts    []string `json:"prompts,omitempty"`
	Themes     []string `json:"themes,omitempty"`
}

// Settings holds all configurable options for the coding agent.
// Aligned to TS Settings interface with the most impactful fields.
type Settings struct {
	// Model configuration
	Model         string   `json:"model,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`

	// Default model/provider (TS: defaultProvider, defaultModel, defaultThinkingLevel)
	DefaultProvider      string `json:"defaultProvider,omitempty"`
	DefaultModel         string `json:"defaultModel,omitempty"`
	DefaultThinkingLevel string `json:"defaultThinkingLevel,omitempty"` // "off"|"minimal"|"low"|"medium"|"high"|"xhigh"

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
	SteeringMode    string `json:"steeringMode,omitempty"` // "all" | "one-at-a-time"
	FollowUpMode    string `json:"followUpMode,omitempty"` // "all" | "one-at-a-time"

	// Tool configuration
	EnabledTools  []string `json:"enabledTools,omitempty"`
	DisabledTools []string `json:"disabledTools,omitempty"`
	EnabledModels []string `json:"enabledModels,omitempty"` // glob patterns for model filtering

	// Shell / execution
	Shell            string `json:"shell,omitempty"`
	CommandTimeout   int    `json:"commandTimeout,omitempty"`
	ShellPath        string `json:"shellPath,omitempty"`
	ShellCommandPrefix string `json:"shellCommandPrefix,omitempty"`
	NpmCommand       []string `json:"npmCommand,omitempty"`

	// Transport
	Transport string `json:"transport,omitempty"` // "sse" | "websocket" | "websocket-cached" | "auto"

	// Output
	Verbose       bool   `json:"verbose,omitempty"`
	StreamOutput  bool   `json:"streamOutput,omitempty"`
	QuietStartup  bool   `json:"quietStartup,omitempty"`

	// UI / display
	Theme             string `json:"theme,omitempty"`
	HideThinkingBlock bool   `json:"hideThinkingBlock,omitempty"`
	DoubleEscapeAction string `json:"doubleEscapeAction,omitempty"` // "fork" | "tree" | "none"
	TreeFilterMode    string `json:"treeFilterMode,omitempty"`    // "default" | "no-tools" | "user-only" | "labeled-only" | "all"

	// Project-specific
	AppendSystemPrompt string   `json:"appendSystemPrompt,omitempty"`
	PromptGuidelines   []string `json:"promptGuidelines,omitempty"`

	// Context files to load (e.g. CLAUDE.md)
	ContextFiles []string `json:"contextFiles,omitempty"`

	// Custom system prompt (replaces default entirely)
	CustomSystemPrompt string `json:"customSystemPrompt,omitempty"`

	// Skills / extensions / packages
	Skills              []string      `json:"skills,omitempty"`
	Extensions          []string      `json:"extensions,omitempty"`
	Packages            []PackageSource `json:"packages,omitempty"`
	EnableSkillCommands bool          `json:"enableSkillCommands,omitempty"`

	// Markdown rendering
	Markdown MarkdownSettings `json:"markdown,omitempty"`

	// Warnings
	Warnings WarningSettings `json:"warnings,omitempty"`

	// Session
	SessionDir string `json:"sessionDir,omitempty"`

	// Sub-struct settings
	Compaction       CompactionSettings       `json:"compaction,omitempty"`
	BranchSummary    BranchSummarySettings    `json:"branchSummary,omitempty"`
	Retry            RetrySettings            `json:"retry,omitempty"`
	Terminal         TerminalSettings         `json:"terminal,omitempty"`
	Image            ImageSettings            `json:"image,omitempty"`
	ThinkingBudgets  ThinkingBudgetsSettings  `json:"thinkingBudgets,omitempty"`

	// Misc
	TelemetryEnabled       *bool `json:"telemetryEnabled,omitempty"`
	EnableInstallTelemetry *bool `json:"enableInstallTelemetry,omitempty"`
	HTTPIdleTimeoutMs      *int  `json:"httpIdleTimeoutMs,omitempty"`
	LastChangelogVersion   string `json:"lastChangelogVersion,omitempty"`
	CollapseChangelog      bool   `json:"collapseChangelog,omitempty"`
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
// After loading, applies migrations for backward compatibility.
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

	// Apply migrations to both scopes
	sm.migrateSettings(sm.global)
	sm.migrateSettings(sm.project)

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

// Reload re-reads settings from disk for both scopes.
func (sm *SettingsManager) Reload() error {
	return sm.Load()
}

// SaveAll persists both scopes to disk.
func (sm *SettingsManager) SaveAll() error {
	if err := sm.Save(ScopeGlobal); err != nil {
		return err
	}
	return sm.Save(ScopeProject)
}

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
	// New TS-aligned model defaults
	if project.DefaultProvider != "" {
		result.DefaultProvider = project.DefaultProvider
	}
	if project.DefaultModel != "" {
		result.DefaultModel = project.DefaultModel
	}
	if project.DefaultThinkingLevel != "" {
		result.DefaultThinkingLevel = project.DefaultThinkingLevel
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
	if project.Retry.TimeoutMs != 0 {
		result.Retry.TimeoutMs = project.Retry.TimeoutMs
	}
	if project.Retry.MaxRetryDelayMs != 0 {
		result.Retry.MaxRetryDelayMs = project.Retry.MaxRetryDelayMs
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
	if project.EnableInstallTelemetry != nil {
		result.EnableInstallTelemetry = project.EnableInstallTelemetry
	}

	// New TS-aligned fields
	if project.SteeringMode != "" {
		result.SteeringMode = project.SteeringMode
	}
	if project.FollowUpMode != "" {
		result.FollowUpMode = project.FollowUpMode
	}
	if project.Transport != "" {
		result.Transport = project.Transport
	}
	if project.ShellPath != "" {
		result.ShellPath = project.ShellPath
	}
	if project.ShellCommandPrefix != "" {
		result.ShellCommandPrefix = project.ShellCommandPrefix
	}
	if project.QuietStartup {
		result.QuietStartup = project.QuietStartup
	}
	if project.Theme != "" {
		result.Theme = project.Theme
	}
	if project.HideThinkingBlock {
		result.HideThinkingBlock = project.HideThinkingBlock
	}
	if project.DoubleEscapeAction != "" {
		result.DoubleEscapeAction = project.DoubleEscapeAction
	}
	if project.TreeFilterMode != "" {
		result.TreeFilterMode = project.TreeFilterMode
	}
	if project.Markdown.CodeBlockIndent != "" {
		result.Markdown.CodeBlockIndent = project.Markdown.CodeBlockIndent
	}
	if project.Warnings.AnthropicExtraUsage {
		result.Warnings.AnthropicExtraUsage = project.Warnings.AnthropicExtraUsage
	}
	if project.SessionDir != "" {
		result.SessionDir = project.SessionDir
	}
	if len(project.Skills) > 0 {
		result.Skills = project.Skills
	}
	if len(project.Extensions) > 0 {
		result.Extensions = project.Extensions
	}
	if project.EnableSkillCommands {
		result.EnableSkillCommands = project.EnableSkillCommands
	}
	if len(project.EnabledModels) > 0 {
		result.EnabledModels = project.EnabledModels
	}
	if len(project.Packages) > 0 {
		result.Packages = project.Packages
	}
	if len(project.NpmCommand) > 0 {
		result.NpmCommand = project.NpmCommand
	}
	if project.LastChangelogVersion != "" {
		result.LastChangelogVersion = project.LastChangelogVersion
	}
	if project.CollapseChangelog {
		result.CollapseChangelog = project.CollapseChangelog
	}

	return result
}

// migrateSettings applies backward-compatible migrations for settings.
// TS equivalents:
// - queueMode -> steeringMode (migration from legacy setting name)
// - websockets -> transport (migration from legacy setting name)
// - legacy skills object format -> string array
func (sm *SettingsManager) migrateSettings(s *Settings) {
	if s == nil {
		return
	}

	// Migration 1: queueMode -> steeringMode
	// If steeringMode is empty but we have legacy queueMode behavior,
	// default to "normal" (which is the TS default after migration)
	if s.SteeringMode == "" {
		s.SteeringMode = "normal"
	}

	// Migration 2: websockets -> transport
	// If transport is empty, default based on EnableStreaming
	if s.Transport == "" {
		if s.EnableStreaming {
			s.Transport = "stdio"
		}
	}

	// Migration 3: default FollowUpMode
	if s.FollowUpMode == "" {
		s.FollowUpMode = "off"
	}

	// Migration 4: default doubleEscapeAction
	if s.DoubleEscapeAction == "" {
		s.DoubleEscapeAction = "cancel"
	}

	// Migration 5: default treeFilterMode
	if s.TreeFilterMode == "" {
		s.TreeFilterMode = "all"
	}

	// Migration 6: default retry provider-level settings
	if s.Retry.TimeoutMs == 0 {
		s.Retry.TimeoutMs = 30000 // 30 seconds
	}
	if s.Retry.MaxRetryDelayMs == 0 {
		s.Retry.MaxRetryDelayMs = 60000 // 60 seconds
	}

	// Migration 7: default SteeringMode (TS: "one-at-a-time")
	if s.SteeringMode == "" || s.SteeringMode == "normal" {
		s.SteeringMode = "one-at-a-time"
	}
	if s.SteeringMode == "queue" {
		s.SteeringMode = "all" // old "queue" → new "all"
	}

	// Migration 8: default FollowUpMode (TS: "one-at-a-time")
	if s.FollowUpMode == "" || s.FollowUpMode == "off" {
		s.FollowUpMode = "one-at-a-time"
	}
	if s.FollowUpMode == "suggest" {
		s.FollowUpMode = "all"
	}
	if s.FollowUpMode == "auto" {
		s.FollowUpMode = "all"
	}

	// Migration 9: default DoubleEscapeAction (TS: "tree", was "cancel")
	if s.DoubleEscapeAction == "" || s.DoubleEscapeAction == "cancel" {
		s.DoubleEscapeAction = "tree"
	}

	// Migration 10: default TreeFilterMode (TS: "default", was "all")
	if s.TreeFilterMode == "" || s.TreeFilterMode == "all" {
		s.TreeFilterMode = "default"
	}

	// Migration 11: default Transport (TS: "auto")
	if s.Transport == "" {
		s.Transport = "auto"
	}
	if s.Transport == "stdio" {
		s.Transport = "sse" // TS uses "sse" not "stdio"
	}

	// Migration 12: default Compaction settings (TS: enabled=true, reserveTokens=16384, keepRecentTokens=20000)
	if !s.Compaction.Enabled {
		s.Compaction.Enabled = true
	}
	if s.Compaction.ReserveTokens == 0 {
		s.Compaction.ReserveTokens = 16384
	}
	if s.Compaction.KeepRecentTokens == 0 {
		s.Compaction.KeepRecentTokens = 20000
	}

	// Migration 13: default Retry settings (TS: enabled=true, maxRetries=3, baseDelayMs=2000)
	if !s.Retry.Enabled {
		s.Retry.Enabled = true
	}
	if s.Retry.MaxRetries == 0 {
		s.Retry.MaxRetries = 3
	}
	if s.Retry.BaseDelayMs == 0 {
		s.Retry.BaseDelayMs = 2000
	}

	// Migration 14: default EnableSkillCommands (TS: true)
	if !s.EnableSkillCommands {
		s.EnableSkillCommands = true
	}

	// Migration 15: default Warnings (TS: anthropicExtraUsage=true)
	if !s.Warnings.AnthropicExtraUsage {
		s.Warnings.AnthropicExtraUsage = true
	}

	// Migration 16: default Markdown codeBlockIndent (TS: "  ")
	if s.Markdown.CodeBlockIndent == "" {
		s.Markdown.CodeBlockIndent = "  "
	}

	// Migration 17: default EnableInstallTelemetry (TS: true)
	if s.EnableInstallTelemetry == nil {
		t := true
		s.EnableInstallTelemetry = &t
	}
}
