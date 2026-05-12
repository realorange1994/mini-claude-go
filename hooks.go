package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ─── Hook event constants ─────────────────────────────────────────────────────

// HookEvent represents the type of hook event being triggered.
type HookEvent string

const (
	// Existing compact hooks
	HookPreCompact  HookEvent = "pre_compact"
	HookPostCompact HookEvent = "post_compact"

	// API lifecycle hooks
	HookPreAPICall  HookEvent = "pre_api_call"
	HookPostAPICall HookEvent = "post_api_call"

	// Message lifecycle hooks
	HookPreUserMessage     HookEvent = "pre_user_message"
	HookPostUserMessage    HookEvent = "post_user_message"
	HookPreAssistantMessage  HookEvent = "pre_assistant_message"
	HookPostAssistantMessage HookEvent = "post_assistant_message"

	// Error and abort hooks
	HookOnError  HookEvent = "on_error"
	HookOnAbort  HookEvent = "on_abort"

	// Notification and lifecycle hooks
	HookOnNotification HookEvent = "on_notification"
	HookOnSubagent     HookEvent = "on_subagent"
	HookOnFork         HookEvent = "on_fork"
	HookOnResume       HookEvent = "on_resume"

	// Tool lifecycle hooks
	HookPreToolUse  HookEvent = "pre_tool_use"
	HookPostToolUse HookEvent = "post_tool_use"
)

// ─── HookTrigger (existing) ───────────────────────────────────────────────────

// HookTrigger identifies what triggered the compaction.
type HookTrigger string

const (
	HookTriggerManual HookTrigger = "manual"
	HookTriggerAuto   HookTrigger = "auto"
	HookTriggerSM     HookTrigger = "sm_compact"
)

// ─── Default timeout ─────────────────────────────────────────────────────────

const DefaultHookTimeout = 30 * time.Second

// ─── Compact hook types (existing) ────────────────────────────────────────────

// PreCompactInput is passed to PreCompact hooks.
type PreCompactInput struct {
	Trigger          HookTrigger
	CustomInstructions string // instructions already queued for the summarizer; hooks can append
}

// PreCompactOutput is what a PreCompact hook can return.
type PreCompactOutput struct {
	CustomInstructions string // additional instructions for the compaction prompt
	UserMessage        string // message to display to the user (logged, not injected into prompt)
}

// PostCompactInput is passed to PostCompact hooks.
type PostCompactInput struct {
	Trigger        HookTrigger
	CompactSummary string // the summary that replaced the compacted conversation
	RecoveredFiles []string // files that were re-injected post-compaction
}

// PostCompactOutput is what a PostCompact hook can return.
type PostCompactOutput struct {
	UserMessage string // message to display to the user
	Attachment  string // content to inject as an attachment (added to prompt context)
}

// ─── Generic hook types ───────────────────────────────────────────────────────

// HookInput is the generic input passed to all hooks.
type HookInput struct {
	HookType string                 // which hook event triggered this
	Metadata map[string]interface{} // arbitrary context data (error, message, tool name, etc.)
}

// HookOutput is the generic output from a hook handler.
type HookOutput struct {
	Metadata map[string]interface{} // arbitrary data to pass back
}

// HookHandler is a generic hook handler function.
type HookHandler func(ctx context.Context, input HookInput) (HookOutput, error)

// registeredHook stores a registered hook with its timeout.
type registeredHook struct {
	name    string
	handler HookHandler
	timeout time.Duration
}

// HookResult tracks the outcome of a single hook execution.
type HookResult struct {
	Name    string
	Success bool
	Err     error
	Dur     time.Duration
}

// ─── Death spiral prevention ─────────────────────────────────────────────────

const maxErrorDepthDefault = 2 // maximum nested on-error hook depth

// HookExecutor manages hook execution with timeout and death spiral prevention.
// It prevents infinite loops when hooks themselves trigger errors that would
// trigger more hooks.
type HookExecutor struct {
	mu            sync.Mutex
	executing     bool // currently executing any hook?
	errorDepth    int  // nested error hook depth
	maxErrorDepth int  // maximum allowed depth
}

// NewHookExecutor creates a HookExecutor with default death spiral limits.
func NewHookExecutor() *HookExecutor {
	return &HookExecutor{
		maxErrorDepth: maxErrorDepthDefault,
	}
}

