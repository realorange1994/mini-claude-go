package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	WorkTaskBlocked    WorkTaskStatus = "blocked"   // waiting on dependency
	WorkTaskCancelled  WorkTaskStatus = "cancelled"  // abandoned without completion
)

// StatusTransition records a status change event.
type StatusTransition struct {
	From      WorkTaskStatus `json:"from"`
	To        WorkTaskStatus `json:"to"`
	Timestamp time.Time      `json:"timestamp"`
	Reason    string         `json:"reason,omitempty"` // optional reason for transition
}

// validTransitions defines the allowed state transitions.
// Key: current status, Value: set of allowed target statuses.
var validTransitions = map[WorkTaskStatus]map[WorkTaskStatus]bool{
	WorkTaskPending: {
		WorkTaskInProgress: true,
		WorkTaskDeleted:    true,
		WorkTaskBlocked:    true,
		WorkTaskCancelled:  true,
	},
	WorkTaskInProgress: {
		WorkTaskCompleted: true,
		WorkTaskPending:   true, // pause: move back to pending
		WorkTaskBlocked:   true,
		WorkTaskDeleted:   true,
		WorkTaskCancelled: true,
	},
	WorkTaskBlocked: {
		WorkTaskPending:   true, // unblock
		WorkTaskInProgress: true,
		WorkTaskDeleted:   true,
		WorkTaskCancelled: true,
	},
	WorkTaskCompleted: {
		WorkTaskPending: true, // reopen
		WorkTaskDeleted: true,
	},
	WorkTaskDeleted: {
		// terminal: no transitions out
	},
	WorkTaskCancelled: {
		WorkTaskPending: true, // restart
		WorkTaskDeleted: true,
	},
}

// WorkTaskPriority represents the priority level of a work task.
type WorkTaskPriority string

const (
	PriorityCritical WorkTaskPriority = "critical" // must be done immediately, blocking other work
	PriorityHigh     WorkTaskPriority = "high"     // important, should be done soon
	PriorityMedium   WorkTaskPriority = "medium"   // normal priority (default)
	PriorityLow      WorkTaskPriority = "low"      // nice to have, do when convenient
)

// priorityOrder maps priorities to numeric values for sorting (higher = more important).
var priorityOrder = map[WorkTaskPriority]int{
	PriorityCritical: 4,
	PriorityHigh:     3,
	PriorityMedium:   2,
	PriorityLow:      1,
	"":               2, // default to medium
}

// WorkTask represents a single work item tracked by the LLM.
// This is distinct from TaskState which tracks async sub-agent tasks.
// Supports hierarchical subtasks via ParentID, tagging, priority,
// time tracking, and workflow state machine.
type WorkTask struct {
	ID          string
	Subject     string
	Description string
	ActiveForm  string           // present continuous form for spinner (e.g., "Running tests")
	Status      WorkTaskStatus   // pending, in_progress, completed, deleted, blocked, cancelled
	Priority    WorkTaskPriority // critical, high, medium, low (default: medium)
	Owner       string           // optional agent ID
	ParentID    string           // parent task ID (empty for top-level tasks)
	Tags        []string         // categorization tags (e.g., "bug", "feature", "urgent")
	Metadata    map[string]any   // arbitrary metadata
	Blocks      []string         // task IDs this task blocks
	BlockedBy   []string         // task IDs that block this task
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Time tracking
	StartedAt   *time.Time        `json:"started_at,omitempty"`   // when status first became in_progress
	CompletedAt *time.Time        `json:"completed_at,omitempty"` // when status became completed/cancelled
	TimeSpent   time.Duration     `json:"time_spent"`             // total time in progress (accumulated across pauses)

	// Workflow state machine
	History     []StatusTransition `json:"history,omitempty"` // full status transition history
}

// WorkTaskStore manages work tasks (LLM TODO items) for an agent session.
// This is separate from TaskStore which manages async sub-agent tasks.
// Tasks are persisted to disk at .claude/tasks.json to survive compaction and restarts.
// ─── Task Event System ──────────────────────────────────────────────────────

// TaskEventType represents the type of task event.
type TaskEventType string

const (
	TaskEventCreated      TaskEventType = "created"       // new task created
	TaskEventUpdated      TaskEventType = "updated"       // task fields updated
	TaskEventStatusChange TaskEventType = "status_change" // status transition
	TaskEventCompleted    TaskEventType = "completed"      // task completed
	TaskEventDeleted      TaskEventType = "deleted"        // task deleted
	TaskEventBlocked      TaskEventType = "blocked"        // task became blocked
	TaskEventUnblocked    TaskEventType = "unblocked"       // task became unblocked
	TaskEventTagAdded     TaskEventType = "tag_added"       // tag added
	TaskEventTagRemoved   TaskEventType = "tag_removed"     // tag removed
	TaskEventPrioritySet  TaskEventType = "priority_set"    // priority changed
	TaskEventDepAdded     TaskEventType = "dep_added"       // dependency added
	TaskEventDepRemoved   TaskEventType = "dep_removed"     // dependency removed
)

// TaskEvent represents a task state change event.
type TaskEvent struct {
	Type      TaskEventType `json:"type"`
	TaskID    string        `json:"task_id"`
	Task      *WorkTask     `json:"task"`       // snapshot of task at event time
	Timestamp time.Time     `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"` // extra event data
}

// TaskEventListener is a callback function for task events.
type TaskEventListener func(event TaskEvent)

// TaskEventFilter allows filtering events before delivery.
type TaskEventFilter func(event TaskEvent) bool

// taskListenerEntry holds a listener with its optional filter.
type taskListenerEntry struct {
	id       string
	listener TaskEventListener
	filter   TaskEventFilter
}

// WorkTaskStore manages work tasks (LLM TODO items) for an agent session.
// This is separate from TaskStore which tracks async sub-agent tasks.
// Tasks are persisted to disk at .claude/tasks.json to survive compaction and restarts.
// Supports event-driven notifications for task state changes.
type WorkTaskStore struct {
	mu        sync.RWMutex
	tasks     map[string]*WorkTask
	nextID    atomic.Int64
	filePath  string
	dirty     bool
	stopCh    chan struct{}
	stopOnce  sync.Once
	wg        sync.WaitGroup

	// Event system
	listeners    []taskListenerEntry
	listenerMu   sync.RWMutex
	nextListener atomic.Int64
}

// NewWorkTaskStore creates a new work task store with disk persistence.
// If projectDir is empty, tasks are not persisted (in-memory only).
func NewWorkTaskStore(projectDir string) *WorkTaskStore {
	s := &WorkTaskStore{
		tasks:  make(map[string]*WorkTask),
		stopCh: make(chan struct{}),
	}

	if projectDir != "" {
		dir := filepath.Join(projectDir, ".claude")
		os.MkdirAll(dir, 0o755)
		s.filePath = filepath.Join(dir, "tasks.json")
		s.loadFromDisk()
	}

	return s
}

// CreateTask creates a new work task with status "pending" and returns its ID.
func (s *WorkTaskStore) CreateTask(subject, description, activeForm string, metadata map[string]any) string {
	s.mu.Lock()

	id := s.nextID.Add(1)
	taskID := fmt.Sprintf("%d", id)
	now := time.Now()

	task := &WorkTask{
		ID:          taskID,
		Subject:     subject,
		Description: description,
		ActiveForm:  activeForm,
		Status:      WorkTaskPending,
		Tags:        []string{},
		Metadata:    metadata,
		Blocks:      []string{},
		BlockedBy:   []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.tasks[taskID] = task
	s.dirty = true

	// Snapshot for event (before releasing lock)
	eventTask := *task
	s.mu.Unlock()

	// Emit event after releasing lock
	s.emitEvent(TaskEvent{
		Type:   TaskEventCreated,
		TaskID: taskID,
		Task:   &eventTask,
		Data:   map[string]any{"subject": subject},
	})

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

// ListActiveTasks returns all pending or in_progress tasks (for post-compact injection).
func (s *WorkTaskStore) ListActiveTasks() []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*WorkTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		if t.Status == WorkTaskPending || t.Status == WorkTaskInProgress {
			tasks = append(tasks, t)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return tasks
}

// CreateSubtask creates a new task as a subtask of the given parent.
// Returns the new task ID, or empty string if parent not found.
func (s *WorkTaskStore) CreateSubtask(parentID, subject, description, activeForm string, metadata map[string]any) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	parent, ok := s.tasks[parentID]
	if !ok {
		return ""
	}

	id := s.nextID.Add(1)
	taskID := fmt.Sprintf("%d", id)
	now := time.Now()

	task := &WorkTask{
		ID:          taskID,
		Subject:     subject,
		Description: description,
		ActiveForm:  activeForm,
		Status:      WorkTaskPending,
		Owner:       parent.Owner, // inherit parent's owner
		ParentID:    parentID,
		Tags:        []string{},
		Metadata:    metadata,
		Blocks:      []string{},
		BlockedBy:   []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.tasks[taskID] = task
	s.dirty = true
	return taskID
}

// ListSubtasks returns all non-deleted subtasks of the given parent task.
func (s *WorkTaskStore) ListSubtasks(parentID string) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var subtasks []*WorkTask
	for _, t := range s.tasks {
		if t.ParentID == parentID && t.Status != WorkTaskDeleted {
			subtasks = append(subtasks, t)
		}
	}
	sort.Slice(subtasks, func(i, j int) bool {
		return subtasks[i].ID < subtasks[j].ID
	})
	return subtasks
}

// ListTopLevelTasks returns all non-deleted tasks without a parent.
func (s *WorkTaskStore) ListTopLevelTasks() []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tasks []*WorkTask
	for _, t := range s.tasks {
		if t.ParentID == "" && t.Status != WorkTaskDeleted {
			tasks = append(tasks, t)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return tasks
}

// AddTag adds a tag to a task. Returns error if task not found.
// Tags are deduplicated — adding an existing tag is a no-op.
func (s *WorkTaskStore) AddTaskTag(id, tag string) error {
	s.mu.Lock()

	task, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task %s not found", id)
	}
	if tag == "" {
		s.mu.Unlock()
		return nil
	}
	if containsString(task.Tags, tag) {
		s.mu.Unlock()
		return nil // already has this tag
	}
	task.Tags = append(task.Tags, tag)
	task.UpdatedAt = time.Now()
	s.dirty = true
	eventTask := *task
	s.mu.Unlock()

	s.emitEvent(TaskEvent{
		Type:   TaskEventTagAdded,
		TaskID: id,
		Task:   &eventTask,
		Data:   map[string]any{"tag": tag},
	})
	return nil
}

// RemoveTag removes a tag from a task. Returns error if task not found.
func (s *WorkTaskStore) RemoveTaskTag(id, tag string) error {
	s.mu.Lock()

	task, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task %s not found", id)
	}
	task.Tags = removeString(task.Tags, tag)
	task.UpdatedAt = time.Now()
	s.dirty = true
	eventTask := *task
	s.mu.Unlock()

	s.emitEvent(TaskEvent{
		Type:   TaskEventTagRemoved,
		TaskID: id,
		Task:   &eventTask,
		Data:   map[string]any{"tag": tag},
	})
	return nil
}

