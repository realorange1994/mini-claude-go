package extensions

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"miniclaudecode-go/pkg/core/eventbus"
)

// ExtensionRunner manages extension lifecycle and event emission.
// It keeps track of registered extensions, their handlers, and tool definitions,
// and dispatches typed events to handlers in priority order.
//
// Mirrors pi's core/extensions/runner.ts.
type ExtensionRunner struct {
	ctx        *ExtensionContext
	extensions []Extension
	toolDefs   map[string]ToolDefinition

	mu sync.RWMutex
}

// NewExtensionRunner creates a new extension runner backed by the given context.
func NewExtensionRunner(ctx *ExtensionContext) *ExtensionRunner {
	return &ExtensionRunner{
		ctx:        ctx,
		extensions: []Extension{},
		toolDefs:   make(map[string]ToolDefinition),
	}
}

// ---------------------------------------------------------------------------
// Extension lifecycle
// ---------------------------------------------------------------------------

// LoadExtensions calls Register on every factory-registered extension.
// It returns the first non-nil error encountered; partial loads are still
// applied before the error is returned.
func (r *ExtensionRunner) LoadExtensions() error {
	for _, name := range Registered() {
		ext := Get(name)
		if ext == nil {
			continue
		}
		if err := r.RegisterExtension(ext); err != nil {
			return fmt.Errorf("extension %q: %w", name, err)
		}
	}
	return nil
}

// RegisterExtension adds an extension to the runner and calls its Register hook.
// Extensions are kept sorted by descending priority.
func (r *ExtensionRunner) RegisterExtension(ext Extension) error {
	if ext == nil {
		return fmt.Errorf("RegisterExtension: nil extension")
	}
	for _, e := range r.extensions {
		if e.Name() == ext.Name() {
			return fmt.Errorf("extension %q is already registered", ext.Name())
		}
	}

	// Inject the extension name into the context so the event bus On() method
	// can tag handlers with their owning extension name.
	if r.ctx != nil {
		r.ctx.ExtensionName = ext.Name()
	}

	if err := ext.Register(r.ctx); err != nil {
		return fmt.Errorf("extension %q Register: %w", ext.Name(), err)
	}

	r.mu.Lock()
	r.extensions = append(r.extensions, ext)
	sort.Slice(r.extensions, func(i, j int) bool {
		return r.extensions[i].Priority() > r.extensions[j].Priority()
	})
	r.mu.Unlock()

	return nil
}

// UnregisterExtension removes an extension by name and calls its Close hook.
func (r *ExtensionRunner) UnregisterExtension(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	idx := -1
	for i, e := range r.extensions {
		if e.Name() == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("extension %q not found", name)
	}

	ext := r.extensions[idx]
	r.extensions = append(r.extensions[:idx], r.extensions[idx+1:]...)

	if err := ext.Close(); err != nil {
		return fmt.Errorf("extension %q Close: %w", ext.Name(), err)
	}

	// Remove all handlers registered by this extension for all event types
	// via the EventBus's name-based removal.
	events := r.ctx.Events.Events()
	for _, event := range events {
		r.ctx.Events.RemoveHandlersByName(event, ext.Name())
	}

	return nil
}

// Close calls Close on every registered extension and clears all handlers.
func (r *ExtensionRunner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for _, ext := range r.extensions {
		if err := ext.Close(); err != nil {
			errs = append(errs, fmt.Errorf("extension %q: %w", ext.Name(), err))
		}
	}
	r.extensions = nil
	r.toolDefs = make(map[string]ToolDefinition)

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Extensions returns a snapshot of the currently registered extensions.
func (r *ExtensionRunner) Extensions() []Extension {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Extension, len(r.extensions))
	copy(out, r.extensions)
	return out
}

// ToolDefinitions returns a snapshot of all tool definitions registered
// by extensions.
func (r *ExtensionRunner) ToolDefinitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]ToolDefinition, 0, len(r.toolDefs))
	for _, def := range r.toolDefs {
		defs = append(defs, def)
	}
	return defs
}

