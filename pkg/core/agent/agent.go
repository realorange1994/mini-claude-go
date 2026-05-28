// Package agent provides the core agent session and runtime.
// Mirrors pi's core/agent-session.ts and core/agent-session-runtime.ts.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"miniclaudecode-go/pkg/core/compaction"
	"miniclaudecode-go/pkg/core/eventbus"
	"miniclaudecode-go/pkg/core/extensions"
	"miniclaudecode-go/pkg/core/session"
	"miniclaudecode-go/pkg/core/shellexec"
	"miniclaudecode-go/pkg/core/tools"
)

// AgentConfig holds agent configuration.
type AgentConfig struct {
	Model        string
	Cwd          string
	SessionId    string
	SessionPath  string
	MaxTurns     int
	AutoCompact  bool
	CompactAfter int // turns before auto-compact
	StreamOutput bool
}

// LLMClient is the interface for LLM API calls.
// Implement this to connect to different model providers.
type LLMClient interface {
	// Complete sends messages to the model and returns the response.
	Complete(ctx context.Context, model string, messages []map[string]interface{}, tools []extensions.ToolDefinition) (string, error)
	// CompleteStreaming is like Complete but streams the response.
	CompleteStreaming(ctx context.Context, model string, messages []map[string]interface{}, tools []extensions.ToolDefinition, onChunk func(string)) error
}

