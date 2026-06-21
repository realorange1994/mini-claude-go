package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ─── Config Variable Substitution (MiMo-Code 1) ────────────────────────────
//
// Allows config values to reference environment variables via {env:VAR}
// and file contents via {file:path}. Variables are expanded at parse time.
//
// MiMo-Code source: config/variable.ts (33-90 lines)

var (
	envVarPattern  = regexp.MustCompile(`\{env:([^}]+)\}`)
	fileVarPattern = regexp.MustCompile(`{file:([^}]+)}`)
)

// ExpandConfigVariables expands {env:VAR} and {file:path} in config values.
func ExpandConfigVariables(value string, configDir string) string {
	// Expand {env:VAR}
	value = envVarPattern.ReplaceAllStringFunc(value, func(match string) string {
		varName := strings.TrimPrefix(strings.TrimSuffix(match, "}"), "{env:")
		if envVal := os.Getenv(varName); envVal != "" {
			return envVal
		}
		return match // Keep original if env var not found
	})

	// Expand {file:path}
	value = fileVarPattern.ReplaceAllStringFunc(value, func(match string) string {
		filePath := strings.TrimPrefix(strings.TrimSuffix(match, "}"), "{file:")

		// Expand ~ to home directory
		if strings.HasPrefix(filePath, "~") {
			homeDir, _ := os.UserHomeDir()
			if homeDir != "" {
				filePath = filepath.Join(homeDir, filePath[1:])
			}
		}

		// Resolve relative paths from config directory
		if !filepath.IsAbs(filePath) && configDir != "" {
			filePath = filepath.Join(configDir, filePath)
		}

		// Read file content
		data, err := os.ReadFile(filePath)
		if err != nil {
			return match // Keep original if file not found
		}

		// Skip commented-out lines
		lines := strings.Split(string(data), "\n")
		var result []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "//") {
				result = append(result, line)
			}
		}

		return strings.Join(result, "\n")
	})

	return value
}

// ExpandConfigMap expands variables in all string values of a config map.
func ExpandConfigMap(config map[string]any, configDir string) map[string]any {
	result := make(map[string]any)
	for k, v := range config {
		switch val := v.(type) {
		case string:
			result[k] = ExpandConfigVariables(val, configDir)
		case map[string]any:
			result[k] = ExpandConfigMap(val, configDir)
		default:
			result[k] = v
		}
	}
	return result
}
