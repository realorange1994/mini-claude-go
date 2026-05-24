package main

import (
	"context"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// CronCreateTool
// ---------------------------------------------------------------------------

func TestCronCreateTool_ValidRecurring(t *testing.T) {
	tool := &CronCreateTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"cron":      "*/5 * * * *",
		"prompt":    "check the deploy",
		"recurring": true,
		"durable":   false,
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Scheduled recurring job") {
		t.Errorf("output should mention recurring job: %q", result.Output)
	}
	if !strings.Contains(result.Output, "Every 5 minutes") {
		t.Errorf("output should contain human schedule: %q", result.Output)
	}
	if !strings.Contains(result.Output, "Session-only") {
		t.Errorf("output should mention session-only: %q", result.Output)
	}
}

func TestCronCreateTool_OneShot(t *testing.T) {
	tool := &CronCreateTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"cron":      "30 14 28 2 *",
		"prompt":    "quarterly review",
		"recurring": false,
		"durable":   false,
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "one-shot") {
		t.Errorf("output should mention one-shot: %q", result.Output)
	}
}

func TestCronCreateTool_Durable(t *testing.T) {
	dir := t.TempDir()
	origGetProjectDir := getProjectDir
	getProjectDir = func() string { return dir }
	defer func() { getProjectDir = origGetProjectDir }()

	tool := &CronCreateTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"cron":    "0 9 * * *",
		"prompt":  "morning checkin",
		"durable": true,
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Persisted") {
		t.Errorf("durable task should mention Persisted: %q", result.Output)
	}

	// Verify written to disk
	tasks, _ := readCronTasks(dir)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task on disk, got %d", len(tasks))
	}
	if tasks[0].Prompt != "morning checkin" {
		t.Errorf("prompt mismatch: %q", tasks[0].Prompt)
	}
}

func TestCronCreateTool_MissingCron(t *testing.T) {
	tool := &CronCreateTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"prompt": "no cron",
	})

	if !result.IsError {
		t.Error("expected error for missing cron")
	}
	if !strings.Contains(result.Output, "cron parameter is required") {
		t.Errorf("wrong error: %q", result.Output)
	}
}

func TestCronCreateTool_MissingPrompt(t *testing.T) {
	tool := &CronCreateTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"cron": "*/5 * * * *",
	})

	if !result.IsError {
		t.Error("expected error for missing prompt")
	}
}

func TestCronCreateTool_InvalidCron(t *testing.T) {
	tool := &CronCreateTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"cron":   "not a cron",
		"prompt": "test",
	})

	if !result.IsError {
		t.Error("expected error for invalid cron")
	}
	if !strings.Contains(result.Output, "invalid cron expression") {
		t.Errorf("wrong error: %q", result.Output)
	}
}

func TestCronCreateTool_NoMatchInNextYear(t *testing.T) {
	tool := &CronCreateTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"cron":   "0 0 31 2 *",
		"prompt": "feb 31 never matches",
	})

	if !result.IsError {
		t.Error("expected error for cron that never matches")
	}
	if !strings.Contains(result.Output, "does not match") {
		t.Errorf("wrong error: %q", result.Output)
	}
}

func TestCronCreateTool_MaxJobsLimit(t *testing.T) {
	dir := t.TempDir()
	origGetProjectDir := getProjectDir
	getProjectDir = func() string { return dir }
	defer func() { getProjectDir = origGetProjectDir }()

	// Fill up to maxJobs
	for i := 0; i < maxJobs; i++ {
		_, _ = addCronTask("*/5 * * * *", "filler", true, true, "", dir)
	}

	tool := &CronCreateTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"cron":   "0 9 * * *",
		"prompt": "over limit",
		"durable": true,
	})

	if !result.IsError {
		t.Error("expected error when over max jobs limit")
	}
	if !strings.Contains(result.Output, "too many scheduled jobs") {
		t.Errorf("wrong error: %q", result.Output)
	}
}

// ---------------------------------------------------------------------------
// CronDeleteTool
// ---------------------------------------------------------------------------

func TestCronDeleteTool_ExistingTask(t *testing.T) {
	dir := t.TempDir()
	origGetProjectDir := getProjectDir
	getProjectDir = func() string { return dir }
	defer func() { getProjectDir = origGetProjectDir }()

	id, _ := addCronTask("*/5 * * * *", "to delete", true, true, "", dir)

	tool := &CronDeleteTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"id": id,
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Cancelled job") {
		t.Errorf("should confirm cancellation: %q", result.Output)
	}

	// Verify deleted
	tasks, _ := readCronTasks(dir)
	for _, ct := range tasks {
		if ct.ID == id {
			t.Error("task should have been deleted")
		}
	}
}

