package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"miniclaudecode-go/tools"
	"miniclaudecode-go/transcript"
)

// AgentLoop drives the core agentic loop.
type AgentLoop struct {
	config       Config
	registry     *tools.Registry
	gate         PermissionGate
	context      *ConversationContext
	client       anthropic.Client
	snapshots    *SnapshotHistory
	transcript   *transcript.Writer
	useStream    bool
	maxToolChars int    // max chars per tool result (default 8192)
	toolTimeout  time.Duration // per-tool execution timeout (default 60s)
	maxTurns     int    // hard cap on turns (default from config.MaxTurns)
}

// NewAgentLoop creates a new agent loop.
func NewAgentLoop(cfg Config, registry *tools.Registry, useStream bool) *AgentLoop {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: ANTHROPIC_API_KEY environment variable is not set (or use --api-key)")
		os.Exit(1)
	}

	opts := []option.RequestOption{option.WithHeader("Authorization", "Bearer "+apiKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := anthropic.NewClient(opts...)

	ctx := NewConversationContext(cfg)
	gate := NewPermissionGate(cfg)
	snapshots := NewSnapshotHistory("")

	// Initialize transcript writer
	sessionID := time.Now().Format("20060102-150405")
	transcriptDir := filepath.Join(".claude", "transcripts")
	tw := transcript.NewWriter(sessionID, filepath.Join(transcriptDir, sessionID+".jsonl"))
	_ = tw.Write(transcript.Entry{Type: "system", Content: fmt.Sprintf("model=%s, mode=%s", cfg.Model, cfg.PermissionMode)})

	maxTurns := cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 20 // default from ggbot
	}

	agent := &AgentLoop{
		config:      cfg,
		registry:    registry,
		gate:        gate,
		context:     ctx,
		client:      client,
		snapshots:   snapshots,
		transcript:  tw,
		useStream:   useStream,
		maxToolChars: 8192,
		toolTimeout:  120 * time.Second,
		maxTurns:     maxTurns,
	}

	sysPrompt := BuildSystemPrompt(registry, string(cfg.PermissionMode), "", cfg.SkillLoader)
	ctx.SetSystemPrompt(sysPrompt)

	return agent
}

