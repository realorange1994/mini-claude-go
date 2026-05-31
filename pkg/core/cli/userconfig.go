// Package cli provides the CLI entry point for miniClaude Code.
// Handles config loading, argument parsing, session creation, REPL loop, and slash commands.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// UserConfig mirrors the ~/.miniclaude/config.json structure.
type UserConfig struct {
	APIKey      string `json:"api_key,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	Model       string `json:"model,omitempty"`
	MaxTokens   int    `json:"max_tokens,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
	Stream      bool   `json:"stream,omitempty"`
	AutoCompact bool   `json:"auto_compact,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
	ProjectDir  string `json:"project_dir,omitempty"`
	Editor      string `json:"editor,omitempty"`
	HistoryFile string `json:"history_file,omitempty"`
}

// ReadUserConfig loads config from the standard locations:
// 1. ~/.miniclaude/config.json (global)
// 2. .miniclaude.json in the project directory (project-local, overrides global)
func ReadUserConfig(projectDir string) *UserConfig {
	cfg := &UserConfig{}

	// Global config: ~/.miniclaude/config.json
	home, _ := os.UserHomeDir()
	if home != "" {
		mergeConfigFile(filepath.Join(home, ".miniclaude", "config.json"), cfg)
	}

	// Project-local config: .miniclaude.json in project dir
	if projectDir != "" {
		mergeConfigFile(filepath.Join(projectDir, ".miniclaude.json"), cfg)
	}

	return cfg
}

// mergeConfigFile reads a JSON config file and merges non-zero fields into cfg.
func mergeConfigFile(path string, cfg *UserConfig) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var overlay UserConfig
	if err := json.Unmarshal(data, &overlay); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Invalid config file %s: %v\n", path, err)
		return
	}
	// Merge: overlay non-zero fields
	if overlay.APIKey != "" {
		cfg.APIKey = overlay.APIKey
	}
	if overlay.BaseURL != "" {
		cfg.BaseURL = overlay.BaseURL
	}
	if overlay.Model != "" {
		cfg.Model = overlay.Model
	}
	if overlay.MaxTokens != 0 {
		cfg.MaxTokens = overlay.MaxTokens
	}
	if overlay.Timeout != "" {
		cfg.Timeout = overlay.Timeout
	}
	if overlay.Stream {
		cfg.Stream = overlay.Stream
	}
	if overlay.AutoCompact {
		cfg.AutoCompact = overlay.AutoCompact
	}
	if overlay.Prompt != "" {
		cfg.Prompt = overlay.Prompt
	}
	if overlay.ProjectDir != "" {
		cfg.ProjectDir = overlay.ProjectDir
	}
	if overlay.Editor != "" {
		cfg.Editor = overlay.Editor
	}
	if overlay.HistoryFile != "" {
		cfg.HistoryFile = overlay.HistoryFile
	}
}
