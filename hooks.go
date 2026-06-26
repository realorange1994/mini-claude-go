package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// packageGitBashPath is set from main after config loading.
// Used by detectGitBashForHook() and exec_tool.go as a config-derived source
// (above env fallback).
var packageGitBashPath string

// ─── Hook event constants ─────────────────────────────────────────────────────

// HookEvent represents the type of hook event being triggered.
type HookEvent string

const (
	// API lifecycle hooks
	HookPreAPICall  HookEvent = "pre_api_call"
	HookPostAPICall HookEvent = "post_api_call"

	// Message lifecycle hooks
	HookPreUserMessage       HookEvent = "pre_user_message"
	HookPostUserMessage      HookEvent = "post_user_message"
	HookPreAssistantMessage  HookEvent = "pre_assistant_message"
	HookPostAssistantMessage HookEvent = "post_assistant_message"

	// Error and abort hooks
	HookOnError HookEvent = "on_error"
	HookOnAbort HookEvent = "on_abort"

	// Notification and lifecycle hooks
	HookOnNotification HookEvent = "on_notification"
	HookOnSubagent     HookEvent = "on_subagent"
	HookOnFork         HookEvent = "on_fork"
	HookOnResume       HookEvent = "on_resume"

	// Tool lifecycle hooks
	HookPreToolUse  HookEvent = "pre_tool_use"
	HookPostToolUse HookEvent = "post_tool_use"

	// Session lifecycle
	HookStop HookEvent = "stop"
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
	Trigger            HookTrigger
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
	CompactSummary string   // the summary that replaced the compacted conversation
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
	preCompactHooks []struct {
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

	_ = startTotal // [STUB] reserved for future timeout-at-iteration-level feature
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
type PreCompactHandler func(ctx context.Context, input PreCompactInput) (PreCompactOutput, error)
type PostCompactHandler func(ctx context.Context, input PostCompactInput) (PostCompactOutput, error)

// ─── Shell command hook configuration ─────────────────────────────────────────

// HookCommand represents a shell command hook loaded from settings.json.
// Matching upstream's HookCommand type in settings/types.ts.
type HookCommand struct {
	Matcher string                 `json:"matcher"`           // tool name or glob pattern (e.g., "Bash", "Write", "Edit")
	Command string                 `json:"command"`           // shell command to execute
	Shell   string                 `json:"shell,omitempty"`   // "bash" (default) or "powershell"
	Timeout int                    `json:"timeout,omitempty"` // timeout in seconds (default: 600)
	Async   bool                   `json:"async,omitempty"`   // run in background
	Type    string                 `json:"type,omitempty"`    // "command" (default)
	When    map[string]interface{} `json:"when,omitempty"`    // conditional execution
}

// HookConfig represents the hooks section in settings.json.
// Keys are hook event names (e.g., "PreToolUse", "PostToolUse").
type HookConfig map[string][]HookCommand

// HookShellResult is the parsed result from a shell hook's stdout.
// Matching upstream's HookJSONOutput schema.
type HookShellResult struct {
	// Common fields
	Continue       bool   `json:"continue,omitempty"`
	SuppressOutput bool   `json:"suppressOutput,omitempty"`
	StopReason     string `json:"stopReason,omitempty"`
	Decision       string `json:"decision,omitempty"` // "approve" or "block"
	Reason         string `json:"reason,omitempty"`
	SystemMessage  string `json:"systemMessage,omitempty"`

	// PreToolUse specific
	HookSpecificOutput *json.RawMessage `json:"hookSpecificOutput,omitempty"`

	// Parsed from hookSpecificOutput for PreToolUse
	PermissionDecision       string                 `json:"-"` // "allow", "deny", "ask"
	PermissionDecisionReason string                 `json:"-"`
	UpdatedInput             map[string]interface{} `json:"-"`

	// Raw output for diagnostics
	RawStdout string
	RawStderr string
	ExitCode  int
}

// HookBlockError signals that a PreToolUse hook blocked tool execution.
type HookBlockError struct {
	ToolName string
	Command  string
	Reason   string
}

func (e HookBlockError) Error() string {
	return fmt.Sprintf("Hook blocked %s: %s (command: %s)", e.ToolName, e.Reason, e.Command)
}

// ─── Shell command execution ──────────────────────────────────────────────────

// defaultHookTimeout is the default timeout for shell command hooks (10 minutes).
// Matching upstream's TOOL_HOOK_EXECUTION_TIMEOUT_MS = 10 * 60 * 1000.
const defaultHookTimeout = 10 * 60 * time.Second

// ExecuteShellHook runs a shell command hook with JSON input via stdin.
// It spawns a shell process, passes the serialized input as JSON, and
// parses the stdout for structured output.
//
// Matching upstream's execCommandHook() in hooks.ts:830.
func ExecuteShellHook(ctx context.Context, hook HookCommand, hookEvent HookEvent, jsonInput string, extraEnv map[string]string) (*HookShellResult, error) {
	timeout := defaultHookTimeout
	if hook.Timeout > 0 {
		timeout = time.Duration(hook.Timeout) * time.Second
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build environment
	env := buildHookEnv(extraEnv)

	// Determine shell and command
	var cmd *exec.Cmd
	shellType := hook.Shell
	if shellType == "" {
		shellType = "bash"
	}

	if runtime.GOOS == "windows" && shellType == "powershell" {
		// PowerShell path detection
		pwsh := detectPowerShellForHook()
		if pwsh == "" {
			return nil, fmt.Errorf("hook has shell: 'powershell' but no PowerShell executable found")
		}
		args := buildPowerShellHookArgs(hook.Command)
		cmd = exec.CommandContext(execCtx, pwsh, args...)
	} else {
		// Bash/sh: use system shell
		if runtime.GOOS == "windows" {
			// On Windows, prefer Git Bash if available
			bash := detectGitBashForHook()
			if bash != "" {
				cmd = exec.CommandContext(execCtx, bash, "-c", hook.Command)
			} else {
				cmd = exec.CommandContext(execCtx, "cmd", "/c", hook.Command)
			}
		} else {
			cmd = exec.CommandContext(execCtx, "bash", "-c", hook.Command)
		}
	}

	cmd.Env = env
	cmd.Dir = getCwd()

	// Pipe stdin/stdout/stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start hook command: %w", err)
	}

	// Write JSON input to stdin (with trailing newline, matching upstream)
	stdin.Write([]byte(jsonInput + "\n"))
	stdin.Close()

	// Read stdout and stderr
	var stdoutBuf, stderrBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				stdoutBuf.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				stderrBuf.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	wg.Wait()

	exitCode := -1
	err = cmd.Wait()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	result := &HookShellResult{
		RawStdout: stdoutBuf.String(),
		RawStderr: stderrBuf.String(),
		ExitCode:  exitCode,
	}

	// Parse stdout for structured output
	result.ParseStdout()

	return result, nil
}

// ParseStdout parses the hook's stdout for JSON output.
// If stdout starts with '{', it's treated as JSON and validated
// against the hook output schema. Otherwise, it's plain text.
// Matching upstream's parseHookOutput() in hooks.ts:400.
func (r *HookShellResult) ParseStdout() {
	stdout := strings.TrimSpace(r.RawStdout)
	if stdout == "" || !strings.HasPrefix(stdout, "{") {
		// Plain text output — no structured parsing
		return
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		// Failed to parse as JSON — treat as plain text
		return
	}

	// Extract common fields
	if v, ok := parsed["continue"].(bool); ok && !v {
		r.Continue = false
	}
	if v, ok := parsed["suppressOutput"].(bool); ok {
		r.SuppressOutput = v
	}
	if v, ok := parsed["stopReason"].(string); ok {
		r.StopReason = v
	}
	if v, ok := parsed["decision"].(string); ok {
		r.Decision = v
	}
	if v, ok := parsed["reason"].(string); ok {
		r.Reason = v
	}
	if v, ok := parsed["systemMessage"].(string); ok {
		r.SystemMessage = v
	}

	// Extract hookSpecificOutput for PreToolUse hooks
	if raw, ok := parsed["hookSpecificOutput"]; ok {
		if spec, ok := raw.(map[string]interface{}); ok {
			if v, ok := spec["permissionDecision"].(string); ok {
				r.PermissionDecision = v
			}
			if v, ok := spec["permissionDecisionReason"].(string); ok {
				r.PermissionDecisionReason = v
			}
			if v, ok := spec["updatedInput"].(map[string]interface{}); ok {
				r.UpdatedInput = v
			}
			if v, ok := spec["additionalContext"].(string); ok {
				// Store additionalContext in SystemMessage if not already set
				if r.SystemMessage == "" {
					r.SystemMessage = v
				}
			}
		}
	}
}

// ShouldBlock returns true if the hook result indicates the tool should be blocked.
func (r *HookShellResult) ShouldBlock() bool {
	// Check decision field
	if r.Decision == "block" {
		return true
	}
	// Check hookSpecificOutput.permissionDecision
	if r.PermissionDecision == "deny" {
		return true
	}
	return false
}

// BlockReason returns the reason why the hook blocked the tool.
func (r *HookShellResult) BlockReason() string {
	if r.PermissionDecisionReason != "" {
		return r.PermissionDecisionReason
	}
	if r.Reason != "" {
		return r.Reason
	}
	return "Blocked by hook"
}

// ShouldAsk returns true if the hook wants the user to be prompted.
func (r *HookShellResult) ShouldAsk() bool {
	return r.PermissionDecision == "ask"
}

// ─── Hook matcher ─────────────────────────────────────────────────────────────

// MatchHook checks if a hook's matcher pattern matches the given query string.
// Supports exact match and glob patterns (* and ?).
// Matching upstream's matchesPattern() in hooks.ts:1484.
func MatchHook(hook HookCommand, query string) bool {
	pattern := hook.Matcher
	if pattern == query {
		return true
	}
	// Simple glob matching (supports * and ?)
	return hookGlobMatch(pattern, query)
}

// hookGlobMatch performs simple glob pattern matching for hook matchers.
// Supports * (any characters) and ? (single character).
// This is a simpler version than the file_history_tools globMatch which
// supports ** (recursive directory matching).
func hookGlobMatch(pattern, text string) bool {
	pLen := len(pattern)
	tLen := len(text)

	// dp[i][j] = can pattern[:i] match text[:j]?
	dp := make([][]bool, pLen+1)
	for i := range dp {
		dp[i] = make([]bool, tLen+1)
	}
	dp[0][0] = true

	for i := 1; i <= pLen; i++ {
		if pattern[i-1] == '*' {
			dp[i][0] = dp[i-1][0]
		}
		for j := 1; j <= tLen; j++ {
			if pattern[i-1] == '*' {
				dp[i][j] = dp[i-1][j] || dp[i][j-1]
			} else if pattern[i-1] == '?' || pattern[i-1] == text[j-1] {
				dp[i][j] = dp[i-1][j-1]
			}
		}
	}
	return dp[pLen][tLen]
}

// ─── Environment helpers ──────────────────────────────────────────────────────

// buildHookEnv builds the environment variables for a hook command.
// Inherits the current process environment and adds CLAUDE_* variables.
func buildHookEnv(extra map[string]string) []string {
	env := os.Environ()

	// Add Claude-specific environment variables
	cwd := getCwd()
	env = append(env, "CLAUDE_PROJECT_DIR="+cwd)
	env = append(env, "CLAUDE_CWD="+cwd)

	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// detectGitBashForHook finds the Git Bash executable on Windows.
// Uses the same path derivation logic as exec_tool.go's findGitBashForWindows().
func detectGitBashForHook() string {
	// 1. Check config-derived path (from settings.json)
	if packageGitBashPath != "" {
		if _, err := os.Stat(packageGitBashPath); err == nil {
			return packageGitBashPath
		}
	}

	// 2. Check CLAUDE_CODE_GIT_BASH_PATH env var (backward compat)
	if envPath := os.Getenv("CLAUDE_CODE_GIT_BASH_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	// 2. Find git.exe and derive bash.exe path
	gitPath := findGitExecutableForHook()
	if gitPath != "" {
		bashPath := filepath.Clean(filepath.Join(gitPath, "..", "..", "bin", "bash.exe"))
		if _, err := os.Stat(bashPath); err == nil {
			return bashPath
		}
	}

	// 3. Fallback: try PATH
	if p, err := exec.LookPath("bash"); err == nil {
		return p
	}

	return ""
}

// findGitExecutableForHook locates git.exe on Windows.
func findGitExecutableForHook() string {
	defaultLocations := []string{
		`C:\Program Files\Git\cmd\git.exe`,
		`C:\Program Files (x86)\Git\cmd\git.exe`,
	}
	for _, loc := range defaultLocations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	// Fall back to where.exe
	out, err := exec.Command("where.exe", "git").Output()
	if err != nil {
		return ""
	}

	cwd, _ := os.Getwd()
	cwdLower := strings.ToLower(cwd)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\r\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		absPath, err := filepath.Abs(line)
		if err != nil {
			continue
		}
		dirLower := strings.ToLower(filepath.Dir(absPath))
		if dirLower == cwdLower {
			continue
		}
		return line
	}

	return ""
}

// detectPowerShellForHook finds the PowerShell executable.
func detectPowerShellForHook() string {
	// Try pwsh first (cross-platform PowerShell 7+)
	if p, err := exec.LookPath("pwsh"); err == nil {
		return p
	}
	// Fall back to Windows PowerShell
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return ""
}

// buildPowerShellHookArgs builds the command-line arguments for PowerShell.
// Using -NoProfile for faster startup and -NonInteractive for fail-fast.
func buildPowerShellHookArgs(command string) []string {
	return []string{"-NoProfile", "-NonInteractive", "-Command", command}
}

// ─── Settings.json hook loading ───────────────────────────────────────────────

// LoadHooksFromSettings loads hook commands from a settings.json file.
// Returns a map of hook event to list of commands.
func LoadHooksFromSettings(filePath string) (HookConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	hooksRaw, ok := raw["hooks"]
	if !ok {
		return nil, nil
	}

	hooksMap, ok := hooksRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("hooks must be an object, got %T", hooksRaw)
	}

	result := make(HookConfig)
	for eventName, hookList := range hooksMap {
		hooks, ok := hookList.([]interface{})
		if !ok {
			continue
		}
		for _, h := range hooks {
			hookJSON, err := json.Marshal(h)
			if err != nil {
				continue
			}
			var hook HookCommand
			if err := json.Unmarshal(hookJSON, &hook); err != nil {
				continue
			}
			if hook.Type == "" {
				hook.Type = "command"
			}
			eventNameUpper := strings.ToUpper(eventName[:1]) + eventName[1:]
			result[eventNameUpper] = append(result[eventNameUpper], hook)
		}
	}

	return result, nil
}

// LoadAllHooks loads hooks from all config sources (project + home).
// Matching upstream's getHooksConfigFromSnapshot().
func LoadAllHooks(projectDir string) HookConfig {
	result := make(HookConfig)

	// Load from project-level settings
	projectHooksPath := filepath.Join(projectDir, ".claude", "settings.json")
	if hooks, err := LoadHooksFromSettings(projectHooksPath); err == nil {
		for event, cmds := range hooks {
			result[event] = append(result[event], cmds...)
		}
	}

	// Load from home directory settings
	if homeDir, err := os.UserHomeDir(); err == nil {
		homeHooksPath := filepath.Join(homeDir, ".claude", "settings.json")
		if hooks, err := LoadHooksFromSettings(homeHooksPath); err == nil {
			for event, cmds := range hooks {
				result[event] = append(result[event], cmds...)
			}
		}
	}

	return result
}

// RegisterShellHooks registers shell command hooks from settings into the HookManager.
// This converts HookCommand configs into generic hook handlers that execute
// the shell commands.
func (hm *HookManager) RegisterShellHooks(hookConfig HookConfig, event HookEvent) {
	cmds, ok := hookConfig[string(event)]
	if !ok {
		return
	}

	for _, cmd := range cmds {
		if cmd.Type != "" && cmd.Type != "command" {
			continue // only support command-type hooks for now
		}

		hookCmd := cmd // capture for closure
		name := fmt.Sprintf("shell:%s", hookCmd.Command)

		hm.RegisterGeneric(event, name, func(ctx context.Context, input HookInput) (HookOutput, error) {
			// Build JSON input for the hook
			jsonBytes, err := json.Marshal(input)
			if err != nil {
				return HookOutput{}, fmt.Errorf("failed to marshal hook input: %w", err)
			}

			result, err := ExecuteShellHook(ctx, hookCmd, event, string(jsonBytes), nil)
			if err != nil {
				return HookOutput{}, fmt.Errorf("hook command failed: %w", err)
			}

			out := HookOutput{
				Metadata: map[string]interface{}{
					"stdout":     result.RawStdout,
					"stderr":     result.RawStderr,
					"exit_code":  result.ExitCode,
					"continue":   result.Continue,
					"blocked":    result.ShouldBlock(),
					"ask":        result.ShouldAsk(),
					"reason":     result.BlockReason(),
					"stopReason": result.StopReason,
				},
			}

			if result.ShouldBlock() {
				return out, HookBlockError{
					ToolName: fmt.Sprintf("%v", input.Metadata["tool_name"]),
					Command:  hookCmd.Command,
					Reason:   result.BlockReason(),
				}
			}

			return out, nil
		}, time.Duration(cmd.Timeout)*time.Second)
	}
}
