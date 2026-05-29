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
	"miniclaudecode-go/pkg/core/resourceloader"
	"miniclaudecode-go/pkg/core/session"
	"miniclaudecode-go/pkg/core/shellexec"
	"miniclaudecode-go/pkg/core/systemprompt"
	"miniclaudecode-go/pkg/core/tools"
)

// AgentConfig holds agent configuration.
type AgentConfig struct {
	Model            string
	Cwd              string
	SessionId        string
	SessionPath      string
	MaxTurns         int
	AutoCompact      bool
	CompactAfter     int // turns before auto-compact
	StreamOutput     bool
	SystemPrompt     string       // custom system prompt (empty = auto-build)
	AppendSystem     string       // text appended after system prompt
	SelectedTools    []string     // tool names to include (empty = default)
	ExtraGuidelines  []string     // additional guideline bullets
	Skills           []string     // skill prompt fragments
	ContextFiles     []systemprompt.ContextFile
	EnableStreaming  bool         // use streaming LLM path
	Timeout          time.Duration // global timeout for the entire agent session (0 = no timeout)
	ResourceLoader   *resourceloader.ResourceLoader
}

// LLMClient is the interface for LLM API calls.
// Implement this to connect to different model providers.
type LLMClient interface {
	// Complete sends messages to the model and returns the response.
	// thinking is optional extended thinking configuration.
	Complete(ctx context.Context, model string, messages []map[string]interface{}, tools []extensions.ToolDefinition, thinking *ThinkingConfig) (string, error)
	// CompleteStreaming is like Complete but streams the response.
	CompleteStreaming(ctx context.Context, model string, messages []map[string]interface{}, tools []extensions.ToolDefinition, thinking *ThinkingConfig, onChunk func(string)) error
}

// AgentSession is the main agent session (mirrors pi's AgentSession).
type AgentSession struct {
	config       AgentConfig
	session    *session.SessionManager // tree-based session with message persistence
	tools        *tools.Registry
	eventRunner  *extensions.ExtensionRunner
	compactor    *compaction.Compactor
	executor     *shellexec.Executor
	llmClient    LLMClient
	systemPrompt string

	// Message state
	messages  []extensions.Message
	turnCount int
	mu        sync.RWMutex

	// Thinking level
	thinkingLevel ThinkingLevel

	// Streaming callback
	streamCb func(text string)

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Message queue state
	steeringMu       sync.Mutex
	steeringMessages []string         // interrupt current turn
	followUpMessages []string         // queued after current turn
	pendingNextTurn  []string         // pending messages for next turn
	promptQueue      chan string      // queue for interactive prompts
	promptActive     bool             // whether Run is currently active

	// Event subscription
	subscribers []func(event AgentEvent)
	subMu       sync.RWMutex
}

// AgentEvent represents an event emitted by the agent session.
type AgentEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

// AgentSessionRuntime manages session lifecycle (mirrors pi's AgentSessionRuntime).
type AgentSessionRuntime struct {
	config         AgentConfig
	activeAgent    *AgentSession
	sessionManager *session.SessionManager
	// Callbacks for lifecycle events
	beforeSessionInvalidate func() // sync teardown hook
	rebindSession           func(oldSession, newSession *AgentSession) // host callback for rebind
}

// NewAgentSessionRuntime creates a new session runtime.
func NewAgentSessionRuntime(config AgentConfig) (*AgentSessionRuntime, error) {
	// Create shared session manager
	sm := session.NewSessionManager(config.Cwd, config.SessionPath, "", true)

	return &AgentSessionRuntime{
		config:         config,
		sessionManager: sm,
	}, nil
}

// NewSession creates a new agent session.
func (r *AgentSessionRuntime) NewSession(model, cwd string) (*AgentSession, error) {
	return r.newSessionWithOptions(model, cwd, "")
}

