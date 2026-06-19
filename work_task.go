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
)

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
// Supports hierarchical subtasks via ParentID, tagging, and priority.
type WorkTask struct {
	ID          string
	Subject     string
	Description string
	ActiveForm  string           // present continuous form for spinner (e.g., "Running tests")
	Status      WorkTaskStatus   // pending, in_progress, completed, deleted
	Priority    WorkTaskPriority // critical, high, medium, low (default: medium)
	Owner       string           // optional agent ID
	ParentID    string           // parent task ID (empty for top-level tasks)
	Tags        []string         // categorization tags (e.g., "bug", "feature", "urgent")
	Metadata    map[string]any   // arbitrary metadata
	Blocks      []string         // task IDs this task blocks
	BlockedBy   []string         // task IDs that block this task
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WorkTaskStore manages work tasks (LLM TODO items) for an agent session.
// This is separate from TaskStore which manages async sub-agent tasks.
// Tasks are persisted to disk at .claude/tasks.json to survive compaction and restarts.
type WorkTaskStore struct {
	mu       sync.RWMutex
	tasks    map[string]*WorkTask
	nextID   atomic.Int64
	filePath string
	dirty    bool
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
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
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if tag == "" {
		return nil
	}
	if containsString(task.Tags, tag) {
		return nil // already has this tag
	}
	task.Tags = append(task.Tags, tag)
	task.UpdatedAt = time.Now()
	s.dirty = true
	return nil
}

// RemoveTag removes a tag from a task. Returns error if task not found.
func (s *WorkTaskStore) RemoveTaskTag(id, tag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	task.Tags = removeString(task.Tags, tag)
	task.UpdatedAt = time.Now()
	s.dirty = true
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

func isValidPriority(p WorkTaskPriority) bool {
	switch p {
	case PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow:
		return true
	default:
		return false
	}
}

// ─── Dependency Management ──────────────────────────────────────────────────

// AddDependency adds a dependency: blockedID is blocked by blockerID.
// This means blockerID must be completed before blockedID can start.
// Returns error if either task not found or if it would create a cycle.
func (s *WorkTaskStore) AddDependency(blockedID, blockerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if blockedID == blockerID {
		return fmt.Errorf("task cannot block itself")
	}

	blocked, ok := s.tasks[blockedID]
	if !ok {
		return fmt.Errorf("task %s not found", blockedID)
	}
	_, ok = s.tasks[blockerID]
	if !ok {
		return fmt.Errorf("task %s not found", blockerID)
	}

	// Check for cycle
	if s.wouldCreateCycle(blockedID, blockerID) {
		return fmt.Errorf("adding dependency %s -> %s would create a cycle", blockerID, blockedID)
	}

	// Add edge: blockerID blocks blockedID
	if !containsString(blocked.BlockedBy, blockerID) {
		blocked.BlockedBy = append(blocked.BlockedBy, blockerID)
	}
	blocker := s.tasks[blockerID]
	if !containsString(blocker.Blocks, blockedID) {
		blocker.Blocks = append(blocker.Blocks, blockedID)
	}

	blocked.UpdatedAt = time.Now()
	blocker.UpdatedAt = time.Now()
	s.dirty = true
	return nil
}

// RemoveDependency removes a dependency between two tasks.
func (s *WorkTaskStore) RemoveDependency(blockedID, blockerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	blocked, ok := s.tasks[blockedID]
	if !ok {
		return fmt.Errorf("task %s not found", blockedID)
	}
	blocker, ok := s.tasks[blockerID]
	if !ok {
		return fmt.Errorf("task %s not found", blockerID)
	}

	blocked.BlockedBy = removeString(blocked.BlockedBy, blockerID)
	blocker.Blocks = removeString(blocker.Blocks, blockedID)

	blocked.UpdatedAt = time.Now()
	blocker.UpdatedAt = time.Now()
	s.dirty = true
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

// Close stops the flush loop and saves final state.
func (s *WorkTaskStore) Close() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
	s.SaveToDisk()
}
