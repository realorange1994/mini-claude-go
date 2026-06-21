package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// ─── Auto-Formatter Service (MiMo-Code 3) ──────────────────────────────────
//
// Automatically detects and runs project formatters after file writes.
// Supports gofmt, prettier, ruff, biome, rustfmt, clang-format, shfmt, etc.
//
// MiMo-Code source: format/formatter.ts (403 lines), format/index.ts (203 lines)

// FormatterProfile represents a formatter configuration.
type FormatterProfile struct {
	Name       string
	Command    string
	Extensions []string
	ConfigFile string // config file to check for presence
	Args       []string
}

// FormatterService manages automatic formatting.
type FormatterService struct {
	mu        sync.Mutex
	enabled   bool
	profiles  []FormatterProfile
	available map[string]bool // formatter name -> available
}

// NewFormatterService creates a new formatter service.
func NewFormatterService(enabled bool) *FormatterService {
	s := &FormatterService{
		enabled:   enabled,
		available: make(map[string]bool),
	}
	s.registerDefaultProfiles()
	s.detectAvailable()
	return s
}

// registerDefaultProfiles registers default formatter profiles.
func (s *FormatterService) registerDefaultProfiles() {
	s.profiles = []FormatterProfile{
		{Name: "gofmt", Command: "gofmt", Extensions: []string{".go"}, Args: []string{"-w"}},
		{Name: "prettier", Command: "prettier", Extensions: []string{".js", ".ts", ".jsx", ".tsx", ".json", ".css", ".md"}, ConfigFile: "package.json", Args: []string{"--write"}},
		{Name: "ruff", Command: "ruff", Extensions: []string{".py"}, ConfigFile: "pyproject.toml", Args: []string{"format"}},
		{Name: "biome", Command: "biome", Extensions: []string{".js", ".ts", ".jsx", ".tsx"}, ConfigFile: "biome.json", Args: []string{"format", "--write"}},
		{Name: "rustfmt", Command: "rustfmt", Extensions: []string{".rs"}, Args: []string{}},
		{Name: "clang-format", Command: "clang-format", Extensions: []string{".c", ".cpp", ".h", ".hpp"}, Args: []string{"-i"}},
		{Name: "shfmt", Command: "shfmt", Extensions: []string{".sh", ".bash"}, Args: []string{"-w"}},
		{Name: "nixfmt", Command: "nixfmt", Extensions: []string{".nix"}, Args: []string{}},
	}
}

// detectAvailable checks which formatters are installed.
func (s *FormatterService) detectAvailable() {
	for _, p := range s.profiles {
		_, err := exec.LookPath(p.Command)
		s.available[p.Name] = err == nil
	}
}

// FormatFile formats a file using the appropriate formatter.
func (s *FormatterService) FormatFile(filePath string) error {
	if !s.enabled {
		return nil
	}

	ext := filepath.Ext(filePath)
	for _, p := range s.profiles {
		if s.matchesExtension(ext, p.Extensions) && s.available[p.Name] {
			// Check for config file if required
			if p.ConfigFile != "" && !s.hasConfigFile(filePath, p.ConfigFile) {
				continue
			}

			return s.runFormatter(p, filePath)
		}
	}

	return nil
}

// matchesExtension checks if a file extension matches the formatter.
func (s *FormatterService) matchesExtension(ext string, extensions []string) bool {
	for _, e := range extensions {
		if ext == e {
			return true
		}
	}
	return false
}

// hasConfigFile checks if a config file exists in the project.
func (s *FormatterService) hasConfigFile(filePath, configFile string) bool {
	dir := filepath.Dir(filePath)
	for {
		configPath := filepath.Join(dir, configFile)
		if _, err := os.Stat(configPath); err == nil {
			return true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}

// runFormatter runs a formatter on a file.
func (s *FormatterService) runFormatter(profile FormatterProfile, filePath string) error {
	args := append(profile.Args, filePath)
	cmd := exec.Command(profile.Command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("formatter %s failed: %s: %w", profile.Name, string(output), err)
	}
	return nil
}

// FormatFiles formats multiple files.
func (s *FormatterService) FormatFiles(filePaths []string) []error {
	var errors []error
	for _, path := range filePaths {
		if err := s.FormatFile(path); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

// IsEnabled returns true if the formatter service is enabled.
func (s *FormatterService) IsEnabled() bool {
	return s.enabled
}

// SetEnabled enables or disables the formatter service.
func (s *FormatterService) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
}

// GetAvailable returns a list of available formatters.
func (s *FormatterService) GetAvailable() []string {
	var available []string
	for name, ok := range s.available {
		if ok {
			available = append(available, name)
		}
	}
	return available
}

// AddProfile adds a custom formatter profile.
func (s *FormatterService) AddProfile(profile FormatterProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles = append(s.profiles, profile)
	_, err := exec.LookPath(profile.Command)
	s.available[profile.Name] = err == nil
}

// FormatStatus returns a status string for the formatter service.
func (s *FormatterService) FormatStatus() string {
	available := s.GetAvailable()
	if len(available) == 0 {
		return "No formatters available."
	}
	return fmt.Sprintf("Available formatters: %s", strings.Join(available, ", "))
}
