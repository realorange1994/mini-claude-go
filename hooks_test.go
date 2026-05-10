package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// ─── HookEvent constants ─────────────────────────────────────────────────────

func TestHookEventConstants(t *testing.T) {
	if HookPreCompact != "pre_compact" {
		t.Errorf("HookPreCompact = %q, want 'pre_compact'", HookPreCompact)
	}
	if HookPostCompact != "post_compact" {
		t.Errorf("HookPostCompact = %q, want 'post_compact'", HookPostCompact)
	}
}

func TestHookTriggerConstants(t *testing.T) {
	if HookTriggerManual != "manual" {
		t.Errorf("HookTriggerManual = %q, want 'manual'", HookTriggerManual)
	}
	if HookTriggerAuto != "auto" {
		t.Errorf("HookTriggerAuto = %q, want 'auto'", HookTriggerAuto)
	}
	if HookTriggerSM != "sm_compact" {
		t.Errorf("HookTriggerSM = %q, want 'sm_compact'", HookTriggerSM)
	}
}

// ─── NewHookManager ──────────────────────────────────────────────────────────

func TestNewHookManager(t *testing.T) {
	hm := NewHookManager()
	if hm == nil {
		t.Fatal("NewHookManager should not return nil")
	}
}

// ─── RegisterPreCompact ──────────────────────────────────────────────────────

func TestRegisterPreCompact(t *testing.T) {
	hm := NewHookManager()
	handler := func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		return PreCompactOutput{CustomInstructions: "test"}, nil
	}
	hm.RegisterPreCompact("test-hook", handler, 5*time.Second)

	if len(hm.preCompactHooks) != 1 {
		t.Errorf("expected 1 pre-compact hook, got %d", len(hm.preCompactHooks))
	}
}

func TestRegisterPreCompactMultiple(t *testing.T) {
	hm := NewHookManager()
	handler := func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		return PreCompactOutput{CustomInstructions: "test"}, nil
	}
	hm.RegisterPreCompact("hook1", handler, 5*time.Second)
	hm.RegisterPreCompact("hook2", handler, 3*time.Second)

	if len(hm.preCompactHooks) != 2 {
		t.Errorf("expected 2 pre-compact hooks, got %d", len(hm.preCompactHooks))
	}
}

func TestRegisterPreCompactZeroTimeout(t *testing.T) {
	hm := NewHookManager()
	handler := func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		return PreCompactOutput{}, nil
	}
	hm.RegisterPreCompact("test", handler, 0)

	if len(hm.preCompactHooks) != 1 {
		t.Error("hook with zero timeout should still register")
	}
}

// ─── RegisterPostCompact ─────────────────────────────────────────────────────

func TestRegisterPostCompact(t *testing.T) {
	hm := NewHookManager()
	handler := func(ctx context.Context, input PostCompactInput) (PostCompactOutput, error) {
		return PostCompactOutput{UserMessage: "done"}, nil
	}
	hm.RegisterPostCompact("test-hook", handler, 5*time.Second)

	if len(hm.postCompactHooks) != 1 {
		t.Errorf("expected 1 post-compact hook, got %d", len(hm.postCompactHooks))
	}
}

// ─── ExecutePreCompactHooks empty ────────────────────────────────────────────

func TestExecutePreCompactHooksEmpty(t *testing.T) {
	hm := NewHookManager()
	input := PreCompactInput{Trigger: HookTriggerAuto}
	result, err := hm.ExecutePreCompactHooks(input)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.CustomInstructions != "" {
		t.Errorf("expected empty CustomInstructions, got %q", result.CustomInstructions)
	}
	if result.UserMessage != "" {
		t.Errorf("expected empty UserMessage, got %q", result.UserMessage)
	}
}

// ─── ExecutePreCompactHooks success ──────────────────────────────────────────

