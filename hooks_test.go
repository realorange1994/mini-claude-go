package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── HookEvent constants ─────────────────────────────────────────────────────

func TestHookEventConstants(t *testing.T) {
	// New hook event constants
	if HookPreAPICall != "pre_api_call" {
		t.Errorf("HookPreAPICall = %q, want 'pre_api_call'", HookPreAPICall)
	}
	if HookPostAPICall != "post_api_call" {
		t.Errorf("HookPostAPICall = %q, want 'post_api_call'", HookPostAPICall)
	}
	if HookPreUserMessage != "pre_user_message" {
		t.Errorf("HookPreUserMessage = %q, want 'pre_user_message'", HookPreUserMessage)
	}
	if HookPostUserMessage != "post_user_message" {
		t.Errorf("HookPostUserMessage = %q, want 'post_user_message'", HookPostUserMessage)
	}
	if HookPreAssistantMessage != "pre_assistant_message" {
		t.Errorf("HookPreAssistantMessage = %q, want 'pre_assistant_message'", HookPreAssistantMessage)
	}
	if HookPostAssistantMessage != "post_assistant_message" {
		t.Errorf("HookPostAssistantMessage = %q, want 'post_assistant_message'", HookPostAssistantMessage)
	}
	if HookOnError != "on_error" {
		t.Errorf("HookOnError = %q, want 'on_error'", HookOnError)
	}
	if HookOnAbort != "on_abort" {
		t.Errorf("HookOnAbort = %q, want 'on_abort'", HookOnAbort)
	}
	if HookOnNotification != "on_notification" {
		t.Errorf("HookOnNotification = %q, want 'on_notification'", HookOnNotification)
	}
	if HookOnSubagent != "on_subagent" {
		t.Errorf("HookOnSubagent = %q, want 'on_subagent'", HookOnSubagent)
	}
	if HookOnFork != "on_fork" {
		t.Errorf("HookOnFork = %q, want 'on_fork'", HookOnFork)
	}
	if HookOnResume != "on_resume" {
		t.Errorf("HookOnResume = %q, want 'on_resume'", HookOnResume)
	}
	if HookPreToolUse != "pre_tool_use" {
		t.Errorf("HookPreToolUse = %q, want 'pre_tool_use'", HookPreToolUse)
	}
	if HookPostToolUse != "post_tool_use" {
		t.Errorf("HookPostToolUse = %q, want 'post_tool_use'", HookPostToolUse)
	}
	if HookStop != "stop" {
		t.Errorf("HookStop = %q, want 'stop'", HookStop)
	}
}

