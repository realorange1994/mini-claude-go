package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"miniclaudecode-go/tools"
)

// ─── CacheSafeParams ─────────────────────────────────────────────────────────
//
// Capture these at the point where the fork is created. They must be identical
// between parent and fork API calls for Anthropic prompt cache sharing:
//   - System prompt: identical text
//   - Tools: identical tool schemas (names, parameters, descriptions)
//   - Model: identical model string
//   - Messages prefix: the first N messages must match byte-for-byte
//
// The only difference between parent and fork API requests is the fork's own
// messages appended at the end. Anthropic's cache key matches on the prefix.

type CacheSafeParams struct {
	SystemPrompt   string
	Model          string
	Tools          []anthropic.ToolUnionParam // must match parent's tool schemas exactly
	Messages       []anthropic.MessageParam
	ThinkingConfig map[string]any // optional, for cache stability
}

// ─── CanUseToolFn ────────────────────────────────────────────────────────────

// CanUseToolFn is a runtime permission hook called before each tool execution
// in a forked agent. Return (true, "") to allow, (false, reason) to deny.
type CanUseToolFn func(toolName string, args map[string]any) (allowed bool, reason string)

// ─── ForkedAgentConfig ───────────────────────────────────────────────────────

type ForkedAgentConfig struct {
	CacheSafeParams     CacheSafeParams
	ForkMessages        []anthropic.MessageParam // fork's own messages (only these differ → cache HIT)
	CanUseTool          CanUseToolFn
	MaxTokens           int
	QuerySource         string // tracking label (e.g., "session_memory")
	MaxTurns            int    // max tool call rounds (default: 10)
	Registry            *tools.Registry
	ProjectDir          string
	SkipParentMessages  bool // skip CacheSafeParams.Messages (for lightweight forks like session memory)
}

// ─── ForkedAgentResult ───────────────────────────────────────────────────────

type ForkedAgentResult struct {
	OutputText string
	ToolCalls  int
	Usage      anthropic.Usage
}

// ─── RunForkedAgent ──────────────────────────────────────────────────────────
//
// Runs a forked query loop that shares the Anthropic API prompt cache with the
// parent. The combined message list is:
//
//   cfg.CacheSafeParams.Messages (parent's conversation → cache HIT)
//   cfg.ForkMessages             (fork's prompt        → cache MISS, new tokens)
//
// For each tool call, CanUseToolFn is invoked. If denied, a tool result
// describing the denial is injected. If allowed, the tool executes normally.
// The loop continues until the assistant stops producing tool calls or
// MaxTurns is reached.