// ExecuteWithSpiralPrevention executes a hook with timeout and re-entrancy protection.
// Returns (output, error, skipped) where skipped is true if the hook was prevented
// from running due to death spiral or re-entrancy rules.
func (e *HookExecutor) ExecuteWithSpiralPrevention(
	ctx context.Context,
	hookType string,
	handler HookHandler,
	input HookInput,
	timeout time.Duration,
) (HookOutput, error, bool) {
	e.mu.Lock()

	// Death spiral prevention: limit nested on-error hook calls
	if hookType == string(HookOnError) {
		if e.errorDepth >= e.maxErrorDepth {
			e.mu.Unlock()
			log.Printf("[hook] death spiral prevented: %s at depth %d (max %d)",
				hookType, e.errorDepth, e.maxErrorDepth)
			return HookOutput{}, nil, true // skipped
		}
		e.errorDepth++
	}

	// Re-entrancy prevention: skip if already executing a hook
	if e.executing {
		currentDepth := e.errorDepth
		if hookType == string(HookOnError) {
			currentDepth-- // undo increment above
			e.errorDepth = currentDepth
		}
		e.mu.Unlock()
		log.Printf("[hook] re-entrancy prevented: %s (already executing)", hookType)
		return HookOutput{}, nil, true // skipped
	}
	e.executing = true
	e.mu.Unlock()

	// Ensure cleanup runs regardless of panic
	defer func() {
		e.mu.Lock()
		e.executing = false
		if hookType == string(HookOnError) {
			if e.errorDepth > 0 {
				e.errorDepth--
			}
		}
		e.mu.Unlock()
	}()

	// Apply timeout
	if timeout <= 0 {
		timeout = DefaultHookTimeout
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := handler(timeoutCtx, input)
	return out, err, false // not skipped
}

// ─── HookManager (expanded) ──────────────────────────────────────────────────

// HookManager manages registered hooks with timeout support and death spiral prevention.
type HookManager struct {
	executor *HookExecutor

	// Existing compact hooks (backward compatible)
	preCompactHooks  []struct {
		name    string
		handler PreCompactHandler
		timeout time.Duration
	}
	postCompactHooks []struct {
		name    string
		handler PostCompactHandler
		timeout time.Duration
	}

	// Generic hooks indexed by event type
	genericHooks map[HookEvent][]registeredHook
	mu           sync.RWMutex // protects genericHooks
}

// NewHookManager creates a HookManager with death spiral prevention.
func NewHookManager() *HookManager {
	return &HookManager{
		executor:     NewHookExecutor(),
		genericHooks: make(map[HookEvent][]registeredHook),
	}
}

// RegisterPreCompact adds a pre-compact hook with the given timeout.
func (hm *HookManager) RegisterPreCompact(name string, handler PreCompactHandler, timeout time.Duration) {
	hm.preCompactHooks = append(hm.preCompactHooks, struct {
		name    string
		handler PreCompactHandler
		timeout time.Duration
	}{name, handler, timeout})
}

// RegisterPostCompact adds a post-compact hook with the given timeout.
func (hm *HookManager) RegisterPostCompact(name string, handler PostCompactHandler, timeout time.Duration) {
	hm.postCompactHooks = append(hm.postCompactHooks, struct {
		name    string
		handler PostCompactHandler
		timeout time.Duration
	}{name, handler, timeout})
}

// RegisterGeneric adds a generic hook for the given event type with a timeout.
// If timeout is zero or negative, DefaultHookTimeout is used during execution.
func (hm *HookManager) RegisterGeneric(event HookEvent, name string, handler HookHandler, timeout time.Duration) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.genericHooks[event] = append(hm.genericHooks[event], registeredHook{
		name:    name,
		handler: handler,
		timeout: timeout,
	})
}

// ExecutePreCompactHooks runs all registered pre-compact hooks sequentially.
// Outputs are merged: CustomInstructions concatenated, UserMessage appended.
func (hm *HookManager) ExecutePreCompactHooks(input PreCompactInput) (PreCompactOutput, error) {
	if len(hm.preCompactHooks) == 0 {
		return PreCompactOutput{}, nil
	}

	var result PreCompactOutput
	var firstErr error
	for _, h := range hm.preCompactHooks {
		timeout := h.timeout
		if timeout <= 0 {
			timeout = 5 * time.Second // default 5s for compact hooks
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		out, err := h.handler(ctx, input)
		cancel()

		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("[hook:%s] %w", h.name, err)
			} else {
				firstErr = fmt.Errorf("%v\n[hook:%s] %w", firstErr, h.name, err)
			}
			result.UserMessage += fmt.Sprintf("\nPreCompact [hook:%s] failed: %v", h.name, err)
		} else {
			if out.CustomInstructions != "" {
				if result.CustomInstructions == "" {
					result.CustomInstructions = out.CustomInstructions
				} else {
					result.CustomInstructions += "\n\n" + out.CustomInstructions
				}
			}
			if out.UserMessage != "" {
				result.UserMessage += "\nPreCompact [hook:" + h.name + "] completed: " + out.UserMessage
			}
		}
	}
	return result, firstErr
}