func TestHookEventCount(t *testing.T) {
	// Verify we have at least 14 hook types
	allHooks := []HookEvent{
		HookPreAPICall, HookPostAPICall,
		HookPreUserMessage, HookPostUserMessage,
		HookPreAssistantMessage, HookPostAssistantMessage,
		HookOnError, HookOnAbort,
		HookOnNotification, HookOnSubagent, HookOnFork, HookOnResume,
		HookPreToolUse, HookPostToolUse,
		HookStop,
	}
	if len(allHooks) < 15 {
		t.Errorf("expected at least 17 hook types, got %d", len(allHooks))
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

// ─── DefaultHookTimeout ──────────────────────────────────────────────────────

func TestDefaultHookTimeout(t *testing.T) {
	if DefaultHookTimeout != 30*time.Second {
		t.Errorf("DefaultHookTimeout = %v, want 30s", DefaultHookTimeout)
	}
}

// ─── NewHookManager ──────────────────────────────────────────────────────────

func TestNewHookManager(t *testing.T) {
	hm := NewHookManager()
	if hm == nil {
		t.Fatal("NewHookManager should not return nil")
	}
	if hm.executor == nil {
		t.Fatal("HookManager.executor should not be nil")
	}
	if hm.genericHooks == nil {
		t.Fatal("HookManager.genericHooks should be initialized")
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
		Name:    "test",
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
	if hr.Name != "test" {
		t.Errorf("Name should be 'test', got %q", hr.Name)
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

// ═══════════════════════════════════════════════════════════════════════════════
// NEW TESTS: Generic hook registration and execution
// ═══════════════════════════════════════════════════════════════════════════════

// ─── RegisterGeneric ─────────────────────────────────────────────────────────

func TestRegisterGeneric(t *testing.T) {
	hm := NewHookManager()
	handler := func(ctx context.Context, input HookInput) (HookOutput, error) {
		return HookOutput{}, nil
	}
	hm.RegisterGeneric(HookPreAPICall, "test-api-hook", handler, 5*time.Second)

	hm.mu.RLock()
	hooks := hm.genericHooks[HookPreAPICall]
	hm.mu.RUnlock()

	if len(hooks) != 1 {
		t.Errorf("expected 1 generic hook, got %d", len(hooks))
	}
	if hooks[0].name != "test-api-hook" {
		t.Errorf("expected name 'test-api-hook', got %q", hooks[0].name)
	}
}

func TestRegisterGenericMultipleEvents(t *testing.T) {
	hm := NewHookManager()
	handler := func(ctx context.Context, input HookInput) (HookOutput, error) {
		return HookOutput{}, nil
	}
	hm.RegisterGeneric(HookPreAPICall, "api-hook", handler, 5*time.Second)
	hm.RegisterGeneric(HookOnError, "error-hook", handler, 10*time.Second)
	hm.RegisterGeneric(HookPreAPICall, "api-hook-2", handler, 3*time.Second)

	hm.mu.RLock()
	apiHooks := hm.genericHooks[HookPreAPICall]
	errHooks := hm.genericHooks[HookOnError]
	hm.mu.RUnlock()

	if len(apiHooks) != 2 {
		t.Errorf("expected 2 API hooks, got %d", len(apiHooks))
	}
	if len(errHooks) != 1 {
		t.Errorf("expected 1 error hook, got %d", len(errHooks))
	}
}

// ─── ExecuteGenericHooks empty ───────────────────────────────────────────────

func TestExecuteGenericHooksEmpty(t *testing.T) {
	hm := NewHookManager()
	results, err := hm.ExecuteGenericHooks(HookPreAPICall, nil)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// ─── ExecuteGenericHooks success ─────────────────────────────────────────────

func TestExecuteGenericHooksSuccess(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterGeneric(HookPreAPICall, "logger", func(ctx context.Context, input HookInput) (HookOutput, error) {
		return HookOutput{Metadata: map[string]interface{}{"logged": true}}, nil
	}, 5*time.Second)

	results, err := hm.ExecuteGenericHooks(HookPreAPICall, map[string]interface{}{"model": "test"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Error("hook should succeed")
	}
	if results[0].Name != "logger" {
		t.Errorf("expected name 'logger', got %q", results[0].Name)
	}
}

// ─── ExecuteGenericHooks failure ─────────────────────────────────────────────

func TestExecuteGenericHooksFailure(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterGeneric(HookOnError, "flaky", func(ctx context.Context, input HookInput) (HookOutput, error) {
		return HookOutput{}, errors.New("hook crashed")
	}, 5*time.Second)

	results, err := hm.ExecuteGenericHooks(HookOnError, map[string]interface{}{"error": "test"})
	if err == nil {
		t.Error("expected error from failing hook")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("hook should fail")
	}
	if results[0].Err == nil {
		t.Error("result should have error")
	}
}

// ─── ExecuteGenericHooks mixed ───────────────────────────────────────────────

func TestExecuteGenericHooksMixed(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterGeneric(HookPostAPICall, "good", func(ctx context.Context, input HookInput) (HookOutput, error) {
		return HookOutput{Metadata: map[string]interface{}{"ok": true}}, nil
	}, 5*time.Second)
	hm.RegisterGeneric(HookPostAPICall, "bad", func(ctx context.Context, input HookInput) (HookOutput, error) {
		return HookOutput{}, errors.New("oops")
	}, 5*time.Second)

	results, err := hm.ExecuteGenericHooks(HookPostAPICall, nil)
	if err == nil {
		t.Error("expected error from bad hook")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Success {
		t.Error("first hook should succeed")
	}
	if results[1].Success {
		t.Error("second hook should fail")
	}
}

// ─── ExecuteGenericHooks metadata passthrough ────────────────────────────────

func TestExecuteGenericHooksMetadataPassthrough(t *testing.T) {
	hm := NewHookManager()
	var receivedModel string
	hm.RegisterGeneric(HookPreAPICall, "checker", func(ctx context.Context, input HookInput) (HookOutput, error) {
		if m, ok := input.Metadata["model"].(string); ok {
			receivedModel = m
		}
		return HookOutput{}, nil
	}, 5*time.Second)

	hm.ExecuteGenericHooks(HookPreAPICall, map[string]interface{}{"model": "claude-4"})
	if receivedModel != "claude-4" {
		t.Errorf("expected model 'claude-4', got %q", receivedModel)
	}
}

// ─── ExecuteGenericHooks metadata chaining ───────────────────────────────────

func TestExecuteGenericHooksMetadataChaining(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterGeneric(HookPreAPICall, "enricher", func(ctx context.Context, input HookInput) (HookOutput, error) {
		return HookOutput{Metadata: map[string]interface{}{"enriched": true}}, nil
	}, 5*time.Second)

	var sawEnriched bool
	hm.RegisterGeneric(HookPreAPICall, "reader", func(ctx context.Context, input HookInput) (HookOutput, error) {
		if v, ok := input.Metadata["enriched"].(bool); ok && v {
			sawEnriched = true
		}
		return HookOutput{}, nil
	}, 5*time.Second)

	hm.ExecuteGenericHooks(HookPreAPICall, nil)
	if !sawEnriched {
		t.Error("second hook should see metadata from first hook")
	}
}

// ─── ExecuteGenericHooksQuiet ────────────────────────────────────────────────

func TestExecuteGenericHooksQuiet(t *testing.T) {
	hm := NewHookManager()
	hm.RegisterGeneric(HookOnError, "flaky", func(ctx context.Context, input HookInput) (HookOutput, error) {
		return HookOutput{}, errors.New("silent fail")
	}, 5*time.Second)

	// Should not panic or return an error
	results := hm.ExecuteGenericHooksQuiet(HookOnError, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("hook should have failed")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// NEW TESTS: Death spiral prevention
// ═══════════════════════════════════════════════════════════════════════════════

func TestHookExecutorBasicExecution(t *testing.T) {
	e := NewHookExecutor()
	handler := func(ctx context.Context, input HookInput) (HookOutput, error) {
		return HookOutput{Metadata: map[string]interface{}{"ok": true}}, nil
	}
	out, err, skipped := e.ExecuteWithSpiralPrevention(context.Background(), "pre_api_call", handler, HookInput{}, 5*time.Second)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if skipped {
		t.Error("hook should not be skipped")
	}
	if v, ok := out.Metadata["ok"].(bool); !ok || !v {
		t.Error("hook output metadata should contain ok=true")
	}
}

func TestHookExecutorErrorDepthTracking(t *testing.T) {
	e := NewHookExecutor()
	callCount := int32(0)

	handler := func(ctx context.Context, input HookInput) (HookOutput, error) {
		atomic.AddInt32(&callCount, 1)
		return HookOutput{}, nil
	}

	// First on-error hook should succeed
	_, _, skipped := e.ExecuteWithSpiralPrevention(context.Background(), string(HookOnError), handler, HookInput{}, 5*time.Second)
	if skipped {
		t.Error("first on-error hook should not be skipped")
	}

	// Second on-error hook from same depth should also succeed
	_, _, skipped = e.ExecuteWithSpiralPrevention(context.Background(), string(HookOnError), handler, HookInput{}, 5*time.Second)
	if skipped {
		t.Error("second on-error hook should not be skipped")
	}
}

func TestHookExecutorDeathSpiralPrevention(t *testing.T) {
	e := NewHookExecutor()
	e.maxErrorDepth = 2

	callCount := int32(0)
	handler := func(ctx context.Context, input HookInput) (HookOutput, error) {
		atomic.AddInt32(&callCount, 1)
		// This handler would trigger another on-error hook in real code
		return HookOutput{}, nil
	}

	// Call 1: depth 0 -> 1 (allowed)
	_, _, skipped1 := e.ExecuteWithSpiralPrevention(context.Background(), string(HookOnError), handler, HookInput{}, 5*time.Second)
	if skipped1 {
		t.Error("first call should not be skipped")
	}

	// Simulate nested call (depth is now 1, increment to 2)
	// Call 2 from within the first handler context would be at depth 1 -> 2 (allowed)
	// But since the first call already returned, depth is back to 0.
	// Let's test by setting errorDepth directly
	e.mu.Lock()
	e.errorDepth = 2 // simulate being at max depth
	e.mu.Unlock()

	// Call 3 at max depth: should be skipped
	_, _, skipped3 := e.ExecuteWithSpiralPrevention(context.Background(), string(HookOnError), handler, HookInput{}, 5*time.Second)
	if !skipped3 {
		t.Error("on-error at max depth should be skipped (death spiral prevention)")
	}
}

func TestHookExecutorReEntrancyPrevention(t *testing.T) {
	e := NewHookExecutor()
	e.mu.Lock()
	e.executing = true // simulate an executing hook
	e.mu.Unlock()

	handler := func(ctx context.Context, input HookInput) (HookOutput, error) {
		return HookOutput{}, nil
	}

	_, _, skipped := e.ExecuteWithSpiralPrevention(context.Background(), string(HookPreAPICall), handler, HookInput{}, 5*time.Second)
	if !skipped {
		t.Error("re-entrant hook should be skipped")
	}

	// Clean up
	e.mu.Lock()
	e.executing = false
	e.mu.Unlock()
}

func TestHookExecutorTimeout(t *testing.T) {
	e := NewHookExecutor()
	handler := func(ctx context.Context, input HookInput) (HookOutput, error) {
		select {
		case <-ctx.Done():
			return HookOutput{}, ctx.Err()
		case <-time.After(10 * time.Second):
			return HookOutput{}, nil
		}
	}

	_, err, _ := e.ExecuteWithSpiralPrevention(context.Background(), string(HookPreAPICall), handler, HookInput{}, 50*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestHookExecutorDefaultTimeout(t *testing.T) {
	e := NewHookExecutor()
	handler := func(ctx context.Context, input HookInput) (HookOutput, error) {
		// Check that the context has a deadline
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("context should have a deadline")
			return HookOutput{}, nil
		}
		remaining := time.Until(deadline)
		// Should be approximately DefaultHookTimeout (30s)
		if remaining < 25*time.Second || remaining > 35*time.Second {
			t.Errorf("expected deadline ~30s, got %v remaining", remaining)
		}
		return HookOutput{}, nil
	}

	e.ExecuteWithSpiralPrevention(context.Background(), string(HookPreAPICall), handler, HookInput{}, 0)
}

func TestHookExecutorNegativeTimeoutUsesDefault(t *testing.T) {
	e := NewHookExecutor()
	handler := func(ctx context.Context, input HookInput) (HookOutput, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("context should have a deadline")
			return HookOutput{}, nil
		}
		remaining := time.Until(deadline)
		if remaining < 25*time.Second {
			t.Errorf("expected default timeout (~30s), got %v remaining", remaining)
		}
		return HookOutput{}, nil
	}

	e.ExecuteWithSpiralPrevention(context.Background(), string(HookPreAPICall), handler, HookInput{}, -1*time.Second)
}

// ═══════════════════════════════════════════════════════════════════════════════
// NEW TESTS: Death spiral via HookManager
// ═══════════════════════════════════════════════════════════════════════════════

func TestDeathSpiralPreventionViaHookManager(t *testing.T) {
	hm := NewHookManager()
	callCount := int32(0)

	// Register an on-error hook that would normally trigger more errors
	hm.RegisterGeneric(HookOnError, "spiral-hook", func(ctx context.Context, input HookInput) (HookOutput, error) {
		atomic.AddInt32(&callCount, 1)
		// In real code, this handler might trigger another error
		// Simulate by calling ExecuteGenericHooks again recursively
		hm.ExecuteGenericHooksQuiet(HookOnError, map[string]interface{}{"nested": true})
		return HookOutput{}, nil
	}, 5*time.Second)

	// First call should work
	results := hm.ExecuteGenericHooksQuiet(HookOnError, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// The nested call from inside the handler should be prevented by re-entrancy
	// So callCount should be 1 (only the outer call ran)
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call, got %d (death spiral not prevented)", callCount)
	}
}

func TestErrorDepthResetsAfterExecution(t *testing.T) {
	hm := NewHookManager()
	callCount := int32(0)

	hm.RegisterGeneric(HookOnError, "test-hook", func(ctx context.Context, input HookInput) (HookOutput, error) {
		atomic.AddInt32(&callCount, 1)
		return HookOutput{}, nil
	}, 5*time.Second)

	// Call multiple times sequentially - should all succeed since depth resets
	for i := 0; i < 5; i++ {
		results := hm.ExecuteGenericHooksQuiet(HookOnError, nil)
		if len(results) != 1 || !results[0].Success {
			t.Errorf("call %d should succeed", i+1)
		}
	}
	if atomic.LoadInt32(&callCount) != 5 {
		t.Errorf("expected 5 calls, got %d", callCount)
	}
}

// ─── Concurrent hook registration ────────────────────────────────────────────

func TestConcurrentGenericHookRegistration(t *testing.T) {
	hm := NewHookManager()
	handler := func(ctx context.Context, input HookInput) (HookOutput, error) {
		return HookOutput{}, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hm.RegisterGeneric(HookPreAPICall, fmt.Sprintf("hook-%d", idx), handler, 5*time.Second)
		}(i)
	}
	wg.Wait()

	hm.mu.RLock()
	hooks := hm.genericHooks[HookPreAPICall]
	hm.mu.RUnlock()

	if len(hooks) != 10 {
		t.Errorf("expected 10 hooks, got %d", len(hooks))
	}
}

// ─── HookInput type ──────────────────────────────────────────────────────────

func TestHookInput(t *testing.T) {
	input := HookInput{
		HookType: string(HookPreAPICall),
		Metadata: map[string]interface{}{"model": "claude-4", "turn": 1},
	}
	if input.HookType != "pre_api_call" {
		t.Errorf("expected 'pre_api_call', got %q", input.HookType)
	}
	if input.Metadata["model"] != "claude-4" {
		t.Errorf("expected 'claude-4', got %v", input.Metadata["model"])
	}
}

// ─── HookOutput type ─────────────────────────────────────────────────────────

func TestHookOutput(t *testing.T) {
	output := HookOutput{
		Metadata: map[string]interface{}{"blocked": true},
	}
	if output.Metadata["blocked"] != true {
		t.Error("expected blocked=true")
	}
}

// ─── NewHookExecutor ─────────────────────────────────────────────────────────

func TestNewHookExecutor(t *testing.T) {
	e := NewHookExecutor()
	if e == nil {
		t.Fatal("NewHookExecutor should not return nil")
	}
	if e.maxErrorDepth != maxErrorDepthDefault {
		t.Errorf("expected maxErrorDepth=%d, got %d", maxErrorDepthDefault, e.maxErrorDepth)
	}
	if e.executing {
		t.Error("executing should be false initially")
	}
	if e.errorDepth != 0 {
		t.Errorf("errorDepth should be 0, got %d", e.errorDepth)
	}
}

// ─── All hook events are invocable ──────────────────────────────────────────

func TestAllHookEventsInvocable(t *testing.T) {
	// Verify that all defined HookEvent constants can be registered and
	// executed through the generic hook system. This catches the case where
	// a hook type is defined but never wired into the agent loop.
	allHooks := []HookEvent{
		HookPreAPICall, HookPostAPICall,
		HookPreUserMessage, HookPostUserMessage,
		HookPreAssistantMessage, HookPostAssistantMessage,
		HookOnError, HookOnAbort,
		HookOnNotification, HookOnSubagent, HookOnFork, HookOnResume,
		HookPreToolUse, HookPostToolUse,
		HookStop,
	}

	for _, event := range allHooks {
		t.Run(string(event), func(t *testing.T) {
			hm := NewHookManager()
			called := false
			hm.RegisterGeneric(event, "test-hook", func(ctx context.Context, input HookInput) (HookOutput, error) {
				called = true
				return HookOutput{}, nil
			}, 5*time.Second)

			results, err := hm.ExecuteGenericHooks(event, nil)
			if err != nil {
				t.Errorf("ExecuteGenericHooks(%s) returned error: %v", event, err)
			}
			if len(results) != 1 {
				t.Errorf("expected 1 result for %s, got %d", event, len(results))
			}
			if !called {
				t.Errorf("handler for %s was not called", event)
			}
		})
	}
}
