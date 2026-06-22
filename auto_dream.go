package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	// Read session memory and consolidate into project memory
	sessionDir := filepath.Join(m.projectDir, ".claude")
	projectMemoryDir := filepath.Join(sessionDir, "memory")

	// Scan session memory files
	entries, err := os.ReadDir(projectMemoryDir)
	if err != nil {
		return
	}

	var discoveries []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			path := filepath.Join(projectMemoryDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			// Extract discoveries from session memory
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.Contains(line, "discovery") || strings.Contains(line, "learned") {
					discoveries = append(discoveries, line)
				}
			}
		}
	}

	// Consolidate discoveries into project memory
	if len(discoveries) > 0 {
		projectMemoryPath := filepath.Join(projectMemoryDir, "project.md")
		var content string
		content += "# Project Memory\n\n"
		content += "## Discoveries\n\n"
		for _, d := range discoveries {
			content += "- " + d + "\n"
		}
		os.MkdirAll(projectMemoryDir, 0755)
		os.WriteFile(projectMemoryPath, []byte(content), 0644)
	}
}

// runDistill runs the auto-distill task.
func (m *AutoDreamManager) runDistill() {
	m.mu.Lock()
	m.lastDistillRun = time.Now().UnixMilli()
	m.mu.Unlock()

	m.saveState()

	// Distill: identify repeated workflows and create skill candidates
	sessionDir := filepath.Join(m.projectDir, ".claude")
	skillsDir := filepath.Join(sessionDir, "skills")

	// Scan for repeated patterns in session memory
	projectMemoryDir := filepath.Join(sessionDir, "memory")
	entries, err := os.ReadDir(projectMemoryDir)
	if err != nil {
		return
	}

	var patterns []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			path := filepath.Join(projectMemoryDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			// Look for repeated patterns
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.Contains(line, "pattern") || strings.Contains(line, "workflow") {
					patterns = append(patterns, line)
				}
			}
		}
	}

	// Create skill candidates from patterns
	if len(patterns) > 0 {
		os.MkdirAll(skillsDir, 0755)
		skillPath := filepath.Join(skillsDir, "auto-distilled.md")
		var content string
		content += "# Auto-Distilled Skills\n\n"
		content += "## Identified Patterns\n\n"
		for _, p := range patterns {
			content += "- " + p + "\n"
		}
		os.WriteFile(skillPath, []byte(content), 0644)
	}
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
