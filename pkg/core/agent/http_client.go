package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"miniclaudecode-go/pkg/core/extensions"
)

// HTTPClientConfig holds configuration for the HTTP LLM client.
type HTTPClientConfig struct {
	// BaseURL is the API endpoint (e.g., "https://api.anthropic.com/v1/messages")
	BaseURL string
	// APIKey is the authentication key
	APIKey string
	// DefaultModel is the model to use
	DefaultModel string
	// MaxTokens is the maximum tokens to generate
	MaxTokens int
	// HTTPTimeout is the timeout for individual HTTP requests (default: 5 minutes)
	HTTPTimeout time.Duration
}

// HTTPClient is a simple HTTP LLM client that calls Anthropic-compatible APIs.
type HTTPClient struct {
	config HTTPClientConfig
	http   *http.Client
}

// NewHTTPClient creates a new HTTP LLM client.
func NewHTTPClient(config HTTPClientConfig) *HTTPClient {
	if config.MaxTokens == 0 {
		config.MaxTokens = 8192
	}
	// Default HTTP timeout: 5 minutes per request (LLM responses can be slow)
	httpTimeout := config.HTTPTimeout
	if httpTimeout == 0 {
		httpTimeout = 5 * time.Minute
	}
	return &HTTPClient{
		config: config,
		http: &http.Client{
			Timeout: httpTimeout,
		},
	}
}

// Complete sends messages to the model and returns the response.
func (c *HTTPClient) Complete(ctx context.Context, model string, messages []map[string]interface{}, toolDefs []extensions.ToolDefinition) (string, error) {
	if model == "" {
		model = c.config.DefaultModel
	}

	// Build request body
	body := map[string]interface{}{
		"model":     model,
		"max_tokens": c.config.MaxTokens,
		"messages":  messages,
	}

	// Add tools if available
	if len(toolDefs) > 0 {
		apiTools := make([]map[string]interface{}, len(toolDefs))
		for i, def := range toolDefs {
			apiTools[i] = map[string]interface{}{
				"name":         def.Name,
				"description":  def.Description,
				"input_schema": def.InputSchema,
			}
		}
		body["tools"] = apiTools
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}


	if resp.StatusCode != http.StatusOK {
		preview := string(respBody)
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, preview)
	}

	// Return the entire raw response JSON so parseToolCalls can parse it
	// as a structured Anthropic response (content blocks array).
	return string(respBody), nil
}

// CompleteStreaming calls the model with streaming output.
func (c *HTTPClient) CompleteStreaming(ctx context.Context, model string, messages []map[string]interface{}, toolDefs []extensions.ToolDefinition, onChunk func(string)) error {
	if model == "" {
		model = c.config.DefaultModel
	}

	// Build request body
	body := map[string]interface{}{
		"model":     model,
		"max_tokens": c.config.MaxTokens,
		"messages":  messages,
		"stream":    true,
	}

	// Add tools
	if len(toolDefs) > 0 {
		apiTools := make([]map[string]interface{}, len(toolDefs))
		for i, def := range toolDefs {
			apiTools[i] = map[string]interface{}{
				"name":         def.Name,
				"description":  def.Description,
				"input_schema": def.InputSchema,
			}
		}
		body["tools"] = apiTools
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2024-02-15")
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, respBody)
	}

	// Read streaming response
	reader := resp.Body
	buf := make([]byte, 1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			// Parse SSE events
			lines := strings.Split(chunk, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "data: ") {
					data := strings.TrimPrefix(line, "data: ")
					if data == "[DONE]" {
						return nil
					}

					// Parse JSON event
					var event map[string]interface{}
					if err := json.Unmarshal([]byte(data), &event); err != nil {
						continue
					}

					// Extract text delta
					if delta, ok := event["delta"].(map[string]interface{}); ok {
						if text, ok := delta["text"].(string); ok {
							onChunk(text)
						}
					}

					// Extract tool use from streaming
					if eventType, ok := event["type"].(string); ok {
						if eventType == "content_block_start" {
							if contentBlock, ok := event["content_block"].(map[string]interface{}); ok {
								if contentBlock["type"] == "tool_use" {
									// Tool use started
									if text, ok := contentBlock["text"].(string); ok && text != "" {
										onChunk(text)
									}
								}
							}
						}
						if eventType == "content_block_delta" {
							if delta, ok := event["delta"].(map[string]interface{}); ok {
								if partialJSON, ok := delta["partial_json"].(string); ok && partialJSON != "" {
									onChunk(partialJSON)
								}
							}
						}
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read stream: %w", err)
		}
	}
}

// Ensure HTTPClient implements LLMClient
var _ LLMClient = (*HTTPClient)(nil)