// Run processes a user message through the agent loop, returning the final text response.
func (a *AgentLoop) Run(userMessage string) string {
	a.context.AddUserMessage(userMessage)
	if a.transcript != nil {
		_ = a.transcript.WriteUser(userMessage)
	}
	var finalText string

	// Recovery state (mirrors ggbot's State machine)
	contextErrors := 0
	const maxContextRecovery = 3 // Phase 1: truncate, Phase 2: aggressive truncate, Phase 3: give up

	for turn := 0; turn < a.maxTurns; turn++ {
		var toolCalls []map[string]any
		var textParts []string
		var err error

		if a.useStream {
			toolCalls, textParts, err = a.callAPIStreaming()
		} else {
			var response *anthropic.Message
			response, err = a.callAPI()
			if err == nil {
				toolCalls, textParts = a.parseResponse(response)
			}
		}
		if err != nil {
			errMsg := err.Error()
			// Model confusion — echoed tool syntax as text; recover by retrying
			if strings.Contains(errMsg, "model confused") {
				fmt.Fprintf(os.Stderr, "\n[!] Model confused, retrying...\n")
				// Add a hint so the model doesn't repeat the same mistake
				a.context.AddUserMessage("ERROR: Your previous response was malformed. Do NOT output tool syntax as text. Use proper tool calls only.")
				continue
			}
			// Stream stalled — safety timeout fired or context canceled; recover with truncation
			if strings.Contains(errMsg, "stream stalled") ||
				strings.Contains(errMsg, "context canceled") ||
				strings.Contains(errMsg, "context deadline exceeded") {
				contextErrors++
				if contextErrors > maxContextRecovery {
					fmt.Fprintf(os.Stderr, "\n[x] Stream stalled after %d recovery attempts, giving up.\n", maxContextRecovery)
					return finalText
				}
				if contextErrors <= 1 {
					fmt.Fprintf(os.Stderr, "\n[!]  Stream stalled, truncating history (phase 1/3)...\n")
					a.context.TruncateHistory()
				} else if contextErrors <= 2 {
					fmt.Fprintf(os.Stderr, "\n[!]  Stream still stalled, aggressive truncation (phase 2/3)...\n")
					a.context.AggressiveTruncateHistory()
				} else {
					fmt.Fprintf(os.Stderr, "\n[!]  Stream still stalled, dropping to minimum (phase 3/3)...\n")
					a.context.MinimumHistory()
				}
				continue
			}
			if isContextLengthError(errMsg) {
				contextErrors++
				if contextErrors > maxContextRecovery {
					fmt.Fprintf(os.Stderr, "\n[x] Context length exceeded after %d recovery attempts, giving up.\n", maxContextRecovery)
					return finalText
				}

				if contextErrors <= 1 {
					fmt.Fprintf(os.Stderr, "\n[!]  Context length exceeded, truncating history (phase 1/3)...\n")
					a.context.TruncateHistory()
				} else if contextErrors <= 2 {
					fmt.Fprintf(os.Stderr, "\n[!]  Context still full, aggressive truncation (phase 2/3)...\n")
					a.context.AggressiveTruncateHistory()
				} else {
					fmt.Fprintf(os.Stderr, "\n[!]  Context still full, dropping to minimum (phase 3/3)...\n")
					a.context.MinimumHistory()
				}
				continue
			}
			return fmt.Sprintf("API error: %v", err)
		}

		// Reset context error counter on successful API call
		contextErrors = 0

		if len(textParts) > 0 {
			finalText = strings.Join(textParts, "\n")
		}

		if len(toolCalls) == 0 {
			a.context.AddAssistantText(finalText)
			if a.transcript != nil {
				_ = a.transcript.WriteAssistant(finalText, a.config.Model)
			}
			break
		}

		a.context.AddAssistantToolCalls(toolCalls)
		a.executeToolCallsConcurrent(toolCalls)
	}

	if finalText == "" {
		finalText = "(max turns reached without a final response)"
	}

	// Flush transcript after each turn
	if a.transcript != nil {
		_ = a.transcript.Flush()
	}

	return finalText
}

// Close releases resources (transcript writer).
func (a *AgentLoop) Close() {
	if a.transcript != nil {
		_ = a.transcript.Close()
	}
}

