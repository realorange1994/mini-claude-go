package main

import (
	"testing"
	"time"
)

func TestAutoDreamManager_ShouldAutoDream(t *testing.T) {
	dir := t.TempDir()
	m := NewAutoDreamManager(dir)

	// First run — project too young
	if m.ShouldAutoDream() {
		t.Error("expected no auto-dream for young project")
	}
}

func TestAutoDreamManager_ShouldAutoDream_Disabled(t *testing.T) {
	dir := t.TempDir()
	m := NewAutoDreamManager(dir)
	m.dreamConfig.Enabled = false

	if m.ShouldAutoDream() {
		t.Error("expected no auto-dream when disabled")
	}
}

func TestAutoDreamManager_ShouldAutoDream_IntervalRespected(t *testing.T) {
	dir := t.TempDir()
	m := NewAutoDreamManager(dir)

	// Set last dream run to now
	m.mu.Lock()
	m.lastDreamRun = time.Now().UnixMilli()
	m.mu.Unlock()

	if m.ShouldAutoDream() {
		t.Error("expected no auto-dream when interval not elapsed")
	}
}

func TestAutoDreamManager_ShouldAutoDistill(t *testing.T) {
	dir := t.TempDir()
	m := NewAutoDreamManager(dir)

	// First run — project too young
	if m.ShouldAutoDistill() {
		t.Error("expected no auto-distill for young project")
	}
}

func TestAutoDreamManager_SetDreamInterval(t *testing.T) {
	dir := t.TempDir()
	m := NewAutoDreamManager(dir)

	m.SetDreamInterval(14)
	if m.dreamConfig.IntervalDays != 14 {
		t.Errorf("expected 14 days, got %d", m.dreamConfig.IntervalDays)
	}
}

func TestAutoDreamManager_SetDistillInterval(t *testing.T) {
	dir := t.TempDir()
	m := NewAutoDreamManager(dir)

	m.SetDistillInterval(60)
	if m.distillConfig.IntervalDays != 60 {
		t.Errorf("expected 60 days, got %d", m.distillConfig.IntervalDays)
	}
}

func TestAutoDreamManager_GetLastDreamRun(t *testing.T) {
	dir := t.TempDir()
	m := NewAutoDreamManager(dir)

	lastRun := m.GetLastDreamRun()
	// Unix epoch (0 milliseconds) is expected for never-run
	if lastRun.Unix() != 0 {
		t.Errorf("expected Unix epoch for never-run, got %v", lastRun)
	}
}

func TestAutoDreamManager_GetLastDistillRun(t *testing.T) {
	dir := t.TempDir()
	m := NewAutoDreamManager(dir)

	lastRun := m.GetLastDistillRun()
	// Unix epoch (0 milliseconds) is expected for never-run
	if lastRun.Unix() != 0 {
		t.Errorf("expected Unix epoch for never-run, got %v", lastRun)
	}
}

func TestAutoDreamManager_SaveLoadState(t *testing.T) {
	dir := t.TempDir()
	m := NewAutoDreamManager(dir)

	// Set some state
	m.mu.Lock()
	m.lastDreamRun = 1234567890
	m.lastDistillRun = 9876543210
	m.mu.Unlock()

	// Save
	m.saveState()

	// Create new manager and load state
	m2 := NewAutoDreamManager(dir)

	if m2.lastDreamRun != 1234567890 {
		t.Errorf("expected lastDreamRun=1234567890, got %d", m2.lastDreamRun)
	}
	if m2.lastDistillRun != 9876543210 {
		t.Errorf("expected lastDistillRun=9876543210, got %d", m2.lastDistillRun)
	}
}

func TestAutoDreamManager_SpawnGap(t *testing.T) {
	dir := t.TempDir()
	m := NewAutoDreamManager(dir)

	// First call should update spawn time
	m.mu.Lock()
	m.lastDreamRun = 0
	m.lastDreamSpawnTime = time.Now().UnixMilli()
	m.mu.Unlock()

	// Second call within gap should return false
	if m.ShouldAutoDream() {
		t.Error("expected no auto-dream within spawn gap")
	}
}
