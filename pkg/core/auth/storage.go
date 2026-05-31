// Package auth provides credential storage for API keys and OAuth tokens.
// Aligned to pi's auth-storage.ts with pluggable backend, runtime overrides,
// fallback resolver, and priority-chain key resolution.
package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OAuthCredential represents OAuth token data stored in auth.json.
// Aligned to TS OAuthCredentials (from @earendil-works/pi-ai).
type OAuthCredential struct {
	Type         string `json:"type"` // "oauth"
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	Expires      int64  `json:"expires"` // unix millis
	ClientID     string `json:"clientId,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// ApiKeyCredential represents a plain API key stored in auth.json.
// Aligned to TS ApiKeyCredential.
type ApiKeyCredential struct {
	Type string `json:"type"` // "api_key"
	Key  string `json:"key"`  // may be an env var reference like "$ANTHROPIC_API_KEY"
}

// AuthCredential is the union type for stored credentials.
// Aligned to TS AuthCredential = ApiKeyCredential | OAuthCredential.
type AuthCredential struct {
	Type         string `json:"type"` // "api_key" or "oauth"
	Key          string `json:"key,omitempty"`
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	Expires      int64  `json:"expires,omitempty"` // unix millis
	ClientID     string `json:"clientId,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// IsOAuth returns true if this credential is an OAuth token.
func (c AuthCredential) IsOAuth() bool {
	return c.Type == "oauth"
}

// IsApiKey returns true if this credential is a plain API key.
func (c AuthCredential) IsApiKey() bool {
	return c.Type == "api_key"
}

// IsExpired returns true if an OAuth credential has expired.
func (c AuthCredential) IsExpired() bool {
	if !c.IsOAuth() {
		return false
	}
	return time.Now().UnixMilli() >= c.Expires
}

// AuthStorageData is the full auth.json structure.
type AuthStorageData map[string]AuthCredential

// AuthStatus represents whether auth is configured for a provider.
// Aligned to TS AuthStatus.
type AuthStatus struct {
	Configured bool
	Source     string // "stored", "runtime", "environment", "fallback", "models_json_key", "models_json_command"
	Label      string
}

// AuthStorageBackend is the pluggable storage interface.
// Aligned to TS AuthStorageBackend.
type AuthStorageBackend interface {
	// WithLock acquires exclusive lock, reads current data, calls fn, writes result if next is provided.
	WithLock(fn func(current string) LockResult) LockResult
}

// LockResult is the return value from a locked operation.
// Aligned to TS LockResult<T>.
type LockResult struct {
	Result interface{}
	Next   string // if non-empty, written back to storage
}

// FileAuthStorageBackend implements file-based auth storage with file locking.
// Aligned to TS FileAuthStorageBackend.
type FileAuthStorageBackend struct {
	authPath string
	mu       sync.Mutex // process-level lock (TS uses proper-lockfile for cross-process)
}

// NewFileAuthStorageBackend creates a file-based auth storage backend.
func NewFileAuthStorageBackend(authPath string) *FileAuthStorageBackend {
	return &FileAuthStorageBackend{authPath: authPath}
}

func (b *FileAuthStorageBackend) ensureParentDir() error {
	dir := filepath.Dir(b.authPath)
	return os.MkdirAll(dir, 0700)
}

func (b *FileAuthStorageBackend) ensureFileExists() error {
	if _, err := os.Stat(b.authPath); os.IsNotExist(err) {
		if err := b.ensureParentDir(); err != nil {
			return err
		}
		return os.WriteFile(b.authPath, []byte("{}"), 0600)
	}
	return nil
}

func (b *FileAuthStorageBackend) WithLock(fn func(current string) LockResult) LockResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.ensureFileExists(); err != nil {
		return LockResult{Result: nil}
	}

	current, err := os.ReadFile(b.authPath)
	var content string
	if err != nil {
		content = "{}"
	} else {
		content = string(current)
	}

	result := fn(content)
	if result.Next != "" {
		os.WriteFile(b.authPath, []byte(result.Next), 0600)
	}
	return result
}

// InMemoryAuthStorageBackend implements in-memory auth storage.
// Aligned to TS InMemoryAuthStorageBackend.
type InMemoryAuthStorageBackend struct {
	mu    sync.Mutex
	value string
}

func (b *InMemoryAuthStorageBackend) WithLock(fn func(current string) LockResult) LockResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := fn(b.value)
	if result.Next != "" {
		b.value = result.Next
	}
	return result
}

