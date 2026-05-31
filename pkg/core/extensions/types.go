package extensions

import (
	"fmt"
	"sync"
	"time"

	"miniclaudecode-go/pkg/core/eventbus"
)

// ---------------------------------------------------------------------------
// EventType — all pi event types as string constants
// ---------------------------------------------------------------------------

// EventType represents a typed event name in the extension system.
type EventType string

const (
	EventSessionStart         EventType = "session_start"
	EventSessionBeforeCompact EventType = "session_before_compact"
	EventSessionCompact       EventType = "session_compact"
	EventContext              EventType = "context"
	EventBeforeAgentStart     EventType = "before_agent_start"
	EventAgentStart           EventType = "agent_start"
	EventAgentEnd             EventType = "agent_end"
	EventTurnStart            EventType = "turn_start"
	EventTurnEnd              EventType = "turn_end"
	EventToolCall             EventType = "tool_call"
	EventToolResult           EventType = "tool_result"
	EventToolExecutionStart   EventType = "tool_execution_start"
	EventToolExecutionEnd     EventType = "tool_execution_end"
	EventStreamStart          EventType = "stream_start"
	EventStreamEnd            EventType = "stream_end"
	EventMessage              EventType = "message"
	EventError                EventType = "error"
)

// AllEventTypes is a slice of all defined event types for validation and iteration.
var AllEventTypes = []EventType{
	EventSessionStart,
	EventSessionBeforeCompact,
	EventSessionCompact,
	EventContext,
	EventBeforeAgentStart,
	EventAgentStart,
	EventAgentEnd,
	EventTurnStart,
	EventTurnEnd,
	EventToolCall,
	EventToolResult,
	EventToolExecutionStart,
	EventToolExecutionEnd,
	EventStreamStart,
	EventStreamEnd,
	EventMessage,
	EventError,
}

// String implements fmt.Stringer for EventType.
func (et EventType) String() string {
	return string(et)
}

