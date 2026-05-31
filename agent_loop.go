package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"miniclaudecode-go/mcp"
	"miniclaudecode-go/skills"
	"miniclaudecode-go/tools"
	"miniclaudecode-go/transcript"
)

// LoopTransitionReason identifies why the agent loop continues to the next turn.
// Matching upstream's structured continue paths — each `continue` in the main
// loop carries an explicit reason for observability and debugging.
type LoopTransitionReason string

const (
	TransitionModelFallback   LoopTransitionReason = "model_fallback"        // 529 overload triggered model switch
	TransitionModelConfused   LoopTransitionReason = "model_confused"        // malformed response, retry with hint
	TransitionToolPairing     LoopTransitionReason = "tool_pairing_error"    // 2013 error, repaired context
	TransitionTruncatedArgs   LoopTransitionReason = "truncated_arguments"   // tool args cut off, retry with hint
	TransitionStreamStalled   LoopTransitionReason = "stream_stalled"        // safety timeout, truncation recovery
	TransitionContextOverflow LoopTransitionReason = "context_overflow"      // precise token-gap reactive compact
	TransitionContextExceeded LoopTransitionReason = "context_exceeded"      // context length error, aggressive compact
	TransitionEmptyResponse   LoopTransitionReason = "empty_response"        // thinking-only, nudge for output
	TransitionMaxTokens       LoopTransitionReason = "max_tokens_escalation" // max_tokens hit, escalate and retry
	TransitionRefusal         LoopTransitionReason = "content_refusal"       // content policy violation
	TransitionNone            LoopTransitionReason = ""                      // normal turn (no special transition)
)

// IterationBudget manages the turn budget for the agent loop.
type IterationBudget struct {
	max         int
	consumed    atomic.Int32
	graceCalled atomic.Bool
}

// NewIterationBudget creates a new iteration budget with the given maximum.
func NewIterationBudget(max int) *IterationBudget {
	return &IterationBudget{max: max}
}

// Consume attempts to consume one iteration unit. Returns false if exhausted.
func (b *IterationBudget) Consume() bool {
	for {
		c := b.consumed.Load()
		if int(c) >= b.max {
			return false
		}
		if b.consumed.CompareAndSwap(c, c+1) {
			return true
		}
	}
}

// Refund returns one consumed unit to the budget.
func (b *IterationBudget) Refund() {
	for {
		c := b.consumed.Load()
		if c <= 0 {
			return
		}
		if b.consumed.CompareAndSwap(c, c-1) {
			return
		}
	}
}

// GraceCall allows one extra call when the budget is exhausted.
// Returns true the first time it is called after exhaustion.
func (b *IterationBudget) GraceCall() bool {
	return !b.graceCalled.Swap(true)
}

// Consumed returns the number of iterations consumed so far.
func (b *IterationBudget) Consumed() int {
	return int(b.consumed.Load())
}

// registerAgentTool registers the AgentTool with this loop's SpawnFunc.
func (a *AgentLoop) registerAgentTool() {
	agentTool := &tools.AgentTool{
		SpawnFunc:     a.SpawnSubAgent,
		SpawnSyncFunc: a.SpawnSubAgent, // same callback; sync mode is controlled by the runInBackground flag
		HandleStore:   a.agentHandleStore,
	}
	a.registry.Register(agentTool)
}

// registerSendMessageTool registers the SendMessage tool with this loop's callback.
func (a *AgentLoop) registerSendMessageTool() {
	sendMsgTool := &tools.SendMessageTool{
		SendMessageFunc: a.SendMessageToSubAgent,
		GetStatusFunc:   a.GetSubAgentStatus,
		ResolveNameFunc: a.resolveAgentID,
		HandleStore:     a.agentHandleStore,
	}
	a.registry.Register(sendMsgTool)
}

// registerTodoWriteTool registers the TodoWriteTool with this loop's todo list.
func (a *AgentLoop) registerTodoWriteTool() {
	todoTool := &tools.TodoWriteTool{TodoList: a.todoList}
	a.registry.Register(todoTool)
}

// registerAskUserQuestionTool registers the AskUserQuestion tool.
func (a *AgentLoop) registerAskUserQuestionTool() {
	a.registry.Register(&tools.AskUserQuestionTool{})
}

// registerPlanModeTools registers the EnterPlanMode and ExitPlanMode tools.
func (a *AgentLoop) registerPlanModeTools() {
	a.registry.Register(&tools.EnterPlanModeTool{
		GetMode: func() string { return string(a.config.PermissionMode) },
		SetMode: func(mode string) {
			if a.config.PermissionMode != ModePlan {
				a.config.PrePlanMode = a.config.PermissionMode
			}
			a.config.PermissionMode = PermissionMode(mode)
		},
	})
	a.registry.Register(&tools.ExitPlanModeTool{
		GetMode:        func() string { return string(a.config.PermissionMode) },
		SetMode:        func(mode string) { a.config.PermissionMode = PermissionMode(mode) },
		GetPrePlanMode: func() string { return string(a.config.PrePlanMode) },
	})
}

// registerTaskOutputTool registers the TaskOutputTool with this loop's callback.
func (a *AgentLoop) registerTaskOutputTool() {
	taskOutputTool := &tools.TaskOutputTool{
		GetOutputFunc:   a.GetSubAgentOutput,
		GetProgressFunc: a.GetTaskProgress,
	}
	a.registry.Register(taskOutputTool)
}

// registerTaskStopTool registers the TaskStopTool with this loop's callback.
func (a *AgentLoop) registerTaskStopTool() {
	taskStopTool := &tools.TaskStopTool{
		StopFunc: a.StopBackgroundTask,
		GetFunc: func(id string) (*tools.WorkTaskInfo, bool) {
			if a.workTaskStore == nil {
				return nil, false
			}
			task := a.workTaskStore.GetTask(id)
			if task == nil {
				return nil, false
			}
			info := &tools.WorkTaskInfo{
				ID:          task.ID,
				Subject:     task.Subject,
				Description: task.Description,
				ActiveForm:  task.ActiveForm,
				Status:      string(task.Status),
				Owner:       task.Owner,
				Metadata:    task.Metadata,
				Blocks:      task.Blocks,
				BlockedBy:   task.BlockedBy,
				CreatedAt:   task.CreatedAt.Format(time.RFC3339),
				UpdatedAt:   task.UpdatedAt.Format(time.RFC3339),
			}
			return info, true
		},
	}
	a.registry.Register(taskStopTool)
}

// registerBashBgTool wires the ExecTool's BackgroundTaskCallback to this loop's
// spawnBackgroundBashCommand method, enabling run_in_background support.
// TimeoutCallback is wired to registerExistingProcessAsBgTask for auto-backgrounding
// timed-out exec commands.
// Also wires MCPToolCaller's TimeoutCallback for auto-backgrounding timed-out MCP calls.
func (a *AgentLoop) registerBashBgTool() {
	if tool, ok := a.registry.Get("exec"); ok {
		if execTool, ok := tool.(*tools.ExecTool); ok {
			execTool.BackgroundTaskCallback = a.spawnBackgroundBashCommand
			execTool.TimeoutCallback = a.registerExistingProcessAsBgTask
		}
	}
	if tool, ok := a.registry.Get("mcp_call_tool"); ok {
		if mcpTool, ok := tool.(*tools.MCPToolCaller); ok {
			mcpTool.TimeoutCallback = a.registerMCPTimeoutAsBgTask
		}
	}
}

// registerAgentManagementTools registers the agent_list, agent_get, and agent_kill
// tools wired to the agentTaskStore.
func (a *AgentLoop) registerAgentManagementTools() {
	if a.agentTaskStore == nil {
		return
	}
	a.registry.Register(&tools.AgentListTool{Store: a.agentTaskStore})
	a.registry.Register(&tools.AgentGetTool{Store: a.agentTaskStore})
	a.registry.Register(&tools.AgentKillTool{Store: a.agentTaskStore})
}

// registerWorkTaskTools registers the TaskCreate/List/Get/Update tools
// wired to this loop's WorkTaskStore.
func (a *AgentLoop) registerWorkTaskTools() {
	if a.workTaskStore == nil {
		return
	}

	a.registry.Register(&tools.TaskCreateTool{
		CreateFunc: a.workTaskStore.CreateTask,
	})

	a.registry.Register(&tools.TaskListTool{
		ListFunc: func() []tools.WorkTaskInfo {
			tasks := a.workTaskStore.ListTasks()
			result := make([]tools.WorkTaskInfo, len(tasks))
			for i, t := range tasks {
				result[i] = tools.WorkTaskInfo{
					ID:          t.ID,
					Subject:     t.Subject,
					Description: t.Description,
					ActiveForm:  t.ActiveForm,
					Status:      string(t.Status),
					Owner:       t.Owner,
					Metadata:    t.Metadata,
					Blocks:      t.Blocks,
					BlockedBy:   t.BlockedBy,
					CreatedAt:   t.CreatedAt.Format(time.RFC3339),
					UpdatedAt:   t.UpdatedAt.Format(time.RFC3339),
				}
			}
			return result
		},
	})

	a.registry.Register(&tools.TaskGetTool{
		GetFunc: func(id string) (*tools.WorkTaskInfo, bool) {
			task := a.workTaskStore.GetTask(id)
			if task == nil {
				return nil, false
			}
			info := &tools.WorkTaskInfo{
				ID:          task.ID,
				Subject:     task.Subject,
				Description: task.Description,
				ActiveForm:  task.ActiveForm,
				Status:      string(task.Status),
				Owner:       task.Owner,
				Metadata:    task.Metadata,
				Blocks:      task.Blocks,
				BlockedBy:   task.BlockedBy,
				CreatedAt:   task.CreatedAt.Format(time.RFC3339),
				UpdatedAt:   task.UpdatedAt.Format(time.RFC3339),
			}
			return info, true
		},
	})

	a.registry.Register(&tools.TaskUpdateTool{
		UpdateFunc: a.workTaskStore.UpdateTask,
	})
}

// EnqueueAgentNotification pushes a formatted task notification XML to the notification channel.
func (a *AgentLoop) EnqueueAgentNotification(taskID, status, result, transcriptPath, outputFile string, toolsUsed int, totalTokens int, durationMs int64) {
	notification := fmt.Sprintf(`<task-notification>
<agentId>%s</agentId>
<status>%s</status>
<result>%s</result>
<output_file>%s</output_file>
<transcript_path>%s</transcript_path>
<usage><total_tokens>%d</total_tokens><tool_uses>%d</tool_uses><duration_ms>%d</duration_ms></usage>
</task-notification>`, taskID, status, result, outputFile, transcriptPath, totalTokens, toolsUsed, durationMs)

	select {
	case a.notificationChan <- notification:
	default:
		// Channel is full — log instead of silently dropping.
		// With 64 slots this should only happen with many concurrent sub-agents.
		a.logDebug("[warning] notification channel full, dropping: %s\n", taskID)
	}
}

// DrainNotifications returns all pending notifications and clears the channel.
func (a *AgentLoop) DrainNotifications() []string {
	var notifications []string
	for {
		select {
		case n := <-a.notificationChan:
			notifications = append(notifications, n)
		default:
			return notifications
		}
	}
}

// InjectNotifications adds notification text as a user message to the conversation context.
// This ensures the LLM can see and act on async agent completions.
func (a *AgentLoop) InjectNotifications(notifications []string) {
	if len(notifications) == 0 {
		return
	}

	// Hook: OnNotification — when sub-agent completions are injected
	if a.hooks != nil {
		a.hooks.ExecuteGenericHooksQuiet(HookOnNotification, map[string]interface{}{
			"notification_count": len(notifications),
		})
	}

	var sb strings.Builder
	// Use different prefix depending on whether any task was killed vs completed
	hasKilled := false
	for _, n := range notifications {
		if strings.Contains(n, "<status>killed</status>") {
			hasKilled = true
			break
		}
	}
	if hasKilled {
		sb.WriteString("[System: The following sub-agent tasks finished while you were waiting (some were killed)]\n\n")
	} else {
		sb.WriteString("[System: The following sub-agent tasks completed while you were waiting]\n\n")
	}
	for _, n := range notifications {
		sb.WriteString(n)
		sb.WriteString("\n\n")
	}
	a.context.AddUserMessage(sb.String())
}

// AgentLoop drives the core agentic loop.
type AgentLoop struct {
	config                    Config
	registry                  *tools.Registry
	gate                      *PermissionGate
	context                   *ConversationContext
	client                    anthropic.Client
	snapshots                 *SnapshotHistory
	transcript                *transcript.Writer
	skillTracker              *skills.SkillTracker
	compactor                 *Compactor
	useStream                 bool
	maxToolChars              int // max chars per tool result (default 8000, matching openclacky's 4000-byte terminal budget)
	toolTimeoutMs             int // per-tool execution timeout in ms (default 600000 = 10min)
	maxTurns                  int // hard cap on turns (default from config.MaxTurns)
	budget                    *IterationBudget
	interrupted               atomic.Bool                         // set by Ctrl+C handler to stop the loop
	lastDeltasState           DeltasState                         // tracks what was streamed in last attempt
	rateLimitState            RateLimitState                      // rate limit headers from API responses
	prevTurnTokens            int                                 // tracks token count from previous turn for reactive compact
	activeSubAgents           sync.WaitGroup                      // tracks running sub-agents (Wait blocks until all complete)
	taskStore                 *TaskStore                          // tracks all sub-agent tasks (bash + sub-agents)
	agentTaskStore            *tools.AgentTaskStore               // tracks background agent tasks (with output capture)
	currentMaxTokens          atomic.Int64                        // effective max_tokens for API calls (escates on max_tokens hit)
	notificationChan          chan string                         // buffered channel for async task notifications
	evictionDone              chan struct{}                       // signals the eviction ticker goroutine to stop
	agentNameRegistry         map[string]string                   // maps short agent names to task IDs
	agentHandleStore          *tools.AgentHandleStore             // named agent handle store for routing
	cancelCtx                 context.Context                     // cancellable context for async sub-agents
	cancelFunc                context.CancelFunc                  // cancel function for async sub-agents
	workTaskStore             *WorkTaskStore                      // tracks LLM work items (TODO list)
	redundantCallDetector     *RedundantCallDetector              // tracks redundant tool calls
	stormBreaker              *StormBreaker                       // detects and suppresses repeat-loop tool call storms
	truncatedResultSaver      *TruncatedResultSaver               // saves truncated tool output to disk for recall
	consecutiveCallTracker    *ConsecutiveCallTracker             // tracks consecutive identical tool call failures for sharper errors
	toolListFingerprint       *ToolListFingerprint                // tracks tool list schema hash to detect cache-invalidating drift
	prefixFingerprint         *PrefixFingerprint                  // tracks system+tools+fewshots hash to detect prefix cache drift
	foldSummaryPin            *FoldSummaryPin                    // tracks content that must survive compaction (active skills, constraints)
	cacheMetrics             *CacheMetrics                      // tracks cache hit/miss tokens per API call
	readTracker              *ReadTracker                       // tracks files read for read-before-edit validation
	budgetManager            *ProactiveBudgetManager            // proactive context window budget management
	agentOutput               io.Writer                           // configurable output for terminal (defaults to os.Stderr); background agents override to capture output
	drainPendingMessagesFunc  func() []string                     // called at turn boundaries to drain pending messages from parent task store
	toolStateTracker          *ToolStateTracker                   // tracks tool state for injection into system prompt
	todoList                  *tools.TodoList                     // structured task list for TodoWrite tool
	totalInputTokens          atomic.Int64                        // cumulative input tokens across all turns
	totalOutputTokens         atomic.Int64                        // cumulative output tokens across all turns
	lastAPIInputTokens        atomic.Int64                        // exact input tokens from the most recent API response
	lastAPIOutputTokens       atomic.Int64                        // exact output tokens from the most recent API response
	totalCacheCreationTokens  atomic.Int64                        // cumulative cache_creation_input_tokens
	totalCacheReadTokens      atomic.Int64                        // cumulative cache_read_input_tokens
	totalCacheEditsDeletions  atomic.Int64                        // cumulative tool results deleted by cache_edits (proxy for cache_deleted_input_tokens)
	costTracker               *CostTracker                        // per-model USD cost tracking with session persistence
	cacheBreakDetector        *CacheBreakDetector                 // detects KV cache breaks between API calls
	cachedMC                  *CachedMicrocompactTracker          // cache_edits tracking
	extractionState           *ExtractionState                    // session memory extraction threshold tracking
	sonnetModel               string                              // fallback model for 529 overload (defaults to claude-sonnet-4-20250514)
	hooks                     *HookManager                        // compact pre/post hook handlers
	shellHooks                HookConfig                          // shell command hooks from settings.json
	consecutiveContextErrors  int                                 // tracks consecutive context overflow errors for reactive compact
	consecutive2013Errors     int                                 // tracks consecutive 2013 tool pairing errors
	consecutive529Errors      int                                 // tracks consecutive 529 overloaded errors for model fallback
	modelCapabilities         *ModelCapabilitiesCache             // per-model context window and capability lookup
	consecutiveStreamFailures int                                 // tracks consecutive streaming failures for non-streaming fallback
	timeoutHintInjected       bool                                // one-shot: timeout hint injected this session (openclacky pattern)
	truncationHintInjected    bool                                // one-shot: truncation hint injected this session (openclacky pattern)
	budgetHint75Injected      bool                                // one-shot: 75%% budget hint injected this session (openclacky pattern)
	inlineCompressionMode     bool                                // set during inline cache-reusing compaction (openclacky pattern)
	toolSchemaCache           map[string]anthropic.ToolUnionParam // tool name → cached schema (avoid re-serialize)
	toolSchemaCacheHash       uint64                              // hash of tool names for cache invalidation
	thinkingClearLatched      bool                                // once set (>1h idle), stays true for session — clears thinking via context_management
	lastApiCompletionTime     time.Time                           // timestamp of last successful API call (for thinking latch)
	ttlLockedUntilUnix       int64                               // TTL lock: unix timestamp cache stays valid until (atomic, session-stable locking)
	announcedMCPServers       map[string]bool                     // servers whose instructions have been announced this session (delta tracking)
	betaHeadersLatched        []string                            // once set, stays same for session — prevents mid-session header churn
	errorReporter             *ErrorReporter                      // captures error events for analysis
	lastTransition            LoopTransitionReason                // reason for the most recent loop continue
	telemetry                 *TelemetryManager                   // telemetry event tracking
	cronScheduler             *CronScheduler                      // cron task scheduler (started after agent setup)
	debugLog                  *os.File                            // file handle for diagnostic logging (not console)
	// Task-scoped iteration tracking (openclacky pattern): tracks iteration
	// count and skill read count at task start, used to compute task-local
	// iteration counts for skill evolution and memory updater triggers.
	taskStartTurns            int                                 // turns consumed at task start
	taskStartReadSkillCount   int                                 // skillTracker.ReadCount() at task start
	// Injection queue (openclacky pattern): deferred message injection
	// ensures correct tool_use/tool_result pairing. When read_skill or
	// similar tools produce inline content that must be injected as an
	// assistant message, enqueuing prevents the injection from appearing
	// between the toolUse and its toolResult (which would break API pairing).
	pendingInjections         []InjectionEntry                    // queue of deferred injections to flush after observe()
}

// InjectionEntry represents a pending injection to be flushed into the
// conversation history after tool results are appended. This ensures
// correct message ordering: tool_use → tool_result → injection.
// Matches openclacky's @pending_injections pattern.
type InjectionEntry struct {
	AssistantContent string // content for the assistant message (e.g., skill instructions)
	UserShim         string // synthetic user message to satisfy strict API alternation rules
}

// SetCronScheduler attaches a cron scheduler to the agent loop and starts it.
// The scheduler will inject cron prompts as user messages into the conversation.
func (a *AgentLoop) SetCronScheduler(s *CronScheduler) {
	if s == nil {
		return
	}
	a.cronScheduler = s
	s.onFire = func(prompt string) {
		a.EnqueueCronPrompt(prompt)
	}
	s.start()
}

// EnqueueCronPrompt injects a cron prompt as a user message into the conversation.
func (a *AgentLoop) EnqueueCronPrompt(prompt string) {
	a.context.AddUserMessage(prompt)
}

// EnqueueInjection defers a message injection until after the current tool
// results are appended to the conversation. This is critical for inline skill
// loading: if we inject the skill instructions as an assistant message
// directly in the tool's Execute(), it would appear between the toolUse block
// and its toolResult, breaking API pairing (causing 2013 errors).
//
// The injection consists of:
//   - AssistantContent: the actual content to inject (e.g., skill instructions)
//   - UserShim: a synthetic user message to satisfy strict user/assistant
//     alternation rules required by Anthropic/Bedrock APIs
//
// Matches openclacky's enqueue_injection() pattern in agent.rb:1003-1005.
func (a *AgentLoop) EnqueueInjection(assistantContent, userShim string) {
	a.pendingInjections = append(a.pendingInjections, InjectionEntry{
		AssistantContent: assistantContent,
		UserShim:         userShim,
	})
}

// FlushPendingInjections processes all queued injections, appending them to
// the conversation history as assistant + user message pairs. Must be called
// AFTER tool results are added (observe step), ensuring correct message order:
//
//	assistant: {toolUse: read_skill}
//	user:      {toolResult: skill content}
//	assistant: {text: skill instructions}   ← flush injects this
//	user:      "[SYSTEM] proceed..."        ← flush injects this shim
//
// Matches openclacky's flush_pending_injections() in agent.rb:1042-1049.
func (a *AgentLoop) FlushPendingInjections() {
	if len(a.pendingInjections) == 0 {
		return
	}
	for _, entry := range a.pendingInjections {
		a.context.AddAssistantText(entry.AssistantContent)
		if entry.UserShim != "" {
			a.context.AddUserMessage(entry.UserShim)
		}
	}
	a.pendingInjections = a.pendingInjections[:0] // clear without realloc
}

// handle529Error processes a 529 Overloaded error. It increments the consecutive
// 529 counter and triggers model fallback after 3 consecutive 529s.
// Returns true if the caller should continue retrying, false if fallback was triggered.
func (a *AgentLoop) handle529Error() bool {
	a.consecutive529Errors++
	a.logDebug("[529] consecutive=%d model=%s fallback=%s\n", a.consecutive529Errors, a.config.Model, a.sonnetModel)
	if a.consecutive529Errors >= 3 {
		originalModel := a.config.Model
		fallbackModel := a.sonnetModel
		a.out("\n[529 Overloaded] Falling back from %s to %s after %d consecutive 529 errors\n",
			originalModel, fallbackModel, a.consecutive529Errors)
		a.config.Model = fallbackModel
		a.consecutive529Errors = 0
		return false
	}
	return true
}

// handleRefusal checks if stopReason is "refusal" (content policy filter) and
// returns an error message if so. Matching upstream's getErrorMessageIfRefusal()
// in errors.ts:1187.
func (a *AgentLoop) handleRefusal(stopReason string) string {
	if stopReason != "refusal" {
		return ""
	}
	a.out("\n[refusal] Claude Code is unable to respond to this request, which appears to violate our Usage Policy.\n")
	return "Error: Claude Code is unable to respond to this request, which appears to violate our Usage Policy (https://www.anthropic.com/legal/aup). Please double press esc to edit your last message or start a new session for Claude Code to assist with a different task."
}

// trackStreamFailure increments the consecutive stream failure counter.
// After 3 consecutive failures, it disables streaming for the rest of the session
// and falls back to non-streaming API calls.
func (a *AgentLoop) trackStreamFailure() {
	a.consecutiveStreamFailures++
	a.logDebug("[stream-fallback] consecutive failures=%d threshold=%d\n", a.consecutiveStreamFailures, 3)
	if a.consecutiveStreamFailures >= 3 {
		a.out("\n[WARN] Streaming failed %d times consecutively — switching to non-streaming mode for this session\n",
			a.consecutiveStreamFailures)
		a.useStream = false
		a.consecutiveStreamFailures = 0
	}
}

func (r LoopTransitionReason) String() string {
	if r == "" {
		return "normal"
	}
	return string(r)
}

// handle429Error determines whether a 429 rate-limit error should be retried
// based on the subscriber's tier. Returns true if the caller should retry.
func (a *AgentLoop) handle429Error(errMsg string) bool {
	a.logDebug("[429] subscription=%s errMsg=%s\n", a.config.SubscriptionType, errMsg)
	isOverage := containsOverageSignal(errMsg)
	if !shouldRetry429(a.config.SubscriptionType, isOverage) {
		a.out("\n[429 Rate Limit] Subscription type %q -- skipping retry (usage limit hit)%s\n",
			a.config.SubscriptionType, overageSuffix(isOverage))
		return false
	}
	return true
}

// overageSuffix returns a parenthetical note if overage was detected.
func overageSuffix(isOverage bool) string {
	if isOverage {
		return " (overage detected)"
	}
	return ""
}

// recordTokenUsageWithCache accumulates API token usage including cache tokens.
func (a *AgentLoop) recordTokenUsageWithCache(inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens int64) {
	if inputTokens > 0 {
		a.totalInputTokens.Add(inputTokens)
		a.lastAPIInputTokens.Store(inputTokens)
	}
	if outputTokens > 0 {
		a.totalOutputTokens.Add(outputTokens)
		a.lastAPIOutputTokens.Store(outputTokens)
	}
	if cacheWriteTokens > 0 {
		a.totalCacheCreationTokens.Add(cacheWriteTokens)
	}
	if cacheReadTokens > 0 {
		a.totalCacheReadTokens.Add(cacheReadTokens)
	}
	if a.costTracker != nil {
		a.costTracker.AddUsage(a.config.Model, inputTokens, outputTokens)
	}
}

// RemainingTokenBudget returns the estimated remaining tokens in the context window.
// Uses the exact input_tokens from the most recent API response if available,
// otherwise falls back to EstimatedTokens().
func (a *AgentLoop) RemainingTokenBudget() int {
	var contextWindow int
	if a.modelCapabilities != nil {
		contextWindow = int(a.modelCapabilities.GetContextWindow(a.config.Model, a.config.MaxContextTokens))
	} else {
		contextWindow = modelContextWindow(a.config.Model)
	}
	if contextWindow < 1 {
		contextWindow = 200_000
	}

	lastAPI := a.lastAPIInputTokens.Load()
	if lastAPI > 0 {
		return contextWindow - int(lastAPI)
	}
	// Fallback to heuristic estimate
	return contextWindow - a.context.EstimatedTokens()
}

// ExactContextTokens returns the exact token count from the most recent API response,
// or 0 if no API call has been made yet.
func (a *AgentLoop) ExactContextTokens() int64 {
	return a.lastAPIInputTokens.Load()
}

// out writes formatted output to the agent's configured output writer.
// For foreground agents this goes to os.Stderr; for background agents it goes to the task buffer.
// This avoids process-level stdout/stderr redirection which would block the main REPL.
func (a *AgentLoop) out(format string, args ...interface{}) {
	w := a.agentOutput
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, format, args...)
}

// logDebug writes an internal diagnostic message to the debug log file.
// Messages are NOT printed to console — they go to .claude/debug.log instead.
// If the debug log file is not open, the message is silently dropped.
func (a *AgentLoop) logDebug(format string, args ...interface{}) {
	if a.debugLog == nil {
		return
	}
	fmt.Fprintf(a.debugLog, format, args...)
	a.debugLog.Sync()
}

// newHTTPClient creates an HTTP client with sensible timeouts to prevent
// the agent from hanging on slow or unresponsive providers.
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 300 * time.Second, // overall request timeout
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second, // connection timeout
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 300 * time.Second, // time to read headers
		},
	}
}

// NewAgentLoop creates a new agent loop.
func NewAgentLoop(cfg Config, registry *tools.Registry, useStream bool) (*AgentLoop, error) {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set (or use --api-key)")
	}

	opts := []option.RequestOption{
		option.WithHeader("Authorization", "Bearer "+apiKey),
		option.WithHTTPClient(newHTTPClient()),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	// Resolve model alias (may add [1m] suffix) and build beta headers
	model := cfg.Model
	if resolved, _ := ResolveModelAlias(model); resolved != "" {
		model = resolved
	}
	// Add anthropic-beta headers for API features (prompt caching, 1M context, etc.)
	betas := BuildBetaHeaders(model)
	if len(betas) > 0 {
		opts = append(opts, option.WithHeader("anthropic-beta", strings.Join(betas, ",")))
	}

	client := anthropic.NewClient(opts...)

	ctx := NewConversationContext(cfg)
	// Initialize transcript writer first to get sessionID
	sessionID := time.Now().Format("20060102-150405")
	// Initialize tool result disk persistence store with session-scoped directory
	if cfg.ProjectDir != "" {
		ctx.SetToolResultStore(NewToolResultStore(cfg.ProjectDir, sessionID))
		ctx.SetContentReplacementState(NewContentReplacementState())
	}

	transcriptDir := filepath.Join(".claude", "transcripts")
	tw := transcript.NewWriter(sessionID, filepath.Join(transcriptDir, sessionID+".jsonl"))
	_ = tw.Write(transcript.Entry{Type: "system", Content: fmt.Sprintf("model=%s, mode=%s", cfg.Model, cfg.PermissionMode)})

	maxTurns := cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = math.MaxInt
	}

	agent := &AgentLoop{
		config:              cfg,
		registry:            registry,
		gate:                nil, // set below to point to agent.config
		context:             ctx,
		client:              client,
		snapshots:           cfg.FileHistory,
		transcript:          tw,
		skillTracker:        cfg.SkillTracker,
		compactor:           NewCompactor(),
		useStream:           useStream,
		maxToolChars:        8000,
		toolTimeoutMs:       600000, // 10 minutes
		maxTurns:            maxTurns,
		budget:              NewIterationBudget(maxTurns),
		taskStore:           NewTaskStore(),
		agentTaskStore:      tools.NewAgentTaskStore(),
		notificationChan:    make(chan string, 64),
		evictionDone:        make(chan struct{}),
		agentNameRegistry:   make(map[string]string),
		agentHandleStore:    tools.NewAgentHandleStore(),
		workTaskStore:       NewWorkTaskStore(),
		agentOutput:         os.Stderr,
		toolStateTracker:    NewToolStateTracker(),
		todoList:            tools.NewTodoList(),
		cachedMC:            NewCachedMicrocompactTracker(),
		costTracker:         NewCostTracker(),
		cacheBreakDetector:  &CacheBreakDetector{},
		extractionState:     NewExtractionState(),
		hooks:               cfg.Hooks,
		shellHooks:          LoadAllHooks(cfg.ProjectDir),
		sonnetModel:         getDefaultSonnetModel(),
		errorReporter:       NewErrorReporter(),
		announcedMCPServers: make(map[string]bool),
		telemetry:           NewTelemetryManager(cfg.TelemetryDisabled),
	}
	// Latch beta headers for session stability — once set, stays same for the
	// entire session to prevent mid-session anthropic-beta header churn.
	// Upstream: sticky-on latch pattern (claude.ts:1456-1507).
	agent.betaHeadersLatched = append([]string(nil), betas...)
	// Initialize model capabilities cache and wire it globally
	agent.modelCapabilities = NewModelCapabilitiesCacheDefault()
	SetGlobalModelCapabilities(agent.modelCapabilities)
	// Update compactor's max tokens based on model context window
	contextWindow := agent.modelCapabilities.GetContextWindow(cfg.Model, cfg.MaxContextTokens)
	agent.compactor.SetMaxTokens(int(contextWindow))
	// Initialize proactive budget manager
	agent.budgetManager = NewProactiveBudgetManager(int(contextWindow))
	agent.redundantCallDetector = NewRedundantCallDetector()
	agent.stormBreaker = NewStormBreaker()
	if cfg.ProjectDir != "" {
		agent.truncatedResultSaver = NewTruncatedResultSaver(cfg.ProjectDir)
	}
	agent.consecutiveCallTracker = NewConsecutiveCallTracker()
	agent.toolListFingerprint = NewToolListFingerprint()
	agent.prefixFingerprint = NewPrefixFingerprint()
	agent.foldSummaryPin = NewFoldSummaryPin()
	agent.cacheMetrics = NewCacheMetrics()
	agent.readTracker = NewReadTracker()
	// Initialize currentMaxTokens from config
	agent.currentMaxTokens.Store(int64(cfg.MaxOutputTokens))
	// Fix gate to point to agent's config (not the local cfg copy)
	agent.gate = NewPermissionGate(&agent.config)

	// Wire auto mode classifier if enabled
	if cfg.AutoClassifierEnabled && cfg.PermissionMode == ModeAuto {
		classifierModel := cfg.AutoClassifierModel
		if classifierModel == "" {
			classifierModel = cfg.Model // default: same as main model
		}
		classifier := NewAutoModeClassifier(apiKey, cfg.BaseURL, classifierModel)
		classifier.SetClaudeMd(LoadProjectInstructions(cfg.ProjectDir))
		agent.gate.WithClassifier(classifier)
		agent.gate.WithTranscriptSource(agent.context)
		if classifier.IsEnabled() {
			fmt.Fprintf(os.Stderr, "  [auto-classifier] enabled (model=%s)\n", classifierModel)
		} else {
			fmt.Fprintf(os.Stderr, "  [auto-classifier] disabled (no API key or model)\n")
		}
	}

	// Start grace eviction ticker: clean up completed tasks after 30s
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if agent.taskStore != nil {
					agent.taskStore.CleanupEvicted()
				}
			case <-agent.evictionDone:
				return
			}
		}
	}()

	if cfg.cachedPrompt != nil {
		sysPrompt := cfg.cachedPrompt.GetOrBuild(registry, string(cfg.PermissionMode), "", cfg.Model, cfg.SkillLoader, cfg.SkillTracker, cfg.SessionMemory)
		ctx.SetSystemPrompt(sysPrompt)
	} else {
		sysPrompt := BuildSystemPrompt(registry, string(cfg.PermissionMode), "", cfg.Model, cfg.SkillLoader, cfg.SkillTracker, cfg.SessionMemory)
		ctx.SetSystemPrompt(sysPrompt)
	}

	// Inject todo reminder as a system-injected user message (not system prompt)
	// to keep the system prompt fully static and cacheable.
	if reminder := agent.todoList.BuildReminder(); reminder != "" {
		ctx.InjectTodoReminder(reminder)
	}

	// Register the sub-agent tool (wires AgentTool.SpawnFunc to this loop's SpawnSubAgent)
	agent.registerAgentTool()
	agent.registerSendMessageTool()
	agent.registerTodoWriteTool()
	agent.registerTaskOutputTool()
	agent.registerTaskStopTool()
	agent.registerWorkTaskTools()
	agent.registerBashBgTool()
	agent.registerAgentManagementTools()
	agent.registerAskUserQuestionTool()
	agent.registerPlanModeTools()

	// Wire LispGuideTool to the built-in LLM client.
	if lg, ok := agent.registry.Get("lisp_guide"); ok {
		if lg, ok := lg.(*tools.LispGuideTool); ok {
			lg.LLMCall = func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
				params := anthropic.MessageNewParams{
					Model:     GetModelForAPI(agent.config.Model),
					MaxTokens: int64(maxTokens),
					Messages: []anthropic.MessageParam{
						anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
					},
					System: []anthropic.TextBlockParam{
						{Text: systemPrompt},
					},
				}
				resp, err := agent.client.Messages.New(ctx, params)
				if err != nil {
					return "", err
				}
				var sb strings.Builder
				for _, block := range resp.Content {
					if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
						sb.WriteString(tb.Text)
					}
				}
				return sb.String(), nil
			}
		}
	}

	// Wire ToolSearchTool to the registry so it can look up tools at runtime.
	if tst, ok := agent.registry.Get("tool_search"); ok {
		if tst, ok := tst.(*tools.ToolSearchTool); ok {
			tst.Registry = agent.registry
		}
	}

	// Open debug log file for diagnostic messages (not printed to console)
	if logDir, err := filepath.Abs(filepath.Join(cfg.ProjectDir, ".claude")); err == nil {
		os.MkdirAll(logDir, 0o755)
		if f, err := os.OpenFile(filepath.Join(logDir, "debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			agent.debugLog = f
		}
	}

	return agent, nil
}

