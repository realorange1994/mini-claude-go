package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// addCronTask + readCronTasks roundtrip
// ---------------------------------------------------------------------------

func TestAddAndReadCronTask(t *testing.T) {
	dir := t.TempDir()

	id, err := addCronTask("*/5 * * * *", "check the deploy", true, true, "", dir)
	if err != nil {
		t.Fatalf("addCronTask: %v", err)
	}
	if len(id) != 8 {
		t.Errorf("expected 8-char ID, got %q (len=%d)", id, len(id))
	}

	tasks, err := readCronTasks(dir)
	if err != nil {
		t.Fatalf("readCronTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != id {
		t.Errorf("ID: got %q, want %q", tasks[0].ID, id)
	}
	if tasks[0].Cron != "*/5 * * * *" {
		t.Errorf("Cron: got %q", tasks[0].Cron)
	}
	if tasks[0].Prompt != "check the deploy" {
		t.Errorf("Prompt: got %q", tasks[0].Prompt)
	}
	if !tasks[0].Recurring {
		t.Error("expected Recurring=true")
	}
	if !tasks[0].Durable {
		t.Error("expected Durable=true for file-backed task")
	}
}

func TestAddOneShotCronTask(t *testing.T) {
	dir := t.TempDir()

	id, err := addCronTask("30 14 28 2 *", "review quarterly", false, true, "", dir)
	if err != nil {
		t.Fatalf("addCronTask: %v", err)
	}

	tasks, _ := readCronTasks(dir)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != id {
		t.Errorf("ID mismatch")
	}
	if tasks[0].Recurring {
		t.Error("expected Recurring=false for one-shot")
	}
}

// ---------------------------------------------------------------------------
// removeCronTasks by ID
// ---------------------------------------------------------------------------

func TestRemoveCronTask(t *testing.T) {
	dir := t.TempDir()

	id1, _ := addCronTask("*/5 * * * *", "task1", true, true, "", dir)
	id2, _ := addCronTask("0 9 * * *", "task2", true, true, "", dir)

	// Remove first task
	if err := removeCronTasks([]string{id1}, dir); err != nil {
		t.Fatalf("removeCronTasks: %v", err)
	}

	tasks, _ := readCronTasks(dir)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task after removal, got %d", len(tasks))
	}
	if tasks[0].ID != id2 {
		t.Errorf("remaining task ID: got %q, want %q", tasks[0].ID, id2)
	}

	// Remove second task
	if err := removeCronTasks([]string{id2}, dir); err != nil {
		t.Fatalf("removeCronTasks: %v", err)
	}
	tasks, _ = readCronTasks(dir)
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

// ---------------------------------------------------------------------------
// session-only tasks
// ---------------------------------------------------------------------------

func TestSessionCronTask(t *testing.T) {
	// Clear session store
	removeSessionCronTasks(getSessionCronIDs())

	id, err := addCronTask("*/10 * * * *", "session task", true, false, "", "")
	if err != nil {
		t.Fatalf("addCronTask session: %v", err)
	}

	stasks := getSessionCronTasks()
	found := false
	for _, st := range stasks {
		if st.ID == id {
			found = true
			if st.Durable {
				t.Error("session task should not be durable")
			}
		}
	}
	if !found {
		t.Errorf("session task %q not found", id)
	}

	// Remove
	removeSessionCronTasks([]string{id})
	stasks = getSessionCronTasks()
	for _, st := range stasks {
		if st.ID == id {
			t.Error("session task should have been removed")
		}
	}
}

func getSessionCronIDs() []string {
	tasks := getSessionCronTasks()
	ids := make([]string, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	return ids
}

// ---------------------------------------------------------------------------
// listAllCronTasks merges file + session
// ---------------------------------------------------------------------------

func TestListAllCronTasks(t *testing.T) {
	dir := t.TempDir()
	// Clear session store
	removeSessionCronTasks(getSessionCronIDs())

	// Add file-backed task
	_, _ = addCronTask("0 9 * * *", "file task", true, true, "", dir)

	// Add session task
	_, _ = addCronTask("*/15 * * * *", "session task", true, false, "", "")

	all := listAllCronTasks(dir)
	if len(all) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(all))
	}

	fileCount := 0
	sessionCount := 0
	for _, ct := range all {
		if ct.Durable {
			fileCount++
		} else {
			sessionCount++
		}
	}
	if fileCount != 1 {
		t.Errorf("expected 1 file task, got %d", fileCount)
	}
	if sessionCount != 1 {
		t.Errorf("expected 1 session task, got %d", sessionCount)
	}

	// Cleanup session
	removeSessionCronTasks(getSessionCronIDs())
}

