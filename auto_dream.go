package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Auto-Dream (MiMo-Code P9) ──────────────────────────────────────────────
//
// Periodically spawns background LLM tasks for memory consolidation.
// - Auto Dream: consolidates session knowledge into project memory (7-day interval)
// - Auto Distill: reviews past sessions for repeated workflows (30-day interval)
//
// MiMo-Code source: session/auto-dream.ts (123 lines)

const (
	// DefaultDreamIntervalDays default interval for auto-dream
	DefaultDreamIntervalDays = 7
	// DefaultDistillIntervalDays default interval for auto-distill
	DefaultDistillIntervalDays = 30
	// MinSpawnGapMs minimum gap between spawns
	MinSpawnGapMs = 10 * 1000 // 10 seconds
	// DayMs milliseconds in a day
	DayMs = 24 * 60 * 60 * 1000
)

// AutoDreamConfig holds auto-dream configuration.
type AutoDreamConfig struct {
	Enabled      bool `json:"enabled"`
	IntervalDays int  `json:"interval_days"`
}

// AutoDreamManager manages auto-dream and auto-distill scheduling.
type AutoDreamManager struct {
	mu                 sync.Mutex
	projectDir         string
	dreamConfig        AutoDreamConfig
	distillConfig      AutoDreamConfig
	lastDreamSpawnTime int64
	lastDistillSpawnTime int64
	lastDreamRun       int64
	lastDistillRun     int64
	stopCh             chan struct{}
}

// AutoDreamState persists the last run timestamps.
type AutoDreamState struct {
	LastDreamRun   int64 `json:"last_dream_run"`
	LastDistillRun int64 `json:"last_distill_run"`
}

// NewAutoDreamManager creates a new auto-dream manager.
func NewAutoDreamManager(projectDir string) *AutoDreamManager {
	m := &AutoDreamManager{
		projectDir: projectDir,
		dreamConfig: AutoDreamConfig{
			Enabled:      true,
			IntervalDays: DefaultDreamIntervalDays,
		},
		distillConfig: AutoDreamConfig{
			Enabled:      true,
			IntervalDays: DefaultDistillIntervalDays,
		},
		stopCh: make(chan struct{}),
	}

	// Load persisted state
	m.loadState()

	return m
}

// Start starts the auto-dream scheduler.
func (m *AutoDreamManager) Start() {
	go m.run()
}

// Stop stops the auto-dream scheduler.
func (m *AutoDreamManager) Stop() {
	close(m.stopCh)
}

// run is the background goroutine that checks for auto-dream triggers.
func (m *AutoDreamManager) run() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAndRun()
		}
	}
}

// checkAndRun checks if auto-dream or auto-distill should run.
func (m *AutoDreamManager) checkAndRun() {
	// Check auto-dream
	if m.ShouldAutoDream() {
		m.runDream()
	}

	// Check auto-distill
	if m.ShouldAutoDistill() {
		m.runDistill()
	}
}

// ShouldAutoDream returns true if auto-dream should run.
func (m *AutoDreamManager) ShouldAutoDream() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.dreamConfig.Enabled {
		return false
	}

	// Check spawn gap
	now := time.Now().UnixMilli()
	if now-m.lastDreamSpawnTime < MinSpawnGapMs {
		return false
	}

	// Check interval
	intervalMs := int64(m.dreamConfig.IntervalDays) * int64(DayMs)
	if m.lastDreamRun > 0 && now-m.lastDreamRun < intervalMs {
		return false
	}

	// Check if project is old enough (first run only)
	if m.lastDreamRun == 0 {
		projectAge := m.getProjectAge()
		if projectAge < intervalMs {
			return false
		}
	}

	m.lastDreamSpawnTime = now
	return true
}

// ShouldAutoDistill returns true if auto-distill should run.
func (m *AutoDreamManager) ShouldAutoDistill() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.distillConfig.Enabled {
		return false
	}

	now := time.Now().UnixMilli()
	if now-m.lastDistillSpawnTime < MinSpawnGapMs {
		return false
	}

	intervalMs := int64(m.distillConfig.IntervalDays) * int64(DayMs)
	if m.lastDistillRun > 0 && now-m.lastDistillRun < intervalMs {
		return false
	}

	if m.lastDistillRun == 0 {
		projectAge := m.getProjectAge()
		if projectAge < intervalMs {
			return false
		}
	}

	m.lastDistillSpawnTime = now
	return true
}

// runDream runs the auto-dream task.
func (m *AutoDreamManager) runDream() {
	m.mu.Lock()
	m.lastDreamRun = time.Now().UnixMilli()
	m.mu.Unlock()

	// Persist state
	m.saveState()

	// Dream task: consolidate session knowledge into project memory
	task := `Run one automatic dream memory consolidation pass for the current project.

Use the memory files as the working index and the raw session data as the source of truth.
Consolidate only durable, verified information into project memory.
Focus on: decisions, discoveries, patterns, and lessons learned.`

	// In a real implementation, this would spawn a background LLM subagent
	// For now, we just log the task
	fmt.Printf("[auto-dream] Task: %s\n", task)
}

// runDistill runs the auto-distill task.
func (m *AutoDreamManager) runDistill() {
	m.mu.Lock()
	m.lastDistillRun = time.Now().UnixMilli()
	m.mu.Unlock()

	m.saveState()

	task := `Run one automatic distill pass for the current project.

Review the past month of sessions and identify repeated manual workflows worth packaging.
Inventory existing skills and commands first so you reuse or extend instead of duplicating.
Produce a compact shortlist of high-confidence missing assets.`

	fmt.Printf("[auto-distill] Task: %s\n", task)
}

// getProjectAge returns the project age in milliseconds.
func (m *AutoDreamManager) getProjectAge() int64 {
	// Check .claude directory creation time
	claudeDir := filepath.Join(m.projectDir, ".claude")
	info, err := os.Stat(claudeDir)
	if err != nil {
		return 0
	}
	return time.Now().UnixMilli() - info.ModTime().UnixMilli()
}

// saveState persists the auto-dream state to disk.
func (m *AutoDreamManager) saveState() {
	state := AutoDreamState{
		LastDreamRun:   m.lastDreamRun,
		LastDistillRun: m.lastDistillRun,
	}

	statePath := filepath.Join(m.projectDir, ".claude", "auto_dream_state.json")
	os.MkdirAll(filepath.Dir(statePath), 0755)

	data, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(statePath, data, 0644)
}

// loadState loads the auto-dream state from disk.
func (m *AutoDreamManager) loadState() {
	statePath := filepath.Join(m.projectDir, ".claude", "auto_dream_state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return
	}

	var state AutoDreamState
	if json.Unmarshal(data, &state) == nil {
		m.lastDreamRun = state.LastDreamRun
		m.lastDistillRun = state.LastDistillRun
	}
}

// GetLastDreamRun returns the last dream run time.
func (m *AutoDreamManager) GetLastDreamRun() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return time.UnixMilli(m.lastDreamRun)
}

// GetLastDistillRun returns the last distill run time.
func (m *AutoDreamManager) GetLastDistillRun() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return time.UnixMilli(m.lastDistillRun)
}

// SetDreamInterval sets the dream interval in days.
func (m *AutoDreamManager) SetDreamInterval(days int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dreamConfig.IntervalDays = days
}

// SetDistillInterval sets the distill interval in days.
func (m *AutoDreamManager) SetDistillInterval(days int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.distillConfig.IntervalDays = days
}