func TestCronDeleteTool_NonexistentID(t *testing.T) {
	tool := &CronDeleteTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"id": "nonexist",
	})

	if !result.IsError {
		t.Error("expected error for nonexistent ID")
	}
	if !strings.Contains(result.Output, "no scheduled job") {
		t.Errorf("wrong error: %q", result.Output)
	}
}

func TestCronDeleteTool_MissingID(t *testing.T) {
	tool := &CronDeleteTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{})

	if !result.IsError {
		t.Error("expected error for missing id")
	}
}

func TestCronDeleteTool_SessionTask(t *testing.T) {
	removeSessionCronTasks(getSessionCronIDs())

	id, _ := addCronTask("*/10 * * * *", "session task", true, false, "", "")

	tool := &CronDeleteTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"id": id,
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Cancelled job") {
		t.Errorf("should confirm cancellation: %q", result.Output)
	}

	// Verify deleted from session store
	for _, st := range getSessionCronTasks() {
		if st.ID == id {
			t.Error("session task should have been removed")
		}
	}
}

// ---------------------------------------------------------------------------
// CronListTool
// ---------------------------------------------------------------------------

func TestCronListTool_Empty(t *testing.T) {
	dir := t.TempDir()
	origGetProjectDir := getProjectDir
	getProjectDir = func() string { return dir }
	defer func() { getProjectDir = origGetProjectDir }()

	tool := &CronListTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "No scheduled tasks") {
		t.Errorf("empty list: %q", result.Output)
	}
}

func TestCronListTool_WithTasks(t *testing.T) {
	dir := t.TempDir()
	origGetProjectDir := getProjectDir
	getProjectDir = func() string { return dir }
	defer func() { getProjectDir = origGetProjectDir }()

	addCronTask("*/5 * * * *", "task one", true, true, "", dir)
	addCronTask("30 14 * * *", "task two", false, true, "", dir)

	tool := &CronListTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "task one") {
		t.Errorf("should contain first task: %q", result.Output)
	}
	if !strings.Contains(result.Output, "task two") {
		t.Errorf("should contain second task: %q", result.Output)
	}
	if !strings.Contains(result.Output, "recurring") {
		t.Errorf("should mention recurring: %q", result.Output)
	}
	if !strings.Contains(result.Output, "one-shot") {
		t.Errorf("should mention one-shot: %q", result.Output)
	}
}

func TestCronListTool_TruncatesLongPrompt(t *testing.T) {
	dir := t.TempDir()
	origGetProjectDir := getProjectDir
	getProjectDir = func() string { return dir }
	defer func() { getProjectDir = origGetProjectDir }()

	longPrompt := strings.Repeat("x", 200)
	addCronTask("*/5 * * * *", longPrompt, true, true, "", dir)

	tool := &CronListTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if strings.Contains(result.Output, longPrompt) {
		t.Error("long prompt should be truncated")
	}
	if !strings.Contains(result.Output, "...") {
		t.Error("truncated prompt should end with ...")
	}
}

// ---------------------------------------------------------------------------
// Tool names and schemas
// ---------------------------------------------------------------------------

func TestCronToolNames(t *testing.T) {
	if (&CronCreateTool{}).Name() != "cron_create" {
		t.Error("CronCreateTool name mismatch")
	}
	if (&CronDeleteTool{}).Name() != "cron_delete" {
		t.Error("CronDeleteTool name mismatch")
	}
	if (&CronListTool{}).Name() != "cron_list" {
		t.Error("CronListTool name mismatch")
	}
}

func TestCronToolSchemas(t *testing.T) {
	createSchema := (&CronCreateTool{}).InputSchema()
	props := createSchema["properties"].(map[string]any)
	if _, ok := props["cron"]; !ok {
		t.Error("CronCreateTool missing cron property")
	}
	if _, ok := props["prompt"]; !ok {
		t.Error("CronCreateTool missing prompt property")
	}

	deleteSchema := (&CronDeleteTool{}).InputSchema()
	props2 := deleteSchema["properties"].(map[string]any)
	if _, ok := props2["id"]; !ok {
		t.Error("CronDeleteTool missing id property")
	}

	listSchema := (&CronListTool{}).InputSchema()
	props3 := listSchema["properties"].(map[string]any)
	if len(props3) != 0 {
		t.Error("CronListTool should have no properties")
	}
}

func TestCronToolDescriptions(t *testing.T) {
	createDesc := (&CronCreateTool{}).Description()
	if !strings.Contains(createDesc, "cron") {
		t.Error("CronCreateTool description should mention cron")
	}

	deleteDesc := (&CronDeleteTool{}).Description()
	if !strings.Contains(deleteDesc, "Cancel") {
		t.Error("CronDeleteTool description should mention Cancel")
	}

	listDesc := (&CronListTool{}).Description()
	if !strings.Contains(listDesc, "scheduled") {
		t.Error("CronListTool description should mention scheduled")
	}
}
