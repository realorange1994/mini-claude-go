// Package auth provides credential storage for API keys and OAuth tokens.
// Aligned to pi's auth-storage.ts.
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuthCredential represents a stored credential.
type AuthCredential struct {
	Type     string `json:"type"` // "api_key" or "oauth"
	Key      string `json:"key,omitempty"`
	ClientID string `json:"clientId,omitempty"`
	// Additional OAuth fields would go here
}

// AuthStorageData is the full auth.json structure.
type AuthStorageData map[string]AuthCredential

// AuthStatus represents whether auth is configured for a provider.
type AuthStatus struct {
	Configured bool
	Source     string // "stored", "runtime", "environment", "fallback"
	Label      string
}

// AuthStorage manages credential storage with file-based persistence.
type AuthStorage struct {
	mu         sync.RWMutex
	authPath   string
	data       AuthStorageData
	loaded     bool
}

// Create creates a new AuthStorage with file-based backend.
func Create(authPath string) *AuthStorage {
	a := &AuthStorage{
		authPath: authPath,
		data:     make(AuthStorageData),
	}
	// Try to load existing auth file
	a.load()
	return a
}

// load reads the auth file from disk.
func (a *AuthStorage) load() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.authPath == "" {
		a.data = make(AuthStorageData)
		a.loaded = true
		return nil
	}

	data, err := os.ReadFile(a.authPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create empty auth file
			if err := a.ensureParentDir(); err != nil {
				return err
			}
			a.data = make(AuthStorageData)
			a.loaded = true
			return nil
		}
		return fmt.Errorf("read auth file: %w", err)
	}

	a.data = make(AuthStorageData)
	if err := json.Unmarshal(data, &a.data); err != nil {
		return fmt.Errorf("parse auth file: %w", err)
	}

	a.loaded = true
	return nil
}

// save persists the auth data to disk.
func (a *AuthStorage) save() error {
	if a.authPath == "" {
		return nil
	}

	data, err := json.MarshalIndent(a.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth data: %w", err)
	}

	if err := os.WriteFile(a.authPath, data, 0600); err != nil {
		return fmt.Errorf("write auth file: %w", err)
	}

	return nil
}

// ensureParentDir creates the parent directory if needed.
func (a *AuthStorage) ensureParentDir() error {
	dir := filepath.Dir(a.authPath)
	return os.MkdirAll(dir, 0700)
}

// GetCredential returns a credential by provider key.
func (a *AuthStorage) GetCredential(providerKey string) (AuthCredential, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.loaded {
		return AuthCredential{}, false
	}

	cred, ok := a.data[providerKey]
	return cred, ok
}

// SetCredential stores a credential.
func (a *AuthStorage) SetCredential(providerKey string, cred AuthCredential) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.data[providerKey] = cred
	return a.save()
}

// RemoveCredential removes a credential.
func (a *AuthStorage) RemoveCredential(providerKey string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.data, providerKey)
	return a.save()
}

// GetStatus returns auth status for a provider.
func (a *AuthStorage) GetStatus(providerKey string) AuthStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.loaded {
		return AuthStatus{Configured: false, Source: "environment"}
	}

	if cred, ok := a.data[providerKey]; ok && cred.Key != "" {
		return AuthStatus{
			Configured: true,
			Source:     "stored",
			Label:      providerKey,
		}
	}

	// Check environment variable as fallback
	envKey := providerKey + "_API_KEY"
	if os.Getenv(envKey) != "" {
		return AuthStatus{
			Configured: true,
			Source:     "environment",
			Label:      providerKey,
		}
	}

	return AuthStatus{Configured: false}
}

// IsConfigured returns whether any credentials are stored.
func (a *AuthStorage) IsConfigured() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.loaded {
		return false
	}

	for _, cred := range a.data {
		if cred.Key != "" {
			return true
		}
	}

	return false
}

// Reload refreshes auth storage from file.
func (a *AuthStorage) Reload() error {
	return a.load()
}

// Clear removes all stored credentials.
func (a *AuthStorage) Clear() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.data = make(AuthStorageData)
	return a.save()
}

// GetAuthPath returns the path to the auth file.
func (a *AuthStorage) GetAuthPath() string {
	return a.authPath
}

// WithLock executes a function while holding exclusive lock.
func (a *AuthStorage) WithLock(fn func(data AuthStorageData)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	fn(a.data)
}

// WithLockResult executes a function while holding exclusive lock and returns a result.
func (a *AuthStorage) WithLockResult(fn func(data AuthStorageData) interface{}) interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	return fn(a.data)
}

// Token represents an OAuth token with expiry.
type Token struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

// IsExpired returns true if the token has expired.
func (t *Token) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsExpiringSoon returns true if token expires within 5 minutes.
func (t *Token) IsExpiringSoon() bool {
	return time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
}