// SetTags replaces all tags on a task. Returns error if task not found.
func (s *WorkTaskStore) SetTaskTags(id string, tags []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	// Deduplicate
	seen := make(map[string]bool)
	deduped := make([]string, 0, len(tags))
	for _, t := range tags {
		if t != "" && !seen[t] {
			seen[t] = true
			deduped = append(deduped, t)
		}
	}
	task.Tags = deduped
	task.UpdatedAt = time.Now()
	s.dirty = true
	return nil
}

// FilterByTag returns all non-deleted tasks that have the specified tag.
func (s *WorkTaskStore) FilterByTag(tag string) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*WorkTask
	for _, t := range s.tasks {
		if t.Status != WorkTaskDeleted && containsString(t.Tags, tag) {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// FilterByTags returns all non-deleted tasks that have ALL of the specified tags.
func (s *WorkTaskStore) FilterByTags(tags []string) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*WorkTask
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		hasAll := true
		for _, tag := range tags {
			if !containsString(t.Tags, tag) {
				hasAll = false
				break
			}
		}
		if hasAll {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// ListAllTags returns all unique tags across all non-deleted tasks, sorted alphabetically.
func (s *WorkTaskStore) ListAllTags() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]bool)
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		for _, tag := range t.Tags {
			seen[tag] = true
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// ListTasksByTagGroup returns tasks grouped by tag. Returns a map of tag -> tasks.
func (s *WorkTaskStore) ListTasksByTagGroup() map[string][]*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]*WorkTask)
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		for _, tag := range t.Tags {
			result[tag] = append(result[tag], t)
		}
	}
	// Sort each group
	for tag := range result {
		sort.Slice(result[tag], func(i, j int) bool {
			return result[tag][i].ID < result[tag][j].ID
		})
	}
	return result
}

// TagStats returns the count of tasks per tag.
func (s *WorkTaskStore) TagStats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]int)
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		for _, tag := range t.Tags {
			result[tag]++
		}
	}
	return result
}

// SetTaskPriority sets the priority of a task. Returns error if task not found.
func (s *WorkTaskStore) SetTaskPriority(id string, priority WorkTaskPriority) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if !isValidPriority(priority) {
		return fmt.Errorf("invalid priority: %s", priority)
	}
	task.Priority = priority
	task.UpdatedAt = time.Now()
	s.dirty = true
	return nil
}

// ─── Batch Operations ───────────────────────────────────────────────────────

// BatchResult holds the result of a batch operation.
type BatchResult struct {
	Succeeded []string `json:"succeeded"` // IDs that succeeded
	Failed    []BatchError `json:"failed"`    // IDs that failed with reasons
}

