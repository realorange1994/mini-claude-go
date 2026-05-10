package main

import (
	"context"
	"fmt"
	"time"
)

// HookEvent represents the type of hook event being triggered.
type HookEvent string

const (
	HookPreCompact  HookEvent = "pre_compact"
	HookPostCompact HookEvent = "post_compact"
)

// HookTrigger identifies what triggered the compaction.
type HookTrigger string

const (
	HookTriggerManual HookTrigger = "manual"
	HookTriggerAuto   HookTrigger = "auto"
	HookTriggerSM     HookTrigger = "sm_compact"
)

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

// HookResult is a generic hook result type.
type HookResult struct {
	Success bool
	Err     error
	Dur     time.Duration
}

// HookHandler is the callback signature for compact hooks.
// Called synchronously with a timeout context.
// ctx is cancelled if the hook exceeds its timeout.
type PreCompactHandler  func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error)
type PostCompactHandler func(ctx context.Context, input PostCompactInput) (PostCompactOutput, error)

// HookManager manages registered compact hooks.
type HookManager struct {
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
}

// NewHookManager creates a HookManager.
func NewHookManager() *HookManager {
	return &HookManager{}
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
			timeout = 5 * time.Second // default 5s
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