func RunForkedAgent(cfg ForkedAgentConfig) (*ForkedAgentResult, error) {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 10
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 4096
	}

	// Combine parent messages with fork messages (skip parent if requested)
	var allMessages []anthropic.MessageParam
	if cfg.SkipParentMessages {
		allMessages = make([]anthropic.MessageParam, len(cfg.ForkMessages))
		copy(allMessages, cfg.ForkMessages)
	} else {
		allMessages = make([]anthropic.MessageParam, len(cfg.CacheSafeParams.Messages)+len(cfg.ForkMessages))
		copy(allMessages, cfg.CacheSafeParams.Messages)
		copy(allMessages[len(cfg.CacheSafeParams.Messages):], cfg.ForkMessages)
	}

	params := anthropic.MessageNewParams{
		Model:     cfg.CacheSafeParams.Model,
		MaxTokens: int64(cfg.MaxTokens),
		Messages:  allMessages,
		System: []anthropic.TextBlockParam{
			{Text: cfg.CacheSafeParams.SystemPrompt},
		},
	}
	if len(cfg.CacheSafeParams.Tools) > 0 {
		params.Tools = cfg.CacheSafeParams.Tools
	}

	var totalUsage anthropic.Usage
	toolCallCount := 0

	for turn := 0; turn < cfg.MaxTurns; turn++ {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		client := createForkedClient()
		resp, err := client.Messages.New(ctx, params)
		cancel()

		if err != nil {
			errMsg := err.Error()
			cr := classifyError(errMsg, 0, 0)
			if !cr.Retryable {
				return nil, fmt.Errorf("forked agent API error (non-retryable %s): %w", cr.Class, err)
			}
			if retryErr := retryForkedCall(&params, cr); retryErr != nil {
				return nil, fmt.Errorf("forked agent API error: %w", retryErr)
			}
			// Retry succeeded, re-parse response
			continue
		}

		// Accumulate usage
		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens
		totalUsage.CacheCreationInputTokens += resp.Usage.CacheCreationInputTokens
		totalUsage.CacheReadInputTokens += resp.Usage.CacheReadInputTokens

		// Extract text and tool calls from response
		var toolCalls []anthropic.ToolUseBlock
		var textParts []string
		for _, block := range resp.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				textParts = append(textParts, b.Text)
			case anthropic.ToolUseBlock:
				toolCalls = append(toolCalls, b)
			}
		}

		// If no tool calls, the assistant is done
		if len(toolCalls) == 0 {
			return &ForkedAgentResult{
				OutputText: strings.Join(textParts, "\n"),
				ToolCalls:  toolCallCount,
				Usage:      totalUsage,
			}, nil
		}

		toolCallCount += len(toolCalls)

		// Execute tool calls with permission check
		var toolResults []anthropic.ToolResultBlockParam
		var wg sync.WaitGroup
		type toolExecResult struct {
			index  int
			result anthropic.ToolResultBlockParam
		}
		results := make([]toolExecResult, len(toolCalls))

		for i, tc := range toolCalls {
			wg.Add(1)
			go func(idx int, tc anthropic.ToolUseBlock) {
				defer wg.Done()

				// ToolUseBlock.Input is []byte (JSON), unmarshal it
				var args map[string]any
				if len(tc.Input) > 0 {
					_ = json.Unmarshal(tc.Input, &args)
				}
				if args == nil {
					args = make(map[string]any)
				}
				toolName := tc.Name

				// Permission check
				if cfg.CanUseTool != nil {
					if allowed, reason := cfg.CanUseTool(toolName, args); !allowed {
						results[idx] = toolExecResult{
							index: idx,
							result: anthropic.ToolResultBlockParam{
								ToolUseID: tc.ID,
								Content: []anthropic.ToolResultBlockParamContentUnion{
									{OfText: &anthropic.TextBlockParam{Text: "Permission denied: " + reason}},
								},
								IsError: param.NewOpt(true),
							},
						}
						return
					}
				}

				// Execute tool
				output := executeForkedTool(cfg, toolName, args)
				results[idx] = toolExecResult{
					index: idx,
					result: anthropic.ToolResultBlockParam{
						ToolUseID: tc.ID,
						Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: output}}},
					},
				}
			}(i, tc)
		}
		wg.Wait()

		// Sort results by index
		for _, r := range results {
			toolResults = append(toolResults, r.result)
		}

		// Build assistant message blocks with tool calls
		var assistantBlocks []anthropic.ContentBlockParamUnion
		for _, tc := range toolCalls {
			var input map[string]any
			if len(tc.Input) > 0 {
				_ = json.Unmarshal(tc.Input, &input)
			}
			assistantBlocks = append(assistantBlocks, anthropic.ContentBlockParamUnion{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				},
			})
		}
		allMessages = append(allMessages, anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleAssistant,
			Content: assistantBlocks,
		})

		// Build user message with tool results
		var toolResultBlocks []anthropic.ContentBlockParamUnion
		for _, tr := range toolResults {
			for _, c := range tr.Content {
				if c.OfText != nil {
					toolResultBlocks = append(toolResultBlocks, anthropic.ContentBlockParamUnion{
						OfToolResult: &anthropic.ToolResultBlockParam{
							ToolUseID: tr.ToolUseID,
							Content:   []anthropic.ToolResultBlockParamContentUnion{c},
						},
					})
				}
			}
		}
		allMessages = append(allMessages, anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleUser,
			Content: toolResultBlocks,
		})

		// Update params with new messages
		params.Messages = allMessages
	}

	// Max turns reached
	return &ForkedAgentResult{
		ToolCalls: toolCallCount,
		Usage:     totalUsage,
	}, nil
}

