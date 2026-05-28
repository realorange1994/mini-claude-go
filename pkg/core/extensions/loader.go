package extensions

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// -------------------------------------------------------------------------
// Configuration types
// -------------------------------------------------------------------------

// ExtensionConfig holds per-extension configuration from YAML.
type ExtensionConfig struct {
	Enabled  bool                   `yaml:"enabled"`
	Priority int                    `yaml:"priority"`
	Options  map[string]interface{} `yaml:"options"`
}

// ExtensionsConfig holds the full extensions section of the settings YAML.
type ExtensionsConfig struct {
	Enabled    []string                      `yaml:"enabled"`
	Extensions map[string]ExtensionConfig     `yaml:"extensions"`
}

// DefaultExtensionsConfig returns an empty default configuration.
func DefaultExtensionsConfig() ExtensionsConfig {
	return ExtensionsConfig{
		Enabled:    []string{},
		Extensions: map[string]ExtensionConfig{},
	}
}

// -------------------------------------------------------------------------
// Loader
// -------------------------------------------------------------------------

// Loader discovers and loads extensions from configuration and optional
// filesystem directories.
//
// In Go, extensions are imported statically via their packages' init()
// functions calling extensions.Register(). The Loader's job is to read
// configuration, determine which registered extensions to activate, and
// feed them to the ExtensionRunner in the correct order.
//
// Mirrors pi's core/extensions/loader.ts.
type Loader struct {
	runner *ExtensionRunner
	config ExtensionsConfig
}

// NewLoader creates a new extension loader bound to the given runner.
func NewLoader(runner *ExtensionRunner, config ExtensionsConfig) *Loader {
	return &Loader{runner: runner, config: config}
}

// DiscoverAndLoad loads extensions from the configuration and optionally
// scans a local extensions directory for additional YAML manifests.
func (l *Loader) DiscoverAndLoad(extensionsDir string) error {
	// Load any per-directory YAML manifests first.
	if extensionsDir != "" {
		if err := l.loadFromDirectory(extensionsDir); err != nil {
			return fmt.Errorf("extensions directory: %w", err)
		}
	}

	// Then load the globally-enabled extensions from config.
	for _, name := range l.config.Enabled {
		if err := l.loadByName(name); err != nil {
			return fmt.Errorf("extension %q: %w", name, err)
		}
	}
	return nil
}

// LoadFromConfig loads extensions based on a fresh configuration.
// This replaces the current config and loads all enabled extensions.
func (l *Loader) LoadFromConfig(config ExtensionsConfig) error {
	l.config = config
	for _, name := range config.Enabled {
		if err := l.loadByName(name); err != nil {
			return fmt.Errorf("extension %q: %w", name, err)
		}
	}
	return nil
}

// LoadFromFile reads extensions configuration from a YAML file and
// loads the enabled extensions.
func (l *Loader) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read extensions config %s: %w", path, err)
	}

	var config ExtensionsConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse extensions config %s: %w", path, err)
	}

	// Merge with existing config: prefer file values, keep defaults for
	// extensions not mentioned in the file.
	if l.config.Extensions == nil {
		l.config.Extensions = map[string]ExtensionConfig{}
	}
	for name, cfg := range config.Extensions {
		l.config.Extensions[name] = cfg
	}
	if len(config.Enabled) > 0 {
		l.config.Enabled = config.Enabled
	}

	return l.LoadFromConfig(l.config)
}

// -------------------------------------------------------------------------
// Internal loading
// -------------------------------------------------------------------------

// loadByName creates an instance of a named extension from the global
// factory registry and registers it with the runner.
func (l *Loader) loadByName(name string) error {
	cfg, hasCfg := l.config.Extensions[name]

	// If the config entry exists and is explicitly disabled, skip it.
	if hasCfg && !cfg.Enabled {
		return nil
	}

	ext := Get(name)
	if ext == nil {
		return fmt.Errorf("no factory registered for extension %q", name)
	}

	// Inject per-extension config into the context so the extension can
	// read its options during Register().
	if l.runner.ctx != nil {
		if hasCfg {
			l.runner.ctx.Config = cfg.Options
		} else {
			l.runner.ctx.Config = nil
		}
	}

	if err := l.runner.RegisterExtension(ext); err != nil {
		return err
	}
	return nil
}

// loadFromDirectory scans a directory for YAML files describing local
// extensions. Each YAML file is expected to be named <extension>.yaml
// and contain an ExtensionConfig structure.
func (l *Loader) loadFromDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // directory does not exist is not an error
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		name := filepath.Base(entry.Name())
		name = name[:len(name)-len(".yaml")]

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		var cfg ExtensionConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		// Default to enabled if the YAML doesn't explicitly set it.
		if l.config.Extensions == nil {
			l.config.Extensions = map[string]ExtensionConfig{}
		}
		l.config.Extensions[name] = cfg

		// Auto-add to the enabled list unless explicitly disabled.
		if cfg.Enabled {
			found := false
			for _, e := range l.config.Enabled {
				if e == name {
					found = true
					break
				}
			}
			if !found {
				l.config.Enabled = append(l.config.Enabled, name)
			}
		}
	}

	return nil
}