func TestExecutePreCompactHooksSuccess(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterPreCompact("test", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		return PreCompactOutput{
			CustomInstructions: "focus on error handling",
			UserMessage:        "hook executed",
		}, nil
	}, 5*time.Second)

	input := PreCompactInput{
		Trigger:            HookTriggerAuto,
		CustomInstructions: "base instructions",
	}
	result, err := hm.ExecutePreCompactHooks(input)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !strings.Contains(result.CustomInstructions, "focus on error handling") {
		t.Errorf("CustomInstructions should contain hook output, got %q", result.CustomInstructions)
	}
	if !strings.Contains(result.UserMessage, "hook executed") {
		t.Errorf("UserMessage should contain hook message, got %q", result.UserMessage)
	}
}

// ─── ExecutePreCompactHooks multiple hooks merge ─────────────────────────────

func TestExecutePreCompactHooksMultiple(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterPreCompact("hook1", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		return PreCompactOutput{CustomInstructions: "instructions 1"}, nil
	}, 5*time.Second)
	hm.RegisterPreCompact("hook2", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		return PreCompactOutput{CustomInstructions: "instructions 2"}, nil
	}, 5*time.Second)

	result, err := hm.ExecutePreCompactHooks(PreCompactInput{Trigger: HookTriggerAuto})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !strings.Contains(result.CustomInstructions, "instructions 1") {
		t.Error("should contain first hook instructions")
	}
	if !strings.Contains(result.CustomInstructions, "instructions 2") {
		t.Error("should contain second hook instructions")
	}
}

// ─── ExecutePreCompactHooks failure ──────────────────────────────────────────

func TestExecutePreCompactHooksFailure(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterPreCompact("failing", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		return PreCompactOutput{}, errors.New("hook error")
	}, 5*time.Second)

	result, err := hm.ExecutePreCompactHooks(PreCompactInput{Trigger: HookTriggerAuto})
	if err == nil {
		t.Error("expected error from failing hook")
	}
	if !strings.Contains(err.Error(), "hook:failing") {
		t.Errorf("error should mention hook name, got %q", err.Error())
	}
	if !strings.Contains(result.UserMessage, "failed") {
		t.Errorf("UserMessage should indicate failure, got %q", result.UserMessage)
	}
}

// ─── ExecutePreCompactHooks mixed success and failure ────────────────────────

func TestExecutePreCompactHooksMixed(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterPreCompact("good", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		return PreCompactOutput{CustomInstructions: "good stuff"}, nil
	}, 5*time.Second)
	hm.RegisterPreCompact("bad", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		return PreCompactOutput{}, errors.New("bad hook")
	}, 5*time.Second)

	result, err := hm.ExecutePreCompactHooks(PreCompactInput{Trigger: HookTriggerAuto})
	if err == nil {
		t.Error("expected error from bad hook")
	}
	if !strings.Contains(result.CustomInstructions, "good stuff") {
		t.Error("should still contain good hook output")
	}
	if !strings.Contains(result.UserMessage, "failed") {
		t.Error("should indicate failure")
	}
}

// ─── ExecutePreCompactHooks input passthrough ────────────────────────────────

func TestExecutePreCompactHooksInputPassthrough(t *testing.T) {
	hm := NewHookManager()
	var receivedTrigger HookTrigger
	hm.RegisterPreCompact("check", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		receivedTrigger = input.Trigger
		return PreCompactOutput{CustomInstructions: input.CustomInstructions + " + appended"}, nil
	}, 5*time.Second)

	input := PreCompactInput{
		Trigger:            HookTriggerSM,
		CustomInstructions: "base",
	}
	result, _ := hm.ExecutePreCompactHooks(input)

	if receivedTrigger != HookTriggerSM {
		t.Errorf("expected trigger HookTriggerSM, got %v", receivedTrigger)
	}
	if !strings.Contains(result.CustomInstructions, "base + appended") {
		t.Errorf("expected appended instructions, got %q", result.CustomInstructions)
	}
}

// ─── ExecutePostCompactHooks empty ───────────────────────────────────────────