// ---------------------------------------------------------------------------
// findMissedTasks
// ---------------------------------------------------------------------------

func TestFindMissedTasks(t *testing.T) {
	now := time.Now().UnixMilli()

	// Task created 1 hour ago with every-minute cron — definitely missed
	pastTask := CronTask{
		ID:        "missed01",
		Cron:      "* * * * *",
		Prompt:    "past task",
		CreatedAt: now - 3600*1000, // 1 hour ago
	}

	// Task scheduled 1 minute in the future (not missed)
	futureTask := CronTask{
		ID:        "future01",
		Cron:      "0 0 1 1 *",
		Prompt:    "future task",
		CreatedAt: now,
	}

	missed := findMissedTasks([]CronTask{pastTask, futureTask}, now)
	if len(missed) != 1 {
		t.Fatalf("expected 1 missed task, got %d", len(missed))
	}
	if missed[0].ID != "missed01" {
		t.Errorf("missed task ID: got %q, want %q", missed[0].ID, "missed01")
	}
}

func TestFindMissedTasks_NoneMissed(t *testing.T) {
	now := time.Now().UnixMilli()

	task := CronTask{
		ID:        "future02",
		Cron:      "0 9 * * *",
		Prompt:    "future task",
		CreatedAt: now,
	}

	missed := findMissedTasks([]CronTask{task}, now)
	if len(missed) != 0 {
		t.Errorf("expected 0 missed tasks, got %d", len(missed))
	}
}

// ---------------------------------------------------------------------------
// jitteredNextCronRunMs — recurring forward jitter
// ---------------------------------------------------------------------------

