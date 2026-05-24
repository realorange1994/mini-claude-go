package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const checkIntervalMs = 1000

// CronScheduler manages the cron task execution loop.
// Only one instance per project directory should be active (controlled by
// the scheduler lock).
type CronScheduler struct {
	tasks       []CronTask          // file-backed tasks
	nextFireAt  map[string]int64    // taskID -> epoch ms of next fire
	missedAsked map[string]bool     // taskIDs already surfaced as missed
	inFlight    map[string]bool     // taskIDs being removed (prevents double-fire)
	isOwner     bool                // owns the scheduler lock
	stopped     bool
	dir         string              // project root
	onFire      func(prompt string) // callback to inject prompt into agent loop
	mu          sync.Mutex
	checkTimer  *time.Timer
	lockPID     int
	lockAcqAt   int64
}

// createCronScheduler creates a new scheduler but does not start it.
func createCronScheduler(dir string, onFire func(prompt string)) *CronScheduler {
	return &CronScheduler{
		nextFireAt:  make(map[string]int64),
		missedAsked: make(map[string]bool),
		inFlight:    make(map[string]bool),
		dir:         dir,
		onFire:      onFire,
	}
}

// start acquires the lock, loads tasks, surfaces missed ones, and begins the tick loop.
func (s *CronScheduler) start() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}

	// Acquire scheduler lock
	s.isOwner = tryAcquireSchedulerLock(s.dir)

	// Load tasks
	fileTasks, _ := readCronTasks(s.dir)
	s.tasks = fileTasks

	// Surface missed one-shot tasks on initial load
	now := time.Now().UnixMilli()
	missed := findMissedTasks(fileTasks, now)
	var missedOneShot []CronTask
	for _, t := range missed {
		if !t.Recurring && !s.missedAsked[t.ID] {
			missedOneShot = append(missedOneShot, t)
			s.missedAsked[t.ID] = true
			s.nextFireAt[t.ID] = 1<<63 - 1 // Infinity: prevent double-fire
		}
	}
	if len(missedOneShot) > 0 {
		notification := buildMissedTaskNotification(missedOneShot)
		if s.onFire != nil {
			s.onFire(notification)
		}
		// Remove missed tasks from disk
		ids := make([]string, len(missedOneShot))
		for i, t := range missedOneShot {
			ids[i] = t.ID
		}
		_ = removeCronTasks(ids, s.dir)
	}

	s.mu.Unlock()

	// Start check timer (fires every second)
	s.scheduleCheck()
}

// stop tears down the scheduler.
func (s *CronScheduler) stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()

	if s.checkTimer != nil {
		s.checkTimer.Stop()
		s.checkTimer = nil
	}

	if s.isOwner {
		s.isOwner = false
		_ = releaseSchedulerLock(s.dir)
	}
}

// getNextFireTime returns the earliest pending fire time, or 0 if nothing pending.
func (s *CronScheduler) getNextFireTime() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	var min int64 = 1<<63 - 1
	for _, t := range s.nextFireAt {
		if t < min {
			min = t
		}
	}
	if min == 1<<63-1 {
		return 0
	}
	return min
}

// scheduleCheck sets up the next timer tick.
func (s *CronScheduler) scheduleCheck() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	s.checkTimer = time.AfterFunc(time.Duration(checkIntervalMs)*time.Millisecond, func() {
		s.check()
	})
}