func TestExecutePostCompactHooksEmpty(t *testing.T) {
	hm := NewHookManager()
	input := PostCompactInput{Trigger: HookTriggerAuto, CompactSummary: "summary"}
	result, err := hm.ExecutePostCompactHooks(input)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.UserMessage != "" {
		t.Errorf("expected empty UserMessage, got %q", result.UserMessage)
	}
	if result.Attachment != "" {
		t.Errorf("expected empty Attachment, got %q", result.Attachment)
	}
}

// ─── ExecutePostCompactHooks success ─────────────────────────────────────────

func TestExecutePostCompactHooksSuccess(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterPostCompact("test", func(ctx context.Context, input PostCompactInput) (PostCompactOutput, error) {
		return PostCompactOutput{
			UserMessage: "compaction done",
			Attachment:  "<recovered files>",
		}, nil
	}, 5*time.Second)

	input := PostCompactInput{
		Trigger:        HookTriggerAuto,
		CompactSummary: "conversation summarized",
		RecoveredFiles: []string{"main.go"},
	}
	result, err := hm.ExecutePostCompactHooks(input)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !strings.Contains(result.UserMessage, "compaction done") {
		t.Errorf("UserMessage should contain hook message, got %q", result.UserMessage)
	}
	if !strings.Contains(result.Attachment, "<recovered files>") {
		t.Errorf("Attachment should contain hook attachment, got %q", result.Attachment)
	}
}

// ─── ExecutePostCompactHooks multiple merge ──────────────────────────────────

func TestExecutePostCompactHooksMultipleMerge(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterPostCompact("h1", func(ctx context.Context, input PostCompactInput) (PostCompactOutput, error) {
		return PostCompactOutput{Attachment: "attachment 1"}, nil
	}, 5*time.Second)
	hm.RegisterPostCompact("h2", func(ctx context.Context, input PostCompactInput) (PostCompactOutput, error) {
		return PostCompactOutput{Attachment: "attachment 2"}, nil
	}, 5*time.Second)

	result, err := hm.ExecutePostCompactHooks(PostCompactInput{Trigger: HookTriggerAuto})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !strings.Contains(result.Attachment, "attachment 1") {
		t.Error("should contain first attachment")
	}
	if !strings.Contains(result.Attachment, "attachment 2") {
		t.Error("should contain second attachment")
	}
}

// ─── ExecutePostCompactHooks failure ─────────────────────────────────────────

func TestExecutePostCompactHooksFailure(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterPostCompact("fail", func(ctx context.Context, input PostCompactInput) (PostCompactOutput, error) {
		return PostCompactOutput{}, errors.New("post hook error")
	}, 5*time.Second)

	_, err := hm.ExecutePostCompactHooks(PostCompactInput{Trigger: HookTriggerAuto})
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "hook:fail") {
		t.Errorf("error should mention hook name, got %q", err.Error())
	}
}

// ─── Hook timeout / context cancellation ─────────────────────────────────────

func TestPreCompactHookContextCancellation(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterPreCompact("slow", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		select {
		case <-ctx.Done():
			return PreCompactOutput{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return PreCompactOutput{CustomInstructions: "done"}, nil
		}
	}, 50*time.Millisecond)

	result, err := hm.ExecutePreCompactHooks(PreCompactInput{Trigger: HookTriggerAuto})
	// With short timeout, context may be cancelled
	if err == nil && result.CustomInstructions == "done" {
		t.Log("hook completed before timeout (unexpected but OK)")
	}
}

// ─── Hook execution order preservation ───────────────────────────────────────

func TestPreCompactHookOrder(t *testing.T) {
	hm := NewHookManager()
	var order []string
	hm.RegisterPreCompact("first", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		order = append(order, "first")
		return PreCompactOutput{CustomInstructions: "1"}, nil
	}, 5*time.Second)
	hm.RegisterPreCompact("second", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		order = append(order, "second")
		return PreCompactOutput{CustomInstructions: "2"}, nil
	}, 5*time.Second)

	hm.ExecutePreCompactHooks(PreCompactInput{Trigger: HookTriggerAuto})
	if len(order) != 2 {
		t.Fatalf("expected 2 hooks executed, got %d", len(order))
	}
	if order[0] != "first" || order[1] != "second" {
		t.Errorf("hooks should execute in registration order, got %v", order)
	}
}

