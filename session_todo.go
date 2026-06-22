package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Session Todo Persistence (MiMo-Code 3) ────────────────────────────────
//
// Database-backed per-session todo list with ordered position tracking.
//
// MiMo-Code source: session/todo.ts (77 lines)

// TodoStatus represents the status of a todo item.
type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
	TodoCancelled  TodoStatus = "cancelled"
)

// TodoItem represents a todo item.
type TodoItem struct {
	ID        string     `json:"id"`
	Content   string     `json:"content"`
	Status    TodoStatus `json:"status"`
	Position  int        `json:"position"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// SessionTodoManager manages session todos.
type SessionTodoManager struct {
	mu       sync.Mutex
	items    []*TodoItem
	savePath string
	nextID   int
}

// NewSessionTodoManager creates a new todo manager.
func NewSessionTodoManager(savePath string) *SessionTodoManager {
	m := &SessionTodoManager{
		items:    make([]*TodoItem, 0),
		savePath: savePath,
		nextID:   1,
	}
	m.load()
	return m
}

// Add adds a new todo item.
func (m *SessionTodoManager) Add(content string) *TodoItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	item := &TodoItem{
		ID:        fmt.Sprintf("todo-%d", m.nextID),
		Content:   content,
		Status:    TodoPending,
		Position:  len(m.items),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	m.nextID++
	m.items = append(m.items, item)
	m.save()

	return item
}

// Update updates a todo item's status.
func (m *SessionTodoManager) Update(id string, status TodoStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range m.items {
		if item.ID == id {
			item.Status = status
			item.UpdatedAt = time.Now()
			m.save()
			return nil
		}
	}

	return fmt.Errorf("todo not found: %s", id)
}

// Remove removes a todo item.
func (m *SessionTodoManager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, item := range m.items {
		if item.ID == id {
			m.items = append(m.items[:i], m.items[i+1:]...)
			m.save()
			return nil
		}
	}

	return fmt.Errorf("todo not found: %s", id)
}

// List returns all todo items.
func (m *SessionTodoManager) List() []*TodoItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*TodoItem, len(m.items))
	copy(result, m.items)
	return result
}

// ListByStatus returns todo items filtered by status.
func (m *SessionTodoManager) ListByStatus(status TodoStatus) []*TodoItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*TodoItem
	for _, item := range m.items {
		if item.Status == status {
			result = append(result, item)
		}
	}
	return result
}

// Get returns a todo item by ID.
func (m *SessionTodoManager) Get(id string) *TodoItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, item := range m.items {
		if item.ID == id {
			return item
		}
	}
	return nil
}

// Count returns the count of todos by status.
func (m *SessionTodoManager) Count(status TodoStatus) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for _, item := range m.items {
		if item.Status == status {
			count++
		}
	}
	return count
}

// save persists todos to disk.
func (m *SessionTodoManager) save() {
	if m.savePath == "" {
		return
	}
	os.MkdirAll(filepath.Dir(m.savePath), 0755)
	data, _ := json.MarshalIndent(m.items, "", "  ")
	os.WriteFile(m.savePath, data, 0644)
}

// load loads todos from disk.
func (m *SessionTodoManager) load() {
	if m.savePath == "" {
		return
	}
	data, err := os.ReadFile(m.savePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &m.items)
}

// FormatTodoList formats a todo list for display.
func FormatTodoList(items []*TodoItem) string {
	if len(items) == 0 {
		return "No todos."
	}

	var sb string
	sb += "## Todo List\n\n"
	for _, item := range items {
		status := "[ ]"
		if item.Status == TodoInProgress {
			status = "[>]"
		} else if item.Status == TodoCompleted {
			status = "[✓]"
		} else if item.Status == TodoCancelled {
			status = "[✗]"
		}
		sb += fmt.Sprintf("- %s %s\n", status, item.Content)
	}
	return sb
}
