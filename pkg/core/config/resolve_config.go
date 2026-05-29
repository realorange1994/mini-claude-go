// Package config provides configuration value resolution.
// Supports shell commands (prefixed with "!"), environment variables, and literal values.
package config

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
)

var (
	// commandResultCache caches shell command results
	commandResultCache = &cache{
		data: make(map[string]*cacheEntry),
	}
	// Command timeout for shell execution
	commandTimeout = 10 * time.Second
)

type cache struct {
	mu   sync.RWMutex
	data map[string]*cacheEntry
}

type cacheEntry struct {
	value   string
	expires time.Time
}

// ResolveConfigValue resolves a config value that may be:
// - A shell command (prefixed with "!") - executes and caches result
// - An environment variable name - returns env var value or literal
// - A literal string - returns as-is
func ResolveConfigValue(config string) string {
	if config == "" {
		return ""
	}

	// If starts with "!", execute as shell command
	if len(config) > 0 && config[0] == '!' {
		return executeCommand(config)
	}

	// Otherwise check environment variable
	if envVal := os.Getenv(config); envVal != "" {
		return envVal
	}

	// Return as literal
	return config
}

// ResolveConfigValueUncached resolves without caching (for one-time use).
func ResolveConfigValueUncached(config string) string {
	if config == "" {
		return ""
	}

	if len(config) > 0 && config[0] == '!' {
		return executeCommandUncached(config)
	}

	if envVal := os.Getenv(config); envVal != "" {
		return envVal
	}

	return config
}

// ResolveConfigValueOrThrow resolves a config value, throwing an error if resolution fails.
func ResolveConfigValueOrThrow(config string, description string) (string, error) {
	if config == "" {
		return "", &ConfigError{Description: "empty config: " + description}
	}

	resolved := ResolveConfigValueUncached(config)
	if resolved != "" {
		return resolved, nil
	}

	// Check if it was a shell command that failed
	if len(config) > 0 && config[0] == '!' {
		return "", &ConfigError{Description: "failed to resolve " + description + " from shell command: " + config[1:]}
	}

	return "", &ConfigError{Description: "failed to resolve " + description}
}

// ResolveHeaders resolves all header values using the same resolution logic.
func ResolveHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}

	resolved := make(map[string]string)
	for key, value := range headers {
		resolvedValue := ResolveConfigValue(value)
		if resolvedValue != "" {
			resolved[key] = resolvedValue
		}
	}

	if len(resolved) == 0 {
		return nil
	}
	return resolved
}

// ResolveHeadersOrThrow resolves all header values, throwing on failure.
func ResolveHeadersOrThrow(headers map[string]string, description string) (map[string]string, error) {
	if headers == nil {
		return nil, nil
	}

	resolved := make(map[string]string)
	for key, value := range headers {
		resolvedValue, err := ResolveConfigValueOrThrow(value, description+" header \""+key+"\"")
		if err != nil {
			return nil, err
		}
		resolved[key] = resolvedValue
	}

	if len(resolved) == 0 {
		return nil, nil
	}
	return resolved, nil
}

// ClearConfigValueCache clears the shell command cache.
func ClearConfigValueCache() {
	commandResultCache.mu.Lock()
	defer commandResultCache.mu.Unlock()
	commandResultCache.data = make(map[string]*cacheEntry)
}

// CommandResultCache returns the current cache for testing.
func CommandResultCache() *cache {
	return commandResultCache
}

func executeCommand(cmdConfig string) string {
	commandResultCache.mu.RLock()
	if entry, ok := commandResultCache.data[cmdConfig]; ok {
		if time.Now().Before(entry.expires) {
			commandResultCache.mu.RUnlock()
			return entry.value
		}
	}
	commandResultCache.mu.RUnlock()

	// Execute and cache
	result := executeCommandUncached(cmdConfig)

	commandResultCache.mu.Lock()
	defer commandResultCache.mu.Unlock()
	commandResultCache.data[cmdConfig] = &cacheEntry{
		value:   result,
		expires: time.Now().Add(5 * time.Minute), // 5 min cache
	}

	return result
}

func executeCommandUncached(cmdConfig string) string {
	if len(cmdConfig) == 0 || cmdConfig[0] != '!' {
		return ""
	}

	command := cmdConfig[1:] // Remove leading "!"

	if runtime.GOOS == "windows" {
		return executeWithWindowsShell(command)
	}
	return executeWithDefaultShell(command)
}

func executeWithDefaultShell(command string) string {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return string(output)
}

func executeWithWindowsShell(command string) string {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	// Try PowerShell first, then fall back to cmd
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	output, err := cmd.Output()
	if err != nil {
		// Fall back to cmd
		cmd := exec.CommandContext(ctx, "cmd", "/C", command)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		cmd.Stdout = nil
		cmd.Stderr = nil
		cmd.Stdin = nil

		output, err = cmd.Output()
		if err != nil {
			return ""
		}
	}

	return string(output)
}

// ConfigError represents a configuration resolution error.
type ConfigError struct {
	Description string
}

func (e *ConfigError) Error() string {
	return e.Description
}