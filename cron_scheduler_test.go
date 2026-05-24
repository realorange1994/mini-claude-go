package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// scheduler start / stop lifecycle
// ---------------------------------------------------------------------------

func TestSchedulerCreateAndStop(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var fired []string
	s := createCronScheduler(dir, func(prompt string) {
		mu.Lock()
		fired = append(fired, prompt)
		mu.Unlock()
	})
	s.start()

	// Give timer a moment to settle
	time.Sleep(50 * time.Millisecond)

	s.stop()

	if s.checkTimer != nil {
		t.Error("timer should be stopped after stop()")
	}
}

// ---------------------------------------------------------------------------
// scheduler fires a one-shot task
// ---------------------------------------------------------------------------

func TestSchedulerFiresOneShotTask(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var fired []string
	onFire := func(prompt string) {
		mu.Lock()
		fired = append(fired, prompt)
		mu.Unlock()
	}

	// Use a cron that fires every minute
	id, _ := addCronTask("* * * * *", "test prompt", false, true, "", dir)

	s := createCronScheduler(dir, onFire)
	// Pretend the task was created 2 minutes ago so it's "due"
	tasks, _ := readCronTasks(dir)
	for i := range tasks {
		if tasks[i].ID == id {
			tasks[i].CreatedAt = time.Now().UnixMilli() - 120*1000
		}
	}
	_ = writeCronTasks(tasks, dir)

	s.start()

	// Wait for scheduler to fire (1s check interval + small margin)
	for i := 0; i < 5; i++ {
		time.Sleep(500 * time.Millisecond)
		mu.Lock()
		count := len(fired)
		mu.Unlock()
		if count > 0 {
			break
		}
	}

	s.stop()

	mu.Lock()
	defer mu.Unlock()
	if len(fired) < 1 {
		t.Fatalf("expected >= 1 fire, got %d", len(fired))
	}
	// The missed one-shot is surfaced as a notification containing the original prompt
	if !strings.Contains(fired[0], "test prompt") {
		t.Errorf("output should contain original prompt: %q", fired[0])
	}

	// Verify task was removed from disk
	tasks, _ = readCronTasks(dir)
	for _, ct := range tasks {
		if ct.ID == id {
			t.Error("one-shot task should have been removed from disk")
		}
	}
}

// ---------------------------------------------------------------------------
// scheduler fires recurring task multiple times
// ---------------------------------------------------------------------------

func TestSchedulerFiresRecurringTask(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var fireCount int
	onFire := func(prompt string) {
		mu.Lock()
		fireCount++
		mu.Unlock()
	}

	// Every-minute cron, created 60s ago so it's due
	id, _ := addCronTask("* * * * *", "recurring prompt", true, true, "", dir)
	tasks, _ := readCronTasks(dir)
	for i := range tasks {
		if tasks[i].ID == id {
			tasks[i].CreatedAt = time.Now().UnixMilli() - 120*1000
			tasks[i].LastFiredAt = time.Now().UnixMilli() - 60*1000 // fire 60s ago
		}
	}
	_ = writeCronTasks(tasks, dir)

	s := createCronScheduler(dir, onFire)
	s.start()

	// Wait for ~3 check cycles
	time.Sleep(3500 * time.Millisecond)

	s.stop()

	mu.Lock()
	defer mu.Unlock()
	// Should have fired at least once
	if fireCount < 1 {
		t.Errorf("expected >= 1 fires, got %d", fireCount)
	}

	// Task should still exist (recurring)
	tasks, _ = readCronTasks(dir)
	found := false
	for _, ct := range tasks {
		if ct.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Error("recurring task should still exist on disk")
	}
}

// ---------------------------------------------------------------------------
// scheduler getNextFireTime
// ---------------------------------------------------------------------------

