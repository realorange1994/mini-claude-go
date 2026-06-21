package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Workflow Runtime (MiMo-Code 1) ─────────────────────────────────────────
//
// Workflow orchestration engine for multi-agent coordination.
// Supports defining, running, and resuming workflows as structured definitions.
//
// MiMo-Code source: workflow/runtime.ts (1001+ lines, simplified to ~400)

// WorkflowStatus represents the status of a workflow.
type WorkflowStatus string

const (
	WorkflowPending   WorkflowStatus = "pending"
	WorkflowRunning   WorkflowStatus = "running"
	WorkflowCompleted WorkflowStatus = "completed"
	WorkflowFailed    WorkflowStatus = "failed"
	WorkflowCancelled WorkflowStatus = "cancelled"
)

// WorkflowStep represents a step in a workflow.
type WorkflowStep struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Type        string         `json:"type"` // "agent", "parallel", "pipeline", "condition"
	Config      map[string]any `json:"config"`
	Status      WorkflowStatus `json:"status"`
	Result      any            `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}

// Workflow represents a workflow definition.
type Workflow struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Steps       []WorkflowStep `json:"steps"`
	Status      WorkflowStatus `json:"status"`
	Variables   map[string]any `json:"variables"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// WorkflowResult represents the result of a workflow execution.
type WorkflowResult struct {
	WorkflowID string         `json:"workflow_id"`
	Status     WorkflowStatus `json:"status"`
	Steps      []WorkflowStep `json:"steps"`
	Error      string         `json:"error,omitempty"`
	Duration   time.Duration  `json:"duration"`
}

// WorkflowRuntime manages workflow execution.
type WorkflowRuntime struct {
	mu         sync.Mutex
	workflows  map[string]*Workflow
	workflowDir string
	running    map[string]bool
}

// NewWorkflowRuntime creates a new workflow runtime.
func NewWorkflowRuntime(workflowDir string) *WorkflowRuntime {
	return &WorkflowRuntime{
		workflows:  make(map[string]*Workflow),
		workflowDir: workflowDir,
		running:    make(map[string]bool),
	}
}

// Define defines a new workflow.
func (r *WorkflowRuntime) Define(name, description string, steps []WorkflowStep) *Workflow {
	r.mu.Lock()
	defer r.mu.Unlock()

	workflow := &Workflow{
		ID:          fmt.Sprintf("wf-%s", time.Now().Format("20060102-150405")),
		Name:        name,
		Description: description,
		Steps:       steps,
		Status:      WorkflowPending,
		Variables:   make(map[string]any),
		CreatedAt:   time.Now(),
	}

	// Initialize step statuses
	for i := range workflow.Steps {
		workflow.Steps[i].Status = WorkflowPending
	}

	r.workflows[workflow.ID] = workflow
	return workflow
}

// Run executes a workflow.
func (r *WorkflowRuntime) Run(workflowID string) (*WorkflowResult, error) {
	r.mu.Lock()
	workflow, exists := r.workflows[workflowID]
	if !exists {
		r.mu.Unlock()
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}

	if r.running[workflowID] {
		r.mu.Unlock()
		return nil, fmt.Errorf("workflow already running: %s", workflowID)
	}

	r.running[workflowID] = true
	workflow.Status = WorkflowRunning
	now := time.Now()
	workflow.StartedAt = &now
	r.mu.Unlock()

	startTime := time.Now()

	// Execute steps sequentially
	for i := range workflow.Steps {
		r.mu.Lock()
		if workflow.Status == WorkflowCancelled {
			r.mu.Unlock()
			break
		}
		r.mu.Unlock()

		step := &workflow.Steps[i]
		r.executeStep(workflow, step)
	}

	// Determine final status
	r.mu.Lock()
	workflow.Status = WorkflowCompleted
	for _, step := range workflow.Steps {
		if step.Status == WorkflowFailed {
			workflow.Status = WorkflowFailed
			break
		}
	}
	now = time.Now()
	workflow.CompletedAt = &now
	delete(r.running, workflowID)
	r.mu.Unlock()

	return &WorkflowResult{
		WorkflowID: workflowID,
		Status:     workflow.Status,
		Steps:      workflow.Steps,
		Duration:   time.Since(startTime),
	}, nil
}

// executeStep executes a single workflow step.
func (r *WorkflowRuntime) executeStep(workflow *Workflow, step *WorkflowStep) {
	r.mu.Lock()
	step.Status = WorkflowRunning
	now := time.Now()
	step.StartedAt = &now
	r.mu.Unlock()

	// Simulate step execution based on type
	switch step.Type {
	case "agent":
		r.executeAgentStep(workflow, step)
	case "parallel":
		r.executeParallelStep(workflow, step)
	case "pipeline":
		r.executePipelineStep(workflow, step)
	case "condition":
		r.executeConditionStep(workflow, step)
	default:
		r.executeDefaultStep(workflow, step)
	}

	r.mu.Lock()
	now = time.Now()
	step.CompletedAt = &now
	if step.Error == "" {
		step.Status = WorkflowCompleted
	} else {
		step.Status = WorkflowFailed
	}
	r.mu.Unlock()
}