// NewAgentLoopFromTranscript creates an agent loop from an existing transcript file.
// If continueTranscript is true, new messages are appended to the original file
// instead of creating a new session transcript.
func NewAgentLoopFromTranscript(cfg Config, registry *tools.Registry, useStream bool, transcriptPath string, continueTranscript bool) (*AgentLoop, error) {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set")
	}

	opts := []option.RequestOption{
		option.WithHeader("Authorization", "Bearer "+apiKey),
		option.WithHTTPClient(newHTTPClient()),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	// Resolve model alias and add beta headers (same as NewAgentLoop)
	model := cfg.Model
	if resolved, _ := ResolveModelAlias(model); resolved != "" {
		model = resolved
	}
	betas := BuildBetaHeaders(model)
	if len(betas) > 0 {
		opts = append(opts, option.WithHeader("anthropic-beta", strings.Join(betas, ",")))
	}

	client := anthropic.NewClient(opts...)

	gate := NewPermissionGate(&cfg)

	// Generate session ID for tool result store (needed before rebuildContextFromTranscript)
	sessionID := time.Now().Format("20060102-150405")

	// Read transcript and rebuild context
	tr := transcript.NewReader(transcriptPath)
	entries, err := tr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read transcript: %w", err)
	}

	convCtx := rebuildContextFromTranscript(entries, cfg, sessionID)

	maxTurns := cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = math.MaxInt
	}

	// Create transcript writer: continue original file or start a new session
	var tw *transcript.Writer
	if continueTranscript {
		tw = transcript.NewWriterFromExisting(transcriptPath)
	} else {
		sessionID := time.Now().Format("20060102-150405")
		transcriptDir := filepath.Join(".claude", "transcripts")
		tw = transcript.NewWriter(sessionID, filepath.Join(transcriptDir, sessionID+".jsonl"))
		_ = tw.Write(transcript.Entry{Type: "system", Content: fmt.Sprintf("model=%s, mode=%s", cfg.Model, cfg.PermissionMode)})
		_ = tw.Write(transcript.Entry{Type: "user", Content: fmt.Sprintf("Resumed from %s (%d messages restored)", transcriptPath, len(entries))})
	}

	agent := &AgentLoop{
		config:              cfg,
		registry:            registry,
		gate:                gate,
		context:             convCtx,
		client:              client,
		snapshots:           cfg.FileHistory,
		transcript:          tw,
		skillTracker:        cfg.SkillTracker,
		compactor:           NewCompactor(),
		useStream:           useStream,
		maxToolChars:        8000,
		toolTimeoutMs:       600000, // 10 minutes
		maxTurns:            maxTurns,
		budget:              NewIterationBudget(maxTurns),
		taskStore:           NewTaskStore(),
		agentTaskStore:      tools.NewAgentTaskStore(),
		notificationChan:    make(chan string, 64),
		evictionDone:        make(chan struct{}),
		agentNameRegistry:   make(map[string]string),
		agentHandleStore:    tools.NewAgentHandleStore(),
		workTaskStore:       NewWorkTaskStore(),
		agentOutput:         os.Stderr,
		toolStateTracker:    NewToolStateTracker(),
		todoList:            tools.NewTodoList(),
		cachedMC:            NewCachedMicrocompactTracker(),
		costTracker:         NewCostTracker(),
		cacheBreakDetector:  &CacheBreakDetector{},
		extractionState:     NewExtractionState(),
		hooks:               cfg.Hooks,
		shellHooks:          LoadAllHooks(cfg.ProjectDir),
		errorReporter:       NewErrorReporter(),
		announcedMCPServers: make(map[string]bool),
		telemetry:           NewTelemetryManager(cfg.TelemetryDisabled),
	}
	// Latch beta headers for session stability — once set, stays same for the
	// entire session to prevent mid-session anthropic-beta header churn.
	// Upstream: sticky-on latch pattern (claude.ts:1456-1507).
	agent.betaHeadersLatched = append([]string(nil), betas...)
	// Initialize model capabilities cache and wire it globally
	agent.modelCapabilities = NewModelCapabilitiesCacheDefault()
	SetGlobalModelCapabilities(agent.modelCapabilities)
	// Update compactor's max tokens based on model context window
	contextWindow := agent.modelCapabilities.GetContextWindow(cfg.Model, cfg.MaxContextTokens)
	agent.compactor.SetMaxTokens(int(contextWindow))
	// Initialize proactive budget manager
	agent.budgetManager = NewProactiveBudgetManager(int(contextWindow))
	agent.redundantCallDetector = NewRedundantCallDetector()
	agent.stormBreaker = NewStormBreaker()
	if cfg.ProjectDir != "" {
		agent.truncatedResultSaver = NewTruncatedResultSaver(cfg.ProjectDir)
	}
	agent.consecutiveCallTracker = NewConsecutiveCallTracker()
	agent.toolListFingerprint = NewToolListFingerprint()
	agent.prefixFingerprint = NewPrefixFingerprint()
	agent.foldSummaryPin = NewFoldSummaryPin()
	agent.cacheMetrics = NewCacheMetrics()
	agent.readTracker = NewReadTracker()

	// Restore skill state from transcript entries so skillTracker reflects
	// which skills were already read in this session. This ensures skills
	// survive multiple compaction cycles after resume (matching upstream's
	// restoreSkillStateFromMessages).
	restoreSkillStateFromEntries(agent.skillTracker, entries)

	// Initialize currentMaxTokens from config
	agent.currentMaxTokens.Store(int64(cfg.MaxOutputTokens))

	// Fix gate to point to agent's config
	agent.gate = NewPermissionGate(&agent.config)

	// Wire auto mode classifier if enabled
	if cfg.AutoClassifierEnabled && cfg.PermissionMode == ModeAuto {
		classifierModel := cfg.AutoClassifierModel
		if classifierModel == "" {
			classifierModel = cfg.Model
		}
		classifier := NewAutoModeClassifier(apiKey, cfg.BaseURL, classifierModel)
		classifier.SetClaudeMd(LoadProjectInstructions(cfg.ProjectDir))
		agent.gate.WithClassifier(classifier)
		agent.gate.WithTranscriptSource(agent.context)
	}

	// Start grace eviction ticker: clean up completed tasks after 30s
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if agent.taskStore != nil {
					agent.taskStore.CleanupEvicted()
				}
			case <-agent.evictionDone:
				return
			}
		}
	}()

	if cfg.cachedPrompt != nil {
		sysPrompt := cfg.cachedPrompt.GetOrBuild(registry, string(cfg.PermissionMode), "", cfg.Model, cfg.SkillLoader, cfg.SkillTracker, cfg.SessionMemory)
		convCtx.SetSystemPrompt(sysPrompt)
	} else {
		sysPrompt := BuildSystemPrompt(registry, string(cfg.PermissionMode), "", cfg.Model, cfg.SkillLoader, cfg.SkillTracker, cfg.SessionMemory)
		convCtx.SetSystemPrompt(sysPrompt)
	}

	// Register the sub-agent tools
	agent.registerAgentTool()
	agent.registerSendMessageTool()
	agent.registerTodoWriteTool()
	agent.registerTaskOutputTool()
	agent.registerTaskStopTool()
	agent.registerWorkTaskTools()
	agent.registerBashBgTool()
	agent.registerAgentManagementTools()
	agent.registerAskUserQuestionTool()
	agent.registerPlanModeTools()

	// Wire LispGuideTool to the built-in LLM client.
	if lg, ok := agent.registry.Get("lisp_guide"); ok {
		if lg, ok := lg.(*tools.LispGuideTool); ok {
			lg.LLMCall = func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
				params := anthropic.MessageNewParams{
					Model:     GetModelForAPI(agent.config.Model),
					MaxTokens: int64(maxTokens),
					Messages: []anthropic.MessageParam{
						anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
					},
					System: []anthropic.TextBlockParam{
						{Text: systemPrompt},
					},
				}
				resp, err := agent.client.Messages.New(ctx, params)
				if err != nil {
					return "", err
				}
				var sb strings.Builder
				for _, block := range resp.Content {
					if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
						sb.WriteString(tb.Text)
					}
				}
				return sb.String(), nil
			}
		}
	}

	// Wire ToolSearchTool to the registry so it can look up tools at runtime.
	if tst, ok := agent.registry.Get("tool_search"); ok {
		if tst, ok := tst.(*tools.ToolSearchTool); ok {
			tst.Registry = agent.registry
		}
	}

	// Initialize prevTurnTokens from the rebuilt context so reactive compact
	// doesn't trigger a false positive on the first resumed turn.
	agent.prevTurnTokens = agent.context.EstimatedTokens()

	// Hook: OnResume — when a session is resumed from transcript
	if agent.hooks != nil {
		agent.hooks.ExecuteGenericHooksQuiet(HookOnResume, map[string]interface{}{
			"transcript_path":   transcriptPath,
			"restored_messages": len(entries),
			"continue_session":  continueTranscript,
		})
	}

	// Open debug log file for diagnostic messages (not printed to console)
	if logDir, err := filepath.Abs(filepath.Join(cfg.ProjectDir, ".claude")); err == nil {
		os.MkdirAll(logDir, 0o755)
		if f, err := os.OpenFile(filepath.Join(logDir, "debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			agent.debugLog = f
		}
	}

	return agent, nil
}

// rebuildContextFromTranscript rebuilds conversation context from transcript entries.
// It groups consecutive tool_use and tool_result entries correctly:
// - Multiple consecutive tool_use entries become one assistant message
// - Multiple consecutive tool_result entries become one user message
func rebuildContextFromTranscript(entries []transcript.Entry, cfg Config, sessionID string) *ConversationContext {
	ctx := NewConversationContext(cfg)
	if cfg.ProjectDir != "" {
		store := NewToolResultStore(cfg.ProjectDir, sessionID)
		ctx.SetToolResultStore(store)
		ctx.SetContentReplacementState(NewContentReplacementState())
	}

	var pendingToolUses []map[string]any
	var pendingToolResults []anthropic.ToolResultBlockParam

	flushToolUses := func() {
		if len(pendingToolUses) > 0 {
			ctx.AddAssistantToolCalls(pendingToolUses)
			pendingToolUses = nil
		}
	}
	flushToolResults := func() {
		if len(pendingToolResults) > 0 {
			ctx.AddToolResults(pendingToolResults)
			pendingToolResults = nil
		}
	}

	for _, entry := range entries {
		switch entry.Type {
		case "user":
			flushToolResults()
			flushToolUses()
			ctx.AddUserMessage(entry.Content)

		case "assistant":
			flushToolResults()
			flushToolUses()
			if entry.Content != "" {
				ctx.AddAssistantText(entry.Content)
			}

		case "tool_use":
			if entry.ToolID != "" && entry.ToolName != "" {
				input := entry.ToolArgs
				if input == nil {
					input = make(map[string]any)
				}
				pendingToolUses = append(pendingToolUses, map[string]any{
					"id":    entry.ToolID,
					"name":  entry.ToolName,
					"input": input,
				})
			}

		case "tool_result":
			// Flush pending tool uses first
			flushToolUses()
			if entry.ToolID != "" {
				pendingToolResults = append(pendingToolResults, anthropic.ToolResultBlockParam{
					ToolUseID: entry.ToolID,
					Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: entry.Content}}},
				})
			}

		case "compact":
			// Compaction boundary: discard all pending tool state.
			// The summary replaces the compacted messages, so pending
			// tool_uses/tool_results from before compaction are orphaned.
			pendingToolUses = nil
			pendingToolResults = nil
			// Extract token count from compact message if available
			preTokens := 0
			if entry.ToolArgs != nil {
				if tokens, ok := entry.ToolArgs["pre_compact_tokens"].(float64); ok {
					preTokens = int(tokens)
				}
			}
			ctx.AddCompactBoundary(CompactTriggerAuto, preTokens)
			continue

		case "summary":
			// Add the summary as a user-role message after the boundary
			if entry.Content != "" {
				ctx.AddSummary(entry.Content)
			}
			continue

		case "system", "error":
			flushToolResults()
			flushToolUses()
			// Skip system and error entries
		}
	}

	// Flush any remaining pending items
	flushToolUses()
	flushToolResults()

	// Fix any inconsistencies from interrupted sessions:
	// - Orphaned tool_use without matching tool_result
	// - Orphaned tool_result without matching tool_use
	// - Consecutive same-role messages (breaks Anthropic API)
	ctx.ValidateToolPairing()
	ctx.FixRoleAlternation()

	// Detect turn interruption and apply resume logic.
	// Matching upstream's deserializeMessagesWithInterruptDetection + detectTurnInterruption
	// (conversationRecovery.ts:164-333).
	interruptionState := ctx.DetectTurnInterruption()
	if interruptionState.Kind != TurnInterruptedNone {
		ctx.ApplyTurnInterruptionResume(interruptionState)
		// Re-validate after injection
		ctx.ValidateToolPairing()
		ctx.FixRoleAlternation()
	}

	return ctx
}

// restoreSkillStateFromEntries scans transcript entries for skill-related events
// and restores the skill tracker's state on resume. This mirrors upstream's
// restoreSkillStateFromMessages: it extracts read_skill invocations and
// skill_listing attachments to restore skill read/shown state so skills survive
// multiple compaction cycles after resume.
func restoreSkillStateFromEntries(skillTracker *skills.SkillTracker, entries []transcript.Entry) {
	if skillTracker == nil {
		return
	}
	for _, entry := range entries {
		// Track read_skill tool invocations: these indicate skills that were read.
		// On resume, we restore the "read" state so the next compaction can
		// re-inject skill content. The "shown" state is also restored so the
		// discovery reminder isn't re-triggered for skills already announced.
		if entry.Type == "tool_use" && entry.ToolName == "read_skill" {
			if args, ok := entry.ToolArgs["name"].(string); ok && args != "" {
				skillTracker.MarkShown(args) // mark as shown so it's not re-announced
				skillTracker.MarkRead(args)  // mark as read so it's re-injected on next compact
			}
		}
		// Also check for skill_listing attachment text to suppress re-announcement.
		// These are injected as "[Post-compact skill recovery: <name>]" patterns.
		if entry.Type == "user" && strings.Contains(entry.Content, "[Post-compact skill recovery:") {
			// Extract skill name from the pattern
			lines := strings.Split(entry.Content, "\n")
			if len(lines) > 0 {
				line := lines[0]
				// Format: "[Post-compact skill recovery: <name>]"
				start := strings.Index(line, "[Post-compact skill recovery: ")
				if start >= 0 {
					name := line[start+len("[Post-compact skill recovery: "):]
					if end := strings.Index(name, "]"); end >= 0 {
						name = name[:end]
					}
					if name != "" {
						skillTracker.MarkShown(name)
						skillTracker.MarkRead(name)
					}
				}
			}
		}
	}
}

// SetInterrupted sets or clears the interrupted flag.
func (a *AgentLoop) SetInterrupted(v bool) {
	a.interrupted.Store(v)
}

// IsInterrupted returns true if the loop has been interrupted.
func (a *AgentLoop) IsInterrupted() bool {
	return a.interrupted.Load()
}

// IsStreaming returns true if streaming mode is enabled.
func (a *AgentLoop) IsStreaming() bool {
	return a.useStream
}

// fireStopHook fires HookStop with session metadata. Call at every Run() exit point.
func (a *AgentLoop) fireStopHook(reason string, turnsUsed int, interrupted bool) {
	a.logDebug("[session] Run ended: reason=%s turns=%d in=%d out=%d cache_read=%d cache_write=%d int=%v\n",
		reason, turnsUsed, a.totalInputTokens.Load(), a.totalOutputTokens.Load(),
		a.totalCacheReadTokens.Load(), a.totalCacheCreationTokens.Load(), interrupted)
	if a.hooks == nil {
		return
	}
	a.hooks.ExecuteGenericHooksQuiet(HookStop, map[string]interface{}{
		"reason":                      reason,
		"model":                       a.config.Model,
		"turns":                       turnsUsed,
		"interrupted":                 interrupted,
		"total_input_tokens":          a.totalInputTokens.Load(),
		"total_output_tokens":         a.totalOutputTokens.Load(),
		"last_api_input_tokens":       a.lastAPIInputTokens.Load(),
		"total_cache_creation_tokens": a.totalCacheCreationTokens.Load(),
		"total_cache_read_tokens":     a.totalCacheReadTokens.Load(),
		"total_cache_edits_deletions": a.totalCacheEditsDeletions.Load(),
		"remaining_token_budget":      a.RemainingTokenBudget(),
	})
}

// TranscriptPath returns the path to the current transcript file.
func (a *AgentLoop) TranscriptPath() string {
	if a.transcript == nil {
		return ""
	}
	return a.transcript.FilePath()
}

// extractConclusions scans assistant text for stated conclusions and records them.
// This helps the agent remember key findings across turns without relying on
// its own unreliable extraction from the conversation history.
// Pre-compiled conclusion extraction patterns. These capture concrete facts
// from assistant text that are worth preserving across compaction.
// Patterns focus on task progress and semantic conclusions, not just code structure.
var conclusionPatterns = []*regexp.Regexp{
	// Code structure conclusions
	regexp.MustCompile(`(?i)(?:defined in|defined at)\s+([^\s.,]+)`),
	regexp.MustCompile(`(?i)(?:returns?|yields?)\s+([^\s.,]+)`),
	regexp.MustCompile(`(?i)(?:uses?|calls?)\s+([^\s.,]+)\s+for\s+`),
	regexp.MustCompile(`(?i)(?:is defined as|is an?)\s+([^\s.,]+)`),
	// File/function semantic conclusions
	regexp.MustCompile(`(?i)(?:the file|file)\s+([^\s]+)\s+(?:contains|has|defines)\s+(.{10,80})`),
	regexp.MustCompile(`(?i)(?:the function|func)\s+([^\s(]+)\s+(?:does|implements|handles)\s+(.{10,80})`),
	regexp.MustCompile(`(?i)([^\s]+)\s+depends\s+on\s+([^\s]+)`),
	// Task progress conclusions
	regexp.MustCompile(`(?i)(?:completed|finished|done with)\s+(.{10,80})`),
	regexp.MustCompile(`(?i)(?:we need to|must|should)\s+(.{10,80})`),
	regexp.MustCompile(`(?i)(?:next step|proceed with|continue with)\s+(.{10,80})`),
	// Bug/fix conclusions
	regexp.MustCompile(`(?i)(?:the root cause|the bug|the issue)\s+(?:was|is|in|at)\s+(.{10,80})`),
	regexp.MustCompile(`(?i)(?:the fix|the solution|fix|workaround)\s+(?:was|is|to)\s+(.{10,80})`),
	// Error conclusions
	regexp.MustCompile(`(?i)(?:error|failed|failure)[: ]\s*(.{10,120})`),
	// Discovery conclusions
	regexp.MustCompile(`(?i)(?:found|discovered|identified)\s+(?:that\s+)?(.{10,80})`),
	regexp.MustCompile(`(?i)(?:the result|output|value)\s+(?:is|was)\s+(.{10,80})`),
	regexp.MustCompile(`(?i)(?:note|important|key|critical)[: ]\s*(.{10,80})`),
}

func (a *AgentLoop) extractConclusions(text string) {
	for _, re := range conclusionPatterns {
		matches := re.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if len(m) > 1 && len(m[1]) > 3 {
				a.toolStateTracker.RecordConclusion(m[1])
			}
		}
	}
}

func (a *AgentLoop) interruptCtx(baseCtx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(baseCtx, timeout)

	// Watch for interrupt flag and cancelCtx in background
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if a.IsInterrupted() {
					cancel()
					return
				}
			}
		}
	}()

	// Also watch cancelCtx (for sub-agent Kill from parent)
	if a.cancelCtx != nil {
		go func() {
			select {
			case <-ctx.Done():
				return
			case <-a.cancelCtx.Done():
				cancel()
			}
		}()
	}

	return ctx, cancel
}

// interruptibleSleep sleeps for the given duration but returns early
// if the agent is interrupted (Ctrl+C). Returns true if interrupted.
func (a *AgentLoop) interruptibleSleep(d time.Duration) bool {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	elapsed := time.Duration(0)
	for elapsed < d {
		select {
		case <-ticker.C:
			if a.IsInterrupted() {
				return true
			}
			if elapsed+100*time.Millisecond > d {
				return false
			}
			elapsed += 100 * time.Millisecond
		}
	}
	return false
}