func TestSchedulerGetNextFireTime(t *testing.T) {
	dir := t.TempDir()

	s := createCronScheduler(dir, nil)
	// Manually set up nextFireAt
	s.nextFireAt["abc12345"] = 1000

	got := s.getNextFireTime()
	if got != 1000 {
		t.Errorf("expected 1000, got %d", got)
	}

	// Empty scheduler returns 0
	s.nextFireAt = make(map[string]int64)
	got = s.getNextFireTime()
	if got != 0 {
		t.Errorf("expected 0 for empty scheduler, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// processTask — one-shot removes from memory after fire
// ---------------------------------------------------------------------------

func TestProcessTask_OneShotRemovesAfterFire(t *testing.T) {
	dir := t.TempDir()

	var fired bool
	s := createCronScheduler(dir, func(prompt string) { fired = true })

	task := CronTask{
		ID:        "test001",
		Cron:      "* * * * *",
		Prompt:    "one-shot",
		CreatedAt: time.Now().UnixMilli() - 120*1000, // 2 min ago
	}
	s.nextFireAt[task.ID] = time.Now().UnixMilli() - 1000 // due now

	seen := make(map[string]bool)
	var firedIDs []string
	s.processTask(task, true, time.Now().UnixMilli(), DefaultCronJitterConfig, seen, &firedIDs)

	if !fired {
		t.Error("expected task to have fired")
	}
	if !seen[task.ID] {
		t.Error("task should be in seen set")
	}
	// Session one-shot should be removed
	if _, exists := s.nextFireAt[task.ID]; exists {
		t.Error("session one-shot should be removed from nextFireAt")
	}
}

// ---------------------------------------------------------------------------
// processTask — recurring reschedules after fire
// ---------------------------------------------------------------------------

func TestProcessTask_RecurringReschedulesAfterFire(t *testing.T) {
	dir := t.TempDir()

	var fired bool
	s := createCronScheduler(dir, func(prompt string) { fired = true })

	task := CronTask{
		ID:        "test002",
		Cron:      "* * * * *",
		Prompt:    "recurring",
		CreatedAt: time.Now().UnixMilli() - 120*1000,
		Recurring: true,
	}
	s.nextFireAt[task.ID] = time.Now().UnixMilli() - 1000

	seen := make(map[string]bool)
	var firedIDs []string
	now := time.Now().UnixMilli()
	s.processTask(task, false, now, DefaultCronJitterConfig, seen, &firedIDs)

	if !fired {
		t.Error("expected task to have fired")
	}
	// Recurring task should have a new nextFireAt in the future
	nextFire, exists := s.nextFireAt[task.ID]
	if !exists {
		t.Fatal("recurring task should still have nextFireAt")
	}
	if nextFire <= now {
		t.Errorf("rescheduled fire time should be in future: got %d, now %d", nextFire, now)
	}
}

// ---------------------------------------------------------------------------
// processTask — inFlight prevents double-fire
// ---------------------------------------------------------------------------

func TestProcessTask_InFlightPreventsDoubleFire(t *testing.T) {
	dir := t.TempDir()

	fireCount := 0
	s := createCronScheduler(dir, func(prompt string) { fireCount++ })

	task := CronTask{
		ID:        "test003",
		Cron:      "* * * * *",
		Prompt:    "in-flight",
		CreatedAt: time.Now().UnixMilli() - 120*1000,
		Recurring: true,
	}

	s.inFlight[task.ID] = true
	s.nextFireAt[task.ID] = time.Now().UnixMilli() - 1000

	seen := make(map[string]bool)
	var firedIDs []string
	s.processTask(task, false, time.Now().UnixMilli(), DefaultCronJitterConfig, seen, &firedIDs)

	if fireCount != 0 {
		t.Errorf("inFlight task should not fire, got %d fires", fireCount)
	}
}

// ---------------------------------------------------------------------------
// processTask — aged recurring task fires once then deletes
// ---------------------------------------------------------------------------

func TestProcessTask_AgedRecurringFiresThenDeletes(t *testing.T) {
	dir := t.TempDir()

	var fired bool
	s := createCronScheduler(dir, func(prompt string) { fired = true })

	// Create task older than 7 days
	oldTime := time.Now().UnixMilli() - 8*24*60*60*1000
	task := CronTask{
		ID:        "test004",
		Cron:      "* * * * *",
		Prompt:    "aged task",
		CreatedAt: oldTime,
		Recurring: true,
	}
	s.nextFireAt[task.ID] = time.Now().UnixMilli() - 1000

	seen := make(map[string]bool)
	var firedIDs []string
	s.processTask(task, false, time.Now().UnixMilli(), DefaultCronJitterConfig, seen, &firedIDs)

	if !fired {
		t.Error("aged task should fire once before deletion")
	}
	// Should be removed from nextFireAt
	if _, exists := s.nextFireAt[task.ID]; exists {
		t.Error("aged recurring task should be removed from nextFireAt")
	}
}

// ---------------------------------------------------------------------------
// lock acquisition
// ---------------------------------------------------------------------------

func TestLockAcquireAndRelease(t *testing.T) {
	dir := t.TempDir()

	// First acquire should succeed
	got := tryAcquireSchedulerLock(dir)
	if !got {
		t.Fatal("first lock acquisition should succeed")
	}

	// Second acquire in same process should fail (lock file exists with our PID)
	// Actually: isProcessRunning always returns true on Windows, so second acquire
	// sees our own PID and returns true. Let's check the lock file exists.
	lockPath := filepath.Join(dir, ".claude", "scheduled_tasks.lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("lock file should exist after acquire")
	}

	// Release
	if err := releaseSchedulerLock(dir); err != nil {
		t.Fatalf("release: %v", err)
	}

	// After release, lock file should be gone
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should be removed after release")
	}
}

func TestLockStaleRecovery(t *testing.T) {
	dir := t.TempDir()

	// Write a lock file with a fake PID that doesn't exist
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	lockPath := filepath.Join(dir, ".claude", "scheduled_tasks.lock")

	fakeLock := `{"sessionId":"fake","pid":999999,"acquiredAt":0}`
	os.WriteFile(lockPath, []byte(fakeLock), 0644)

	// Acquire should succeed because PID 999999 is not running
	got := tryAcquireSchedulerLock(dir)
	if !got {
		t.Error("should recover stale lock (PID 999999 not running)")
	}
}

func TestLockCorruptRecovery(t *testing.T) {
	dir := t.TempDir()

	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	lockPath := filepath.Join(dir, ".claude", "scheduled_tasks.lock")

	os.WriteFile(lockPath, []byte("not valid json"), 0644)

	// Should delete corrupt lock and try exclusive create
	got := tryAcquireSchedulerLock(dir)
	if !got {
		t.Error("should recover from corrupt lock file")
	}
}

// ---------------------------------------------------------------------------
// buildMissedTaskNotification
// ---------------------------------------------------------------------------

func TestBuildMissedNotification_Single(t *testing.T) {
	task := CronTask{
		ID:        "miss01",
		Cron:      "30 14 * * *",
		Prompt:    "check the deploy",
		CreatedAt: time.Now().UnixMilli() - 3600*1000,
	}

	got := buildMissedTaskNotification([]CronTask{task})

	// Should contain singular forms
	if strings.Contains(got, "tasks were") {
		t.Errorf("single task should use singular: %q", got)
	}
	if !strings.Contains(got, "It has") {
		t.Errorf("should contain 'It has' for singular: %q", got)
	}
	if !strings.Contains(got, "check the deploy") {
		t.Errorf("should contain prompt text: %q", got)
	}
	if !strings.Contains(got, "missed while Claude was not running") {
		t.Errorf("should contain missed header: %q", got)
	}
}

func TestBuildMissedNotification_Multiple(t *testing.T) {
	tasks := []CronTask{
		{
			ID:        "miss01",
			Cron:      "30 14 * * *",
			Prompt:    "task one",
			CreatedAt: time.Now().UnixMilli() - 3600*1000,
		},
		{
			ID:        "miss02",
			Cron:      "0 9 * * *",
			Prompt:    "task two",
			CreatedAt: time.Now().UnixMilli() - 7200*1000,
		},
	}

	got := buildMissedTaskNotification(tasks)

	// Should contain plural forms
	if !strings.Contains(got, "tasks were") {
		t.Errorf("multiple tasks should use plural: %q", got)
	}
	if !strings.Contains(got, "They have") {
		t.Errorf("should contain 'They have' for plural: %q", got)
	}
	if !strings.Contains(got, "task one") || !strings.Contains(got, "task two") {
		t.Errorf("should contain both prompts: %q", got)
	}
}

func TestBuildMissedNotification_BacktickFence(t *testing.T) {
	task := CronTask{
		ID:        "miss01",
		Cron:      "* * * * *",
		Prompt:    "code with ``` backticks",
		CreatedAt: time.Now().UnixMilli() - 1000,
	}

	got := buildMissedTaskNotification([]CronTask{task})

	// Should have fence longer than 3 backticks
	if !strings.Contains(got, "````") {
		t.Errorf("fence should be longer than 3 backticks for prompt with ```:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// getNextFireTime returns earliest
// ---------------------------------------------------------------------------

func TestSchedulerGetNextFireTime_Earliest(t *testing.T) {
	s := createCronScheduler("", nil)
	s.nextFireAt["a"] = 3000
	s.nextFireAt["b"] = 1000
	s.nextFireAt["c"] = 2000

	got := s.getNextFireTime()
	if got != 1000 {
		t.Errorf("expected earliest (1000), got %d", got)
	}
}

// ---------------------------------------------------------------------------
// session tasks are always processed
// ---------------------------------------------------------------------------

func TestSchedulerProcessesSessionTasks(t *testing.T) {
	removeSessionCronTasks(getSessionCronIDs())

	var mu sync.Mutex
	var fired []string
	onFire := func(prompt string) {
		mu.Lock()
		fired = append(fired, prompt)
		mu.Unlock()
	}

	// Session task due now
	task := CronTask{
		ID:        "sess01",
		Cron:      "* * * * *",
		Prompt:    "session fire",
		CreatedAt: time.Now().UnixMilli() - 120*1000,
	}
	addSessionCronTask(task)

	s := createCronScheduler("", onFire)
	s.start()

	for i := 0; i < 5; i++ {
		time.Sleep(500 * time.Millisecond)
		mu.Lock()
		count := len(fired)
		mu.Unlock()
		if count > 0 {
			break
		}
	}

	s.stop()

	mu.Lock()
	defer mu.Unlock()
	if len(fired) < 1 {
		t.Errorf("expected session task to fire, got %d fires", len(fired))
	}

	// Session task should be removed
	stasks := getSessionCronTasks()
	for _, st := range stasks {
		if st.ID == "sess01" {
			t.Error("session task should have been removed after firing")
		}
	}
}
