package main

// ─── Core Interfaces ────────────────────────────────────────────────────────
//
// Core interfaces for the agent system. These interfaces enable:
// - Dependency injection
// - Testability
// - Decoupling between modules

// AgentRunner defines the interface for running an agent loop.
type AgentRunner interface {
	Run(message string) string
	Stop()
	IsInterrupted() bool
	SetInterrupted(v bool)
}

// ContextManager defines the interface for managing conversation context.
type ContextManager interface {
	AddUserMessage(message string)
	AddAssistantText(text string)
	BuildMessages() []interface{}
	EstimatedTokens() int
	PressureLevel(ctxMax int) int
}

// MemoryStore defines the interface for memory operations.
type MemoryStore interface {
	AddNote(category, content, source string)
	SearchMemory(query string, limit int) []interface{}
	FlushToDisk()
	GetRecentEntries(n int) []interface{}
}

// TaskManager defines the interface for task management.
type TaskManager interface {
	CreateTask(subject, description, activeForm string, metadata map[string]any) string
	GetTask(id string) interface{}
	ListTasks() []interface{}
	UpdateTask(id string, updates map[string]any) error
	TransitionTo(id string, status interface{}, reason string) error
}

// ToolExecutor defines the interface for tool execution.
type ToolExecutor interface {
	Execute(toolName string, params map[string]any) (interface{}, error)
	GetTool(name string) (interface{}, bool)
	ListTools() []string
}

// CheckpointManager defines the interface for checkpoint operations.
type CheckpointManager interface {
	WriteCheckpoint(tasks interface{}) (string, error)
	LoadCheckpoint(id string) (interface{}, error)
	ListCheckpoints() []string
	RevertToCheckpoint(id string) error
}

// CompactionManager defines the interface for compaction operations.
type CompactionManager interface {
	Compact(messages []interface{}, config interface{}) (interface{}, error)
	NeedsCompaction(messages []interface{}, config interface{}) bool
	SelectiveCompact(rounds []interface{}, tools map[string]bool, placeholder string) interface{}
}

// SessionManager defines the interface for session operations.
type SessionManager interface {
	ForkSession(checkpointID string) (interface{}, error)
	ForkSessionAtPoint(entries []interface{}, point int) (interface{}, error)
	GetForkInfo() string
}

// DiagnosticCollector defines the interface for collecting diagnostics.
type DiagnosticCollector interface {
	Collect(filePath string) []interface{}
	GetAll() []interface{}
	Clear()
	ClearFile(filePath string)
}

// PluginManager defines the interface for plugin operations.
type PluginManagerInterface interface {
	Load() error
	Register(pluginName string, hook interface{}, fn interface{}) error
	Execute(hook interface{}, ctx interface{}) ([]interface{}, error)
	GetPlugin(name string) interface{}
	ListPlugins() []interface{}
}