// RegisterTool registers a tool definition from an extension.
func (r *ExtensionRunner) RegisterTool(def ToolDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.toolDefs[def.Name] = def
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// emit dispatches an event to the event bus and returns collected handler errors.
// It logs (but does not propagate) errors from individual handlers.
func (r *ExtensionRunner) emit(event string, payload interface{}) []error {
	if r.ctx == nil || r.ctx.Events == nil {
		return nil
	}
	errs := r.ctx.Events.Emit(event, payload)
	for _, err := range errs {
		fmt.Printf("[extension] handler error on %q: %v\n", event, err)
	}
	return errs
}

// createContext is called by individual emit methods to build the shared
// context fields that most payloads include.
func (r *ExtensionRunner) createContext() *ExtensionCommandContext {
	var sessionID string
	if r.ctx != nil && r.ctx.API != nil && r.ctx.API.GetSessionId != nil {
		sessionID = r.ctx.API.GetSessionId()
	}
	return &ExtensionCommandContext{
		SessionID: sessionID,
		Timestamp: time.Now(),
	}
}

// dropHandlersForExtension removes handlers belonging to the given extension
// from the handler list for the given event type.
func (r *ExtensionRunner) dropHandlersForExtension(et EventType, ext Extension) {
	// Deprecated: handlers are now removed via EventBus.RemoveHandlersByName.
	// This method is kept for backward compatibility with any external callers.
}

// handlerBelongsToExtension checks whether a handler was registered by the
// given extension by looking up the handler token ownership map.
func (r *ExtensionRunner) handlerBelongsToExtension(h eventbus.Handler, ext Extension) bool {
	// Deprecated: function values cannot be compared in Go.
	// Use EventBus.RemoveHandlersByName instead.
	return false
}

// ExtensionCommandContext is the context passed to extension command handlers.
type ExtensionCommandContext struct {
	SessionID string
	Timestamp time.Time
}

// ---------------------------------------------------------------------------
// Session events
// ---------------------------------------------------------------------------

// EmitSessionStart fires the session_start event.
// Returns errors from all handlers.
func (r *ExtensionRunner) EmitSessionStart(sessionID, sessionPath, model string) []error {
	payload := SessionStartPayload{
		SessionId:   sessionID,
		SessionPath: sessionPath,
		Model:       model,
		Timestamp:   time.Now(),
	}
	return r.emit(string(EventSessionStart), payload)
}

// EmitSessionBeforeCompact fires the session_before_compact event.
func (r *ExtensionRunner) EmitSessionBeforeCompact(sessionID string, messageCount int, tokenUsage TokenUsage) []error {
	payload := SessionBeforeCompactPayload{
		SessionId:    sessionID,
		MessageCount: messageCount,
		TokenUsage:   tokenUsage,
		Timestamp:    time.Now(),
	}
	return r.emit(string(EventSessionBeforeCompact), payload)
}

// EmitSessionCompact fires the session_compact event.
func (r *ExtensionRunner) EmitSessionCompact(sessionID string, messagesBefore, messagesAfter, tokensSaved int) []error {
	payload := SessionCompactPayload{
		SessionId:      sessionID,
		MessagesBefore: messagesBefore,
		MessagesAfter:  messagesAfter,
		TokensSaved:    tokensSaved,
		Timestamp:      time.Now(),
	}
	return r.emit(string(EventSessionCompact), payload)
}

// ---------------------------------------------------------------------------
// Context events
// ---------------------------------------------------------------------------

// EmitContext fires the context event.
func (r *ExtensionRunner) EmitContext(sessionID string, messages []Message, tokenUsage TokenUsage) []error {
	payload := ContextPayload{
		SessionId:  sessionID,
		Messages:   messages,
		TokenUsage: tokenUsage,
		Timestamp:  time.Now(),
	}
	return r.emit(string(EventContext), payload)
}

// ---------------------------------------------------------------------------
// Agent events
// ---------------------------------------------------------------------------

// EmitBeforeAgentStart fires the before_agent_start event.
func (r *ExtensionRunner) EmitBeforeAgentStart(sessionID, model string, turn int) []error {
	payload := BeforeAgentStartPayload{
		SessionId: sessionID,
		Model:     model,
		Turn:      turn,
		Timestamp: time.Now(),
	}
	return r.emit(string(EventBeforeAgentStart), payload)
}

// EmitAgentStart fires the agent_start event.
func (r *ExtensionRunner) EmitAgentStart(sessionID, model string, turn int) []error {
	payload := AgentStartPayload{
		SessionId: sessionID,
		Model:     model,
		Turn:      turn,
		Timestamp: time.Now(),
	}
	return r.emit(string(EventAgentStart), payload)
}

// EmitAgentEnd fires the agent_end event.
func (r *ExtensionRunner) EmitAgentEnd(sessionID, model string, turn int, stopReason string, tokenUsage TokenUsage, cost float64, duration time.Duration) []error {
	payload := AgentEndPayload{
		SessionId:  sessionID,
		Model:     model,
		Turn:       turn,
		StopReason: stopReason,
		TokenUsage: tokenUsage,
		Cost:       cost,
		Duration:   duration,
		Timestamp:  time.Now(),
	}
	return r.emit(string(EventAgentEnd), payload)
}

// ---------------------------------------------------------------------------
// Turn events
// ---------------------------------------------------------------------------

// EmitTurnStart fires the turn_start event.
func (r *ExtensionRunner) EmitTurnStart(sessionID string, turn int) []error {
	payload := TurnStartPayload{
		SessionId: sessionID,
		Turn:      turn,
		Timestamp: time.Now(),
	}
	return r.emit(string(EventTurnStart), payload)
}

// EmitTurnEnd fires the turn_end event.
func (r *ExtensionRunner) EmitTurnEnd(sessionID string, turn int, tokenUsage TokenUsage, cost float64, duration time.Duration) []error {
	payload := TurnEndPayload{
		SessionId:  sessionID,
		Turn:       turn,
		TokenUsage: tokenUsage,
		Cost:       cost,
		Duration:   duration,
		Timestamp:  time.Now(),
	}
	return r.emit(string(EventTurnEnd), payload)
}

// ---------------------------------------------------------------------------
// Tool events
// ---------------------------------------------------------------------------

// EmitToolCall fires the tool_call event.
func (r *ExtensionRunner) EmitToolCall(sessionID, toolName, toolUseID string, input map[string]interface{}) []error {
	payload := ToolCallPayload{
		SessionId: sessionID,
		ToolName:  toolName,
		ToolUseID: toolUseID,
		Input:     input,
		Timestamp: time.Now(),
	}
	return r.emit(string(EventToolCall), payload)
}

// EmitToolResult fires the tool_result event.
func (r *ExtensionRunner) EmitToolResult(sessionID, toolName, toolUseID string, result interface{}, isError bool, duration time.Duration) []error {
	payload := ToolResultPayload{
		SessionId: sessionID,
		ToolName:  toolName,
		ToolUseID: toolUseID,
		Result:    result,
		IsError:   isError,
		Duration:  duration,
		Timestamp: time.Now(),
	}
	return r.emit(string(EventToolResult), payload)
}

// EmitToolExecutionStart fires the tool_execution_start event.
func (r *ExtensionRunner) EmitToolExecutionStart(sessionID, toolName, toolUseID string, input map[string]interface{}) []error {
	payload := ToolExecutionStartPayload{
		SessionId: sessionID,
		ToolName:  toolName,
		ToolUseID: toolUseID,
		Input:     input,
		Timestamp: time.Now(),
	}
	return r.emit(string(EventToolExecutionStart), payload)
}

// EmitToolExecutionEnd fires the tool_execution_end event.
func (r *ExtensionRunner) EmitToolExecutionEnd(sessionID, toolName, toolUseID string, result interface{}, isError bool, duration time.Duration) []error {
	payload := ToolExecutionEndPayload{
		SessionId: sessionID,
		ToolName:  toolName,
		ToolUseID: toolUseID,
		Result:    result,
		IsError:   isError,
		Duration:  duration,
		Timestamp: time.Now(),
	}
	return r.emit(string(EventToolExecutionEnd), payload)
}

// ---------------------------------------------------------------------------
// Stream events
// ---------------------------------------------------------------------------

// EmitStreamStart fires the stream_start event.
func (r *ExtensionRunner) EmitStreamStart(sessionID, model string, turn int) []error {
	payload := StreamStartPayload{
		SessionId: sessionID,
		Model:     model,
		Turn:      turn,
		Timestamp: time.Now(),
	}
	return r.emit(string(EventStreamStart), payload)
}

// EmitStreamEnd fires the stream_end event.
func (r *ExtensionRunner) EmitStreamEnd(sessionID, model string, turn int, stopReason string, tokenUsage TokenUsage) []error {
	payload := StreamEndPayload{
		SessionId:  sessionID,
		Model:      model,
		Turn:       turn,
		StopReason: stopReason,
		TokenUsage: tokenUsage,
		Timestamp:  time.Now(),
	}
	return r.emit(string(EventStreamEnd), payload)
}

// ---------------------------------------------------------------------------
// Message and error events
// ---------------------------------------------------------------------------

// EmitMessage fires the message event.
func (r *ExtensionRunner) EmitMessage(sessionID string, msg Message) []error {
	payload := MessagePayload{
		SessionId: sessionID,
		Message:   msg,
		Timestamp: time.Now(),
	}
	return r.emit(string(EventMessage), payload)
}

// EmitError fires the error event.
func (r *ExtensionRunner) EmitError(sessionID, source, errMsg string) []error {
	payload := ErrorPayload{
		SessionId: sessionID,
		Source:    source,
		Message:   errMsg,
		Timestamp: time.Now(),
	}
	return r.emit(string(EventError), payload)
}
