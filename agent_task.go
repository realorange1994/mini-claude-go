package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func (s TaskStatus) String() string {
	switch s {
	case TaskStatusPending:
		return "pending"
	case TaskStatusRunning:
		return "running"
	case TaskStatusCompleted:
		return "completed"
	case TaskStatusFailed:
		return "failed"
	case TaskStatusKilled:
		return "killed"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

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
	TranscriptPath  string           // path to the sub-agent's transcript file
	ToolsUsed       int
	DurationMs      int64
	StartTime       time.Time
	EndTime         time.Time
	PendingMessages []string
	CancelFunc      context.CancelFunc // cancels the sub-agent's context (for async agents)
	Process         *os.Process        // tracked OS process (for bash background tasks)
	OutputFile      string             // path to output file (for bash background tasks)
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

// SetTranscriptPath sets the transcript path for the task.
func (ts *TaskState) SetTranscriptPath(path string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.TranscriptPath = path
}

// GetTranscriptPath returns the transcript path for the task.
func (ts *TaskState) GetTranscriptPath() string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.TranscriptPath
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

// KillTask forcibly terminates a running task and marks it as killed.
func (ts *TaskStore) KillTask(agentID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if task, ok := ts.tasks[agentID]; ok {
		if task.Process != nil {
			_ = task.Process.Kill()
		}
		if task.CancelFunc != nil {
			task.CancelFunc()
		}
		task.Status = TaskStatusKilled
		task.Error = "stopped by user"
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

// RegisterBashBgTask registers a new background bash task with its output file path.
func (ts *TaskStore) RegisterBashBgTask(agentID, description, outputFile string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	task := &TaskState{
		ID:           agentID,
		Status:       TaskStatusRunning,
		Description:  description,
		SubagentType: "bash_background",
		OutputFile:   outputFile,
		StartTime:    time.Now(),
	}
	ts.tasks[agentID] = task
}

// --- Bash background task functions (consolidated from bash_bg_task.go) ---

// generateBashTaskID generates a unique task ID with the format "b" + 8 random alphanumeric chars.
func generateBashTaskID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	rand.Read(b)
	result := make([]byte, 8)
	for i := range 8 {
		result[i] = chars[int(b[i])%len(chars)]
	}
	return "b" + string(result)
}

// bashBgTasksDir returns the directory for background bash task output files.
func bashBgTasksDir() string {
	return filepath.Join(".claude", "tasks", "bash")
}

// spawnBackgroundBashCommand spawns a command as a background process,
// writes output to a file, and tracks the task in the store.
// Returns (taskID, outputFilePath, errText).
func (a *AgentLoop) spawnBackgroundBashCommand(command, workingDir string) (string, string, string) {
	taskID := generateBashTaskID()

	// Create output directory
	outputDir := bashBgTasksDir()
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", "", fmt.Sprintf("Error: failed to create task output directory: %v", err)
	}

	outputFile := filepath.Join(outputDir, taskID+".output")

	// Create/truncate the output file
	f, err := os.Create(outputFile)
	if err != nil {
		return "", "", fmt.Sprintf("Error: failed to create output file: %v", err)
	}

	// Write header to output file
	fmt.Fprintf(f, "--- Background Task: %s ---\n", taskID)
	fmt.Fprintf(f, "Command: %s\n", command)
	fmt.Fprintf(f, "Working Dir: %s\n", workingDir)
	fmt.Fprintf(f, "Started: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "--- Output ---\n\n")
	f.Close()

	// Register in the main task store
	a.taskStore.RegisterBashBgTask(taskID, command, outputFile)

	// Determine shell
	var shell, flag string
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("powershell"); err == nil {
			shell, flag = "powershell", "-Command"
		} else if _, err := exec.LookPath("bash"); err == nil {
			shell, flag = "bash", "-c"
		} else {
			shell, flag = "cmd", "/C"
		}
	} else {
		shell, flag = "bash", "-c"
	}

	// Spawn the command in a goroutine
	go func() {
		start := time.Now()

		// Open output file for appending
		f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			a.finishBashBgTask(taskID, 1, fmt.Sprintf("Error: failed to open output file: %v", err))
			return
		}
		defer f.Close()

		cmd := exec.Command(shell, flag, command)
		cmd.Dir = workingDir
		cmd.Stdout = f
		cmd.Stderr = f
		cmd.Stdin = nil

		runErr := cmd.Run()
		elapsed := time.Since(start)

		exitCode := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}

		// Write footer
		fmt.Fprintf(f, "\n--- Task Complete ---\n")
		fmt.Fprintf(f, "Exit code: %d\n", exitCode)
		fmt.Fprintf(f, "Duration: %s\n", elapsed.Round(time.Millisecond))
		status := "completed"
		if exitCode != 0 {
			status = "failed"
		}
		fmt.Fprintf(f, "Status: %s\n", status)

		a.finishBashBgTask(taskID, exitCode, "")
	}()

	return taskID, outputFile, ""
}

