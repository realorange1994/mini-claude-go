package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ─── Managed Config (MiMo-Code 6) ──────────────────────────────────────────
//
// Supports system-level managed configuration via MDM policies.
// Enables enterprise IT administrators to enforce settings.
//
// MiMo-Code source: config/managed.ts (1-70 lines)

// ManagedConfig manages system-level configuration.
type ManagedConfig struct {
	mu       sync.Mutex
	config   map[string]any
	loaded   bool
}

// NewManagedConfig creates a new managed config.
func NewManagedConfig() *ManagedConfig {
	return &ManagedConfig{
		config: make(map[string]any),
	}
}

// Load loads managed configuration from system paths.
func (c *ManagedConfig) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loaded {
		return nil
	}

	// Get platform-specific config path
	configPath := c.getManagedConfigPath()
	if configPath == "" {
		c.loaded = true
		return nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		// No managed config is not an error
		c.loaded = true
		return nil
	}

	// Parse JSON
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	// Filter out metadata keys
	for k := range config {
		if strings.HasPrefix(k, "_") {
			delete(config, k)
		}
	}

	c.config = config
	c.loaded = true
	return nil
}

// Get returns a config value by key.
func (c *ManagedConfig) Get(key string) any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.config[key]
}

// GetAll returns all managed config values.
func (c *ManagedConfig) GetAll() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]any)
	for k, v := range c.config {
		result[k] = v
	}
	return result
}

// IsLoaded returns true if managed config has been loaded.
func (c *ManagedConfig) IsLoaded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loaded
}

// getManagedConfigPath returns the platform-specific managed config path.
func (c *ManagedConfig) getManagedConfigPath() string {
	switch runtime.GOOS {
	case "darwin":
		// macOS: MDM plist
		homeDir, _ := os.UserHomeDir()
		if homeDir != "" {
			path := filepath.Join(homeDir, "Library", "Managed Preferences", "ai.opencode.managed.plist")
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		return ""
	case "linux":
		// Linux: /etc/opencode
		path := "/etc/opencode/managed.json"
		if _, err := os.Stat(path); err == nil {
			return path
		}
		return ""
	case "windows":
		// Windows: %ProgramData%\opencode
		programData := os.Getenv("ProgramData")
		if programData != "" {
			path := filepath.Join(programData, "opencode", "managed.json")
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		return ""
	default:
		return ""
	}
}

// MergeWithConfig merges managed config into a user config.
func (c *ManagedConfig) MergeWithConfig(userConfig map[string]any) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make(map[string]any)
	for k, v := range userConfig {
		result[k] = v
	}

	// Managed config overrides user config
	for k, v := range c.config {
		result[k] = v
	}

	return result
}