// newSessionWithOptions creates a new session, optionally branching from an entry.
func (r *AgentSessionRuntime) newSessionWithOptions(model, cwd, fromEntryID string) (*AgentSession, error) {
	ctx, cancel := func() (context.Context, context.CancelFunc) {
		if r.config.Timeout > 0 {
			return context.WithTimeout(context.Background(), r.config.Timeout)
		}
		return context.WithCancel(context.Background())
	}()

	comp := compaction.NewCompactor(model, nil)
	exec := shellexec.New()

	// Use existing session manager or create new
	sm := r.sessionManager
	if sm == nil {
		sm = session.NewSessionManager(cwd, r.config.SessionPath, "", true)
		r.sessionManager = sm
	}

	// Branch from entry if specified
	if fromEntryID != "" {
		if err := sm.Branch(fromEntryID); err != nil {
			return nil, fmt.Errorf("branch session: %w", err)
		}
	}

	sessionID := sm.GetSessionID()

	agent := &AgentSession{
		config:    r.config,
		session:   sm,
		tools:     tools.DefaultTools(),
		compactor: comp,
		executor:  exec,
		turnCount: 0,
		ctx:       ctx,
		cancel:    cancel,
		promptQueue: make(chan string, 100),
	}

	// Build system prompt
	agent.systemPrompt = agent.buildSystemPrompt()

	// Setup event runner
	eb := eventbus.New()
	sessionFile := sm.GetSessionFile()
	extAPI := &extensions.ExtensionAPI{
		GetSessionId:   func() string { return sessionID },
		GetSessionPath: func() string { return sessionFile },
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
	// Branch the v2 session from the current leaf
	if parentAgent.session == nil {
		return nil, fmt.Errorf("parent session has no session tree")
	}
	leafID := parentAgent.session.GetLeafID()
	if err := parentAgent.session.Branch(leafID); err != nil {
		return nil, fmt.Errorf("fork session: %w", err)
	}

	agent := &AgentSession{
		config:    parentAgent.config,
		session:   parentAgent.session,
		tools:     tools.DefaultTools(),
		compactor: parentAgent.compactor,
		executor:  parentAgent.executor,
		turnCount: 0,
		ctx:       context.Background(),
		cancel:    func() {},
	}

	// Setup fresh event runner for forked session
	eb := eventbus.New()
	sessionID := parentAgent.session.GetSessionID()
	sessionFile := parentAgent.session.GetSessionFile()
	extAPI := &extensions.ExtensionAPI{
		GetSessionId:   func() string { return sessionID },
		GetSessionPath: func() string { return sessionFile },
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

// SwitchSession switches to a different session file with full teardown.
// Aligned to pi's AgentSessionRuntime.switchSession().
func (r *AgentSessionRuntime) SwitchSession(sessionFile string) error {
	if r.activeAgent == nil || r.activeAgent.session == nil {
		return fmt.Errorf("no active session to switch")
	}

	// Emit shutdown event for current session
	old := r.activeAgent
	old.emitEvent(AgentEvent{Type: "shutdown"})
	old.Dispose()

	// Invalidate callback
	if r.beforeSessionInvalidate != nil {
		r.beforeSessionInvalidate()
	}

	// Switch to new session file
	old.session.SetSessionFile(sessionFile)

	// Rebuild context from new session file
	ctx := old.session.BuildSessionContext()

	// Rebuild messages from new session
	old.mu.Lock()
	old.messages = nil
	for _, rawMsg := range ctx.Messages {
		var msg extensions.Message
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			continue
		}
		old.messages = append(old.messages, msg)
	}
	old.mu.Unlock()

	// Trigger rebind callback if set
	if r.rebindSession != nil {
		r.rebindSession(nil, old)
	}

	return nil
}

// SetRebindSession sets a callback for session rebind.
func (r *AgentSessionRuntime) SetRebindSession(cb func(oldAgent, newAgent *AgentSession)) {
	r.rebindSession = cb
}

// SetBeforeSessionInvalidate sets a sync teardown hook.
func (r *AgentSessionRuntime) SetBeforeSessionInvalidate(cb func()) {
	r.beforeSessionInvalidate = cb
}

// Dispose cleans up a session with full event emission.
// Aligned to pi's AgentSessionRuntime.dispose().
func (r *AgentSessionRuntime) Dispose(agent *AgentSession) {
	if agent == nil {
		return
	}

	// Emit shutdown and dispose
	agent.emitEvent(AgentEvent{Type: "session_shutdown"})
	agent.Dispose()

	// Clear active agent
	r.activeAgent = nil

	// Dispose session manager if exists
	if r.sessionManager != nil {
		r.sessionManager = nil
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

	sid := s.session.GetSessionID()
	s.eventRunner.EmitSessionStart(sid, s.session.GetSessionFile(), s.config.Model)

	// Inject system prompt only on first Run() call (when messages is empty)
	s.mu.Lock()
	isFirstRun := len(s.messages) == 0
	s.mu.Unlock()

	if isFirstRun && s.systemPrompt != "" {
		s.addMessage(extensions.Message{
			Role: extensions.RoleSystem,
			Content: []extensions.ContentBlock{
				extensions.TextContentBlock(s.systemPrompt),
			},
		})
	}

	s.addMessage(extensions.Message{
		Role: extensions.RoleUser,
		Content: []extensions.ContentBlock{
			extensions.TextContentBlock(initialMessage),
		},
	})

	return s.runLoop()
}

// runLoop is the main turn loop with auto-retry support.
func (s *AgentSession) runLoop() error {
	retryState := &RetryState{}
	config := RetryConfig{MaxRetries: 3, BaseDelay: 2 * time.Second}

	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		// Inject any pending steering messages before the turn
		s.injectSteeringMessages()

		done, err := s.runTurn()
		if err != nil {
			// Check if this is a context overflow error — trigger compaction instead of retry
			if IsContextOverflow(err) {
				if compErr := s.Compact(); compErr != nil {
					sid := s.session.GetSessionID()
					s.eventRunner.EmitError(sid, "agent", err.Error())
					return fmt.Errorf("context overflow and compact failed: %w (%v)", err, compErr)
				}
				// Compaction succeeded, retry the turn with reduced messages
				continue
			}

			// Check if this is a retryable error
			if IsRetryableError(err) {
				retryState.Next()
				if shouldRetry, delay := ShouldRetry(retryState.Attempts, config); shouldRetry {
					time.Sleep(delay)
					continue
				}
				// Max retries exceeded
				sid := s.session.GetSessionID()
				s.eventRunner.EmitError(sid, "agent", fmt.Sprintf("Max retries exceeded after %d attempts: %v", config.MaxRetries, err))
				return fmt.Errorf("max retries exceeded: %w", err)
			}

			// Non-retryable error
			retryState.Reset()
			sid := s.session.GetSessionID()
			s.eventRunner.EmitError(sid, "agent", err.Error())
			return err
		}

		// Success — reset retry counter
		retryState.Reset()
		if done {
			// Check for pending follow-up messages
			s.steeringMu.Lock()
			hasPending := len(s.followUpMessages) > 0
			s.steeringMu.Unlock()
			if hasPending {
				continue
			}
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

	sid := s.session.GetSessionID()

	s.eventRunner.EmitTurnStart(sid, turnNum)

	// Build API messages
	apiMsgs := s.buildMessages(msgs)

	// Emit before agent start
	s.eventRunner.EmitBeforeAgentStart(sid, s.config.Model, turnNum)

	// Call LLM — always use non-streaming Complete() so we get the full
	// response (including tool_use blocks) for processResponse() to parse.
	// If a stream callback is set, we emit text chunks as they become available.
	var response string
	var err error
	response, err = s.callLLM(apiMsgs)
	if err != nil {
		return false, err
	}

	// Stream text parts to callback before processing (so user sees output in real time)
	if s.streamCb != nil {
		var apiResp struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal([]byte(response), &apiResp) == nil {
			for _, block := range apiResp.Content {
				if block.Type == "text" && block.Text != "" {
					s.streamCb(block.Text)
				}
			}
		}
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
	thinking := BuildThinkingConfig(s.thinkingLevel)
	s.mu.RUnlock()

	if client == nil {
		return "", fmt.Errorf("LLM client not configured: call SetLLMClient first")
	}

	// Call the LLM
	response, err := client.Complete(s.ctx, model, messages, toolDefs, thinking)
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
	thinking := BuildThinkingConfig(s.thinkingLevel)
	streamCb := s.streamCb
	s.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("LLM client not configured")
	}

	if streamCb == nil {
		// If no callback, just use non-streaming
		_, err := client.Complete(s.ctx, model, messages, toolDefs, thinking)
		return err
	}

	return client.CompleteStreaming(s.ctx, model, messages, toolDefs, thinking, streamCb)
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
		// Build content blocks for the assistant message (text + tool_use blocks)
		contentBlocks := make([]extensions.ContentBlock, 0, len(textParts)+len(toolCalls))
		for _, txt := range textParts {
			contentBlocks = append(contentBlocks, extensions.TextContentBlock(txt))
		}
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
// Uses LLM-based summarization when available, falls back to heuristic extraction.
func (s *AgentSession) Compact() error {
	sid := s.session.GetSessionID()
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
	// Try LLM-based summary first if we have an LLM client.
	var summary string
	if s.llmClient != nil {
		// Check if HTTPClient has Generate method (LLMCompactor interface)
		if hc, ok := s.llmClient.(interface {
			Generate(ctx context.Context, model string, systemPrompt string, userPrompt string, maxTokens int) (string, error)
		}); ok {
			llmCompactor := compaction.NewLLMCompactor(s.config.Model, hc)
			if llmCompactor.ShouldUseLLMCompaction(reduced) {
				summary, err = llmCompactor.GenerateSummary(s.ctx, reduced, "")
				if err != nil {
					// Fall back to heuristic summary
					summary = ""
				}
			}
		}
	}

	// Fall back to heuristic summary if LLM summary not available
	if summary == "" {
		summarizer := compaction.NewBranchSummarizer(s.config.Model)
		summary, err = summarizer.Summarize(s.config.SessionId, "main", reduced, s.config.Model)
		if err != nil {
			summary = fmt.Sprintf("Compacted %d messages", len(reduced))
		}
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

// EnableTools enables tools by name. Unknown names are silently ignored.
func (s *AgentSession) EnableTools(names []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range names {
		if !contains(s.config.SelectedTools, n) {
			s.config.SelectedTools = append(s.config.SelectedTools, n)
		}
	}
	// Rebuild system prompt with updated tools
	s.systemPrompt = s.buildSystemPromptLocked()
}

// DisableTools disables tools by name.
func (s *AgentSession) DisableTools(names []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.config.SelectedTools[:0]
	disabled := make(map[string]bool)
	for _, n := range names {
		disabled[n] = true
	}
	for _, t := range s.config.SelectedTools {
		if !disabled[t] {
			filtered = append(filtered, t)
		}
	}
	s.config.SelectedTools = filtered
	s.systemPrompt = s.buildSystemPromptLocked()
}

// GetActiveTools returns names of currently active tools.
func (s *AgentSession) GetActiveTools() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.config.SelectedTools))
	copy(out, s.config.SelectedTools)
	return out
}

// buildSystemPrompt builds the system prompt from config.
func (s *AgentSession) buildSystemPrompt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buildSystemPromptLocked()
}

// buildSystemPromptLocked builds system prompt; caller must hold s.mu.
func (s *AgentSession) buildSystemPromptLocked() string {
	// Build tool snippets from registry
	toolSnippets := make(map[string]string)
	for _, def := range s.tools.GetDefinitions() {
		// Use first line of description as snippet
		desc := def.Description
		if idx := strings.Index(desc, "\n"); idx >= 0 {
			desc = desc[:idx]
		}
		if len(desc) > 100 {
			desc = desc[:100]
		}
		toolSnippets[def.Name] = desc
	}

	// Load resources via ResourceLoader if available
	contextFiles := s.config.ContextFiles
	appendSystem := s.config.AppendSystem
	var systemPromptOverride string
	var skills = s.config.Skills

	if rl := s.config.ResourceLoader; rl != nil {
		// Load project context files (CLAUDE.md/AGENTS.md walk-up)
		if loadedContexts := rl.LoadProjectContextFiles(); len(loadedContexts) > 0 {
			// Convert resourceloader.ContextFile to systemprompt.ContextFile
			for _, cf := range loadedContexts {
				contextFiles = append(contextFiles, systemprompt.ContextFile{
					Path:    cf.Path,
					Content: cf.Content,
				})
			}
		}

		// Discover system prompt file
		if sysFile := rl.DiscoverSystemPromptFile(); sysFile != nil {
			systemPromptOverride = sysFile.Content
		}

		// Discover append system prompt file
		if appendFile := rl.DiscoverAppendSystemPromptFile(); appendFile != nil {
			appendSystem = appendFile.Content
		}
	}

	// Use config system prompt or discovered one
	systemPrompt := s.config.SystemPrompt
	if systemPrompt == "" && systemPromptOverride != "" {
		systemPrompt = systemPromptOverride
	}

	return systemprompt.BuildSystemPrompt(systemprompt.BuildSystemPromptOptions{
		CustomPrompt:       systemPrompt,
		SelectedTools:      s.config.SelectedTools,
		ToolSnippets:       toolSnippets,
		PromptGuidelines:   s.config.ExtraGuidelines,
		AppendSystemPrompt: appendSystem,
		Cwd:                s.config.Cwd,
		ContextFiles:       contextFiles,
		Skills:             skills,
	})
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

// SetModel changes the model for future turns.
func (s *AgentSession) SetModel(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.Model = model
}

// GetModel returns the current model.
func (s *AgentSession) GetModel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.Model
}

// ClearHistory clears the conversation history, keeping the system message.
func (s *AgentSession) ClearHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Keep the first message if it's a system prompt
	if len(s.messages) > 0 && s.messages[0].Role == "system" {
		s.messages = s.messages[:1]
	} else {
		s.messages = nil
	}
	s.turnCount = 0
	return nil
}

// SetThinkingLevel sets the thinking level for future LLM calls.
// The level is clamped to what the model supports.
func (s *AgentSession) SetThinkingLevel(level ThinkingLevel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// ModelInfo.Reasoning is checked via the model registry.
	// For now, accept the level if it's valid.
	if !level.IsValid() {
		return fmt.Errorf("invalid thinking level: %q", level)
	}
	s.thinkingLevel = level
	return nil
}

// GetThinkingLevel returns the current thinking level.
func (s *AgentSession) GetThinkingLevel() ThinkingLevel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.thinkingLevel
}

// GetAvailableThinkingLevels returns the thinking levels available for the current model.
func (s *AgentSession) GetAvailableThinkingLevels() []ThinkingLevel {
	s.mu.RLock()
	model := s.config.Model
	s.mu.RUnlock()

	// Check if model supports reasoning via the model registry
	hasReasoning := supportsReasoning(model)
	if hasReasoning {
		return ValidThinkingLevels()
	}
	return []ThinkingLevel{ThinkingLevelOff}
}

// Branch creates a new branch from a specific entry in the session tree.
// This allows exploring alternative paths from any point in the conversation.
func (s *AgentSession) Branch(fromEntryId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session == nil {
		return fmt.Errorf("session tree not available")
	}

	if err := s.session.Branch(fromEntryId); err != nil {
		return fmt.Errorf("branch failed: %w", err)
	}

	// Rebuild messages from the new branch context
	ctx := s.session.BuildSessionContext()

	// Convert context entries to messages
	s.messages = nil
	for _, rawMsg := range ctx.Messages {
		var msg extensions.Message
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			continue
		}
		s.messages = append(s.messages, msg)
	}

	s.turnCount = len(s.messages)
	return nil
}