// BatchError pairs a task ID with its failure reason.
type BatchError struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// BatchTransition transitions multiple tasks to a new status.
// Returns results for each task (succeeded/failed).
func (s *WorkTaskStore) BatchTransition(ids []string, newStatus WorkTaskStatus, reason string) BatchResult {
	result := BatchResult{}
	for _, id := range ids {
		err := s.TransitionTo(id, newStatus, reason)
		if err != nil {
			result.Failed = append(result.Failed, BatchError{ID: id, Reason: err.Error()})
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}
	return result
}

// BatchComplete completes multiple tasks (pending -> in_progress -> completed).
func (s *WorkTaskStore) BatchComplete(ids []string) BatchResult {
	result := BatchResult{}
	for _, id := range ids {
		// Try to transition: current -> in_progress -> completed
		_ = s.TransitionTo(id, WorkTaskInProgress, "batch complete")
		err := s.TransitionTo(id, WorkTaskCompleted, "batch completed")
		if err != nil {
			result.Failed = append(result.Failed, BatchError{ID: id, Reason: err.Error()})
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}
	return result
}

// BatchDelete deletes multiple tasks.
func (s *WorkTaskStore) BatchDelete(ids []string) BatchResult {
	result := BatchResult{}
	for _, id := range ids {
		err := s.TransitionTo(id, WorkTaskDeleted, "batch deleted")
		if err != nil {
			result.Failed = append(result.Failed, BatchError{ID: id, Reason: err.Error()})
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}
	return result
}

// BatchSetPriority sets priority for multiple tasks.
func (s *WorkTaskStore) BatchSetPriority(ids []string, priority WorkTaskPriority) BatchResult {
	result := BatchResult{}
	for _, id := range ids {
		err := s.SetTaskPriority(id, priority)
		if err != nil {
			result.Failed = append(result.Failed, BatchError{ID: id, Reason: err.Error()})
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}
	return result
}

// BatchAddTag adds a tag to multiple tasks.
func (s *WorkTaskStore) BatchAddTag(ids []string, tag string) BatchResult {
	result := BatchResult{}
	for _, id := range ids {
		err := s.AddTaskTag(id, tag)
		if err != nil {
			result.Failed = append(result.Failed, BatchError{ID: id, Reason: err.Error()})
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}
	return result
}

// BatchRemoveTag removes a tag from multiple tasks.
func (s *WorkTaskStore) BatchRemoveTag(ids []string, tag string) BatchResult {
	result := BatchResult{}
	for _, id := range ids {
		err := s.RemoveTaskTag(id, tag)
		if err != nil {
			result.Failed = append(result.Failed, BatchError{ID: id, Reason: err.Error()})
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}
	return result
}

// BatchSetOwner sets owner for multiple tasks.
func (s *WorkTaskStore) BatchSetOwner(ids []string, owner string) BatchResult {
	result := BatchResult{}
	for _, id := range ids {
		err := s.UpdateTask(id, map[string]any{"owner": owner})
		if err != nil {
			result.Failed = append(result.Failed, BatchError{ID: id, Reason: err.Error()})
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}
	return result
}

// BatchUpdate applies the same update to multiple tasks.
func (s *WorkTaskStore) BatchUpdate(ids []string, updates map[string]any) BatchResult {
	result := BatchResult{}
	for _, id := range ids {
		err := s.UpdateTask(id, updates)
		if err != nil {
			result.Failed = append(result.Failed, BatchError{ID: id, Reason: err.Error()})
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}
	return result
}

// BatchAddDependency adds a dependency for multiple tasks (all blocked by blockerID).
func (s *WorkTaskStore) BatchAddDependency(ids []string, blockerID string) BatchResult {
	result := BatchResult{}
	for _, id := range ids {
		err := s.AddDependency(id, blockerID)
		if err != nil {
			result.Failed = append(result.Failed, BatchError{ID: id, Reason: err.Error()})
		} else {
			result.Succeeded = append(result.Succeeded, id)
		}
	}
	return result
}

// BatchByFilter applies a batch operation to all tasks matching a filter.
// This is a convenience method that combines FilterTasks + batch operation.
func (s *WorkTaskStore) BatchByFilter(filter TaskFilter, operation string, params map[string]any) BatchResult {
	tasks := s.FilterTasks(filter)
	ids := make([]string, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}

	switch operation {
	case "complete":
		return s.BatchComplete(ids)
	case "delete":
		return s.BatchDelete(ids)
	case "transition":
		if status, ok := params["status"].(string); ok {
			reason, _ := params["reason"].(string)
			return s.BatchTransition(ids, WorkTaskStatus(status), reason)
		}
		return BatchResult{Failed: []BatchError{{Reason: "missing status parameter"}}}
	case "set_priority":
		if priority, ok := params["priority"].(string); ok {
			return s.BatchSetPriority(ids, WorkTaskPriority(priority))
		}
		return BatchResult{Failed: []BatchError{{Reason: "missing priority parameter"}}}
	case "add_tag":
		if tag, ok := params["tag"].(string); ok {
			return s.BatchAddTag(ids, tag)
		}
		return BatchResult{Failed: []BatchError{{Reason: "missing tag parameter"}}}
	case "remove_tag":
		if tag, ok := params["tag"].(string); ok {
			return s.BatchRemoveTag(ids, tag)
		}
		return BatchResult{Failed: []BatchError{{Reason: "missing tag parameter"}}}
	case "set_owner":
		if owner, ok := params["owner"].(string); ok {
			return s.BatchSetOwner(ids, owner)
		}
		return BatchResult{Failed: []BatchError{{Reason: "missing owner parameter"}}}
	default:
		return BatchResult{Failed: []BatchError{{Reason: fmt.Sprintf("unknown operation: %s", operation)}}}
	}
}

// FormatBatchResult returns a human-readable summary of a batch operation.
func FormatBatchResult(result BatchResult, operation string) string {
	var sb strings.Builder
	total := len(result.Succeeded) + len(result.Failed)

	sb.WriteString(fmt.Sprintf("Batch %s: %d/%d succeeded", operation, len(result.Succeeded), total))

	if len(result.Failed) > 0 {
		sb.WriteString(fmt.Sprintf(", %d failed:\n", len(result.Failed)))
		for _, f := range result.Failed {
			sb.WriteString(fmt.Sprintf("  #%s: %s\n", f.ID, f.Reason))
		}
	} else {
		sb.WriteString(".\n")
	}

	return sb.String()
}

// FilterByPriority returns all non-deleted tasks with the specified priority.
func (s *WorkTaskStore) FilterByPriority(priority WorkTaskPriority) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*WorkTask
	for _, t := range s.tasks {
		if t.Status != WorkTaskDeleted && t.Priority == priority {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// ListTasksByPriority returns all non-deleted tasks sorted by priority (critical first).
func (s *WorkTaskStore) ListTasksByPriority() []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*WorkTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		if t.Status != WorkTaskDeleted {
			tasks = append(tasks, t)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		pi := priorityOrder[tasks[i].Priority]
		pj := priorityOrder[tasks[j].Priority]
		if pi != pj {
			return pi > pj // higher priority first
		}
		return tasks[i].ID < tasks[j].ID // then by creation order
	})
	return tasks
}

// ListActiveByPriority returns active (pending/in_progress) tasks sorted by priority.
func (s *WorkTaskStore) ListActiveByPriority() []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*WorkTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		if t.Status == WorkTaskPending || t.Status == WorkTaskInProgress {
			tasks = append(tasks, t)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		pi := priorityOrder[tasks[i].Priority]
		pj := priorityOrder[tasks[j].Priority]
		if pi != pj {
			return pi > pj
		}
		return tasks[i].ID < tasks[j].ID
	})
	return tasks
}

// PriorityStats returns the count of tasks per priority level.
func (s *WorkTaskStore) PriorityStats() map[WorkTaskPriority]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[WorkTaskPriority]int)
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		p := t.Priority
		if p == "" {
			p = PriorityMedium
		}
		result[p]++
	}
	return result
}

// GetHighestPriorityTask returns the highest priority active task, or nil if none.
func (s *WorkTaskStore) GetHighestPriorityTask() *WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var best *WorkTask
	bestPri := 0
	for _, t := range s.tasks {
		if t.Status != WorkTaskPending && t.Status != WorkTaskInProgress {
			continue
		}
		p := priorityOrder[t.Priority]
		if p > bestPri || (p == bestPri && (best == nil || t.ID < best.ID)) {
			best = t
			bestPri = p
		}
	}
	return best
}

// FilterByPriorityAndTag returns tasks matching both priority and tag.
func (s *WorkTaskStore) FilterByPriorityAndTag(priority WorkTaskPriority, tag string) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*WorkTask
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		if t.Priority != priority {
			continue
		}
		if !containsString(t.Tags, tag) {
			continue
		}
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// FilterByStatus returns all tasks with the specified status.
func (s *WorkTaskStore) FilterByStatus(status WorkTaskStatus) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*WorkTask
	for _, t := range s.tasks {
		if t.Status == status {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// SearchTasks performs a full-text search across task subject, description, and tags.
// Case-insensitive substring matching.
func (s *WorkTaskStore) SearchTasks(query string) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lower := strings.ToLower(query)
	var result []*WorkTask
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		if s.taskMatchesQuery(t, lower) {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		// Rank by relevance: subject match > description match > tag match
		ri := s.searchRelevance(result[i], lower)
		rj := s.searchRelevance(result[j], lower)
		if ri != rj {
			return ri > rj
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// taskMatchesQuery checks if a task matches a search query. Caller must hold lock.
func (s *WorkTaskStore) taskMatchesQuery(t *WorkTask, query string) bool {
	if strings.Contains(strings.ToLower(t.Subject), query) {
		return true
	}
	if strings.Contains(strings.ToLower(t.Description), query) {
		return true
	}
	if strings.Contains(strings.ToLower(t.ActiveForm), query) {
		return true
	}
	for _, tag := range t.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	if strings.Contains(strings.ToLower(string(t.Priority)), query) {
		return true
	}
	if strings.Contains(strings.ToLower(string(t.Status)), query) {
		return true
	}
	return false
}

// searchRelevance returns a relevance score for search ranking. Higher = more relevant.
func (s *WorkTaskStore) searchRelevance(t *WorkTask, query string) int {
	score := 0
	if strings.Contains(strings.ToLower(t.Subject), query) {
		score += 10
	}
	if strings.Contains(strings.ToLower(t.Description), query) {
		score += 5
	}
	for _, tag := range t.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			score += 3
		}
	}
	if strings.Contains(strings.ToLower(t.ActiveForm), query) {
		score += 1
	}
	return score
}

// TaskFilter represents a combined filter criteria for tasks.
type TaskFilter struct {
	Statuses   []WorkTaskStatus   `json:"statuses,omitempty"`
	Priorities []WorkTaskPriority `json:"priorities,omitempty"`
	Tags       []string           `json:"tags,omitempty"`    // ALL tags must match
	TagsAny    []string           `json:"tags_any,omitempty"` // ANY tag must match
	Owner      string             `json:"owner,omitempty"`
	Query      string             `json:"query,omitempty"`     // full-text search
	ParentID   *string            `json:"parent_id,omitempty"` // nil = any, "" = top-level, "id" = specific
	Blocked    *bool              `json:"blocked,omitempty"`   // nil = any, true = blocked only, false = unblocked only
	HasDeps    *bool              `json:"has_deps,omitempty"`  // nil = any, true = has dependencies, false = no dependencies
	SortBy     string             `json:"sort_by,omitempty"`   // "id", "priority", "status", "created", "updated"
	SortDesc   bool               `json:"sort_desc,omitempty"`
	Limit      int                `json:"limit,omitempty"`
}

// FilterTasks returns tasks matching all specified filter criteria.
func (s *WorkTaskStore) FilterTasks(filter TaskFilter) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*WorkTask
	query := strings.ToLower(filter.Query)

	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}

		// Status filter
		if len(filter.Statuses) > 0 {
			matched := false
			for _, status := range filter.Statuses {
				if t.Status == status {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Priority filter
		if len(filter.Priorities) > 0 {
			matched := false
			pri := t.Priority
			if pri == "" {
				pri = PriorityMedium
			}
			for _, p := range filter.Priorities {
				if pri == p {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Tags filter (ALL must match)
		if len(filter.Tags) > 0 {
			hasAll := true
			for _, tag := range filter.Tags {
				if !containsString(t.Tags, tag) {
					hasAll = false
					break
				}
			}
			if !hasAll {
				continue
			}
		}

		// TagsAny filter (ANY must match)
		if len(filter.TagsAny) > 0 {
			hasAny := false
			for _, tag := range filter.TagsAny {
				if containsString(t.Tags, tag) {
					hasAny = true
					break
				}
			}
			if !hasAny {
				continue
			}
		}

		// Owner filter
		if filter.Owner != "" && t.Owner != filter.Owner {
			continue
		}

		// ParentID filter
		if filter.ParentID != nil {
			if *filter.ParentID == "" {
				// Top-level only
				if t.ParentID != "" {
					continue
				}
			} else {
				if t.ParentID != *filter.ParentID {
					continue
				}
			}
		}

		// Blocked filter
		if filter.Blocked != nil {
			isBlocked := len(t.BlockedBy) > 0
			if *filter.Blocked != isBlocked {
				continue
			}
		}

		// Has dependencies filter
		if filter.HasDeps != nil {
			hasDeps := len(t.Blocks) > 0 || len(t.BlockedBy) > 0
			if *filter.HasDeps != hasDeps {
				continue
			}
		}

		// Full-text search
		if query != "" {
			if !s.taskMatchesQuery(t, query) {
				continue
			}
		}

		result = append(result, t)
	}

	// Sort
	s.sortTasks(result, filter.SortBy, filter.SortDesc)

	// Limit
	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}

	return result
}

// sortTasks sorts tasks by the specified field. Caller must NOT hold lock (called from FilterTasks which already released).
func (s *WorkTaskStore) sortTasks(tasks []*WorkTask, sortBy string, desc bool) {
	less := func(i, j int) bool {
		switch sortBy {
		case "priority":
			pi := priorityOrder[tasks[i].Priority]
			pj := priorityOrder[tasks[j].Priority]
			if pi != pj {
				if desc {
					return pi < pj
				}
				return pi > pj
			}
		case "status":
			si := statusOrder(tasks[i].Status)
			sj := statusOrder(tasks[j].Status)
			if si != sj {
				if desc {
					return si > sj
				}
				return si < sj
			}
		case "created":
			if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
				if desc {
					return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
				}
				return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
			}
		case "updated":
			if !tasks[i].UpdatedAt.Equal(tasks[j].UpdatedAt) {
				if desc {
					return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
				}
				return tasks[i].UpdatedAt.Before(tasks[j].UpdatedAt)
			}
		default: // "id" or empty
			if desc {
				return tasks[i].ID > tasks[j].ID
			}
			return tasks[i].ID < tasks[j].ID
		}
		return tasks[i].ID < tasks[j].ID
	}
	sort.Slice(tasks, less)
}

// CountByStatus returns the count of tasks per status.
func (s *WorkTaskStore) CountByStatus() map[WorkTaskStatus]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[WorkTaskStatus]int)
	for _, t := range s.tasks {
		result[t.Status]++
	}
	return result
}

// CountByPriority returns the count of tasks per priority.
func (s *WorkTaskStore) CountByPriority() map[WorkTaskPriority]int {
	return s.PriorityStats()
}

// GetTasksByOwner returns all tasks owned by the specified agent.
func (s *WorkTaskStore) GetTasksByOwner(owner string) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*WorkTask
	for _, t := range s.tasks {
		if t.Status != WorkTaskDeleted && t.Owner == owner {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// GetRecentTasks returns the N most recently updated tasks.
func (s *WorkTaskStore) GetRecentTasks(n int) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tasks []*WorkTask
	for _, t := range s.tasks {
		if t.Status != WorkTaskDeleted {
			tasks = append(tasks, t)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})
	if n > 0 && len(tasks) > n {
		tasks = tasks[:n]
	}
	return tasks
}

// GetStaleTasks returns tasks that haven't been updated for the specified duration.
func (s *WorkTaskStore) GetStaleTasks(duration time.Duration) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-duration)
	var result []*WorkTask
	for _, t := range s.tasks {
		if t.Status != WorkTaskDeleted && t.Status != WorkTaskCompleted && t.UpdatedAt.Before(cutoff) {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.Before(result[j].UpdatedAt)
	})
	return result
}

// FormatSearchResults returns a formatted string of search results with highlights.
func (s *WorkTaskStore) FormatSearchResults(query string) string {
	results := s.SearchTasks(query)
	if len(results) == 0 {
		return fmt.Sprintf("No tasks matching '%s'.", query)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for '%s' (%d matches):\n\n", query, len(results)))

	for _, t := range results {
		sb.WriteString(fmt.Sprintf("  [%s] #%s: %s", s.statusIcon(t.Status), t.ID, t.Subject))
		if t.Priority != "" && t.Priority != PriorityMedium {
			sb.WriteString(fmt.Sprintf(" (%s)", t.Priority))
		}
		if len(t.Tags) > 0 {
			sb.WriteString(fmt.Sprintf(" [%s]", strings.Join(t.Tags, ", ")))
		}
		sb.WriteString("\n")
		if t.Description != "" {
			desc := t.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			sb.WriteString(fmt.Sprintf("      %s\n", desc))
		}
	}

	return sb.String()
}

func isValidPriority(p WorkTaskPriority) bool {
	switch p {
	case PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow:
		return true
	default:
		return false
	}
}

// ─── Time Tracking ──────────────────────────────────────────────────────────

// GetDuration returns the total elapsed time since task creation.
func (t *WorkTask) GetDuration() time.Duration {
	if t.CompletedAt != nil {
		return t.CompletedAt.Sub(t.CreatedAt)
	}
	return time.Since(t.CreatedAt)
}

// GetActiveTime returns the total time spent in "in_progress" status.
// Includes currently ongoing time if task is in_progress.
func (t *WorkTask) GetActiveTime() time.Duration {
	active := t.TimeSpent
	if t.Status == WorkTaskInProgress && t.StartedAt != nil {
		active += time.Since(*t.StartedAt)
	}
	return active
}

// GetIdleTime returns time spent NOT in progress (pending + blocked).
func (t *WorkTask) GetIdleTime() time.Duration {
	return t.GetDuration() - t.GetActiveTime()
}

// TimeInStatus returns how long the task has been in its current status.
func (t *WorkTask) TimeInStatus() time.Duration {
	if len(t.History) == 0 {
		return time.Since(t.CreatedAt)
	}
	last := t.History[len(t.History)-1]
	return time.Since(last.Timestamp)
}

// IsOverdue returns true if the task has been in_progress longer than the threshold.
func (t *WorkTask) IsOverdue(threshold time.Duration) bool {
	return t.Status == WorkTaskInProgress && t.GetActiveTime() > threshold
}

// StatusHistorySummary returns a human-readable summary of status transitions.
func (t *WorkTask) StatusHistorySummary() string {
	if len(t.History) == 0 {
		return fmt.Sprintf("Created %s ago, status: %s", roundDuration(time.Since(t.CreatedAt)), t.Status)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Task %s: %s\n", t.ID, t.Subject))
	for _, h := range t.History {
		reason := ""
		if h.Reason != "" {
			reason = fmt.Sprintf(" (%s)", h.Reason)
		}
		sb.WriteString(fmt.Sprintf("  %s: %s -> %s%s\n",
			h.Timestamp.Format("15:04:05"), h.From, h.To, reason))
	}
	sb.WriteString(fmt.Sprintf("  Active time: %s, Idle time: %s\n",
		roundDuration(t.GetActiveTime()), roundDuration(t.GetIdleTime())))
	return sb.String()
}

// roundDuration rounds a duration to the nearest second.
func roundDuration(d time.Duration) time.Duration {
	return d.Round(time.Second)
}

// ─── Workflow State Machine ─────────────────────────────────────────────────

// CanTransitionTo checks if a status transition is valid.
func (s *WorkTaskStore) CanTransitionTo(id string, newStatus WorkTaskStatus) (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return false, fmt.Sprintf("task %s not found", id)
	}

	if allowed, ok := validTransitions[task.Status]; !ok || !allowed[newStatus] {
		return false, fmt.Sprintf("cannot transition from %s to %s", task.Status, newStatus)
	}
	return true, ""
}

// TransitionTo performs a validated status transition with time tracking.
// Returns error if transition is invalid.
func (s *WorkTaskStore) TransitionTo(id string, newStatus WorkTaskStatus, reason string) error {
	s.mu.Lock()

	oldStatus := ""
	if task, ok := s.tasks[id]; ok {
		oldStatus = string(task.Status)
	}

	err := s.transitionToLocked(id, newStatus, reason)
	if err != nil {
		s.mu.Unlock()
		return err
	}

	// Snapshot for event
	task := s.tasks[id]
	eventTask := *task
	s.mu.Unlock()

	// Emit status change event
	eventType := TaskEventStatusChange
	if newStatus == WorkTaskCompleted {
		eventType = TaskEventCompleted
	} else if newStatus == WorkTaskDeleted {
		eventType = TaskEventDeleted
	} else if newStatus == WorkTaskBlocked {
		eventType = TaskEventBlocked
	}

	s.emitEvent(TaskEvent{
		Type:   eventType,
		TaskID: id,
		Task:   &eventTask,
		Data: map[string]any{
			"old_status": oldStatus,
			"new_status": string(newStatus),
			"reason":     reason,
		},
	})

	return nil
}

// transitionToLocked performs a validated status transition. Caller must hold write lock.
func (s *WorkTaskStore) transitionToLocked(id string, newStatus WorkTaskStatus, reason string) error {
	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}

	// Validate transition
	if allowed, ok := validTransitions[task.Status]; !ok || !allowed[newStatus] {
		return fmt.Errorf("invalid transition: %s -> %s", task.Status, newStatus)
	}

	now := time.Now()

	// Time tracking: accumulate time when leaving in_progress
	if task.Status == WorkTaskInProgress && task.StartedAt != nil {
		task.TimeSpent += now.Sub(*task.StartedAt)
		task.StartedAt = nil
	}

	// Record transition in history
	task.History = append(task.History, StatusTransition{
		From:      task.Status,
		To:        newStatus,
		Timestamp: now,
		Reason:    reason,
	})

	// Apply new status
	task.Status = newStatus
	task.UpdatedAt = now

	// Time tracking: record start time when entering in_progress
	if newStatus == WorkTaskInProgress {
		task.StartedAt = &now
	}

	// Record completion time
	if newStatus == WorkTaskCompleted || newStatus == WorkTaskCancelled {
		task.CompletedAt = &now
	}

	// Clear completion time if reopening
	if task.CompletedAt != nil && (newStatus == WorkTaskPending || newStatus == WorkTaskInProgress) {
		task.CompletedAt = nil
	}

	// Cleanup on delete
	if newStatus == WorkTaskDeleted {
		for _, other := range s.tasks {
			other.Blocks = removeString(other.Blocks, id)
			other.BlockedBy = removeString(other.BlockedBy, id)
		}
	}

	s.dirty = true
	return nil
}

// GetValidTransitions returns the list of statuses the task can transition to.
func (s *WorkTaskStore) GetValidTransitions(id string) []WorkTaskStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil
	}

	allowed, ok := validTransitions[task.Status]
	if !ok {
		return nil
	}

	var transitions []WorkTaskStatus
	for status := range allowed {
		transitions = append(transitions, status)
	}
	sort.Slice(transitions, func(i, j int) bool {
		return string(transitions[i]) < string(transitions[j])
	})
	return transitions
}

