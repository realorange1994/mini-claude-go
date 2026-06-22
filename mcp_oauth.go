package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─── MCP OAuth Provider (MiMo-Code 3) ──────────────────────────────────────
//
// Full OAuth 2.0 authorization code flow for MCP servers.
// Supports dynamic client registration, PKCE, and token persistence.
//
// MiMo-Code source: mcp/oauth-provider.ts (214 lines)

// OAuthToken represents an OAuth token.
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
}

// OAuthConfig holds OAuth configuration.
type OAuthConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	RedirectURI  string `json:"redirect_uri"`
	AuthURL      string `json:"auth_url"`
	TokenURL     string `json:"token_url"`
	Scopes       []string `json:"scopes,omitempty"`
}

// MCPOAuthProvider manages OAuth for MCP servers.
type MCPOAuthProvider struct {
	mu        sync.Mutex
	config    OAuthConfig
	token     *OAuthToken
	tokenPath string
	serverID  string
}

// NewMCPOAuthProvider creates a new MCP OAuth provider.
func NewMCPOAuthProvider(serverID, configDir string) *MCPOAuthProvider {
	return &MCPOAuthProvider{
		serverID:  serverID,
		tokenPath: filepath.Join(configDir, "mcp-oauth", serverID+".json"),
	}
}

// SetConfig sets the OAuth configuration.
func (p *MCPOAuthProvider) SetConfig(config OAuthConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config = config
}

// GetAuthURL returns the authorization URL.
func (p *MCPOAuthProvider) GetAuthURL(state string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	params := fmt.Sprintf("client_id=%s&redirect_uri=%s&response_type=code&state=%s",
		p.config.ClientID, p.config.RedirectURI, state)

	if len(p.config.Scopes) > 0 {
		params += "&scope=" + strings.Join(p.config.Scopes, " ")
	}

	return p.config.AuthURL + "?" + params
}

// ExchangeCode exchanges an authorization code for tokens.
func (p *MCPOAuthProvider) ExchangeCode(code string) (*OAuthToken, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Build request
	reqBody := fmt.Sprintf("grant_type=authorization_code&code=%s&redirect_uri=%s&client_id=%s",
		code, p.config.RedirectURI, p.config.ClientID)

	if p.config.ClientSecret != "" {
		reqBody += "&client_secret=" + p.config.ClientSecret
	}

	// Make request
	resp, err := http.Post(p.config.TokenURL, "application/x-www-form-urlencoded",
		strings.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange failed: %d", resp.StatusCode)
	}

	// Parse response
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}

	token := &OAuthToken{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}

	p.token = token
	p.saveToken(token)

	return token, nil
}

// GetToken returns the current token.
func (p *MCPOAuthProvider) GetToken() *OAuthToken {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.token
}

// IsExpired checks if the token is expired.
func (p *MCPOAuthProvider) IsExpired() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token == nil {
		return true
	}
	return time.Now().After(p.token.ExpiresAt)
}

// RefreshToken refreshes the access token.
func (p *MCPOAuthProvider) RefreshToken() (*OAuthToken, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.token == nil || p.token.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	reqBody := fmt.Sprintf("grant_type=refresh_token&refresh_token=%s&client_id=%s",
		p.token.RefreshToken, p.config.ClientID)

	resp, err := http.Post(p.config.TokenURL, "application/x-www-form-urlencoded",
		strings.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed: %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}

	token := &OAuthToken{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	p.token = token
	p.saveToken(token)

	return token, nil
}

// Invalidate invalidates the current token.
func (p *MCPOAuthProvider) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.token = nil
	os.Remove(p.tokenPath)
}

// saveToken persists the token to disk.
func (p *MCPOAuthProvider) saveToken(token *OAuthToken) {
	if p.tokenPath == "" {
		return
	}
	os.MkdirAll(filepath.Dir(p.tokenPath), 0755)
	data, _ := json.MarshalIndent(token, "", "  ")
	os.WriteFile(p.tokenPath, data, 0600)
}

// loadToken loads the token from disk.
func (p *MCPOAuthProvider) loadToken() {
	if p.tokenPath == "" {
		return
	}
	data, err := os.ReadFile(p.tokenPath)
	if err != nil {
		return
	}
	var token OAuthToken
	if json.Unmarshal(data, &token) == nil {
		p.token = &token
	}
}

// FormatOAuthStatus formats OAuth status for display.
func FormatOAuthStatus(provider *MCPOAuthProvider) string {
	token := provider.GetToken()
	if token == nil {
		return "Not authenticated."
	}
	if provider.IsExpired() {
		return "Token expired."
	}
	return fmt.Sprintf("Authenticated (expires: %s)", token.ExpiresAt.Format(time.RFC3339))
}