// retryForkedCall retries the API call with backoff on transient errors.
// It respects the error classification: non-retryable errors return immediately,
// rate limits use RetryAfter duration, others use exponential backoff with jitter.
func retryForkedCall(params *anthropic.MessageNewParams, cr ClassifyResult) error {
	const maxRetries = 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		var delay time.Duration
		if cr.RetryAfter > 0 {
			delay = cr.RetryAfter
		} else {
			delay = time.Duration(1<<uint(attempt)) * time.Second
			// Add jitter
			delay += time.Duration(rand.Int63n(int64(delay / 2)))
		}
		time.Sleep(delay)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		client := createForkedClient()
		_, err := client.Messages.New(ctx, *params)
		cancel()

		if err == nil {
			return nil
		}
		// Re-classify on each retry attempt in case error type changes
		newCr := classifyError(err.Error(), 0, 0)
		if !newCr.Retryable {
			return fmt.Errorf("non-retryable error on retry (%s): %w", newCr.Class, err)
		}
		cr = newCr
	}
	return fmt.Errorf("forked agent API call failed after %d retries", maxRetries)
}

// createForkedClient creates an anthropic client matching the parent's auth.
func createForkedClient() anthropic.Client {
	opts := []option.RequestOption{}
	if apiKey := getForkedAPIKey(); apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if baseURL := getForkedBaseURL(); baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return anthropic.NewClient(opts...)
}

func getForkedAPIKey() string {
	return os.Getenv("ANTHROPIC_API_KEY")
}

func getForkedBaseURL() string {
	return os.Getenv("ANTHROPIC_BASE_URL")
}

// executeForkedTool executes a tool call using the registry directly.
// No permission check is done here — the caller should check CanUseTool first.
func executeForkedTool(cfg ForkedAgentConfig, toolName string, args map[string]any) string {
	if cfg.Registry == nil {
		return "Error: no tool registry available in forked agent"
	}

	tool, ok := cfg.Registry.Get(toolName)
	if !ok {
		return fmt.Sprintf("Error: unknown tool %q", toolName)
	}

	// Coerce argument types to match schema
	tools.CoerceArguments(tool.InputSchema(), args)

	// Remap directory parameter name (official: directory, internal: dir)
	tools.RemapDirParam(args)

	// Validate required parameters
	if err := tools.ValidateParams(tool, args); err != nil {
		return "Error: " + err.Error()
	}

	// Context timeout for tool execution
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := tools.ExecuteWithContext(ctx, tool, args)
	output := result.Output

	// Truncate long outputs
	if len(output) > 50000 {
		output = output[:50000] + "\n\n[... output truncated, 50000 char limit ...]"
	}
	return output
}

// ─── Helper: build tool params from registry ─────────────────────────────────

func buildForkedToolParams(registry *tools.Registry) []anthropic.ToolUnionParam {
	if registry == nil {
		return nil
	}
	var params []anthropic.ToolUnionParam
	for _, t := range registry.AllTools() {
		schema := t.InputSchema()
		if schema == nil {
			continue
		}
		params = append(params, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name(),
				Description: param.NewOpt(t.Description()),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: schema["properties"],
					Required:   getStringSlice(schema, "required"),
				},
			},
		})
	}
	return params
}

// getStringSlice is defined in agent_loop.go — shared across package main.

// ─── CacheSafeParams capture ─────────────────────────────────────────────────
//
// Capture at any point during the main agent loop. These params are stable
// until the system prompt changes, tools change, or messages are added.

func CaptureCacheSafeParams(systemPrompt string, model string, registry *tools.Registry, messages []anthropic.MessageParam) CacheSafeParams {
	return CacheSafeParams{
		SystemPrompt: systemPrompt,
		Model:       model,
		Tools:       buildForkedToolParams(registry),
		Messages:    messages,
	}
}