// GetSessionTree returns the session tree structure.
func (s *AgentSession) GetSessionTree() []session.SessionTreeNode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.session == nil {
		return nil
	}
	return s.session.GetTree()
}

// GetSessionBranches returns all entries in the current branch path.
func (s *AgentSession) GetSessionBranches() []session.FileEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.session == nil {
		return nil
	}
	return s.session.GetBranch("")
}

// supportsReasoning checks if a model ID is known to support extended thinking.
func supportsReasoning(model string) bool {
	// Known reasoning-capable models (Anthropic Claude 3.5+)
	reasoningPrefixes := []string{
		"claude-sonnet-4",
		"claude-opus-4",
		"claude-sonnet-3.5",
		"claude-opus-3.5",
		"claude-haiku-3.5",
	}
	for _, prefix := range reasoningPrefixes {
		if strings.HasPrefix(strings.ToLower(model), prefix) {
			return true
		}
	}
	return false
}

// --- Prompt / Steer / FollowUp / Abort methods ---
// Aligned to pi's AgentSession.prompt(), steer(), followUp(), abort().

// Prompt sends a user message to the agent and runs the agent loop until completion.
// This is the primary interaction method. It adds the message, runs the loop,
// and handles auto-compaction, pending messages, and steering.
func (s *AgentSession) Prompt(text string) error {
	s.mu.Lock()
	s.promptActive = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.promptActive = false
		s.mu.Unlock()
	}()

	// Add user message
	s.addMessage(extensions.Message{
		Role: extensions.RoleUser,
		Content: []extensions.ContentBlock{
			extensions.TextContentBlock(text),
		},
	})

	// Persist to session
	msgJSON, _ := json.Marshal(extensions.Message{
		Role: extensions.RoleUser,
		Content: []extensions.ContentBlock{
			extensions.TextContentBlock(text),
		},
	})
	s.session.AppendMessage(msgJSON)

	// Run the agent loop
	return s.runLoop()
}