// Run processes a user message through the agent loop, returning the final text response.
func (a *AgentLoop) Run(userMessage string) string {
	// Clear any stale interrupted flag from previous run
	a.SetInterrupted(false)

	// Reset the turn budget so each new conversation starts fresh
	a.budget = NewIterationBudget(a.maxTurns)
	a.lastDeltasState = DeltasStateNone // reset streaming state

	// Track task-scoped iteration start (openclacky pattern): records the
	// budget consumption and skill read count at task start, used by the
	// post-run skill evolution and memory updater to compute task-local
	// iteration counts (rather than session-cumulative).
	a.taskStartTurns = a.budget.Consumed()
	if a.skillTracker != nil {
		a.taskStartReadSkillCount = a.skillTracker.ReadCount()
	}

	a.logDebug("[session] Run started: model=%s mode=%s maxTurns=%d stream=%v input=%dchars\n",
		a.config.Model, a.config.PermissionMode, a.maxTurns, a.useStream, len(userMessage))

	// Expand @ context references (e.g., @file:main.go, @diff)
	cwd, _ := os.Getwd()

	// Hook: PreUserMessage — allows hooks to inspect/modify incoming user message
	if a.hooks != nil {
		a.hooks.ExecuteGenericHooksQuiet(HookPreUserMessage, map[string]interface{}{
			"message": userMessage,
		})
	}

	var contextWindow int
	if a.modelCapabilities != nil {
		contextWindow = int(a.modelCapabilities.GetContextWindow(a.config.Model, a.config.MaxContextTokens))
	} else {
		contextWindow = modelContextWindow(a.config.Model)
	}
	if contextWindow < 1 {
		contextWindow = 200_000
	}
	expanded := PreprocessContextReferences(userMessage, cwd, contextWindow)
	if expanded.Expanded && !expanded.Blocked {
		userMessage = expanded.Message
	} else if len(expanded.Warnings) > 0 {
		// Log warnings even if blocked
		for _, w := range expanded.Warnings {
			a.out("[WARN] %s\n", w)
		}
	}

	a.context.AddUserMessage(userMessage)
	// Track change for cache break detection
	a.cacheBreakDetector.RecordChange(CacheChangeUserMessage, 1)

	// Openclacky pattern: adaptive prompting — inject task-specific instructions
	// based on detected task type (debug/refactor/create/search). This improves
	// output quality without modifying the system prompt or breaking cache.
	a.context.InjectAdaptiveInstructions(userMessage)
	if a.transcript != nil {
		_ = a.transcript.WriteUser(userMessage)
	}

	// Idle auto-compact (openclacky pattern): if the user has been idle for
	// a while (>10 min since last API call), run a lightweight micro-compact
	// to clear old tool results before processing the new message. This
	// prevents context bloat across long-lived sessions where the user walks
	// away and comes back later.
	if !a.lastApiCompletionTime.IsZero() && time.Since(a.lastApiCompletionTime) > 10*time.Minute {
		a.logDebug("[idle-compact] Last API was %v ago, running micro-compact before processing\n", time.Since(a.lastApiCompletionTime))
		a.runPerTurnMicroCompact()
	}

	// Hook: PostUserMessage — after user message is added to context
	if a.hooks != nil {
		a.hooks.ExecuteGenericHooksQuiet(HookPostUserMessage, map[string]interface{}{
			"message": userMessage,
		})
	}

	var finalText string

	// Recovery state (mirrors ggbot's State machine)
	contextErrors := 0
	const maxContextRecovery = 3 // Phase 1: truncate, Phase 2: aggressive truncate, Phase 3: give up

	// Empty response tracking -- prevents infinite loops on thinking-only responses
	consecutiveEmptyResponses := 0
	const maxEmptyResponses = 3

	// Preflight compression: turn-start fold estimation (DeepSeek-Reasonix pattern)
	// If estimated tokens > 90% of ctxMax, fold BEFORE making the API call
	// to prevent wasting cached tokens on a request that will exceed limits.
	preflightCtxMax := int(a.modelCapabilities.GetContextWindow(a.config.Model, a.config.MaxContextTokens))
	preflightRatio := a.context.EstimatedTokenRatio(preflightCtxMax)
	if preflightRatio > TURN_START_FOLD_THRESHOLD {
		a.logDebug("[compact] turn-start fold: est_tokens=%d, ctxMax=%d, ratio=%.2f > %.2f\n",
			a.context.EstimatedTokens(), preflightCtxMax, preflightRatio, TURN_START_FOLD_THRESHOLD)
		a.tryCompaction()
		a.RunPostCompactCleanup()
	}

	for a.budget.Consume() {
		a.lastTransition = TransitionNone // reset per-turn

		// Clear redundant call detector at turn boundaries
		if a.redundantCallDetector != nil {
			a.redundantCallDetector.Clear()
		}
		// Reset storm breaker window at turn boundaries
		if a.stormBreaker != nil {
			a.stormBreaker.Reset()
		}
		// Reset consecutive failure tracker at turn boundaries
		if a.consecutiveCallTracker != nil {
			a.consecutiveCallTracker.Clear()
		}

		// Per-turn micro-compact: clear old tool results before each API call.
		// Prevents context from growing unboundedly between full compactions,
		// matching upstream's per-turn microCompact call in query.ts.
		a.runPerTurnMicroCompact()

		// Check for cancelCtx (set by sub-agent Kill) at the start of each turn
		if a.cancelCtx != nil {
			select {
			case <-a.cancelCtx.Done():
				a.out("\n[WARN] Cancelled by parent.\n")
				a.fireStopHook("cancelled_by_parent", a.budget.Consumed(), false)
				return finalText
			default:
			}
		}

		// Check for interrupt at the start of each turn
		if a.IsInterrupted() {
			// Hook: OnAbort — when the agent is interrupted
			if a.hooks != nil {
				a.hooks.ExecuteGenericHooksQuiet(HookOnAbort, map[string]interface{}{
					"reason": "interrupt_at_turn_start",
				})
			}
			a.out("\n[WARN] Interrupted by user.\n")
			a.SetInterrupted(false) // reset for next request
			a.fireStopHook("interrupted", a.budget.Consumed(), true)
			return finalText
		}

		// Reactive compaction: check for token spike before proceeding.
		// If the token count has jumped significantly (e.g., large file read),
		// proactively compact before the context becomes too large.
		if a.config.ReactiveCompactEnabled {
			currentTokens := a.context.EstimatedTokens()
			threshold := a.config.ReactiveCompactThreshold
			if threshold <= 0 {
				threshold = 5000
			}
			if result := CheckReactiveCompact(currentTokens, a.prevTurnTokens, threshold); result != nil {
				a.logDebug("\n[reactive-compact] Token spike detected: %d -> %d (delta=%d, threshold=%d)\n",
					a.prevTurnTokens, currentTokens, result.TokenDelta, threshold)
				a.tryCompaction()
				a.consecutiveContextErrors = 0 // reset after successful compaction
			}
			// Update previous token count for next turn
			a.prevTurnTokens = a.context.EstimatedTokens()
		}

		// Rebuild system prompt each turn to update skill discovery
		if a.config.cachedPrompt != nil {
			sysPrompt := a.config.cachedPrompt.GetOrBuild(a.registry, string(a.config.PermissionMode), "", a.config.Model, a.config.SkillLoader, a.skillTracker, a.config.SessionMemory)
			a.context.SetSystemPrompt(sysPrompt)
		} else if a.config.SkillLoader != nil {
			sysPrompt := BuildSystemPrompt(a.registry, string(a.config.PermissionMode), "", a.config.Model, a.config.SkillLoader, a.skillTracker, a.config.SessionMemory)
			a.context.SetSystemPrompt(sysPrompt)
		}

		// Update active task from the most recent user message. This prevents
		// "task drift" — the LLM losing track of what it was doing and jumping
		// back to old topics. The active task is injected into the system prompt
		// via BuildSessionStateNote.
		if a.toolStateTracker != nil {
			if latestUser := a.context.LatestUserMessage(); latestUser != "" {
				// Truncate to avoid very long messages from bloating the prompt
				task := latestUser
				if len(task) > 500 {
					task = task[:500] + "..."
				}
				a.toolStateTracker.SetActiveTask(task)
			}
		}

		// Inject session state as a system-injected user message (not system prompt)
		// to keep the system prompt fully static and cacheable.
		// This gives the agent visibility into what it has already done,
		// preventing redundant reads and searches.
		if a.toolStateTracker != nil {
			sessionState := a.toolStateTracker.BuildSessionStateNote()
			a.context.InjectSessionState(sessionState)
		}

		// Inject todo reminder as a system-injected user message (not system prompt)
		// to keep the system prompt fully static and cacheable.
		// The reminder message is skipped for cache breakpoint placement.
		if reminder := a.todoList.BuildReminder(); reminder != "" {
			a.context.InjectTodoReminder(reminder)
		}

		// Periodic TodoWrite idle reminder: if model hasn't used TodoWrite
		// for 10+ turns, inject a nudge to create/update task list.
		if a.todoList.IncrementTurn() {
			if a.todoList.BuildReminder() == "" {
				// No tasks exist and model is idle — nudge to use TodoWrite
				idleMsg := a.todoList.BuildIdleReminder()
				a.context.InjectIdleReminder(idleMsg)
			}
		}

		var toolCalls []map[string]any
		var textParts []string
		var err error

		// Hook: PreAPICall — before each API call
		if a.hooks != nil {
			a.hooks.ExecuteGenericHooksQuiet(HookPreAPICall, map[string]interface{}{
				"model":  a.config.Model,
				"stream": a.useStream,
			})
		}

		// Openclacky pattern: proactive budget management — inject a behavioral hint
		// when context window usage approaches thresholds, preventing hard 400 errors.
		if a.budgetManager != nil {
			estTokens := a.context.EstimatedTokens()
			if hint := a.budgetManager.BudgetHint(estTokens); hint != "" {
				a.context.AddUserMessage(hint)
			}
		}

		// Streaming vs non-streaming decision
		streamingExecDone := false       // set true when streaming executor handled tool calls
		toolCallsAddedToContext := false // tracks if AddAssistantToolCalls was already called
		if a.useStream {
			// Create streaming tool executor for pipelined tool execution.
			// Tools start executing as their content blocks complete during streaming,
			// overlapping with remaining stream processing.
			toolCallDoneCh := make(chan int, 20)
			executor := NewStreamingToolExecutor(a.registry, a.gate, a.shellHooks, a.snapshots)

			toolCalls, textParts, err = a.callWithRetryAndFallbackStreaming(toolCallDoneCh, executor)

			// Close the channel to signal no more tool calls will arrive
			close(toolCallDoneCh)

			// If we got tool calls and have a streaming executor, wait for
			// pipelined execution to complete instead of synchronous execution.
			if len(toolCalls) > 0 && executor != nil {
				// CRITICAL: Add tool_use blocks to context BEFORE tool_results.
				// The Anthropic API requires tool_use to precede its tool_result.
				// If we add results first, BuildMessages produces user(tool_result)
				// before assistant(tool_use), causing API error 2013.
				a.context.AddAssistantToolCalls(toolCalls)
				toolCallsAddedToContext = true // streaming path already added tool_use

				waitCtx, waitCancel := a.interruptCtx(context.Background(), 5*time.Minute)
				streamingResults := executor.Wait(waitCtx, len(toolCalls))
				waitCancel()
				if len(streamingResults) > 0 {
					// Streaming executor completed all tool calls — use results directly
					streamingExecDone = true
					var toolResults []anthropic.ToolResultBlockParam
					for _, sr := range streamingResults {
						output := a.truncateOutput(sr.output)
						// Truncated result saver for streaming path
						if output != sr.output && a.truncatedResultSaver != nil {
							savedMsg := a.truncatedResultSaver.Save(sr.toolName, sr.output)
							if savedMsg != "" {
								output += "\n\n" + savedMsg
							}
						}
						// Openclacky patterns: UTF-8 sanitize + TODO reminder injection
						resultText := deepSanitizeUTF8(output).(string)
							resultText = injectTodoReminder("", resultText, a)
							toolResults = append(toolResults, anthropic.ToolResultBlockParam{
								ToolUseID: sr.toolUseID,
								Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: resultText}}},
								IsError:   anthropic.Bool(sr.isError),
						})
					}
					a.context.AddToolResults(toolResults)
					a.cacheBreakDetector.RecordChange(CacheChangeToolResult, len(toolResults))
				} else {
					// Streaming executor returned no results — remove the orphaned
					// tool_use entry we just added. Without matching tool_result,
					// the API will reject the next request with error 2013.
					a.context.RemoveLastAssistantEntry()
					toolCallsAddedToContext = false
				}
			} else {
				// Streaming succeeded with no tool calls (text-only response) or executor
				// was nil. No second API call needed — use the streaming result directly.
				// Previously this called callWithNonStreamingOnly() which made a redundant
				// duplicate second API call, causing double-thinking display and wasted tokens.
				_ = streamingExecDone // no-op for text-only streaming response
			}
		} else {
			// Non-streaming mode: call API synchronously and execute tools.
			toolCalls, textParts, err = a.callWithNonStreamingOnly()
		}

		// Hook: PostAPICall — after each API call (success or failure)
		if a.hooks != nil {
			a.hooks.ExecuteGenericHooksQuiet(HookPostAPICall, map[string]interface{}{
				"model":  a.config.Model,
				"stream": a.useStream,
				"error":  err,
			})
		}
		if err != nil {
			errMsg := err.Error()

			// Capture error in local reporter for later analysis
			if a.errorReporter != nil {
				a.errorReporter.CaptureError(errMsg, map[string]interface{}{
					"model":  a.config.Model,
					"stream": a.useStream,
				})
			}

			// Hook: OnError — when an API error occurs (with death spiral prevention)
			if a.hooks != nil {
				a.hooks.ExecuteGenericHooksQuiet(HookOnError, map[string]interface{}{
					"error": errMsg,
					"model": a.config.Model,
				})
			}

			// User interrupt -- return immediately
			if strings.Contains(errMsg, "interrupted by user") {
				// Hook: OnAbort — when the agent is interrupted
				if a.hooks != nil {
					a.hooks.ExecuteGenericHooksQuiet(HookOnAbort, map[string]interface{}{
						"reason": "user_interrupt",
					})
				}
				a.out("\n[WARN] Interrupted.\n")
				a.fireStopHook("interrupted", a.budget.Consumed(), true)
				return finalText
			}
			// Model confusion -- echoed tool syntax as text; recover by retrying
			// Model fallback triggered: continue with new model
			var fbErr *FallbackTriggeredError
			if errors.As(err, &fbErr) {
				a.out("\n[Fallback] %v -- continuing with %s\n", fbErr, fbErr.FallbackModel)
				a.logDebug("[fallback] model switched: %s -> %s\n", a.config.Model, fbErr.FallbackModel)
				a.lastTransition = TransitionModelFallback
				continue
			}
			// Upstream truncation: API router cut the response mid-tool_use.
				// Retry -- the truncation hint was already injected by parseResponse.
				if strings.Contains(errMsg, "upstream truncation") {
					a.out("\n[WARN] Upstream truncation detected, retrying...\n")
					a.lastTransition = TransitionTruncatedArgs
					continue
				}
			if strings.Contains(errMsg, "model confused") {
				a.out("\n[WARN] Model confused, retrying...\n")
				// Add a hint so the model doesn't repeat the same mistake
				a.context.AddUserMessage("ERROR: Your previous response was malformed. Do NOT output tool syntax as text. Use proper tool calls only.")
				a.lastTransition = TransitionModelConfused
				continue
			}
			// 2013 error: tool_result doesn't follow tool_call -- repair pairing before retry.
			// Cap consecutive repairs to prevent infinite loops. After 3 failed
			// repairs, the pairing issue is structural (e.g., BuildMessages merge
			// logic breaking what entries-level repair fixed) — force aggressive
			// compaction to break the cycle.
			if strings.Contains(errMsg, "2013") || strings.Contains(errMsg, "tool call result does not follow tool call") {
				a.consecutive2013Errors++
				a.logDebug("[2013] error recovery: consecutive=%d entries=%d\n", a.consecutive2013Errors, len(a.context.entries))
				if a.consecutive2013Errors > 3 {
					a.out("\n[ERR] Tool pairing error after %d repair attempts, giving up.\n", a.consecutive2013Errors-1)
					a.fireStopHook("tool_pairing_failed", a.budget.Consumed(), false)
					return finalText
				}
				a.out("\n[WARN] Tool pairing error (2013), repairing context... (attempt %d/3)\n", a.consecutive2013Errors)
				if a.toolStateTracker != nil {
					a.toolStateTracker.OnCompaction()
				}
				// Escalating repair: first two attempts validate entries,
				// only call MinimumHistory() as last resort (not on every retry).
				// Previous behavior called MinimumHistory() on every retry >1,
				// which was too aggressive and broke more pairings than it fixed.
				if a.consecutive2013Errors <= 2 {
					// Gentle repair: validate and fix role alternation only
					a.context.ValidateToolPairing()
					a.context.FixRoleAlternation()
				} else {
					// Last resort: aggressive but preserve more entries than before
					preTokens := a.context.EstimatedTokens()
					a.context.MinimumHistory()
					a.context.ValidateToolPairing()
					a.context.FixRoleAlternation()
					a.out("\n[WARN] 2013 repair: aggressive compaction applied (%d tokens)\n", preTokens)
				}
				// Inject a recovery hint so the model produces properly sequenced tool calls
				a.context.AddUserMessage("A tool call result was not properly paired with its call. Please ensure each tool_use block is immediately followed by its corresponding tool_result, with no extra assistant messages in between. Resume with your next action.")
				a.lastTransition = TransitionToolPairing
				continue
			}
			// Truncated tool arguments -- model cut off mid-tool-call
			if strings.Contains(errMsg, "truncated") || strings.Contains(errMsg, "incomplete JSON") {
				a.out("\n[WARN] Tool arguments truncated, injecting corrective hint...\n")
				a.context.AddUserMessage("ERROR: Your tool call arguments was cut off due to length limits. Do NOT repeat the truncated tool call. If you need to make multiple tool calls, make them one at a time with shorter arguments.")
				a.lastTransition = TransitionTruncatedArgs
				continue
			}
			// Stream stalled -- safety timeout fired; recover with truncation
			// Error withholding: suppress user-visible warnings until recovery exhausted
			if strings.Contains(errMsg, "stream stalled") {
				contextErrors++
				if contextErrors > maxContextRecovery {
					a.out("\n[ERR] Stream stalled after %d recovery attempts, giving up.\n", maxContextRecovery)
					a.fireStopHook("stream_stalled", a.budget.Consumed(), false)
					return finalText
				}
				if a.toolStateTracker != nil {
					a.toolStateTracker.OnCompaction()
				}
				preTokens := a.context.EstimatedTokens()
				if contextErrors <= 1 {
					a.context.TruncateHistory()
				} else if contextErrors <= 2 {
					a.context.AggressiveTruncateHistory()
				} else {
					a.context.MinimumHistory()
				}
				a.injectTruncationContinuation(preTokens)
				a.lastTransition = TransitionStreamStalled
				continue
			}
			if isContextLengthError(errMsg) {
				contextErrors++
				if contextErrors > maxContextRecovery {
					a.out("\n[ERR] Context length exceeded after %d recovery attempts, giving up.\n", maxContextRecovery)
					a.fireStopHook("context_length_exceeded", a.budget.Consumed(), false)
					return finalText
				}

				// Try precise token-gap parsing for reactive compaction.
				if a.config.ReactiveCompactEnabled {
					if overflowTokens, found := parseMaxTokensContextOverflowError(err); found {
						a.logDebug("\n[reactive-compact] Parsed context overflow: %d tokens over, shedding precisely...\n",
							overflowTokens)
						currentTokens := a.context.EstimatedTokens()
						safetyMargin := 5000
						targetTokens := currentTokens - overflowTokens - safetyMargin
						a.reactiveCompact(targetTokens)
						contextErrors = 0
						a.lastTransition = TransitionContextOverflow
						continue
					}
				}

				// Two-layer context overflow recovery (openclacky pattern).
				// Layer 1: Standard cache-preserving compact (pull back K=1 message,
				//   compact, re-append). Preserves prompt cache checkpoint #A.
				// Layer 2: Aggressive half-history compact (only if Layer 1 fails).
				//   Pulls back ~half the history, sacrificing cache but guaranteeing fit.
				twoLayerSucceeded := false
				if a.config.ReactiveCompactEnabled {
					preTokens := a.context.EstimatedTokens()
					a.out("\n[two-layer-recovery] Layer 1: standard cache-preserving compact (%d tokens)...\n", preTokens)
					preTwoLayer, attempted := a.StandardCachePreservingCompact()
					if attempted && a.context.EstimatedTokens() < preTwoLayer {
						a.consecutiveContextErrors = 0
						a.lastTransition = TransitionContextOverflow
						continue
					}
					if attempted {
						a.out("\n[two-layer-recovery] Layer 1 insufficient, escalating to Layer 2...\n")
						preTwoLayer2, attempted2 := a.AggressiveHalfHistoryCompact()
						if attempted2 && a.context.EstimatedTokens() < preTwoLayer2 {
							twoLayerSucceeded = true
							a.consecutiveContextErrors = 0
							a.lastTransition = TransitionContextOverflow
							continue
						}
					}
				}

				if !twoLayerSucceeded && a.toolStateTracker != nil {
					a.toolStateTracker.OnCompaction()
				}
				preTokens := a.context.EstimatedTokens()
				// Use reactive compact when enabled, falling back to truncation
				if a.config.ReactiveCompactEnabled {
					a.reactiveCompact(0) // 0 = compact aggressively
				} else {
					if contextErrors <= 1 {
						a.context.TruncateHistory()
					} else if contextErrors <= 2 {
						a.context.AggressiveTruncateHistory()
					} else {
						a.context.MinimumHistory()
					}
					a.injectTruncationContinuation(preTokens)
				}
				a.consecutiveContextErrors = 0
				a.lastTransition = TransitionContextExceeded
				continue
			}
			return fmt.Sprintf("API error: %v", err)
		}

		// Reset context error counter on successful API call
		contextErrors = 0
		a.consecutive2013Errors = 0

		// Openclacky pattern: proactive budget management — inject one-shot hint
		// when context window usage exceeds 75%%. This prevents hard 400 errors
		// by nudging the LLM to be more concise before the crisis point.
		if a.budgetManager != nil && !a.budgetHint75Injected {
			estTokens := a.context.EstimatedTokens()
			if a.budgetManager.ShouldProactiveCompact(estTokens) {
			a.budgetHint75Injected = true
				hint := a.budgetManager.BudgetHint(estTokens)
				a.context.InjectBudgetHint(hint)
			}
		}

		if len(textParts) > 0 {
			finalText = strings.Join(textParts, "\n")
		}

		if len(toolCalls) == 0 {
			// No tool calls -- could be a thinking-only response (model uses extended
			// thinking but hasn't produced text yet) or a genuine final answer.
			if len(textParts) == 0 {
				// No text and no tool calls -- thinking-only response
				consecutiveEmptyResponses++
				if consecutiveEmptyResponses >= maxEmptyResponses {
					a.out("\n[ERR] No actionable response after %d attempts, giving up\n", maxEmptyResponses)
					return fmt.Sprintf("Model returned no actionable response %d times in a row", maxEmptyResponses)
				}
				a.out("\n[WARN] No text/tool_use in response (attempt %d/%d), continuing...\n",
					consecutiveEmptyResponses, maxEmptyResponses)
				// Inject hint to encourage actual output
				a.context.AddUserMessage("Please continue and provide your response in text or use a tool.")
				a.lastTransition = TransitionEmptyResponse
				continue
			}
			// Genuine final answer with text
			consecutiveEmptyResponses = 0
			// No tool calls -- model gave final answer.
			// Like Claude Code's stop hooks: the loop could continue here
			// with additional checks (token budget, quality check, etc.)
			// but for now we simply exit.

			// Hook: PreAssistantMessage — before adding assistant text to context
			if a.hooks != nil {
				a.hooks.ExecuteGenericHooksQuiet(HookPreAssistantMessage, map[string]interface{}{
					"has_tools": false,
					"text_len":  len(finalText),
				})
			}

			a.context.AddAssistantText(finalText)
			if a.transcript != nil {
				_ = a.transcript.WriteAssistant(finalText, a.config.Model)
			}
			// Extract key findings from the final answer for next-turn reference
			if a.toolStateTracker != nil {
				a.extractConclusions(finalText)
			}

			// Hook: PostAssistantMessage — after assistant text is fully processed
			if a.hooks != nil {
				a.hooks.ExecuteGenericHooksQuiet(HookPostAssistantMessage, map[string]interface{}{
					"has_tools": false,
					"text_len":  len(finalText),
				})
			}

			break
		}

		// Reset empty response counter on successful tool call
		consecutiveEmptyResponses = 0

		// Hook: PreAssistantMessage — before adding assistant tool calls to context
		if a.hooks != nil {
			a.hooks.ExecuteGenericHooksQuiet(HookPreAssistantMessage, map[string]interface{}{
				"has_tools":  true,
				"tool_count": len(toolCalls),
				"text_len":   len(textParts),
			})
		}

		// Add tool_use blocks to context (skip if already added by streaming path)
		if !toolCallsAddedToContext {
			a.context.AddAssistantToolCalls(toolCalls)
		}

		// Extract conclusions from intermediate text before tool calls
		if a.toolStateTracker != nil && len(textParts) > 0 {
			for _, tp := range textParts {
				a.extractConclusions(tp)
			}
		}

		// Track read_skill usage for skill tracker
		if a.skillTracker != nil {
			for _, call := range toolCalls {
				if name, _ := call["name"].(string); name == "read_skill" {
					if input, ok := call["input"].(map[string]any); ok {
						if skillName, _ := input["name"].(string); skillName != "" {
							a.skillTracker.MarkRead(skillName)
							a.skillTracker.MarkUsed(skillName)
						}
					}
				}
			}
		}

		if !streamingExecDone {
			a.executeToolCallsConcurrent(toolCalls)
		}

		// Increment extraction tracking for tool calls used this turn
		if a.extractionState != nil && len(toolCalls) > 0 {
			for range toolCalls {
				a.extractionState.IncrementToolCall()
			}
		}

		// Session memory extraction: check if thresholds are met and run
		// forked agent to update session_memory.md with LLM extraction.
		// This matches upstream's extractSessionMemory hook pattern.
		if a.extractionState != nil && a.config.SessionMemory != nil {
			currentTokens := int64(a.context.EstimatedTokens())
			if a.extractionState.ShouldExtract(currentTokens, len(toolCalls) > 0) {
				go a.runSessionMemoryExtraction()
			}
		}

		// Update tool state tracker after tool execution
		if a.toolStateTracker != nil {
			for _, call := range toolCalls {
				name, _ := call["name"].(string)
				input, _ := call["input"].(map[string]any)
				if input == nil {
					continue
				}
				switch name {
				case "read_file":
					if path := extractFilePath(input); path != "" {
						a.toolStateTracker.RecordFileRead(path)
					}
				case "grep":
					if pattern, ok := input["pattern"].(string); ok {
						a.toolStateTracker.RecordSearch(pattern, true)
					}
				case "glob":
					if pattern, ok := input["pattern"].(string); ok {
						a.toolStateTracker.RecordSearch(pattern, true)
					}
				}
			}
		}

		// Hook: PostAssistantMessage — after assistant tool calls are fully processed
		if a.hooks != nil {
			a.hooks.ExecuteGenericHooksQuiet(HookPostAssistantMessage, map[string]interface{}{
				"has_tools":  true,
				"tool_count": len(toolCalls),
			})
		}

		// Between-turn drain: inject sub-agent completion notifications
		// into the conversation context (matching Claude Code's query.ts
		// between-turn drain pattern). This ensures the LLM sees
		// completed sub-agent results at the next tool-round boundary.
		if notifications := a.DrainNotifications(); len(notifications) > 0 {
			a.InjectNotifications(notifications)
		}

		// Between-turn drain: inject pending messages from parent agent
		// (e.g., messages sent via send_message tool). These are drained
		// at tool-round boundaries so the sub-agent can process them
		// without interrupting in-flight tool calls.
		if a.drainPendingMessagesFunc != nil {
			if pendingMsgs := a.drainPendingMessagesFunc(); len(pendingMsgs) > 0 {
				var sb strings.Builder
				sb.WriteString("[System: The parent agent sent the following messages while you were working]\n\n")
				for _, msg := range pendingMsgs {
					sb.WriteString(msg)
					sb.WriteString("\n\n")
				}
				a.context.AddUserMessage(sb.String())
			}
		}

		// Flush pending injections: deferred assistant+user shim pairs
		// that were enqueued during tool execution. Must happen after
		// tool results are appended to preserve tool_use/tool_result
		// pairing (avoids 2013 API errors).
		a.FlushPendingInjections()

		// Check for interrupt after tool execution
		if a.IsInterrupted() {
			// Hook: OnAbort — when the agent is interrupted after tool execution
			if a.hooks != nil {
				a.hooks.ExecuteGenericHooksQuiet(HookOnAbort, map[string]interface{}{
					"reason": "interrupt_after_tool",
				})
			}
			a.out("\n[WARN] Interrupted by user.\n")
			a.SetInterrupted(false)
			a.fireStopHook("interrupted", a.budget.Consumed(), true)
			return finalText
		}

	}

	// If max turns reached without a final response, try one last non-streaming call
	// to get a conclusive answer (like Claude Code's max_turns handling).
	// Tools are removed in this call to force a text-only response.
	if finalText == "" && a.budget.GraceCall() {
		a.out("\n[WARN] Max turns (%d) reached, requesting final answer...\n", a.maxTurns)
		a.context.AddUserMessage("You have reached the maximum number of tool use turns. Please provide a final summary based on the work done so far. Do NOT call any more tools.")
		// Call WITHOUT tools to force text-only response
		toolCallsGrace, textPartsGrace, err := a.callWithNonStreamingNoTools()
		if err == nil && len(textPartsGrace) > 0 {
			finalText = strings.Join(textPartsGrace, "\n")
		}
		_ = toolCallsGrace // ignore any tool calls in grace response (should be none)
	}

	if finalText == "" {
		finalText = "(max turns reached without a final response)"
	}

	// Flush transcript after each turn
	if a.transcript != nil {
		_ = a.transcript.Flush()
	}

	// Post-run skill evolution and memory update (openclacky pattern):
	// After the main loop completes (normal exit, not interrupted), run the
	// skill evolution system to reflect on executed skills or auto-create new
	// ones from complex workflows, then run memory update to persist knowledge.
	taskTurns := a.budget.Consumed() - a.taskStartTurns
	if !a.IsInterrupted() {
		a.runSkillEvolutionIntegration(taskTurns)
		a.runMemoryUpdate(taskTurns)
	}

	a.fireStopHook("completed", a.budget.Consumed(), false)
	return finalText
}

// Close releases resources (transcript writer) and stops background goroutines.
func (a *AgentLoop) Close() {
	// Wait for background sub-agents to finish (with timeout)
	done := make(chan struct{})
	go func() {
		a.activeSubAgents.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		a.out("[WARN] Timed out waiting for sub-agents after 60s\n")
	}

	// Note: we no longer kill background tasks on Close() timeout.
	// Background tasks (sub-agents, exec) continue running independently.
	// They complete naturally or are cleaned up by the eviction ticker.
	// This matches Claude Code upstream behavior where the parent stops
	// waiting but the background task keeps running.

	// Signal the eviction ticker to stop
	if a.evictionDone != nil {
		close(a.evictionDone)
		a.evictionDone = nil
	}

	// Stop cron scheduler
	if a.cronScheduler != nil {
		a.cronScheduler.stop()
		a.cronScheduler = nil
	}

	if a.transcript != nil {
		_ = a.transcript.Close()
	}

	// Save and display cost summary at session end.
	if a.costTracker != nil {
		if tp := a.TranscriptPath(); tp != "" {
			costPath := tp + ".cost.json"
			_ = a.costTracker.SaveToFile(costPath)
			a.logDebug("\n[cost] Session cost saved to %s\n", costPath)
		}
		a.out("\n%s\n", a.costTracker.FormatCostDisplay())
	}
}

// ForceCompact forces a context compaction (for /compact command).
// Skips NeedsCompaction check -- always performs truncation.
func (a *AgentLoop) ForceCompact() {
	entries := a.context.Entries()
	if len(entries) == 0 {
		fmt.Println("[compact] No messages to compact.")
		return
	}

	// Capture full conversation data before any truncation.
	// buildCompactSummaryMessage needs pre-compacted entries to generate useful
	// structured metadata ("N conversation turns with N tool calls").
	preCompactMessages := a.context.BuildMessages()
	preCompactToolCalls := a.extractRecentToolCallsForSummary(5)

	// Execute pre-compact hooks before any compaction action.
	var preCompactInst string
	if a.hooks != nil {
		hookInput := PreCompactInput{Trigger: HookTriggerManual, CustomInstructions: ""}
		if hookResult, err := a.hooks.ExecutePreCompactHooks(hookInput); err == nil {
			preCompactInst = hookResult.CustomInstructions
		}
	}

	// Try normal compaction first (may skip if not needed)
	preTokens := a.context.EstimatedTokens()
	preEntries := len(entries)
	if a.context.CompactContext() {
		postTokens := a.context.EstimatedTokens()
		postEntries := len(a.context.Entries())
		saved := preTokens - postTokens
		a.logDebug("[compact] Compacted: %d → %d entries, %s → %s tokens (saved %s)\n",
			preEntries, postEntries,
			formatTokenCount(int64(preTokens)), formatTokenCount(int64(postTokens)),
			formatTokenCount(int64(saved)))

		a.context.AddCompactBoundary(CompactTriggerAuto, preTokens)

		// Build structured summary matching upstream's getCompactUserSummaryMessage format.
		summaryContent := a.buildCompactSummaryMessage(preTokens, preCompactMessages, preCompactToolCalls)
		if preCompactInst != "" {
			summaryContent += "\n\n## Custom instructions for this compaction:\n" + preCompactInst
		}
		a.context.AddSummary(summaryContent)

		if a.toolStateTracker != nil {
			a.toolStateTracker.OnCompaction()
		}
		a.InjectRunningAgentStatus()
		recoveredPaths := a.PostCompactRecovery(HookTriggerManual, summaryContent)
		if a.toolStateTracker != nil {
			for _, path := range recoveredPaths {
				a.toolStateTracker.MarkFileFresh(path)
			}
		}
		return
	}

	// Normal compaction skipped (not enough tokens) -- force truncation
	before := len(entries)
	beforeTokens := a.context.EstimatedTokens()
	a.context.TruncateHistory()
	after := len(a.context.Entries())
	afterTokens := a.context.EstimatedTokens()
	if after < before {
		preTokens := beforeTokens
		saved := beforeTokens - afterTokens
		a.logDebug("[compact] Truncated: %d → %d entries, %s → %s tokens (saved %s)\n",
			before, after,
			formatTokenCount(int64(beforeTokens)), formatTokenCount(int64(afterTokens)),
			formatTokenCount(int64(saved)))
		a.context.AddCompactBoundary(CompactTriggerAuto, preTokens)
		summaryContent := a.buildCompactSummaryMessage(preTokens, preCompactMessages, preCompactToolCalls)
		if preCompactInst != "" {
			summaryContent += "\n\n## Custom instructions for this compaction:\n" + preCompactInst
		}
		a.context.AddSummary(summaryContent)
		if a.toolStateTracker != nil {
			a.toolStateTracker.OnCompaction()
		}
		a.InjectRunningAgentStatus()
		recoveredPaths := a.PostCompactRecovery(HookTriggerManual, summaryContent)
		if a.toolStateTracker != nil {
			for _, path := range recoveredPaths {
				a.toolStateTracker.MarkFileFresh(path)
			}
		}
		fmt.Printf("[compact] %d -> %d entries (truncated)\n", before, after)
	} else {
		fmt.Printf("[compact] No compaction needed (%d entries)\n", before)
	}
	if a.config.cachedPrompt != nil {
		a.config.cachedPrompt.MarkDirty()
	}
}

// ClearHistory clears all conversation messages (for /clear command).
// Returns the number of messages cleared.
func (a *AgentLoop) ClearHistory() int {
	count := a.context.Len()
	a.context.Clear()
	// Mark all tracked items as stale (everything is gone).
	if a.toolStateTracker != nil {
		a.toolStateTracker.OnCompaction()
	}
	// Mark system prompt dirty after clearing
	if a.config.cachedPrompt != nil {
		a.config.cachedPrompt.MarkDirty()
	}
	return count
}

// ForcePartialCompact forces a directional partial compaction (for /partialcompact command).
// Direction "up_to" summarizes everything before the pivot, keeping recent context.
// Direction "from" summarizes everything after the pivot, keeping early context.
func (a *AgentLoop) ForcePartialCompact(direction string, pivotIndex int) {
	if !a.config.PartialCompactEnabled {
		fmt.Println("[partial-compact] Partial compaction is disabled.")
		return
	}

	dir := PartialCompactDirection(direction)
	if dir != PartialCompactUpTo && dir != PartialCompactFrom {
		fmt.Printf("[partial-compact] Invalid direction: %s (use 'up_to' or 'from')\n", direction)
		return
	}

	entries := a.context.Entries()
	if len(entries) == 0 {
		fmt.Println("[partial-compact] No messages to compact.")
		return
	}

	// Auto-detect pivot if not specified
	if pivotIndex <= 0 {
		// Default: midpoint of conversation
		pivotIndex = len(entries) / 2
	}
	if pivotIndex >= len(entries) {
		pivotIndex = len(entries) - 1
	}

	var conclusions []string
	if a.toolStateTracker != nil {
		conclusions = a.toolStateTracker.GetConclusions()
	}
	result, err := a.context.PartialCompact(dir, pivotIndex, a.TranscriptPath(), 3, conclusions)
	if err != nil {
		fmt.Printf("[partial-compact] Error: %v\n", err)
		return
	}

	fmt.Printf("[partial-compact: %s] %d entries summarized, %d kept, ~%d tokens saved\n",
		dir, result.MessagesSummarized, result.MessagesKept, result.TokensSaved)

	// Mark all tracked items as stale (partial compact still removes tool results).
	if a.toolStateTracker != nil {
		a.toolStateTracker.OnCompaction()
	}

	// Post-compact recovery: re-inject critical context after partial compact.
	// PostCompactRecovery includes RunPostCompactCleanup internally.
	recoveredPaths := a.PostCompactRecovery(HookTriggerManual, result.Summary)
	if a.toolStateTracker != nil {
		for _, path := range recoveredPaths {
			a.toolStateTracker.MarkFileFresh(path)
		}
	}

	// Inject running agent status so model doesn't spawn duplicates
	a.InjectRunningAgentStatus()

	// Keep recent messages — preserve actual message objects with tool structure intact.
	// This matches the LLM-compact and SM-compact paths.
	keepCount := a.config.PostCompactHistorySnipCount
	if keepCount <= 0 {
		keepCount = 8
	}
	a.context.KeepRecentMessages(keepCount)
	a.context.ValidateToolPairing()
	a.context.FixRoleAlternation()
}

// callWithRetryAndFallbackStreaming is like callWithRetryAndFallback but supports
// pipelined tool execution during streaming. When toolCallDoneCh and executor
// are non-nil, tool calls start executing as their content blocks complete,
// overlapping with remaining stream processing.
// checkThinkingClearLatch checks if the session has been idle for >1h and
// latches the thinking clear flag. Once latched, all subsequent API calls
// will send context_management edits to clear old thinking blocks, saving
// ~4-8K tokens per thinking turn in the cached prefix.
// Matching upstream's thinkingClearLatched latch (claude.ts:1497-1507).
func (a *AgentLoop) checkThinkingClearLatch() {
	if a.thinkingClearLatched {
		return
	}
	if !a.lastApiCompletionTime.IsZero() && time.Since(a.lastApiCompletionTime) > time.Hour {
		a.thinkingClearLatched = true
		a.logDebug("[cache] thinking clear latched (>1h idle) — clearing old thinking blocks\n")
	}
}

// thinkingClearOption returns a request option that clears thinking blocks
// via context_management edits, if the latch is active.
func (a *AgentLoop) thinkingClearOption() option.RequestOption {
	if !a.thinkingClearLatched {
		return nil
	}
	return option.WithJSONSet("context_management", map[string]any{
		"edits": []map[string]any{
			{"type": "clear_thinking_20251015"},
		},
	})
}

// recordApiCompletion updates the last API completion timestamp for the
// thinking clear latch idle-time detection.
func (a *AgentLoop) recordApiCompletion() {
	a.lastApiCompletionTime = time.Now()
}

// buildMCPInstructionsDelta returns an announcement string for newly-connected
// MCP servers whose instructions haven't yet been announced this session.
// This is cache-friendly: only new servers produce new content, existing
// server announcements persist in conversation history as attachments.
func (a *AgentLoop) buildMCPInstructionsDelta() string {
	mgr := a.config.MCPManager
	if mgr == nil {
		return ""
	}
	servers := mgr.ListServers()
	if len(servers) == 0 {
		return ""
	}

	// Build server → tools map from AllToolsWithServer
	serverTools := make(map[string][]mcp.Tool)
	for _, tws := range mgr.AllToolsWithServer() {
		serverTools[tws.Server] = append(serverTools[tws.Server], tws.Tool)
	}

	// Detect newly-connected servers not yet announced
	var newServers, removedServers []string
	for _, srv := range servers {
		if !a.announcedMCPServers[srv] {
			newServers = append(newServers, srv)
		}
	}
	// Detect previously-announced servers that are now disconnected
	for srv := range a.announcedMCPServers {
		if !sliceContains(servers, srv) {
			removedServers = append(removedServers, srv)
		}
	}

	if len(newServers) == 0 && len(removedServers) == 0 {
		return ""
	}

	var sb strings.Builder
	if len(newServers) > 0 {
		sb.WriteString("MCP servers connected:\n\n")
		for _, srv := range newServers {
			a.announcedMCPServers[srv] = true
			status := mgr.GetServerStatus(srv)
			tools := serverTools[srv]
			icon := "●"
			if status != "connected" {
				icon = "○"
			}
			sb.WriteString(fmt.Sprintf("%s %s [%s] (%d tools)\n", icon, srv, status, len(tools)))
			for _, t := range tools {
				desc := t.Description
				if len(desc) > 80 {
					desc = desc[:80] + "..."
				}
				sb.WriteString(fmt.Sprintf("  - %s: %s\n", t.Name, desc))
			}
			if instr := mgr.GetServerInstructions(srv); instr != "" {
				sb.WriteString(fmt.Sprintf("  Instructions: %s\n", instr))
			}
			sb.WriteString("\n")
		}
	}
	if len(removedServers) > 0 {
		sb.WriteString("MCP servers disconnected:\n\n")
		for _, srv := range removedServers {
			delete(a.announcedMCPServers, srv)
			sb.WriteString(fmt.Sprintf("○ %s (no longer connected)\n", srv))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// injectMCPInstructionsDelta appends an MCP server delta announcement as a
// user message content block when there are newly-connected or disconnected servers.
func (a *AgentLoop) injectMCPInstructionsDelta(messages []anthropic.MessageParam) []anthropic.MessageParam {
	delta := a.buildMCPInstructionsDelta()
	if delta == "" {
		return messages
	}
	// Append as a user message attachment. This persists in conversation history
	// and is cache-friendly — only new servers produce new content blocks.
	messages = append(messages, anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "<system-reminder>\n" + delta + "</system-reminder>"}},
		},
	})
	return messages
}