// GetTransitionHistory returns the full transition history for a task.
func (s *WorkTaskStore) GetTransitionHistory(id string) []StatusTransition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil
	}
	result := make([]StatusTransition, len(task.History))
	copy(result, task.History)
	return result
}

// GetTimeReport returns a time tracking report for a task.
func (s *WorkTaskStore) GetTimeReport(id string) *TimeReport {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil
	}

	report := &TimeReport{
		TaskID:      task.ID,
		Subject:     task.Subject,
		Status:      task.Status,
		CreatedAt:   task.CreatedAt,
		StartedAt:   task.StartedAt,
		CompletedAt: task.CompletedAt,
		TotalTime:   task.GetDuration(),
		ActiveTime:  task.GetActiveTime(),
		IdleTime:    task.GetIdleTime(),
		Transitions: len(task.History),
	}

	// Calculate time per status
	report.TimePerStatus = make(map[WorkTaskStatus]time.Duration)
	for i, h := range task.History {
		var end time.Time
		if i+1 < len(task.History) {
			end = task.History[i+1].Timestamp
		} else if task.CompletedAt != nil {
			end = *task.CompletedAt
		} else {
			end = time.Now()
		}
		report.TimePerStatus[h.To] += end.Sub(h.Timestamp)
	}

	return report
}