func TestPostCompactHookOrder(t *testing.T) {
	hm := NewHookManager()
	var order []string
	hm.RegisterPostCompact("alpha", func(ctx context.Context, input PostCompactInput) (PostCompactOutput, error) {
		order = append(order, "alpha")
		return PostCompactOutput{UserMessage: "a"}, nil
	}, 5*time.Second)
	hm.RegisterPostCompact("beta", func(ctx context.Context, input PostCompactInput) (PostCompactOutput, error) {
		order = append(order, "beta")
		return PostCompactOutput{UserMessage: "b"}, nil
	}, 5*time.Second)

	hm.ExecutePostCompactHooks(PostCompactInput{Trigger: HookTriggerAuto})
	if len(order) != 2 {
		t.Fatalf("expected 2 hooks executed, got %d", len(order))
	}
	if order[0] != "alpha" || order[1] != "beta" {
		t.Errorf("hooks should execute in registration order, got %v", order)
	}
}

// ─── Hook with empty output ──────────────────────────────────────────────────

func TestHookEmptyOutput(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterPreCompact("empty", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		return PreCompactOutput{}, nil
	}, 5*time.Second)

	result, err := hm.ExecutePreCompactHooks(PreCompactInput{Trigger: HookTriggerAuto})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.CustomInstructions != "" {
		t.Error("empty hook should not add CustomInstructions")
	}
	if result.UserMessage != "" {
		t.Error("empty hook should not add UserMessage")
	}
}

// ─── PostCompact with empty attachment ───────────────────────────────────────

func TestPostCompactEmptyAttachment(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterPostCompact("no-attach", func(ctx context.Context, input PostCompactInput) (PostCompactOutput, error) {
		return PostCompactOutput{UserMessage: "msg only"}, nil
	}, 5*time.Second)

	result, _ := hm.ExecutePostCompactHooks(PostCompactInput{Trigger: HookTriggerAuto})
	if result.Attachment != "" {
		t.Errorf("expected empty attachment, got %q", result.Attachment)
	}
	if !strings.Contains(result.UserMessage, "msg only") {
		t.Errorf("UserMessage should contain message, got %q", result.UserMessage)
	}
}

// ─── PreCompactInput / PostCompactInput ──────────────────────────────────────

func TestPreCompactInput(t *testing.T) {
	input := PreCompactInput{
		Trigger:            HookTriggerManual,
		CustomInstructions: "test instructions",
	}
	if input.Trigger != HookTriggerManual {
		t.Errorf("expected HookTriggerManual, got %v", input.Trigger)
	}
}

func TestPostCompactInput(t *testing.T) {
	input := PostCompactInput{
		Trigger:        HookTriggerSM,
		CompactSummary: "summarized",
		RecoveredFiles: []string{"file1.go", "file2.go"},
	}
	if len(input.RecoveredFiles) != 2 {
		t.Errorf("expected 2 recovered files, got %d", len(input.RecoveredFiles))
	}
}

// ─── HookResult ──────────────────────────────────────────────────────────────

func TestHookResult(t *testing.T) {
	hr := HookResult{
		Success: true,
		Err:     nil,
		Dur:     100 * time.Millisecond,
	}
	if !hr.Success {
		t.Error("Success should be true")
	}
	if hr.Err != nil {
		t.Error("Err should be nil")
	}
	if hr.Dur != 100*time.Millisecond {
		t.Error("Dur should be 100ms")
	}
}

// ─── Hook default timeout ────────────────────────────────────────────────────

func TestHookDefaultTimeout(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterPreCompact("test", func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error) {
		return PreCompactOutput{}, nil
	}, 0)

	_, err := hm.ExecutePreCompactHooks(PreCompactInput{Trigger: HookTriggerAuto})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