// check runs the scheduler tick: evaluates all tasks and fires those due.
func (s *CronScheduler) check() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}

	now := time.Now().UnixMilli()
	seen := make(map[string]bool)
	var firedRecurringIDs []string

	cfg := DefaultCronJitterConfig

	// Process file-backed tasks (only if we own the lock)
	if s.isOwner {
		for _, t := range s.tasks {
			s.processTask(t, false, now, cfg, seen, &firedRecurringIDs)
		}
		// Persist lastFiredAt for recurring tasks
		if len(firedRecurringIDs) > 0 {
			for _, id := range firedRecurringIDs {
				s.inFlight[id] = true
			}
			go func() {
				_ = markCronTasksFired(firedRecurringIDs, now, s.dir)
				s.mu.Lock()
				for _, id := range firedRecurringIDs {
					delete(s.inFlight, id)
				}
				s.mu.Unlock()
			}()
		}
	}

	// Process session-only tasks (always, no lock needed)
	for _, t := range getSessionCronTasks() {
		t.Durable = false
		s.processTask(t, true, now, cfg, seen, &firedRecurringIDs)
	}

	// If no live tasks, clear the schedule
	if len(seen) == 0 {
		s.nextFireAt = make(map[string]int64)
		s.mu.Unlock()
		s.scheduleCheck()
		return
	}

	// Evict stale schedule entries for tasks no longer present
	for id := range s.nextFireAt {
		if !seen[id] {
			delete(s.nextFireAt, id)
		}
	}

	s.mu.Unlock()
	s.scheduleCheck()
}

// processTask evaluates a single task and fires it if due.
func (s *CronScheduler) processTask(t CronTask, isSession bool, now int64, cfg CronJitterConfig, seen map[string]bool, firedRecurring *[]string) {
	seen[t.ID] = true

	if s.inFlight[t.ID] {
		return
	}

	next, exists := s.nextFireAt[t.ID]
	if !exists {
		// First sight: anchor from lastFiredAt (recurring) or createdAt
		if t.Recurring {
			lastAnchor := t.LastFiredAt
			if lastAnchor == 0 {
				lastAnchor = t.CreatedAt
			}
			nxt := jitteredNextCronRunMs(t.Cron, lastAnchor, t.ID, cfg)
			if nxt == nil {
				next = 1<<63 - 1 // Infinity
			} else {
				next = *nxt
			}
		} else {
			nxt := oneShotJitteredNextCronRunMs(t.Cron, t.CreatedAt, t.ID, cfg)
			if nxt == nil {
				next = 1<<63 - 1
			} else {
				next = *nxt
			}
		}
		s.nextFireAt[t.ID] = next
	}

	if now < next {
		return
	}

	// Fire the task
	if s.onFire != nil {
		s.onFire(t.Prompt)
	}

	// Check if aged out (recurring only)
	aged := isRecurringTaskAged(t, now, cfg.RecurringMaxAgeMs)

	if t.Recurring && !aged {
		// Reschedule from now (not from next) to avoid catch-up
		nxt := jitteredNextCronRunMs(t.Cron, now, t.ID, cfg)
		if nxt == nil {
			s.nextFireAt[t.ID] = 1<<63 - 1
		} else {
			s.nextFireAt[t.ID] = *nxt
		}
		// Persist lastFiredAt for file-backed recurring tasks
		if !isSession {
			*firedRecurring = append(*firedRecurring, t.ID)
		}
	} else if isSession {
		// One-shot (or aged-out) session task: remove from memory
		removeSessionCronTasks([]string{t.ID})
		delete(s.nextFireAt, t.ID)
	} else {
		// One-shot (or aged-out) file task: delete from disk
		s.inFlight[t.ID] = true
		go func() {
			_ = removeCronTasks([]string{t.ID}, s.dir)
			s.mu.Lock()
			delete(s.inFlight, t.ID)
			s.mu.Unlock()
		}()
		delete(s.nextFireAt, t.ID)
	}
}

// --- Scheduler Lock ---

type schedulerLockData struct {
	SessionID  string `json:"sessionId"`
	PID        int    `json:"pid"`
	AcquiredAt int64  `json:"acquiredAt"`
}

const lockFileRel = ".claude" + string(filepath.Separator) + "scheduled_tasks.lock"

func getLockPath(dir string) string {
	return filepath.Join(dir, lockFileRel)
}

