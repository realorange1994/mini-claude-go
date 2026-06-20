package main

import (
	"strings"
	"testing"
)

func TestTaskGateDecide_NoTasks(t *testing.T) {
	store := NewWorkTaskStore(t.TempDir())
	decision := TaskGateDecide(TaskGateInput{
		TaskStore:  store,
		ReactCount: 0,
		MaxReact:   3,
		Mode:       GateModeMain,
	})

	if decision.NeedReentry {
		t.Error("expected no reentry when no tasks")
	}
}

func TestTaskGateDecide_HasActiveTasks(t *testing.T) {
	store := NewWorkTaskStore(t.TempDir())
	store.CreateTask("Implement auth", "", "", nil)
	store.CreateTask("Write tests", "", "", nil)

	decision := TaskGateDecide(TaskGateInput{
		TaskStore:  store,
		ReactCount: 0,
		MaxReact:   3,
		Mode:       GateModeMain,
	})

	if !decision.NeedReentry {
		t.Error("expected reentry when active tasks exist")
	}
	if len(decision.IncompleteTasks) != 2 {
		t.Errorf("expected 2 incomplete tasks, got %d", len(decision.IncompleteTasks))
	}
	if !strings.Contains(decision.ReentryText, "Implement auth") {
		t.Error("expected reentry text to contain task subject")
	}
}

func TestTaskGateDecide_CapExceeded(t *testing.T) {
	store := NewWorkTaskStore(t.TempDir())
	store.CreateTask("Implement auth", "", "", nil)

	decision := TaskGateDecide(TaskGateInput{
		TaskStore:  store,
		ReactCount: 3,
		MaxReact:   3,
		Mode:       GateModeMain,
	})

	if decision.NeedReentry {
		t.Error("expected no reentry when cap exceeded")
	}
	if !decision.CapExceeded {
		t.Error("expected cap exceeded")
	}
}

func TestTaskGateDecide_CompletedTasksIgnored(t *testing.T) {
	store := NewWorkTaskStore(t.TempDir())
	id := store.CreateTask("Implement auth", "", "", nil)
	store.TransitionTo(id, WorkTaskInProgress, "started")
	store.TransitionTo(id, WorkTaskCompleted, "done")

	decision := TaskGateDecide(TaskGateInput{
		TaskStore:  store,
		ReactCount: 0,
		MaxReact:   3,
		Mode:       GateModeMain,
	})

	if decision.NeedReentry {
		t.Error("expected no reentry when all tasks completed")
	}
}

func TestTaskGateDecide_BlockedTasksIgnored(t *testing.T) {
	store := NewWorkTaskStore(t.TempDir())
	id := store.CreateTask("Implement auth", "", "", nil)
	store.TransitionTo(id, WorkTaskInProgress, "started")
	store.TransitionTo(id, WorkTaskBlocked, "blocked")

	decision := TaskGateDecide(TaskGateInput{
		TaskStore:  store,
		ReactCount: 0,
		MaxReact:   3,
		Mode:       GateModeMain,
	})

	if decision.NeedReentry {
		t.Error("expected no reentry when tasks are blocked")
	}
}

func TestTaskGateDecide_SubagentMode(t *testing.T) {
	store := NewWorkTaskStore(t.TempDir())
	id := store.CreateTask("Implement auth", "", "", nil)
	store.UpdateTask(id, map[string]any{"owner": "agent-1"})

	decision := TaskGateDecide(TaskGateInput{
		TaskStore:  store,
		Owner:      "agent-1",
		ReactCount: 0,
		MaxReact:   2,
		Mode:       GateModeSubagent,
	})

	if !decision.NeedReentry {
		t.Error("expected reentry for subagent with active tasks")
	}
	if !strings.Contains(decision.ReentryText, "you own") {
		t.Error("expected subagent-specific headline")
	}
}

func TestTaskGateDecide_OwnerFiltering(t *testing.T) {
	store := NewWorkTaskStore(t.TempDir())
	id1 := store.CreateTask("Task A", "", "", nil)
	store.UpdateTask(id1, map[string]any{"owner": "agent-1"})
	id2 := store.CreateTask("Task B", "", "", nil)
	store.UpdateTask(id2, map[string]any{"owner": "agent-2"})

	decision := TaskGateDecide(TaskGateInput{
		TaskStore:  store,
		Owner:      "agent-1",
		ReactCount: 0,
		MaxReact:   2,
		Mode:       GateModeSubagent,
	})

	if len(decision.IncompleteTasks) != 1 {
		t.Errorf("expected 1 incomplete task for agent-1, got %d", len(decision.IncompleteTasks))
	}
}

func TestGetMaxReact(t *testing.T) {
	if GetMaxReact(GateModeMain) != 3 {
		t.Error("expected main max react to be 3")
	}
	if GetMaxReact(GateModeSubagent) != 2 {
		t.Error("expected subagent max react to be 2")
	}
}

func TestTaskGateDecide_NilStore(t *testing.T) {
	decision := TaskGateDecide(TaskGateInput{
		TaskStore:  nil,
		ReactCount: 0,
		MaxReact:   3,
		Mode:       GateModeMain,
	})

	if decision.NeedReentry {
		t.Error("expected no reentry when store is nil")
	}
}
