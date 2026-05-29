package agent

import (
	"bufio"
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

// HTTPClientConfig holds configuration for the HTTP-based LLM client.
type HTTPClientConfig struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	MaxTokens    int
	Timeout      time.Duration
}

// HTTPClient implements LLMClient using Anthropic-compatible HTTP API.
type HTTPClient struct {
	config HTTPClientConfig
	client *http.Client
}

// NewHTTPClient creates a new HTTP-based LLM client.
func NewHTTPClient(config HTTPClientConfig) *HTTPClient {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &HTTPClient{
		config: config,
		client: &http.Client{Timeout: timeout},
	}
}

// apiRequest represents the JSON body sent to the Anthropic Messages API.
type apiRequest struct {
	Model     string                 `json:"model"`
	MaxTokens int                    `json:"max_tokens"`
	Messages  []map[string]interface{} `json:"messages"`
	System    interface{}            `json:"system,omitempty"`
	Tools     []toolDefJSON          `json:"tools,omitempty"`
	Thinking  *ThinkingConfig        `json:"thinking,omitempty"`
	Stream    bool                   `json:"stream,omitempty"`
}

type toolDefJSON struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema interface{} `json:"input_schema,omitempty"`
}

// apiResponse represents the JSON response from the Anthropic Messages API.
type apiResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
		// For tool_use content blocks
		Name  string          `json:"name,omitempty"`
		ID    string          `json:"id,omitempty"`
		Input json.RawMessage `json:"input,omitempty"`
	} `json:"content"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason,omitempty"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// apiErrorResponse represents an error response from the API.
type apiErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// APIError represents an error from the LLM API.
type APIError struct {
	StatusCode int
	Type       string
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error (status %d): %s: %s", e.StatusCode, e.Type, e.Message)
}

// IsRetryable returns true if the API error can be retried.
func (e *APIError) IsRetryable() bool {
	switch e.StatusCode {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// IsOverloaded returns true if the API is overloaded.
func (e *APIError) IsOverloaded() bool {
	return e.StatusCode == 529 || (e.StatusCode == 429 && strings.Contains(strings.ToLower(e.Message), "overloaded"))
}

// IsContextOverflow returns true if the error is due to context window overflow.
func (e *APIError) IsContextOverflow() bool {
	return strings.Contains(strings.ToLower(e.Message), "prompt is too long") ||
		strings.Contains(strings.ToLower(e.Message), "context window") ||
		strings.Contains(strings.ToLower(e.Message), "token limit") ||
		strings.Contains(strings.ToLower(e.Message), "max_tokens")
}

// Complete sends messages to the model and returns the response.
func (c *HTTPClient) Complete(ctx context.Context, model string, messages []map[string]interface{}, tools []extensions.ToolDefinition, thinking *ThinkingConfig) (string, error) {
	if model == "" {
		model = c.config.DefaultModel
	}

	maxTokens := c.config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	// Separate system messages from conversation messages
	var systemContent interface{}
	var convMessages []map[string]interface{}
	for _, msg := range messages {
		if role, _ := msg["role"].(string); role == "system" {
			// Anthropic API expects system as a top-level field
			if content, ok := msg["content"].(string); ok {
				systemContent = content
			}
		} else {
			convMessages = append(convMessages, msg)
		}
	}

	reqBody := apiRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  convMessages,
		System:    systemContent,
		Thinking:  thinking,
	}

	// Convert tool definitions
	if len(tools) > 0 {
		for _, t := range tools {
			reqBody.Tools = append(reqBody.Tools, toolDefJSON{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp apiErrorResponse
		_ = json.Unmarshal(respBody, &errResp)
		return "", &APIError{
			StatusCode: resp.StatusCode,
			Type:       errResp.Error.Type,
			Message:    errResp.Error.Message,
		}
	}

	var result apiResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Return the full response JSON so processResponse() can parse tool_use blocks.
	// We reconstruct it with the same structure as the API returned.
	respJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}
	return string(respJSON), nil
}

// CompleteStreaming sends messages to the model and streams the response via callback.
func (c *HTTPClient) CompleteStreaming(ctx context.Context, model string, messages []map[string]interface{}, tools []extensions.ToolDefinition, thinking *ThinkingConfig, onChunk func(string)) error {
	if model == "" {
		model = c.config.DefaultModel
	}

	maxTokens := c.config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	// Separate system messages from conversation messages
	var systemContent interface{}
	var convMessages []map[string]interface{}
	for _, msg := range messages {
		if role, _ := msg["role"].(string); role == "system" {
			if content, ok := msg["content"].(string); ok {
				systemContent = content
			}
		} else {
			convMessages = append(convMessages, msg)
		}
	}

	reqBody := apiRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  convMessages,
		System:    systemContent,
		Thinking:  thinking,
		Stream:    true,
	}

	// Convert tool definitions
	if len(tools) > 0 {
		for _, t := range tools {
			reqBody.Tools = append(reqBody.Tools, toolDefJSON{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp apiErrorResponse
		_ = json.Unmarshal(respBody, &errResp)
		return &APIError{
			StatusCode: resp.StatusCode,
			Type:       errResp.Error.Type,
			Message:    errResp.Error.Message,
		}
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}

		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" && event.Delta.Text != "" {
			if onChunk != nil {
				onChunk(event.Delta.Text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream read error: %w", err)
	}

	return nil
}

// Generate is a simplified LLM call for compaction and other utilities.
// It sends a system + user prompt and returns the text response.
func (c *HTTPClient) Generate(ctx context.Context, model string, systemPrompt string, userPrompt string, maxTokens int) (string, error) {
	if model == "" {
		model = c.config.DefaultModel
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}

	messages := []map[string]interface{}{
		{"role": "user", "content": userPrompt},
	}

	reqBody := apiRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  messages,
		System:    systemPrompt,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp apiErrorResponse
		_ = json.Unmarshal(respBody, &errResp)
		return "", &APIError{
			StatusCode: resp.StatusCode,
			Type:       errResp.Error.Type,
			Message:    errResp.Error.Message,
		}
	}

	var result apiResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	var textParts []string
	for _, block := range result.Content {
		if block.Type == "text" && block.Text != "" {
			textParts = append(textParts, block.Text)
		}
	}

	return strings.Join(textParts, "\n"), nil
}

