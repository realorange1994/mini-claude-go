package tools

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"
)

const maxOutputBuffer = 50 * 1024 // 50KB cap for task output buffer

// TaskStatus represents the state of a sub-agent task.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskKilled    TaskStatus = "killed"
)

func (s TaskStatus) String() string {
	return string(s)
}

// IsTerminal returns true if the task is in a terminal (non-running) state.
func (s TaskStatus) IsTerminal() bool {
	return s == TaskCompleted || s == TaskFailed || s == TaskKilled
}

// AgentTask tracks the state of a background sub-agent.
type AgentTask struct {
	mu              sync.Mutex
	ID              string
	Type            string // "local_agent"
	Description     string
	Status          TaskStatus
	SubagentType    string
	Model           string
	Prompt          string
	Output          strings.Builder // captured output, NOT printed to terminal
	StartTime       time.Time
	EndTime         time.Time
	CancelFunc      context.CancelFunc // for kill
	ParentID        string
	Notified        bool
	TranscriptPath  string
	OutputFile     string // path to live output file (written incrementally by taskOutputWriter)
	ToolsUsed       int
	DurationMs      int64
	PendingMessages []string // queued by send_message, drained at turn boundaries
}

// WriteOutput appends text to the task's output buffer, enforcing a size cap.
// When the cap is exceeded, a truncation marker is inserted and the most recent
// content is preserved. The truncation marker is always kept at the start of
// the buffer so callers can see that truncation occurred.
func (t *AgentTask) WriteOutput(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Fast path: no truncation needed
	currentLen := t.Output.Len()
	if currentLen+len(s) <= maxOutputBuffer {
		t.Output.WriteString(s)
		return
	}

	// Cap exceeded. Strategy:
	// 1. Keep up to quarter of existing content at the start
	// 2. Add a truncation marker
	// 3. Append the new string
	// 4. If the total still exceeds cap, trim the new string from the end
	quarter := maxOutputBuffer / 4
	existing := t.Output.String()

	var prefix string
	var truncated int
	if currentLen > quarter {
		prefix = existing[:quarter]
		truncated = currentLen - quarter
	} else {
		prefix = existing
		truncated = 0
	}

	marker := fmt.Sprintf("\n... (%d chars truncated) ...\n", truncated)
	newContent := prefix + marker + s

	// If total still exceeds cap, trim from the end of newContent
	if len(newContent) > maxOutputBuffer {
		over := len(newContent) - maxOutputBuffer
		newContent = newContent[:len(newContent)-over]
	}
	t.Output.Reset()
	t.Output.WriteString(newContent)
}

// GetOutput returns a copy of the task's captured output.
func (t *AgentTask) GetOutput() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Output.String()
}

// AddPendingMessage queues a message for the sub-agent to process at its next turn boundary.
// This implements the main-agent → sub-agent messaging channel.
func (t *AgentTask) AddPendingMessage(msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.PendingMessages = append(t.PendingMessages, msg)
}

// DrainPendingMessages returns and clears all pending messages for the sub-agent.
// Called at tool-round boundaries so the sub-agent can process messages mid-turn.
func (t *AgentTask) DrainPendingMessages() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.PendingMessages) == 0 {
		return nil
	}
	msgs := t.PendingMessages
	t.PendingMessages = nil
	return msgs
}

// IsTerminal returns true if the task is in a terminal (non-running) state.
func (t *AgentTask) IsTerminal() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Status.IsTerminal()
}

// SetTranscriptPath stores the transcript path for the task.
func (t *AgentTask) SetTranscriptPath(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.TranscriptPath = path
}

// GetTranscriptPath returns the transcript path for the task.
func (t *AgentTask) GetTranscriptPath() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.TranscriptPath
}

// SetStatus updates the task's status.
func (t *AgentTask) SetStatus(status TaskStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Status = status
}

// SetToolsInfo updates the task's tool usage count and duration.
func (t *AgentTask) SetToolsInfo(toolsUsed int, durationMs int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ToolsUsed = toolsUsed
	t.DurationMs = durationMs
}

// AgentTaskStore manages background agent tasks with thread-safe access.
type AgentTaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*AgentTask
}

// NewAgentTaskStore creates an empty task store.
func NewAgentTaskStore() *AgentTaskStore {
	return &AgentTaskStore{
		tasks: make(map[string]*AgentTask),
	}
}