// IsValid checks whether an EventType is one of the known event types.
func (et EventType) IsValid() bool {
	for _, known := range AllEventTypes {
		if et == known {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Extension — main extension interface
// ---------------------------------------------------------------------------

// Extension is the primary interface that all extensions must implement.
// Mirrors pi's Extension interface.
type Extension interface {
	// Name returns the unique identifier for this extension.
	Name() string
	// Version returns the semantic version of this extension.
	Version() string
	// Priority determines the order in which extensions are initialized.
	// Higher values initialize first. The default is 0.
	Priority() int
	// Register is called once when the extension is loaded.
	// Use the ExtensionContext to subscribe to events and register tools.
	Register(ctx *ExtensionContext) error
	// Close is called during shutdown so the extension can release resources.
	Close() error
}

// ExtensionInit is an optional interface extensions can implement to
// receive initialization order hints (higher specificity than Priority).
type ExtensionInit interface {
	Extension
	// InitOrder returns a priority value; lower values initialize first.
	InitOrder() int
}

// ExtensionDestroy is an optional interface extensions can implement for
// graceful shutdown with access to the context.
type ExtensionDestroy interface {
	Extension
	// Destroy is called when the extension is being unloaded.
	Destroy(ctx *ExtensionContext) error
}

// ExtensionConfigurable is an optional interface for extensions that declare
// configuration schemas.
type ExtensionConfigurable interface {
	Extension
	// ConfigSchema returns a JSON Schema describing the extension's config.
	ConfigSchema() map[string]interface{}
	// DefaultConfig returns the default configuration values.
	DefaultConfig() map[string]interface{}
}

// ---------------------------------------------------------------------------
// ExtensionContext — context passed to extension Register()
// ---------------------------------------------------------------------------

// ExtensionContext is the context object passed to Extension.Register().
// It provides access to the event bus, the extension API surface,
// and extension-specific metadata.
type ExtensionContext struct {
	// Events is the typed event bus for subscribing to and emitting events.
	Events *eventbus.EventBus
	// API is the full extension API surface.
	API *ExtensionAPI
	// Meta stores per-extension metadata keyed by "handler_owner:<name>" and
	// similar patterns, mirroring pi's context metadata approach.
	Meta map[string]string
	// Config is the extension-specific configuration, if any.
	Config map[string]interface{}
	// ExtensionName is the name of the extension this context belongs to.
	ExtensionName string
}

// NewExtensionContext creates a new ExtensionContext for the given extension.
func NewExtensionContext(name string, events *eventbus.EventBus, api *ExtensionAPI) *ExtensionContext {
	return &ExtensionContext{
		Events:        events,
		API:           api,
		Meta:          make(map[string]string),
		Config:        make(map[string]interface{}),
		ExtensionName: name,
	}
}

// On is a convenience method that subscribes a handler to an event type
// via the embedded event bus. The handler is automatically named with
// the extension's name as a prefix.
func (ec *ExtensionContext) On(event EventType, handler eventbus.Handler) *eventbus.HandlerToken {
	name := ec.ExtensionName
	return ec.Events.OnNamed(string(event), name, handler)
}

// Off is a convenience method that unsubscribes a handler using its token.
func (ec *ExtensionContext) Off(token *eventbus.HandlerToken) {
	ec.Events.Off(token)
}

// SetMeta stores a key-value pair in the extension's metadata.
func (ec *ExtensionContext) SetMeta(key, value string) {
	ec.Meta[key] = value
}

// GetMeta retrieves a value from the extension's metadata.
func (ec *ExtensionContext) GetMeta(key string) (string, bool) {
	v, ok := ec.Meta[key]
	return v, ok
}

// ---------------------------------------------------------------------------
// ExtensionAPI — the main API surface exposed to extensions
// ---------------------------------------------------------------------------

// ExtensionAPI is the full API surface exposed to extensions.
// Mirrors pi's ExtensionAPI with all methods represented as function fields
// so the host can inject implementations at runtime.
type ExtensionAPI struct {
	// ---- Session info ----

	// GetSessionId returns the current session's unique identifier.
	GetSessionId func() string
	// GetSessionPath returns the filesystem path for the current session.
	GetSessionPath func() string

	// ---- Messages ----

	// GetMessages returns all messages in the current conversation.
	GetMessages func() []Message
	// AddMessages appends multiple messages to the conversation.
	AddMessages func(msgs []Message)
	// AddMessage appends a single message to the conversation.
	AddMessage func(msg Message)

	// ---- Context / tools ----

	// BuildMessages constructs the full message array that would be sent
	// to the model, applying compaction, system prompts, etc.
	BuildMessages func(opts *BuildMessagesOptions) ([]Message, error)
	// GetAvailableTools returns all currently registered tool definitions.
	GetAvailableTools func() []ToolDefinition

	// ---- Model ----

	// GetModel returns the current model identifier (e.g. "claude-sonnet-4-20250514").
	GetModel func() string
	// SetModel changes the active model for subsequent turns.
	SetModel func(model string)

	// ---- Config ----

	// GetConfig returns the current session configuration as a generic map.
	GetConfig func() map[string]interface{}
	// SetConfig updates the session configuration with the given values.
	SetConfig func(key string, value interface{})

	// ---- Permissions ----

	// PromptPermission asks the user for permission to perform an action.
	// Returns true if granted, false if denied.
	PromptPermission func(req PermissionRequest) (bool, error)

	// ---- Tools ----

	// RegisterTool adds a new tool that the agent can invoke.
	RegisterTool func(def ToolDefinition, handler ToolHandler) error
	// UnregisterTool removes a previously registered tool by name.
	UnregisterTool func(name string)

	// ---- Bash ----

	// ExecuteBash runs a shell command and returns its result.
	ExecuteBash func(cmd string, opts *BashOptions) (*BashResult, error)

	// ---- Compact ----

	// Compact triggers manual context compaction.
	Compact func() error
	// SetAutoCompact enables or disables automatic compaction.
	SetAutoCompact func(enabled bool)

	// ---- Sub-agent / Fork ----

	// ForkSession creates a sub-agent session (fork) with the given options.
	ForkSession func(opts *ForkOptions) (*ForkResult, error)

	// ---- Streaming ----

	// GetStreamState returns the current streaming state.
	GetStreamState func() StreamState
	// AbortStream requests cancellation of the current stream.
	AbortStream func() error

	// ---- Token tracking ----

	// GetTokenUsage returns token usage statistics for the current session.
	GetTokenUsage func() TokenUsage
	// GetTokenLimit returns the maximum token budget.
	GetTokenLimit func() int

	// ---- Cost ----

	// GetCost returns the accumulated cost for the current session.
	GetCost func() float64

	// ---- Hooks ----

	// RegisterHook registers a named hook function at the given hook point.
	RegisterHook func(hookPoint HookPoint, name string, fn HookFunc) error
	// UnregisterHook removes a named hook.
	UnregisterHook func(hookPoint HookPoint, name string)

	// ---- Logging ----

	// Log writes a log message at the specified level.
	Log func(level LogLevel, msg string, fields ...interface{})
}

// ---------------------------------------------------------------------------
// ToolHandler / ToolContext
// ---------------------------------------------------------------------------

// ToolHandler is the function signature for tool handlers.
type ToolHandler func(ctx *ToolContext, input map[string]interface{}) (interface{}, error)

// ToolContext provides context for a single tool invocation.
type ToolContext struct {
	// SessionId is the current session identifier.
	SessionId string
	// Tools provides access to the tool registry for nested tool calls.
	Tools *ToolRegistry
	// Messages allows the tool to read conversation messages.
	Messages func() []Message
	// Permission allows the tool to request user permission.
	Permission func(req PermissionRequest) (bool, error)
	// Abort signals whether the tool should stop execution.
	Abort func() bool
	// Metadata carries arbitrary per-invocation metadata.
	Metadata map[string]interface{}
}

// ---------------------------------------------------------------------------
// ToolDefinition
// ---------------------------------------------------------------------------

// ToolDefinition describes a tool that can be registered with the agent.
// Mirrors pi's ToolDefinition.
type ToolDefinition struct {
	// Name is the unique tool identifier (e.g. "bash", "edit_file").
	Name string `json:"name"`
	// Description is a human-readable description shown to the model.
	Description string `json:"description"`
	// InputSchema is a JSON Schema object describing the tool's input.
	InputSchema map[string]interface{} `json:"input_schema"`
	// Tags are optional categorization labels (e.g. "system", "user", "mcp").
	Tags []string `json:"tags,omitempty"`
	// ReadOnly indicates the tool does not modify any state.
	ReadOnly bool `json:"read_only,omitempty"`
	// Dangerous indicates the tool requires explicit user permission.
	Dangerous bool `json:"dangerous,omitempty"`
	// Hidden indicates the tool should not be shown in tool listings.
	Hidden bool `json:"hidden,omitempty"`
	// PromptSnippet is a brief description for system prompt injection.
	PromptSnippet string `json:"prompt_snippet,omitempty"`
	// PromptGuidelines are usage rules shown to the model in system prompt.
	PromptGuidelines []string `json:"prompt_guidelines,omitempty"`
}

// ---------------------------------------------------------------------------
// ToolRegistry
// ---------------------------------------------------------------------------

// ToolRegistry maintains the set of available tools.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]registeredTool
}

type registeredTool struct {
	def     ToolDefinition
	handler ToolHandler
}

// NewToolRegistry creates a new empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]registeredTool)}
}

// Register adds a tool definition and its handler to the registry.
// Returns an error if a tool with the same name already exists.
func (tr *ToolRegistry) Register(def ToolDefinition, handler ToolHandler) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if _, exists := tr.tools[def.Name]; exists {
		return fmt.Errorf("tool already registered: %s", def.Name)
	}
	tr.tools[def.Name] = registeredTool{def: def, handler: handler}
	return nil
}