// sliceContains checks if a string slice contains a given value.
func sliceContains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// applyMessageHealing applies all DeepSeek-Reasonix healing patterns to messages.
// This is used in both streaming and non-streaming paths to ensure message integrity.
// Returns the (potentially modified) messages slice.
func (a *AgentLoop) applyMessageHealing(messages []anthropic.MessageParam) []anthropic.MessageParam {
	// Stamp missing tool_call IDs - DeepSeek returns 400 without id
	if stamped := stampMissingToolCallIDs(messages); stamped > 0 {
		a.logDebug("[healing] stamped %d missing tool_call IDs\n", stamped)
	}
	// Reasoning retention - strip stale reasoning to reduce request size
	if pruned, dropped := reasoningRetention(messages); pruned > 0 {
		a.logDebug("[reasoning] pruned %d reasoning blocks (%d chars)\n", pruned, dropped)
	}
	// Thinking mode stamping - ensure reasoning_content for thinking models
	isThinkingMode := strings.Contains(a.config.Model, "reasoner")
	if stamped := thinkingModeStamping(messages, isThinkingMode); stamped > 0 {
		a.logDebug("[thinking] stamped %d missing reasoning blocks\n", stamped)
	}
	// Token-budget aware shrinking: shrink tool args if request is getting large
	currentTokens := estimateMessageTokens(messages)
	maxTokens := int(a.modelCapabilities.GetContextWindow(a.config.Model, a.config.MaxContextTokens))
	if currentTokens > maxTokens/2 {
		if healed, saved := shrinkToolCallArgsByTokens(messages, 8000); healed > 0 {
			a.logDebug("[shrink] shrunk %d tool call args (saved %d chars)\n", healed, saved)
		}
		// DeepSeek-Reasonix pattern: shrink oversized tool results to stay within token budgets
		if healed, saved, _ := shrinkOversizedToolResultsByTokens(messages, 8000); healed > 0 {
			a.logDebug("[shrink] shrunk %d tool results (saved %d chars)\n", healed, saved)
		}
	}
	// DeepSeek-Reasonix pattern: fix tool call pairing - drop unpaired tool_calls and stray tool results
	if filtered, droppedCalls, droppedTools := fixToolCallPairing(messages); droppedCalls > 0 || droppedTools > 0 {
		a.logDebug("[healing] fixed tool pairing: dropped %d unpaired calls, %d stray tools\n", droppedCalls, droppedTools)
		if filtered != nil {
			messages = filtered
		}
	}
	return messages
}

func (a *AgentLoop) callWithRetryAndFallbackStreaming(toolCallDoneCh chan int, executor *StreamingToolExecutor) ([]map[string]any, []string, error) {
	const maxStreamRetries = 9 // 1 attempt + 9 retries = 10 total

	toolParams := a.buildToolParams()
	if !a.config.ReactiveCompactEnabled {
		a.tryCompaction()
	}
	// Thinking clear latch: if session idle >1h, latch and clear old thinking
	a.checkThinkingClearLatch()
	// Validate and fix internal entries BEFORE building API messages.
	a.context.ValidateToolPairing()
	a.context.FixRoleAlternation()

	// Inject current time as a system-injected user message.
	a.context.InjectTimeContext()

	messages := a.context.BuildMessages()

	// DIAG: trace empty messages through the pipeline
	diagEmptyMsgs := func(stage string, msgs []anthropic.MessageParam) {
		for i, msg := range msgs {
			if len(msg.Content) == 0 {
				fmt.Fprintf(os.Stderr, "[pipe-diag] %s: msg[%d] role=%s blocks=0 (EMPTY)\n", stage, i, msg.Role)
			}
			for j, b := range msg.Content {
				if b.OfToolResult != nil && len(b.OfToolResult.Content) == 0 {
					fmt.Fprintf(os.Stderr, "[pipe-diag] %s: msg[%d] block[%d] tool_result id=%s content=EMPTY\n", stage, i, j, b.OfToolResult.ToolUseID)
				}
			}
		}
	}
	diagEmptyMsgs("BuildMessages", messages)

	// Inject MCP instructions delta: announce newly-connected MCP servers.
	// This is cache-friendly — only new servers produce new content blocks.
	messages = a.injectMCPInstructionsDelta(messages)
	diagEmptyMsgs("injectMCP", messages)

	messages = NormalizeAPIMessages(messages) // KV cache reuse
	diagEmptyMsgs("Normalize", messages)

	// Apply all healing patterns before injectCacheEdits/cacheMessageParams.
	// Missing these causes API error 2013 in streaming mode.
	messages = a.applyMessageHealing(messages)
	diagEmptyMsgs("healing", messages)

	// Pre-populate the cached MC tracker with compactable tool IDs from the
	// current conversation context. This ensures the tracker knows about tools
	// registered in earlier turns, enabling cache_edits to delete them.
	a.cachedMC.RegisterCompactableToolIDsFromMessages(messages)

	// Inject cache_edits block if the cached microcompact tracker has deletions pending.
	// This deletes old tool results server-side while preserving the prompt cache.
	messages = a.injectCacheEdits(messages)
	diagEmptyMsgs("injectCacheEdits", messages)

	a.logDebug("[api] streaming call: model=%s msgs=%d est_tokens=%d turn=%d\n",
		GetModelForAPI(a.config.Model), len(messages), a.context.EstimatedTokens(), a.budget.Consumed()+1)

	params := anthropic.MessageNewParams{
		Model:     GetModelForAPI(a.config.Model),
		MaxTokens: a.currentMaxTokens.Load(),
		Messages:  messages,
		System:    buildSystemBlocks(a.context.SystemPrompt(), a.getCacheTTL()),
	}
	if len(toolParams) > 0 {
		params.Tools = toolParams
	}

	// Extended thinking: enable if thinking budget is configured (min 1024 tokens)
	if budget := a.config.ThinkingBudgetTokens; budget >= 1024 {
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(budget))
	}

	cacheMessageParams(&params, a.getCacheTTL()) // Anthropic prompt caching (system_and_3)

	// Pre-flight validation: filter empty messages, enforce role alternation,
	// and fix tool pairing. Running in this order is critical:
	// 1) preflightValidateMessages drops empty messages and merges same-role
	// 2) fixToolCallPairing then cleans up orphaned tool_use/tool_result pairs
	//    left behind by dropped empty messages
	// Without step 2, dropping an empty user message that contained tool_result
	// blocks leaves its matching assistant tool_use orphaned → API error 2013.
	params.Messages = preflightValidateMessages(params.Messages)
	if filtered, droppedCalls, droppedTools := fixToolCallPairing(params.Messages); droppedCalls > 0 || droppedTools > 0 {
		a.logDebug("[preflight-fix] tool pairing: dropped %d unpaired calls, %d stray tools\n", droppedCalls, droppedTools)
		if filtered != nil {
			params.Messages = filtered
		}
	}

	// Persistent collect handler across retries (tracks partial delivery)
	collect := NewCollectHandler()

	// Phase 1: Try streaming with smart retry
	for attempt := 0; attempt <= maxStreamRetries; attempt++ {
		if attempt > 0 {
			// On rate limit errors, prefer header-based delay over jittered backoff
			delay := jitteredBackoff(attempt)
			if rlim := a.rateLimitState.RetryDelay(); rlim > 0 && rlim < delay*3 {
				// Use rate limit header delay if it's reasonable (not >3x backoff)
				delay = rlim
			}
			a.out("\n[WARN] Retrying stream (attempt %d/%d), waiting %v...\n",
				attempt+1, maxStreamRetries+1, delay)
			if a.interruptibleSleep(delay) {
				a.out("\nInterrupted during retry backoff.\n")
				return nil, nil, fmt.Errorf("interrupted during retry backoff")
			}
		}

		toolCalls, textParts, err := a.tryStreamOnce(params, collect, toolCallDoneCh, executor)
		if err == nil {
			// Success: reset consecutive 529 and stream failure counters
			a.consecutive529Errors = 0
			a.consecutiveStreamFailures = 0
			a.recordApiCompletion()
			return toolCalls, textParts, nil
		}

		errMsg := err.Error()

		// Model confused -- special handling: inject corrective message
		if strings.Contains(errMsg, "model confused") {
			return nil, nil, err // let Run loop handle recovery
		}

		// Stream stall -- special handling: let Run loop handle truncation
		if strings.Contains(errMsg, "stream stalled") {
			return nil, nil, err // let Run loop handle recovery
		}

		// Context length -- special handling: let Run loop handle truncation
		if isContextLengthError(errMsg) {
			return nil, nil, err // let Run loop handle recovery
		}

		// User interrupted -- don't fall back to non-streaming, return immediately
		if strings.Contains(errMsg, "interrupted by user") {
			return nil, nil, err
		}

		// 529 Overloaded: track consecutive errors and trigger model fallback
		if is529Error(errMsg) {
			if !a.handle529Error() {
				return nil, nil, &FallbackTriggeredError{
					OriginalModel:  a.config.Model,
					FallbackModel:  a.sonnetModel,
					Consecutive529: 3,
				}
			}
			a.out("\n[WARN] 529 Overloaded during stream (%d/3): %v\n", a.consecutive529Errors, err)
			collect.ClearAll()
			continue
		}

		// 429 Rate limit: subscriber-aware gating
		if classifyError(errMsg, 0, 0).Class == ECRateLimit {
			if !a.handle429Error(errMsg) {
				return nil, nil, fmt.Errorf("rate limit: skipping retry for subscription type %q", a.config.SubscriptionType)
			}
		}

		// Transient error (network, timeout, 5xx): decide retry strategy
		if isTransientError(errMsg) {
			a.out("\n[WARN] Transient error during stream: %v\n", err)
			// Clear accumulated state before retry -- the API will send
			// a completely new response with new tool IDs on reconnect,
			// so old collected data would have mismatched IDs.
			collect.ClearAll()

			// Timeout/deadline: inject one-shot hint telling the model to break work
			// into smaller steps. Without this, the retry re-sends the same request
			// and the model may try the same overly-long approach again.
			// Inspired by openclacky's inject_large_output_hint_if_first_timeout().
			if !a.timeoutHintInjected && (strings.Contains(errMsg, "deadline exceeded") || strings.Contains(errMsg, "timeout")) {
				a.timeoutHintInjected = true
				a.out("\n[WARN] Response timed out — injecting hint to split work into smaller steps\n")
				a.context.AddUserMessage("[SYSTEM] The previous response timed out. This usually means the model was trying to produce too much output. Please adapt:\n- Break the task into multiple smaller steps with separate tool calls\n- For long files: first create a skeleton with `write`, then fill sections individually\n- Keep each single tool-call argument well under ~500 lines\n- Do NOT attempt to output the entire deliverable in one response")
				messages = a.context.BuildMessages()
				messages = NormalizeAPIMessages(messages)
				messages = a.applyMessageHealing(messages)
				a.cachedMC.RegisterCompactableToolIDsFromMessages(messages)
				messages = a.injectCacheEdits(messages)
				params.Messages = messages
				cacheMessageParams(&params, a.getCacheTTL()) // re-apply cache_control on rebuilt messages
				params.Messages = preflightValidateMessages(params.Messages)
				if filtered, droppedCalls, droppedTools := fixToolCallPairing(params.Messages); droppedCalls > 0 || droppedTools > 0 {
					a.logDebug("[preflight-fix] timeout retry: dropped %d unpaired calls, %d stray tools\n", droppedCalls, droppedTools)
					if filtered != nil {
						params.Messages = filtered
					}
				}
			}

			// Smart retry decision based on what was already delivered
			switch a.lastDeltasState {
			case DeltasStateNone:
				// Nothing sent yet -- clean retry
				continue
			case DeltasStateToolInFlight:
				// Tool call started but incomplete -- cleared above, retry
				a.out("  [!] Connection dropped mid-tool-call; reconnecting...\n")
				continue
			case DeltasStateTextOnly:
				// Text already streamed to user -- can't retry without duplication,
				// but we have what was collected so far. Fall back to non-streaming
				// for a complete fresh response (matching Hermes outer retry pattern).
				a.out("  [!] Stream interrupted after text output, falling back to non-streaming...\n")
				a.trackStreamFailure()
				return a.callWithNonStreamingFallback(params, true)
			}
		}

		// Non-transient error during stream -> try non-streaming fallback
		a.out("\n[WARN] Stream failed (%v), falling back to non-streaming...\n", err)
		a.trackStreamFailure()
		return a.callWithNonStreamingFallback(params, true)
	}

	// All stream retries exhausted -> try non-streaming fallback
	a.out("\n[WARN] Stream failed after %d attempts, falling back to non-streaming...\n", maxStreamRetries+1)
	a.trackStreamFailure()
	return a.callWithNonStreamingFallback(params, true)
}

// tryStreamOnce makes a single streaming attempt and returns the result.
// `collect` is passed in (not created) so it persists across retries.
func (a *AgentLoop) tryStreamOnce(params anthropic.MessageNewParams, collect *CollectHandler, toolCallDoneCh chan int, executor *StreamingToolExecutor) ([]map[string]any, []string, error) {
	streamStart := time.Now()
	ctx, cancel := a.interruptCtx(context.Background(), 300*time.Second)
	defer cancel()

	term := &TerminalHandler{}
	adapter := NewStreamAdapter(func(chunk StreamChunk) error {
		_ = collect.Handle(chunk)
		if err := term.Handle(chunk); err != nil {
			return err
		}
		if collect.IsToolUseAsText() {
			a.out("\n[WARN] Model confused, aborting stream...\n")
			cancel()
			return fmt.Errorf("model confused: echoed tool syntax as text")
		}
		return nil
	}, nil)

	// Configure dynamic stall timeout (matching hermes-agent patterns)
	isLocal := isLocalEndpoint(a.config.BaseURL)
	estTokens := estimateMessageTokens(params.Messages)
	adapter.WithStallTimeout(isLocal, estTokens)

	// Set up streaming tool executor callback if provided
	if toolCallDoneCh != nil && executor != nil {
		collect.SetToolCallDoneCh(toolCallDoneCh)
		executor.Start(toolCallDoneCh, &collect.ToolCalls)
	}

	// Build request options for streaming call.
	// Must filter nil options — passing a nil option.RequestOption to the SDK
	// causes a nil pointer dereference in requestconfig.Apply().
	streamOpts := []option.RequestOption{option.WithJSONSet("stream", true)}
	if thinkOpt := a.thinkingClearOption(); thinkOpt != nil {
		streamOpts = append(streamOpts, thinkOpt)
	}

	// DEBUG: dump messages right before streaming API call
	for i, msg := range params.Messages {
		if len(msg.Content) == 0 {
			a.logDebug("[stream-api] msg[%d] role=%s blocks=0 (EMPTY)\n", i, msg.Role)
		}
	}

	stream := a.client.Messages.NewStreaming(ctx, params, streamOpts...)
	if err := adapter.Process(stream, cancel); err != nil {
		a.lastDeltasState = adapter.DeltasState() // record what was streamed before error
		errMsg := err.Error()
		if strings.Contains(errMsg, "context canceled") ||
			strings.Contains(errMsg, "context deadline exceeded") ||
			strings.Contains(errMsg, "deadline exceeded") {
			// Check if the context was cancelled due to user interrupt (Ctrl+C)
			// rather than a genuine stream stall or timeout.
			if a.IsInterrupted() {
				a.SetInterrupted(false)
				return nil, nil, fmt.Errorf("interrupted by user")
			}
			return nil, nil, fmt.Errorf("stream stalled: %w", err)
		}
		return nil, nil, fmt.Errorf("stream error: %w", err)
	}

	// Record what was streamed (for retry safety)
	a.lastDeltasState = adapter.DeltasState()

	// Accumulate token usage from this streaming response
	if collect.Usage != nil {
		a.recordTokenUsageWithCache(
			int64(collect.Usage.InputTokens), int64(collect.Usage.OutputTokens),
			int64(collect.Usage.CacheWriteTokens), int64(collect.Usage.CacheReadTokens))
		// Anchor the token estimate to the actual API response to prevent drift
		a.context.SetAPITokenAnchor(int64(collect.Usage.InputTokens))
		a.context.SetAPITokenAnchor(int64(collect.Usage.InputTokens))
		// Detect cache break: warn if cache reuse dropped significantly from previous call
		if a.cacheBreakDetector.DetectBreak(int64(collect.Usage.CacheReadTokens)) {
			baseline := a.cacheBreakDetector.LastBaseline()
			current := int64(collect.Usage.CacheReadTokens)
			detail := fmt.Sprintf("cache_read dropped: baseline=%d → current=%d (delta=%d)", baseline, current, baseline-current)
			a.logDebug("[cache-break] %s\n", detail)
			if fpath := a.cacheBreakDetector.WriteDiagnosticFile(baseline, current, detail); fpath != "" {
				a.logDebug("[cache-break] diagnostic written to: %s\n", fpath)
			}
		}
		// Update cache break detector baseline with current cache read tokens
		a.cacheBreakDetector.UpdateBaseline(int64(collect.Usage.CacheReadTokens))

		// Phase 2 of two-phase cache break detection: check response for cache break and explain why
		gap := TimeSinceLastAssistantMsg()
		// Check if cache_edits deletions are pending (from cached microcompact tracker)
		cacheDeletionsPending := a.cachedMC.HasPendingDeletions()
		isBreak, reason := CheckResponseForCacheBreak(int64(collect.Usage.CacheReadTokens), int64(collect.Usage.CacheWriteTokens), gap, cacheDeletionsPending, false)
		if isBreak {
			a.logDebug("[cache-break-2] %s\n", reason)
		}
		UpdateLastAssistantMsgTime()
	}

	// Record API call telemetry for streaming
	if a.telemetry != nil {
		var apiErr error
		if len(collect.ToolCalls) == 0 && collect.Text == "" && collect.Thinking == "" && collect.finishReason == "" {
			apiErr = fmt.Errorf("stream ended without receiving any events")
		}
		a.telemetry.RecordAPICall(params.Model, true, time.Since(streamStart).Milliseconds(),
			func() int64 {
				if collect.Usage != nil {
					return int64(collect.Usage.InputTokens)
				}
				return 0
			}(),
			func() int64 {
				if collect.Usage != nil {
					return int64(collect.Usage.OutputTokens)
				}
				return 0
			}(),
			apiErr)
	}

	// Detect incomplete streams: if the stream produced no assistant message
	// (e.g., proxy returned 200 with empty body), treat as a stream error.
	// This mirrors the upstream check: "if (!partialMessage || (newMessages.length === 0 && !stopReason))"
	if len(collect.ToolCalls) == 0 && collect.Text == "" && collect.Thinking == "" && collect.finishReason == "" {
		return nil, nil, fmt.Errorf("stream ended without receiving any events")
	}

	// Check for content policy refusal (stop_reason: "refusal").
	// Matching upstream's getErrorMessageIfRefusal() in errors.ts:1187.
	if collect.IsRefusal() {
		msg := a.handleRefusal("refusal")
		a.lastTransition = TransitionRefusal
		return nil, []string{msg}, nil
	}

	// Preserve redacted_thinking data for context continuity.
	// The API returns opaque data blobs in redacted_thinking blocks when
	// interleaved thinking is enabled but the thinking content is policy-filtered.
	// These must be re-submitted in subsequent API requests.
	if data := collect.RedactedThinkingData(); len(data) > 0 {
		a.context.SetRedactedThinkingData(data)
	}

	// Check for tool-as-text echo and truncated arguments
	if collect.IsToolUseAsText() {
		a.out("\n[WARN] Model echoed tool syntax as text -- recovering\n")
		collect.Text = ""
	}

	// Check for truncated tool arguments (matching Hermes truncated arg detection)
	if collect.HasTruncatedToolArgs() {
		names := make([]string, 0, len(collect.ToolCalls))
		for _, tc := range collect.ToolCalls {
			names = append(names, tc.Name)
		}
		a.out("\n[WARN] Tool arguments truncated: %v\n", names)
		return nil, nil, fmt.Errorf("tool arguments were truncated (incomplete JSON)")
	}

	// Pass finish_reason to collect for downstream access
	if fr := adapter.FinishReason(); fr != "" {
		collect.SetFinishReason(fr)
		// If the model hit the max_tokens ceiling, escalate for the next request.
		// This matches Claude Code's ESCALATED_MAX_TOKENS = 64,000 behavior.
		if fr == "max_tokens" && a.config.EscalatedMaxOutputTokens > int(a.currentMaxTokens.Load()) {
			prev := a.currentMaxTokens.Load()
			a.currentMaxTokens.Store(int64(a.config.EscalatedMaxOutputTokens))
			a.logDebug("\n[auto] max_tokens hit (%d), escalating to %d for next request\n", prev, a.config.EscalatedMaxOutputTokens)
			a.lastTransition = TransitionMaxTokens
		} else if fr == "max_tokens" {
			// Already at escalated level -- inject recovery message for next turn.
			// Matches upstream's MAX_OUTPUT_TOKENS_RECOVERY path.
			a.context.AddUserMessage("Output token limit reached. Resume directly -- no apology, no recap. Pick up mid-thought and break remaining work into smaller pieces.")
		}
	}

	toolCalls, textParts := collect.AsParsedResponse()
	return toolCalls, textParts, nil
}

// callWithNonStreamingOnly is the primary entry point when streaming is disabled.
// It's identical to callWithNonStreamingFallback but named for the non-streaming path.
func (a *AgentLoop) callWithNonStreamingOnly() ([]map[string]any, []string, error) {
	a.logDebug("[api] non-streaming call: model=%s est_tokens=%d turn=%d\n",
		GetModelForAPI(a.config.Model), a.context.EstimatedTokens(), a.budget.Consumed()+1)
	return a.callWithNonStreamingFallback(a.buildMessageParams(), false)
}

// buildMessageParams constructs the API request params from current context.
// Mirrors the validation done in callWithRetryAndFallback (streaming path).
func (a *AgentLoop) buildMessageParams() anthropic.MessageNewParams {
	toolParams := a.buildToolParams()
	if !a.config.ReactiveCompactEnabled {
		a.tryCompaction()
	}
	// Thinking clear latch: if session idle >1h, latch and clear old thinking
	a.checkThinkingClearLatch()
	// Inject current time as a system-injected user message.
	// This replaces the time that was previously inside the system prompt.
	// By injecting it here, the system prompt stays fully static and cacheable,
	// and the time message is skipped for cache breakpoint placement.
	a.context.InjectTimeContext()
	// Validate and fix internal entries BEFORE building API messages.
	// Without this, consecutive user-role entries from compaction
	// (Summary + Attachments + Snips) cause API error 2013.
	a.context.ValidateToolPairing()
	a.context.FixRoleAlternation()
	messages := a.context.BuildMessages()

	// Inject MCP instructions delta: announce newly-connected MCP servers.
	messages = a.injectMCPInstructionsDelta(messages)

	messages = NormalizeAPIMessages(messages) // KV cache reuse

	// Apply all healing patterns to ensure message integrity.
	messages = a.applyMessageHealing(messages)

	params := anthropic.MessageNewParams{
		Model:     GetModelForAPI(a.config.Model),
		MaxTokens: a.currentMaxTokens.Load(),
		Messages:  messages,
		System:    buildSystemBlocks(a.context.SystemPrompt(), a.getCacheTTL()),
	}
	if len(toolParams) > 0 {
		params.Tools = toolParams
	}
	cacheMessageParams(&params, a.getCacheTTL()) // Anthropic prompt caching (system_and_3)

	// Pre-flight validation + tool pairing fix after cacheMessageParams
	params.Messages = preflightValidateMessages(params.Messages)
	if filtered, droppedCalls, droppedTools := fixToolCallPairing(params.Messages); droppedCalls > 0 || droppedTools > 0 {
		a.logDebug("[preflight-fix] dropped %d unpaired calls, %d stray tools\n", droppedCalls, droppedTools)
		if filtered != nil {
			params.Messages = filtered
		}
	}
	// to enable post-call diagnosis if cache_read drops >5%.
	sysPrompt := a.context.SystemPrompt()
	toolNames := make([]string, len(toolParams))
	toolSchemas := make([]map[string]any, len(toolParams))
	for i, tp := range toolParams {
		schemaBytes, _ := json.Marshal(tp)
		var s map[string]any
		json.Unmarshal(schemaBytes, &s)
		toolSchemas[i] = s
		if name, ok := s["name"].(string); ok {
			toolNames[i] = name
		}
	}
	RecordPromptState(sysPrompt, toolSchemas, toolNames, a.config.Model, false)

	// Extended thinking
	if budget := a.config.ThinkingBudgetTokens; budget >= 1024 {
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(budget))
	}
	return params
}

// callWithNonStreamingNoTools makes a non-streaming API call WITHOUT tools.
// Used for the final grace call when max turns reached -- forces text-only response.
func (a *AgentLoop) callWithNonStreamingNoTools() ([]map[string]any, []string, error) {
	const maxRetries = 3 // shorter retry budget for grace call

	a.logDebug("[api] grace call (no tools): model=%s\n", GetModelForAPI(a.config.Model))

	// Build messages WITHOUT tools, but still validate before sending.
	// Skip compaction here (grace call should not trigger new compaction).
	a.context.InjectTimeContext()
	a.context.ValidateToolPairing()
	a.context.FixRoleAlternation()
	messages := a.context.BuildMessages()
	messages = NormalizeAPIMessages(messages)
	params := anthropic.MessageNewParams{
		Model:     GetModelForAPI(a.config.Model),
		MaxTokens: a.currentMaxTokens.Load(),
		Messages:  messages,
		System:    buildSystemBlocks(a.context.SystemPrompt(), a.getCacheTTL()),
	}
	// NOTE: No tools set -- model can only return text
	cacheMessageParams(&params, a.getCacheTTL()) // rolling cache for grace call
	// Pre-flight + tool pairing fix (same as streaming path)
	params.Messages = preflightValidateMessages(params.Messages)
	if filtered, droppedCalls, droppedTools := fixToolCallPairing(params.Messages); droppedCalls > 0 || droppedTools > 0 {
		a.logDebug("[preflight-fix] grace call: dropped %d unpaired calls, %d stray tools\n", droppedCalls, droppedTools)
		if filtered != nil {
			params.Messages = filtered
		}
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := jitteredBackoff(attempt)
			a.out("\n[WARN] Retrying final call (attempt %d/%d), waiting %v...\n",
				attempt+1, maxRetries+1, delay)
			if a.interruptibleSleep(delay) {
				a.out("\nInterrupted during retry backoff.\n")
				return nil, nil, fmt.Errorf("interrupted during retry backoff")
			}
		}

		ctx, cancel := a.interruptCtx(context.Background(), 600*time.Second)
		opts := []option.RequestOption{option.WithJSONSet("stream", false)}
		if thinkOpt := a.thinkingClearOption(); thinkOpt != nil {
			opts = append(opts, thinkOpt)
		}

		// DEBUG: dump messages right before API call to diagnose 2013 errors
		for i, msg := range params.Messages {
			if len(msg.Content) == 0 {
				a.out("[api-send] msg[%d] role=%s blocks=0 (EMPTY!)\n", i, msg.Role)
			}
		}

		response, err := a.client.Messages.New(ctx, params, opts...)
		cancel()

		if err == nil {
			// Success: reset consecutive 529 counter
			a.consecutive529Errors = 0
			a.recordApiCompletion()
			// Accumulate token usage from this non-streaming response
			if response.Usage.InputTokens > 0 || response.Usage.OutputTokens > 0 {
				a.recordTokenUsageWithCache(response.Usage.InputTokens, response.Usage.OutputTokens,
					int64(response.Usage.CacheCreationInputTokens), int64(response.Usage.CacheReadInputTokens))
				// Per-turn cache stats for verification
				a.cacheMetrics.Record(int(response.Usage.InputTokens), int(response.Usage.CacheReadInputTokens),
					int(intMax(0, int(response.Usage.InputTokens-response.Usage.CacheReadInputTokens-response.Usage.CacheCreationInputTokens))),
					int(response.Usage.OutputTokens))
				a.logDebug("[cache] turn: input=%d cache_write=%d cache_read=%d hit_ratio=%.1f%% total_cache_read=%d\n",
					response.Usage.InputTokens, response.Usage.CacheCreationInputTokens, response.Usage.CacheReadInputTokens,
					a.cacheMetrics.CacheHitRatio()*100, a.totalCacheReadTokens.Load())
				// Detect cache break: warn if cache reuse dropped significantly from previous call
				a.context.SetAPITokenAnchor(response.Usage.InputTokens)
				if a.cacheBreakDetector.DetectBreak(int64(response.Usage.CacheReadInputTokens)) {
					baseline := a.cacheBreakDetector.LastBaseline()
					current := int64(response.Usage.CacheReadInputTokens)
					detail := fmt.Sprintf("cache_read dropped: baseline=%d → current=%d (delta=%d)", baseline, current, baseline-current)
					a.logDebug("[cache-break] %s\n", detail)
					if fpath := a.cacheBreakDetector.WriteDiagnosticFile(baseline, current, detail); fpath != "" {
						a.logDebug("[cache-break] diagnostic written to: %s\n", fpath)
					}
				}
				// Update cache break detector baseline with current cache read tokens
				a.cacheBreakDetector.UpdateBaseline(int64(response.Usage.CacheReadInputTokens))

				// Phase 2 of two-phase cache break detection
				gap := TimeSinceLastAssistantMsg()
				cacheDeletionsPending := a.cachedMC.HasPendingDeletions()
				isBreak, reason := CheckResponseForCacheBreak(int64(response.Usage.CacheReadInputTokens), int64(response.Usage.CacheCreationInputTokens), gap, cacheDeletionsPending, false)
				if isBreak {
					a.logDebug("[cache-break-2] %s\n", reason)
				}
				UpdateLastAssistantMsgTime()
				a.decidePostResponseFold(int(response.Usage.InputTokens), int(a.modelCapabilities.GetContextWindow(a.config.Model, a.config.MaxContextTokens)))
			}
			toolCalls, textParts, stopReason := a.parseResponse(response, false)
			// Register compactable tool_use IDs for cache_edits tracking.
			// Only compactable tools (read/exec/edit/grep/glob/web) are tracked.
			for _, call := range toolCalls {
				if id, ok := call["id"].(string); ok {
					if name, ok := call["name"].(string); ok {
						a.cachedMC.RegisterCompactableToolUse(id, name)
					} else {
						a.cachedMC.RegisterToolUse(id)
					}
				}
			}
			// Mark that cache_edits were included in this API call (prevents double-send).
			a.cachedMC.MarkSentToAPI()
			// Detect content policy refusal (stop_reason: "refusal").
			if msg := a.handleRefusal(stopReason); msg != "" {
				a.lastTransition = TransitionRefusal
				return nil, []string{msg}, nil
			}
			// Detect upstream truncation: parseResponse returns a truncation hint
			// as stopReason when tool_call args were empty/placeholder ("{}").
			// This must be retried — matching openclacky's UpstreamTruncatedError pattern.
			if strings.HasPrefix(stopReason, "upstream truncated") {
				return nil, nil, fmt.Errorf("upstream truncation: %s", stopReason)
			}
			// If the model hit the max_tokens ceiling, escalate for the next request.
			// This matches Claude Code's ESCALATED_MAX_TOKENS = 64,000 behavior.
			if stopReason == "max_tokens" && a.config.EscalatedMaxOutputTokens > int(a.currentMaxTokens.Load()) {
				prev := a.currentMaxTokens.Load()
				a.currentMaxTokens.Store(int64(a.config.EscalatedMaxOutputTokens))
				a.logDebug("\n[auto] max_tokens hit (%d), escalating to %d for next request\n", prev, a.config.EscalatedMaxOutputTokens)
				a.lastTransition = TransitionMaxTokens
			} else if stopReason == "max_tokens" {
				// Already at escalated level -- inject recovery message for next turn.
				a.context.AddUserMessage("Output token limit reached. Resume directly -- no apology, no recap. Pick up mid-thought and break remaining work into smaller pieces.")
			}
			return toolCalls, textParts, nil
		}

		if a.IsInterrupted() {
			a.SetInterrupted(false)
			return nil, nil, fmt.Errorf("interrupted by user")
		}

		errMsg := err.Error()
		if strings.Contains(errMsg, "model confused") ||
			strings.Contains(errMsg, "stream stalled") ||
			isContextLengthError(errMsg) {
			return nil, nil, err
		}
		// 529 Overloaded: track consecutive errors and trigger model fallback
		if is529Error(errMsg) {
			if !a.handle529Error() {
				return nil, nil, &FallbackTriggeredError{
					OriginalModel:  a.config.Model,
					FallbackModel:  a.sonnetModel,
					Consecutive529: 3,
				}
			}
			a.out("\n[WARN] 529 Overloaded during grace call (%d/3): %v\n", a.consecutive529Errors, err)
			continue
		}
		// 429 Rate limit: subscriber-aware gating
		if classifyError(errMsg, 0, 0).Class == ECRateLimit {
			if !a.handle429Error(errMsg) {
				return nil, nil, fmt.Errorf("rate limit: skipping retry for subscription type %q", a.config.SubscriptionType)
			}
		}
		a.consecutive529Errors = 0
		if isTransientError(errMsg) {
			continue
		}
		a.consecutive529Errors = 0
		return nil, nil, fmt.Errorf("final call error: %w", err)
	}

	return nil, nil, fmt.Errorf("final call failed after %d retries", maxRetries)
}

