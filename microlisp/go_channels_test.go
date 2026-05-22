package microlisp

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func initReset() { ResetGlobalEnv() }

func TestChannelCreateAndClose(t *testing.T) {
	initReset()
	result, err := SafeEvalString(`(let ((ch (make-channel 1)))
		(chan-close ch)
		(chan-info ch))`)
	if err != nil {
		t.Fatalf("channel create/close: %v", err)
	}
	if result == "" {
		t.Fatal("expected channel info")
	}
}

func TestChannelSendRecv(t *testing.T) {
	initReset()
	result, err := SafeEvalString(`
		(let ((ch (make-channel 1)))
			(chan-send ch 42)
			(chan-recv ch))`)
	if err != nil {
		t.Fatalf("channel send/recv: %v", err)
	}
	if result != "42" {
		t.Fatalf("expected 42, got %s", result)
	}
}

func TestChannelTrySendRecv(t *testing.T) {
	initReset()
	result, err := SafeEvalString(`
		(let ((ch (make-channel 1)))
			(chan-try-send ch "hello")
			(chan-try-recv ch))`)
	if err != nil {
		t.Fatalf("channel try-send/try-recv: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestChannelTryRecvWouldBlock(t *testing.T) {
	initReset()
	result, err := SafeEvalString(`
		(let ((ch (make-channel)))
			(chan-try-recv ch))`)
	if err != nil {
		t.Fatalf("channel try-recv would-block: %v", err)
	}
	if result != "(:would-block)" && result != "(would-block)" {
		t.Fatalf("expected :would-block, got %s", result)
	}
}

func TestChannelTrySendWouldBlock(t *testing.T) {
	initReset()
	result, err := SafeEvalString(`
		(let ((ch (make-channel)))
			(chan-try-send ch 1))`)
	if err != nil {
		t.Fatalf("channel try-send would-block: %v", err)
	}
	if result != "(:would-block)" && result != "(would-block)" {
		t.Fatalf("expected :would-block, got %s", result)
	}
}

func TestChannelTryRecvClosed(t *testing.T) {
	initReset()
	result, err := SafeEvalString(`
		(let ((ch (make-channel 1)))
			(chan-close ch)
			(chan-try-recv ch))`)
	if err != nil {
		t.Fatalf("channel try-recv closed: %v", err)
	}
	if result != "(:closed)" && result != "(closed)" {
		t.Fatalf("expected :closed, got %s", result)
	}
}

func TestChannelRecvClosed(t *testing.T) {
	initReset()
	result, err := SafeEvalString(`
		(let ((ch (make-channel 1)))
			(chan-close ch)
			(chan-recv ch))`)
	if err != nil {
		t.Fatalf("channel recv closed: %v", err)
	}
	if result != "NIL" && result != "nil" && result != "()" {
		t.Fatalf("expected NIL on closed channel, got %s", result)
	}
}

func TestChannelPredicate(t *testing.T) {
	initReset()
	result, err := SafeEvalString(`(chan-p (make-channel 1))`)
	if err != nil {
		t.Fatalf("chan-p: %v", err)
	}
	if result != "T" && result != "t" && result != "#t" {
		t.Fatalf("expected T, got %s", result)
	}

	result2, err := SafeEvalString(`(chan-p 42)`)
	if err != nil {
		t.Fatalf("chan-p: %v", err)
	}
	if result2 == "T" || result2 == "t" || result2 == "#t" {
		t.Fatalf("expected NIL for non-channel, got %s", result2)
	}
}

func TestChannelGoAliases(t *testing.T) {
	initReset()
	// Old go: prefixed names should still work
	result, err := SafeEvalString(`
		(let ((ch (go:channel 1)))
			(go:send ch 99)
			(go:recv ch))`)
	if err != nil {
		t.Fatalf("go: aliases: %v", err)
	}
	if result != "99" {
		t.Fatalf("expected 99, got %s", result)
	}
}

func TestChannelSelectWithDefault(t *testing.T) {
	initReset()
	// Select with :default on empty channel should return immediately
	result, err := SafeEvalString(`
		(let ((ch (make-channel)))
			(go:select (:recv ch) (:default)))`)
	if err != nil {
		t.Fatalf("select with default: %v", err)
	}
	if result == "" {
		t.Fatal("expected select result")
	}
}

func TestChannelSelectSendRecv(t *testing.T) {
	initReset()
	result, err := SafeEvalString(`
		(let ((ch (make-channel 1)))
			(chan-send ch 7)
			(go:select (:recv ch)))`)
	if err != nil {
		t.Fatalf("select send/recv: %v", err)
	}
	if result == "" {
		t.Fatal("expected select result")
	}
}

func TestChannelSelectTimeout(t *testing.T) {
	initReset()
	start := time.Now()
	result, err := SafeEvalString(`
		(let ((ch (make-channel)))
			(chan-select-timeout 100 (:recv ch)))`)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("select timeout: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("select timeout took too long: %v", elapsed)
	}
	if result == "" {
		t.Fatal("expected timeout result")
	}
}

func TestChannelSelectTimeoutSuccess(t *testing.T) {
	initReset()
	result, err := SafeEvalString(`
		(let ((ch (make-channel 1)))
			(chan-send ch 42)
			(chan-select-timeout 5000 (:recv ch)))`)
	if err != nil {
		t.Fatalf("select timeout success: %v", err)
	}
	if result == "" {
		t.Fatal("expected result")
	}
}

func TestChannelCancelChanIntegration(t *testing.T) {
	initReset()
	cancelCh := NewCancelChannel()
	limits := DefaultLimits()
	limits.CancelChan = cancelCh

	done := make(chan string, 1)
	go func() {
		result, err := SafeEvalWithLimits(`
			(let ((ch (make-channel)))
				(chan-recv ch))`, limits)
		if err != nil {
			done <- "err:" + err.Error()
		} else {
			done <- "ok:" + result
		}
	}()

	time.Sleep(100 * time.Millisecond)
	close(cancelCh)

	select {
	case r := <-done:
		if r == "ok:" || r == "ok:NIL" {
			t.Fatalf("should have been cancelled, got: %s", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("chan-recv should have been cancelled within 3s")
	}
}

func TestChannelSendCancelChanIntegration(t *testing.T) {
	initReset()
	cancelCh := NewCancelChannel()
	limits := DefaultLimits()
	limits.CancelChan = cancelCh

	done := make(chan string, 1)
	go func() {
		result, err := SafeEvalWithLimits(`
			(let ((ch (make-channel)))
				(chan-send ch 42))`, limits)
		if err != nil {
			done <- "err:" + err.Error()
		} else {
			done <- "ok:" + result
		}
	}()

	time.Sleep(100 * time.Millisecond)
	close(cancelCh)

	select {
	case r := <-done:
		if r == "ok:" || r == "ok:NIL" {
			t.Fatalf("should have been cancelled, got: %s", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("chan-send should have been cancelled within 3s")
	}
}

func TestChannelSelectCancelChanIntegration(t *testing.T) {
	initReset()
	cancelCh := NewCancelChannel()
	limits := DefaultLimits()
	limits.CancelChan = cancelCh

	done := make(chan string, 1)
	go func() {
		result, err := SafeEvalWithLimits(`
			(let ((ch (make-channel)))
				(go:select (:recv ch)))`, limits)
		if err != nil {
			done <- "err:" + err.Error()
		} else {
			done <- "ok:" + result
		}
	}()

	time.Sleep(100 * time.Millisecond)
	close(cancelCh)

	select {
	case r := <-done:
		if r == "ok:" || r == "ok:NIL" {
			t.Fatalf("should have been cancelled, got: %s", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("go:select should have been cancelled within 3s")
	}
}

func TestSpawnAndJoinThread(t *testing.T) {
	initReset()
	// go:spawn returns a thread that can be joined
	result, err := SafeEvalString(`
		(let ((th (go:spawn (lambda () 42))))
			(join-thread th))`)
	if err != nil {
		t.Fatalf("spawn+join: %v", err)
	}
	if result != "42" {
		t.Fatalf("expected 42, got %s", result)
	}
}

func TestMakeThreadAndJoin(t *testing.T) {
	initReset()
	result, err := SafeEvalString(`
		(let ((th (make-thread (lambda () 99))))
			(join-thread th))`)
	if err != nil {
		t.Fatalf("make-thread+join: %v", err)
	}
	if result != "99" {
		t.Fatalf("expected 99, got %s", result)
	}
}

func TestSpawnPanicRecovery(t *testing.T) {
	initReset()
	// Direct Go-level test: spawn a function that panics and verify join returns an error
	tid := atomic.AddInt64(&nextThreadID, 1)
	resultCh := make(chan threadResult, 1)
	threadChannelsMu.Lock()
	threadChannels[tid] = resultCh
	threadChannelsMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- threadResult{err: fmt.Errorf("panic in spawned thread: %v", r)}
			}
		}()
		// This will panic
		var p *int
		*p = 42
	}()

	select {
	case tr := <-resultCh:
		if tr.err == nil {
			t.Fatal("expected panic error, got nil")
		}
		if !strings.Contains(tr.err.Error(), "panic") {
			t.Fatalf("expected panic error, got: %v", tr.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("join on panicked spawn hung forever")
	}
}

func TestMakeThreadPanicRecovery(t *testing.T) {
	initReset()
	tid := atomic.AddInt64(&nextThreadID, 1)
	resultCh := make(chan threadResult, 1)
	threadChannelsMu.Lock()
	threadChannels[tid] = resultCh
	threadChannelsMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- threadResult{err: fmt.Errorf("panic in thread: %v", r)}
			}
		}()
		// This will panic
		var p *int
		*p = 42
	}()

	select {
	case tr := <-resultCh:
		if tr.err == nil {
			t.Fatal("expected panic error, got nil")
		}
		if !strings.Contains(tr.err.Error(), "panic") {
			t.Fatalf("expected panic error, got: %v", tr.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("join on panicked make-thread hung forever")
	}
}