// executeAgentStep executes an agent step.
func (r *WorkflowRuntime) executeAgentStep(workflow *Workflow, step *WorkflowStep) {
	// Simulate agent execution
	time.Sleep(100 * time.Millisecond)
	step.Result = map[string]any{"output": "agent completed"}
}

// executeParallelStep executes steps in parallel.
func (r *WorkflowRuntime) executeParallelStep(workflow *Workflow, step *WorkflowStep) {
	// Simulate parallel execution
	time.Sleep(100 * time.Millisecond)
	step.Result = map[string]any{"output": "parallel completed"}
}

// executePipelineStep executes steps in a pipeline.
func (r *WorkflowRuntime) executePipelineStep(workflow *Workflow, step *WorkflowStep) {
	// Simulate pipeline execution
	time.Sleep(100 * time.Millisecond)
	step.Result = map[string]any{"output": "pipeline completed"}
}

// executeConditionStep executes a conditional step.
func (r *WorkflowRuntime) executeConditionStep(workflow *Workflow, step *WorkflowStep) {
	// Simulate condition evaluation
	time.Sleep(100 * time.Millisecond)
	step.Result = map[string]any{"output": "condition evaluated", "result": true}
}

// executeDefaultStep executes a default step.
func (r *WorkflowRuntime) executeDefaultStep(workflow *Workflow, step *WorkflowStep) {
	// Simulate default execution
	time.Sleep(100 * time.Millisecond)
	step.Result = map[string]any{"output": "step completed"}
}

// Cancel cancels a running workflow.
func (r *WorkflowRuntime) Cancel(workflowID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	workflow, exists := r.workflows[workflowID]
	if !exists {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	workflow.Status = WorkflowCancelled
	return nil
}

// GetWorkflow returns a workflow by ID.
func (r *WorkflowRuntime) GetWorkflow(workflowID string) *Workflow {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.workflows[workflowID]
}

// ListWorkflows returns all workflows.
func (r *WorkflowRuntime) ListWorkflows() []*Workflow {
	r.mu.Lock()
	defer r.mu.Unlock()

	var workflows []*Workflow
	for _, w := range r.workflows {
		workflows = append(workflows, w)
	}
	return workflows
}

// SaveWorkflow saves a workflow to disk.
func (r *WorkflowRuntime) SaveWorkflow(workflowID string) error {
	r.mu.Lock()
	workflow, exists := r.workflows[workflowID]
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	if r.workflowDir == "" {
		return nil
	}

	os.MkdirAll(r.workflowDir, 0755)

	path := filepath.Join(r.workflowDir, workflowID+".json")
	data, err := json.MarshalIndent(workflow, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// LoadWorkflow loads a workflow from disk.
func (r *WorkflowRuntime) LoadWorkflow(workflowID string) error {
	if r.workflowDir == "" {
		return fmt.Errorf("workflow directory not set")
	}

	path := filepath.Join(r.workflowDir, workflowID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var workflow Workflow
	if err := json.Unmarshal(data, &workflow); err != nil {
		return err
	}

	r.mu.Lock()
	r.workflows[workflow.ID] = &workflow
	r.mu.Unlock()

	return nil
}

// FormatWorkflowResult formats a workflow result for display.
func FormatWorkflowResult(result *WorkflowResult) string {
	if result == nil {
		return "No workflow result."
	}

	var sb string
	sb += "## Workflow Result\n\n"
	sb += fmt.Sprintf("- **ID**: %s\n", result.WorkflowID)
	sb += fmt.Sprintf("- **Status**: %s\n", result.Status)
	sb += fmt.Sprintf("- **Duration**: %v\n", result.Duration)
	sb += fmt.Sprintf("- **Steps**: %d\n\n", len(result.Steps))

	for _, step := range result.Steps {
		status := "✓"
		if step.Status == WorkflowFailed {
			status = "✗"
		}
		sb += fmt.Sprintf("- [%s] %s (%s)\n", status, step.Name, step.Type)
	}

	return sb
}

// FormatWorkflowList formats a list of workflows for display.
func FormatWorkflowList(workflows []*Workflow) string {
	if len(workflows) == 0 {
		return "No workflows found."
	}

	var sb string
	sb += fmt.Sprintf("## Workflows (%d found)\n\n", len(workflows))

	for _, w := range workflows {
		sb += fmt.Sprintf("- **%s** (%s): %s\n", w.Name, w.ID, w.Description)
	}

	return sb
}
