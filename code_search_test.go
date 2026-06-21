package main

import (
	"testing"
)

func TestCodeSearchConfig_Defaults(t *testing.T) {
	config := NewCodeSearchConfig()

	if config.Enabled {
		t.Error("expected disabled by default")
	}
	if config.MaxTokens != DefaultCodeSearchTokens {
		t.Errorf("expected %d tokens, got %d", DefaultCodeSearchTokens, config.MaxTokens)
	}
	if config.Endpoint != ExaAPIEndpoint {
		t.Errorf("expected %s, got %s", ExaAPIEndpoint, config.Endpoint)
	}
}

func TestCodeSearchService_IsEnabled(t *testing.T) {
	config := NewCodeSearchConfig()
	s := NewCodeSearchService(config)

	if s.IsEnabled() {
		t.Error("expected disabled without API key")
	}

	config.Enabled = true
	config.APIKey = "test-key"
	s = NewCodeSearchService(config)

	if !s.IsEnabled() {
		t.Error("expected enabled with API key")
	}
}

func TestCodeSearchService_Search_Disabled(t *testing.T) {
	config := NewCodeSearchConfig()
	s := NewCodeSearchService(config)

	_, err := s.Search("test query", 5000)
	if err == nil {
		t.Error("expected error when disabled")
	}
}

func TestCodeSearchService_Search_NoAPIKey(t *testing.T) {
	config := NewCodeSearchConfig()
	config.Enabled = true
	s := NewCodeSearchService(config)

	_, err := s.Search("test query", 5000)
	if err == nil {
		t.Error("expected error without API key")
	}
}

func TestFormatResults(t *testing.T) {
	result := &CodeSearchResult{
		Query:  "test query",
		Tokens: 100,
		Results: []CodeSearchHit{
			{Title: "Test Result", URL: "https://example.com", Snippet: "test snippet"},
		},
	}

	output := FormatResults(result)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatResults_Empty(t *testing.T) {
	result := &CodeSearchResult{
		Query:   "test query",
		Results: nil,
	}

	output := FormatResults(result)
	if output != "No results found." {
		t.Errorf("expected 'No results found.', got %q", output)
	}
}

func TestNewCodeSearchService_NilConfig(t *testing.T) {
	s := NewCodeSearchService(nil)
	if s == nil {
		t.Error("expected non-nil service")
	}
}
