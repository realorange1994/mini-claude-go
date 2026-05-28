package config

import (
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the agent
type Config struct {
	// Model settings
	Model       string                  `yaml:"model"`
	APIKey      string                  `yaml:"apiKey"`
	BaseURL     string                  `yaml:"baseURL"`
	MaxTokens   int                     `yaml:"maxTokens"`
	Temperature float64                 `yaml:"temperature"`

	// Session settings
	SessionPath  string `yaml:"sessionPath"`
	MaxTurns     int    `yaml:"maxTurns"`
	AutoCompact  bool   `yaml:"autoCompact"`
	CompactAfter int    `yaml:"compactAfter"`

	// Tool settings
	AllowedTools []string `yaml:"allowedTools"`
	BlockedTools []string `yaml:"blockedTools"`
	BashTimeout  int      `yaml:"bashTimeout"` // seconds

	// Permissions
	DefaultPermission string `yaml:"defaultPermission"` // "allow", "deny", "ask"

	// Extensions
	Extensions ExtensionsConfig `yaml:"extensions"`

	// Custom settings
	Custom map[string]interface{} `yaml:"custom"`
}

// ExtensionsConfig holds extension configuration
type ExtensionsConfig struct {
	Enabled    []string                   `yaml:"enabled"`
	Extensions map[string]ExtensionOpts   `yaml:"extensions"`
}

// ExtensionOpts holds per-extension options
type ExtensionOpts struct {
	Enabled  bool                  `yaml:"enabled"`
	Priority int                   `yaml:"priority"`
	Options  map[string]interface{} `yaml:"options"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Model:             "MiniMax-M2.7",
		MaxTokens:         200000,
		Temperature:       1.0,
		SessionPath:       filepath.Join(home, ".miniclaude", "sessions"),
		MaxTurns:          100,
		AutoCompact:       false,
		CompactAfter:      50,
		BashTimeout:       600,
		DefaultPermission: "ask",
		Custom:            make(map[string]interface{}),
		Extensions: ExtensionsConfig{
			Enabled:    []string{},
			Extensions: make(map[string]ExtensionOpts),
		},
	}
}

// LoadFromFile loads config from a YAML file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadFromDir loads config from .claude/config.yaml in a directory
func LoadFromDir(dir string) (*Config, error) {
	configPath := filepath.Join(dir, ".claude", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		return DefaultConfig(), nil
	}
	return LoadFromFile(configPath)
}

// Load loads config from file, then applies env var overrides
func Load(configPath string) (*Config, error) {
	var cfg *Config
	var err error

	if configPath != "" {
		cfg, err = LoadFromFile(configPath)
	} else {
		cfg, err = LoadFromDir(".")
	}
	if err != nil {
		cfg = DefaultConfig()
	}

	// Apply env var overrides
	if v := os.Getenv("MINICLAUDE_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("MINICLAUDE_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("MINICLAUDE_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("MINICLAUDE_SESSION_PATH"); v != "" {
		cfg.SessionPath = v
	}
	if v := os.Getenv("MINICLAUDE_MAX_TURNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxTurns = n
		}
	}

	// Ensure API key from env if not set
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	return cfg, nil
}

// Save writes config to a YAML file
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// IsToolAllowed checks if a tool is in the allowed list
func (c *Config) IsToolAllowed(toolName string) bool {
	if len(c.BlockedTools) > 0 {
		for _, t := range c.BlockedTools {
			if t == toolName {
				return false
			}
		}
	}
	if len(c.AllowedTools) == 0 {
		return true
	}
	for _, t := range c.AllowedTools {
		if t == toolName {
			return true
		}
	}
	return false
}

// GetExtensionConfig returns config for a specific extension
func (c *Config) GetExtensionConfig(name string) map[string]interface{} {
	if ext, ok := c.Extensions.Extensions[name]; ok {
		return ext.Options
	}
	return make(map[string]interface{})
}

// IsExtensionEnabled checks if an extension is enabled
func (c *Config) IsExtensionEnabled(name string) bool {
	// Check global enabled list
	for _, e := range c.Extensions.Enabled {
		if e == name {
			return true
		}
	}
	// Check per-extension setting
	if ext, ok := c.Extensions.Extensions[name]; ok {
		return ext.Enabled
	}
	return false
}