// ExecutePostCompactHooks runs all registered post-compact hooks sequentially.
// Outputs are merged: UserMessage appended, Attachments concatenated.
func (hm *HookManager) ExecutePostCompactHooks(input PostCompactInput) (PostCompactOutput, error) {
	if len(hm.postCompactHooks) == 0 {
		return PostCompactOutput{}, nil
	}

	var result PostCompactOutput
	var firstErr error
	for _, h := range hm.postCompactHooks {
		timeout := h.timeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		out, err := h.handler(ctx, input)
		cancel()

		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("[hook:%s] %w", h.name, err)
			} else {
				firstErr = fmt.Errorf("%v\n[hook:%s] %w", firstErr, h.name, err)
			}
			result.UserMessage += fmt.Sprintf("\nPostCompact [hook:%s] failed: %v", h.name, err)
		} else {
			if out.UserMessage != "" {
				result.UserMessage += "\nPostCompact [hook:" + h.name + "] completed: " + out.UserMessage
			}
			if out.Attachment != "" {
				if result.Attachment == "" {
					result.Attachment = out.Attachment
				} else {
					result.Attachment += "\n\n" + out.Attachment
				}
			}
		}
	}
	return result, firstErr
}

// ─── Generic hook execution ───────────────────────────────────────────────────

// ExecuteGenericHooks runs all registered generic hooks for the given event type.
// It applies timeout and death spiral prevention automatically.
// Returns results for each hook and an aggregated error.
func (hm *HookManager) ExecuteGenericHooks(event HookEvent, metadata map[string]interface{}) ([]HookResult, error) {
	hm.mu.RLock()
	hooks := make([]registeredHook, len(hm.genericHooks[event]))
	copy(hooks, hm.genericHooks[event])
	hm.mu.RUnlock()

	if len(hooks) == 0 {
		return nil, nil
	}

	input := HookInput{
		HookType: string(event),
		Metadata: metadata,
	}

	var results []HookResult
	var firstErr error
	startTotal := time.Now()

	for _, h := range hooks {
		timeout := h.timeout
		if timeout <= 0 {
			timeout = DefaultHookTimeout
		}

		hookStart := time.Now()
		out, err, skipped := hm.executor.ExecuteWithSpiralPrevention(
			context.Background(), string(event), h.handler, input, timeout,
		)
		dur := time.Since(hookStart)

		if skipped {
			results = append(results, HookResult{
				Name:    h.name,
				Success: true, // not a failure, just skipped
				Dur:     dur,
			})
			continue
		}

		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("[hook:%s:%s] %w", event, h.name, err)
			} else {
				firstErr = fmt.Errorf("%v\n[hook:%s:%s] %w", firstErr, event, h.name, err)
			}
			results = append(results, HookResult{
				Name:    h.name,
				Success: false,
				Err:     err,
				Dur:     dur,
			})
		} else {
			// If hook returned metadata, merge it back into the input for the next hook
			if len(out.Metadata) > 0 {
				if input.Metadata == nil {
					input.Metadata = make(map[string]interface{})
				}
				for k, v := range out.Metadata {
					input.Metadata[k] = v
				}
			}
			results = append(results, HookResult{
				Name:    h.name,
				Success: true,
				Dur:     dur,
			})
		}
	}

	_ = startTotal // reserved for future timeout-at-iteration-level feature
	return results, firstErr
}

// ExecuteGenericHooksQuiet runs generic hooks but logs errors instead of returning them.
// This is useful for non-critical hooks that should never block the main loop.
func (hm *HookManager) ExecuteGenericHooksQuiet(event HookEvent, metadata map[string]interface{}) []HookResult {
	results, err := hm.ExecuteGenericHooks(event, metadata)
	if err != nil {
		log.Printf("[hook] %s hooks had error: %v", event, err)
	}
	return results
}

// ─── Legacy handler types (backward compatible) ──────────────────────────────

// HookHandler is the callback signature for compact hooks.
// Called synchronously with a timeout context.
// ctx is cancelled if the hook exceeds its timeout.
type PreCompactHandler  func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error)
type PostCompactHandler func(ctx context.Context, input PostCompactInput) (PostCompactOutput, error)
