package main

import (
	"testing"
	"time"
)

// ─── Bounded Queue Tests ────────────────────────────────────────────────────

func TestBoundedQueue_PushPop(t *testing.T) {
	q := NewBoundedQueue(10)
	q.Push("a")
	q.Push("b")

	item, ok := q.TryPop()
	if !ok || item != "a" {
		t.Errorf("expected 'a', got %v", item)
	}
}

func TestBoundedQueue_DropOldest(t *testing.T) {
	q := NewBoundedQueue(2)
	q.Push("a")
	q.Push("b")
	q.Push("c") // should drop "a"

	item, _ := q.TryPop()
	if item != "b" {
		t.Errorf("expected 'b', got %v", item)
	}
	if q.Dropped() != 1 {
		t.Errorf("expected 1 dropped, got %d", q.Dropped())
	}
}

func TestBoundedQueue_TryPopEmpty(t *testing.T) {
	q := NewBoundedQueue(10)
	_, ok := q.TryPop()
	if ok {
		t.Error("expected false for empty queue")
	}
}

func TestBoundedQueue_Len(t *testing.T) {
	q := NewBoundedQueue(10)
	q.Push("a")
	q.Push("b")

	if q.Len() != 2 {
		t.Errorf("expected 2, got %d", q.Len())
	}
}

// ─── RWLock Tests ───────────────────────────────────────────────────────────

func TestRWLockManager_ReadLock(t *testing.T) {
	m := NewRWLockManager()

	m.AcquireRead("key")
	m.ReleaseRead("key") // Should not panic
}

func TestRWLockManager_WriteLock(t *testing.T) {
	m := NewRWLockManager()

	m.AcquireWrite("key")
	m.ReleaseWrite("key") // Should not panic
}

func TestRWLockManager_ConcurrentRead(t *testing.T) {
	m := NewRWLockManager()

	done := make(chan bool, 2)
	go func() {
		m.AcquireRead("key")
		time.Sleep(50 * time.Millisecond)
		m.ReleaseRead("key")
		done <- true
	}()
	go func() {
		m.AcquireRead("key")
		time.Sleep(50 * time.Millisecond)
		m.ReleaseRead("key")
		done <- true
	}()

	<-done
	<-done
}

// ─── Bash Arity Tests ───────────────────────────────────────────────────────

func TestBashArityClassifier_Safe(t *testing.T) {
	c := NewBashArityClassifier()
	if c.Classify("ls -la") != 0 {
		t.Error("expected safe")
	}
}

func TestBashArityClassifier_Dangerous(t *testing.T) {
	c := NewBashArityClassifier()
	if c.Classify("rm -rf /") != 2 {
		t.Error("expected dangerous")
	}
}

func TestBashArityClassifier_Moderate(t *testing.T) {
	c := NewBashArityClassifier()
	if c.Classify("git status") != 1 {
		t.Error("expected moderate")
	}
}

func TestFormatArity(t *testing.T) {
	if FormatArity(0) != "safe" {
		t.Error("expected 'safe'")
	}
	if FormatArity(2) != "dangerous" {
		t.Error("expected 'dangerous'")
	}
}

// ─── Metrics Collector Tests ────────────────────────────────────────────────

func TestMetricsCollector_New(t *testing.T) {
	c := NewMetricsCollector(MetricsConfig{Enabled: true}, "s1")
	if c == nil {
		t.Error("expected non-nil collector")
	}
}

func TestMetricsCollector_RecordModelCall(t *testing.T) {
	c := NewMetricsCollector(MetricsConfig{Enabled: true}, "s1")
	c.RecordModelCall(map[string]any{"model": "claude"})

	if c.Count() != 1 {
		t.Errorf("expected 1 event, got %d", c.Count())
	}
}

func TestMetricsCollector_Disabled(t *testing.T) {
	c := NewMetricsCollector(MetricsConfig{Enabled: false}, "s1")
	c.RecordModelCall(map[string]any{})

	if c.Count() != 0 {
		t.Error("expected 0 events when disabled")
	}
}

func TestMetricsCollector_Flush(t *testing.T) {
	c := NewMetricsCollector(MetricsConfig{Enabled: true}, "s1")
	c.RecordModelCall(map[string]any{})

	err := c.Flush() // No endpoint, should not error
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ─── Session Run State Tests ────────────────────────────────────────────────

func TestSessionRunManager_New(t *testing.T) {
	m := NewSessionRunManager()
	if m == nil {
		t.Error("expected non-nil manager")
	}
}

func TestSessionRunManager_GetState(t *testing.T) {
	m := NewSessionRunManager()
	if m.GetState("s1") != RunStateIdle {
		t.Error("expected idle initially")
	}
}

func TestSessionRunManager_TransitionTo(t *testing.T) {
	m := NewSessionRunManager()

	err := m.TransitionTo("s1", RunStateRunning)
	if err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	if m.GetState("s1") != RunStateRunning {
		t.Errorf("expected running, got %s", m.GetState("s1"))
	}
}

func TestSessionRunManager_InvalidTransition(t *testing.T) {
	m := NewSessionRunManager()

	// From idle, can only go to running or shell
	err := m.TransitionTo("s1", RunStateShellThenRun)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
}

func TestSessionRunManager_EnsureRunning(t *testing.T) {
	m := NewSessionRunManager()

	err := m.EnsureRunning("s1")
	if err != nil {
		t.Fatalf("ensure running failed: %v", err)
	}

	if m.GetState("s1") != RunStateRunning {
		t.Errorf("expected running, got %s", m.GetState("s1"))
	}
}

func TestSessionRunManager_Cancel(t *testing.T) {
	m := NewSessionRunManager()

	m.TransitionTo("s1", RunStateRunning)
	m.Cancel("s1")

	if m.GetState("s1") != RunStateIdle {
		t.Errorf("expected idle after cancel, got %s", m.GetState("s1"))
	}
}

func TestFormatRunState(t *testing.T) {
	if FormatRunState(RunStateIdle) != "Idle" {
		t.Error("expected 'Idle'")
	}
	if FormatRunState(RunStateRunning) != "Running" {
		t.Error("expected 'Running'")
	}
}