// finishBashBgTask marks a background bash task as completed and sends a notification.
func (a *AgentLoop) finishBashBgTask(taskID string, exitCode int, errMsg string) {
	// Look up the task from the task store for the notification
	task := a.taskStore.GetTask(taskID)

	// Update the main task store
	if exitCode == 0 && errMsg == "" {
		durationMs := time.Since(task.StartTime).Milliseconds()
		a.taskStore.CompleteTask(taskID, fmt.Sprintf("Background command completed (exit code 0)"), 1, durationMs)
	} else {
		errDetail := errMsg
		if errDetail == "" {
			errDetail = fmt.Sprintf("Command failed with exit code %d", exitCode)
		}
		a.taskStore.FailTask(taskID, errDetail)
	}

	// Send notification
	status := "completed"
	summary := "Command completed successfully"
	if exitCode != 0 || errMsg != "" {
		status = "failed"
		summary = fmt.Sprintf("Command failed (exit code %d)", exitCode)
		if errMsg != "" {
			summary = errMsg
		}
	}

	outputFile := ""
	command := ""
	if task != nil {
		outputFile = task.OutputFile
		command = task.Description
	}

	notification := fmt.Sprintf(`<task-notification>
<task_id>%s</task_id>
<task_type>bash_background</task_type>
<status>%s</status>
<output_file>%s</output_file>
<command>%s</command>
<summary>%s</summary>
</task-notification>`, taskID, status, outputFile, escapeXML(command), escapeXML(summary))

	select {
	case a.notificationChan <- notification:
	default:
		// Channel is full, drop the notification
	}
}

// escapeXML escapes special characters for XML content.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// readBashBgTaskOutput reads the output file of a background bash task.
// Returns (output, errorText).
func (a *AgentLoop) readBashBgTaskOutput(taskID string, block bool, timeout time.Duration) (string, string) {
	task := a.taskStore.GetTask(taskID)
	if task == nil {
		return "", fmt.Sprintf("Background task %s not found", taskID)
	}

	// If block is true, wait for completion
	if block {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if task.IsTerminal() {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	// Read the output file
	if task.OutputFile == "" {
		return "", fmt.Sprintf("Task %s has no output file", taskID)
	}
	data, err := os.ReadFile(task.OutputFile)
	if err != nil {
		return "", fmt.Sprintf("Error reading output file: %v", err)
	}

	output := string(data)

	// Truncate if too large
	const maxOutput = 50000
	if len(output) > maxOutput {
		half := maxOutput / 2
		truncated := len(output) - maxOutput
		output = output[:half] + fmt.Sprintf("\n\n... (%d chars truncated) ...\n\n", truncated) + output[len(output)-half:]
	}

	return fmt.Sprintf("Task %s (%s):\n%s", taskID, task.Status, output), ""
}
