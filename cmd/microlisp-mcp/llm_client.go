package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type AnthropicClient struct {
	APIKey  string
	BaseURL string
	Model   string
}

func NewAnthropicClient() *AnthropicClient {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_AUTH_TOKEN")
	}
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &AnthropicClient{APIKey: apiKey, BaseURL: baseURL, Model: model}
}

func (c *AnthropicClient) Call(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	body := map[string]any{
		"model":      c.Model,
		"max_tokens": maxTokens,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userPrompt},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, block := range result.Content {
		sb.WriteString(block.Text)
	}
	return sb.String(), nil
}