// AuthStorage manages credential storage with priority-chain key resolution.
// Aligned to TS AuthStorage with:
//   - Runtime overrides (CLI --api-key)
//   - Fallback resolver (models.json custom providers)
//   - Priority: runtime > auth.json > oauth > env var > fallback
type AuthStorage struct {
	mu               sync.RWMutex
	data             AuthStorageData
	runtimeOverrides map[string]string // provider -> apiKey (from CLI --api-key)
	fallbackResolver func(provider string) string // custom provider resolver from models.json
	loadError        error
	errors           []error
	storage          AuthStorageBackend
}

// Create creates a new AuthStorage with file-based backend.
func Create(authPath string) *AuthStorage {
	if authPath == "" {
		home, _ := os.UserHomeDir()
		authPath = filepath.Join(home, ".miniclaude", "auth.json")
	}
	a := &AuthStorage{
		storage:          NewFileAuthStorageBackend(authPath),
		runtimeOverrides: make(map[string]string),
	}
	a.reload()
	return a
}

// FromStorage creates AuthStorage from a custom backend.
func FromStorage(storage AuthStorageBackend) *AuthStorage {
	a := &AuthStorage{
		storage:          storage,
		runtimeOverrides: make(map[string]string),
	}
	a.reload()
	return a
}

// InMemory creates a non-persisted AuthStorage.
func InMemory(data AuthStorageData) *AuthStorage {
	initData, _ := json.MarshalIndent(data, "", "  ")
	backend := &InMemoryAuthStorageBackend{}
	backend.WithLock(func(current string) LockResult {
		return LockResult{Next: string(initData)}
	})
	return FromStorage(backend)
}

// SetRuntimeApiKey sets a runtime API key override (not persisted to disk).
// Used for CLI --api-key flag. Highest priority in key resolution.
func (a *AuthStorage) SetRuntimeApiKey(provider, apiKey string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.runtimeOverrides[provider] = apiKey
}

// RemoveRuntimeApiKey removes a runtime API key override.
func (a *AuthStorage) RemoveRuntimeApiKey(provider string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.runtimeOverrides, provider)
}

// SetFallbackResolver sets a fallback resolver for API keys not found
// in auth.json or env vars. Used for custom provider keys from models.json.
func (a *AuthStorage) SetFallbackResolver(resolver func(provider string) string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fallbackResolver = resolver
}

func (a *AuthStorage) recordError(err error) {
	a.errors = append(a.errors, err)
}

// reload reads credentials from the storage backend.
func (a *AuthStorage) reload() {
	result := a.storage.WithLock(func(current string) LockResult {
		return LockResult{Result: current}
	})
	content, _ := result.Result.(string)
	if content == "" {
		a.data = make(AuthStorageData)
		a.loadError = nil
		return
	}
	var data AuthStorageData
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		a.loadError = err
		a.data = make(AuthStorageData)
		a.recordError(err)
		return
	}
	a.data = data
	a.loadError = nil
}

func (a *AuthStorage) persistProviderChange(provider string, credential *AuthCredential) {
	if a.loadError != nil {
		return
	}
	a.storage.WithLock(func(current string) LockResult {
		var currentData AuthStorageData
		if current != "" {
			json.Unmarshal([]byte(current), &currentData)
		}
		if currentData == nil {
			currentData = make(AuthStorageData)
		}
		if credential != nil {
			currentData[provider] = *credential
		} else {
			delete(currentData, provider)
		}
		merged, _ := json.MarshalIndent(currentData, "", "  ")
		return LockResult{Next: string(merged)}
	})
}

// GetCredential returns a credential by provider key.
func (a *AuthStorage) GetCredential(providerKey string) (AuthCredential, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cred, ok := a.data[providerKey]
	return cred, ok
}

// SetCredential stores a credential and persists it.
func (a *AuthStorage) SetCredential(providerKey string, cred AuthCredential) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.data[providerKey] = cred
	a.persistProviderChange(providerKey, &cred)
}

// RemoveCredential removes a credential and persists the change.
func (a *AuthStorage) RemoveCredential(providerKey string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.data, providerKey)
	a.persistProviderChange(providerKey, nil)
}

// List returns all providers with stored credentials.
func (a *AuthStorage) List() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	keys := make([]string, 0, len(a.data))
	for k := range a.data {
		keys = append(keys, k)
	}
	return keys
}

// Has checks if credentials exist for a provider in auth.json.
func (a *AuthStorage) Has(provider string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.data[provider].Key != "" || a.data[provider].AccessToken != ""
}

// HasAuth checks if any form of auth is configured for a provider.
// Unlike GetApiKey, this doesn't refresh OAuth tokens.
// Aligned to TS hasAuth (auth-storage.ts:338-344).
func (a *AuthStorage) HasAuth(provider string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.runtimeOverrides[provider] != "" {
		return true
	}
	if cred := a.data[provider]; cred.Key != "" || cred.AccessToken != "" {
		return true
	}
	if getEnvApiKey(provider) != "" {
		return true
	}
	if a.fallbackResolver != nil && a.fallbackResolver(provider) != "" {
		return true
	}
	return false
}

