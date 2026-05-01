package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// WorkTaskStatus represents the status of a work task (LLM TODO item).
type WorkTaskStatus string

const (
	WorkTaskPending    WorkTaskStatus = "pending"
	WorkTaskInProgress WorkTaskStatus = "in_progress"
	WorkTaskCompleted  WorkTaskStatus = "completed"
	WorkTaskDeleted    WorkTaskStatus = "deleted"
)

// WorkTask represents a single work item tracked by the LLM.
// This is distinct from TaskState which tracks async sub-agent tasks.
type WorkTask struct {
	ID          string
	Subject     string
	Description string
	ActiveForm  string         // present continuous form for spinner (e.g., "Running tests")
	Status      WorkTaskStatus // pending, in_progress, completed, deleted
	Owner       string         // optional agent ID
	Metadata    map[string]any // arbitrary metadata
	Blocks      []string       // task IDs this task blocks
	BlockedBy   []string       // task IDs that block this task
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WorkTaskStore manages work tasks (LLM TODO items) for an agent session.
// This is separate from TaskStore which manages async sub-agent tasks.
type WorkTaskStore struct {
	mu     sync.RWMutex
	tasks  map[string]*WorkTask
	nextID atomic.Int64
}

// NewWorkTaskStore creates a new empty work task store.
func NewWorkTaskStore() *WorkTaskStore {
	return &WorkTaskStore{
		tasks: make(map[string]*WorkTask),
	}
}

// CreateTask creates a new work task with status "pending" and returns its ID.
func (s *WorkTaskStore) CreateTask(subject, description, activeForm string, metadata map[string]any) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID.Add(1)
	taskID := fmt.Sprintf("%d", id)
	now := time.Now()

	task := &WorkTask{
		ID:          taskID,
		Subject:     subject,
		Description: description,
		ActiveForm:  activeForm,
		Status:      WorkTaskPending,
		Metadata:    metadata,
		Blocks:      []string{},
		BlockedBy:   []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.tasks[taskID] = task
	return taskID
}

// GetTask returns a work task by ID, or nil if not found.
func (s *WorkTaskStore) GetTask(id string) *WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tasks[id]
}

// ListTasks returns all non-deleted tasks, sorted by ID (creation order).
func (s *WorkTaskStore) ListTasks() []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*WorkTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		if t.Status != WorkTaskDeleted {
			tasks = append(tasks, t)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return tasks
}

// UpdateTask updates a work task with the given fields.
// Supported update keys: status, subject, description, activeForm, owner, metadata, addBlocks, addBlockedBy.
func (s *WorkTaskStore) UpdateTask(id string, updates map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}

	// Apply updates
	for key, val := range updates {
		switch key {
		case "status":
			if statusStr, ok := val.(string); ok {
				newStatus := WorkTaskStatus(statusStr)
				// Validate status
				switch newStatus {
				case WorkTaskPending, WorkTaskInProgress, WorkTaskCompleted, WorkTaskDeleted:
					task.Status = newStatus
				default:
					return fmt.Errorf("invalid status: %s", statusStr)
				}
			}
		case "subject":
			if v, ok := val.(string); ok {
				task.Subject = v
			}
		case "description":
			if v, ok := val.(string); ok {
				task.Description = v
			}
		case "activeForm":
			if v, ok := val.(string); ok {
				task.ActiveForm = v
			}
		case "owner":
			if v, ok := val.(string); ok {
				task.Owner = v
			}
		case "metadata":
			if m, ok := val.(map[string]any); ok {
				// Merge metadata: set a key to nil to delete it
				if task.Metadata == nil {
					task.Metadata = make(map[string]any)
				}
				for k, v := range m {
					if v == nil {
						delete(task.Metadata, k)
					} else {
						task.Metadata[k] = v
					}
				}
			}
		case "addBlocks":
			if arr, ok := val.([]any); ok {
				for _, item := range arr {
					depID := fmt.Sprintf("%v", item)
					depID = strings.TrimPrefix(depID, "#")
					if depID == "" {
						continue
					}
					if !containsString(task.Blocks, depID) {
						task.Blocks = append(task.Blocks, depID)
					}
					// Also update the blocked task's BlockedBy
					if blocked, exists := s.tasks[depID]; exists {
						if !containsString(blocked.BlockedBy, id) {
							blocked.BlockedBy = append(blocked.BlockedBy, id)
						}
					}
				}
				// Silently remove references to non-existent tasks
				task.Blocks = s.filterValidDeps(task.Blocks)
			}
		case "addBlockedBy":
			if arr, ok := val.([]any); ok {
				for _, item := range arr {
					depID := fmt.Sprintf("%v", item)
					depID = strings.TrimPrefix(depID, "#")
					if depID == "" {
						continue
					}
					// Skip if adding this edge would create a cycle
					if s.wouldCreateCycle(depID, id) {
						continue
					}
					if !containsString(task.BlockedBy, depID) {
						task.BlockedBy = append(task.BlockedBy, depID)
					}
					// Also update the blocking task's Blocks
					if blocker, exists := s.tasks[depID]; exists {
						if !containsString(blocker.Blocks, id) {
							blocker.Blocks = append(blocker.Blocks, id)
						}
					}
				}
				// Silently remove references to non-existent tasks
				task.BlockedBy = s.filterValidDeps(task.BlockedBy)
			}
		}
	}

	task.UpdatedAt = time.Now()

	// If task is deleted, remove references from other tasks
	if task.Status == WorkTaskDeleted {
		for _, other := range s.tasks {
			other.Blocks = removeString(other.Blocks, id)
			other.BlockedBy = removeString(other.BlockedBy, id)
		}
	}

	return nil
}

// containsString checks if a string slice contains a given string.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// removeString removes all occurrences of a string from a slice.
func removeString(slice []string, s string) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// wouldCreateCycle checks if adding blockerID as a dependency of taskID would create a cycle.
// It searches BOTH BlockedBy and Blocks edges from blockerID. If we reach taskID, the edge creates a cycle.
func (s *WorkTaskStore) wouldCreateCycle(taskID, blockerID string) bool {
	if taskID == blockerID {
		return true
	}
	visited := map[string]bool{}
	queue := []string{blockerID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == taskID {
			return true
		}
		if visited[id] {
			continue
		}
		visited[id] = true
		if t, ok := s.tasks[id]; ok {
			queue = append(queue, t.Blocks...)
			queue = append(queue, t.BlockedBy...)
		}
	}
	return false
}

// filterValidDeps removes IDs that do not correspond to existing tasks from a dependency list.
func (s *WorkTaskStore) filterValidDeps(ids []string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, exists := s.tasks[id]; exists {
			result = append(result, id)
		}
	}
	return result
}