// AgentSession is the main agent session (mirrors pi's AgentSession).
type AgentSession struct {
	config      AgentConfig
	session     *session.SessionManager
	tools       *tools.Registry
	eventRunner *extensions.ExtensionRunner
	compactor   *compaction.Compactor
	executor    *shellexec.Executor
	llmClient   LLMClient

	// Message state
	messages  []extensions.Message
	turnCount int
	mu        sync.RWMutex

	// Streaming callback
	streamCb func(text string)

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// AgentSessionRuntime manages session lifecycle (mirrors pi's AgentSessionRuntime).
type AgentSessionRuntime struct {
	config      AgentConfig
	session     *session.SessionManager
	activeAgent *AgentSession
}

// NewAgentSessionRuntime creates a new session runtime.
func NewAgentSessionRuntime(config AgentConfig) (*AgentSessionRuntime, error) {
	sm, err := session.NewSessionManager(config.SessionPath)
	if err != nil {
		return nil, fmt.Errorf("create session manager: %w", err)
	}
	return &AgentSessionRuntime{
		config:  config,
		session: sm,
	}, nil
}

// NewSession creates a new agent session.
func (r *AgentSessionRuntime) NewSession(model, cwd string) (*AgentSession, error) {
	sess, err := r.session.NewSession(model, cwd)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	comp := compaction.NewCompactor(model, nil)
	exec := shellexec.New()

	agent := &AgentSession{
		config:    r.config,
		session:   r.session,
		tools:     tools.DefaultTools(),
		compactor: comp,
		executor:  exec,
		turnCount: 0,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Setup event runner
	eb := eventbus.New()
	extAPI := &extensions.ExtensionAPI{
		GetSessionId:   func() string { return sess.Id },
		GetSessionPath: func() string { return sess.Path },
		GetMessages: func() []extensions.Message {
			agent.mu.RLock()
			defer agent.mu.RUnlock()
			return agent.toTypedMessages()
		},
		AddMessage: func(msg extensions.Message) {
			agent.addMessage(msg)
		},
		GetAvailableTools: func() []extensions.ToolDefinition {
			return agent.tools.GetDefinitions()
		},
		GetModel: func() string { return agent.config.Model },
		SetModel: func(model string) { agent.config.Model = model },
		Compact:  func() error { return agent.Compact() },
	}

	extCtx := &extensions.ExtensionContext{
		Events: eb,
		API:    extAPI,
	}

	agent.eventRunner = extensions.NewExtensionRunner(extCtx)
	r.activeAgent = agent
	return agent, nil
}

// Fork creates a forked session (mirrors pi's fork).
func (r *AgentSessionRuntime) Fork(parentAgent *AgentSession, message string) (*AgentSession, error) {
	_, err := r.session.Fork(parentAgent.getSessionId(), message)
	if err != nil {
		return nil, fmt.Errorf("fork session: %w", err)
	}

	agent := &AgentSession{
		config:    parentAgent.config,
		session:   r.session,
		tools:     tools.DefaultTools(),
		compactor: parentAgent.compactor,
		executor:  parentAgent.executor,
		turnCount: 0,
		ctx:       context.Background(),
		cancel:    func() {},
	}

	// Setup fresh event runner for forked session
	eb := eventbus.New()
	sess := r.session.GetActiveSession()
	extAPI := &extensions.ExtensionAPI{
		GetSessionId:   func() string { return sess.Id },
		GetSessionPath: func() string { return sess.Path },
		GetMessages: func() []extensions.Message {
			agent.mu.RLock()
			defer agent.mu.RUnlock()
			return agent.toTypedMessages()
		},
		AddMessage:        func(msg extensions.Message) { agent.addMessage(msg) },
		GetAvailableTools: func() []extensions.ToolDefinition { return agent.tools.GetDefinitions() },
		GetModel:          func() string { return agent.config.Model },
		SetModel:          func(model string) { agent.config.Model = model },
		Compact:           func() error { return agent.Compact() },
	}
	extCtx := &extensions.ExtensionContext{
		Events: eb,
		API:    extAPI,
	}
	agent.eventRunner = extensions.NewExtensionRunner(extCtx)

	r.activeAgent = agent
	return agent, nil
}

// SwitchSession switches to a different session.
func (r *AgentSessionRuntime) SwitchSession(sessionId string) error {
	return r.session.SwitchSession(sessionId)
}

// Dispose cleans up a session.
func (r *AgentSessionRuntime) Dispose(agent *AgentSession) {
	if agent.cancel != nil {
		agent.cancel()
	}
}

// --- AgentSession methods ---

// getSessionId returns the session ID.
func (s *AgentSession) getSessionId() string {
	return s.config.SessionId
}

func (s *AgentSession) addMessage(msg extensions.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
}

// toTypedMessages returns a copy of the messages slice.
func (s *AgentSession) toTypedMessages() []extensions.Message {
	return append([]extensions.Message(nil), s.messages...)
}

// Run starts the agent loop with the given initial message.
func (s *AgentSession) Run(initialMessage string) error {
	s.mu.Lock()
	if s.config.MaxTurns == 0 {
		s.config.MaxTurns = 100
	}
	if s.config.CompactAfter == 0 {
		s.config.CompactAfter = 50
	}
	s.mu.Unlock()

	sess := s.session.GetActiveSession()
	sid := sess.Id
	s.eventRunner.EmitSessionStart(sid, sess.Path, s.config.Model)

	s.addMessage(extensions.Message{
		Role: extensions.RoleUser,
		Content: []extensions.ContentBlock{
			extensions.TextContentBlock(initialMessage),
		},
	})

	return s.runLoop()
}

// runLoop is the main turn loop.
func (s *AgentSession) runLoop() error {
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		done, err := s.runTurn()
		if err != nil {
			sid := s.session.GetActiveSession().Id
			s.eventRunner.EmitError(sid, "agent", err.Error())
			return err
		}
		if done {
			return nil
		}
	}
}

// runTurn executes a single turn and returns (stop, error).
func (s *AgentSession) runTurn() (bool, error) {
	s.mu.Lock()
	// Check max-turns before executing the turn
	if s.turnCount >= s.config.MaxTurns {
		s.mu.Unlock()
		return true, nil
	}
	s.turnCount++
	turnNum := s.turnCount
	msgs := make([]extensions.Message, len(s.messages))
	copy(msgs, s.messages)
	s.mu.Unlock()

	sid := s.session.GetActiveSession().Id

	s.eventRunner.EmitTurnStart(sid, turnNum)

	// Build API messages
	apiMsgs := s.buildMessages(msgs)

	// Emit before agent start
	s.eventRunner.EmitBeforeAgentStart(sid, s.config.Model, turnNum)

	// Call LLM
	response, err := s.callLLM(apiMsgs)
	if err != nil {
		return false, err
	}

	// Process response (tool calls or text)
	stop, err := s.processResponse(response)
	if err != nil {
		return false, err
	}

	s.eventRunner.EmitAgentEnd(sid, s.config.Model, turnNum, "complete", extensions.TokenUsage{}, 0, 0)
	s.eventRunner.EmitTurnEnd(sid, turnNum, extensions.TokenUsage{}, 0, 0)

	// Auto-compact check
	s.mu.Lock()
	shouldCompact := s.config.AutoCompact && s.turnCount >= s.config.CompactAfter
	s.mu.Unlock()

	if shouldCompact {
		if err := s.Compact(); err != nil {
			// Log but do not fail the turn
			fmt.Printf("[agent] compact error: %v\n", err)
		}
		s.mu.Lock()
		s.turnCount = 0
		s.mu.Unlock()
	}

	return stop, nil
}

func (s *AgentSession) copyMessages() []extensions.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := make([]extensions.Message, len(s.messages))
	copy(msgs, s.messages)
	return msgs
}

