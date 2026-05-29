package messages

import (
	"encoding/json"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Message roles
// ---------------------------------------------------------------------------

// MessageRole represents the role of a message sender.
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleTool      MessageRole = "tool"
)

// ---------------------------------------------------------------------------
// Message interface
// ---------------------------------------------------------------------------

// Message is the base message interface implemented by all message types.
type Message interface {
	GetRole() MessageRole
	GetContent() string
}

// ---------------------------------------------------------------------------
// TextMessage
// ---------------------------------------------------------------------------

// TextMessage represents a simple text message.
type TextMessage struct {
	role    MessageRole
	content string
}

// NewTextMessage creates a new TextMessage.
func NewTextMessage(role MessageRole, content string) *TextMessage {
	return &TextMessage{role: role, content: content}
}

func (m *TextMessage) GetRole() MessageRole { return m.role }
func (m *TextMessage) GetContent() string   { return m.content }

// Content returns the text content (convenience accessor).
func (m *TextMessage) Content() string { return m.content }

// ---------------------------------------------------------------------------
// ToolMessage — represents a tool-use request from the assistant
// ---------------------------------------------------------------------------

// ToolMessage represents a tool use message (assistant requesting a tool call).
type ToolMessage struct {
	id      string
	name    string
	input   map[string]interface{}
	content string
}

// NewToolMessage creates a new ToolMessage.
func NewToolMessage(id, name string, input map[string]interface{}) *ToolMessage {
	return &ToolMessage{id: id, name: name, input: input}
}

func (m *ToolMessage) GetRole() MessageRole { return RoleAssistant }
func (m *ToolMessage) GetContent() string   { return m.content }

// ID returns the tool call ID.
func (m *ToolMessage) ID() string { return m.id }

// Name returns the tool name.
func (m *ToolMessage) Name() string { return m.name }

// Input returns the tool input parameters.
func (m *ToolMessage) Input() map[string]interface{} { return m.input }