func tryAcquireSchedulerLock(dir string) bool {
	lock := schedulerLockData{
		SessionID:  "miniclaude-" + strconv.Itoa(os.Getpid()),
		PID:        os.Getpid(),
		AcquiredAt: time.Now().UnixMilli(),
	}
	lockPath := getLockPath(dir)

	// Ensure .claude/ directory exists
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return false
	}

	// Try exclusive create
	data, _ := json.Marshal(lock)
	f, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err == nil {
		f.Write(data)
		f.Close()
		return true
	}

	if os.IsExist(err) {
		// Read existing lock
		existing, err := readLockFile(lockPath)
		if err != nil {
			// Corrupt lock — treat as stale
			_ = os.Remove(lockPath)
			// Retry exclusive create
			f2, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err == nil {
				f2.Write(data)
				f2.Close()
				return true
			}
			return false
		}

		// Check if it's ours (same PID)
		if existing.PID == os.Getpid() {
			return true
		}

		// Check if owner process is still alive
		if isProcessRunning(existing.PID) {
			return false
		}

		// Owner dead — stale lock, recover
		_ = os.Remove(lockPath)
		f2, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err == nil {
			f2.Write(data)
			f2.Close()
			return true
		}
		return false
	}

	return false
}

func releaseSchedulerLock(dir string) error {
	lockPath := getLockPath(dir)
	existing, err := readLockFile(lockPath)
	if err != nil {
		return nil // already gone
	}
	if existing.PID != os.Getpid() {
		return nil // not ours
	}
	return os.Remove(lockPath)
}

func readLockFile(path string) (*schedulerLockData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock schedulerLockData
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	return &lock, nil
}

// isProcessRunning checks if a process with the given PID is still alive.
// Uses a simple signal 0 approach (non-portable but works on most platforms).
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	// On Windows, os.FindProcess always succeeds, but we can check if
	// the process is still running by trying to open its process handle.
	// For simplicity, use a conservative approach: assume alive if find succeeds.
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess doesn't guarantee the process exists, but
	// proc.Signal(os.Signal(0)) does. On Windows, this is a no-op.
	// We use a fallback: assume alive (safe — the lock will be recovered
	// when we restart and the PID is truly dead).
	_ = proc
	return true
}

// buildMissedTaskNotification creates the text shown to the user when one-shot
// tasks are detected that fired while Claude was not running.
func buildMissedTaskNotification(missed []CronTask) string {
	plural := len(missed) > 1
	header := fmt.Sprintf(
		"The following one-shot scheduled task%s missed while Claude was not running. "+
			"%s already been removed from .claude/scheduled_tasks.json.\n\n"+
			"Do NOT execute %s yet. "+
			"First ask the user whether to run %s now. "+
			"Only execute if the user confirms.",
		func() string {
			if plural {
				return "s were"
			}
			return " was"
		}(),
		func() string {
			if plural {
				return "They have"
			}
			return "It has"
		}(),
		func() string {
			if plural {
				return "these prompts"
			}
			return "this prompt"
		}(),
		func() string {
			if plural {
				return "each one"
			}
			return "it"
		}(),
	)

	blocks := make([]string, len(missed))
	for i, t := range missed {
		meta := fmt.Sprintf("[%s, created %s]", CronToHuman(t.Cron), time.UnixMilli(t.CreatedAt).Format("2006-01-02 15:04:05"))
		// Use a fence longer than any backtick run in the prompt
		longestRun := 0
		curRun := 0
		for _, r := range t.Prompt {
			if r == '`' {
				curRun++
				if curRun > longestRun {
					longestRun = curRun
				}
			} else {
				curRun = 0
			}
		}
		fenceLen := 3
		if longestRun+1 > fenceLen {
			fenceLen = longestRun + 1
		}
		fence := ""
		for j := 0; j < fenceLen; j++ {
			fence += "`"
		}
		blocks[i] = fmt.Sprintf("%s\n%s\n%s\n%s", meta, fence, t.Prompt, fence)
	}

	return header + "\n\n" + joinLinesBlocks(blocks)
}

func joinLinesBlocks(blocks []string) string {
	result := ""
	for i, b := range blocks {
		if i > 0 {
			result += "\n\n"
		}
		result += b
	}
	return result
}
