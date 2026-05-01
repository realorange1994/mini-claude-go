package main

import (
	"context"
	"sync"
	"time"
)

// TaskStatus represents the state of a sub-agent task.
type TaskStatus int

const (
	TaskStatusPending   TaskStatus = iota // Task created but not yet started
	TaskStatusRunning                     // Task is actively executing
	TaskStatusCompleted                   // Task finished successfully
	TaskStatusFailed                      // Task encountered an error
	TaskStatusKilled                      // Task was forcibly terminated
)

// TaskState holds the state of a running or completed sub-agent.
type TaskState struct {
	mu              sync.Mutex
	ID              string
	Status          TaskStatus
	Result          string
	Error           string
	Description     string
	Model           string
	SubagentType    string
	ToolsUsed       int
	DurationMs      int64
	StartTime       time.Time
	EndTime         time.Time
	PendingMessages []string
	CancelFunc      context.CancelFunc // cancels the sub-agent's context (for async agents)
	evictAfter      time.Time          // set when task completes; zero means no eviction
}

// AddPendingMessage adds a message to the task's pending message queue.
func (ts *TaskState) AddPendingMessage(message string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.PendingMessages = append(ts.PendingMessages, message)
}

// IsTerminal returns true if the task is in a terminal state.
func (ts *TaskState) IsTerminal() bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.Status == TaskStatusCompleted || ts.Status == TaskStatusFailed || ts.Status == TaskStatusKilled
}

// TaskStore manages all sub-agent tasks for an agent session.
type TaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*TaskState
}

// NewTaskStore creates a new empty task store.
func NewTaskStore() *TaskStore {
	return &TaskStore{
		tasks: make(map[string]*TaskState),
	}
}

// CreateTask registers a new task and returns its initial state.
func (ts *TaskStore) CreateTask(agentID, description, model, subagentType string) *TaskState {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	task := &TaskState{
		ID:           agentID,
		Status:       TaskStatusPending,
		Description:  description,
		Model:        model,
		SubagentType: subagentType,
		StartTime:    time.Now(),
	}
	ts.tasks[agentID] = task
	return task
}

// GetTask returns the task state for a given agent ID, or nil if not found.
func (ts *TaskStore) GetTask(agentID string) *TaskState {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.tasks[agentID]
}

// CompleteTask marks a task as completed with the given result.
func (ts *TaskStore) CompleteTask(agentID string, result string, toolsUsed int, durationMs int64) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if task, ok := ts.tasks[agentID]; ok {
		task.Status = TaskStatusCompleted
		task.Result = result
		task.ToolsUsed = toolsUsed
		task.DurationMs = durationMs
		task.EndTime = time.Now()
		task.evictAfter = time.Now().Add(30 * time.Second)
	}
}

// FailTask marks a task as failed with the given error text.
func (ts *TaskStore) FailTask(agentID string, errText string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if task, ok := ts.tasks[agentID]; ok {
		task.Status = TaskStatusFailed
		task.Error = errText
		task.EndTime = time.Now()
		task.evictAfter = time.Now().Add(30 * time.Second)
	}
}

// CleanupEvicted removes tasks whose evictAfter timestamp has passed.
// Safe to call periodically from a ticker goroutine.
func (ts *TaskStore) CleanupEvicted() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	now := time.Now()
	for id, task := range ts.tasks {
		if !task.evictAfter.IsZero() && now.After(task.evictAfter) {
			delete(ts.tasks, id)
		}
	}
}

// AllTasks returns all tasks ordered by creation time (oldest first).
func (ts *TaskStore) AllTasks() []*TaskState {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	tasks := make([]*TaskState, 0, len(ts.tasks))
	for _, t := range ts.tasks {
		tasks = append(tasks, t)
	}
	// Sort by StartTime
	for i := 0; i < len(tasks)-1; i++ {
		for j := i + 1; j < len(tasks); j++ {
			if tasks[j].StartTime.Before(tasks[i].StartTime) {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}
	return tasks
}

// AddPendingMessage adds a message to the task's pending message queue.
func (ts *TaskStore) AddPendingMessage(agentID string, message string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if task, ok := ts.tasks[agentID]; ok {
		task.PendingMessages = append(task.PendingMessages, message)
	}
}

// DrainPendingMessages returns and clears all pending messages for a task.
func (ts *TaskStore) DrainPendingMessages(agentID string) []string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if task, ok := ts.tasks[agentID]; ok {
		msgs := task.PendingMessages
		task.PendingMessages = nil
		return msgs
	}
	return nil
}

// IsRunning returns true if the task exists and is not in a terminal state.
func (ts *TaskStore) IsRunning(agentID string) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	if task, ok := ts.tasks[agentID]; ok {
		return !task.IsTerminal()
	}
	return false
}

// UpdateStatus sets the status of a task.
func (ts *TaskStore) UpdateStatus(agentID string, status TaskStatus) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if task, ok := ts.tasks[agentID]; ok {
		task.Status = status
	}
}

// Delete removes a task from the store.
func (ts *TaskStore) Delete(agentID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.tasks, agentID)
}