// InputJSON returns the tool input as a JSON string.
func (m *ToolMessage) InputJSON() string {
	b, err := json.Marshal(m.input)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// ToolResultMessage — result returned from a tool execution
// ---------------------------------------------------------------------------

// ToolResultMessage represents a tool result message.
type ToolResultMessage struct {
	toolCallId string
	content    string
	isError    bool
}

// NewToolResultMessage creates a new ToolResultMessage.
func NewToolResultMessage(toolCallId, content string) *ToolResultMessage {
	return &ToolResultMessage{toolCallId: toolCallId, content: content}
}

// NewToolResultMessageWithError creates a ToolResultMessage that indicates an error.
func NewToolResultMessageWithError(toolCallId, content string) *ToolResultMessage {
	return &ToolResultMessage{toolCallId: toolCallId, content: content, isError: true}
}

func (m *ToolResultMessage) GetRole() MessageRole { return RoleTool }
func (m *ToolResultMessage) GetContent() string   { return m.content }

// ToolCallId returns the associated tool call ID.
func (m *ToolResultMessage) ToolCallId() string { return m.toolCallId }

// IsError returns whether this result represents an error.
func (m *ToolResultMessage) IsError() bool { return m.isError }

// ---------------------------------------------------------------------------
// BashExecutionMessage — custom message for bash execution results
// ---------------------------------------------------------------------------

// BashExecutionMessage is a custom message type for bash execution results.
type BashExecutionMessage struct {
	command            string
	stdout             string
	stderr             string
	exitCode           int
	duration           time.Duration
	cancelled          bool
	truncated          bool
	fullOutputPath     string
	excludeFromContext bool // !! prefix
	timestamp          int64
}

// NewBashExecutionMessage creates a new BashExecutionMessage.
func NewBashExecutionMessage(command, stdout, stderr string, exitCode int) *BashExecutionMessage {
	return &BashExecutionMessage{
		command:  command,
		stdout:   stdout,
		stderr:   stderr,
		exitCode: exitCode,
	}
}

func (m *BashExecutionMessage) GetRole() MessageRole { return RoleTool }
func (m *BashExecutionMessage) GetContent() string {
	return m.stdout
}

// Command returns the executed command string.
func (m *BashExecutionMessage) Command() string { return m.command }

// Stdout returns the standard output.
func (m *BashExecutionMessage) Stdout() string { return m.stdout }

// Stderr returns the standard error output.
func (m *BashExecutionMessage) Stderr() string { return m.stderr }

// ExitCode returns the process exit code.
func (m *BashExecutionMessage) ExitCode() int { return m.exitCode }

// Duration returns how long the command ran.
func (m *BashExecutionMessage) Duration() time.Duration { return m.duration }

// SetDuration sets the execution duration.
func (m *BashExecutionMessage) SetDuration(d time.Duration) { m.duration = d }

// IsSuccess returns true if the exit code is 0.
func (m *BashExecutionMessage) IsSuccess() bool { return m.exitCode == 0 }

// Cancelled returns whether the command was cancelled.
func (m *BashExecutionMessage) Cancelled() bool { return m.cancelled }

// SetCancelled sets whether the command was cancelled.
func (m *BashExecutionMessage) SetCancelled(v bool) { m.cancelled = v }

// Truncated returns whether the output was truncated.
func (m *BashExecutionMessage) Truncated() bool { return m.truncated }

// SetTruncated sets whether the output was truncated.
func (m *BashExecutionMessage) SetTruncated(v bool) { m.truncated = v }

// FullOutputPath returns the path to the full output file.
func (m *BashExecutionMessage) FullOutputPath() string { return m.fullOutputPath }

// SetFullOutputPath sets the path to the full output file.
func (m *BashExecutionMessage) SetFullOutputPath(v string) { m.fullOutputPath = v }

// ExcludeFromContext returns whether this message should be excluded from LLM context.
func (m *BashExecutionMessage) ExcludeFromContext() bool { return m.excludeFromContext }

// SetExcludeFromContext sets whether to exclude from LLM context.
func (m *BashExecutionMessage) SetExcludeFromContext(v bool) { m.excludeFromContext = v }

// Timestamp returns the Unix timestamp.
func (m *BashExecutionMessage) GetTimestamp() int64 { return m.timestamp }

// SetTimestamp sets the Unix timestamp.
func (m *BashExecutionMessage) SetTimestamp(v int64) { m.timestamp = v }

// FormattedOutput returns a human-readable summary of the execution.
func (m *BashExecutionMessage) FormattedOutput() string {
	out := fmt.Sprintf("$ %s\n", m.command)
	if m.stdout != "" {
		out += m.stdout
	}
	if m.stderr != "" {
		out += "\n[stderr]\n" + m.stderr
	}
	if m.exitCode != 0 {
		out += fmt.Sprintf("\n[exit code: %d]", m.exitCode)
	}
	return out
}

// ---------------------------------------------------------------------------
// CustomMessage — user-defined custom message
// ---------------------------------------------------------------------------

// CustomMessage is a user-defined custom message with arbitrary data.
type CustomMessage struct {
	name       string
	content    string
	data       map[string]interface{}
	customType string
	display    bool
	details    interface{}
	timestamp  int64
}

// NewCustomMessage creates a new CustomMessage.
func NewCustomMessage(name, content string) *CustomMessage {
	return &CustomMessage{name: name, content: content, data: make(map[string]interface{})}
}

func (m *CustomMessage) GetRole() MessageRole { return RoleUser }
func (m *CustomMessage) GetContent() string   { return m.content }

// Name returns the custom message name.
func (m *CustomMessage) Name() string { return m.name }

// Data returns the custom message data map.
func (m *CustomMessage) Data() map[string]interface{} { return m.data }

// SetData sets a key in the custom message data.
func (m *CustomMessage) SetData(key string, value interface{}) {
	m.data[key] = value
}

// CustomType returns the custom message type identifier.
func (m *CustomMessage) CustomType() string { return m.customType }

// SetCustomType sets the custom message type identifier.
func (m *CustomMessage) SetCustomType(v string) { m.customType = v }

// Display returns whether this message should be displayed.
func (m *CustomMessage) Display() bool { return m.display }

// SetDisplay sets whether this message should be displayed.
func (m *CustomMessage) SetDisplay(v bool) { m.display = v }

// Details returns the custom message details.
func (m *CustomMessage) Details() interface{} { return m.details }

// SetDetails sets the custom message details.
func (m *CustomMessage) SetDetails(v interface{}) { m.details = v }

// GetTimestamp returns the Unix timestamp.
func (m *CustomMessage) GetTimestamp() int64 { return m.timestamp }

// SetTimestamp sets the Unix timestamp.
func (m *CustomMessage) SetTimestamp(v int64) { m.timestamp = v }

// ---------------------------------------------------------------------------
// BranchSummaryMessage — summary of a branch session
// ---------------------------------------------------------------------------

// BranchSummaryMessage holds a summary of a branch session.
type BranchSummaryMessage struct {
	branchId  string
	summary   string
	model     string
	fromId    string
	timestamp int64
}

// NewBranchSummaryMessage creates a new BranchSummaryMessage.
func NewBranchSummaryMessage(branchId, summary, model string) *BranchSummaryMessage {
	return &BranchSummaryMessage{
		branchId:  branchId,
		summary:   summary,
		model:     model,
		timestamp: time.Now().Unix(),
	}
}

func (m *BranchSummaryMessage) GetRole() MessageRole { return RoleAssistant }
func (m *BranchSummaryMessage) GetContent() string   { return m.summary }

// BranchId returns the branch identifier.
func (m *BranchSummaryMessage) BranchId() string { return m.branchId }

// Summary returns the branch summary text.
func (m *BranchSummaryMessage) Summary() string { return m.summary }

// Model returns the model used in the branch.
func (m *BranchSummaryMessage) Model() string { return m.model }

// Timestamp returns the Unix timestamp.
func (m *BranchSummaryMessage) Timestamp() int64 { return m.timestamp }

// FromId returns the source entry ID for the branch.
func (m *BranchSummaryMessage) FromId() string { return m.fromId }

// SetFromId sets the source entry ID for the branch.
func (m *BranchSummaryMessage) SetFromId(v string) { m.fromId = v }

// ---------------------------------------------------------------------------
// CompactionSummaryMessage — summary after context compaction
// ---------------------------------------------------------------------------

// CompactionSummaryMessage holds a summary after context compaction.
type CompactionSummaryMessage struct {
	summary      string
	model        string
	tokenCount   int
	tokensBefore int
	timestamp    int64
}

// NewCompactionSummaryMessage creates a new CompactionSummaryMessage.
func NewCompactionSummaryMessage(summary, model string, tokenCount int) *CompactionSummaryMessage {
	return &CompactionSummaryMessage{
		summary:    summary,
		model:      model,
		tokenCount: tokenCount,
		timestamp:  time.Now().Unix(),
	}
}

func (m *CompactionSummaryMessage) GetRole() MessageRole { return RoleSystem }
func (m *CompactionSummaryMessage) GetContent() string   { return m.summary }

// Summary returns the compaction summary text.
func (m *CompactionSummaryMessage) Summary() string { return m.summary }

// Model returns the model used for compaction.
func (m *CompactionSummaryMessage) Model() string { return m.model }

// TokenCount returns the number of tokens after compaction.
func (m *CompactionSummaryMessage) TokenCount() int { return m.tokenCount }

// Timestamp returns the Unix timestamp.
func (m *CompactionSummaryMessage) Timestamp() int64 { return m.timestamp }

// TokensBefore returns the token count before compaction.
func (m *CompactionSummaryMessage) TokensBefore() int { return m.tokensBefore }

// SetTokensBefore sets the token count before compaction.
func (m *CompactionSummaryMessage) SetTokensBefore(v int) { m.tokensBefore = v }

// ---------------------------------------------------------------------------
// MessageBuilder — helps construct messages for API calls
// ---------------------------------------------------------------------------

// MessageBuilder helps construct a sequence of messages for API calls.
type MessageBuilder struct {
	messages []interface{}
}

// NewMessageBuilder creates a new MessageBuilder.
func NewMessageBuilder() *MessageBuilder {
	return &MessageBuilder{}
}

// Add appends a message to the builder.
func (mb *MessageBuilder) Add(msg interface{}) {
	mb.messages = append(mb.messages, msg)
}

// AddText is a convenience method to add a TextMessage.
func (mb *MessageBuilder) AddText(role MessageRole, content string) {
	mb.Add(NewTextMessage(role, content))
}

// AddSystem is a convenience method to add a system TextMessage.
func (mb *MessageBuilder) AddSystem(content string) {
	mb.AddText(RoleSystem, content)
}

// AddUser is a convenience method to add a user TextMessage.
func (mb *MessageBuilder) AddUser(content string) {
	mb.AddText(RoleUser, content)
}

// AddAssistant is a convenience method to add an assistant TextMessage.
func (mb *MessageBuilder) AddAssistant(content string) {
	mb.AddText(RoleAssistant, content)
}

// GetMessages returns all accumulated messages.
func (mb *MessageBuilder) GetMessages() []interface{} {
	return mb.messages
}

// Len returns the number of messages.
func (mb *MessageBuilder) Len() int {
	return len(mb.messages)
}

// Clear removes all messages.
func (mb *MessageBuilder) Clear() {
	mb.messages = nil
}

// Last returns the most recently added message, or nil if empty.
func (mb *MessageBuilder) Last() interface{} {
	if len(mb.messages) == 0 {
		return nil
	}
	return mb.messages[len(mb.messages)-1]
}