// TimeReport holds time tracking information for a task.
type TimeReport struct {
	TaskID        string                    `json:"task_id"`
	Subject       string                    `json:"subject"`
	Status        WorkTaskStatus            `json:"status"`
	CreatedAt     time.Time                 `json:"created_at"`
	StartedAt     *time.Time                `json:"started_at,omitempty"`
	CompletedAt   *time.Time                `json:"completed_at,omitempty"`
	TotalTime     time.Duration             `json:"total_time"`
	ActiveTime    time.Duration             `json:"active_time"`
	IdleTime      time.Duration             `json:"idle_time"`
	Transitions   int                       `json:"transitions"`
	TimePerStatus map[WorkTaskStatus]time.Duration `json:"time_per_status"`
}

// GetStoreTimeReport returns aggregated time tracking for all tasks.
func (s *WorkTaskStore) GetStoreTimeReport() *StoreTimeReport {
	s.mu.RLock()
	defer s.mu.RUnlock()

	report := &StoreTimeReport{
		TimePerStatus: make(map[WorkTaskStatus]time.Duration),
		TimePerPriority: make(map[WorkTaskPriority]time.Duration),
	}

	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		report.TotalTasks++
		report.TotalActiveTime += t.GetActiveTime()
		report.TotalIdleTime += t.GetIdleTime()

		// Aggregate by status
		report.TimePerStatus[t.Status] += t.GetActiveTime()

		// Aggregate by priority
		p := t.Priority
		if p == "" {
			p = PriorityMedium
		}
		report.TimePerPriority[p] += t.GetActiveTime()

		// Track overdue tasks
		if t.IsOverdue(30 * time.Minute) {
			report.OverdueTasks++
		}
	}

	return report
}

// StoreTimeReport holds aggregated time tracking for all tasks.
type StoreTimeReport struct {
	TotalTasks      int                            `json:"total_tasks"`
	TotalActiveTime time.Duration                  `json:"total_active_time"`
	TotalIdleTime   time.Duration                  `json:"total_idle_time"`
	OverdueTasks    int                            `json:"overdue_tasks"`
	TimePerStatus   map[WorkTaskStatus]time.Duration `json:"time_per_status"`
	TimePerPriority map[WorkTaskPriority]time.Duration `json:"time_per_priority"`
}

// ─── Dependency Management ──────────────────────────────────────────────────

// AddDependency adds a dependency: blockedID is blocked by blockerID.
// This means blockerID must be completed before blockedID can start.
// Returns error if either task not found or if it would create a cycle.
func (s *WorkTaskStore) AddDependency(blockedID, blockerID string) error {
	s.mu.Lock()

	if blockedID == blockerID {
		s.mu.Unlock()
		return fmt.Errorf("task cannot block itself")
	}

	blocked, ok := s.tasks[blockedID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task %s not found", blockedID)
	}
	blocker, ok := s.tasks[blockerID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task %s not found", blockerID)
	}

	// Check for cycle
	if s.wouldCreateCycle(blockedID, blockerID) {
		s.mu.Unlock()
		return fmt.Errorf("adding dependency %s -> %s would create a cycle", blockerID, blockedID)
	}

	// Add edge: blockerID blocks blockedID
	if !containsString(blocked.BlockedBy, blockerID) {
		blocked.BlockedBy = append(blocked.BlockedBy, blockerID)
	}
	if !containsString(blocker.Blocks, blockedID) {
		blocker.Blocks = append(blocker.Blocks, blockedID)
	}

	blocked.UpdatedAt = time.Now()
	blocker.UpdatedAt = time.Now()
	s.dirty = true

	// Snapshot for event
	eventTask := *blocked
	s.mu.Unlock()

	s.emitEvent(TaskEvent{
		Type:   TaskEventDepAdded,
		TaskID: blockedID,
		Task:   &eventTask,
		Data:   map[string]any{"blocker_id": blockerID},
	})
	return nil
}

// RemoveDependency removes a dependency between two tasks.
func (s *WorkTaskStore) RemoveDependency(blockedID, blockerID string) error {
	s.mu.Lock()

	blocked, ok := s.tasks[blockedID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task %s not found", blockedID)
	}
	blocker, ok := s.tasks[blockerID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task %s not found", blockerID)
	}

	blocked.BlockedBy = removeString(blocked.BlockedBy, blockerID)
	blocker.Blocks = removeString(blocker.Blocks, blockedID)

	blocked.UpdatedAt = time.Now()
	blocker.UpdatedAt = time.Now()
	s.dirty = true

	// Snapshot for event
	eventTask := *blocked
	s.mu.Unlock()

	s.emitEvent(TaskEvent{
		Type:   TaskEventDepRemoved,
		TaskID: blockedID,
		Task:   &eventTask,
		Data:   map[string]any{"blocker_id": blockerID},
	})
	return nil
}

// IsBlocked returns true if the task is blocked by any incomplete tasks.
func (s *WorkTaskStore) IsBlocked(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return false
	}

	for _, blockerID := range task.BlockedBy {
		blocker, exists := s.tasks[blockerID]
		if exists && blocker.Status != WorkTaskCompleted && blocker.Status != WorkTaskDeleted {
			return true
		}
	}
	return false
}

// GetBlockers returns all tasks that are blocking the given task.
func (s *WorkTaskStore) GetBlockers(id string) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil
	}

	var blockers []*WorkTask
	for _, blockerID := range task.BlockedBy {
		if blocker, exists := s.tasks[blockerID]; exists && blocker.Status != WorkTaskDeleted {
			blockers = append(blockers, blocker)
		}
	}
	return blockers
}

// GetBlocked returns all tasks that are blocked by the given task.
func (s *WorkTaskStore) GetBlocked(id string) []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil
	}

	var blocked []*WorkTask
	for _, blockedID := range task.Blocks {
		if t, exists := s.tasks[blockedID]; exists && t.Status != WorkTaskDeleted {
			blocked = append(blocked, t)
		}
	}
	return blocked
}

// CanStart returns true if the task is not blocked by any incomplete tasks.
// A task can start if all its blockers are completed or deleted.
func (s *WorkTaskStore) CanStart(id string) bool {
	return !s.IsBlocked(id)
}

// GetReadyTasks returns all pending tasks that can start (not blocked by incomplete tasks).
func (s *WorkTaskStore) GetReadyTasks() []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var ready []*WorkTask
	for _, t := range s.tasks {
		if t.Status != WorkTaskPending {
			continue
		}
		// Check if any blockers are still incomplete
		blocked := false
		for _, blockerID := range t.BlockedBy {
			if blocker, exists := s.tasks[blockerID]; exists {
				if blocker.Status != WorkTaskCompleted && blocker.Status != WorkTaskDeleted {
					blocked = true
					break
				}
			}
		}
		if !blocked {
			ready = append(ready, t)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		// Sort by priority first, then by ID
		pi := priorityOrder[ready[i].Priority]
		pj := priorityOrder[ready[j].Priority]
		if pi != pj {
			return pi > pj
		}
		return ready[i].ID < ready[j].ID
	})
	return ready
}

// ListBlockedTasks returns all pending tasks that are blocked by incomplete tasks.
func (s *WorkTaskStore) ListBlockedTasks() []*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var blocked []*WorkTask
	for _, t := range s.tasks {
		if t.Status != WorkTaskPending {
			continue
		}
		for _, blockerID := range t.BlockedBy {
			if blocker, exists := s.tasks[blockerID]; exists {
				if blocker.Status != WorkTaskCompleted && blocker.Status != WorkTaskDeleted {
					blocked = append(blocked, t)
					break
				}
			}
		}
	}
	sort.Slice(blocked, func(i, j int) bool {
		return blocked[i].ID < blocked[j].ID
	})
	return blocked
}

// GetDependencyGraph returns a map of task ID -> list of task IDs it blocks.
// Only includes non-deleted tasks.
func (s *WorkTaskStore) GetDependencyGraph() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	graph := make(map[string][]string)
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		var deps []string
		for _, blockedID := range t.Blocks {
			if blocked, exists := s.tasks[blockedID]; exists && blocked.Status != WorkTaskDeleted {
				deps = append(deps, blockedID)
			}
		}
		if len(deps) > 0 {
			graph[t.ID] = deps
		}
	}
	return graph
}

// TopologicalSort returns tasks in dependency order (tasks with no dependencies first).
// Returns error if there's a cycle (should not happen with cycle detection in AddDependency).
func (s *WorkTaskStore) TopologicalSort() ([]*WorkTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build in-degree map
	inDegree := make(map[string]int)
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		if _, exists := inDegree[t.ID]; !exists {
			inDegree[t.ID] = 0
		}
		for _, blockedID := range t.Blocks {
			if blocked, exists := s.tasks[blockedID]; exists && blocked.Status != WorkTaskDeleted {
				inDegree[blockedID]++
			}
		}
	}

	// Kahn's algorithm
	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue) // deterministic order

	var result []*WorkTask
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		task := s.tasks[id]
		if task != nil {
			result = append(result, task)
		}

		// Reduce in-degree for blocked tasks
		if task != nil {
			for _, blockedID := range task.Blocks {
				if _, exists := inDegree[blockedID]; exists {
					inDegree[blockedID]--
					if inDegree[blockedID] == 0 {
						queue = append(queue, blockedID)
						sort.Strings(queue) // keep deterministic
					}
				}
			}
		}
	}

	// Check for cycle
	if len(result) != len(inDegree) {
		return result, fmt.Errorf("dependency graph contains a cycle")
	}

	return result, nil
}