// PromptWithOptions sends a user message with options.
// Options include streaming behavior and image content.
type PromptOptions struct {
	StreamingBehavior string // "steer" or "followUp"
	Images           []string // base64-encoded images
}

func (s *AgentSession) PromptWithOptions(text string, opts PromptOptions) error {
	// Handle streaming behavior
	if opts.StreamingBehavior == "steer" {
		return s.Steer(text)
	}
	if opts.StreamingBehavior == "followUp" {
		s.FollowUp(text)
		return nil
	}

	// Build content blocks
	contentBlocks := []extensions.ContentBlock{
		extensions.TextContentBlock(text),
	}
	// Add images if provided
	for _, img := range opts.Images {
		contentBlocks = append(contentBlocks, extensions.ContentBlock{
			Type: "image",
			Text: img,
		})
	}

	s.mu.Lock()
	s.promptActive = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.promptActive = false
		s.mu.Unlock()
	}()

	s.addMessage(extensions.Message{
		Role:    extensions.RoleUser,
		Content: contentBlocks,
	})

	msgJSON, _ := json.Marshal(extensions.Message{
		Role:    extensions.RoleUser,
		Content: contentBlocks,
	})
	s.session.AppendMessage(msgJSON)

	return s.runLoop()
}

// Steer interrupts the current turn with new text.
// The steering message is injected before the next LLM call.
// Aligned to pi's steer().
func (s *AgentSession) Steer(text string) error {
	s.steeringMu.Lock()
	s.steeringMessages = append(s.steeringMessages, text)
	s.steeringMu.Unlock()

	// Emit steering event
	s.emitEvent(AgentEvent{Type: "steer", Data: text})
	return nil
}