// callWithNonStreamingFallback tries non-streaming API call with retries.
// Mirrors Claude Code's non-streaming fallback + retry budget.
func (a *AgentLoop) callWithNonStreamingFallback(params anthropic.MessageNewParams, skipThinkingDisplay bool) ([]map[string]any, []string, error) {
	const maxRetries = 9 // 1 attempt + 9 retries = 10 total

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := jitteredBackoff(attempt)
			if rlim := a.rateLimitState.RetryDelay(); rlim > 0 && rlim < delay*3 {
				delay = rlim
			}
			a.out("\n[WARN] Retrying non-streaming call (attempt %d/%d), waiting %v...\n",
				attempt+1, maxRetries+1, delay)
			if a.interruptibleSleep(delay) {
				a.out("\nInterrupted during retry backoff.\n")
				return nil, nil, fmt.Errorf("interrupted during retry backoff")
			}
		}

		ctx, cancel := a.interruptCtx(context.Background(), 600*time.Second)
		opts := []option.RequestOption{option.WithJSONSet("stream", false)}
		if thinkOpt := a.thinkingClearOption(); thinkOpt != nil {
			opts = append(opts, thinkOpt)
		}

		// DEBUG: dump messages right before API call to diagnose 2013 errors
		for i, msg := range params.Messages {
			if len(msg.Content) == 0 {
				a.out("[api-send] msg[%d] role=%s blocks=0 (EMPTY!)\n", i, msg.Role)
			}
		}

		response, err := a.client.Messages.New(ctx, params, opts...)
		cancel()

		if err == nil {
			// Success: reset consecutive 529 counter
			a.consecutive529Errors = 0
			a.recordApiCompletion()
			// Accumulate token usage from this non-streaming response
			if response.Usage.InputTokens > 0 || response.Usage.OutputTokens > 0 {
				a.recordTokenUsageWithCache(response.Usage.InputTokens, response.Usage.OutputTokens,
					int64(response.Usage.CacheCreationInputTokens), int64(response.Usage.CacheReadInputTokens))
				// Detect cache break: warn if cache reuse dropped significantly from previous call
				a.context.SetAPITokenAnchor(response.Usage.InputTokens)
				if a.cacheBreakDetector.DetectBreak(int64(response.Usage.CacheReadInputTokens)) {
					baseline := a.cacheBreakDetector.LastBaseline()
					current := int64(response.Usage.CacheReadInputTokens)
					detail := fmt.Sprintf("cache_read dropped: baseline=%d → current=%d (delta=%d)", baseline, current, baseline-current)
					a.logDebug("[cache-break] %s\n", detail)
					if fpath := a.cacheBreakDetector.WriteDiagnosticFile(baseline, current, detail); fpath != "" {
						a.logDebug("[cache-break] diagnostic written to: %s\n", fpath)
					}
				}
				// Update cache break detector baseline with current cache read tokens
				a.cacheBreakDetector.UpdateBaseline(int64(response.Usage.CacheReadInputTokens))

				// Phase 2 of two-phase cache break detection
				gap := TimeSinceLastAssistantMsg()
				cacheDeletionsPending := a.cachedMC.HasPendingDeletions()
				isBreak, reason := CheckResponseForCacheBreak(int64(response.Usage.CacheReadInputTokens), int64(response.Usage.CacheCreationInputTokens), gap, cacheDeletionsPending, false)
				if isBreak {
					a.logDebug("[cache-break-2] %s\n", reason)
				}
				UpdateLastAssistantMsgTime()
				a.decidePostResponseFold(int(response.Usage.InputTokens), int(a.modelCapabilities.GetContextWindow(a.config.Model, a.config.MaxContextTokens)))
			}
			toolCalls, textParts, stopReason := a.parseResponse(response, false)
			// Register compactable tool_use IDs for cache_edits tracking.
			// Only compactable tools (read/exec/edit/grep/glob/web) are tracked.
			for _, call := range toolCalls {
				if id, ok := call["id"].(string); ok {
					if name, ok := call["name"].(string); ok {
						a.cachedMC.RegisterCompactableToolUse(id, name)
					} else {
						a.cachedMC.RegisterToolUse(id)
					}
				}
			}
			// Mark that cache_edits were included in this API call (prevents double-send).
			a.cachedMC.MarkSentToAPI()
			// Detect content policy refusal (stop_reason: "refusal").
			if msg := a.handleRefusal(stopReason); msg != "" {
				a.lastTransition = TransitionRefusal
				return nil, []string{msg}, nil
			}
			// Detect upstream truncation: parseResponse returns a truncation hint
			// as stopReason when tool_call args were empty/placeholder ("{}").
			// This must be retried — matching openclacky's UpstreamTruncatedError pattern.
			if strings.HasPrefix(stopReason, "upstream truncated") {
				return nil, nil, fmt.Errorf("upstream truncation: %s", stopReason)
			}
			// If the model hit the max_tokens ceiling, escalate for the next request.
			// This matches Claude Code's ESCALATED_MAX_TOKENS = 64,000 behavior.
			if stopReason == "max_tokens" && a.config.EscalatedMaxOutputTokens > int(a.currentMaxTokens.Load()) {
				prev := a.currentMaxTokens.Load()
				a.currentMaxTokens.Store(int64(a.config.EscalatedMaxOutputTokens))
				a.logDebug("\n[auto] max_tokens hit (%d), escalating to %d for next request\n", prev, a.config.EscalatedMaxOutputTokens)
				a.lastTransition = TransitionMaxTokens
			} else if stopReason == "max_tokens" {
				// Already at escalated level -- inject recovery message for next turn.
				a.context.AddUserMessage("Output token limit reached. Resume directly -- no apology, no recap. Pick up mid-thought and break remaining work into smaller pieces.")
			}
			return toolCalls, textParts, nil
		}

		// Interrupt -- check the actual flag, not ctx.Err(), because
		// the interrupt watcher goroutine can race with the timeout.
		if a.IsInterrupted() {
			a.SetInterrupted(false)
			return nil, nil, fmt.Errorf("interrupted by user")
		}

		errMsg := err.Error()

		// 2013 error: tool pairing broken -- repair and rebuild params before retry
		if strings.Contains(errMsg, "2013") || strings.Contains(errMsg, "tool call result does not follow tool call") {
			a.out("\n[WARN] Tool pairing error (2013) in fallback, repairing context...\n")
			// DEBUG: dump messages that caused the error to console
			a.out("[2013-DEBUG] Dumping messages that caused the error:\n")
			for i, msg := range params.Messages {
				roleStr := string(msg.Role)
				a.out("  msg[%d] role=%s blocks=%d\n", i, roleStr, len(msg.Content))
				for j, block := range msg.Content {
					if block.OfToolUse != nil {
						a.out("    [%d] tool_use id=%s name=%s\n", j, block.OfToolUse.ID, block.OfToolUse.Name)
					} else if block.OfToolResult != nil {
						a.out("    [%d] tool_result id=%s\n", j, block.OfToolResult.ToolUseID)
					} else if block.OfText != nil {
						text := block.OfText.Text
						if len(text) > 60 {
							text = text[:60] + "..."
						}
						a.out("    [%d] text: %q\n", j, text)
					} else if block.OfThinking != nil {
						a.out("    [%d] thinking\n", j)
					} else {
						a.out("    [%d] other\n", j)
					}
				}
			}
			a.context.ValidateToolPairing()
			a.context.FixRoleAlternation()
			// Rebuild messages from repaired entries
			rebuilt := a.context.BuildMessages()
			rebuilt = NormalizeAPIMessages(rebuilt)
			// Apply full healing (stamp IDs, fix pairing, shrink, etc.)
			rebuilt = a.applyMessageHealing(rebuilt)
			// Re-inject cache_edits after rebuild
			rebuilt = a.injectCacheEdits(rebuilt)
			// Final pre-flight validation after all repairs
			rebuilt = preflightValidateMessages(rebuilt)
			params.Messages = rebuilt
			a.consecutiveContextErrors = 0
			continue
		}

		// Special errors: pass through to Run loop for handling
		if strings.Contains(errMsg, "model confused") ||
			strings.Contains(errMsg, "stream stalled") ||
			isContextLengthError(errMsg) {
			return nil, nil, err

		}

		// Track consecutive 500 errors as a heuristic for context overflow.
		// When using a proxy (e.g., coze.site), context overflow often returns
		// a generic 500 instead of "context_length_exceeded". We now use
		// precise token-gap parsing when possible, falling back to a reduced
		// consecutive-500 threshold only when we can't parse the exact count.
		is500 := strings.Contains(errMsg, " 500 ") || strings.Contains(errMsg, "500 Internal Server Error")

		// Try precise token-gap parsing from the error message.
		// If the error contains exact token counts, we can trigger compaction
		// immediately with a precise token target instead of waiting for
		// multiple consecutive 500s.
		if overflowTokens, found := parseMaxTokensContextOverflowError(err); found {
			currentTokens := a.context.EstimatedTokens()
			safetyMargin := 5000 // shed extra tokens to avoid boundary issues
			targetTokens := currentTokens - overflowTokens - safetyMargin
			a.logDebug("\n[reactive-compact] Parsed token overflow: %d tokens over limit, triggering precise compaction (current=%d, target=%d)...\n",
				overflowTokens, currentTokens, targetTokens)
			a.reactiveCompact(targetTokens)
			return nil, nil, fmt.Errorf("context_length_exceeded")
		}

		// Fallback: if the error is a context length error but we couldn't
		// parse exact token counts, use the reduced consecutive-500 heuristic.
		if isContextLengthError(errMsg) || is500 {
			a.consecutiveContextErrors++
			if a.consecutiveContextErrors >= 3 { // reduced from 5 for faster recovery
				a.out("\n[WARN] Consecutive context/500 errors detected (%d), triggering reactive compaction...\n",
					a.consecutiveContextErrors)
				a.reactiveCompact(0) // 0 = compact aggressively
				a.consecutiveContextErrors = 0
				return nil, nil, fmt.Errorf("context_length_exceeded")
			}
			if is500 {
				a.out("\n[WARN] Transient 500 during non-streaming (attempt %d/3): %v\n", a.consecutiveContextErrors, err)
			} else {
				a.out("\n[WARN] Context length error during non-streaming (attempt %d/3): %v\n", a.consecutiveContextErrors, err)
			}
			continue
		}
		a.consecutiveContextErrors = 0

		// 529 Overloaded: track consecutive errors and trigger model fallback
		if is529Error(errMsg) {
			if !a.handle529Error() {
				return nil, nil, &FallbackTriggeredError{
					OriginalModel:  a.config.Model,
					FallbackModel:  a.sonnetModel,
					Consecutive529: 3,
				}
			}
			a.out("\n[WARN] 529 Overloaded during non-streaming fallback (%d/3): %v\n", a.consecutive529Errors, err)
			continue
		}

		// 429 Rate limit: subscriber-aware gating
		if classifyError(errMsg, 0, 0).Class == ECRateLimit {
			if !a.handle429Error(errMsg) {
				return nil, nil, fmt.Errorf("rate limit: skipping retry for subscription type %q", a.config.SubscriptionType)
			}
		}

		// Transient error: retry
		a.consecutive529Errors = 0
		if isTransientError(errMsg) {
			a.out("\n[WARN] Transient error during non-streaming: %v\n", err)
			continue
		}

		// Non-transient error: give up
		a.consecutive529Errors = 0
		return nil, nil, fmt.Errorf("stream fallback error: %w", err)
	}

	return nil, nil, fmt.Errorf("stream fallback error after %d retries", maxRetries)
}

func (a *AgentLoop) buildToolParams() []anthropic.ToolUnionParam {
	tools := a.registry.AllTools()
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name()
	}
	sort.Strings(names)
	currentHash := fnvHash(strings.Join(names, ","))

	// Return cached schemas if tool set hasn't changed
	if a.toolSchemaCache != nil && a.toolSchemaCacheHash == currentHash {
		result := make([]anthropic.ToolUnionParam, len(tools))
		for i, t := range tools {
			result[i] = a.toolSchemaCache[t.Name()]
		}
		// Apply cache_control to last tool (re-added each call)
		if len(result) > 0 && result[len(result)-1].OfTool != nil {
			result[len(result)-1].OfTool.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		return result
	}

	// Build fresh schemas
	toolParams := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		schema := t.InputSchema()
		toolParams = append(toolParams, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name(),
				Description: param.NewOpt(t.Description()),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: schema["properties"],
					Required:   getStringSlice(schema, "required"),
				},
			},
		})
	}

	// Cache schemas for next turn (without cache_control markers)
	a.toolSchemaCache = make(map[string]anthropic.ToolUnionParam, len(tools))
	a.toolSchemaCacheHash = currentHash
	for _, param := range toolParams {
		if param.OfTool != nil {
			cached := param
			cached.OfTool.CacheControl = anthropic.CacheControlEphemeralParam{} // don't cache the marker
			a.toolSchemaCache[param.OfTool.Name] = cached
		}
	}

	// Mark last tool definition for cache
	if len(toolParams) > 0 && toolParams[len(toolParams)-1].OfTool != nil {
		toolParams[len(toolParams)-1].OfTool.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	// DeepSeek-Reasonix pattern: tool list fingerprinting for drift detection.
	// When tools are added/removed/schemas change, the prompt prefix shifts and
	// all cached tokens are lost. Track the fingerprint to log when this happens.
	// DeepSeek-Reasonix pattern: fingerprint tool list + prefix to detect cache-invalidating drift
	fpSchemas := make(map[string]string, len(tools))
	if a.toolListFingerprint != nil || a.prefixFingerprint != nil {
		for _, t := range tools {
			schema := t.InputSchema()
			if schemaBytes, err := json.Marshal(schema); err == nil {
				fpSchemas[t.Name()] = string(schemaBytes)
			}
		}
	}
	if a.toolListFingerprint != nil {
		fpNames := make([]string, len(tools))
		for i, t := range tools {
			fpNames[i] = t.Name()
		}
		if a.toolListFingerprint.CheckAndRecord(fpNames, fpSchemas) {
			a.logDebug("[tool-list] drift detected: hash changed to %s (%d tools)\n",
				a.toolListFingerprint.LastHash(), len(tools))
		}
	}

	// DeepSeek-Reasonix pattern: prefix fingerprinting for drift detection.
	// When system prompt, tool schemas, or fewshots change, the cached prefix
	// is invalidated. Track this to detect cache misses from prefix drift.
	if a.prefixFingerprint != nil {
		sysPrompt := ""
		if a.config.cachedPrompt != nil {
			sysPrompt = a.config.cachedPrompt.GetOrBuild(a.registry, string(a.config.PermissionMode), "", a.config.Model, a.config.SkillLoader, a.skillTracker, a.config.SessionMemory)
		}
		fewshots := []map[string]any{} // fewshots from config if available
		if a.prefixFingerprint.CheckAndRecord(sysPrompt, fpSchemas, fewshots) {
			a.logDebug("[prefix-fingerprint] drift detected: hash changed to %s\n",
				a.prefixFingerprint.LastHash())
		}
	}

	return toolParams
}

func (a *AgentLoop) parseResponse(response *anthropic.Message, skipThinkingDisplay bool) ([]map[string]any, []string, string) {
	var toolCalls []map[string]any
	var textParts []string
	var thinking string
	var redactedData []string

	for _, block := range response.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			textParts = append(textParts, v.Text)
		case anthropic.ToolUseBlock:
			var input map[string]any
			if len(v.Input) > 0 {
				if err := json.Unmarshal(v.Input, &input); err != nil {
					// Openclacky pattern: JSON argument repair — attempt to fix
					// common LLM mistakes (trailing commas, unquoted keys, etc.)
					// before rejecting the tool call outright.
					repaired := tools.RepairJSON(string(v.Input))
					if repaired != string(v.Input) {
						_ = json.Unmarshal([]byte(repaired), &input)
						a.logDebug("[json-repair] repaired tool arguments for %s\n", v.Name)
					} else {
						// DeepSeek-Reasonix pattern: truncated JSON repair with
						// brace balancing, string closing, and dangling key filling.
						// This catches cases where the model's output was cut off
						// mid-argument by the API router.
						trResult := repairTruncatedJson(string(v.Input))
						if trResult.Changed {
							_ = json.Unmarshal([]byte(trResult.Repaired), &input)
							a.logDebug("[truncation-repair] repaired %s for %s (notes: %v)\n",
								trResult.Repaired, v.Name, trResult.Notes)
						}
					}
				}
			}
			if input == nil {
				input = make(map[string]any)
			}

			// Detect upstream truncation: only if JSON parsing failed AND
			// the partial content looks truncated (unclosed strings/brackets).
			// Empty arguments {} are NOT truncation - they may be legitimate
			// model output (uncertain about parameters). Let the API handle it.
			// Previously this flagged empty args as "truncation" which was wrong.
			// The correct detection is: parsing failed + looks incomplete.
			if toolCallArgsTruncated(v.Input) {
				// Only report as truncation if there WAS content that failed to parse
				// (empty args = legitimate model uncertainty, not truncation)
				_ = input // already set to empty map above
				// Don't treat empty args as truncation - continue with execution
				// The API will return proper error if args are truly invalid
			}

			call := map[string]any{
				"id":    v.ID,
				"name":  v.Name,
				"input": input,
			}
			toolCalls = append(toolCalls, call)
		case anthropic.ThinkingBlock:
			thinking = v.Thinking
		case anthropic.RedactedThinkingBlock:
			redactedData = append(redactedData, v.Data)
		}
	}

	// Display thinking if present (matches Rust behavior).
	// Skip when called from streaming fallback since TerminalHandler already
	// printed thinking during the stream phase.
	if !skipThinkingDisplay && thinking != "" {
		lines := strings.Split(thinking, "\n")
		preview := lines[0]
		if len(preview) > 120 {
			preview = preview[:120] + "..."
		}
		a.out("\n[THINK] %s\n", preview)
	}

	// Capture stop_reason for max_tokens escalation
	stopReason := string(response.StopReason)
	// Preserve redacted_thinking data for context continuity.
	// The API returns opaque data blobs in redacted_thinking blocks when
	// interleaved thinking is enabled but the thinking content is policy-filtered.
	if len(redactedData) > 0 {
		a.context.SetRedactedThinkingData(redactedData)
	}

	// Tool call scavenge (DeepSeek-Reasonix pattern): search text content for
	// tool calls that weren't properly structured (e.g., emitted in reasoning_content,
	// DSML markup, or raw JSON). Without scavenge, the model's intended tool
	// invocations are silently lost.
	if len(textParts) > 0 {
		scavenged := ScavengeToolCalls(textParts)
		if len(scavenged) > 0 {
			a.logDebug("[scavenge] recovered %d tool call(s) from text content\n", len(scavenged))
			for _, s := range scavenged {
				// Generate a synthetic tool_use_id for scavenged calls
				scavengedID := fmt.Sprintf("scavenged_%s_%x", s["name"], s["input"])
				s["id"] = scavengedID
				toolCalls = append(toolCalls, s)
			}
		}
	}

	// DeepSeek-Reasonix pattern: strip hallucinated tool markup from text output.
	// Models can hallucinate DSML-style function call markup that should be stripped.
	for i, tp := range textParts {
		textParts[i] = stripHallucinatedToolMarkup(tp)
	}

	return toolCalls, textParts, stopReason
}

// toolCallArgsTruncated detects whether a tool_call's arguments were truncated
// by the upstream API provider. This is a narrow detector that only flags
// arguments that are completely empty — it deliberately does NOT flag partial
// or invalid JSON, which should be handled by the ArgumentsParser path.
//
// Detection rules (matching openclacky's tool_call_args_truncated?):
//   - nil/empty byte slice: raw Input is empty
//   - empty object placeholder: input is exactly "{}"
//
// Cases deliberately NOT detected here:
//   - Partial JSON (e.g. {"path": "/tmp/x") — let ArgumentsParser handle it
//   - Invalid/non-parseable JSON — let ArgumentsParser handle it
func toolCallArgsTruncated(input []byte) bool {
	if len(input) == 0 {
		return true
	}
	// Check for empty object placeholder "{}"
	if len(input) == 2 && input[0] == '{' && input[1] == '}' {
		return true
	}
	return false
}

func (a *AgentLoop) executeToolCallsConcurrent(toolCalls []map[string]any) {
	var toolResults []anthropic.ToolResultBlockParam

	// Print all tool calls upfront
	for _, call := range toolCalls {
		toolName, _ := call["name"].(string)
		// Resolve aliases: LLMs often use non-canonical names (bash, read, cat, etc.)
		if resolved, _ := a.registry.Resolve(toolName); resolved != "" && resolved != toolName {
			call["name"] = resolved
			toolName = resolved
		}
		input, _ := call["input"].(map[string]any)
		inputPreview := formatToolArgs(toolName, input)

		if toolName == "exec" {
			a.out("  [%s]: %s\n", toolName, inputPreview)
		} else {
			a.out("  [%s] %s\n", toolName, inputPreview)
		}
	}

	// Pre-check permissions sequentially (avoid concurrent stdin reads in ask mode)
	type toolCallEntry struct {
		call    map[string]any
		index   int
		denied  bool
		errText string
	}
	entries := make([]toolCallEntry, len(toolCalls))
	for i, call := range toolCalls {
		entries[i] = toolCallEntry{call: call, index: i}
		toolName, _ := call["name"].(string)
		input, _ := call["input"].(map[string]any)
		if input == nil {
			input = make(map[string]any)
		}

		// Permission gate check (may read stdin in ask mode)
		tool, _ := a.registry.Get(toolName)
		if tool != nil {
			denial := a.gate.Check(tool, input)
			if denial != nil {
				entries[i].denied = true
				entries[i].errText = denial.Output
			}
		}
	}

	// Execute approved tool calls sequentially to preserve LLM-intended order
	for _, e := range entries {
		// Safe extraction of toolUseID (guard against missing id)
		toolUseID, _ := e.call["id"].(string)
		if toolUseID == "" {
			toolUseID = "synthetic_tool_use_id"
		}
		// Check for interrupt before starting each tool
		if a.IsInterrupted() {
			toolResults = append(toolResults, anthropic.ToolResultBlockParam{
				ToolUseID: toolUseID,
				Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: "Interrupted by user"}}},
				IsError:   param.NewOpt(true),
			})
			continue
		}
		if e.denied {
			toolResults = append(toolResults, anthropic.ToolResultBlockParam{
				ToolUseID: toolUseID,
				Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: injectTodoReminder("", deepSanitizeUTF8(e.errText).(string), a)}}},
				IsError:   param.NewOpt(true),
			})
			continue
		}
		// Skip already-validated gate check in executeSingleTool
		p, output := a.executeSingleToolApproved(e.call)
		toolResults = append(toolResults, p)
		_ = output
	}

	a.context.AddToolResults(toolResults)
	a.cacheBreakDetector.RecordChange(CacheChangeToolResult, len(toolResults))
}

// truncateOutput limits tool output to maxToolChars.
func (a *AgentLoop) truncateOutput(output string) string {
	limit := a.maxToolChars
	if limit <= 0 {
		limit = 8192
	}
	if len(output) <= limit {
		return output
	}
	// Openclacky pattern: size-aware truncation with type hints.
	// Instead of dumb head/tail, detect output structure and preserve it:
	//   - File-like (contains line structure): head + tail with line count marker
	//   - JSON (starts/ends with { or [): truncate middle array/object elements
	//   - Line-based (many newlines): head/tail with line count marker
	//   - Raw text: fallback to head/tail truncation
	first := limit * 4 / 5
	last := limit - first

	// Detect output type for structured truncation
	lines := strings.Split(output, "\n")
	totalLines := len(lines)
	if totalLines > 10 {
		// Line-based output: keep head+tail with line count marker
		firstLines := int(float64(first) * 0.75)
		lastLines := totalLines - int(float64(last)*0.75)
		if firstLines > totalLines-5 {
			firstLines = totalLines - 5
		}
		if lastLines < firstLines+5 {
			lastLines = firstLines + 5
		}

		// Safe UTF-8 boundary for first part
		firstStr := strings.Join(lines[:firstLines], "\n")
		lastStr := strings.Join(lines[lastLines:], "\n")

		if len(firstStr)+len(lastStr)+80 > limit {
			// Adjust to fit within limit
			overflow := len(firstStr) + len(lastStr) + 80 - limit
			cut := overflow / 2
			firstStr = safeTruncateUTF8(firstStr, len(firstStr)-cut)
			lastStr = safeTruncateUTF8(lastStr, cut)
		}

		truncated := fmt.Sprintf("%s\n\n... [TRUNCATED: %d lines omitted] ...\n\n%s",
			firstStr, lastLines-firstLines, lastStr)
		return truncated
	}

	// Fallback: basic head/tail with safe UTF-8 boundary truncation
	firstEnd := first
	for firstEnd > 0 && (output[firstEnd]&0xc0) == 0x80 {
		firstEnd--
	}
	lastStart := len(output) - last
	for lastStart < len(output) && (output[lastStart]&0xc0) == 0x80 {
		lastStart++
	}
	return output[:firstEnd] + "\n\n... [OUTPUT TRUNCATED] ...\n\n" + output[lastStart:]
}

// safeTruncateUTF8 truncates a string to at most n bytes, respecting UTF-8 boundaries.
func safeTruncateUTF8(s string, n int) string {
	if n >= len(s) {
		return s
	}
	// Walk back to find a valid UTF-8 boundary
	for n > 0 && (s[n]&0xc0) == 0x80 {
		n--
	}
	return s[:n]
}

// executeSingleToolApproved runs one tool call with permission already checked.
// Skips the gate.Check call to avoid concurrent stdin reads in ask mode.
func (a *AgentLoop) executeSingleToolApproved(call map[string]any) (anthropic.ToolResultBlockParam, string) {
	return a.executeTool(call, false)
}