func (a *AgentLoop) callAPI() (*anthropic.Message, error) {
	toolParams := a.buildToolParams()

	// Try compaction before sending to API
	a.context.CompactContext()

	messages := a.context.BuildMessages()

	params := anthropic.MessageNewParams{
		Model:     a.config.Model,
		MaxTokens: 16384,
		Messages:  messages,
		System: []anthropic.TextBlockParam{
			{Text: a.context.SystemPrompt()},
		},
	}
	if len(toolParams) > 0 {
		params.Tools = toolParams
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	return a.client.Messages.New(ctx, params)
}

func (a *AgentLoop) callAPIStreaming() ([]map[string]any, []string, error) {
	toolParams := a.buildToolParams()

	// Try compaction before sending to API
	a.context.CompactContext()

	messages := a.context.BuildMessages()

	params := anthropic.MessageNewParams{
		Model:     a.config.Model,
		MaxTokens: 16384,
		Messages:  messages,
		System: []anthropic.TextBlockParam{
			{Text: a.context.SystemPrompt()},
		},
	}
	if len(toolParams) > 0 {
		params.Tools = toolParams
	}

	// 180s overall timeout for streaming — the stall detector inside Process
	// will force-close earlier if no events arrive for 15s.
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	collect := NewCollectHandler()
	term := &TerminalHandler{}
	adapter := NewStreamAdapter(func(chunk StreamChunk) error {
		_ = collect.Handle(chunk)
		if err := term.Handle(chunk); err != nil {
			return err
		}
		// Check if model is confused — detect right after collect sets the flag
		if collect.toolUseAsText {
			fmt.Fprint(os.Stderr, "\n[!]  Model confused, aborting stream...\n")
			cancel()
			return fmt.Errorf("model confused: echoed tool syntax as text")
		}
		return nil
	}, nil)

	stream := a.client.Messages.NewStreaming(ctx, params)
	if err := adapter.Process(stream, cancel); err != nil {
		// Propagate stall/cancel errors with a recognizable prefix
		// so the Run loop's recovery logic can match them.
		errMsg := err.Error()
		if strings.Contains(errMsg, "context canceled") ||
			strings.Contains(errMsg, "context deadline exceeded") ||
			strings.Contains(errMsg, "deadline exceeded") {
			return nil, nil, fmt.Errorf("stream stalled: %w", err)
		}
		return nil, nil, fmt.Errorf("stream error: %w", err)
	}

	// Check if the model was confused and echoed tool syntax as text
	if collect.toolUseAsText {
		fmt.Fprintf(os.Stderr, "\n[!]  Model echoed tool syntax as text -- recovering\n")
		// The model is confused. Clear the confused text so it doesn't pollute context.
		collect.Text = ""
	}

	// Return tool calls and text parts directly, bypassing ContentBlockUnion
	// (which loses text when AsAny() casts back with non-Claude models).
	toolCalls, textParts := collect.AsParsedResponse()
	return toolCalls, textParts, nil
}

func (a *AgentLoop) buildToolParams() []anthropic.ToolUnionParam {
	toolParams := make([]anthropic.ToolUnionParam, 0, len(a.registry.AllTools()))
	for _, t := range a.registry.AllTools() {
		schema := t.InputSchema()
		toolParams = append(toolParams, anthropic.ToolUnionParam{
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
	return toolParams
}

func (a *AgentLoop) parseResponse(response *anthropic.Message) ([]map[string]any, []string) {
	var toolCalls []map[string]any
	var textParts []string

	for _, block := range response.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			textParts = append(textParts, v.Text)
		case anthropic.ToolUseBlock:
			var input map[string]any
			if len(v.Input) > 0 {
				_ = json.Unmarshal(v.Input, &input)
			}
			if input == nil {
				input = make(map[string]any)
			}

			call := map[string]any{
				"id":    v.ID,
				"name":  v.Name,
				"input": input,
			}
			toolCalls = append(toolCalls, call)
		}
		// Thinking blocks (type "thinking") are intentionally ignored —
		// they're internal reasoning, not part of the visible response.
	}

	return toolCalls, textParts
}

func (a *AgentLoop) executeToolCallsConcurrent(toolCalls []map[string]any) {
	var toolResults []anthropic.ToolResultBlockParam

	// Print all tool calls upfront
	for _, call := range toolCalls {
		toolName, _ := call["name"].(string)
		input, _ := call["input"].(map[string]any)
		inputPreview := formatToolArgs(toolName, input)

		if toolName == "exec" {
			fmt.Fprintf(os.Stderr, "  $ %s\n", inputPreview)
		} else {
			fmt.Fprintf(os.Stderr, "  [%s] %s\n", toolName, inputPreview)
		}
	}

	// Pre-check permissions sequentially (avoid concurrent stdin reads in ask mode)
	type toolCallEntry struct {
		call    map[string]any
		index   int
		denied  bool
		errText string
	}
	entries := make([]toolCallEntry, len(toolCalls))
	for i, call := range toolCalls {
		entries[i] = toolCallEntry{call: call, index: i}
		toolName, _ := call["name"].(string)
		input, _ := call["input"].(map[string]any)
		if input == nil {
			input = make(map[string]any)
		}

		// Permission gate check (may read stdin in ask mode)
		tool := a.registry.Get(toolName)
		if tool != nil {
			denial := a.gate.Check(tool, input)
			if denial != nil {
				entries[i].denied = true
				entries[i].errText = denial.Output
			}
		}
	}

	// Execute approved tool calls concurrently
	type jobResult struct {
		index int
		param anthropic.ToolResultBlockParam
		output string
	}
	ch := make(chan jobResult, len(entries))

	for _, e := range entries {
		go func(ent toolCallEntry) {
			if ent.denied {
				ch <- jobResult{
					index: ent.index,
					param: anthropic.ToolResultBlockParam{
						ToolUseID: ent.call["id"].(string),
						Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: ent.errText}}},
						IsError:   param.NewOpt(true),
					},
					output: ent.errText,
				}
				return
			}
			// Skip already-validated gate check in executeSingleTool
			param, output := a.executeSingleToolApproved(ent.call)
			ch <- jobResult{index: ent.index, param: param, output: output}
		}(e)
	}

	// Collect results and sort by original index
	collected := make([]jobResult, 0, len(entries))
	for i := 0; i < len(entries); i++ {
		collected = append(collected, <-ch)
	}
	// Sort by index to preserve original order
	for i := 0; i < len(collected); i++ {
		for j := i + 1; j < len(collected); j++ {
			if collected[j].index < collected[i].index {
				collected[i], collected[j] = collected[j], collected[i]
			}
		}
	}

	// Append results in order
	for _, jr := range collected {
		toolResults = append(toolResults, jr.param)
	}

	a.context.AddToolResults(toolResults)
}