func TestJitteredNextCronRunMs_ProducesTimeAfterBase(t *testing.T) {
	now := time.Now().UnixMilli()
	cfg := DefaultCronJitterConfig

	result := jitteredNextCronRunMs("*/5 * * * *", now, "a1b2c3d4", cfg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result <= now {
		t.Errorf("jittered next must be after now: got %d, now %d", *result, now)
	}
}

func TestJitteredNextCronRunMs_JitterWithinCap(t *testing.T) {
	now := time.Now().UnixMilli()
	cfg := DefaultCronJitterConfig

	// Compute without jitter
	base := nextCronRunMs("*/5 * * * *", now)
	if base == nil {
		t.Fatal("nextCronRunMs returned nil")
	}

	// Compute with jitter
	jittered := jitteredNextCronRunMs("*/5 * * * *", now, "a1b2c3d4", cfg)
	if jittered == nil {
		t.Fatal("jitteredNextCronRunMs returned nil")
	}

	// Jittered time should be >= base and <= base + cap
	diff := *jittered - *base
	if diff < 0 {
		t.Errorf("jittered time before base: diff=%d", diff)
	}
	if diff > cfg.RecurringCapMs {
		t.Errorf("jitter exceeds cap: diff=%dms, cap=%dms", diff, cfg.RecurringCapMs)
	}
}

// ---------------------------------------------------------------------------
// oneShotJitteredNextCronRunMs — backward jitter on :00/:30
// ---------------------------------------------------------------------------

func TestOneShotJitteredNextCronRunMs_NonBoundaryNoJitter(t *testing.T) {
	now := time.Now().UnixMilli()
	cfg := DefaultCronJitterConfig

	result := oneShotJitteredNextCronRunMs("7 14 * * *", now, "a1b2c3d4", cfg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// For non-boundary minutes, should match plain nextCronRunMs
	plain := nextCronRunMs("7 14 * * *", now)
	if plain == nil {
		t.Fatal("plain nextCronRunMs returned nil")
	}
	if *result != *plain {
		t.Errorf("non-boundary one-shot should not have jitter: got %d, plain %d", *result, *plain)
	}
}

func TestOneShotJitteredNextCronRunMs_BoundaryHasBackwardJitter(t *testing.T) {
	now := time.Now().UnixMilli()
	cfg := DefaultCronJitterConfig

	result := oneShotJitteredNextCronRunMs("0 14 * * *", now, "a1b2c3d4", cfg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	plain := nextCronRunMs("0 14 * * *", now)
	if plain == nil {
		t.Fatal("plain nextCronRunMs returned nil")
	}

	// Result should be <= plain (backward jitter) and >= now
	if *result > *plain {
		t.Errorf("one-shot jittered should be <= plain: got %d, plain %d", *result, *plain)
	}
	if *result < now {
		t.Errorf("one-shot jittered should be >= now: got %d, now %d", *result, now)
	}
}

// ---------------------------------------------------------------------------
// isRecurringTaskAged
// ---------------------------------------------------------------------------

func TestIsRecurringTaskAged_NotAged(t *testing.T) {
	now := time.Now().UnixMilli()
	cfg := DefaultCronJitterConfig

	task := CronTask{
		ID:        "young01",
		Cron:      "*/5 * * * *",
		Prompt:    "test",
		CreatedAt: now - 1000, // just created
		Recurring: true,
	}

	if isRecurringTaskAged(task, now, cfg.RecurringMaxAgeMs) {
		t.Error("young recurring task should not be aged")
	}
}

func TestIsRecurringTaskAged_Aged(t *testing.T) {
	now := time.Now().UnixMilli()
	cfg := DefaultCronJitterConfig

	task := CronTask{
		ID:        "old01",
		Cron:      "*/5 * * * *",
		Prompt:    "test",
		CreatedAt: now - cfg.RecurringMaxAgeMs - 1000, // older than max age
		Recurring: true,
	}

	if !isRecurringTaskAged(task, now, cfg.RecurringMaxAgeMs) {
		t.Error("old recurring task should be aged")
	}
}

func TestIsRecurringTaskAged_PermanentExempt(t *testing.T) {
	now := time.Now().UnixMilli()
	cfg := DefaultCronJitterConfig

	task := CronTask{
		ID:        "perm01",
		Cron:      "*/5 * * * *",
		Prompt:    "test",
		CreatedAt: now - cfg.RecurringMaxAgeMs - 1000,
		Recurring: true,
		Permanent: true,
	}

	if isRecurringTaskAged(task, now, cfg.RecurringMaxAgeMs) {
		t.Error("permanent task should not be aged")
	}
}

func TestIsRecurringTaskAged_OneShotNotAged(t *testing.T) {
	now := time.Now().UnixMilli()
	cfg := DefaultCronJitterConfig

	task := CronTask{
		ID:        "oneshot01",
		Cron:      "30 14 * * *",
		Prompt:    "test",
		CreatedAt: now - cfg.RecurringMaxAgeMs - 1000,
		Recurring: false,
	}

	if isRecurringTaskAged(task, now, cfg.RecurringMaxAgeMs) {
		t.Error("one-shot task should not be aged (only recurring)")
	}
}

// ---------------------------------------------------------------------------
// markCronTasksFired
// ---------------------------------------------------------------------------

func TestMarkCronTasksFired(t *testing.T) {
	dir := t.TempDir()

	id1, _ := addCronTask("*/5 * * * *", "task1", true, true, "", dir)
	_, _ = addCronTask("0 9 * * *", "task2", true, true, "", dir)

	now := time.Now().UnixMilli()
	if err := markCronTasksFired([]string{id1}, now, dir); err != nil {
		t.Fatalf("markCronTasksFired: %v", err)
	}

	tasks, _ := readCronTasks(dir)
	for _, ct := range tasks {
		if ct.ID == id1 {
			if ct.LastFiredAt != now {
				t.Errorf("LastFiredAt: got %d, want %d", ct.LastFiredAt, now)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// hasCronTasksSync
// ---------------------------------------------------------------------------

func TestHasCronTasksSync(t *testing.T) {
	dir := t.TempDir()

	if hasCronTasksSync(dir) {
		t.Error("expected false for empty dir")
	}

	_, _ = addCronTask("*/5 * * * *", "test", true, true, "", dir)

	if !hasCronTasksSync(dir) {
		t.Error("expected true after adding task")
	}
}

// ---------------------------------------------------------------------------
// generateTaskID
// ---------------------------------------------------------------------------

func TestGenerateTaskID(t *testing.T) {
	id := generateTaskID()
	if len(id) != 8 {
		t.Errorf("expected 8-char ID, got %q (len=%d)", id, len(id))
	}
	// Should be hex
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex char in ID: %c", c)
		}
	}
}

// ---------------------------------------------------------------------------
// jitterFrac
// ---------------------------------------------------------------------------

func TestJitterFrac_Deterministic(t *testing.T) {
	v1 := jitterFrac("a1b2c3d4")
	v2 := jitterFrac("a1b2c3d4")
	if v1 != v2 {
		t.Errorf("jitterFrac should be deterministic: %f != %f", v1, v2)
	}
}

func TestJitterFrac_InRange(t *testing.T) {
	for _, id := range []string{"00000000", "a1b2c3d4", "ffffffff", "12345678"} {
		v := jitterFrac(id)
		if v < 0 || v >= 1 {
			t.Errorf("jitterFrac(%q) = %f, want [0, 1)", id, v)
		}
	}
}

// ---------------------------------------------------------------------------
// writeCronTasks strips runtime-only fields
// ---------------------------------------------------------------------------

func TestWriteCronTasks_StripsRuntimeFields(t *testing.T) {
	dir := t.TempDir()

	tasks := []CronTask{
		{
			ID:        "test01",
			Cron:      "*/5 * * * *",
			Prompt:    "test",
			CreatedAt: nowMs(),
			Recurring: true,
			Durable:   true,  // runtime-only — should be stripped
			AgentID:   "foo", // runtime-only — should be stripped
		},
	}

	if err := writeCronTasks(tasks, dir); err != nil {
		t.Fatalf("writeCronTasks: %v", err)
	}

	readBack, _ := readCronTasks(dir)
	if len(readBack) != 1 {
		t.Fatalf("expected 1 task, got %d", len(readBack))
	}
	// Durable and AgentID are json:"-" so they won't be in the file
	if readBack[0].AgentID != "" {
		t.Errorf("AgentID should be empty from file read, got %q", readBack[0].AgentID)
	}
}

// ---------------------------------------------------------------------------
// readCronTasks drops invalid cron expressions
// ---------------------------------------------------------------------------

func TestReadCronTasks_DropsInvalidCron(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	file := CronFile{
		Tasks: []CronTask{
			{ID: "good01", Cron: "*/5 * * * *", Prompt: "valid", CreatedAt: nowMs()},
			{ID: "bad01", Cron: "invalid", Prompt: "invalid cron", CreatedAt: nowMs()},
			{ID: "bad02", Cron: "", Prompt: "empty cron", CreatedAt: nowMs()},
		},
	}
	data, _ := json.MarshalIndent(file, "", "  ")
	os.WriteFile(getCronFilePath(dir), data, 0644)

	tasks, _ := readCronTasks(dir)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 valid task, got %d", len(tasks))
	}
	if tasks[0].ID != "good01" {
		t.Errorf("expected good01, got %q", tasks[0].ID)
	}
}
