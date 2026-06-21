package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ─── Code Search Tool (MiMo-Code 7) ────────────────────────────────────────
//
// Provides external code search via the Exa API.
// Returns up-to-date documentation, code examples, and API references.
//
// MiMo-Code source: tool/codesearch.ts (63 lines)

const (
	// DefaultCodeSearchTokens default token budget for code search
	DefaultCodeSearchTokens = 5000
	// MaxCodeSearchTokens maximum token budget
	MaxCodeSearchTokens = 50000
	// MinCodeSearchTokens minimum token budget
	MinCodeSearchTokens = 1000
	// ExaAPIEndpoint Exa API endpoint for code search
	ExaAPIEndpoint = "https://api.exa.ai/search"
)

// CodeSearchConfig holds code search configuration.
type CodeSearchConfig struct {
	Enabled    bool   `json:"enabled"`
	APIKey     string `json:"api_key"`
	MaxTokens  int    `json:"max_tokens"`
	Endpoint   string `json:"endpoint"`
}

// NewCodeSearchConfig creates a new code search config with defaults.
func NewCodeSearchConfig() *CodeSearchConfig {
	return &CodeSearchConfig{
		Enabled:   false,
		MaxTokens: DefaultCodeSearchTokens,
		Endpoint:  ExaAPIEndpoint,
	}
}

// CodeSearchResult holds the result of a code search.
type CodeSearchResult struct {
	Results  []CodeSearchHit `json:"results"`
	Tokens   int             `json:"tokens"`
	Query    string          `json:"query"`
}

// CodeSearchHit represents a single search result.
type CodeSearchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Score   float64 `json:"score"`
}

// CodeSearchService provides code search functionality.
type CodeSearchService struct {
	config *CodeSearchConfig
	client *http.Client
}

// NewCodeSearchService creates a new code search service.
func NewCodeSearchService(config *CodeSearchConfig) *CodeSearchService {
	if config == nil {
		config = NewCodeSearchConfig()
	}
	return &CodeSearchService{
		config: config,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Search performs a code search.
func (s *CodeSearchService) Search(query string, maxTokens int) (*CodeSearchResult, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("code search is not enabled")
	}

	if s.config.APIKey == "" {
		return nil, fmt.Errorf("code search API key not configured")
	}

	// Clamp token budget
	if maxTokens < MinCodeSearchTokens {
		maxTokens = MinCodeSearchTokens
	}
	if maxTokens > MaxCodeSearchTokens {
		maxTokens = MaxCodeSearchTokens
	}

	// Build request
	reqBody := map[string]any{
		"query":      query,
		"numResults": 5,
		"type":       "neural",
		"contents": map[string]any{
			"text": map[string]any{
				"maxCharacters": maxTokens * 4, // ~4 chars per token
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Make request
	endpoint := s.config.Endpoint
	if endpoint == "" {
		endpoint = ExaAPIEndpoint
	}

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.config.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed (%d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var exaResp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Text    string `json:"text"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&exaResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Convert to our format
	var hits []CodeSearchHit
	totalTokens := 0
	for _, r := range exaResp.Results {
		snippet := r.Text
		if len(snippet) > 500 {
			snippet = snippet[:500] + "..."
		}
		tokens := (len(snippet) + 3) / 4
		totalTokens += tokens

		hits = append(hits, CodeSearchHit{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snippet,
			Score:   r.Score,
		})
	}

	return &CodeSearchResult{
		Results: hits,
		Tokens:  totalTokens,
		Query:   query,
	}, nil
}

// FormatResults formats search results for display.
func FormatResults(result *CodeSearchResult) string {
	if result == nil || len(result.Results) == 0 {
		return "No results found."
	}

	var sb string
	sb += fmt.Sprintf("## Code Search Results for: %s\n\n", result.Query)
	sb += fmt.Sprintf("_%d results, ~%d tokens_\n\n", len(result.Results), result.Tokens)

	for i, hit := range result.Results {
		sb += fmt.Sprintf("### %d. %s\n", i+1, hit.Title)
		sb += fmt.Sprintf("URL: %s\n\n", hit.URL)
		sb += hit.Snippet + "\n\n"
	}

	return sb
}

// IsEnabled returns true if code search is enabled.
func (s *CodeSearchService) IsEnabled() bool {
	return s.config.Enabled && s.config.APIKey != ""
}