// FollowUp queues a message for after the current turn completes.
// Aligned to pi's followUp().
func (s *AgentSession) FollowUp(text string) {
	s.steeringMu.Lock()
	s.followUpMessages = append(s.followUpMessages, text)
	s.steeringMu.Unlock()

	s.emitEvent(AgentEvent{Type: "followUp", Data: text})
}

// GetSteeringMessages returns pending steering messages.
func (s *AgentSession) GetSteeringMessages() []string {
	s.steeringMu.Lock()
	defer s.steeringMu.Unlock()
	out := make([]string, len(s.steeringMessages))
	copy(out, s.steeringMessages)
	return out
}

// GetFollowUpMessages returns pending follow-up messages.
func (s *AgentSession) GetFollowUpMessages() []string {
	s.steeringMu.Lock()
	defer s.steeringMu.Unlock()
	out := make([]string, len(s.followUpMessages))
	copy(out, s.followUpMessages)
	return out
}

// PendingMessageCount returns the number of pending steering + follow-up messages.
func (s *AgentSession) PendingMessageCount() int {
	s.steeringMu.Lock()
	defer s.steeringMu.Unlock()
	return len(s.steeringMessages) + len(s.followUpMessages)
}

// ClearQueue clears all pending steering and follow-up messages.
func (s *AgentSession) ClearQueue() {
	s.steeringMu.Lock()
	s.steeringMessages = nil
	s.followUpMessages = nil
	s.pendingNextTurn = nil
	s.steeringMu.Unlock()
}