// Create registers a new task and returns it. The task ID is an 8-char hex string.
func (ts *AgentTaskStore) Create(description, subagentType, prompt, model string) *AgentTask {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	taskID := generateAgentTaskID()
	task := &AgentTask{
		ID:           taskID,
		Type:         "local_agent",
		Description:  description,
		Status:       TaskPending,
		SubagentType: subagentType,
		Model:        model,
		Prompt:       prompt,
		StartTime:    time.Now(),
	}
	ts.tasks[taskID] = task
	return task
}

// CreateWithID registers a new task with the given ID and returns it.
// Use this when you need a specific ID (e.g., to match an existing task in another store).
func (ts *AgentTaskStore) CreateWithID(id, description, subagentType, prompt, model string) *AgentTask {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	task := &AgentTask{
		ID:           id,
		Type:         "local_agent",
		Description:  description,
		Status:       TaskPending,
		SubagentType: subagentType,
		Model:        model,
		Prompt:       prompt,
		StartTime:    time.Now(),
	}
	ts.tasks[id] = task
	return task
}

// Start marks a task as running and stores its cancel function.
func (ts *AgentTaskStore) Start(id string, cancel context.CancelFunc) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if task, ok := ts.tasks[id]; ok {
		task.Status = TaskRunning
		task.CancelFunc = cancel
	}
}

// Complete marks a task as completed.
func (ts *AgentTaskStore) Complete(id string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if task, ok := ts.tasks[id]; ok {
		task.Status = TaskCompleted
		task.EndTime = time.Now()
	}
}

// Fail marks a task as failed with the given error.
func (ts *AgentTaskStore) Fail(id string, err error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if task, ok := ts.tasks[id]; ok {
		task.Status = TaskFailed
		task.EndTime = time.Now()
		if err != nil {
			task.Output.WriteString("\nError: " + err.Error())
		}
	}
}

// Kill calls the task's CancelFunc and marks it as killed.
// Returns true if the task was found and killed, false otherwise.
func (ts *AgentTaskStore) Kill(id string) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if task, ok := ts.tasks[id]; ok {
		if task.CancelFunc != nil {
			task.CancelFunc()
		}
		task.Status = TaskKilled
		task.EndTime = time.Now()
		return true
	}
	return false
}

// Get returns a task by ID, or nil if not found.
func (ts *AgentTaskStore) Get(id string) *AgentTask {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.tasks[id]
}

// List returns all tasks sorted newest first.
func (ts *AgentTaskStore) List() []*AgentTask {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	tasks := make([]*AgentTask, 0, len(ts.tasks))
	for _, t := range ts.tasks {
		tasks = append(tasks, t)
	}
	// Sort by StartTime descending (newest first)
	for i := 0; i < len(tasks)-1; i++ {
		for j := i + 1; j < len(tasks); j++ {
			if tasks[j].StartTime.After(tasks[i].StartTime) {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}
	return tasks
}

// ListByStatus returns tasks filtered by status, sorted newest first.
func (ts *AgentTaskStore) ListByStatus(status TaskStatus) []*AgentTask {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	var result []*AgentTask
	for _, t := range ts.tasks {
		if t.Status == status {
			result = append(result, t)
		}
	}
	// Sort by StartTime descending (newest first)
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].StartTime.After(result[i].StartTime) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// UpdateTranscriptPath sets the transcript path for a task.
func (ts *AgentTaskStore) UpdateTranscriptPath(id string, path string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if task, ok := ts.tasks[id]; ok {
		task.TranscriptPath = path
	}
}

// Delete removes a task from the store.
func (ts *AgentTaskStore) Delete(id string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.tasks, id)
}

// Count returns the number of tasks in the store.
func (ts *AgentTaskStore) Count() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return len(ts.tasks)
}

// AddPendingMessage queues a message for a specific sub-agent.
// Returns false if the task was not found or is not running.
func (ts *AgentTaskStore) AddPendingMessage(id string, msg string) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	if task, ok := ts.tasks[id]; ok {
		if task.Status == TaskRunning {
			task.AddPendingMessage(msg)
			return true
		}
	}
	return false
}

// DrainPendingMessages returns and clears all pending messages for a specific sub-agent.
// Returns nil if the task was not found.
func (ts *AgentTaskStore) DrainPendingMessages(id string) []string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	if task, ok := ts.tasks[id]; ok {
		return task.DrainPendingMessages()
	}
	return nil
}

// generateAgentTaskID creates a short hex ID (8 chars) for readability.
func generateAgentTaskID() string {
	b := make([]byte, 4) // 4 bytes = 8 hex chars
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based ID if crypto/rand fails
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	}
	return fmt.Sprintf("%08x", b)
}