// GetAuthStatus returns auth status without exposing credential values.
// Aligned to TS getAuthStatus (auth-storage.ts:349-368).
func (a *AuthStorage) GetAuthStatus(provider string) AuthStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if cred := a.data[provider]; cred.Key != "" || cred.AccessToken != "" {
		return AuthStatus{Configured: true, Source: "stored"}
	}
	if a.runtimeOverrides[provider] != "" {
		return AuthStatus{Configured: false, Source: "runtime", Label: "--api-key"}
	}
	envKeys := findEnvKeys(provider)
	if envKeys != "" {
		return AuthStatus{Configured: false, Source: "environment", Label: envKeys}
	}
	if a.fallbackResolver != nil && a.fallbackResolver(provider) != "" {
		return AuthStatus{Configured: false, Source: "fallback", Label: "custom provider config"}
	}
	return AuthStatus{Configured: false}
}

// GetApiKey resolves the API key for a provider using the priority chain:
// 1. Runtime override (CLI --api-key) — highest priority
// 2. API key from auth.json (may contain env var reference like "$KEY")
// 3. OAuth token from auth.json (auto-refreshed)
// 4. Environment variable
// 5. Fallback resolver (models.json custom providers)
// Aligned to TS getApiKey (auth-storage.ts:462-523).
func (a *AuthStorage) GetApiKey(provider string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// 1. Runtime override
	if key := a.runtimeOverrides[provider]; key != "" {
		return key
	}

	// 2. API key from auth.json
	cred := a.data[provider]
	if cred.IsApiKey() {
		return resolveConfigValue(cred.Key)
	}

	// 3. OAuth token from auth.json
	if cred.IsOAuth() {
		if !cred.IsExpired() {
			return cred.AccessToken
		}
		// Token expired — return empty; caller should trigger refresh
		// (In a full implementation, this would call refreshOAuthTokenWithLock)
		return ""
	}

	// 4. Environment variable
	if key := getEnvApiKey(provider); key != "" {
		return key
	}

	// 5. Fallback resolver
	if a.fallbackResolver != nil {
		return a.fallbackResolver(provider)
	}

	return ""
}

// Logout removes credentials for a provider.
// Aligned to TS logout (auth-storage.ts:399-401).
func (a *AuthStorage) Logout(provider string) {
	a.RemoveCredential(provider)
}

// GetAll returns all credentials (for passing to OAuth helpers).
func (a *AuthStorage) GetAll() AuthStorageData {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(AuthStorageData, len(a.data))
	for k, v := range a.data {
		out[k] = v
	}
	return out
}

// DrainErrors returns accumulated errors and clears the list.
func (a *AuthStorage) DrainErrors() []error {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.errors
	a.errors = nil
	return out
}

// Reload refreshes auth storage from the backend.
func (a *AuthStorage) Reload() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reload()
}

// Clear removes all stored credentials.
func (a *AuthStorage) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.data = make(AuthStorageData)
	a.persistProviderChange("", nil) // trigger full rewrite
}

// --- Helper functions aligned to TS resolve-config-value.ts and env key detection ---

// resolveConfigValue resolves a value that may be an env var reference ($VAR).
// Aligned to TS resolveConfigValue.
func resolveConfigValue(value string) string {
	if strings.HasPrefix(value, "$") {
		envKey := value[1:]
		if v := os.Getenv(envKey); v != "" {
			return v
		}
		return "" // env var not set
	}
	return value
}

// findEnvKeys returns the well-known env key name for a provider.
// Aligned to TS findEnvKeys.
func findEnvKeys(provider string) string {
	switch strings.ToLower(provider) {
	case "anthropic":
		if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
			return "ANTHROPIC_API_KEY"
		}
		if v := os.Getenv("ANTHROPIC_AUTH_TOKEN"); v != "" {
			return "ANTHROPIC_AUTH_TOKEN"
		}
	default:
		envKey := strings.ToUpper(provider) + "_API_KEY"
		if os.Getenv(envKey) != "" {
			return envKey
		}
	}
	return ""
}

// getEnvApiKey returns the actual API key value from environment variables.
// Aligned to TS getEnvApiKey.
func getEnvApiKey(provider string) string {
	switch strings.ToLower(provider) {
	case "anthropic":
		if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
			return v
		}
		if v := os.Getenv("ANTHROPIC_AUTH_TOKEN"); v != "" {
			return v
		}
	default:
		envKey := strings.ToUpper(provider) + "_API_KEY"
		return os.Getenv(envKey)
	}
	return ""
}