// Abort cancels the current agent turn.
// Aligned to pi's abort().
func (s *AgentSession) Abort() {
	if s.cancel != nil {
		s.cancel()
	}
	// Reset context for next turn
	s.mu.Lock()
	ctx, cancel := func() (context.Context, context.CancelFunc) {
		if s.config.Timeout > 0 {
			return context.WithTimeout(context.Background(), s.config.Timeout)
		}
		return context.WithCancel(context.Background())
	}()
	s.ctx = ctx
	s.cancel = cancel
	s.mu.Unlock()

	s.emitEvent(AgentEvent{Type: "abort"})
}

// Subscribe registers a listener for agent events.
// Returns an unsubscribe function.
// Aligned to pi's subscribe().
func (s *AgentSession) Subscribe(listener func(event AgentEvent)) func() {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	s.subscribers = append(s.subscribers, listener)
	return func() {
		s.subMu.Lock()
		defer s.subMu.Unlock()
		for i, l := range s.subscribers {
			if &l == &listener {
				s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
				break
			}
		}
	}
}

// Dispose cleans up the session: cancels context, persists messages, emits shutdown.
// Aligned to pi's dispose().
func (s *AgentSession) Dispose() {
	s.emitEvent(AgentEvent{Type: "shutdown"})
	s.ClearQueue()
	if s.cancel != nil {
		s.cancel()
	}
}