// GetCriticalPath returns the longest chain of dependencies from the given task.
// Useful for understanding the longest sequence of blocking work.
func (s *WorkTaskStore) GetCriticalPath(id string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	visited := make(map[string]bool)
	var path []string
	s.dfsLongestPath(id, visited, &path)
	return path
}

// dfsLongestPath performs DFS to find the longest dependency chain. Caller must hold lock.
func (s *WorkTaskStore) dfsLongestPath(id string, visited map[string]bool, currentPath *[]string) {
	if visited[id] {
		return
	}
	visited[id] = true
	*currentPath = append(*currentPath, id)

	task, ok := s.tasks[id]
	if !ok {
		return
	}

	// Find the longest path among all blocked tasks
	var longestNext []string
	for _, blockedID := range task.Blocks {
		if blocked, exists := s.tasks[blockedID]; exists && blocked.Status != WorkTaskDeleted {
			nextVisited := make(map[string]bool)
			for k, v := range visited {
				nextVisited[k] = v
			}
			var nextPath []string
			s.dfsLongestPath(blockedID, nextVisited, &nextPath)
			if len(nextPath) > len(longestNext) {
				longestNext = nextPath
			}
		}
	}

	*currentPath = append(*currentPath, longestNext...)
}

// DependencyStats returns statistics about the dependency graph.
func (s *WorkTaskStore) DependencyStats() DependencyStatsResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := DependencyStatsResult{}
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		stats.TotalTasks++
		if len(t.BlockedBy) > 0 {
			stats.BlockedTasks++
		}
		if len(t.Blocks) > 0 {
			stats.BlockingTasks++
		}
	}
	return stats
}

// DependencyStatsResult holds dependency graph statistics.
type DependencyStatsResult struct {
	TotalTasks    int
	BlockedTasks  int // tasks that have at least one blocker
	BlockingTasks int // tasks that block at least one other task
}

// ─── Parallel Execution ─────────────────────────────────────────────────────

// GetParallelTasks returns tasks that can run in parallel — they have no
// dependencies between each other and are all ready to start.
func (s *WorkTaskStore) GetParallelTasks() []*WorkTask {
	ready := s.GetReadyTasks()
	if len(ready) <= 1 {
		return ready
	}

	// All ready tasks can run in parallel since they have no incomplete blockers
	return ready
}

// GetExecutionGroups returns tasks grouped by execution order.
// Group 0 can run in parallel, then group 1 after group 0 completes, etc.
// Each group contains tasks that can execute concurrently.
func (s *WorkTaskStore) GetExecutionGroups() [][]*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build in-degree map (only for non-deleted, non-completed tasks)
	inDegree := make(map[string]int)
	taskMap := make(map[string]*WorkTask)
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted || t.Status == WorkTaskCompleted {
			continue
		}
		inDegree[t.ID] = 0
		taskMap[t.ID] = t
	}

	// Calculate in-degrees
	for id := range taskMap {
		t := s.tasks[id]
		for _, blockerID := range t.BlockedBy {
			if _, exists := taskMap[blockerID]; exists {
				inDegree[id]++
			}
		}
	}

	var groups [][]*WorkTask
	remaining := make(map[string]bool)
	for id := range taskMap {
		remaining[id] = true
	}

	for len(remaining) > 0 {
		// Find all tasks with in-degree 0 (current group)
		var group []*WorkTask
		for id := range remaining {
			if inDegree[id] == 0 {
				group = append(group, taskMap[id])
			}
		}

		if len(group) == 0 {
			// Cycle detected — add remaining tasks as final group
			var cycleTasks []*WorkTask
			for id := range remaining {
				cycleTasks = append(cycleTasks, taskMap[id])
			}
			if len(cycleTasks) > 0 {
				groups = append(groups, cycleTasks)
			}
			break
		}

		// Sort group by priority
		sort.Slice(group, func(i, j int) bool {
			pi := priorityOrder[group[i].Priority]
			pj := priorityOrder[group[j].Priority]
			if pi != pj {
				return pi > pj
			}
			return group[i].ID < group[j].ID
		})

		groups = append(groups, group)

		// Remove group from remaining and reduce in-degrees
		for _, t := range group {
			delete(remaining, t.ID)
			for _, blockedID := range t.Blocks {
				if _, exists := taskMap[blockedID]; exists {
					inDegree[blockedID]--
				}
			}
		}
	}

	return groups
}

// GetNextExecutableBatch returns the next batch of tasks that can start now.
// This is the first execution group that has at least one pending task.
func (s *WorkTaskStore) GetNextExecutableBatch() []*WorkTask {
	groups := s.GetExecutionGroups()
	for _, group := range groups {
		var pending []*WorkTask
		for _, t := range group {
			if t.Status == WorkTaskPending {
				pending = append(pending, t)
			}
		}
		if len(pending) > 0 {
			return pending
		}
	}
	return nil
}

// GetParallelSiblings returns tasks at the same level that can run in parallel.
// Two tasks are siblings if they share the same parent and have no dependency between them.
func (s *WorkTaskStore) GetParallelSiblings(parentID string) [][]*WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get all direct children of the parent
	var children []*WorkTask
	for _, t := range s.tasks {
		if t.ParentID == parentID && t.Status != WorkTaskDeleted {
			children = append(children, t)
		}
	}

	if len(children) <= 1 {
		if len(children) == 1 {
			return [][]*WorkTask{children}
		}
		return nil
	}

	// Group children by dependency chains
	// Children with no dependency between them can run in parallel
	visited := make(map[string]bool)
	var groups [][]*WorkTask

	for _, child := range children {
		if visited[child.ID] {
			continue
		}

		// Find all children that can run in parallel with this one
		// (no dependency path between them)
		group := []*WorkTask{child}
		visited[child.ID] = true

		for _, other := range children {
			if visited[other.ID] {
				continue
			}
			// Check if there's a dependency between child and other
			if !s.hasDependencyPath(child.ID, other.ID) && !s.hasDependencyPath(other.ID, child.ID) {
				group = append(group, other)
				visited[other.ID] = true
			}
		}

		groups = append(groups, group)
	}

	return groups
}

// hasDependencyPath checks if there's a dependency path from src to dst.
// Caller must hold read lock.
func (s *WorkTaskStore) hasDependencyPath(src, dst string) bool {
	visited := make(map[string]bool)
	queue := []string{src}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == dst {
			return true
		}
		if visited[current] {
			continue
		}
		visited[current] = true

		task, ok := s.tasks[current]
		if !ok {
			continue
		}
		for _, blockedID := range task.Blocks {
			if !visited[blockedID] {
				queue = append(queue, blockedID)
			}
		}
	}
	return false
}

// EstimateParallelTime estimates the total execution time assuming parallel execution.
// Uses the critical path (longest dependency chain) as the estimate.
func (s *WorkTaskStore) EstimateParallelTime(avgTaskDuration time.Duration) time.Duration {
	groups := s.GetExecutionGroups()
	if len(groups) == 0 {
		return 0
	}
	// Each group runs in parallel, so total time = number of groups * avg duration
	return time.Duration(len(groups)) * avgTaskDuration
}

// EstimateSequentialTime estimates the total execution time if run sequentially.
func (s *WorkTaskStore) EstimateSequentialTime(avgTaskDuration time.Duration) time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, t := range s.tasks {
		if t.Status == WorkTaskPending || t.Status == WorkTaskInProgress {
			count++
		}
	}
	return time.Duration(count) * avgTaskDuration
}

// GetParallelismRatio returns the speedup ratio of parallel vs sequential execution.
// A ratio of 2.0 means parallel execution is 2x faster than sequential.
func (s *WorkTaskStore) GetParallelismRatio() float64 {
	groups := s.GetExecutionGroups()
	totalTasks := 0
	for _, g := range groups {
		totalTasks += len(g)
	}
	if len(groups) == 0 || totalTasks == 0 {
		return 1.0
	}
	return float64(totalTasks) / float64(len(groups))
}

// GetBlockedByMap returns a map of task ID -> list of tasks that block it.
// Useful for visualizing dependency chains.
func (s *WorkTaskStore) GetBlockedByMap() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]string)
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		if len(t.BlockedBy) > 0 {
			// Filter out deleted/completed blockers
			var active []string
			for _, blockerID := range t.BlockedBy {
				if blocker, exists := s.tasks[blockerID]; exists {
					if blocker.Status != WorkTaskDeleted && blocker.Status != WorkTaskCompleted {
						active = append(active, blockerID)
					}
				}
			}
			if len(active) > 0 {
				result[t.ID] = active
			}
		}
	}
	return result
}

// GetBlocksMap returns a map of task ID -> list of tasks it blocks.
// Useful for visualizing forward dependencies.
func (s *WorkTaskStore) GetBlocksMap() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]string)
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		if len(t.Blocks) > 0 {
			// Filter out deleted tasks
			var active []string
			for _, blockedID := range t.Blocks {
				if blocked, exists := s.tasks[blockedID]; exists && blocked.Status != WorkTaskDeleted {
					active = append(active, blockedID)
				}
			}
			if len(active) > 0 {
				result[t.ID] = active
			}
		}
	}
	return result
}

// FormatExecutionPlan returns a human-readable execution plan showing parallel groups.
func (s *WorkTaskStore) FormatExecutionPlan() string {
	groups := s.GetExecutionGroups()
	if len(groups) == 0 {
		return "No tasks to execute."
	}

	var sb strings.Builder
	sb.WriteString("Execution Plan:\n")

	for i, group := range groups {
		sb.WriteString(fmt.Sprintf("\n--- Group %d (parallel) ---\n", i+1))
		for _, t := range group {
			priority := string(t.Priority)
			if priority == "" {
				priority = "medium"
			}
			sb.WriteString(fmt.Sprintf("  [%s] %s: %s\n", priority, t.ID, t.Subject))
		}
	}

	sb.WriteString(fmt.Sprintf("\nTotal: %d tasks in %d groups (parallelism ratio: %.1fx)\n",
		s.countActiveTasks(), len(groups), s.GetParallelismRatio()))

	return sb.String()
}

