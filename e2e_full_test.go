package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── End-to-End Integration Tests ──────────────────────────────────────────
// Test complete user scenarios to verify all modules work together.

func TestE2E_TaskManagement(t *testing.T) {
	dir := t.TempDir()
	store := NewWorkTaskStore(dir)

	// 1. Create tasks
	id1 := store.CreateTask("Implement auth", "", "", nil)
	id2 := store.CreateTask("Write tests", "", "", nil)

	// 2. Set dependencies
	store.AddDependency(id2, id1)

	// 3. Start first task
	store.TransitionTo(id1, WorkTaskInProgress, "started")

	// 4. Complete first task
	store.TransitionTo(id1, WorkTaskCompleted, "done")

	// 5. Now second task should be startable
	err := store.TransitionTo(id2, WorkTaskInProgress, "started")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}


func TestE2E_BudgetedRead(t *testing.T) {
	dir := t.TempDir()

	// 1. Create a large file (over 4000 tokens)
	largeContent := "## Section 1\n" + string(make([]byte, 20000)) + "\n## Section 2\ncontent"
	filePath := filepath.Join(dir, "test.md")
	os.WriteFile(filePath, []byte(largeContent), 0644)

	// 2. Read with budget
	result, err := ReadBudgeted(filePath, 1000)
	if err != nil {
		t.Fatalf("read budgeted failed: %v", err)
	}

	// 3. Verify truncation
	if !result.Truncated {
		t.Error("expected truncation for large file")
	}

	// 4. Section-aware read
	result2, err := ReadBudgetedSectionAware(filePath, 1000)
	if err != nil {
		t.Fatalf("read budgeted section aware failed: %v", err)
	}

	if !result2.Truncated {
		t.Error("expected truncation for section-aware read")
	}
}

func TestE2E_StepClassification(t *testing.T) {
	// Test all step categories
	tests := []struct {
		name     string
		text     string
		tools    []map[string]any
		thinking string
		expected StepCategory
	}{
		{"final", "answer", nil, "", StepFinal},
		{"continue", "", []map[string]any{{"name": "bash"}}, "", StepContinue},
		{"think-only", "", nil, "thinking", StepThinkOnly},
		{"invalid", "", nil, "", StepInvalid},
		{"failed", "", nil, "", StepFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.name == "failed" {
				err = &testError{}
			}
			result := ClassifyAssistantStep(tt.text, tt.tools, tt.thinking, err)
			if result.Category != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result.Category)
			}
		})
	}
}

type testError struct{}

func (e *testError) Error() string {
	return "test error"
}

func TestE2E_DoomLoopDetection(t *testing.T) {
	detector := NewDoomLoopDetector()

	// 1. First call - no doom loop
	result := detector.CheckRecord("bash", map[string]any{"command": "ls"})
	if result {
		t.Error("expected no doom loop on first call")
	}

	// 2. Second call - no doom loop
	result = detector.CheckRecord("bash", map[string]any{"command": "ls"})
	if result {
		t.Error("expected no doom loop on second call")
	}

	// 3. Third call - doom loop detected
	result = detector.CheckRecord("bash", map[string]any{"command": "ls"})
	if !result {
		t.Error("expected doom loop on third call")
	}
}