func (s *AgentSession) buildMessages(msgs []extensions.Message) []map[string]interface{} {
	apiMsgs := make([]map[string]interface{}, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Role == "tool" {
			// Anthropic API: tool_result must be a "user" message with a content block
			// of type "tool_result". The content of tool_result must be an array.
			toolUseID := ""
			var resultTexts []string
			for _, block := range msg.Content {
				if block.ToolUseID != "" {
					toolUseID = block.ToolUseID
				}
				if block.Type == "tool_result" {
					text := block.Text
					if block.IsError {
						text = "Error: " + text
					}
					resultTexts = append(resultTexts, text)
				}
			}
			contentBlock := map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": toolUseID,
				"content": []map[string]interface{}{
					{"type": "text", "text": strings.Join(resultTexts, "\n")},
				},
			}
			m := map[string]interface{}{
				"role":    "user",
				"content": []map[string]interface{}{contentBlock},
			}
			apiMsgs = append(apiMsgs, m)
		} else if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
			// Simple text message
			m := map[string]interface{}{
				"role":    string(msg.Role),
				"content": msg.Content[0].Text,
			}
			apiMsgs = append(apiMsgs, m)
		} else {
			// Tool use or other complex content
			contentBlocks := make([]map[string]interface{}, 0, len(msg.Content))
			for _, block := range msg.Content {
				b := map[string]interface{}{
					"type": block.Type,
				}
				if block.Text != "" {
					b["text"] = block.Text
				}
				if block.ToolUseID != "" {
					b["id"] = block.ToolUseID
				}
				if block.ToolName != "" {
					b["name"] = block.ToolName
				}
				if block.ToolInput != nil {
					b["input"] = block.ToolInput
				}
				contentBlocks = append(contentBlocks, b)
			}
			m := map[string]interface{}{
				"role":    string(msg.Role),
				"content": contentBlocks,
			}
			apiMsgs = append(apiMsgs, m)
		}
	}
	return apiMsgs
}

// callLLM invokes the language model using the configured LLM client.
func (s *AgentSession) callLLM(messages []map[string]interface{}) (string, error) {
	s.mu.RLock()
	client := s.llmClient
	model := s.config.Model
	toolDefs := s.tools.GetDefinitions()
	s.mu.RUnlock()

	if client == nil {
		return "", fmt.Errorf("LLM client not configured: call SetLLMClient first")
	}

	// Call the LLM
	response, err := client.Complete(s.ctx, model, messages, toolDefs)
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return response, nil
}

// callLLMStreaming invokes the LLM with streaming output.
func (s *AgentSession) callLLMStreaming(messages []map[string]interface{}) error {
	s.mu.RLock()
	client := s.llmClient
	model := s.config.Model
	toolDefs := s.tools.GetDefinitions()
	streamCb := s.streamCb
	s.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("LLM client not configured")
	}

	if streamCb == nil {
		// If no callback, just use non-streaming
		_, err := client.Complete(s.ctx, model, messages, toolDefs)
		return err
	}

	return client.CompleteStreaming(s.ctx, model, messages, toolDefs, streamCb)
}