// countActiveTasks returns the count of non-deleted, non-completed tasks.
func (s *WorkTaskStore) countActiveTasks() int {
	count := 0
	for _, t := range s.tasks {
		if t.Status != WorkTaskDeleted && t.Status != WorkTaskCompleted {
			count++
		}
	}
	return count
}

// ─── ASCII Tree Visualization ───────────────────────────────────────────────

// FormatDependencyTree returns an ASCII tree visualization of all task dependencies.
// Shows which tasks block which, with status and priority indicators.
func (s *WorkTaskStore) FormatDependencyTree() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find root tasks (not blocked by anyone)
	var roots []*WorkTask
	for _, t := range s.tasks {
		if t.Status == WorkTaskDeleted {
			continue
		}
		if len(t.BlockedBy) == 0 {
			roots = append(roots, t)
		}
	}

	if len(roots) == 0 {
		return "No tasks found."
	}

	// Sort roots by priority then ID
	sort.Slice(roots, func(i, j int) bool {
		pi := priorityOrder[roots[i].Priority]
		pj := priorityOrder[roots[j].Priority]
		if pi != pj {
			return pi > pj
		}
		return roots[i].ID < roots[j].ID
	})

	var sb strings.Builder
	sb.WriteString("Task Dependency Tree:\n")
	sb.WriteString("(arrows show: task → blocks → dependent tasks)\n\n")

	visited := make(map[string]bool)
	for i, root := range roots {
		if i > 0 {
			sb.WriteString("\n")
		}
		s.formatTaskNode(&sb, root, "", true, visited)
	}

	return sb.String()
}

// formatTaskNode writes a single task node and its children as ASCII tree.
func (s *WorkTaskStore) formatTaskNode(sb *strings.Builder, task *WorkTask, prefix string, isLast bool, visited map[string]bool) {
	if visited[task.ID] {
		sb.WriteString(fmt.Sprintf("%s%s [circular: #%s]\n", prefix, s.formatTaskLabel(task), task.ID))
		return
	}
	visited[task.ID] = true

	// Draw connector
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	sb.WriteString(fmt.Sprintf("%s%s%s\n", prefix, connector, s.formatTaskLabel(task)))

	// Get blocked tasks
	var blocked []*WorkTask
	for _, blockedID := range task.Blocks {
		if t, ok := s.tasks[blockedID]; ok && t.Status != WorkTaskDeleted {
			blocked = append(blocked, t)
		}
	}

	// Sort blocked by priority then ID
	sort.Slice(blocked, func(i, j int) bool {
		pi := priorityOrder[blocked[i].Priority]
		pj := priorityOrder[blocked[j].Priority]
		if pi != pj {
			return pi > pj
		}
		return blocked[i].ID < blocked[j].ID
	})

	// Draw children
	childPrefix := prefix + "│   "
	if isLast {
		childPrefix = prefix + "    "
	}

	for i, child := range blocked {
		s.formatTaskNode(sb, child, childPrefix, i == len(blocked)-1, visited)
	}
}

// formatTaskLabel returns a formatted task label with status and priority icons.
func (s *WorkTaskStore) formatTaskLabel(task *WorkTask) string {
	statusIcon := s.statusIcon(task.Status)
	priorityIcon := s.priorityIcon(task.Priority)

	label := fmt.Sprintf("#%s %s", task.ID, task.Subject)
	if len(label) > 50 {
		label = label[:47] + "..."
	}

	return fmt.Sprintf("%s%s %s", statusIcon, priorityIcon, label)
}

// statusIcon returns an ASCII icon for the task status.
func (s *WorkTaskStore) statusIcon(status WorkTaskStatus) string {
	switch status {
	case WorkTaskPending:
		return "[ ]"
	case WorkTaskInProgress:
		return "[>]"
	case WorkTaskCompleted:
		return "[x]"
	case WorkTaskBlocked:
		return "[!]"
	case WorkTaskCancelled:
		return "[-]"
	case WorkTaskDeleted:
		return "[~]"
	default:
		return "[?]"
	}
}

// priorityIcon returns an ASCII icon for the task priority.
func (s *WorkTaskStore) priorityIcon(priority WorkTaskPriority) string {
	switch priority {
	case PriorityCritical:
		return "!!!"
	case PriorityHigh:
		return "!! "
	case PriorityMedium:
		return "!  "
	case PriorityLow:
		return "   "
	default:
		return "!  " // default medium
	}
}

// FormatDependencyGraph returns an ASCII visualization of the full dependency graph.
// Uses a matrix-style layout showing all connections.
func (s *WorkTaskStore) FormatDependencyGraph() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect all non-deleted tasks
	var tasks []*WorkTask
	for _, t := range s.tasks {
		if t.Status != WorkTaskDeleted {
			tasks = append(tasks, t)
		}
	}

	if len(tasks) == 0 {
		return "No tasks found."
	}

	// Sort by ID
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	// Build adjacency matrix
	taskIDs := make([]string, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}

	idToIdx := make(map[string]int)
	for i, id := range taskIDs {
		idToIdx[id] = i
	}

	// Build adjacency: blocked[i][j] = true means task i blocks task j
	blocked := make([][]bool, len(tasks))
	for i := range blocked {
		blocked[i] = make([]bool, len(tasks))
	}

	for _, t := range tasks {
		i := idToIdx[t.ID]
		for _, blockedID := range t.Blocks {
			if j, ok := idToIdx[blockedID]; ok {
				blocked[i][j] = true
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("Dependency Graph (row blocks column):\n\n")

	// Header row
	sb.WriteString("         ")
	for _, id := range taskIDs {
		sb.WriteString(fmt.Sprintf("%-4s", id))
	}
	sb.WriteString("\n")

	// Separator
	sb.WriteString("         ")
	for range taskIDs {
		sb.WriteString("----")
	}
	sb.WriteString("\n")

	// Data rows
	for i, t := range tasks {
		sb.WriteString(fmt.Sprintf("  %-4s   ", t.ID))
		for j := range taskIDs {
			if i == j {
				sb.WriteString("  . ")
			} else if blocked[i][j] {
				sb.WriteString("  X ")
			} else {
				sb.WriteString("    ")
			}
		}
		sb.WriteString(fmt.Sprintf("  # %s", t.Subject))
		sb.WriteString("\n")
	}

	sb.WriteString("\nLegend: X = blocks, . = self\n")

	return sb.String()
}

// FormatTaskStatus returns a compact ASCII status board of all tasks.
func (s *WorkTaskStore) FormatTaskStatus() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tasks []*WorkTask
	for _, t := range s.tasks {
		if t.Status != WorkTaskDeleted {
			tasks = append(tasks, t)
		}
	}

	if len(tasks) == 0 {
		return "No tasks."
	}

	// Sort by status then priority
	sort.Slice(tasks, func(i, j int) bool {
		si := statusOrder(tasks[i].Status)
		sj := statusOrder(tasks[j].Status)
		if si != sj {
			return si < sj
		}
		pi := priorityOrder[tasks[i].Priority]
		pj := priorityOrder[tasks[j].Priority]
		if pi != pj {
			return pi > pj
		}
		return tasks[i].ID < tasks[j].ID
	})

	var sb strings.Builder
	sb.WriteString("Task Status Board:\n")
	sb.WriteString("┌─────┬──────────┬────────────┬─────────┬──────────────────────────────────────┐\n")
	sb.WriteString(fmt.Sprintf("│ %-4s│ %-9s│ %-11s│ %-8s│ %-37s│\n", "ID", "Status", "Priority", "Tags", "Subject"))
	sb.WriteString("├─────┼──────────┼────────────┼─────────┼──────────────────────────────────────┤\n")

	for _, t := range tasks {
		status := s.statusIcon(t.Status)
		priority := string(t.Priority)
		if priority == "" {
			priority = "medium"
		}
		tags := strings.Join(t.Tags, ",")
		if tags == "" {
			tags = "-"
		}
		subject := t.Subject
		if len(subject) > 35 {
			subject = subject[:32] + "..."
		}
		sb.WriteString(fmt.Sprintf("│ %-4s│ %-9s│ %-11s│ %-8s│ %-37s│\n",
			t.ID, status, priority, tags, subject))
	}

	sb.WriteString("└─────┴──────────┴────────────┴─────────┴──────────────────────────────────────┘\n")

	return sb.String()
}

// statusOrder returns a numeric order for status sorting.
func statusOrder(status WorkTaskStatus) int {
	switch status {
	case WorkTaskInProgress:
		return 0
	case WorkTaskBlocked:
		return 1
	case WorkTaskPending:
		return 2
	case WorkTaskCompleted:
		return 3
	case WorkTaskCancelled:
		return 4
	default:
		return 5
	}
}

// FormatCriticalPath returns an ASCII visualization of the critical path.
func (s *WorkTaskStore) FormatCriticalPath(startID string) string {
	path := s.GetCriticalPath(startID)
	if len(path) == 0 {
		return "No critical path found."
	}

	var sb strings.Builder
	sb.WriteString("Critical Path:\n")

	for i, id := range path {
		task := s.GetTask(id)
		if task == nil {
			continue
		}

		prefix := "  "
		connector := "↓"
		if i == 0 {
			prefix = "  "
			connector = "●"
		}
		if i == len(path)-1 {
			connector = "●"
		}

		sb.WriteString(fmt.Sprintf("%s%s\n", prefix, connector))
		sb.WriteString(fmt.Sprintf("  │ #%s: %s [%s]\n", task.ID, task.Subject, task.Status))
	}

	sb.WriteString(fmt.Sprintf("\n  Path length: %d tasks\n", len(path)))

	return sb.String()
}

// TaskTreeNode represents a task with its subtask tree.
type TaskTreeNode struct {
	Task     *WorkTask
	Subtasks []*TaskTreeNode
}

// GetTaskTree returns a task with all its descendants (subtasks, sub-subtasks, etc.).
// Returns nil if task not found.
func (s *WorkTaskStore) GetTaskTree(id string) *TaskTreeNode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok || task.Status == WorkTaskDeleted {
		return nil
	}

	return s.buildTreeNode(id)
}