// executeTool is the unified tool execution method.
// When checkPermissions is true, it runs the permission gate check.
func (a *AgentLoop) executeTool(call map[string]any, checkPermissions bool) (anthropic.ToolResultBlockParam, string) {
	toolUseID, _ := call["id"].(string)
	toolName, _ := call["name"].(string)
	// Resolve aliases: LLMs often use non-canonical names (bash, read, cat, etc.)
	if resolved, _ := a.registry.Resolve(toolName); resolved != "" && resolved != toolName {
		call["name"] = resolved
		toolName = resolved
	}
	input, _ := call["input"].(map[string]any)
	if input == nil {
		input = make(map[string]any)
	}

	a.logDebug("[tool] executing: %s (id=%s, checkPerms=%v)\n", toolName, toolUseID, checkPermissions)

	// Guard: if toolUseID is empty, try to recover it from the raw map
	// or generate a fallback. Empty toolUseID causes API error 2013.
	if toolUseID == "" {
		// Try alternate key locations (some proxies/models use different field names)
		if altID, ok := call["tool_use_id"].(string); ok && altID != "" {
			toolUseID = altID
		} else if altID, ok := call["toolId"].(string); ok && altID != "" {
			toolUseID = altID
		}
		// If still empty, log the full call map and generate a synthetic ID
		if toolUseID == "" {
			fmt.Fprintf(os.Stderr, "[WARN-EXEC] Empty toolUseID! call=%v\n", call)
			toolUseID = "synthetic_tool_use_id"
		}
	}

	// Hook: PreToolUse — before tool execution (matches upstream's PreToolUse hook)
	if a.hooks != nil {
		a.hooks.ExecuteGenericHooksQuiet(HookPreToolUse, map[string]interface{}{
			"tool_name":   toolName,
			"tool_use_id": toolUseID,
			"input":       input,
		})
	}

	// Coerce argument types to match schema
	if tool, ok := a.registry.Get(toolName); ok {
		tools.CoerceArguments(tool.InputSchema(), input)
	}

	// Remap directory parameter name (official: directory, internal: dir)
	tools.RemapDirParam(input)

	// Storm breaker: detect and suppress repeat-loop tool call storms.
	// If the same tool+args has been called threshold times in a row, suppress
	// and return a hint instead of executing.
	if a.stormBreaker != nil {
		if hint := a.stormBreaker.Inspect(toolName, input); hint != "" {
			a.logDebug("[storm] suppressed: %s\n", hint)
			return anthropic.ToolResultBlockParam{
				ToolUseID: toolUseID,
				Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: hint}}},
				IsError:   param.NewOpt(true),
			}, hint
		}
	}

	// Record tool use to transcript
	if a.transcript != nil {
		_ = a.transcript.WriteToolUse(toolUseID, toolName, input)
	}

	// Agent-controlled timeout in milliseconds.
	// The timeout param from input is in ms (per tool schema).
	// Note: after CoerceArguments, timeout may be int (from float64 coercion) or float64.
	timeoutMs := a.toolTimeoutMs
	if t, ok := input["timeout"].(float64); ok && t > 0 {
		timeoutMs = int(t)
	} else if t, ok := input["timeout"].(int); ok && t > 0 {
		timeoutMs = t
	}
	if timeoutMs < 1000 {
		timeoutMs = 1000
	}
	if timeoutMs > 600000 {
		timeoutMs = 600000
	}
	// Restore timeout (as int ms) for exec, agent, and MCP tools.
	// For other tools, the timeout is handled via context deadline.
	delete(input, "timeout")
	if toolName == "exec" || toolName == "mcp_call_tool" || toolName == "agent" {
		input["timeout"] = timeoutMs
	}

	// Auto-snapshot before write/edit tools
	if a.snapshots != nil && (toolName == "write_file" || toolName == "edit_file" || toolName == "multi_edit") {
		if path := extractFilePath(input); path != "" {
			_ = a.snapshots.TakeSnapshotWithDesc(path, "before "+toolName)
		}
	}

	tool, toolFound := a.registry.Get(toolName)
	if !toolFound {
		msg := "Error: unknown tool '" + toolName + "'"
		return anthropic.ToolResultBlockParam{
			ToolUseID: toolUseID,
			Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: msg}}},
			IsError:   param.NewOpt(true),
		}, msg
	}

	// Validate required parameters
	if err := tools.ValidateParams(tool, input); err != nil {
		msg := "Error: " + err.Error()
		return anthropic.ToolResultBlockParam{
			ToolUseID: toolUseID,
			Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: msg}}},
			IsError:   param.NewOpt(true),
		}, msg
	}

	// ─── Pre-flight validation (openclacky tool preview validation) ──────
	// Validate edit_file before execution to prevent wasted turns.
	// Instead of executing and failing, we deny upfront with structured
	// feedback telling the LLM exactly what to do differently.
	// This matches openclacky's show_tool_preview / show_edit_preview.
	if toolName == "edit_file" {
		pathStr, _ := input["file_path"].(string)
		oldStr, _ := input["old_string"].(string)
		if pathStr != "" && oldStr != "" {
			if previewErr := tools.ValidateEditPreview(pathStr, oldStr); previewErr != "" {
				a.logDebug("[tool] edit_file pre-flight denied: %s\n", previewErr)
				return anthropic.ToolResultBlockParam{
					ToolUseID: toolUseID,
					Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: "Error: " + previewErr}}},
					IsError:   param.NewOpt(true),
				}, previewErr
			}
		}
	}
	if toolName == "multi_edit" {
		pathStr, _ := input["file_path"].(string)
		if pathStr != "" {
			if edits, ok := input["edits"].([]any); ok {
				for _, edit := range edits {
					if editMap, ok2 := edit.(map[string]any); ok2 {
						oldStr, _ := editMap["old_string"].(string)
						if oldStr != "" {
							if previewErr := tools.ValidateEditPreview(pathStr, oldStr); previewErr != "" {
								a.logDebug("[tool] multi_edit pre-flight denied: %s\n", previewErr)
								return anthropic.ToolResultBlockParam{
									ToolUseID: toolUseID,
									Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: "Error: " + previewErr}}},
									IsError:   param.NewOpt(true),
								}, previewErr
							}
						}
					}
				}
			}
		}
	}

	// Read-before-write/edit enforcement (matches Claude Code official behavior):
	// All write operations (write_file, edit_file, multi_edit) require the file to have
	// been read first IF the file already exists. New file creation is always allowed.
	// If the file was read but externally modified since, the write is blocked.
	if (toolName == "write_file" || toolName == "edit_file" || toolName == "multi_edit") && !checkPermissions {
		if path := extractFilePath(input); path != "" {
			if staleMsg := a.registry.CheckFileStale(path); staleMsg != "" {
				return anthropic.ToolResultBlockParam{
					ToolUseID: toolUseID,
					Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: staleMsg}}},
					IsError:   param.NewOpt(true),
				}, staleMsg
			}
		}
	}

	if checkPermissions {
		denial := a.gate.Check(tool, input)
		if denial != nil {
			return anthropic.ToolResultBlockParam{
				ToolUseID: toolUseID,
				Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: denial.Output}}},
				IsError:   param.NewOpt(true),
			}, denial.Output
		}
	}

	// Execute with interrupt-aware context. For exec and MCP tools, we use a very long
	// timeout (10min) because they manage their own timeouts (which auto-background
	// timed-out calls instead of killing them). For other tools, use the user-specified
	// For exec/mcp_call_tool, give a 10-minute context deadline so their own
	// timer-based timeouts fire first (auto-backgrounding the call instead of killing it).
	// For other tools, use the user-specified timeout as the context deadline.
	var ctxDeadline time.Duration
	if toolName == "exec" || toolName == "mcp_call_tool" {
		ctxDeadline = 600000 * time.Millisecond // 10 minutes
	} else {
		ctxDeadline = time.Duration(timeoutMs) * time.Millisecond
	}
	ctx, cancel := a.interruptCtx(context.Background(), ctxDeadline)
	defer cancel()

	resultCh := make(chan tools.ToolResult, 1)
	start := time.Now()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case resultCh <- tools.ToolResult{
					Output:  fmt.Sprintf("Tool panic: %v", r),
					IsError: true,
				}:
				default:
				}
			}
		}()
		resultCh <- tools.ExecuteWithContext(ctx, tool, input)
	}()

	var result tools.ToolResult
	var cancelled bool
	select {
	case result = <-resultCh:
		// Normal completion
	case <-ctx.Done():
		cancelled = true
		if a.IsInterrupted() {
			result = tools.ToolResult{
				Output:  "Interrupted by user",
				IsError: true,
			}
		} else {
			result = tools.ToolResult{
				Output:  fmt.Sprintf("Error: %s timed out after %v", toolName, ctxDeadline.Round(time.Millisecond)),
				IsError: true,
			}
		}
	}
	elapsed := time.Since(start)

	// Record tool call telemetry
	if a.telemetry != nil {
		a.telemetry.RecordToolCall(toolName, elapsed.Milliseconds(), result.IsError)
	}

	// NOTE: FileReadTool.Execute now handles MarkFileRead internally.
	// The agent_loop.go call below was removed to avoid duplication
	// with potentially different path normalization.

	// Post-snapshot for write tools: capture the new state with a meaningful description
	if a.snapshots != nil && !result.IsError && (toolName == "write_file" || toolName == "edit_file" || toolName == "multi_edit") {
		if path := extractFilePath(input); path != "" {
			desc := toolName
			if toolName == "edit_file" {
				if oldStr, ok := input["old_string"].(string); ok {
					if newStr, ok2 := input["new_string"].(string); ok2 {
						desc = fmt.Sprintf("edit: '%s' -> '%s'", limitStr(oldStr, 50), limitStr(newStr, 50))
					}
				}
			} else if toolName == "write_file" {
				if content, ok := input["content"].(string); ok {
					desc = fmt.Sprintf("write: %d bytes", len(content))
				}
			} else if toolName == "multi_edit" {
				if edits, ok := input["edits"].([]any); ok {
					desc = fmt.Sprintf("multi_edit: %d change(s)", len(edits))
				}
			}
			_ = a.snapshots.TakeSnapshotWithDesc(path, desc)
		}
	}

	// rm/rmrf cleanup: clear snapshot history for deleted files
	if a.snapshots != nil && !result.IsError && toolName == "fileops" {
		if op, ok := input["operation"].(string); ok && (op == "rm" || op == "rmrf") {
			if path := extractFilePath(input); path != "" {
				if op == "rm" {
					a.snapshots.ClearPath(path)
				} else {
					a.snapshots.ClearUnderDir(path)
				}
			}
		}
	}

	// Append unified diff to tool result for write/edit tools.
	// Snapshots were taken before and after the tool execution above.
	if a.snapshots != nil && !result.IsError && (toolName == "write_file" || toolName == "edit_file" || toolName == "multi_edit") {
		if path := extractFilePath(input); path != "" {
			if diffStr := diffLastTwoSnapshots(a.snapshots, path); diffStr != "" {
				result.Output += "\n\n--- diff ---\n" + diffStr
			}
		}
	}

	// Record tool call for redundancy detection (after execution, so we have the result preview)
	if a.redundantCallDetector != nil {
		preview := result.Output
		if len(preview) > 200 {
			preview = preview[:200]
		}
		a.redundantCallDetector.Record(toolName, input, preview)
	}

	// Openclacky pattern: redundant tool call detection. If the LLM repeats the
	// same call (e.g., reading the same file twice this turn), append a hint.
	if a.redundantCallDetector != nil {
		if hint := a.redundantCallDetector.DetectRedundancy(toolName, input); hint != "" {
			a.logDebug("[tool] redundant call detected: %s\n", hint)
			// Still execute (the call may be intentional) but warn
			result.Output = hint + "\n\n" + result.Output
		}
	}

	// Openclacky pattern: tool error self-correction hints. Instead of just
	// returning the raw error, inject a hint that guides the LLM toward a
	// better approach (e.g., "file not found" → "try glob to find it").
	if result.IsError {
		if hint := ToolErrorSelfCorrectionHint(toolName, result.Output, input); hint != "" {
			a.logDebug("[tool] self-correction hint: %s\n", hint)
			result.Output = hint + "\n\n" + result.Output
		}
	}

	// Truncate long outputs
	output := a.truncateOutput(result.Output)

	// Truncated result saver: if output was truncated, save full content to disk
	// so the agent can recall it later with read_file instead of re-running the tool.
	if output != result.Output && a.truncatedResultSaver != nil {
		savedMsg := a.truncatedResultSaver.Save(toolName, result.Output)
		if savedMsg != "" {
			output += "\n\n" + savedMsg
		}
	}

	// Display timing to stderr
	if cancelled {
		a.out("  [TIMEOUT] timed out after %v\n", ctxDeadline.Round(time.Millisecond))
	} else if result.IsError {
		preview := limitStr(output, 150)
		a.out("  [ERR] %s (%v): %s\n", toolName, elapsed.Round(10*time.Millisecond), preview)
	} else {
		preview := toolResultPreview(toolName, output)
		if toolName == "exec" {
			// For exec, show result with tool name prefix
			a.out("  [+] %s: %s\n", toolName, preview)
		} else if preview == "" {
			a.out("  [+] %s\n", toolName)
		} else {
			a.out("  [+] %s: %s\n", toolName, preview)
		}
	}

	// Record result to transcript
	if a.transcript != nil {
		_ = a.transcript.WriteToolResult(toolUseID, toolName, output)
	}

	// Hook: PostToolUse — after tool execution (matches upstream's PostToolUse hook)
	if a.hooks != nil {
		a.hooks.ExecuteGenericHooksQuiet(HookPostToolUse, map[string]interface{}{
			"tool_name":   toolName,
			"tool_use_id": toolUseID,
			"output":      output,
			"is_error":    result.IsError,
		})
	}

	a.logDebug("[tool] completed: %s (isError=%v, output_len=%d)\n", toolName, result.IsError, len(output))

	// Openclacky pattern: inject TODO reminder into tool result text.
	// Keeps task list accurate — LLMs frequently forget to update tasks after work.
	resultText := injectTodoReminder(toolName, output, a)

	// Openclacky pattern: sanitize UTF-8 in tool result to prevent invalid bytes
	// from reaching the API (which can cause 400 errors on some providers).
	// Fast path: returns original string unchanged if already valid UTF-8.
	resultText = deepSanitizeUTF8(resultText).(string)

	return anthropic.ToolResultBlockParam{
		ToolUseID: toolUseID,
		Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: resultText}}},
		IsError:   param.NewOpt(result.IsError),
	}, resultText
}

// toolResultPreview extracts the most relevant part of a tool result for display.
func toolResultPreview(toolName, output string) string {
	lines := strings.Split(output, "\n")

	switch toolName {
	case "exec":
		// For exec, show the first line of output, or "(no output)" if empty
		// Skip "STDOUT:" / "STDERR:" headers -- just show the actual content
		cleaned := cleanExecOutput(output)
		if cleaned == "" {
			return "(no output)"
		}
		preview := limitStr(cleaned, 120)
		return preview
	case "read_file":
		if len(lines) > 0 && strings.Contains(lines[0], "File:") {
			return lines[0]
		}
	case "write_file", "edit_file", "multi_edit":
		if strings.Contains(output, "/") || strings.Contains(output, "\\") {
			for _, line := range lines {
				if strings.Contains(line, ".") && (strings.Contains(line, "/") || strings.Contains(line, "\\")) {
					return line
				}
			}
		}
	case "list_dir":
		return limitStr(output, 100)
	}

	preview := lines[0]
	if len(preview) > 120 {
		preview = preview[:120] + "..."
	}
	return preview
}

// cleanExecOutput strips STDOUT/STDERR headers and returns the actual content.
func cleanExecOutput(output string) string {
	// Remove "STDOUT:\n" and "STDERR:\n" headers
	cleaned := strings.TrimPrefix(output, "STDOUT:\n")
	cleaned = strings.TrimPrefix(cleaned, "STDERR:\n")
	cleaned = strings.TrimSuffix(cleaned, "\n")

	// If both stdout and stderr are present, prefer stdout
	if strings.HasPrefix(output, "STDOUT:\n") && strings.Contains(output, "\nSTDERR:\n") {
		parts := strings.SplitN(output, "\nSTDERR:\n", 2)
		stdout := strings.TrimSpace(parts[0])
		stderr := strings.TrimSpace(parts[1])
		if stdout != "" {
			return stdout
		}
		return stderr
	}

	return strings.TrimSpace(cleaned)
}

func limitStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Find the last valid UTF-8 boundary at or before max
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max] + "..."
}

// extractFilePath extracts the file path from tool input.
// Checks only "file_path" — matching official Claude Code schema.
// Uses expandPath to normalize paths (e.g., /e/ → E:\ on Windows)
// so they match what file_history tools use.
func extractFilePath(input map[string]any) string {
	if path, ok := input["file_path"].(string); ok && path != "" {
		return expandPath(path)
	}
	return ""
}

// diffLastTwoSnapshots returns a unified diff between the last two snapshots
// for filePath. Returns empty string if insufficient snapshots or no changes.
func diffLastTwoSnapshots(h *SnapshotHistory, filePath string) string {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return ""
	}
	snaps := h.ListSnapshots(absPath)
	n := len(snaps)
	if n < 2 {
		return ""
	}
	before := snaps[n-2]
	after := snaps[n-1]
	if before.Checksum == after.Checksum {
		return ""
	}
	return generateUnifiedDiff(before.Content, after.Content, "before", "after", 3)
}

// formatToolArgs formats tool input as a compact string, showing file paths prominently.
func formatToolArgs(toolName string, input map[string]any) string {
	// Show the most relevant arg for each tool type
	switch toolName {
	case "read_file", "write_file", "edit_file", "list_dir":
		if path := extractFilePath(input); path != "" {
			return path
		}
	case "exec":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 120 {
				return cmd[:120] + "..."
			}
			return cmd
		}
	case "grep":
		if pattern, ok := input["pattern"].(string); ok {
			if path := extractFilePath(input); path != "" {
				return fmt.Sprintf("%q in %s", pattern, path)
			}
			return fmt.Sprintf("%q", pattern)
		}
	case "glob":
		if pattern, ok := input["pattern"].(string); ok {
			return pattern
		}
	}
	// Fallback: format all args compactly
	parts := make([]string, 0, len(input))
	for k, v := range input {
		s := fmt.Sprintf("%v", v)
		if len(s) > 80 {
			s = s[:80] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	return strings.Join(parts, " ")
}

// Helper: extract required field names from input schema
func getStringSlice(schema map[string]any, key string) []string {
	if v, ok := schema[key]; ok {
		if arr, ok := v.([]string); ok {
			return arr
		}
		if iface, ok := v.([]any); ok {
			out := make([]string, len(iface))
			for i, e := range iface {
				out[i], _ = e.(string)
			}
			return out
		}
	}
	return nil
}

// fileUnchangedStub is the prefix of the "file unchanged" dedup stub returned by read_file
// when the file hasn't changed since the last read. Re-exports the constant from tools/file_read.go.
var fileUnchangedStub = tools.FileUnchangedStub

// shouldExcludeFromPostCompactRestore returns true if the file should NOT be
// re-injected after compaction. Excludes CLAUDE.md (already in system prompt)
// and plan files (.claude/plan/*.md). Matches upstream's shouldExcludeFromPostCompactRestore.
func shouldExcludeFromPostCompactRestore(filename string, projectDir string) bool {
	normalized := filepath.Clean(filename)
	normalized = strings.ReplaceAll(normalized, "\\", "/")

	// Exclude CLAUDE.md — already loaded into system prompt
	base := filepath.Base(normalized)
	if strings.EqualFold(base, "CLAUDE.md") {
		return true
	}

	// Exclude plan files if .claude directory exists
	claudeDir := filepath.Join(projectDir, ".claude")
	if info, err := os.Stat(claudeDir); err == nil && info.IsDir() {
		planPath := filepath.Join(claudeDir, "plan")
		if info, err := os.Stat(planPath); err == nil && info.IsDir() {
			// Check if file is under .claude/plan/
			normalized = strings.ReplaceAll(normalized, "\\", "/")
			planPathNorm := strings.ReplaceAll(filepath.ToSlash(planPath), "\\", "/")
			if strings.HasPrefix(normalized, planPathNorm) {
				return true
			}
		}
	}

	return false
}

// collectReadToolFilePaths walks the preserved message entries (those after the most
// recent CompactBoundaryContent) and collects file paths from read_file tool_use blocks.
// Files whose tool_result is a file_unchanged stub are excluded — the stub points at
// an earlier full read that may have been compacted away, so we want the recovery
// to re-inject the real content. Matches upstream's collectReadToolFilePaths.
func collectReadToolFilePaths(ctx *ConversationContext) map[string]bool {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	// Find entries after the most recent CompactBoundaryContent
	boundaryIdx := -1
	for i := len(ctx.entries) - 1; i >= 0; i-- {
		if _, ok := ctx.entries[i].content.(CompactBoundaryContent); ok {
			boundaryIdx = i
			break
		}
	}
	if boundaryIdx < 0 {
		return nil
	}

	preserved := ctx.entries[boundaryIdx+1:]

	// Step 1: collect tool_use_ids whose tool_result is a file_unchanged stub
	stubToolUseIDs := make(map[string]bool)
	for _, entry := range preserved {
		if entry.role != "user" {
			continue
		}
		results, ok := entry.content.(ToolResultContent)
		if !ok {
			continue
		}
		for _, r := range results {
			for _, c := range r.Content {
				if c.OfText == nil {
					continue
				}
				if strings.HasPrefix(c.OfText.Text, fileUnchangedStub) {
					stubToolUseIDs[r.ToolUseID] = true
				}
			}
		}
	}

	// Step 2: collect file paths from read_file tool_use blocks, skipping stubs
	paths := make(map[string]bool)
	for _, entry := range preserved {
		if entry.role != "assistant" {
			continue
		}
		blocks, ok := entry.content.(ToolUseContent)
		if !ok {
			continue
		}
		for _, b := range blocks {
			if b.OfToolUse == nil || b.OfToolUse.Name != "read_file" {
				continue
			}
			if stubToolUseIDs[b.OfToolUse.ID] {
				continue
			}
			if m, ok := b.OfToolUse.Input.(map[string]any); ok {
				if fp, ok := m["file_path"].(string); ok && fp != "" {
					paths[fp] = true
				}
			}
		}
	}

	return paths
}

// collectUsedToolNamesInPreservedMessages walks the preserved message entries (those
// after the most recent CompactBoundaryContent) and collects tool names from
// tool_use blocks. This lets post-compact recovery only re-announce tools that
// aren't already visible in the preserved tail, avoiding redundant repetition.
func collectUsedToolNamesInPreservedMessages(ctx *ConversationContext) map[string]bool {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	// Find entries after the most recent CompactBoundaryContent
	boundaryIdx := -1
	for i := len(ctx.entries) - 1; i >= 0; i-- {
		if _, ok := ctx.entries[i].content.(CompactBoundaryContent); ok {
			boundaryIdx = i
			break
		}
	}
	if boundaryIdx < 0 {
		// No compact boundary found yet — all messages are preserved.
		// Return all tool names since none are "after" anything special.
		return nil
	}

	preserved := ctx.entries[boundaryIdx+1:]
	names := make(map[string]bool)

	for _, entry := range preserved {
		if entry.role != "assistant" {
			continue
		}
		blocks, ok := entry.content.(ToolUseContent)
		if !ok {
			continue
		}
		for _, b := range blocks {
			if b.OfToolUse != nil && b.OfToolUse.Name != "" {
				names[b.OfToolUse.Name] = true
			}
		}
	}

	return names
}

// collectDiscoveredToolNames extracts discovered tool names from the conversation
// context. It walks all entries before the most recent compact boundary to find
// tool uses that discovered deferred tools (e.g., via tool_search). Also extracts
// previously carried tools from the most recent boundary marker.
// This mirrors upstream's extractDiscoveredToolNames in toolSearch.ts.
func collectDiscoveredToolNames(ctx *ConversationContext) []string {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	discovered := make(map[string]bool)

	// First, extract carried tools from the most recent compact boundary
	boundaryIdx := -1
	for i := len(ctx.entries) - 1; i >= 0; i-- {
		if bc, ok := ctx.entries[i].content.(CompactBoundaryContent); ok {
			boundaryIdx = i
			for _, t := range bc.PreCompactDiscoveredTools {
				discovered[t] = true
			}
			break
		}
	}

	// Walk entries between the boundary and the end (new content since last compact)
	startIdx := 0
	if boundaryIdx >= 0 {
		startIdx = boundaryIdx
	}

	for _, entry := range ctx.entries[startIdx:] {
		if entry.role != "assistant" {
			continue
		}
		blocks, ok := entry.content.(ToolUseContent)
		if !ok {
			continue
		}
		for _, b := range blocks {
			if b.OfToolUse == nil {
				continue
			}
			// Track tool_search usage — when the model uses tool_search,
			// it discovers deferred tools and should carry their schemas.
			if b.OfToolUse.Name == "tool_search" {
				// Parse the search results from the corresponding tool_result
				// to extract discovered tool names. For now, we rely on the
				// fact that the tool_search tool returns tool names in its
				// output text. We'll check the tool_result for tool names.
			}
		}
	}

	// Also scan user messages for tool_results from tool_search to extract
	// discovered tool names from the result text.
	for i := startIdx; i < len(ctx.entries); i++ {
		entry := ctx.entries[i]
		if entry.role != "user" {
			continue
		}
		results, ok := entry.content.(ToolResultContent)
		if !ok {
			continue
		}
		for _, r := range results {
			for _, c := range r.Content {
				if c.OfText != nil && strings.Contains(c.OfText.Text, "tool_search") {
					// Extract tool names from tool_search results.
					// Results look like: "### Grep\nGrep for a regex in files..."
					// Parse the tool names from the markdown headings.
					lines := strings.Split(c.OfText.Text, "\n")
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if strings.HasPrefix(line, "### ") {
							name := strings.TrimSpace(strings.TrimPrefix(line, "### "))
							if name != "" && name != "tool_search" {
								discovered[name] = true
							}
						}
					}
				}
			}
		}
	}

	result := make([]string, 0, len(discovered))
	for name := range discovered {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// PostCompactRecovery re-injects critical context after compaction.
// This prevents the model from losing awareness of files it was working on
// and skills it was using, reducing wasted turns re-reading them.
// Returns the list of recovered file paths (for deduplication in AddHistorySnip).
// trigger and compactSummary are passed to post-compact hooks.
func (a *AgentLoop) PostCompactRecovery(trigger HookTrigger, compactSummary string) []string {
	// Reset cache break detector baseline — compaction invalidates all cached prefixes.
	a.cacheBreakDetector.ResetBaseline()
	// Guard against false-positive break detection on next API call after compaction.
	a.cacheBreakDetector.MarkPostCompaction()
	// File content recovery — optional, skipped when PostCompactRecoverFiles is false.
	// All other recovery steps (tools, agents, todo, session memory) always run.
	var recoveredPaths []string

	// --- File content recovery (optional) ---
	// Implements upstream's snapshot-then-clear pattern:
	// 1. Snapshot all cached file content from registry.filesRead BEFORE compaction clears it
	// 2. After compaction, re-inject the most recently accessed files as attachments
	// 3. Prefer cached content (what the model saw) over disk re-read (may have changed)
	if a.config.PostCompactRecoverFiles && a.registry != nil {
		maxFiles := a.config.PostCompactMaxFiles
		if maxFiles <= 0 {
			maxFiles = 5
		}
		// Use token-based budget (upstream-compatible), fall back to char-based if not set
		maxFileTokens := a.config.PostCompactMaxFileTokens
		if maxFileTokens <= 0 {
			// Legacy char-based fallback: convert chars to approximate tokens
			maxFileChars := a.config.PostCompactMaxFileChars
			if maxFileChars <= 0 {
				maxFileChars = 50000
			}
			maxFileTokens = maxFileChars / 4
		}
		// Per-file token cap (matches upstream POST_COMPACT_MAX_TOKENS_PER_FILE = 5000)
		maxTokensPerFile := a.config.PostCompactMaxTokensPerFile
		if maxTokensPerFile <= 0 {
			maxTokensPerFile = 5000
		}

		// Collect file paths already visible in preserved messages (after boundary).
		// These are files whose read results survived compaction, so re-injecting
		// them would be redundant. Matches upstream's collectReadToolFilePaths.
		preservedReadPaths := collectReadToolFilePaths(a.context)

		paths := a.registry.GetRecentlyReadFiles(maxFiles)
		totalTokens := 0
		filesRecovered := 0

		for _, path := range paths {
			// Expand the normalized path back to a real path
			realPath := path
			if !filepath.IsAbs(realPath) {
				realPath = filepath.Join(a.config.ProjectDir, realPath)
			}

			// Skip plan files and memory files (CLAUDE.md, etc.)
			if shouldExcludeFromPostCompactRestore(realPath, a.config.ProjectDir) {
				continue
			}

			// Skip files already visible in the preserved message tail
			if preservedReadPaths != nil && preservedReadPaths[realPath] {
				continue
			}

			// Snapshot-then-clear pattern: prefer cached content from registry
			// (what the model actually saw) over disk re-read.
			// This matches upstream's generateFileAttachment using readFileState.content.
			var content string
			if cached := a.registry.GetCachedFileContent(path); cached != "" {
				content = cached
			} else {
				// Fallback: re-read from disk if no cached content
				data, err := os.ReadFile(realPath)
				if err != nil {
					continue // file may have been deleted
				}
				content = string(data)
			}

			// Per-file token truncation (matches upstream POST_COMPACT_MAX_TOKENS_PER_FILE)
			contentTokens := EstimateTokens(content)
			if contentTokens > maxTokensPerFile {
				charLimit := maxTokensPerFile * 4
				if charLimit < len(content) {
					content = content[:charLimit] + "\n... [truncated for compaction; use Read to get full content]"
					contentTokens = EstimateTokens(content)
				}
			}

			// Total budget check (matches upstream POST_COMPACT_TOKEN_BUDGET = 50000)
			if totalTokens+contentTokens > maxFileTokens {
				break
			}

			attachment := fmt.Sprintf("[Post-compact file recovery: %s]\n```\n%s\n```", path, content)
			a.context.AddAttachment(attachment)
			totalTokens += contentTokens
			filesRecovered++
			recoveredPaths = append(recoveredPaths, path)

			// Re-mark file as read so edit checks still work
			a.registry.MarkFileRead(path)
		}

		if filesRecovered > 0 {
			a.logDebug("[post-compact] Recovered %d files (~%d tokens)\n", filesRecovered, totalTokens)
		}
	}

	// --- Skill content recovery ---
	if a.skillTracker != nil && a.config.SkillLoader != nil {
		// Use token-based budgets (upstream-compatible), fall back to char-based if not set
		maxSkillTokens := a.config.PostCompactMaxSkillTokens
		if maxSkillTokens <= 0 {
			maxSkillChars := a.config.PostCompactMaxSkillChars
			if maxSkillChars <= 0 {
				maxSkillChars = 5000
			}
			maxSkillTokens = maxSkillChars / 4
		}
		maxTotalSkillTokens := a.config.PostCompactMaxTotalSkillTokens
		if maxTotalSkillTokens <= 0 {
			maxTotalSkillChars := a.config.PostCompactMaxTotalSkillChars
			if maxTotalSkillChars <= 0 {
				maxTotalSkillChars = 25000
			}
			maxTotalSkillTokens = maxTotalSkillChars / 4
		}

		readSkills := a.skillTracker.GetReadSkillNames()
		totalSkillTokens := 0
		skillsRecovered := 0

		for _, name := range readSkills {
			content := a.config.SkillLoader.LoadSkill(name)
			if content == "" {
				continue
			}

			contentTokens := EstimateTokens(content)
			if contentTokens > maxSkillTokens {
				// Truncate per-skill: approximate char limit from token budget
				charLimit := maxSkillTokens * 4
				if charLimit < len(content) {
					content = content[:charLimit] + "\n\n[... skill content truncated for compaction; use Read on the skill path if you need the full text]"
					contentTokens = EstimateTokens(content)
				}
			}

			if totalSkillTokens+contentTokens > maxTotalSkillTokens {
				break
			}

			attachment := fmt.Sprintf("[Post-compact skill recovery: %s]\n%s", name, content)
			a.context.AddAttachment(attachment)
			totalSkillTokens += contentTokens
			skillsRecovered++
		}

		if skillsRecovered > 0 {
			a.logDebug("[post-compact] Recovered %d skills (~%d tokens)\n", skillsRecovered, totalSkillTokens)
		}
	}

	// --- Plan file recovery ---
	// Re-inject the current plan file if one exists, so the model knows
	// what it was working on and what to do next.
	planAttachment := buildPostCompactPlanAttachment(a.config.ProjectDir)
	if planAttachment != "" {
		a.context.AddAttachment(planAttachment)
		a.logDebug("[post-compact] Recovered plan file\n")
	}

	// --- Plan mode recovery ---
	// If in plan mode, remind the model to continue planning without executing.
	if a.config.PermissionMode == ModePlan {
		a.context.AddAttachment("## Plan Mode Active\n\nYou are in plan mode. Do NOT execute any tools without first presenting your plan to the user and getting their approval. Continue planning — do not execute.")
		a.logDebug("[post-compact] Plan mode reminder injected\n")
	}

	// --- Tools re-announcement (delta-based) ---
	// After compaction the model loses all tool-use history. Re-announce the
	// tool inventory so the model knows what capabilities are available.
	// Only announce tools NOT already visible in the preserved message tail,
	// matching upstream's getDeferredToolsDeltaAttachment pattern.
	// Compute once and share with MCP/Agent announcements.
	preservedToolNames := collectUsedToolNamesInPreservedMessages(a.context)
	if a.registry != nil {
		toolsAttachment := a.buildPostCompactToolsAnnouncement(preservedToolNames)
		if toolsAttachment != "" {
			a.context.AddAttachment(toolsAttachment)
			a.logDebug("[post-compact] Re-announced tool inventory (delta)\n")
		} else {
			a.logDebug("[post-compact] All tools already visible in preserved tail, skipping re-announcement\n")
		}
	}

	// --- MCP tools re-announcement (delta-based) ---
	// Re-announce available MCP servers and their tools after compaction.
	// Only announce MCP servers NOT already visible in the preserved message tail
	// (via mcp_call_tool usage), matching upstream's getMcpInstructionsDeltaAttachment.
	if a.config.MCPManager != nil {
		mcpAttachment := a.buildPostCompactMCPAnnouncement(preservedToolNames)
		if mcpAttachment != "" {
			a.context.AddAttachment(mcpAttachment)
			a.logDebug("[post-compact] Re-announced MCP tools (delta)\n")
		}
	}

	// --- Agent listing re-announcement (delta-based) ---
	// Re-announce active background sub-agents after compaction so the model
	// doesn't lose track of running tasks.
	// Only announce agents NOT already visible in the preserved message tail,
	// matching upstream's getAgentListingDeltaAttachment.
	if a.agentTaskStore != nil {
		agentAttachment := a.buildPostCompactAgentAnnouncement(preservedToolNames)
		if agentAttachment != "" {
			a.context.AddAttachment(agentAttachment)
			a.logDebug("[post-compact] Re-announced background agents (delta)\n")
		}
	}

	// --- Todo/Task recovery ---
	// Re-inject task state by scanning transcript for task_create, task_update,
	// and TodoWrite tool calls. This survives compact since the transcript persists.
	taskAttachment := buildTaskRecoveryAttachment(a.context)
	if taskAttachment != "" {
		a.context.AddAttachment(taskAttachment)
		a.logDebug("[post-compact] Task/Todo state recovered\n")
	}

	// --- Session Memory Recovery ---
	// Re-inject session memory after compaction. Session memory contains
	// user-defined notes that must survive context compaction.
	// Uses per-section truncation matching upstream's truncateSessionMemoryForCompact.
	if a.config.SessionMemory != nil {
		smContent := a.config.SessionMemory.FormatForPromptCompact()
		if smContent != "" {
			attachment := fmt.Sprintf("<session_memory>\n%s\n</session_memory>", smContent)
			a.context.AddAttachment(attachment)
			a.logDebug("[post-compact] Session memory recovered\n")
		}
	}

	// --- Post-compact hooks ---
	// Execute registered post-compact hooks. These can inject additional content
	// into the prompt or display user messages. Matches upstream's executePostCompactHooks.
	if a.hooks != nil {
		hookInput := PostCompactInput{
			Trigger:        trigger,
			CompactSummary: compactSummary,
			RecoveredFiles: recoveredPaths,
		}
		hookResult, hookErr := a.hooks.ExecutePostCompactHooks(hookInput)
		if hookErr != nil {
			a.out("[hook] PostCompact error: %v\n", hookErr)
		}
		if hookResult.UserMessage != "" {
			a.out("%s\n", strings.TrimPrefix(hookResult.UserMessage, "\n"))
		}
		if hookResult.Attachment != "" {
			a.context.AddAttachment(hookResult.Attachment)
		}
	}

	// --- Post-compact cleanup ---
	// Clear caches and tracking state that were invalidated by compaction.
	// This matches upstream's runPostCompactCleanup() in postCompactCleanup.ts.

	// Re-inject the in-memory todo list into the system prompt so the model
	// sees its task list. The in-memory TodoList survives compaction; this
	// ensures the reminder is always included after compaction, regardless
	// of which recovery steps ran above.
	a.injectTodoReminder()

	a.RunPostCompactCleanup()

	return recoveredPaths
}

// injectCacheEdits injects a cache_edits content block into the last user message
// if the cached microcompact tracker has pending deletions. The cache_edits block
// deletes old tool results server-side while preserving the prompt cache.
// Returns messages unchanged if no cache edits are pending.
func (a *AgentLoop) injectCacheEdits(messages []anthropic.MessageParam) []anthropic.MessageParam {
	cacheEditsMap, deletedCount := a.cachedMC.GetCacheEditsBlock()
	if cacheEditsMap == nil {
		return messages
	}

	if deletedCount > 0 {
		a.totalCacheEditsDeletions.Add(int64(deletedCount))
		a.logDebug("[cache] cache_edits: deleted=%d total_deletions=%d\n", deletedCount, a.totalCacheEditsDeletions.Load())
	}

	// Marshal to JSON bytes, then unmarshal as ContentBlockParamUnion.
	// This creates a raw content block that the SDK will serialize correctly.
	cacheEditsJSON, err := json.Marshal(cacheEditsMap)
	if err != nil {
		return messages
	}

	var cacheEditsBlock anthropic.ContentBlockParamUnion
	if err := json.Unmarshal(cacheEditsJSON, &cacheEditsBlock); err != nil {
		fmt.Fprintf(os.Stderr, "[injectCacheEdits] unmarshal failed: %v\n", err)
		return messages
	}

	// Find last user message and append cache_edits block
	found := false
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == anthropic.MessageParamRoleUser {
			messages[i].Content = append(messages[i].Content, cacheEditsBlock)
			found = true
			break
		}
	}

	if !found {
		fmt.Fprintf(os.Stderr, "[injectCacheEdits] WARNING: no user message found\n")
	}

	return messages
}

// RunPostCompactCleanup clears caches and tracking state after compaction.
// Call this after PostCompactRecovery in every compaction path.
// This prevents stale references (e.g. file history pointing to deleted messages,
// skill tracker with compacted-away state) from corrupting subsequent turns.
//
// Subagents (agent:*) run in the same process and share module-level state
// with the main thread. Only reset main-thread module-level state for main-thread
// compacts — subagent compaction must not clobber the main thread's state.
func (a *AgentLoop) RunPostCompactCleanup() {
	// Guard: subagents share module-level state with the main thread.
	// Skip clears that would corrupt the main thread when a subagent compacts.
	isMainThread := a.config.querySource == "" || strings.HasPrefix(a.config.querySource, "repl_main_thread") || a.config.querySource == "sdk"

	// Clear skill discovery state: after compaction, the system prompt is rebuilt
	// and should re-announce all skills as "new". The skill content is re-injected
	// via post-compact attachments, so shown/read state should reset.
	// Preserves usedSkills since "used" is a durable fact about the conversation.
	if a.skillTracker != nil {
		a.skillTracker.ResetPostCompact()
	}

	// Invalidate cached system prompt so it rebuilds fresh with post-compact state.
	// The cache was built before compaction and references pre-compact entries.
	if a.config.cachedPrompt != nil {
		a.config.cachedPrompt.MarkDirty()
	}

	// Save tool state conclusions to session memory BEFORE clearing.
	// Conclusions represent the agent's accumulated knowledge about work done;
	// if not saved, they are permanently lost after compaction.
	if a.toolStateTracker != nil && a.config.SessionMemory != nil {
		conclusions := a.toolStateTracker.GetConclusions()
		if len(conclusions) > 0 {
			a.config.SessionMemory.SaveConclusions(conclusions)
		}
		a.toolStateTracker.ClearConclusions()
	}

	// Reset cached microcompact tracker — clear all registered tool IDs
	// and deleted refs. After compaction, tool results are rebuilt from scratch.
	a.cachedMC.Reset()

	// Classifier and permission state — stale decisions may reference
	// compacted messages, so clear them to force re-evaluation.
	a.gate.ResetPostCompact()

	// ─── Missing state clears matching upstream's runPostCompactCleanup() ───────

	// Clear speculative checks (bash permission evaluations) — stale decisions
	// may reference compacted-away messages. Main-thread only since subagents
	// share the same module-level permission state.
	if isMainThread {
		clearSpeculativeChecks()
	}

	// Clear beta tracing state (analytics) — after compaction, tracing context
	// is invalidated and should be reset.
	clearBetaTracingState()

	// Clear session messages cache (transcript/CLI display) — prevents stale
	// references to pre-compact messages. Main-thread only.
	if isMainThread {
		clearSessionMessagesCache()
	}

	// Sweep file content cache (commit attribution) — invalidate cached file
	// snapshots that reference compacted content. Only if COMMIT_ATTRIBUTION
	// feature is active.
	if isMainThread {
		sweepFileContentCache()
	}

	// Reset MCP instructions delta tracking — after compaction, the post-compact
	// announcement re-declares visible servers, so per-turn delta state must reset.
	a.announcedMCPServers = make(map[string]bool)

	// Reset beta header memoization and session latch — after compaction,
	// recompute headers from current env vars and model configuration.
	a.betaHeadersLatched = nil
	ClearBetaHeaderCache()
}

// buildPostCompactToolsAnnouncement re-announces available tools after compaction.
// Only tools NOT already visible in the preserved message tail are announced,
// avoiding redundant re-injection. The model loses tool-use history during
// compaction; this reminds it of tools it hasn't used in the preserved tail.
func (a *AgentLoop) buildPostCompactToolsAnnouncement(preservedToolNames map[string]bool) string {
	var sb strings.Builder
	sb.WriteString("## Tools Available After Compaction\n\n")
	sb.WriteString("The following tools are available. Use them as needed.\n\n")

	// Collect native, MCP, and skill tools, skipping those already visible
	// in the preserved message tail (to avoid redundant re-announcement).
	var nativeTools []string
	var mcpTools []string
	var skillTools []string
	for _, t := range a.registry.AllTools() {
		name := t.Name()
		// Skip tools already used in the preserved message tail
		if preservedToolNames != nil && preservedToolNames[name] {
			continue
		}
		desc := t.Description()
		entry := fmt.Sprintf("- **%s**: %s", name, desc)
		switch name {
		case "mcp_call_tool", "mcp_server_status":
			mcpTools = append(mcpTools, entry)
		case "search_skills", "read_skill", "list_skills":
			skillTools = append(skillTools, entry)
		default:
			nativeTools = append(nativeTools, entry)
		}
	}

	if len(nativeTools) > 0 {
		sb.WriteString("### Core Tools\n")
		for _, t := range nativeTools {
			sb.WriteString(t + "\n")
		}
		sb.WriteString("\n")
	}
	if len(mcpTools) > 0 {
		sb.WriteString("### MCP Tools\n")
		for _, t := range mcpTools {
			sb.WriteString(t + "\n")
		}
		sb.WriteString("\n")
	}
	if len(skillTools) > 0 {
		sb.WriteString("### Skill Tools\n")
		for _, t := range skillTools {
			sb.WriteString(t + "\n")
		}
		sb.WriteString("\n")
	}

	result := strings.TrimRight(sb.String(), "\n")
	// If all tools were already visible, return empty to skip the attachment
	if strings.HasSuffix(result, "Use them as needed.\n\n") {
		return ""
	}
	return result
}

// buildPostCompactPlanAttachment reads the most recent plan file from .claude/plan/
// and returns it as an attachment. Matches upstream's createPlanAttachmentIfNeeded.
func buildPostCompactPlanAttachment(projectDir string) string {
	planDir := filepath.Join(projectDir, ".claude", "plan")
	info, err := os.Stat(planDir)
	if err != nil || !info.IsDir() {
		return ""
	}

	entries, err := os.ReadDir(planDir)
	if err != nil {
		return ""
	}

	// Find the most recently modified .md file
	var newestPath string
	var newestTime int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		modTime := info.ModTime().Unix()
		if modTime > newestTime {
			newestTime = modTime
			newestPath = filepath.Join(planDir, entry.Name())
		}
	}

	if newestPath == "" {
		return ""
	}

	data, err := os.ReadFile(newestPath)
	if err != nil {
		return ""
	}

	content := string(data)
	return fmt.Sprintf("A plan file exists from plan mode at: %s\n\nPlan contents:\n\n%s\n\nIf this plan is relevant to the current work and not already complete, continue working on it.", newestPath, content)
}