// truncateOutput limits tool output to maxToolChars.
func (a *AgentLoop) truncateOutput(output string) string {
	limit := a.maxToolChars
	if limit <= 0 {
		limit = 8192
	}
	if len(output) <= limit {
		return output
	}
	// Keep first 80% and last 20%
	first := limit * 4 / 5
	last := limit - first
	return output[:first] + "\n\n... [OUTPUT TRUNCATED] ...\n\n" + output[len(output)-last:]
}

// executeSingleTool runs one tool call with timing, truncation, and timeout.
// Returns the ToolResultBlockParam and the output string.
func (a *AgentLoop) executeSingleTool(call map[string]any) (anthropic.ToolResultBlockParam, string) {
	toolUseID, _ := call["id"].(string)
	toolName, _ := call["name"].(string)
	input, _ := call["input"].(map[string]any)
	if input == nil {
		input = make(map[string]any)
	}

	// Record tool use to transcript
	if a.transcript != nil {
		_ = a.transcript.WriteToolUse(toolUseID, toolName, input)
	}

	// Auto-snapshot before write/edit tools
	if toolName == "write_file" || toolName == "edit_file" || toolName == "multi_edit" {
		if path, ok := input["path"].(string); ok && path != "" {
			_ = a.snapshots.TakeSnapshot(path)
		}
	}

	tool := a.registry.Get(toolName)
	if tool == nil {
		msg := "Error: unknown tool '" + toolName + "'"
		return anthropic.ToolResultBlockParam{
			ToolUseID: toolUseID,
			Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: msg}}},
			IsError:   param.NewOpt(true),
		}, msg
	}

	// Validate required parameters
	if err := tools.ValidateParams(tool, input); err != nil {
		msg := "Error: " + err.Error()
		return anthropic.ToolResultBlockParam{
			ToolUseID: toolUseID,
			Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: msg}}},
			IsError:   param.NewOpt(true),
		}, msg
	}

	denial := a.gate.Check(tool, input)
	if denial != nil {
		return anthropic.ToolResultBlockParam{
			ToolUseID: toolUseID,
			Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: denial.Output}}},
			IsError:   param.NewOpt(true),
		}, denial.Output
	}

	// Execute with timeout (mirrors ggbot's executeToolWithStreaming timeout)
	timeout := a.toolTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultCh := make(chan tools.ToolResult, 1)
	start := time.Now()
	go func() {
		resultCh <- tools.ExecuteWithContext(ctx, tool, input)
	}()

	var result tools.ToolResult
	var cancelled bool
	select {
	case result = <-resultCh:
		// Normal completion
	case <-time.After(timeout):
		cancelled = true
		result = tools.ToolResult{
			Output:  fmt.Sprintf("Error: %s timed out after %v", toolName, timeout),
			IsError: true,
		}
	}
	elapsed := time.Since(start)

	// Truncate long outputs
	output := a.truncateOutput(result.Output)

	// Display timing to stderr
	if cancelled {
		fmt.Fprintf(os.Stderr, "  [T]  timed out after %v\n", timeout)
	} else if result.IsError {
		preview := limitStr(output, 150)
		fmt.Fprintf(os.Stderr, "  [x] %s (%v): %s\n", toolName, elapsed.Round(10*time.Millisecond), preview)
	} else {
		preview := toolResultPreview(toolName, output)
		if toolName == "exec" {
			// For exec, just show the result indented, no prefix
			fmt.Fprintf(os.Stderr, "  %s\n", preview)
		} else {
			fmt.Fprintf(os.Stderr, "  [+] %s: %s\n", toolName, preview)
		}
	}

	// Record result to transcript
	if a.transcript != nil {
		_ = a.transcript.WriteToolResult(toolUseID, toolName, output)
	}

	return anthropic.ToolResultBlockParam{
		ToolUseID: toolUseID,
		Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: output}}},
		IsError:   param.NewOpt(result.IsError),
	}, output
}