// buildTreeNode recursively builds a task tree node. Caller must hold lock.
func (s *WorkTaskStore) buildTreeNode(id string) *TaskTreeNode {
	task, ok := s.tasks[id]
	if !ok || task.Status == WorkTaskDeleted {
		return nil
	}

	node := &TaskTreeNode{
		Task:     task,
		Subtasks: make([]*TaskTreeNode, 0),
	}

	for _, t := range s.tasks {
		if t.ParentID == id && t.Status != WorkTaskDeleted {
			child := s.buildTreeNode(t.ID)
			if child != nil {
				node.Subtasks = append(node.Subtasks, child)
			}
		}
	}

	sort.Slice(node.Subtasks, func(i, j int) bool {
		return node.Subtasks[i].Task.ID < node.Subtasks[j].Task.ID
	})

	return node
}

// GetDepth returns the depth of a task in the hierarchy (0 for top-level).
func (s *WorkTaskStore) GetDepth(id string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	depth := 0
	current := id
	for {
		task, ok := s.tasks[current]
		if !ok || task.ParentID == "" {
			return depth
		}
		depth++
		current = task.ParentID
		// Prevent infinite loops
		if depth > 100 {
			return depth
		}
	}
}

// GetRootTask returns the root ancestor of a task (the top-level parent).
func (s *WorkTaskStore) GetRootTask(id string) *WorkTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	current := id
	for i := 0; i < 100; i++ { // prevent infinite loops
		task, ok := s.tasks[current]
		if !ok {
			return nil
		}
		if task.ParentID == "" {
			return task
		}
		current = task.ParentID
	}
	return nil
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
				// Use state machine for transition validation and time tracking
				if err := s.transitionToLocked(id, newStatus, ""); err != nil {
					return err
				}
				// transitionToLocked already applied the status, skip direct assignment
				continue
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
		case "priority":
			if v, ok := val.(string); ok {
				pri := WorkTaskPriority(v)
				if !isValidPriority(pri) {
					return fmt.Errorf("invalid priority: %s", v)
				}
				task.Priority = pri
			}
		case "parentID", "parentId":
			if v, ok := val.(string); ok {
				// Prevent self-reference
				if v == id {
					return fmt.Errorf("cannot set task as its own parent")
				}
				// Prevent circular parentage
				if v != "" && s.wouldCreateCycle(id, v) {
					return fmt.Errorf("setting parent %s would create a cycle", v)
				}
				task.ParentID = v
			}
		case "tags":
			if arr, ok := val.([]any); ok {
				// Replace all tags
				seen := make(map[string]bool)
				deduped := make([]string, 0, len(arr))
				for _, item := range arr {
					if tag, ok := item.(string); ok && tag != "" && !seen[tag] {
						seen[tag] = true
						deduped = append(deduped, tag)
					}
				}
				task.Tags = deduped
			}
		case "addTags":
			if arr, ok := val.([]any); ok {
				for _, item := range arr {
					if tag, ok := item.(string); ok && tag != "" && !containsString(task.Tags, tag) {
						task.Tags = append(task.Tags, tag)
					}
				}
			}
		case "removeTags":
			if arr, ok := val.([]any); ok {
				for _, item := range arr {
					if tag, ok := item.(string); ok {
						task.Tags = removeString(task.Tags, tag)
					}
				}
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
	s.dirty = true

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
// containsString checks if a string slice contains a string.
// Uses common utility function.
func containsString(slice []string, s string) bool {
	return ContainsStr(slice, s)
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

// ─── Disk Persistence ────────────────────────────────────────────────────────

// taskStoreData is the JSON serialization format for task persistence.
type taskStoreData struct {
	NextID int64      `json:"nextId"`
	Tasks  []*WorkTask `json:"tasks"`
}

// SaveToDisk persists all tasks to disk. Called on Close() and periodically.
func (s *WorkTaskStore) SaveToDisk() error {
	s.mu.Lock()
	if !s.dirty || s.filePath == "" {
		s.mu.Unlock()
		return nil
	}

	// Build snapshot under lock
	tasks := make([]*WorkTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	data := taskStoreData{
		NextID: s.nextID.Load(),
		Tasks:  tasks,
	}
	s.dirty = false
	s.mu.Unlock()

	// Write outside lock (atomic: temp + rename)
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tasks: %w", err)
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return fmt.Errorf("write tasks tmp: %w", err)
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename tasks: %w", err)
	}
	return nil
}

// loadFromDisk loads tasks from disk. Called on initialization.
func (s *WorkTaskStore) loadFromDisk() {
	if s.filePath == "" {
		return
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return // no file yet
	}

	var storeData taskStoreData
	if err := json.Unmarshal(data, &storeData); err != nil {
		return // corrupt file, start fresh
	}

	// Restore tasks
	s.tasks = make(map[string]*WorkTask, len(storeData.Tasks))
	for _, t := range storeData.Tasks {
		s.tasks[t.ID] = t
	}
	// Set nextID to last used ID (so first Add(1) returns the correct next ID)
	// storeData.NextID is the value AFTER the last created task, so we need to
	// decrement by 1 so that Add(1) returns storeData.NextID.
	if storeData.NextID > 0 {
		s.nextID.Store(storeData.NextID - 1)
	}

	// Find the max existing ID to ensure nextID is correct
	for _, t := range s.tasks {
		var id int64
		fmt.Sscanf(t.ID, "%d", &id)
		if id > s.nextID.Load() {
			s.nextID.Store(id)
		}
	}
}

// StartFlushLoop starts a background goroutine that flushes tasks to disk periodically.
func (s *WorkTaskStore) StartFlushLoop() {
	if s.filePath == "" {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.SaveToDisk()
			}
		}
	}()
}

// ─── Event System Methods ───────────────────────────────────────────────────

// On registers a listener for all task events. Returns a function to unregister.
func (s *WorkTaskStore) On(listener TaskEventListener) func() {
	return s.OnFilter(listener, nil)
}

// OnFilter registers a listener with an optional filter. Returns a function to unregister.
// The filter function is called before delivering the event; if it returns false, the event is skipped.
func (s *WorkTaskStore) OnFilter(listener TaskEventListener, filter TaskEventFilter) func() {
	id := fmt.Sprintf("listener-%d", s.nextListener.Add(1))
	entry := taskListenerEntry{
		id:       id,
		listener: listener,
		filter:   filter,
	}

	s.listenerMu.Lock()
	s.listeners = append(s.listeners, entry)
	s.listenerMu.Unlock()

	// Return unregister function
	return func() {
		s.listenerMu.Lock()
		defer s.listenerMu.Unlock()
		for i, l := range s.listeners {
			if l.id == id {
				s.listeners = append(s.listeners[:i], s.listeners[i+1:]...)
				return
			}
		}
	}
}

// OnType registers a listener for a specific event type. Returns a function to unregister.
func (s *WorkTaskStore) OnType(eventType TaskEventType, listener TaskEventListener) func() {
	return s.OnFilter(listener, func(event TaskEvent) bool {
		return event.Type == eventType
	})
}

// OnTask registers a listener for events on a specific task. Returns a function to unregister.
func (s *WorkTaskStore) OnTask(taskID string, listener TaskEventListener) func() {
	return s.OnFilter(listener, func(event TaskEvent) bool {
		return event.TaskID == taskID
	})
}

// OnStatusChange registers a listener for status change events. Returns a function to unregister.
func (s *WorkTaskStore) OnStatusChange(listener TaskEventListener) func() {
	return s.OnType(TaskEventStatusChange, listener)
}

// OnComplete registers a listener for task completion events. Returns a function to unregister.
func (s *WorkTaskStore) OnComplete(listener TaskEventListener) func() {
	return s.OnType(TaskEventCompleted, listener)
}

// emitEvent sends an event to all matching listeners. Caller must NOT hold the task lock.
func (s *WorkTaskStore) emitEvent(event TaskEvent) {
	event.Timestamp = time.Now()

	s.listenerMu.RLock()
	listeners := make([]taskListenerEntry, len(s.listeners))
	copy(listeners, s.listeners)
	s.listenerMu.RUnlock()

	for _, entry := range listeners {
		if entry.filter != nil && !entry.filter(event) {
			continue
		}
		// Deliver event in a goroutine to avoid blocking
		go entry.listener(event)
	}
}

// emitEventSync sends an event to all matching listeners synchronously. Caller must NOT hold the task lock.
func (s *WorkTaskStore) emitEventSync(event TaskEvent) {
	event.Timestamp = time.Now()

	s.listenerMu.RLock()
	listeners := make([]taskListenerEntry, len(s.listeners))
	copy(listeners, s.listeners)
	s.listenerMu.RUnlock()

	for _, entry := range listeners {
		if entry.filter != nil && !entry.filter(event) {
			continue
		}
		entry.listener(event)
	}
}

// ListenerCount returns the number of registered listeners.
func (s *WorkTaskStore) ListenerCount() int {
	s.listenerMu.RLock()
	defer s.listenerMu.RUnlock()
	return len(s.listeners)
}

// Close stops the flush loop and saves final state.
func (s *WorkTaskStore) Close() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
	s.SaveToDisk()
}