// buildPostCompactMCPAnnouncement re-announces MCP servers and tools after compaction.
// Includes per-server instructions from the MCP initialize response.
// Delta-aware: only announces servers whose tools are NOT already visible in the
// preserved message tail, matching upstream's getMcpInstructionsDeltaAttachment.
func (a *AgentLoop) buildPostCompactMCPAnnouncement(preservedToolNames map[string]bool) string {
	mgr := a.config.MCPManager
	servers := mgr.ListServers()
	if len(servers) == 0 {
		return ""
	}

	// Collect MCP server names from preserved tool names.
	// MCP servers are identified by their tools (e.g., mcp_call_tool with server param,
	// or specific MCP tool names like mcp_server_status).
	preservedMCPServers := make(map[string]bool)
	for name := range preservedToolNames {
		if strings.HasPrefix(name, "mcp_") || name == "mcp_call_tool" {
			preservedMCPServers[name] = true
		}
	}

	// Filter servers to only those not already visible in preserved tail.
	// A server is "visible" if any of its tools have been used in the preserved messages.
	var serversToAnnounce []string
	serverTools := make(map[string][]mcp.Tool)
	serverInstructions := mgr.AllServerInstructions()
	for _, tws := range mgr.AllToolsWithServer() {
		serverTools[tws.Server] = append(serverTools[tws.Server], tws.Tool)
	}
	for _, server := range servers {
		// Check if this server's tools were used in preserved messages.
		// A server is visible if any of its tools appear in preservedToolNames.
		seen := false
		for _, tool := range serverTools[server] {
			if preservedToolNames[tool.Name] {
				seen = true
				break
			}
		}
		if seen {
			continue // skip — already visible in preserved tail
		}
		serversToAnnounce = append(serversToAnnounce, server)
	}

	if len(serversToAnnounce) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## MCP Servers After Compaction\n\n")
	sb.WriteString("The following MCP servers are connected. Use list_mcp_tools to discover their tools, or call mcp_call_tool directly.\n\n")

	for _, server := range serversToAnnounce {
		status := mgr.GetServerStatus(server)
		tools := serverTools[server]
		statusIcon := "●"
		if status != "connected" {
			statusIcon = "○"
		}
		sb.WriteString(fmt.Sprintf("%s **%s** [%s] (%d tools)\n", statusIcon, server, status, len(tools)))
		for _, tool := range tools {
			desc := tool.Description
			if len(desc) > 80 {
				desc = desc[:80] + "..."
			}
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", tool.Name, desc))
		}
		// Inject per-server instructions if available
		if instr, ok := serverInstructions[server]; ok && instr != "" {
			sb.WriteString(fmt.Sprintf("\n  **Usage instructions for %s:**\n  %s\n", server, instr))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// buildTaskRecoveryAttachment scans conversation entries for task_create, task_update,
// and TodoWrite tool calls, extracts the most recent task state, and formats it as
// an attachment for post-compact recovery. Matches upstream's extractTodosFromTranscript.
func buildTaskRecoveryAttachment(ctx *ConversationContext) string {
	entries := ctx.Entries()

	// Scan backward for task tool calls and collect task state
	type taskState struct {
		id          string
		subject     string
		status      string
		description string
	}
	latestTasks := make(map[string]taskState) // keyed by task ID
	var todoItems []string                    // TodoWrite items

	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		tu, ok := entry.content.(ToolUseContent)
		if !ok {
			continue
		}
		for _, block := range tu {
			// Extract tool_use block info
			jsonBytes, _ := json.Marshal(block)
			var parsed struct {
				Type  string `json:"type"`
				Name  string `json:"name"`
				Input struct {
					Subject     string `json:"subject"`
					Description string `json:"description"`
					Status      string `json:"status"`
					TaskId      string `json:"taskId"`
					ID          string `json:"id"`
					Todos       []struct {
						Content string `json:"content"`
						Status  string `json:"status"`
						Subject string `json:"subject"`
					} `json:"todos"`
				} `json:"input"`
			}
			if json.Unmarshal(jsonBytes, &parsed) != nil || parsed.Type != "tool_use" {
				continue
			}

			switch parsed.Name {
			case "task_create":
				id := parsed.Input.TaskId
				if id == "" {
					id = parsed.Input.ID
				}
				if id != "" {
					if _, exists := latestTasks[id]; !exists {
						latestTasks[id] = taskState{
							id:          id,
							subject:     parsed.Input.Subject,
							status:      "pending",
							description: parsed.Input.Description,
						}
					}
				}
			case "task_update":
				id := parsed.Input.TaskId
				if id == "" {
					id = parsed.Input.ID
				}
				if id != "" {
					if existing, ok := latestTasks[id]; ok {
						if parsed.Input.Status != "" {
							existing.status = parsed.Input.Status
						}
						if parsed.Input.Subject != "" {
							existing.subject = parsed.Input.Subject
						}
						latestTasks[id] = existing
					} else {
						latestTasks[id] = taskState{
							id:      id,
							subject: parsed.Input.Subject,
							status:  parsed.Input.Status,
						}
					}
				}
			case "TodoWrite":
				if len(parsed.Input.Todos) > 0 && len(todoItems) == 0 {
					for _, t := range parsed.Input.Todos {
						status := t.Status
						if status == "" {
							status = "pending"
						}
						content := t.Content
						if content == "" {
							content = t.Subject
						}
						icon := "O" // pending
						if status == "in_progress" {
							icon = "◐"
						} else if status == "completed" {
							icon = "●"
						}
						todoItems = append(todoItems, fmt.Sprintf("%s %s [%s]", icon, content, status))
					}
				}
			}
		}
	}

	var sb strings.Builder

	// Format task items
	if len(latestTasks) > 0 {
		sb.WriteString("## Tasks (recovered from transcript)\n\n")
		for _, t := range latestTasks {
			icon := "O"
			if t.status == "in_progress" {
				icon = "◐"
			} else if t.status == "completed" {
				icon = "●"
			}
			sb.WriteString(fmt.Sprintf("%s [%s] %s\n", icon, t.id, t.subject))
			if t.description != "" {
				sb.WriteString(fmt.Sprintf("  %s\n", t.description))
			}
		}
	}

	// Format todo items
	if len(todoItems) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## Todo List (recovered from transcript)\n\n")
		for _, item := range todoItems {
			sb.WriteString(item + "\n")
		}
		sb.WriteString("\nUse task_list, task_update, or TodoWrite to manage these items.\n")
	}

	return sb.String()
}

// injectTodoReminder re-injects the in-memory todo list into the conversation
// as a system-injected user message.
// Called after compaction when the todo reminder entry may have been dropped.
// The TodoList survives compaction in memory; this just ensures
// it's visible to the model.
func (a *AgentLoop) injectTodoReminder() {
	reminder := a.todoList.BuildReminder()
	if reminder == "" {
		return
	}
	a.context.InjectTodoReminder(reminder)
}

// buildPostCompactAgentAnnouncement re-announces active and completed-but-unretrieved
// background sub-agents after compaction. Matches upstream's createAsyncAgentAttachmentsIfNeeded
// which includes all agents with retrieved==false (running + completed but not yet collected).
func (a *AgentLoop) buildPostCompactAgentAnnouncement(preservedToolNames map[string]bool) string {
	tasks := a.agentTaskStore.List()
	if len(tasks) == 0 {
		return ""
	}

	var active []*tools.AgentTask
	var completedUnretrieved []*tools.AgentTask
	for _, t := range tasks {
		if t.Status == tools.TaskRunning || t.Status == tools.TaskPending {
			active = append(active, t)
		} else if t.Status == tools.TaskCompleted && !t.Notified {
			// Completed but results not yet retrieved by the user/main agent.
			// Include these to prevent the model from re-spawning the same task.
			completedUnretrieved = append(completedUnretrieved, t)
		}
	}

	if len(active) == 0 && len(completedUnretrieved) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Background Agents After Compaction\n\n")

	if len(active) > 0 {
		sb.WriteString("The following sub-agents are running. Do NOT spawn duplicates for the same task.\n\n")
		for _, t := range active {
			sb.WriteString(fmt.Sprintf("- agentId: %s, status: %s, description: %s\n", t.ID, t.Status, t.Description))
			if !t.StartTime.IsZero() {
				dur := time.Since(t.StartTime)
				sb.WriteString(fmt.Sprintf("  (running for %s)\n", dur.Round(time.Second)))
			}
		}
	}

	if len(completedUnretrieved) > 0 {
		sb.WriteString("\nThe following sub-agents completed but results have not been retrieved. Check their output before spawning duplicates.\n\n")
		for _, t := range completedUnretrieved {
			outputInfo := ""
			if t.OutputFile != "" {
				outputInfo = fmt.Sprintf(", output: %s", t.OutputFile)
			}
			sb.WriteString(fmt.Sprintf("- agentId: %s, status: completed, description: %s%s\n", t.ID, t.Description, outputInfo))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// buildPreCompactFileSnapshot generates a snapshot of recently read files' content
// from the registry's cache, for injection into the compaction summary.
// This matches upstream's post-compact file attachment restoration, but embedded
// directly in the summary text for the non-LLM compact path.
//
// Parameters match upstream defaults:
//   - maxFiles: maximum number of files to include (upstream POST_COMPACT_MAX_FILES_TO_RESTORE = 5, but we use more for the summary)
//   - maxTokensPerFile: per-file token cap (upstream POST_COMPACT_MAX_TOKENS_PER_FILE = 5000)
//   - maxTotalTokens: total token budget (upstream POST_COMPACT_TOKEN_BUDGET = 50000)
func (a *AgentLoop) buildPreCompactFileSnapshot(maxFiles int, maxTokensPerFile int, maxTotalTokens int) string {
	if a.registry == nil {
		return ""
	}

	// Collect all recently read files sorted by recency
	type fileEntry struct {
		path    string
		content string
		tokens  int
	}
	var entries []fileEntry
	for _, path := range a.registry.GetRecentlyReadFiles(maxFiles) {
		cached := a.registry.GetCachedFileContent(path)
		if cached == "" {
			continue
		}
		tokens := EstimateTokens(cached)
		if tokens > maxTokensPerFile {
			charLimit := maxTokensPerFile * 4
			if charLimit < len(cached) {
				truncated := cached[:charLimit] + "\n... [truncated for compaction; use Read to get full content]"
				tokens = EstimateTokens(truncated)
				entries = append(entries, fileEntry{path: path, content: truncated, tokens: tokens})
			} else {
				entries = append(entries, fileEntry{path: path, content: cached, tokens: tokens})
			}
		} else {
			entries = append(entries, fileEntry{path: path, content: cached, tokens: tokens})
		}
	}

	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	totalTokens := 0
	fileCount := 0
	sb.WriteString("## File Contents at Compaction Time\n")
	for _, e := range entries {
		if totalTokens+e.tokens > maxTotalTokens {
			break
		}
		sb.WriteString(fmt.Sprintf("\n### %s\n```go\n%s\n```", e.path, e.content))
		totalTokens += e.tokens
		fileCount++
	}

	if fileCount == 0 {
		return ""
	}
	sb.WriteString(fmt.Sprintf("\n\n[Snapshot of %d recently read files, ~%d tokens]\n", fileCount, totalTokens))
	return sb.String()
}

// buildStructuredGoalBlock extracts goal state from in-memory structures.
// Unlike upstream's LLM-based goal extraction (probabilistic), this uses
// deterministic structured state that survives compaction intact.
// Returns the goal block text and whether any content was generated.
func (a *AgentLoop) buildStructuredGoalBlock(messages []anthropic.MessageParam) (string, bool) {
	var sb strings.Builder

	// 1. Pending Tasks from TodoList — the model's primary "what's left to do"
	if a.todoList != nil {
		pending := a.todoList.GetPendingTasks()
		if len(pending) > 0 {
			sb.WriteString("## Pending Tasks\n")
			for i, item := range pending {
				status := string(item.Status)
				active := ""
				if item.ActiveForm != "" {
					active = " (" + item.ActiveForm + ")"
				}
				sb.WriteString(fmt.Sprintf("%d. [%s] %s%s\n", i+1, status, item.Content, active))
			}
		}
	}

	// 2. Completed Tasks — the anti-replay guard. Tells the model what's done
	// so it doesn't re-execute completed work after compaction.
	if a.todoList != nil {
		completed := a.todoList.GetCompletedTasks()
		if len(completed) > 0 {
			sb.WriteString("\n## Completed Work (DO NOT RE-EXECUTE)\n")
			for _, item := range completed {
				active := ""
				if item.ActiveForm != "" {
					active = " (" + item.ActiveForm + ")"
				}
				sb.WriteString(fmt.Sprintf("- %s%s\n", item.Content, active))
			}
		}
	}

	// 3. Active task from toolStateTracker — what the model claims it's doing
	if a.toolStateTracker != nil {
		activeTask := a.toolStateTracker.GetActiveTask()
		if activeTask != "" {
			sb.WriteString(fmt.Sprintf("\n## Current Work\nActive task: %s\n", activeTask))
		}
	}

	// 4. In-progress task from TodoList — cross-reference with tracker
	if a.todoList != nil {
		inProgress := a.todoList.GetInProgressTask()
		if inProgress != "" {
			sb.WriteString(fmt.Sprintf("Currently working on: %s\n", inProgress))
		}
	}

	// 5. ToolStateTracker conclusions — key findings the agent claimed
	if a.toolStateTracker != nil {
		conclusions := a.toolStateTracker.GetConclusions()
		if len(conclusions) > 0 {
			sb.WriteString("\n## Key Findings\n")
			for _, c := range conclusions {
				sb.WriteString(fmt.Sprintf("- %s\n", c))
			}
		}
	}

	// 6. Error memory — errors encountered, extracted structurally from tool results
	if len(messages) > 0 {
		errorMem := a.extractErrorMemory(messages)
		if len(errorMem) > 0 {
			sb.WriteString("\n## Errors Encountered\n")
			for _, e := range errorMem {
				sb.WriteString(fmt.Sprintf("- %s\n", e))
			}
		}
	}

	return sb.String(), sb.Len() > 0
}

// extractErrorMemory extracts error messages from tool results structurally.
// Unlike upstream's "Errors and fixes" section (LLM-extracted), this uses
// deterministic regex-based extraction from tool_result content.
func (a *AgentLoop) extractErrorMemory(messages []anthropic.MessageParam) []string {
	seen := make(map[string]bool)
	var errors []string
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult == nil {
				continue
			}
			for _, tc := range block.OfToolResult.Content {
				if tc.OfText == nil {
					continue
				}
				text := tc.OfText.Text
				for _, line := range strings.Split(text, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					if strings.Contains(strings.ToLower(line), "error") {
						if len(line) > 200 {
							line = line[:200] + "..."
						}
						if !seen[line] {
							seen[line] = true
							errors = append(errors, line)
						}
					}
				}
			}
		}
	}
	// Keep only the most recent errors (upstream only keeps what LLM remembers)
	if len(errors) > 10 {
		errors = errors[len(errors)-10:]
	}
	return errors
}

// buildCompactSummaryMessage generates a structured summary message for the non-LLM
// compact path, matching upstream's getCompactUserSummaryMessage format.
// It uses deterministic structured state from TodoList + toolStateTracker for
// goal preservation (surpassing upstream's purely LLM-based goal extraction).
//
// messages and recentToolCalls are the pre-compacted conversation data.
// If messages is nil, BuildMessages() is called (backwards compat).
func (a *AgentLoop) buildCompactSummaryMessage(preTokens int, messages []anthropic.MessageParam, recentToolCalls []string) string {
	var sb strings.Builder

	// Preamble matching upstream's getCompactUserSummaryMessage
	sb.WriteString("This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion of the conversation.\n\n")
	sb.WriteString(fmt.Sprintf("[compact: %d tokens compressed]\n", preTokens))

	// Include structured metadata from the full conversation before compaction.
	// This ensures the model sees an explicit inventory of files, tool calls,
	// and user messages — matching the LLM compact path's structured output.
	if messages == nil {
		messages = a.context.BuildMessages()
	}
	structuredMeta := entriesToSummaryTextForMessagesParams(messages)
	if structuredMeta != "" {
		sb.WriteString("\n## Structured context from compacted messages:\n")
		sb.WriteString(structuredMeta)
	}

	// Inject file content snapshot — the model's most recently read files with
	// their actual content at read time. This is the key anti-amnesia mechanism
	// matching upstream's post-compact file attachment restoration.
	// Uses registry cached content (what the model saw) not disk re-read.
	fileSnapshot := a.buildPreCompactFileSnapshot(10, 5000, 50000)
	if fileSnapshot != "" {
		sb.WriteString("\n")
		sb.WriteString(fileSnapshot)
	}

	// Inject the structured goal block — the key anti-amnesia mechanism.
	// This replaces upstream's purely LLM-based goal extraction with
	// deterministic structured state from TodoList + toolStateTracker.
	if goalBlock, hasContent := a.buildStructuredGoalBlock(messages); hasContent {
		sb.WriteString("\n")
		sb.WriteString(goalBlock)
	}

	sb.WriteString("\n(compact truncated the conversation — recent messages are preserved verbatim below)\n")

	// Anti-replay directive: explicit rules to prevent re-execution
	sb.WriteString("\n## Rules After Compaction\n")
	sb.WriteString("1. DO NOT re-execute any task listed in \"Completed Work\" — those are done.\n")
	sb.WriteString("2. Start from the first item in \"Pending Tasks\" that you have not yet completed.\n")

	// Transcript path for detail recovery.
	if tp := a.TranscriptPath(); tp != "" {
		sb.WriteString(fmt.Sprintf("3. If unsure what to do next, read the transcript at: %s.\n", tp))
	}
	sb.WriteString("4. Do NOT ask the user what to work on — you already know.\n")

	if tp := a.TranscriptPath(); tp != "" {
		sb.WriteString(fmt.Sprintf("\nIf you need specific details from before compaction (like exact code snippets, error messages, or content you generated), read the full transcript at: %s\n", tp))
	}

	sb.WriteString("\nRecent messages are preserved verbatim.\n\n")
	sb.WriteString("Continue the conversation from where it left off without asking the user any further questions. Resume directly — do not acknowledge the summary, do not recap what was happening, do not preface with \"I'll continue\" or similar. Pick up the last task as if the break never happened.")

	return sb.String()
}

// injectTruncationContinuation adds a CompactBoundary + summary after truncation-based
// recovery. Without this, the model receives truncated context with no directive to
// continue, causing it to re-execute old instructions or ask what to do.
// This matches the boundary+summary pattern used by the LLM compact path.
func (a *AgentLoop) injectTruncationContinuation(preTokens int) {
	if a.context == nil {
		return
	}
	a.context.AddCompactBoundary(CompactTriggerAuto, preTokens)
	summaryContent := a.buildCompactSummaryMessage(preTokens, nil, nil)
	a.context.AddSummary(summaryContent)
}

// extractRecentToolCallsForSummary returns a list of recent tool call descriptions
// for inclusion in compaction summaries. This helps the model understand what
// work was done before compaction truncated the conversation history.
func (a *AgentLoop) extractRecentToolCallsForSummary(n int) []string {
	if a.context == nil {
		return nil
	}
	entries := a.context.Entries()
	var results []string
	count := 0
	for i := len(entries) - 1; i >= 0 && count < n; i-- {
		entry := entries[i]
		if blocks, ok := entry.content.(ToolUseContent); ok {
			for j := len(blocks) - 1; j >= 0 && count < n; j-- {
				if b := blocks[j].OfToolUse; b != nil {
					inputMap, _ := b.Input.(map[string]any)
					desc := b.Name
					if inputMap != nil {
						if pathVal, ok := inputMap["path"].(string); ok {
							desc += " " + pathVal
						} else if cmdVal, ok := inputMap["command"].(string); ok {
							shortCmd := cmdVal
							if len(shortCmd) > 80 {
								shortCmd = shortCmd[:80] + "..."
							}
							desc += " " + shortCmd
						} else if queryVal, ok := inputMap["query"].(string); ok {
							desc += " " + queryVal
						} else if promptVal, ok := inputMap["prompt"].(string); ok {
							short := promptVal
							if len(short) > 80 {
								short = short[:80] + "..."
							}
							desc += " " + short
						}
					}
					results = append([]string{desc}, results...)
					count++
				}
			}
		}
	}
	return results
}

// InjectRunningAgentStatus adds attachments for sub-agents that are still
// running in the background. This prevents the model from spawning duplicate
// agents after compaction. Matches upstream's createAsyncAgentAttachmentsIfNeeded.
func (a *AgentLoop) InjectRunningAgentStatus() {
	if a.agentTaskStore == nil {
		return
	}
	tasks := a.agentTaskStore.ListByStatus(tools.TaskRunning)
	for _, task := range tasks {
		statusLine := fmt.Sprintf(
			"[task_status] taskId: %s, type: local_agent, description: %s, status: running\nThis agent is still running in the background. Do NOT spawn a duplicate agent for this task.",
			task.ID, task.Description,
		)
		a.context.AddAttachment(statusLine)
	}
}

// runPerTurnMicroCompact executes lightweight micro-compact at the start of each
// turn to prevent old tool results from accumulating between full compactions.
// This matches upstream's per-turn microCompact call in query.ts.
//
// Strategy: prefer the cached MC path (cache_edits API) which deletes tool
// results server-side WITHOUT modifying local messages, preserving the prompt
// cache prefix. Fall back to local content-replacement (MicroCompactEntries)
// only when cached MC has no pending deletions — typically after a full
// compaction reset or when below the trigger threshold.
func (a *AgentLoop) runPerTurnMicroCompact() {
	if !a.config.MicroCompactEnabled {
		return
	}

	// Phase 1: Try cached micro-compact (cache_edits API path).
	// This is the preferred approach — it deletes tool results server-side
	// while preserving the cached prefix, matching upstream's cached MC.
	// injectCacheEdits() is called later in the API call path to actually
	// inject the block, but we need to check here if deletions are pending
	// to avoid the destructive local MC path.
	if a.cachedMC != nil && a.cachedMC.HasPendingDeletions() {
		a.logDebug("\n[per-turn-micro-compact] Cached MC has pending deletions, using cache_edits path\n")
		return
	}

	// Phase 2: Fall back to local content-replacement when cached MC is
	// not applicable (below threshold, or after a full compaction reset).
	// This DOES break the prompt cache, but only fires when cached MC
	// can't help (e.g., fewer than 10 compactable tool results accumulated).
	keepRecent := a.config.MicroCompactKeepRecent
	if keepRecent <= 0 {
		keepRecent = 5
	}
	cleared := a.context.MicroCompactEntries(keepRecent, a.config.MicroCompactPlaceholder, a.config.MicroCompactMinCharCount)
	if cleared > 0 {
		a.logDebug("\n[per-turn-micro-compact] Local MC: cleared %d old tool results\n", cleared)
		// Local MC mutates message content, invalidating the server cache.
		// Reset cached MC state to prevent stale cache_edit attempts.
		a.cachedMC.ResetForTimeBasedMC()
	}
}

// tryCompaction attempts LLM-driven compaction, falling back to truncation.
// When session memory exists and has content, uses SM-compact (API-free compaction)
// to skip the LLM call and use session memory as the summary directly.

// decidePostResponseFold implements DeepSeek-Reasonix's multi-tier fold decision.
// After each API response, checks if token usage exceeds fold thresholds and
// decides the appropriate compaction strategy:
//   - ratio > FORCE_SUMMARY_THRESHOLD (0.80): exit with summary
//   - ratio > HISTORY_FOLD_AGGRESSIVE_THRESHOLD (0.78): aggressive fold (10% tail)
//   - ratio > HISTORY_FOLD_THRESHOLD (0.75): normal fold (20% tail)
func (a *AgentLoop) decidePostResponseFold(promptTokens int, ctxMax int) {
	if promptTokens <= 0 || ctxMax <= 0 {
		return
	}
	ratio := float64(promptTokens) / float64(ctxMax)

	if ratio > FORCE_SUMMARY_THRESHOLD {
		// Above 80%: exit with summary instead of folding
		a.logDebug("[multi-tier-fold] ratio=%.2f > %.2f (FORCE_SUMMARY): exiting with summary\n",
			ratio, FORCE_SUMMARY_THRESHOLD)
		// The loop will naturally complete; the user can see the summary.
		return
	}
	if ratio > HISTORY_FOLD_AGGRESSIVE_THRESHOLD {
		// Above 78%: aggressive fold with 10% tail budget
		a.logDebug("[multi-tier-fold] ratio=%.2f > %.2f (AGGRESSIVE): folding with 10%% tail\n",
			ratio, HISTORY_FOLD_AGGRESSIVE_THRESHOLD)
		a.tryCompaction()
		a.RunPostCompactCleanup()
		return
	}
	if ratio > HISTORY_FOLD_THRESHOLD {
		// Above 75%: normal fold with 20% tail budget
		a.logDebug("[multi-tier-fold] ratio=%.2f > %.2f (NORMAL): folding with 20%% tail\n",
			ratio, HISTORY_FOLD_THRESHOLD)
		a.tryCompaction()
		a.RunPostCompactCleanup()
		return
	}
	// Below 75%: no fold needed
}

// tryCompaction:
// When session memory exists and has content, uses SM-compact (API-free compaction)
// to skip the LLM call and use session memory as the summary directly.
func (a *AgentLoop) tryCompaction() {
	a.logDebug("[compact] tryCompaction called: est_tokens=%d\n", a.context.EstimatedTokens())

	// Phase 0: Micro-compact — clear old tool results (cheap, no LLM call)
	// Time-based trigger: only fire when gap since last assistant > threshold (default 60 min).
	// When gapMinutes=0, fires every turn (legacy count-based behavior).
	if a.config.MicroCompactEnabled && a.context.ShouldTimeBasedMicroCompact(a.config.MicroCompactGapMinutes) {
		keepRecent := a.config.MicroCompactKeepRecent
		if keepRecent <= 0 {
			keepRecent = 5
		}
		cleared := a.context.MicroCompactEntries(keepRecent, a.config.MicroCompactPlaceholder, a.config.MicroCompactMinCharCount)
		if cleared > 0 {
			a.logDebug("\n[micro-compact] Cleared %d old tool results\n", cleared)
			// Time-based microcompact content-clears tool results and invalidates the
			// server prompt cache. The cached MC state would reference non-existent
			// server entries, so reset it to prevent stale cache_edit attempts.
			// This matches upstream's resetMicrocompactState() after time-based MC.
			a.cachedMC.ResetForTimeBasedMC()
			// NOTE: do NOT call toolStateTracker.OnCompaction() here.
			// Micro-compact clears OLD tool results (beyond keepRecent threshold) by
			// replacing their text with placeholders. This is lightweight text replacement,
			// not a structural context compaction. The files and searches themselves
			// remain relevant — only the detailed output is trimmed. Incrementing the
			// epoch here would incorrectly mark all files and searches as stale, causing
			// the Session State note to say "RE-READ if needed" for files whose
			// content is still in context. The epoch advances only during real compaction
			// (where context is structurally reduced and the summary may miss details).
		}
	}

	if a.compactor == nil {
		preTokens := a.context.EstimatedTokens()
		// Capture messages and recent tool calls BEFORE compaction clears entries.
		// buildCompactSummaryMessage needs the full conversation context to generate
		// a useful summary; after CompactContext(), entries are truncated and the
		// summary would show "0 conversation turns with 0 tool calls".
		preCompactMessages := a.context.BuildMessages()
		preCompactToolCalls := a.extractRecentToolCallsForSummary(5)
		if a.context.CompactContext() {
			// CompactContext truncates messages but doesn't add a continuation directive.
			// Without one, the model sees an incomplete conversation and re-executes
			// historical instructions instead of continuing. Inject boundary + summary
			// with continuation directive, matching the SM-compact and LLM-compact paths.
			a.context.AddCompactBoundary(CompactTriggerAuto, preTokens)

			// Build a structured summary matching upstream's getCompactUserSummaryMessage
			// format. Without this, the model sees a bare "[compact: N tokens]" and
			// re-executes historical instructions instead of continuing.
			summaryContent := a.buildCompactSummaryMessage(preTokens, preCompactMessages, preCompactToolCalls)
			// DeepSeek-Reasonix pattern: prepend pinned content that must survive compaction
			if a.foldSummaryPin != nil {
				a.foldSummaryPin.SetSystemPrompt(a.context.SystemPrompt())
				pinPrompt := a.foldSummaryPin.BuildPinPrompt()
				if pinPrompt != "" {
					summaryContent = pinPrompt + "\n\n" + summaryContent
				}
			}
			a.context.AddSummary(summaryContent)

			if a.toolStateTracker != nil {
				a.toolStateTracker.OnCompaction()
			}
			a.InjectRunningAgentStatus()

			// Persist compaction boundary and summary to transcript for resume support.
			// Without this, --resume replays the full un-compacted history.
			if a.transcript != nil {
				_ = a.transcript.WriteCompact("compact_context", preTokens)
				_ = a.transcript.WriteSummary(summaryContent)
			}

			// Post-compact recovery: re-inject recently read files
			recoveredPaths := a.PostCompactRecovery(HookTriggerAuto, summaryContent)
			if a.toolStateTracker != nil {
				for _, path := range recoveredPaths {
					a.toolStateTracker.MarkFileFresh(path)
				}
				// NOTE: RunPostCompactCleanup() (called from PostCompactRecovery above)
				// already saves conclusions to session memory and clears them.
				// Do NOT save/clear conclusions here to avoid double-operation.
			}

			// Keep recent messages — preserve actual message objects with tool structure intact.
			// Use adaptive token-based calculation matching SM-compact (10K min, 5 text msgs, 40K max)
			// instead of fixed count. Fixed count is too small for large tool results and too
			// large for small text messages.
			a.context.KeepRecentMessagesAdaptive(10_000, 5, 40_000)
			a.context.ValidateToolPairing()
			a.context.FixRoleAlternation()

			// Calculate post-compact token count for cooldown tracking.
			// Without this, postCompactTokens stays 0 and the cooldown check never activates,
			// causing the next turn to immediately try compaction again even if it just ran.
			postCompactMessages := a.context.BuildMessages()
			postCompactTokens := estimateMessageParamsTokens(postCompactMessages)
			a.compactor.SetPostCompactTokens(postCompactTokens)
		}
		return
	}

	// Execute pre-compact hooks before any compaction action.
	// Hooks can inject custom instructions that affect the compaction summary.
	var preCompactInst string
	if a.hooks != nil {
		hookInput := PreCompactInput{
			Trigger:            HookTriggerAuto,
			CustomInstructions: "",
		}
		if hookResult, err := a.hooks.ExecutePreCompactHooks(hookInput); err == nil {
			preCompactInst = hookResult.CustomInstructions
		}
	}

	// Phase 1: SM-compact — use session memory as summary instead of calling LLM API.
	// This is the preferred path when memory is available: saves an LLM API call
	// and leverages incrementally collected session memory as the context summary.
	// Uses the full structured session_memory.md content (10-section template) rather
	// than the flattened FormatForPromptCompact output — the structured template
	// matches upstream's getSessionMemoryContent() behavior and provides much richer
	// context for post-compact recovery.
	if a.config.SessionMemory != nil {
		// Wait for any in-progress session memory extraction to complete.
		// Matching upstream's waitForSessionMemoryExtraction() in trySessionMemoryCompaction.
		// If extraction is stale (>60s old) or timed out, proceed anyway.
		if a.extractionState != nil {
			a.extractionState.WaitForExtraction(15 * time.Second)
		}

		var smContent string

		// Try reading the actual session_memory.md file (structured 10-section template)
		memoryPath := filepath.Join(a.config.ProjectDir, ".claude", "session_memory.md")
		if data, err := os.ReadFile(memoryPath); err == nil {
			content := strings.TrimSpace(string(data))
			// Only use if content has actual user-written content (not just the template)
			if content != "" && !IsSessionMemoryTemplateOnly(content) {
				smContent = content
			}
		}

		// Fall back to the flattened FormatForPromptCompact if file doesn't exist or is empty
		if smContent == "" {
			smContent = a.config.SessionMemory.FormatForPromptCompact()
		}

		if smContent != "" {
			a.trySMCompact(smContent, preCompactInst)
			// Mark system prompt dirty after compaction
			if a.config.cachedPrompt != nil {
				a.config.cachedPrompt.MarkDirty()
			}
			return
		}
	}

	// Phase 2: LLM-driven compaction (existing path)
	a.tryLLMCompaction(preCompactInst)

	// Mark system prompt dirty after compaction
	if a.config.cachedPrompt != nil {
		a.config.cachedPrompt.MarkDirty()
	}
}

// trySMCompact performs compaction using session memory as the summary,
// skipping the LLM API call entirely. Inspired by the official Claude Code
// SM-compact mechanism (sessionMemoryCompact.ts).
func (a *AgentLoop) trySMCompact(sessionMemoryContent string, preCompactInst string) {
	messages := a.context.BuildMessages()
	preTokens := estimateMessageParamsTokens(messages)

	if !a.compactor.ShouldCompact(messages) {
		return // Not enough tokens to justify compaction
	}

	// Cooldown: skip if tokens haven't grown 25% since last compaction
	// (handled inside ShouldCompact, but double-check pre-compact tokens)
	a.compactor.mu.Lock()
	postTokens := a.compactor.postCompactTokens
	a.compactor.mu.Unlock()
	if postTokens > 0 {
		cooldownThreshold := postTokens + postTokens/4
		if preTokens < cooldownThreshold {
			return // Still in cooldown period
		}
	}

	// Advance compaction epoch BEFORE clearing context — marks all tracked items as stale.
	// After this point, items from the previous epoch are marked "cleared from context".
	if a.toolStateTracker != nil {
		a.toolStateTracker.OnCompaction()
	}

	// Clear stale state entries from session memory — the old state context is no
	// longer valid after compaction, and the new compaction will write fresh state.
	if a.config.SessionMemory != nil {
		a.config.SessionMemory.ClearStateEntries()
	}

	// Build structured metadata from the messages being compacted.
	// This ensures the model sees an explicit inventory of files, tool calls,
	// and user messages even when session memory only has high-level notes.
	// Matches upstream's structured_meta injection in do_compact_llm_call.
	structuredMeta := entriesToSummaryTextForMessagesParams(messages)
	if structuredMeta != "" {
		structuredMeta = "\n\n## Structured context from compacted messages:\n" + structuredMeta
	}

	// Write structured worklog and error entries to session memory before compaction.
	// This populates the previously empty Worklog and Errors sections in session_memory.md
	// with concrete entries extracted from the messages being compacted.
	if a.config.SessionMemory != nil && structuredMeta != "" {
		worklogEntries := extractWorklogFromStructuredMeta(structuredMeta)
		for _, entry := range worklogEntries {
			a.config.SessionMemory.AddNote("worklog", entry, "auto")
		}
		errorEntries := extractErrorsFromMessagesParams(messages)
		for _, entry := range errorEntries {
			a.config.SessionMemory.AddNote("error", entry, "auto")
		}
	}

	// Inject file content snapshot — reduced size (5 files, 20K total)
	// to avoid bloating the summary. The model can re-read files it needs
	// after compaction via PostCompactRecovery.
	fileSnapshot := a.buildPreCompactFileSnapshot(5, 5000, 20000)
	if fileSnapshot != "" {
		fileSnapshot = "\n" + fileSnapshot
	}

	// Format the session memory as a compact summary
	// Cap session memory content at ~40K tokens to prevent context overflow.
	// Matches upstream's DEFAULT_SM_COMPACT_CONFIG.maxTokens = 40_000.
	// Uses per-section truncation (truncateSessionMemoryForCompact) to preserve
	// section headers while truncating oversized sections.
	const maxSessionMemoryTokens = 40_000
	smTokens := EstimateTokens(sessionMemoryContent)
	smContentForSummary := sessionMemoryContent
	if smTokens > maxSessionMemoryTokens {
		smContentForSummary = truncateSessionMemoryForCompact(sessionMemoryContent, maxSessionMemoryTokens)
		a.logDebug("\n[sm-compact] Session memory truncated: %d tokens -> %d token limit\n", smTokens, maxSessionMemoryTokens)
	}

	boundaryText := fmt.Sprintf("[SM-compact: %d tokens compressed, session memory used as summary]", preTokens)
	// Match upstream's getCompactUserSummaryMessage: add transcript path for
	// detail recovery, recentMessagesPreserved notice, and continuation instruction.
	summaryContent := "This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion of the conversation.\n\n" + boundaryText + "\n\n" + smContentForSummary + structuredMeta + fileSnapshot
	if tp := a.TranscriptPath(); tp != "" {
		summaryContent += fmt.Sprintf("\n\nIf you need specific details from before compaction (like exact code snippets, error messages, or content you generated), read the full transcript at: %s", tp)
	}

	// Inject deterministic goal block — pending/completed tasks, current work,
	// key findings, and errors. SM-compact uses session memory as the summary
	// which has no explicit task tracking, so the structured goal block is the
	// model's only signal for what's pending vs done.
	if goalBlock, hasContent := a.buildStructuredGoalBlock(messages); hasContent {
		summaryContent += "\n\n" + goalBlock
	}

	summaryContent += "\n\nRecent messages are preserved verbatim.\n\nContinue the conversation from where it left off without asking the user any further questions. Resume directly — do not acknowledge the summary, do not recap what was happening, do not preface with \"I'll continue\" or similar. Pick up the last task as if the break never happened."

	// Anti-replay directive: explicit rules to prevent re-execution
	summaryContent += "\n\n## Rules After Compaction\n"
	summaryContent += "1. DO NOT re-execute any task listed in \"Completed Work\" — those are done.\n"
	summaryContent += "2. Start from the first item in \"Pending Tasks\" that you have not yet completed.\n"
	if tp := a.TranscriptPath(); tp != "" {
		summaryContent += fmt.Sprintf("3. If unsure what to do next, read the transcript at: %s.\n", tp)
	}
	summaryContent += "4. Do NOT ask the user what to work on — you already know.\n"

	if preCompactInst != "" {
		summaryContent += "\n\n## Custom instructions for this compaction:\n" + preCompactInst
	}

	a.logDebug("\n[sm-compact] Using session memory as summary (%d tokens -> ~%d tokens)\n",
		preTokens, EstimateTokens(summaryContent)+6)

	// Inject boundary + summary into context
	a.context.AddCompactBoundary(CompactTriggerSMCompact, preTokens)
	a.context.AddSummary(summaryContent)

	// Persist compaction boundary and summary to transcript for resume support.
	if a.transcript != nil {
		_ = a.transcript.WriteCompact("sm-compact", preTokens)
		_ = a.transcript.WriteSummary(summaryContent)
	}

	// Update session memory with compaction state
	if a.config.SessionMemory != nil {
		a.config.SessionMemory.AddNote("state", fmt.Sprintf("Compaction (sm-compact): %d tokens compressed", preTokens), "auto")
	}

	// Phase 2: Keep recent messages — preserve with tool structure intact.
	// Run BEFORE post-compact recovery so attachments appear AFTER kept messages.
	// KeepRecentMessagesAdaptive uses token-based adaptive calculation instead of fixed count,
	// matching upstream's calculateMessagesToKeepIndex:
	//   - minTokens: enough context for recovery (~10K tokens)
	//   - minTextMsgs: ensure at least 5 text messages are visible post-compact
	//   - maxTokens: cap tail at ~40K to avoid bloating context with tool results
	a.context.KeepRecentMessagesAdaptive(10_000, 5, 40_000)

	// Fix message structure after KeepRecentMessages: remove orphaned tool_results
	// (whose tool_use was in the summarized portion) and merge consecutive same-role
	// messages. Without this, the API returns error 2013 for invalid message structure.
	a.context.ValidateToolPairing()
	a.context.FixRoleAlternation()

	// Phase 3: Post-compact recovery — re-inject critical context
	recoveredPaths := a.PostCompactRecovery(HookTriggerSM, summaryContent)

	// Phase 3b: Inject running agent status so model doesn't spawn duplicates
	a.InjectRunningAgentStatus()

	// Mark recovered files as fresh (content is back in context).
	// Also mark ALL tracked files as fresh if no specific recovery was done
	// (the summary now contains the distilled knowledge).
	// RunPostCompactCleanup (called from PostCompactRecovery) handles ClearConclusions.
	if a.toolStateTracker != nil {
		for _, path := range recoveredPaths {
			a.toolStateTracker.MarkFileFresh(path)
		}
	}

	// Calculate real post-compact token count for cooldown
	actualMessages := a.context.BuildMessages()
	actualPostTokens := estimateMessageParamsTokens(actualMessages)
	a.compactor.SetPostCompactTokens(actualPostTokens)

	// Post-compact threshold check: if SM-compact didn't reduce tokens below
	// the autocompact threshold, fall back to LLM compaction. This matches
	// upstream's autoCompactThreshold check in trySessionMemoryCompaction.
	compactThreshold := a.compactor.CompactThreshold()
	if actualPostTokens >= compactThreshold {
		a.logDebug("\n[sm-compact] Post-compact tokens (%d) still above threshold (%d), falling back to LLM compaction\n",
			actualPostTokens, compactThreshold)
		// Undo SM-compact and fall back to LLM compaction.
		// tryLLMCompaction will re-check ShouldCompact and may skip if tokens
		// are too low, which is the correct behavior (context pressure is gone).
		a.tryLLMCompaction("")
		return
	}

	// Track lastSummarizedMessageUUID for incremental SM-compact.
	// The compaction boundary is inserted before summary, so the boundary's
	// UUID marks the end of the summarized portion. Subsequent compactions
	// will only compact forward from this point.
	// Mirrors upstream's setLastSummarizedMessageId() called after compaction.
	if a.config.SessionMemory != nil {
		a.config.SessionMemory.SetLastSummarizedMessageUUID(a.context.LastCompactBoundaryUUID())
	}
}

// tryLLMCompaction performs LLM-driven compaction using the insert-then-compress
// pattern from openclacky: inject a compression instruction into the existing
// conversation, make a single API call (reusing cached system prompt + tools),
// parse the <topics> + <summary> response, and rebuild context.
// Returns true if compaction was performed.
func (a *AgentLoop) tryLLMCompaction(preCompactInst string) {
	messages := a.context.BuildMessages()

	// Advance compression level for progressive summarization.
	// Higher levels produce shorter summaries, preventing summary bloat
	// across multiple compactions. Inspired by openclacky's hierarchical
	// summarization (Level 1=full, 2=concise, 3=minimal, 4+=ultra-minimal).
	level := a.context.NextCompressionLevel()

	// Chunk archival: archive pre-compaction messages to disk for
	// on-demand recall. Matches openclacky's chunk archival pattern.
	var chunkPath string
	var prevChunksIndex string
	if a.config.ProjectDir != "" {
		chunkMsgs := messagesToChunkMessages(messages)
		chunkPath = archiveChunkMessages(a.config.ProjectDir, level, chunkMsgs)
		// Discover existing chunks and build index for the summary
		prevChunksIndex = tools.BuildPreviousChunksIndex(
			tools.ListChunks(a.config.ProjectDir, ""))
	}

	// ─── Insert-then-compress (openclacky pattern) ────────────────────────────
	// Instead of making a separate API call with its own prompt, inject the
	// compression instruction into the conversation messages and make the API
	// call with the instruction already embedded. This reuses the existing
	// prompt cache prefix (system prompt + tools + prior messages are cached,
	// only the instruction text itself is new tokens).
	//
	// The instruction is built and appended as the final user message:
	compressPrompt := buildCompressionPrompt(level)
	finalMsgs := make([]anthropic.MessageParam, len(messages)+1)
	copy(finalMsgs, messages)
	finalMsgs[len(messages)] = anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: SystemInjectedPrefix + compressPrompt}},
		},
	}

	// Call the inline compaction API (no additional prompt appended — the
	// instruction is already in the messages as the final user message).
	result, err := doInlineCompactLLMCall(finalMsgs, a.config.Model, a.config.APIKey, a.config.BaseURL, a.context.SystemPrompt(), a.TranscriptPath(), level)

	if err != nil || result.Summary == "" {
		// Fall back to the existing separate-call path
		a.logDebug("\n[Compaction] inline compaction failed or empty, falling back to separate call\n")
		summary, performed := a.compactor.Compact(messages, a.config.Model, a.config.APIKey, a.config.BaseURL, a.context.SystemPrompt(), a.TranscriptPath())
		if performed && summary != "" {
			// Augment summary with chunk path reference and previous chunks index
			if chunkPath != "" {
				summary = augmentSummaryWithChunk(summary, chunkPath)
			}
			if prevChunksIndex != "" {
				summary += prevChunksIndex
			}
			// Advance compaction epoch BEFORE clearing context — marks all tracked items as stale.
			if a.toolStateTracker != nil {
				a.toolStateTracker.OnCompaction()
			}
			// Clear stale state entries from session memory - the old state context is no
			// longer valid after compaction, and the new compaction will write fresh state.
			if a.config.SessionMemory != nil {
				a.config.SessionMemory.ClearStateEntries()
			}

			// Inject boundary marker and summary into context
			preTokens := a.context.EstimatedTokens()

			// Capture discovered tool names before compaction — the summary doesn't
			// preserve tool_reference blocks, so post-compact recovery needs this
			// to keep sending already-loaded deferred tool schemas to the API.
			discoveredTools := collectDiscoveredToolNames(a.context)

			a.context.AddCompactBoundary(CompactTriggerAuto, preTokens,
				func(bc *CompactBoundaryContent) {
					if len(discoveredTools) > 0 {
						bc.PreCompactDiscoveredTools = discoveredTools
					}
				},
			)
			a.context.AddSummary(summary)

			// Anti-replay rules: injected as a separate system message so they
			// survive compaction even when the LLM summary is compressed further.
			// This matches the SM-compact and truncation paths which include the
			// same rules.
			a.context.AddAntiReplayRules()

			// Inject structured goal block — deterministic task state from TodoList
			// + toolStateTracker. Without this, the LLM-generated summary may lack
			// explicit completed/pending task distinction.
			if goalBlock, hasContent := a.buildStructuredGoalBlock(messages); hasContent {
				a.context.AddGoalBlock(goalBlock)
			}

			if preCompactInst != "" {
				a.context.AddSummary("\n\n## Custom instructions for this compaction:\n" + preCompactInst)
			}

			// Persist compaction boundary and summary to transcript for resume support.
			// Without this, --resume replays the full un-compacted history.
			if a.transcript != nil {
				_ = a.transcript.WriteCompact("auto", preTokens)
				_ = a.transcript.WriteSummary(summary)
			}

			// Save compaction state to session memory — store a compact token count marker,
			// not the full summary text (which bloats session memory for future sessions).
			if a.config.SessionMemory != nil {
				a.config.SessionMemory.AddNote("state", fmt.Sprintf("Compaction (auto): %d tokens compressed", preTokens), "auto")
				// Track lastSummarizedMessageUUID for incremental SM-compact.
				// Subsequent compactions will only compact forward from this point.
				a.config.SessionMemory.SetLastSummarizedMessageUUID(a.context.LastCompactBoundaryUUID())
			}

			// Phase 2: Keep recent messages — preserve with tool structure intact.
			// Run BEFORE post-compact recovery so attachments appear AFTER kept messages.
			// Use adaptive token-based calculation (10K min, 5 text msgs, 40K max)
			// matching SM-compact path. Fixed count is too small for large tool results
			// and too large for small text messages.
			a.context.KeepRecentMessagesAdaptive(10_000, 5, 40_000)

			// Fix message structure after KeepRecentMessages: remove orphaned tool_results
			// (whose tool_use was in the summarized portion) and merge consecutive same-role
			// messages. Without this, the API returns error 2013 for invalid message structure.
			a.context.ValidateToolPairing()
			a.context.FixRoleAlternation()

			// Phase 3: Post-compact recovery — re-inject critical context
			recoveredPaths := a.PostCompactRecovery(HookTriggerAuto, summary)

			// Phase 3b: Inject running agent status so model doesn't spawn duplicates
			a.InjectRunningAgentStatus()

			// Mark recovered files as fresh (content is back in context).
			// NOTE: RunPostCompactCleanup() (called from PostCompactRecovery above)
			// already saves conclusions to session memory and clears them.
			// The outer conditional that checked len(recoveredPaths) was removed —
			// conclusions should ALWAYS be saved before clearing, regardless of
			// whether files were recovered (conclusions contain semantic knowledge
			// that file content doesn't capture, like "the bug was in line 42").
			if a.toolStateTracker != nil {
				for _, path := range recoveredPaths {
					a.toolStateTracker.MarkFileFresh(path)
				}
			}

			// Rebuild messages from the actual context (summary + attachments + any tail entries)
			// and calculate the real post-compact token count for cooldown.
			actualMessages := a.context.BuildMessages()
			postTokens := estimateMessageParamsTokens(actualMessages)
			a.compactor.SetPostCompactTokens(postTokens)
			return
		}

		// LLM compaction was not performed (not needed or disabled).
		// Do NOT fall through to CompactContext() -- the LLM compactor's
		// ShouldCompact() check already determined that compaction isn't needed.
		return
	}

	// ─── Inline compaction succeeded ──────────────────────────────────────────
	summary := result.Summary
	topics := result.Topics

	// Augment summary with chunk path reference and previous chunks index
	if chunkPath != "" {
		summary = augmentSummaryWithChunk(summary, chunkPath)
	}
	if prevChunksIndex != "" {
		summary += prevChunksIndex
	}

	// Advance compaction epoch BEFORE clearing context — marks all tracked items as stale.
	if a.toolStateTracker != nil {
		a.toolStateTracker.OnCompaction()
	}
	// Clear stale state entries from session memory - the old state context is no
	// longer valid after compaction, and the new compaction will write fresh state.
	if a.config.SessionMemory != nil {
		a.config.SessionMemory.ClearStateEntries()
	}

	// Inject boundary marker and summary into context
	preTokens := a.context.EstimatedTokens()

	// Capture discovered tool names before compaction — the summary doesn't
	// preserve tool_reference blocks, so post-compact recovery needs this
	// to keep sending already-loaded deferred tool schemas to the API.
	discoveredTools := collectDiscoveredToolNames(a.context)

	a.context.AddCompactBoundary(CompactTriggerAuto, preTokens,
		func(bc *CompactBoundaryContent) {
			if len(discoveredTools) > 0 {
				bc.PreCompactDiscoveredTools = discoveredTools
			}
		},
	)

	// Use CompressedSummaryContent instead of raw SummaryContent — this preserves
	// topics metadata and chunk path for on-demand recall.
	a.context.AddCompressedSummary(summary, topics, chunkPath)

	// Anti-replay rules: injected as a separate system message so they
	// survive compaction even when the LLM summary is compressed further.
	a.context.AddAntiReplayRules()

	// Inject structured goal block — deterministic task state from TodoList
	// + toolStateTracker. Without this, the LLM-generated summary may lack
	// explicit completed/pending task distinction.
	if goalBlock, hasContent := a.buildStructuredGoalBlock(messages); hasContent {
		a.context.AddGoalBlock(goalBlock)
	}

	if preCompactInst != "" {
		a.context.AddSummary("\n\n## Custom instructions for this compaction:\n" + preCompactInst)
	}

	// Persist compaction boundary and summary to transcript for resume support.
	// Without this, --resume replays the full un-compacted history.
	if a.transcript != nil {
		_ = a.transcript.WriteCompact("auto", preTokens)
		_ = a.transcript.WriteSummary(summary)
	}

	// Save compaction state to session memory
	if a.config.SessionMemory != nil {
		a.config.SessionMemory.AddNote("state", fmt.Sprintf("Compaction (auto, inline): %d tokens compressed", preTokens), "auto")
		a.config.SessionMemory.SetLastSummarizedMessageUUID(a.context.LastCompactBoundaryUUID())
	}

	// Phase 2: Keep recent messages
	a.context.KeepRecentMessagesAdaptive(10_000, 5, 40_000)

	// Fix message structure after KeepRecentMessages
	a.context.ValidateToolPairing()
	a.context.FixRoleAlternation()

	// Phase 3: Post-compact recovery — re-inject critical context
	recoveredPaths := a.PostCompactRecovery(HookTriggerAuto, summary)

	// Phase 3b: Inject running agent status so model doesn't spawn duplicates
	a.InjectRunningAgentStatus()

	// Mark recovered files as fresh
	if a.toolStateTracker != nil {
		for _, path := range recoveredPaths {
			a.toolStateTracker.MarkFileFresh(path)
		}
	}

	// Rebuild messages and calculate post-compact token count
	actualMessages := a.context.BuildMessages()
	postTokens := estimateMessageParamsTokens(actualMessages)
	a.compactor.SetPostCompactTokens(postTokens)

	fmt.Fprintf(os.Stderr, "\n[Compaction] inline: %d messages compressed, topics=[%s]\n",
		len(messages), topics)
}