// processResponse handles an LLM response (tool calls or text).
// Returns true to stop the loop, false to continue.
func (s *AgentSession) processResponse(response string) (bool, error) {
	// Try to parse as Anthropic API response with content blocks
	var apiResp struct {
		Content []struct {
			Type  string                 `json:"type"`
			ID    string                 `json:"id"`
			Name  string                 `json:"name"`
			Text  string                 `json:"text"`
			Input map[string]interface{} `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}

	var textParts []string
	var toolCalls []toolCall

	if err := json.Unmarshal([]byte(response), &apiResp); err != nil {
			}
	for _, block := range apiResp.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case "tool_use":
			id := block.ID
			if id == "" {
				id = generateToolCallId()
			}
			toolCalls = append(toolCalls, toolCall{
				id:    id,
				name:  block.Name,
				input: block.Input,
			})
		}
	}

	// Execute tool calls if any
	if len(toolCalls) > 0 {
		// Build content blocks for the assistant message (all tool_use blocks)
		contentBlocks := make([]extensions.ContentBlock, 0, len(toolCalls))
		for _, tc := range toolCalls {
			contentBlocks = append(contentBlocks, extensions.ContentBlock{
				Type:      "tool_use",
				ToolUseID: tc.id,
				ToolName:  tc.name,
				ToolInput: tc.input,
			})
		}
		s.addMessage(extensions.Message{
			Role:    extensions.RoleAssistant,
			Content: contentBlocks,
		})

		// Execute tools and get results
		toolResults, err := s.executeTools(toolCalls)
		if err != nil {
			return false, err
		}

		// Add tool results to conversation
		for _, result := range toolResults {
			s.addMessage(extensions.Message{
				Role: extensions.RoleTool,
				Content: []extensions.ContentBlock{
					{
						Type:      "tool_result",
						ToolUseID: result.toolCallId,
						Text:      result.content,
						IsError:   result.isError,
					},
				},
			})
		}

		// Continue the loop to process the tool results
		return false, nil
	}

	// No tool calls — text-only response, print and stop
	assistantText := strings.Join(textParts, "\n")
	if assistantText == "" {
		// Fallback: treat raw response as text
		assistantText = response
	}

	s.addMessage(extensions.Message{
		Role: extensions.RoleAssistant,
		Content: []extensions.ContentBlock{
			extensions.TextContentBlock(assistantText),
		},
	})

	return true, nil
}

// toolCall represents a parsed tool call from LLM response
type toolCall struct {
	id    string
	name  string
	input map[string]interface{}
}

// toolResult represents the result of a tool execution
type toolResult struct {
	toolCallId string
	content    string
	isError    bool
}

// parseToolCalls extracts tool use blocks from the LLM response.
// Returns nil if no tool calls are found.
func (s *AgentSession) parseToolCalls(response string) ([]toolCall, bool) {
	// Try to find tool_use blocks in the response
	// Parse as JSON to find tool calls
	var responseData struct {
		Content []struct {
			Type      string                 `json:"type"`
			ID        string                 `json:"id"`
			Name      string                 `json:"name"`
			Input     map[string]interface{} `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}

	if err := json.Unmarshal([]byte(response), &responseData); err == nil {
		var results []toolCall
		for _, block := range responseData.Content {
			if block.Type == "tool_use" && block.Name != "" {
				id := block.ID
				if id == "" {
					id = generateToolCallId()
				}
				results = append(results, toolCall{
					id:    id,
					name:  block.Name,
					input: block.Input,
				})
			}
		}
		if len(results) > 0 {
			return results, true
		}
	}

	// Try parsing as a JSON array of tool calls
	var toolCallArray []struct {
		Type  string                 `json:"type"`
		ID    string                 `json:"id"`
		Name  string                 `json:"name"`
		Input map[string]interface{} `json:"input"`
	}
	if err := json.Unmarshal([]byte(response), &toolCallArray); err == nil {
		var results []toolCall
		for _, tc := range toolCallArray {
			if tc.Name != "" {
				id := tc.ID
				if id == "" {
					id = generateToolCallId()
				}
				results = append(results, toolCall{
					id:    id,
					name:  tc.Name,
					input: tc.Input,
				})
			}
		}
		if len(results) > 0 {
			return results, true
		}
	}

	return nil, false
}

// executeTools runs tool calls and returns their results.
func (s *AgentSession) executeTools(toolCalls []toolCall) ([]toolResult, error) {
	results := make([]toolResult, 0, len(toolCalls))

	for _, tc := range toolCalls {
		// Execute the tool using the registry
		result, err := s.tools.Execute(tc.name, tc.input)
		if err != nil {
			results = append(results, toolResult{
				toolCallId: tc.id,
				content:    fmt.Sprintf("Error: %v", err),
				isError:    true,
			})
			continue
		}

		results = append(results, toolResult{
			toolCallId: tc.id,
			content:    result,
			isError:    false,
		})
	}

	return results, nil
}

// Compact performs context compaction (mirrors pi's compact()).
func (s *AgentSession) Compact() error {
	sid := s.session.GetActiveSession().Id
	s.eventRunner.EmitSessionBeforeCompact(sid, len(s.messages), extensions.TokenUsage{})

	s.mu.Lock()
	msgs := s.copyMessages()
	s.mu.Unlock()

	// Convert messages to strings for the compactor.
	msgStrs := s.toStrings(msgs)

	// Reduce messages to fit within token budget.
	reduced, _, err := s.compactor.CompactMessages(msgStrs, s.config.CompactAfter)
	if err != nil {
		return fmt.Errorf("compact messages: %w", err)
	}

	// Generate a summary of the compacted branch.
	summarizer := compaction.NewBranchSummarizer(s.config.Model)
	summary, err := summarizer.Summarize(s.config.SessionId, "main", reduced, s.config.Model)
	if err != nil {
		summary = fmt.Sprintf("Compacted %d messages", len(reduced))
	}

	// Build the new message list: a compaction marker + the reduced messages.
	s.mu.Lock()
	s.messages = s.buildCompactedMessages(reduced, summary)
	s.mu.Unlock()

	s.eventRunner.EmitSessionCompact(sid, len(s.messages), len(s.messages), 0)
	return nil
}

func (s *AgentSession) toStrings(msgs []extensions.Message) []string {
	strs := make([]string, len(msgs))
	for i, msg := range msgs {
		for _, block := range msg.Content {
			if block.Text != "" {
				strs[i] = block.Text
				break
			}
		}
	}
	return strs
}

func (s *AgentSession) buildCompactedMessages(reducedMsgs []string, summary string) []extensions.Message {
	result := []extensions.Message{
		{
			Role: extensions.RoleSystem,
			Content: []extensions.ContentBlock{
				{Type: "text", Text: "[compaction] " + summary},
			},
		},
	}
	for _, msg := range reducedMsgs {
		result = append(result, extensions.Message{
			Role: extensions.RoleUser,
			Content: []extensions.ContentBlock{
				extensions.TextContentBlock(msg),
			},
		})
	}
	return result
}

// SetLLMClient sets the LLM client for the agent session.
func (s *AgentSession) SetLLMClient(client LLMClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.llmClient = client
}

// SetStreamCallback sets the streaming output callback.
func (s *AgentSession) SetStreamCallback(cb func(text string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamCb = cb
}

// SetTools sets a custom tool registry.
func (s *AgentSession) SetTools(reg *tools.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = reg
}

// GetLastMessage returns the last assistant message text.
func (s *AgentSession) GetLastMessage() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.messages) - 1; i >= 0; i-- {
		msg := s.messages[i]
		for _, block := range msg.Content {
			if block.Text != "" && msg.Role == "assistant" {
				return block.Text
			}
		}
	}
	return ""
}

// GetTurnCount returns the current turn count.
func (s *AgentSession) GetTurnCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turnCount
}

// generateToolCallId generates a unique ID for tool calls.
func generateToolCallId() string {
	// Simple ID generation - in production use UUID
	return fmt.Sprintf("tool_%d", time.Now().UnixNano())
}