// executeSingleToolApproved runs one tool call with permission already checked.
// Skips the gate.Check call to avoid concurrent stdin reads in ask mode.
func (a *AgentLoop) executeSingleToolApproved(call map[string]any) (anthropic.ToolResultBlockParam, string) {
	toolUseID, _ := call["id"].(string)
	toolName, _ := call["name"].(string)
	input, _ := call["input"].(map[string]any)
	if input == nil {
		input = make(map[string]any)
	}

	if a.transcript != nil {
		_ = a.transcript.WriteToolUse(toolUseID, toolName, input)
	}

	if toolName == "write_file" || toolName == "edit_file" || toolName == "multi_edit" {
		if path, ok := input["path"].(string); ok && path != "" {
			_ = a.snapshots.TakeSnapshot(path)
		}
	}

	tool := a.registry.Get(toolName)
	if tool == nil {
		msg := "Error: unknown tool '" + toolName + "'"
		return anthropic.ToolResultBlockParam{
			ToolUseID: toolUseID,
			Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: msg}}},
			IsError:   param.NewOpt(true),
		}, msg
	}

	if err := tools.ValidateParams(tool, input); err != nil {
		msg := "Error: " + err.Error()
		return anthropic.ToolResultBlockParam{
			ToolUseID: toolUseID,
			Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: msg}}},
			IsError:   param.NewOpt(true),
		}, msg
	}

	timeout := a.toolTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultCh := make(chan tools.ToolResult, 1)
	start := time.Now()
	go func() {
		resultCh <- tools.ExecuteWithContext(ctx, tool, input)
	}()

	var result tools.ToolResult
	var cancelled bool
	select {
	case result = <-resultCh:
	case <-time.After(timeout):
		cancelled = true
		result = tools.ToolResult{
			Output:  fmt.Sprintf("Error: %s timed out after %v", toolName, timeout),
			IsError: true,
		}
	}
	elapsed := time.Since(start)

	output := a.truncateOutput(result.Output)

	if cancelled {
		fmt.Fprintf(os.Stderr, "  [T]  timed out after %v\n", timeout)
	} else if result.IsError {
		preview := limitStr(output, 150)
		fmt.Fprintf(os.Stderr, "  [x] %s (%v): %s\n", toolName, elapsed.Round(10*time.Millisecond), preview)
	} else {
		preview := toolResultPreview(toolName, output)
		if toolName == "exec" {
			fmt.Fprintf(os.Stderr, "  %s\n", preview)
		} else {
			fmt.Fprintf(os.Stderr, "  [+] %s: %s\n", toolName, preview)
		}
	}

	if a.transcript != nil {
		_ = a.transcript.WriteToolResult(toolUseID, toolName, output)
	}

	return anthropic.ToolResultBlockParam{
		ToolUseID: toolUseID,
		Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: output}}},
		IsError:   param.NewOpt(result.IsError),
	}, output
}

// toolResultPreview extracts the most relevant part of a tool result for display.
func toolResultPreview(toolName, output string) string {
	lines := strings.Split(output, "\n")

	switch toolName {
	case "exec":
		// For exec, show the first line of output, or "(no output)" if empty
		// Skip "STDOUT:" / "STDERR:" headers — just show the actual content
		cleaned := cleanExecOutput(output)
		if cleaned == "" {
			return "(no output)"
		}
		preview := limitStr(cleaned, 120)
		return preview
	case "read_file":
		if len(lines) > 0 && strings.Contains(lines[0], "File:") {
			return lines[0]
		}
	case "write_file", "edit_file", "multi_edit":
		if strings.Contains(output, "/") || strings.Contains(output, "\\") {
			for _, line := range lines {
				if strings.Contains(line, ".") && (strings.Contains(line, "/") || strings.Contains(line, "\\")) {
					return line
				}
			}
		}
	case "list_dir":
		return limitStr(output, 100)
	}

	preview := lines[0]
	if len(preview) > 120 {
		preview = preview[:120] + "..."
	}
	return preview
}