// Unregister removes a tool by name. No-op if the tool does not exist.
func (tr *ToolRegistry) Unregister(name string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	delete(tr.tools, name)
}

// Get returns a tool's definition and handler by name.
func (tr *ToolRegistry) Get(name string) (ToolDefinition, ToolHandler, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	rt, ok := tr.tools[name]
	if !ok {
		return ToolDefinition{}, nil, false
	}
	return rt.def, rt.handler, true
}

// List returns all registered tool definitions.
func (tr *ToolRegistry) List() []ToolDefinition {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	defs := make([]ToolDefinition, 0, len(tr.tools))
	for _, rt := range tr.tools {
		defs = append(defs, rt.def)
	}
	return defs
}

// ListVisible returns tool definitions that are not hidden.
func (tr *ToolRegistry) ListVisible() []ToolDefinition {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	defs := make([]ToolDefinition, 0, len(tr.tools))
	for _, rt := range tr.tools {
		if !rt.def.Hidden {
			defs = append(defs, rt.def)
		}
	}
	return defs
}

// Has checks whether a tool with the given name is registered.
func (tr *ToolRegistry) Has(name string) bool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	_, ok := tr.tools[name]
	return ok
}

// Count returns the number of registered tools.
func (tr *ToolRegistry) Count() int {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	return len(tr.tools)
}