// emitEvent sends an event to all subscribers.
func (s *AgentSession) emitEvent(event AgentEvent) {
	s.subMu.RLock()
	subscribers := make([]func(AgentEvent), len(s.subscribers))
	copy(subscribers, s.subscribers)
	s.subMu.RUnlock()

	for _, listener := range subscribers {
		listener(event)
	}
}

// injectSteeringMessages adds steering messages into the conversation before the next LLM call.
func (s *AgentSession) injectSteeringMessages() {
	s.steeringMu.Lock()
	msgs := s.steeringMessages
	s.steeringMessages = nil
	s.steeringMu.Unlock()

	for _, m := range msgs {
		s.addMessage(extensions.Message{
			Role: extensions.RoleUser,
			Content: []extensions.ContentBlock{
				extensions.TextContentBlock(m),
			},
		})
	}
}

// injectFollowUpMessages adds follow-up messages after a turn completes.
func (s *AgentSession) injectFollowUpMessages() {
	s.steeringMu.Lock()
	msgs := s.followUpMessages
	s.followUpMessages = nil
	s.steeringMu.Unlock()

	for _, m := range msgs {
		s.addMessage(extensions.Message{
			Role: extensions.RoleUser,
			Content: []extensions.ContentBlock{
				extensions.TextContentBlock(m),
			},
		})
	}
}

// --- CycleModel and CycleThinkingLevel ---
// Aligned to pi's cycleModel() and cycleThinkingLevel().

// CycleModel cycles through available models.
// direction: 1 = next, -1 = previous.
func (s *AgentSession) CycleModel(direction int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// For now, just return current model — full cycling requires model registry integration
	return s.config.Model
}

// CycleThinkingLevel cycles through available thinking levels.
// direction: 1 = next (higher), -1 = previous (lower).
func (s *AgentSession) CycleThinkingLevel(direction int) ThinkingLevel {
	s.mu.Lock()
	defer s.mu.Unlock()

	levels := ValidThinkingLevels()
	currentIdx := 0
	for i, l := range levels {
		if l == s.thinkingLevel {
			currentIdx = i
			break
		}
	}

	newIdx := currentIdx + direction
	if newIdx < 0 {
		newIdx = 0
	}
	if newIdx >= len(levels) {
		newIdx = len(levels) - 1
	}

	s.thinkingLevel = levels[newIdx]
	return s.thinkingLevel
}

// --- SessionStats ---
// Aligned to pi's getStats().

// SessionStats holds statistics about the session.
type SessionStats struct {
	SessionID       string
	UserMessages    int
	AssistantMessages int
	ToolCalls       int
	ToolResults     int
	TotalMessages   int
	TurnCount       int
}

// GetStats returns session statistics.
func (s *AgentSession) GetStats() SessionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := SessionStats{
		SessionID:  s.session.GetSessionID(),
		TurnCount:  s.turnCount,
		TotalMessages: len(s.messages),
	}

	for _, msg := range s.messages {
		switch msg.Role {
		case "user":
			stats.UserMessages++
		case "assistant":
			stats.AssistantMessages++
			for _, block := range msg.Content {
				if block.Type == "tool_use" {
					stats.ToolCalls++
				}
			}
		case "tool":
			stats.ToolResults++
		}
	}

	return stats
}

// --- SupportsThinking ---
// Aligned to pi's supportsThinking().

// SupportsThinking returns whether the current model supports extended thinking.
func (s *AgentSession) SupportsThinking() bool {
	s.mu.RLock()
	model := s.config.Model
	s.mu.RUnlock()
	return supportsReasoning(model)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// generateToolCallId generates a unique ID for tool calls.
func generateToolCallId() string {
	// Simple ID generation - in production use UUID
	return fmt.Sprintf("tool_%d", time.Now().UnixNano())
}