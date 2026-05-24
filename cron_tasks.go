package main

import (
	"encoding/hex"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CronTask represents a scheduled prompt stored in .claude/scheduled_tasks.json.
//
// Tasks come in two flavors:
//   - One-shot (Recurring: false) — fire once, then auto-delete.
//   - Recurring (Recurring: true) — fire on schedule, reschedule from now,
//     persist until explicitly deleted or auto-expire after recurringMaxAgeMs.
type CronTask struct {
	ID          string `json:"id"`
	Cron        string `json:"cron"`
	Prompt      string `json:"prompt"`
	CreatedAt   int64  `json:"createdAt"`       // epoch ms
	LastFiredAt int64  `json:"lastFiredAt,omitempty"` // epoch ms, for recurring reschedule after restart
	Recurring   bool   `json:"recurring,omitempty"`
	Permanent   bool   `json:"permanent,omitempty"` // exempt from auto-expiry (assistant mode built-in tasks)
	// Runtime-only fields (never written to disk):
	Durable bool   `json:"-"` // false → session-scoped, never persisted
	AgentID string `json:"-"` // teammate routing (session-only)
}

// CronFile is the JSON structure for .claude/scheduled_tasks.json.
type CronFile struct {
	Tasks []CronTask `json:"tasks"`
}

// CronJitterConfig holds scheduler jitter tuning knobs.
// Matches upstream DEFAULT_CRON_JITTER_CONFIG.
type CronJitterConfig struct {
	RecurringFrac     float64 // recurring forward delay as fraction of interval
	RecurringCapMs    int64   // upper bound on recurring delay
	OneShotMaxMs      int64   // one-shot backward lead: maximum ms to fire early
	OneShotFloorMs    int64   // one-shot backward lead: minimum ms to fire early
	OneShotMinuteMod  int64   // jitter fires where minute % N == 0
	RecurringMaxAgeMs int64   // auto-expire recurring tasks this many ms after creation (0 = never)
}

// DefaultCronJitterConfig matches upstream defaults.
var DefaultCronJitterConfig = CronJitterConfig{
	RecurringFrac:     0.1,
	RecurringCapMs:    15 * 60 * 1000,      // 15 minutes
	OneShotMaxMs:      90 * 1000,            // 90 seconds
	OneShotFloorMs:    0,
	OneShotMinuteMod:  30,                   // :00 and :30
	RecurringMaxAgeMs: 7 * 24 * 60 * 60 * 1000, // 7 days
}

// Session-level cron task store (non-durable tasks).
var (
	sessionCronTasks   = make(map[string]CronTask)
	sessionCronTasksMu sync.Mutex
)

func addSessionCronTask(t CronTask) {
	sessionCronTasksMu.Lock()
	defer sessionCronTasksMu.Unlock()
	sessionCronTasks[t.ID] = t
}

func getSessionCronTasks() []CronTask {
	sessionCronTasksMu.Lock()
	defer sessionCronTasksMu.Unlock()
	out := make([]CronTask, 0, len(sessionCronTasks))
	for _, t := range sessionCronTasks {
		out = append(out, t)
	}
	return out
}

func removeSessionCronTasks(ids []string) int {
	sessionCronTasksMu.Lock()
	defer sessionCronTasksMu.Unlock()
	count := 0
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	for _, id := range ids {
		if _, ok := sessionCronTasks[id]; ok {
			delete(sessionCronTasks, id)
			count++
		}
	}
	return count
}

const cronFileRel = ".claude" + string(filepath.Separator) + "scheduled_tasks.json"

// getProjectDir returns the working directory for cron file operations.
var getProjectDir = func() string {
	dir, _ := os.Getwd()
	return dir
}

// getCronFilePath returns the full path to .claude/scheduled_tasks.json.
func getCronFilePath(dir string) string {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return filepath.Join(dir, cronFileRel)
}

// readCronTasks reads and parses .claude/scheduled_tasks.json.
// Returns empty list if file missing/malformed. Invalid cron strings are dropped.
func readCronTasks(dir string) ([]CronTask, error) {
	data, err := os.ReadFile(getCronFilePath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var file CronFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, nil
	}

	var out []CronTask
	for _, t := range file.Tasks {
		if t.ID == "" || t.Cron == "" || t.Prompt == "" || t.CreatedAt == 0 {
			continue
		}
		// Validate cron expression
		if ParseCronExpression(t.Cron) == nil {
			continue
		}
		out = append(out, CronTask{
			ID:          t.ID,
			Cron:        t.Cron,
			Prompt:      t.Prompt,
			CreatedAt:   t.CreatedAt,
			LastFiredAt: t.LastFiredAt,
			Recurring:   t.Recurring,
			Permanent:   t.Permanent,
			Durable:     true,
		})
	}
	return out, nil
}

// writeCronTasks overwrites .claude/scheduled_tasks.json with the given tasks.
// Creates .claude/ if missing. Strips runtime-only fields.
func writeCronTasks(tasks []CronTask, dir string) error {
	// Create .claude/ directory
	cronDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(cronDir, 0755); err != nil {
		return err
	}

	// Strip runtime-only fields
	diskTasks := make([]CronTask, 0, len(tasks))
	for _, t := range tasks {
		diskTasks = append(diskTasks, CronTask{
			ID:          t.ID,
			Cron:        t.Cron,
			Prompt:      t.Prompt,
			CreatedAt:   t.CreatedAt,
			LastFiredAt: t.LastFiredAt,
			Recurring:   t.Recurring,
			Permanent:   t.Permanent,
		})
	}

	file := CronFile{Tasks: diskTasks}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(getCronFilePath(dir), data, 0644)
}

// generateTaskID produces an 8-hex-char ID (matches upstream randomUUID().slice(0, 8)).
func generateTaskID() string {
	b := make([]byte, 4)
	for i := range b {
		b[i] = byte(rand.IntN(256))
	}
	return hex.EncodeToString(b)
}

// addCronTask appends a new task. Returns the generated ID.
func addCronTask(cron, prompt string, recurring, durable bool, agentID, dir string) (string, error) {
	id := generateTaskID()
	task := CronTask{
		ID:        id,
		Cron:      cron,
		Prompt:    prompt,
		CreatedAt: nowMs(),
		Recurring: recurring,
	}

	if !durable {
		task.Durable = false
		if agentID != "" {
			task.AgentID = agentID
		}
		addSessionCronTask(task)
		return id, nil
	}

	tasks, err := readCronTasks(dir)
	if err != nil {
		return "", err
	}
	task.Durable = true
	tasks = append(tasks, task)
	return id, writeCronTasks(tasks, dir)
}

// removeCronTasks removes tasks by ID from both session store and file.
func removeCronTasks(ids []string, dir string) error {
	if len(ids) == 0 {
		return nil
	}

	// Sweep session store first
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	sessionCount := removeSessionCronTasks(ids)
	if sessionCount == len(ids) {
		return nil // all accounted for in session store
	}

	// Remove from file
	tasks, err := readCronTasks(dir)
	if err != nil {
		return err
	}
	remaining := make([]CronTask, 0, len(tasks))
	for _, t := range tasks {
		if !idSet[t.ID] {
			remaining = append(remaining, t)
		}
	}
	if len(remaining) == len(tasks) {
		return nil // nothing changed
	}
	return writeCronTasks(remaining, dir)
}

// listAllCronTasks merges file-backed and session tasks.
// Session tasks get Durable: false.
func listAllCronTasks(dir string) []CronTask {
	fileTasks, _ := readCronTasks(dir)
	sessionTasks := getSessionCronTasks()

	// Mark session tasks as non-durable
	for i := range sessionTasks {
		sessionTasks[i].Durable = false
	}

	return append(fileTasks, sessionTasks...)
}

// markCronTasksFired stamps lastFiredAt on recurring tasks and writes back.
func markCronTasksFired(ids []string, firedAt int64, dir string) error {
	if len(ids) == 0 {
		return nil
	}
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	tasks, err := readCronTasks(dir)
	if err != nil {
		return err
	}
	changed := false
	for i := range tasks {
		if idSet[tasks[i].ID] {
			tasks[i].LastFiredAt = firedAt
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writeCronTasks(tasks, dir)
}

// hasCronTasksSync checks if the cron file has any valid tasks.
func hasCronTasksSync(dir string) bool {
	data, err := os.ReadFile(getCronFilePath(dir))
	if err != nil {
		return false
	}
	var file CronFile
	if err := json.Unmarshal(data, &file); err != nil {
		return false
	}
	return len(file.Tasks) > 0
}

// nowMs returns current epoch in milliseconds.
func nowMs() int64 {
	return time.Now().UnixMilli()
}

// timeUnixMs converts epoch milliseconds to time.Time.
func timeUnixMs(ms int64) time.Time {
	return time.UnixMilli(ms)
}

// nextCronRunMs computes the next fire time in epoch ms strictly after fromMs.
// Returns nil if cron is invalid or no match within 366 days.
func nextCronRunMs(cron string, fromMs int64) *int64 {
	fields := ParseCronExpression(cron)
	if fields == nil {
		return nil
	}
	from := timeUnixMs(fromMs)
	next := ComputeNextCronRun(*fields, from)
	if next == nil {
		return nil
	}
	result := next.UnixMilli()
	return &result
}

// jitterFrac produces a deterministic value in [0, 1) from a task ID.
func jitterFrac(taskID string) float64 {
	// Parse first 8 hex chars as uint32
	if len(taskID) < 8 {
		return 0
	}
	val, err := hex.DecodeString(taskID[:8])
	if err != nil || len(val) < 4 {
		return 0
	}
	n := uint32(val[0])<<24 | uint32(val[1])<<16 | uint32(val[2])<<8 | uint32(val[3])
	return float64(n) / float64(0x100000000)
}

// jitteredNextCronRunMs computes next fire time with forward jitter for recurring tasks.
// Delay is proportional to the interval between fires, capped at recurringCapMs.
func jitteredNextCronRunMs(cron string, fromMs int64, taskID string, cfg CronJitterConfig) *int64 {
	t1 := nextCronRunMs(cron, fromMs)
	if t1 == nil {
		return nil
	}
	t2 := nextCronRunMs(cron, *t1)
	// No second match (pinned date) → no herd risk, fire on t1
	if t2 == nil {
		return t1
	}
	jitter := int64(float64(jitterFrac(taskID)) * cfg.RecurringFrac * float64(*t2-*t1))
	if jitter > cfg.RecurringCapMs {
		jitter = cfg.RecurringCapMs
	}
	result := *t1 + jitter
	return &result
}

// oneShotJitteredNextCronRunMs computes next fire time with backward jitter for one-shot tasks.
// Fires early on minute-boundary hotspots (:00, :30 by default).
func oneShotJitteredNextCronRunMs(cron string, fromMs int64, taskID string, cfg CronJitterConfig) *int64 {
	t1 := nextCronRunMs(cron, fromMs)
	if t1 == nil {
		return nil
	}
	// Check if the computed fire time lands on a hot minute boundary
	fireTime := timeUnixMs(*t1)
	if fireTime.Minute()%int(cfg.OneShotMinuteMod) != 0 {
		return t1
	}
	// Compute backward lead: floor + frac * (max - floor)
	lead := cfg.OneShotFloorMs + int64(jitterFrac(taskID)*float64(cfg.OneShotMaxMs-cfg.OneShotFloorMs))
	result := *t1 - lead
	if result < fromMs {
		result = fromMs
	}
	return &result
}

// findMissedTasks returns tasks whose next scheduled run (from createdAt) is in the past.
func findMissedTasks(tasks []CronTask, nowMs int64) []CronTask {
	var missed []CronTask
	for _, t := range tasks {
		next := nextCronRunMs(t.Cron, t.CreatedAt)
		if next != nil && *next < nowMs {
			missed = append(missed, t)
		}
	}
	return missed
}

// isRecurringTaskAged checks if a recurring task should be auto-expired.
func isRecurringTaskAged(t CronTask, nowMs int64, maxAgeMs int64) bool {
	if maxAgeMs == 0 {
		return false
	}
	return t.Recurring && !t.Permanent && (nowMs-t.CreatedAt) >= maxAgeMs
}