// ---------------------------------------------------------------------------
// Permission types
// ---------------------------------------------------------------------------

// PermissionType enumerates the kinds of actions that require permission.
type PermissionType string

const (
	PermBash          PermissionType = "bash"
	PermEdit          PermissionType = "edit"
	PermWrite         PermissionType = "write"
	PermDelete        PermissionType = "delete"
	PermMCPAuth       PermissionType = "mcp_auth"
	PermClientRequest PermissionType = "client_request"
	PermNetwork       PermissionType = "network"
)

// PermissionRequest is the structure sent when asking for user permission.
type PermissionRequest struct {
	// Type is the category of the permission request.
	Type PermissionType `json:"type"`
	// Target is the specific target of the action (e.g. file path, command).
	Target string `json:"target"`
	// Message is a human-readable explanation shown to the user.
	Message string `json:"message"`
	// Metadata carries optional extra context.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// PermissionResult captures the outcome of a permission prompt.
type PermissionResult struct {
	// Granted indicates whether permission was given.
	Granted bool `json:"granted"`
	// Reason provides context when permission is denied.
	Reason string `json:"reason,omitempty"`
}

// ---------------------------------------------------------------------------
// Bash execution types
// ---------------------------------------------------------------------------

// BashOptions configures a bash command execution.
type BashOptions struct {
	// CWD is the working directory for the command.
	CWD string `json:"cwd,omitempty"`
	// Env is additional environment variables to set.
	Env map[string]string `json:"env,omitempty"`
	// Timeout is the maximum execution time in seconds; 0 means no timeout.
	Timeout int `json:"timeout,omitempty"`
	// Interactive indicates the command requires terminal interaction.
	Interactive bool `json:"interactive,omitempty"`
	// StreamOutput indicates output should be streamed in real-time.
	StreamOutput bool `json:"stream_output,omitempty"`
}

// BashResult holds the output of a bash command execution.
type BashResult struct {
	// Stdout is the captured standard output.
	Stdout string `json:"stdout"`
	// Stderr is the captured standard error.
	Stderr string `json:"stderr"`
	// Code is the process exit code.
	Code int `json:"code"`
	// Signal is the termination signal, if killed by signal (empty string if none).
	Signal string `json:"signal,omitempty"`
	// Duration is how long the command took to execute.
	Duration time.Duration `json:"duration"`
}

// ---------------------------------------------------------------------------
// Fork / sub-agent types
// ---------------------------------------------------------------------------

// ForkOptions configures a session fork (sub-agent).
type ForkOptions struct {
	// Message is the initial prompt for the sub-agent.
	Message string `json:"message"`
	// Path is the working directory for the sub-agent session.
	Path string `json:"path,omitempty"`
	// Model overrides the model for the sub-agent (empty = inherit).
	Model string `json:"model,omitempty"`
	// Tools restricts available tools (empty = inherit all).
	Tools []string `json:"tools,omitempty"`
	// MaxTurns limits the number of agent turns (0 = unlimited).
	MaxTurns int `json:"max_turns,omitempty"`
	// Metadata carries arbitrary metadata for the fork.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ForkResult holds the result of a session fork.
type ForkResult struct {
	// SessionId is the unique identifier of the new sub-agent session.
	SessionId string `json:"session_id"`
	// Path is the working directory of the sub-agent session.
	Path string `json:"path"`
	// Output is the final output from the sub-agent.
	Output string `json:"output"`
	// Messages is the full conversation from the sub-agent session.
	Messages []Message `json:"messages"`
	// Cost is the accumulated cost of the sub-agent session.
	Cost float64 `json:"cost"`
	// TokenUsage is the token usage of the sub-agent session.
	TokenUsage TokenUsage `json:"token_usage"`
}

// ---------------------------------------------------------------------------
// BuildMessages options
// ---------------------------------------------------------------------------

// BuildMessagesOptions configures how messages are built for the model.
type BuildMessagesOptions struct {
	// IncludeSystem determines whether to include the system prompt.
	IncludeSystem bool `json:"include_system"`
	// IncludeHistory determines whether to include conversation history.
	IncludeHistory bool `json:"include_history"`
	// MaxTokens limits the total token count of the built messages.
	MaxTokens int `json:"max_tokens,omitempty"`
	// CompactThreshold is the token count at which compaction is triggered.
	CompactThreshold int `json:"compact_threshold,omitempty"`
	// ToolChoice overrides the tool choice strategy.
	ToolChoice *ToolChoice `json:"tool_choice,omitempty"`
	// AdditionalSystemPrompts are extra system prompt fragments to append.
	AdditionalSystemPrompts []string `json:"additional_system_prompts,omitempty"`
}

// ToolChoice specifies how the model should select tools.
type ToolChoice struct {
	// Type is "auto", "any", "none", or "tool".
	Type string `json:"type"`
	// Name is required when Type is "tool".
	Name string `json:"name,omitempty"`
}

// ---------------------------------------------------------------------------
// Message types
// ---------------------------------------------------------------------------

// MessageRole identifies the sender of a message.
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// Message represents a single message in the conversation.
// Mirrors the Anthropic API message structure.
type Message struct {
	// Role identifies who sent this message.
	Role MessageRole `json:"role"`
	// Content is the message body as a list of content blocks.
	Content []ContentBlock `json:"content"`
	// Model is the model that generated this message (for assistant messages).
	Model string `json:"model,omitempty"`
	// StopReason is why the model stopped generating (e.g. "end_turn", "tool_use").
	StopReason string `json:"stop_reason,omitempty"`
	// ToolUseID links a tool result to its tool_use request.
	ToolUseID string `json:"tool_use_id,omitempty"`
	// Timestamp is when the message was created.
	Timestamp time.Time `json:"timestamp"`
	// Metadata carries arbitrary per-message metadata.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ContentBlock represents a single content block within a message.
type ContentBlock struct {
	// Type is the block type: "text", "tool_use", "tool_result", "image", "thinking".
	Type string `json:"type"`
	// Text is the text content (for type "text" and "thinking").
	Text string `json:"text,omitempty"`
	// ToolUseID is the tool use identifier (for "tool_use" and "tool_result").
	ToolUseID string `json:"tool_use_id,omitempty"`
	// ToolName is the tool name (for "tool_use").
	ToolName string `json:"tool_name,omitempty"`
	// ToolInput is the tool input (for "tool_use").
	ToolInput map[string]interface{} `json:"tool_input,omitempty"`
	// ToolResult is the tool result content (for "tool_result").
	ToolResult interface{} `json:"tool_result,omitempty"`
	// IsError indicates the tool result is an error (for "tool_result").
	IsError bool `json:"is_error,omitempty"`
	// SourceType is the media source type (for "image").
	SourceType string `json:"source_type,omitempty"`
	// SourceData is the media source data (for "image").
	SourceData map[string]interface{} `json:"source_data,omitempty"`
	// Thinking is the thinking content (for "thinking").
	Thinking string `json:"thinking,omitempty"`
	// Signature is the thinking signature (for "thinking").
	Signature string `json:"signature,omitempty"`
}

// TextContentBlock creates a simple text content block.
func TextContentBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// ToolUseContentBlock creates a tool_use content block.
func ToolUseContentBlock(toolUseID, toolName string, input map[string]interface{}) ContentBlock {
	return ContentBlock{
		Type:      "tool_use",
		ToolUseID: toolUseID,
		ToolName:  toolName,
		ToolInput: input,
	}
}

// ToolResultContentBlock creates a tool_result content block.
func ToolResultContentBlock(toolUseID string, result interface{}, isError bool) ContentBlock {
	return ContentBlock{
		Type:      "tool_result",
		ToolUseID: toolUseID,
		ToolResult: result,
		IsError:   isError,
	}
}

// ThinkingContentBlock creates a thinking content block.
func ThinkingContentBlock(thinking, signature string) ContentBlock {
	return ContentBlock{
		Type:      "thinking",
		Thinking:  thinking,
		Signature: signature,
	}
}

// ---------------------------------------------------------------------------
// Event payload types — one per EventType
// ---------------------------------------------------------------------------

// SessionStartPayload is the payload for EventSessionStart.
type SessionStartPayload struct {
	SessionId   string                 `json:"session_id"`
	SessionPath string                 `json:"session_path"`
	Model       string                 `json:"model"`
	Config      map[string]interface{} `json:"config"`
	Timestamp   time.Time              `json:"timestamp"`
}

// SessionBeforeCompactPayload is the payload for EventSessionBeforeCompact.
type SessionBeforeCompactPayload struct {
	SessionId    string     `json:"session_id"`
	MessageCount int        `json:"message_count"`
	TokenUsage   TokenUsage `json:"token_usage"`
	Timestamp    time.Time  `json:"timestamp"`
}

// SessionCompactPayload is the payload for EventSessionCompact.
type SessionCompactPayload struct {
	SessionId      string    `json:"session_id"`
	MessagesBefore int       `json:"messages_before"`
	MessagesAfter  int       `json:"messages_after"`
	TokensSaved    int       `json:"tokens_saved"`
	Timestamp      time.Time `json:"timestamp"`
}

// ContextPayload is the payload for EventContext.
type ContextPayload struct {
	SessionId  string     `json:"session_id"`
	Messages   []Message  `json:"messages"`
	TokenUsage TokenUsage `json:"token_usage"`
	Timestamp  time.Time  `json:"timestamp"`
}

// BeforeAgentStartPayload is the payload for EventBeforeAgentStart.
type BeforeAgentStartPayload struct {
	SessionId string    `json:"session_id"`
	Model     string    `json:"model"`
	Turn      int       `json:"turn"`
	Timestamp time.Time `json:"timestamp"`
}

// AgentStartPayload is the payload for EventAgentStart.
type AgentStartPayload struct {
	SessionId string    `json:"session_id"`
	Model     string    `json:"model"`
	Turn      int       `json:"turn"`
	Timestamp time.Time `json:"timestamp"`
}

// AgentEndPayload is the payload for EventAgentEnd.
type AgentEndPayload struct {
	SessionId  string        `json:"session_id"`
	Model      string        `json:"model"`
	Turn       int           `json:"turn"`
	StopReason string        `json:"stop_reason"`
	TokenUsage TokenUsage    `json:"token_usage"`
	Cost       float64       `json:"cost"`
	Duration   time.Duration `json:"duration"`
	Timestamp  time.Time     `json:"timestamp"`
}

// TurnStartPayload is the payload for EventTurnStart.
type TurnStartPayload struct {
	SessionId string    `json:"session_id"`
	Turn      int       `json:"turn"`
	Timestamp time.Time `json:"timestamp"`
}

// TurnEndPayload is the payload for EventTurnEnd.
type TurnEndPayload struct {
	SessionId  string        `json:"session_id"`
	Turn       int           `json:"turn"`
	TokenUsage TokenUsage    `json:"token_usage"`
	Cost       float64       `json:"cost"`
	Duration   time.Duration `json:"duration"`
	Timestamp  time.Time     `json:"timestamp"`
}

// ToolCallPayload is the payload for EventToolCall.
type ToolCallPayload struct {
	SessionId string                 `json:"session_id"`
	ToolName  string                 `json:"tool_name"`
	ToolUseID string                 `json:"tool_use_id"`
	Input     map[string]interface{} `json:"input"`
	Timestamp time.Time              `json:"timestamp"`
}

// ToolResultPayload is the payload for EventToolResult.
type ToolResultPayload struct {
	SessionId string        `json:"session_id"`
	ToolName  string        `json:"tool_name"`
	ToolUseID string        `json:"tool_use_id"`
	Result    interface{}   `json:"result"`
	IsError   bool          `json:"is_error"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
}

// ToolExecutionStartPayload is the payload for EventToolExecutionStart.
type ToolExecutionStartPayload struct {
	SessionId string                 `json:"session_id"`
	ToolName  string                 `json:"tool_name"`
	ToolUseID string                 `json:"tool_use_id"`
	Input     map[string]interface{} `json:"input"`
	Timestamp time.Time              `json:"timestamp"`
}

// ToolExecutionEndPayload is the payload for EventToolExecutionEnd.
type ToolExecutionEndPayload struct {
	SessionId string        `json:"session_id"`
	ToolName  string        `json:"tool_name"`
	ToolUseID string        `json:"tool_use_id"`
	Result    interface{}   `json:"result"`
	IsError   bool          `json:"is_error"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
}

// StreamStartPayload is the payload for EventStreamStart.
type StreamStartPayload struct {
	SessionId string    `json:"session_id"`
	Model     string    `json:"model"`
	Turn      int       `json:"turn"`
	Timestamp time.Time `json:"timestamp"`
}

// StreamEndPayload is the payload for EventStreamEnd.
type StreamEndPayload struct {
	SessionId  string     `json:"session_id"`
	Model      string     `json:"model"`
	Turn       int        `json:"turn"`
	StopReason string     `json:"stop_reason"`
	TokenUsage TokenUsage `json:"token_usage"`
	Timestamp  time.Time  `json:"timestamp"`
}

// MessagePayload is the payload for EventMessage.
type MessagePayload struct {
	SessionId string    `json:"session_id"`
	Message   Message   `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// ErrorPayload is the payload for EventError.
type ErrorPayload struct {
	SessionId string    `json:"session_id"`
	Error     error     `json:"-"`
	Source    string    `json:"source"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// Stream state
// ---------------------------------------------------------------------------

// StreamState represents the current state of the response stream.
type StreamState string

const (
	StreamIdle      StreamState = "idle"
	StreamActive    StreamState = "active"
	StreamAborted   StreamState = "aborted"
	StreamCompleted StreamState = "completed"
)

// ---------------------------------------------------------------------------
// Token usage
// ---------------------------------------------------------------------------

// TokenUsage tracks token consumption for a session or turn.
// Mirrors the Anthropic API usage structure.
type TokenUsage struct {
	// InputTokens is the number of tokens in the prompt.
	InputTokens int `json:"input_tokens"`
	// OutputTokens is the number of tokens in the completion.
	OutputTokens int `json:"output_tokens"`
	// CacheCreationInputTokens is the number of tokens written to the prompt cache.
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	// CacheReadInputTokens is the number of tokens read from the prompt cache.
	CacheReadInputTokens int `json:"cache_read_input_tokens"`
	// ServerToolUseInputTokens is the number of input tokens used by server-side tools.
	ServerToolUseInputTokens int `json:"server_tool_use_input_tokens"`
}

// Total returns the total number of tokens consumed (input + output).
func (tu TokenUsage) Total() int {
	return tu.InputTokens + tu.OutputTokens
}

// Add accumulates another TokenUsage into this one.
func (tu *TokenUsage) Add(other TokenUsage) {
	tu.InputTokens += other.InputTokens
	tu.OutputTokens += other.OutputTokens
	tu.CacheCreationInputTokens += other.CacheCreationInputTokens
	tu.CacheReadInputTokens += other.CacheReadInputTokens
	tu.ServerToolUseInputTokens += other.ServerToolUseInputTokens
}

// IsZero returns true if no tokens have been consumed.
func (tu TokenUsage) IsZero() bool {
	return tu.InputTokens == 0 && tu.OutputTokens == 0 &&
		tu.CacheCreationInputTokens == 0 && tu.CacheReadInputTokens == 0 &&
		tu.ServerToolUseInputTokens == 0
}

// ---------------------------------------------------------------------------
// Hook types
// ---------------------------------------------------------------------------

// HookPoint identifies a point in the agent lifecycle where hooks can run.
type HookPoint string

const (
	HookPreToolCall  HookPoint = "pre_tool_call"
	HookPostToolCall HookPoint = "post_tool_call"
	HookPreStream    HookPoint = "pre_stream"
	HookPostStream   HookPoint = "post_stream"
	HookPreCompact   HookPoint = "pre_compact"
	HookPostCompact  HookPoint = "post_compact"
	HookPreTurn      HookPoint = "pre_turn"
	HookPostTurn     HookPoint = "post_turn"
)

// HookFunc is the function signature for hooks.
// Returning a non-nil error aborts the associated operation.
type HookFunc func(payload interface{}) error

// HookEntry stores a named hook with its function.
type HookEntry struct {
	Name string
	Fn   HookFunc
}

// HookRegistry manages hooks by hook point.
type HookRegistry struct {
	mu    sync.RWMutex
	hooks map[HookPoint][]HookEntry
}

// NewHookRegistry creates a new empty hook registry.
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{hooks: make(map[HookPoint][]HookEntry)}
}

// Register adds a named hook at the given hook point.
// Returns an error if a hook with the same name already exists at that point.
func (hr *HookRegistry) Register(hookPoint HookPoint, name string, fn HookFunc) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	entries := hr.hooks[hookPoint]
	for _, entry := range entries {
		if entry.Name == name {
			return fmt.Errorf("hook already registered at %s: %s", hookPoint, name)
		}
	}
	hr.hooks[hookPoint] = append(entries, HookEntry{Name: name, Fn: fn})
	return nil
}

// Unregister removes a named hook from the given hook point.
func (hr *HookRegistry) Unregister(hookPoint HookPoint, name string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	entries := hr.hooks[hookPoint]
	for i, entry := range entries {
		if entry.Name == name {
			hr.hooks[hookPoint] = append(entries[:i], entries[i+1:]...)
			return
		}
	}
}

// Run executes all hooks at the given hook point, in registration order.
// Returns the first error encountered (which aborts the operation).
func (hr *HookRegistry) Run(hookPoint HookPoint, payload interface{}) error {
	hr.mu.RLock()
	entries := make([]HookEntry, len(hr.hooks[hookPoint]))
	copy(entries, hr.hooks[hookPoint])
	hr.mu.RUnlock()

	for _, entry := range entries {
		if err := entry.Fn(payload); err != nil {
			return fmt.Errorf("hook %s at %s: %w", entry.Name, hookPoint, err)
		}
	}
	return nil
}

// List returns all hook names at the given hook point.
func (hr *HookRegistry) List(hookPoint HookPoint) []string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	entries := hr.hooks[hookPoint]
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names
}

// ---------------------------------------------------------------------------
// Log levels
// ---------------------------------------------------------------------------

// LogLevel identifies the severity of a log message.
type LogLevel int

const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarn
	LogError
)

// String implements fmt.Stringer for LogLevel.
func (l LogLevel) String() string {
	switch l {
	case LogDebug:
		return "DEBUG"
	case LogInfo:
		return "INFO"
	case LogWarn:
		return "WARN"
	case LogError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ---------------------------------------------------------------------------
// Extension metadata / registration result
// ---------------------------------------------------------------------------

// ExtensionMeta describes an extension's metadata for listing and discovery.
type ExtensionMeta struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Author      string   `json:"author,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	InitOrder   int      `json:"init_order,omitempty"`
}

// RegistrationResult is returned after an extension is registered.
type RegistrationResult struct {
	ExtensionName string      `json:"extension_name"`
	ToolsAdded    []string    `json:"tools_added"`
	HooksAdded    []HookPoint `json:"hooks_added"`
	Errors        []error     `json:"-"`
}