// messagesToChunkMessages converts API message params into simplified
// ChunkMessage format for archival to disk.
func messagesToChunkMessages(messages []anthropic.MessageParam) []tools.ChunkMessage {
	var result []tools.ChunkMessage
	for _, msg := range messages {
		role := string(msg.Role)
		var content strings.Builder
		for _, block := range msg.Content {
			if block.OfText != nil {
				content.WriteString(block.OfText.Text)
			} else if block.OfToolResult != nil {
				for _, cb := range block.OfToolResult.Content {
					if cb.OfText != nil {
						content.WriteString(cb.OfText.Text)
					}
				}
			} else if block.OfToolUse != nil {
				fmt.Fprintf(&content, "_Tool call: %s_", block.OfToolUse.Name)
			}
		}
		text := content.String()
		if text == "" {
			continue
		}
		result = append(result, tools.ChunkMessage{
			Role:    role,
			Content: text,
		})
	}
	return result
}

// archiveChunkMessages writes conversation messages to a chunk .md file
// on disk and returns the path. Returns empty string on error or if
// projectDir is empty. Matches openclacky's chunk archival pattern.
func archiveChunkMessages(projectDir string, level int, msgs []tools.ChunkMessage) string {
	if projectDir == "" || len(msgs) == 0 {
		return ""
	}
	// Use the tool-results directory's parent as the base for chunks
	chunkDir := filepath.Join(projectDir, "chunks")
	chunkIndex := tools.NextChunkIndex(chunkDir, "")
	path, err := tools.ArchiveChunk(chunkDir, "", chunkIndex, level, "", msgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[WARN] Chunk archival failed: %v\n", err)
		return ""
	}
	return path
}

// augmentSummaryWithChunk appends a chunk path reference to the summary,
// enabling on-demand recall via file_reader. Matches openclacky's pattern.
func augmentSummaryWithChunk(summary, chunkPath string) string {
	return summary + fmt.Sprintf("\n\n---\nCurrent chunk archived at: %s\nUse file_reader to recall details from this chunk.", chunkPath)
}

// isLocalEndpoint detects if the base URL points to a local provider.
func isLocalEndpoint(baseURL string) bool {
	lower := strings.ToLower(baseURL)
	return strings.Contains(lower, "localhost") ||
		strings.Contains(lower, "127.0.0.1") ||
		strings.Contains(lower, "0.0.0.0") ||
		strings.Contains(lower, "::1") ||
		strings.HasSuffix(lower, ".local") ||
		strings.Contains(lower, "://localhost:")
}

// estimateMessageTokens roughly estimates token count (~4 chars per token).
func estimateMessageTokens(messages []anthropic.MessageParam) int {
	totalChars := 0
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfText != nil {
				totalChars += len(block.OfText.Text)
			}
		}
	}
	return totalChars / 4
}

// runSessionMemoryExtraction runs a forked agent to update session_memory.md.
// It captures the parent's cache-safe params and uses a restricted canUseTool
// (only edit_file on the session memory file). Runs asynchronously in a goroutine.
func (a *AgentLoop) runSessionMemoryExtraction() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[session-memory] extraction panic: %v\n", r)
			// Clear in-progress flag even on panic
			if a.extractionState != nil {
				a.extractionState.MarkExtracted(int64(a.context.EstimatedTokens()))
			}
		}
	}()

	// Mark extraction as in-progress so SM-compact can wait for it.
	// This matches upstream's waitForSessionMemoryExtraction pattern.
	if a.extractionState != nil {
		a.extractionState.MarkExtractionInProgress()
	}

	sm := a.config.SessionMemory
	if sm == nil {
		return
	}

	// Capture cache-safe params from the current state
	messages := a.context.BuildMessages()
	messages = NormalizeAPIMessages(messages)
	cacheParams := CaptureCacheSafeParams(
		a.context.SystemPrompt(),
		a.config.Model,
		a.registry,
		messages,
	)

	// Read current session memory content
	memoryPath := filepath.Join(a.config.ProjectDir, ".claude", "session_memory.md")
	currentContent, _ := os.ReadFile(memoryPath)
	if len(currentContent) == 0 {
		currentContent = []byte(defaultSessionMemoryTemplate)
	}

	// Build the extraction prompt
	prompt := sessionMemoryUpdatePrompt(memoryPath, string(currentContent))
	forkMessages := []anthropic.MessageParam{
		anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{{OfText: &anthropic.TextBlockParam{Text: prompt}}},
		},
	}

	// Run forked agent
	cfg := ForkedAgentConfig{
		CacheSafeParams:    cacheParams,
		ForkMessages:       forkMessages,
		CanUseTool:         createMemoryFileCanUseTool(memoryPath),
		MaxTokens:          8192,
		QuerySource:        "session_memory",
		MaxTurns:           5,
		Registry:           a.registry,
		ProjectDir:         a.config.ProjectDir,
		SkipParentMessages: true,
		Client:             a.client,
	}

	_, err := RunForkedAgent(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[session-memory] extraction error: %v\n", err)
		// Clear in-progress flag on error so SM-compact does not block forever.
		if a.extractionState != nil {
			a.extractionState.MarkExtracted(int64(a.context.EstimatedTokens()))
		}
		return
	}

	// Mark extraction complete and record token count
	if a.extractionState != nil {
		a.extractionState.MarkExtracted(int64(a.context.EstimatedTokens()))
	}
}

// Attribution tracks which model contributed to each file edit.
// This is useful for organizations that need to track AI contributions.
type Attribution struct {
	Model string
}

// NewAttribution creates an attribution tracker for the given model.
func NewAttribution(model string) *Attribution {
	return &Attribution{Model: sanitizeModelName(model)}
}

// sanitizeModelName cleans a model name for use in git notes.
// Removes version hashes and normalizes format.
func sanitizeModelName(model string) string {
	// Remove version hash suffix (e.g., claude-sonnet-4-20250514 -> claude-sonnet-4)
	parts := strings.Split(model, "-")
	// Check if last part looks like a date (8+ digits)
	if len(parts) > 0 && len(parts[len(parts)-1]) >= 8 {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, "-")
}

// GetAttribution retrieves the attribution note for a commit.
func GetAttribution(commitRef string) string {
	cmd := exec.Command("git", "notes", "show", commitRef)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// FormatAttribution formats an attribution string for display.
func FormatAttribution(model string, files []string) string {
	if len(files) == 0 {
		return fmt.Sprintf("Model: %s", sanitizeModelName(model))
	}
	return fmt.Sprintf("Model: %s | Files: %s", sanitizeModelName(model), strings.Join(files, ", "))
}