// cleanExecOutput strips STDOUT/STDERR headers and returns the actual content.
func cleanExecOutput(output string) string {
	// Remove "STDOUT:\n" and "STDERR:\n" headers
	cleaned := strings.TrimPrefix(output, "STDOUT:\n")
	cleaned = strings.TrimPrefix(cleaned, "STDERR:\n")
	cleaned = strings.TrimSuffix(cleaned, "\n")

	// If both stdout and stderr are present, prefer stdout
	if strings.HasPrefix(output, "STDOUT:\n") && strings.Contains(output, "\nSTDERR:\n") {
		parts := strings.SplitN(output, "\nSTDERR:\n", 2)
		stdout := strings.TrimSpace(parts[0])
		stderr := strings.TrimSpace(parts[1])
		if stdout != "" {
			return stdout
		}
		return stderr
	}

	return strings.TrimSpace(cleaned)
}

func limitStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// formatToolArgs formats tool input as a compact string, showing file paths prominently.
func formatToolArgs(toolName string, input map[string]any) string {
	// Show the most relevant arg for each tool type
	switch toolName {
	case "read_file", "write_file", "edit_file", "list_dir":
		if path, ok := input["path"].(string); ok {
			return path
		}
	case "exec":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 120 {
				return cmd[:120] + "..."
			}
			return cmd
		}
	case "grep":
		if pattern, ok := input["pattern"].(string); ok {
			if path, ok2 := input["path"].(string); ok2 {
				return fmt.Sprintf("%q in %s", pattern, path)
			}
			return fmt.Sprintf("%q", pattern)
		}
	case "glob":
		if pattern, ok := input["pattern"].(string); ok {
			return pattern
		}
	}
	// Fallback: format all args compactly
	parts := make([]string, 0, len(input))
	for k, v := range input {
		s := fmt.Sprintf("%v", v)
		if len(s) > 80 {
			s = s[:80] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	return strings.Join(parts, " ")
}

// Helper: extract required field names from input schema
func getStringSlice(schema map[string]any, key string) []string {
	if v, ok := schema[key]; ok {
		if arr, ok := v.([]string); ok {
			return arr
		}
		if iface, ok := v.([]any); ok {
			out := make([]string, len(iface))
			for i, e := range iface {
				out[i], _ = e.(string)
			}
			return out
		}
	}
	return nil
}

// isContextLengthError checks if the error is a context window overflow.
func isContextLengthError(errMsg string) bool {
	if errMsg == "" {
		return false
	}
	err := strings.ToLower(errMsg)
	patterns := []string{
		"context_length", "maximum context", "too many tokens",
		"prompt_too_long", "token limit", "context_exceeded",
		"max_tokens_exceeded", "context window", "context limit",
	}
	for _, p := range patterns {
		if strings.Contains(err, p) {
			return true
		}
	}
	return false
}

// CollectHandler.AsMessageContent reconstructs ContentBlockUnion slices from collected data.
func (h *CollectHandler) AsMessageContent() []anthropic.ContentBlockUnion {
	h.mu.Lock()
	defer h.mu.Unlock()

	var content []anthropic.ContentBlockUnion

	// Use text if available, otherwise fall back to thinking (some models
	// return only thinking blocks when no tools are needed).
	textContent := h.Text
	if textContent == "" {
		textContent = h.Thinking
	}
	if textContent != "" {
		content = append(content, anthropic.ContentBlockUnion{
			Type: "text",
			Text: textContent,
		})
	}
	for _, tc := range h.ToolCalls {
		var input json.RawMessage
		if tc.Arguments != "" {
			input = json.RawMessage(tc.Arguments)
		} else {
			input = json.RawMessage("{}")
		}
		content = append(content, anthropic.ContentBlockUnion{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		})
	}
	return content
}

// StreamAdapter.Process takes a *ssestream.Stream directly
func (sa *StreamAdapter) ProcessStream(stream *ssestream.Stream[anthropic.MessageStreamEventUnion]) error {
	return sa.Process(stream, nil)
}
