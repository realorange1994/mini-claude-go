package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"miniclaudecode-go/tools"
	"miniclaudecode-go/transcript"
	"miniclaudecode-go/skills"
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

// registerAgentTool registers the AgentTool with this loop's SpawnFunc.
func (a *AgentLoop) registerAgentTool() {
	agentTool := &tools.AgentTool{
		SpawnFunc: a.SpawnSubAgent,
	}
	a.registry.Register(agentTool)
}

// registerSendMessageTool registers the SendMessage tool with this loop's callback.
func (a *AgentLoop) registerSendMessageTool() {
	sendMsgTool := &tools.SendMessageTool{
		SendMessageFunc: a.SendMessageToSubAgent,
		GetStatusFunc:   a.GetSubAgentStatus,
	}
	a.registry.Register(sendMsgTool)
}

// registerTaskOutputTool registers the TaskOutputTool with this loop's callback.
func (a *AgentLoop) registerTaskOutputTool() {
	taskOutputTool := &tools.TaskOutputTool{
		GetOutputFunc: a.GetSubAgentOutput,
	}
	a.registry.Register(taskOutputTool)
}

// registerTaskStopTool registers the TaskStopTool with this loop's callback.
func (a *AgentLoop) registerTaskStopTool() {
	taskStopTool := &tools.TaskStopTool{
		StopFunc: a.StopBackgroundTask,
	}
	a.registry.Register(taskStopTool)
}

// registerBashBgTool wires the ExecTool's BackgroundTaskCallback to this loop's
// spawnBackgroundBashCommand method, enabling run_in_background support.
func (a *AgentLoop) registerBashBgTool() {
	if tool := a.registry.Get("exec"); tool != nil {
		if execTool, ok := tool.(*tools.ExecTool); ok {
			execTool.BackgroundTaskCallback = a.spawnBackgroundBashCommand
		}
	}
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
func (a *AgentLoop) EnqueueAgentNotification(taskID, status, result, transcriptPath string, toolsUsed int, durationMs int64) {
	notification := fmt.Sprintf(`<task-notification>
<agentId>%s</agentId>
<status>%s</status>
<result>%s</result>
<output_file></output_file>
<transcript_path>%s</transcript_path>
<usage><total_tokens>%d</total_tokens><tool_uses>%d</tool_uses><duration_ms>%d</duration_ms></usage>
</task-notification>`, taskID, status, result, transcriptPath, toolsUsed, toolsUsed, durationMs)

	select {
	case a.notificationChan <- notification:
	default:
		// Channel is full, drop the notification
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
	var sb strings.Builder
	sb.WriteString("[System: The following sub-agent tasks completed while you were waiting]\n\n")
	for _, n := range notifications {
		sb.WriteString(n)
		sb.WriteString("\n\n")
	}
	a.context.AddUserMessage(sb.String())
}

// AgentLoop drives the core agentic loop.
type AgentLoop struct {
	config       Config
	registry     *tools.Registry
	gate         *PermissionGate
	context      *ConversationContext
	client       anthropic.Client
	snapshots    *SnapshotHistory
	transcript   *transcript.Writer
	skillTracker *skills.SkillTracker
	compactor    *Compactor
	useStream    bool
	maxToolChars int           // max chars per tool result (default 8192)
	toolTimeout  time.Duration // per-tool execution timeout (default 30s)
	maxTurns     int           // hard cap on turns (default from config.MaxTurns)
	budget       *IterationBudget
	interrupted  atomic.Bool   // set by Ctrl+C handler to stop the loop
	interruptOnce sync.Once    // ensures single interrupt watcher goroutine
	lastDeltasState DeltasState // tracks what was streamed in last attempt
	rateLimitState  RateLimitState // rate limit headers from API responses
	prevTurnTokens  int            // tracks token count from previous turn for reactive compact
	activeSubAgents atomic.Int32   // count of currently running sub-agents
	taskStore       *TaskStore     // tracks all sub-agent tasks
	notificationChan chan string   // buffered channel for async task notifications
	evictionDone    chan struct{}  // signals the eviction ticker goroutine to stop
	agentNameRegistry map[string]string // maps short agent names to task IDs
	cancelCtx      context.Context   // cancellable context for async sub-agents
	cancelFunc     context.CancelFunc // cancel function for async sub-agents
	workTaskStore  *WorkTaskStore    // tracks LLM work items (TODO list)
}

// newHTTPClient creates an HTTP client with sensible timeouts to prevent
// the agent from hanging on slow or unresponsive providers.
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 300 * time.Second, // overall request timeout
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,  // connection timeout
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

	client := anthropic.NewClient(opts...)

	ctx := NewConversationContext(cfg)

	// Initialize transcript writer
	sessionID := time.Now().Format("20060102-150405")
	transcriptDir := filepath.Join(".claude", "transcripts")
	tw := transcript.NewWriter(sessionID, filepath.Join(transcriptDir, sessionID+".jsonl"))
	_ = tw.Write(transcript.Entry{Type: "system", Content: fmt.Sprintf("model=%s, mode=%s", cfg.Model, cfg.PermissionMode)})

	maxTurns := cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 20 // default from ggbot
	}

	agent := &AgentLoop{
		config:       cfg,
		registry:     registry,
		gate:         NewPermissionGate(&cfg), // points to agent.config after assignment
		context:      ctx,
		client:       client,
		snapshots:    cfg.FileHistory,
		transcript:   tw,
		skillTracker: cfg.SkillTracker,
		compactor:    NewCompactor(),
		useStream:    useStream,
		maxToolChars: 8192,
		toolTimeout:  600 * time.Second,
		maxTurns:     maxTurns,
		budget:       NewIterationBudget(maxTurns),
		taskStore:       NewTaskStore(),
		notificationChan: make(chan string, 10),
		evictionDone:    make(chan struct{}),
		agentNameRegistry: make(map[string]string),
		workTaskStore:     NewWorkTaskStore(),
	}
	// Fix gate to point to agent's config (not the local cfg copy)
	agent.gate = NewPermissionGate(&agent.config)

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

	// Register the sub-agent tool (wires AgentTool.SpawnFunc to this loop's SpawnSubAgent)
	agent.registerAgentTool()
	agent.registerSendMessageTool()
	agent.registerTaskOutputTool()
	agent.registerTaskStopTool()
	agent.registerWorkTaskTools()
	agent.registerBashBgTool()

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

	client := anthropic.NewClient(opts...)

	gate := NewPermissionGate(&cfg)

	// Read transcript and rebuild context
	tr := transcript.NewReader(transcriptPath)
	entries, err := tr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read transcript: %w", err)
	}

	convCtx := rebuildContextFromTranscript(entries, cfg)

	maxTurns := cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 20
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
		config:           cfg,
		registry:         registry,
		gate:             gate,
		context:          convCtx,
		client:           client,
		snapshots:        cfg.FileHistory,
		transcript:       tw,
		skillTracker:     cfg.SkillTracker,
		compactor:        NewCompactor(),
		useStream:        useStream,
		maxToolChars:     8192,
		toolTimeout:      600 * time.Second,
		maxTurns:         maxTurns,
		budget:           NewIterationBudget(maxTurns),
		taskStore:          NewTaskStore(),
		notificationChan:   make(chan string, 10),
		evictionDone:       make(chan struct{}),
		agentNameRegistry:  make(map[string]string),
		workTaskStore:      NewWorkTaskStore(),
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
	agent.registerTaskOutputTool()
	agent.registerTaskStopTool()
	agent.registerWorkTaskTools()
	agent.registerBashBgTool()

	return agent, nil
}

// rebuildContextFromTranscript rebuilds conversation context from transcript entries.
// It groups consecutive tool_use and tool_result entries correctly:
// - Multiple consecutive tool_use entries become one assistant message
// - Multiple consecutive tool_result entries become one user message
func rebuildContextFromTranscript(entries []transcript.Entry, cfg Config) *ConversationContext {
	ctx := NewConversationContext(cfg)

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

	return ctx
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

// TranscriptPath returns the path to the current transcript file.
func (a *AgentLoop) TranscriptPath() string {
	if a.transcript == nil {
		return ""
	}
	return a.transcript.FilePath()
}

// interruptCtx creates a context that is cancelled either by the timeout
// or when the interrupted flag is set (whichever comes first).
func (a *AgentLoop) interruptCtx(baseCtx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(baseCtx, timeout)

	// Watch for interrupt flag in background (only one goroutine per instance)
	a.interruptOnce.Do(func() {
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
	})

	return ctx, cancel
}

// Run processes a user message through the agent loop, returning the final text response.
func (a *AgentLoop) Run(userMessage string) string {
	// Clear any stale interrupted flag from previous run
	a.SetInterrupted(false)

	// Reset the turn budget so each new conversation starts fresh
	a.budget = NewIterationBudget(a.maxTurns)
	a.lastDeltasState = DeltasStateNone // reset streaming state

	// Expand @ context references (e.g., @file:main.go, @diff)
	cwd, _ := os.Getwd()
	contextWindow := modelContextWindow(a.config.Model)
	if contextWindow < 1 {
		contextWindow = 200_000
	}
	expanded := PreprocessContextReferences(userMessage, cwd, contextWindow)
		if expanded.Expanded && !expanded.Blocked {
		userMessage = expanded.Message
	} else if len(expanded.Warnings) > 0 {
		// Log warnings even if blocked
		for _, w := range expanded.Warnings {
			fmt.Fprintf(os.Stderr, "[WARN] %s\n", w)
		}
	}

	a.context.AddUserMessage(userMessage)
	if a.transcript != nil {
		_ = a.transcript.WriteUser(userMessage)
	}
	var finalText string

	// Recovery state (mirrors ggbot's State machine)
	contextErrors := 0
	const maxContextRecovery = 3 // Phase 1: truncate, Phase 2: aggressive truncate, Phase 3: give up

	// Empty response tracking -- prevents infinite loops on thinking-only responses
	consecutiveEmptyResponses := 0
	const maxEmptyResponses = 3

	// Transition tracking (like Claude Code's state.transition)
	// Records WHY we continued to the next iteration, for debugging.
	var lastTransition string
	_ = lastTransition // used for transcript/debugging

	// Preflight compression for resumed sessions
	const preflightThreshold = 100000 // ~100k tokens
	if a.context.EstimatedTokens() > preflightThreshold {
		for i := 0; i < 3; i++ {
			a.tryCompaction()
			if a.context.EstimatedTokens() <= preflightThreshold {
				break
			}
		}
	}

	for a.budget.Consume() {
		// Check for interrupt at the start of each turn
		if a.IsInterrupted() {
			fmt.Fprintf(os.Stderr, "\n[WARN] Interrupted by user.\n")
			a.SetInterrupted(false) // reset for next request
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
				fmt.Fprintf(os.Stderr, "\n[reactive-compact] Token spike detected: %d -> %d (delta=%d, threshold=%d)\n",
					a.prevTurnTokens, currentTokens, result.TokenDelta, threshold)
				a.tryCompaction()
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

		var toolCalls []map[string]any
		var textParts []string
		var err error

		// Streaming vs non-streaming decision
		if a.useStream {
			toolCalls, textParts, err = a.callWithRetryAndFallback()
		} else {
			toolCalls, textParts, err = a.callWithNonStreamingOnly()
		}
		if err != nil {
			errMsg := err.Error()
			// User interrupt -- return immediately
			if strings.Contains(errMsg, "interrupted by user") {
				fmt.Fprintf(os.Stderr, "\n[WARN] Interrupted.\n")
				return finalText
			}
			// Model confusion -- echoed tool syntax as text; recover by retrying
			if strings.Contains(errMsg, "model confused") {
				fmt.Fprintf(os.Stderr, "\n[WARN] Model confused, retrying...\n")
				// Add a hint so the model doesn't repeat the same mistake
				a.context.AddUserMessage("ERROR: Your previous response was malformed. Do NOT output tool syntax as text. Use proper tool calls only.")
				lastTransition = "model_confusion_retry"
				continue
			}
			// 2013 error: tool_result doesn't follow tool_call -- repair pairing before retry
			if strings.Contains(errMsg, "2013") || strings.Contains(errMsg, "tool call result does not follow tool call") {
				fmt.Fprintf(os.Stderr, "\n[WARN] Tool pairing error (2013), repairing context...\n")
				a.context.ValidateToolPairing()
				a.context.FixRoleAlternation()
				lastTransition = "tool_pairing_repair"
				continue
			}
			// Truncated tool arguments -- model cut off mid-tool-call
			if strings.Contains(errMsg, "truncated") || strings.Contains(errMsg, "incomplete JSON") {
				fmt.Fprintf(os.Stderr, "\n[WARN] Tool arguments truncated, injecting corrective hint...\n")
				a.context.AddUserMessage("ERROR: Your tool call arguments was cut off due to length limits. Do NOT repeat the truncated tool call. If you need to make multiple tool calls, make them one at a time with shorter arguments.")
				lastTransition = "tool_args_truncated_retry"
				continue
			}
			// Stream stalled -- safety timeout fired; recover with truncation
			if strings.Contains(errMsg, "stream stalled") {
				contextErrors++
				if contextErrors > maxContextRecovery {
					fmt.Fprintf(os.Stderr, "\n[ERR] Stream stalled after %d recovery attempts, giving up.\n", maxContextRecovery)
					return finalText
				}
				if contextErrors <= 1 {
					fmt.Fprintf(os.Stderr, "\n[WARN] Stream stalled, truncating history (phase 1/3)...\n")
					a.context.TruncateHistory()
				} else if contextErrors <= 2 {
					fmt.Fprintf(os.Stderr, "\n[WARN] Stream still stalled, aggressive truncation (phase 2/3)...\n")
					a.context.AggressiveTruncateHistory()
				} else {
					fmt.Fprintf(os.Stderr, "\n[WARN] Stream still stalled, dropping to minimum (phase 3/3)...\n")
					a.context.MinimumHistory()
				}
				lastTransition = "stall_recovery"
				continue
			}
			if isContextLengthError(errMsg) {
				contextErrors++
				if contextErrors > maxContextRecovery {
					fmt.Fprintf(os.Stderr, "\n[ERR] Context length exceeded after %d recovery attempts, giving up.\n", maxContextRecovery)
					return finalText
				}

				if contextErrors <= 1 {
					fmt.Fprintf(os.Stderr, "\n[WARN] Context length exceeded, truncating history (phase 1/3)...\n")
					a.context.TruncateHistory()
				} else if contextErrors <= 2 {
					fmt.Fprintf(os.Stderr, "\n[WARN] Context still full, aggressive truncation (phase 2/3)...\n")
					a.context.AggressiveTruncateHistory()
				} else {
					fmt.Fprintf(os.Stderr, "\n[WARN] Context still full, dropping to minimum (phase 3/3)...\n")
					a.context.MinimumHistory()
				}
				lastTransition = "context_overflow_recovery"
				continue
			}
			return fmt.Sprintf("API error: %v", err)
		}

		// Reset context error counter on successful API call
		contextErrors = 0

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
					fmt.Fprintf(os.Stderr, "\n[ERR] No actionable response after %d attempts, giving up\n", maxEmptyResponses)
					return fmt.Sprintf("Model returned no actionable response %d times in a row", maxEmptyResponses)
				}
				fmt.Fprintf(os.Stderr, "\n[WARN] No text/tool_use in response (attempt %d/%d), continuing...\n",
					consecutiveEmptyResponses, maxEmptyResponses)
				// Inject hint to encourage actual output
				a.context.AddUserMessage("Please continue and provide your response in text or use a tool.")
				lastTransition = "empty_response_retry"
				continue
			}
			// Genuine final answer with text
			consecutiveEmptyResponses = 0
			// No tool calls -- model gave final answer.
			// Like Claude Code's stop hooks: the loop could continue here
			// with additional checks (token budget, quality check, etc.)
			// but for now we simply exit.
			a.context.AddAssistantText(finalText)
			if a.transcript != nil {
				_ = a.transcript.WriteAssistant(finalText, a.config.Model)
			}
			lastTransition = "completed"
			break
		}

		// Reset empty response counter on successful tool call
		consecutiveEmptyResponses = 0

		a.context.AddAssistantToolCalls(toolCalls)

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

		a.executeToolCallsConcurrent(toolCalls)

		// Check for interrupt after tool execution
		if a.IsInterrupted() {
			fmt.Fprintf(os.Stderr, "\n[WARN] Interrupted by user.\n")
			a.SetInterrupted(false)
			return finalText
		}

		lastTransition = "next_turn"
	}

	// If max turns reached without a final response, try one last non-streaming call
	// to get a conclusive answer (like Claude Code's max_turns handling).
	// Tools are removed in this call to force a text-only response.
	if finalText == "" && a.budget.GraceCall() {
		fmt.Fprintf(os.Stderr, "\n[WARN] Max turns (%d) reached, requesting final answer...\n", a.maxTurns)
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

	return finalText
}

// Close releases resources (transcript writer) and stops background goroutines.
func (a *AgentLoop) Close() {
	// Kill all running background tasks (sub-agents and bash tasks)
	if a.taskStore != nil {
		for _, task := range a.taskStore.AllTasks() {
			if !task.IsTerminal() {
				if task.Process != nil {
					_ = task.Process.Kill()
				}
				if task.CancelFunc != nil {
					task.CancelFunc()
				}
				task.Status = TaskStatusKilled
			}
		}
	}
	// Signal the eviction ticker to stop
	if a.evictionDone != nil {
		close(a.evictionDone)
		a.evictionDone = nil
	}
	if a.transcript != nil {
		_ = a.transcript.Close()
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

	// Try normal compaction first (may skip if not needed)
	if a.context.CompactContext() {
		if a.config.cachedPrompt != nil {
			a.config.cachedPrompt.MarkDirty()
		}
		return
	}

	// Normal compaction skipped (not enough tokens) -- force truncation
	before := len(entries)
	a.context.TruncateHistory()
	after := len(a.context.Entries())
	if after < before {
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

	result, err := a.context.PartialCompact(dir, pivotIndex, 3)
	if err != nil {
		fmt.Printf("[partial-compact] Error: %v\n", err)
		return
	}

	fmt.Printf("[partial-compact: %s] %d entries summarized, %d kept, ~%d tokens saved\n",
		dir, result.MessagesSummarized, result.MessagesKept, result.TokensSaved)

	if a.config.cachedPrompt != nil {
		a.config.cachedPrompt.MarkDirty()
	}
}

func (a *AgentLoop) callAPI() (*anthropic.Message, error) {
	toolParams := a.buildToolParams()

	// Try LLM compaction before sending to API
	a.tryCompaction()

	// Validate and fix internal entries BEFORE building API messages.
	// Previously this was done AFTER BuildMessages(), so the fixes
	// (orphan removal, role alternation) never reached the API params,
	// causing endless 2013 repair loops.
	a.context.ValidateToolPairing()
	a.context.FixRoleAlternation()

	messages := a.context.BuildMessages()
	messages = NormalizeAPIMessages(messages) // KV cache reuse

	params := anthropic.MessageNewParams{
		Model:     a.config.Model,
		MaxTokens: 16384,
		Messages:  messages,
		System: []anthropic.TextBlockParam{
			{Text: a.context.SystemPrompt()},
		},
	}
	if len(toolParams) > 0 {
		params.Tools = toolParams
	}

	const maxRetries = 9 // 1 attempt + 9 retries = 10 total
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * 2 * time.Second
			time.Sleep(delay)
		}

		ctx, cancel := a.interruptCtx(context.Background(), 90*time.Second)
		response, err := a.client.Messages.New(ctx, params)
		cancel()

		if err == nil {
			return response, nil
		}

		lastErr = err
		errMsg := err.Error()

		// Interrupt -- check the actual flag, not ctx.Err(), because
		// the interrupt watcher goroutine can race with the timeout.
		if a.IsInterrupted() {
			a.SetInterrupted(false)
			return nil, fmt.Errorf("interrupted by user")
		}

		// 2013 error: tool pairing broken -- repair and rebuild params before retry
		if strings.Contains(errMsg, "2013") || strings.Contains(errMsg, "tool call result does not follow tool call") {
			fmt.Fprintf(os.Stderr, "\n[WARN] Tool pairing error (2013), repairing context...\n")
			a.context.ValidateToolPairing()
			a.context.FixRoleAlternation()
			// Rebuild messages from repaired entries so the fix takes effect
			messages = a.context.BuildMessages()
			messages = NormalizeAPIMessages(messages)
			params.Messages = messages
			continue
		}

		// Special errors: pass through to Run loop
		if strings.Contains(errMsg, "model confused") ||
			strings.Contains(errMsg, "stream stalled") ||
			isContextLengthError(errMsg) {
			return nil, err
		}

		// Transient error: retry
		if isTransientError(errMsg) {
			continue
		}

		// Non-transient: give up
		return nil, err
	}

	return nil, fmt.Errorf("API error after %d retries: %w", maxRetries, lastErr)
}

// callWithRetryAndFallback calls the API with streaming, retries on transient
// errors, and falls back to non-streaming if stream persists failing.
// Uses a persistent CollectHandler across retries to track deltas state
// (matching Hermes-agent retry strategy).
func (a *AgentLoop) callWithRetryAndFallback() ([]map[string]any, []string, error) {
	const maxStreamRetries = 9 // 1 attempt + 9 retries = 10 total

	toolParams := a.buildToolParams()
	a.tryCompaction()
	// Validate and fix internal entries BEFORE building API messages.
	a.context.ValidateToolPairing()
	a.context.FixRoleAlternation()

	messages := a.context.BuildMessages()
	messages = NormalizeAPIMessages(messages) // KV cache reuse

	params := anthropic.MessageNewParams{
		Model:     a.config.Model,
		MaxTokens: 16384,
		Messages:  messages,
		System: []anthropic.TextBlockParam{
			{Text: a.context.SystemPrompt()},
		},
	}
	if len(toolParams) > 0 {
		params.Tools = toolParams
	}

	cacheMessageParams(&params) // Anthropic prompt caching (system_and_3)

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
			fmt.Fprintf(os.Stderr, "\n[WARN] Retrying stream (attempt %d/%d), waiting %v...\n",
				attempt+1, maxStreamRetries+1, delay)
			time.Sleep(delay)
		}

		toolCalls, textParts, err := a.tryStreamOnce(params, collect)
		if err == nil {
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

		// Transient error (network, timeout, 5xx): decide retry strategy
		if isTransientError(errMsg) {
			fmt.Fprintf(os.Stderr, "\n[WARN] Transient error during stream: %v\n", err)
			// Clear accumulated state before retry -- the API will send
			// a completely new response with new tool IDs on reconnect,
			// so old collected data would have mismatched IDs.
			collect.ClearAll()
			// Smart retry decision based on what was already delivered
			switch a.lastDeltasState {
			case DeltasStateNone:
				// Nothing sent yet -- clean retry
				continue
			case DeltasStateToolInFlight:
				// Tool call started but incomplete -- cleared above, retry
				fmt.Fprintf(os.Stderr, "  [!] Connection dropped mid-tool-call; reconnecting...\n")
				continue
			case DeltasStateTextOnly:
				// Text already streamed to user -- can't retry without duplication,
				// but we have what was collected so far. Fall back to non-streaming
				// for a complete fresh response (matching Hermes outer retry pattern).
				fmt.Fprintf(os.Stderr, "  [!] Stream interrupted after text output, falling back to non-streaming...\n")
				return a.callWithNonStreamingFallback(params)
			}
		}

		// Non-transient error during stream -> try non-streaming fallback
		fmt.Fprintf(os.Stderr, "\n[WARN] Stream failed (%v), falling back to non-streaming...\n", err)
		return a.callWithNonStreamingFallback(params)
	}

	// All stream retries exhausted -> try non-streaming fallback
	fmt.Fprintf(os.Stderr, "\n[WARN] Stream failed after %d attempts, falling back to non-streaming...\n", maxStreamRetries+1)
	return a.callWithNonStreamingFallback(params)
}

// tryStreamOnce makes a single streaming attempt and returns the result.
// `collect` is passed in (not created) so it persists across retries.
func (a *AgentLoop) tryStreamOnce(params anthropic.MessageNewParams, collect *CollectHandler) ([]map[string]any, []string, error) {
	ctx, cancel := a.interruptCtx(context.Background(), 300*time.Second)
	defer cancel()

	term := &TerminalHandler{}
	adapter := NewStreamAdapter(func(chunk StreamChunk) error {
		_ = collect.Handle(chunk)
		if err := term.Handle(chunk); err != nil {
			return err
		}
		if collect.IsToolUseAsText() {
			fmt.Fprint(os.Stderr, "\n[WARN] Model confused, aborting stream...\n")
			cancel()
			return fmt.Errorf("model confused: echoed tool syntax as text")
		}
		return nil
	}, nil)

	// Configure dynamic stall timeout (matching hermes-agent patterns)
	isLocal := isLocalEndpoint(a.config.BaseURL)
	estTokens := estimateMessageTokens(params.Messages)
	adapter.WithStallTimeout(isLocal, estTokens)

	stream := a.client.Messages.NewStreaming(ctx, params)
	if err := adapter.Process(stream, cancel); err != nil {
		a.lastDeltasState = adapter.DeltasState() // record what was streamed before error
		errMsg := err.Error()
		if strings.Contains(errMsg, "context canceled") ||
			strings.Contains(errMsg, "context deadline exceeded") ||
			strings.Contains(errMsg, "deadline exceeded") {
			return nil, nil, fmt.Errorf("stream stalled: %w", err)
		}
		return nil, nil, fmt.Errorf("stream error: %w", err)
	}

	// Record what was streamed (for retry safety)
	a.lastDeltasState = adapter.DeltasState()

	if collect.IsToolUseAsText() {
		fmt.Fprintf(os.Stderr, "\n[WARN] Model echoed tool syntax as text -- recovering\n")
		collect.Text = ""
	}

	// Check for truncated tool arguments (matching Hermes truncated arg detection)
	if collect.HasTruncatedToolArgs() {
		names := make([]string, 0, len(collect.ToolCalls))
		for _, tc := range collect.ToolCalls {
			names = append(names, tc.Name)
		}
		fmt.Fprintf(os.Stderr, "\n[WARN] Tool arguments truncated: %v\n", names)
		return nil, nil, fmt.Errorf("tool arguments were truncated (incomplete JSON)")
	}

	// Pass finish_reason to collect for downstream access
	if fr := adapter.FinishReason(); fr != "" {
		collect.SetFinishReason(fr)
	}

	toolCalls, textParts := collect.AsParsedResponse()
	return toolCalls, textParts, nil
}

// callWithNonStreamingOnly is the primary entry point when streaming is disabled.
// It's identical to callWithNonStreamingFallback but named for the non-streaming path.
func (a *AgentLoop) callWithNonStreamingOnly() ([]map[string]any, []string, error) {
	return a.callWithNonStreamingFallback(a.buildMessageParams())
}

// buildMessageParams constructs the API request params from current context.
func (a *AgentLoop) buildMessageParams() anthropic.MessageNewParams {
	toolParams := a.buildToolParams()
	messages := a.context.BuildMessages()
	messages = NormalizeAPIMessages(messages)   // KV cache reuse
	params := anthropic.MessageNewParams{
		Model:     a.config.Model,
		MaxTokens: 16384,
		Messages:  messages,
		System: []anthropic.TextBlockParam{
			{Text: a.context.SystemPrompt()},
		},
	}
	if len(toolParams) > 0 {
		params.Tools = toolParams
	}
	cacheMessageParams(&params) // Anthropic prompt caching (system_and_3)
	return params
}

// callWithNonStreamingNoTools makes a non-streaming API call WITHOUT tools.
// Used for the final grace call when max turns reached -- forces text-only response.
func (a *AgentLoop) callWithNonStreamingNoTools() ([]map[string]any, []string, error) {
	const maxRetries = 3 // shorter retry budget for grace call

	// Build messages WITHOUT tools
	messages := a.context.BuildMessages()
	messages = NormalizeAPIMessages(messages)
	params := anthropic.MessageNewParams{
		Model:     a.config.Model,
		MaxTokens: 16384,
		Messages:  messages,
		System: []anthropic.TextBlockParam{
			{Text: a.context.SystemPrompt()},
		},
	}
	// NOTE: No tools set -- model can only return text

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := jitteredBackoff(attempt)
			fmt.Fprintf(os.Stderr, "\n[WARN] Retrying final call (attempt %d/%d), waiting %v...\n",
				attempt+1, maxRetries+1, delay)
			time.Sleep(delay)
		}

		ctx, cancel := a.interruptCtx(context.Background(), 120*time.Second)
		response, err := a.client.Messages.New(ctx, params)
		cancel()

		if err == nil {
			toolCalls, textParts := a.parseResponse(response)
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
		if isTransientError(errMsg) {
			continue
		}
		return nil, nil, fmt.Errorf("final call error: %w", err)
	}

	return nil, nil, fmt.Errorf("final call failed after %d retries", maxRetries)
}

// callWithNonStreamingFallback tries non-streaming API call with retries.
// Mirrors Claude Code's non-streaming fallback + retry budget.
func (a *AgentLoop) callWithNonStreamingFallback(params anthropic.MessageNewParams) ([]map[string]any, []string, error) {
	const maxRetries = 9 // 1 attempt + 9 retries = 10 total

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := jitteredBackoff(attempt)
			if rlim := a.rateLimitState.RetryDelay(); rlim > 0 && rlim < delay*3 {
				delay = rlim
			}
			fmt.Fprintf(os.Stderr, "\n[WARN] Retrying non-streaming call (attempt %d/%d), waiting %v...\n",
				attempt+1, maxRetries+1, delay)
			time.Sleep(delay)
		}

		ctx, cancel := a.interruptCtx(context.Background(), 120*time.Second)
		response, err := a.client.Messages.New(ctx, params)
		cancel()

		if err == nil {
			toolCalls, textParts := a.parseResponse(response)
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
			fmt.Fprintf(os.Stderr, "\n[WARN] Tool pairing error (2013) in fallback, repairing context...\n")
			a.context.ValidateToolPairing()
			a.context.FixRoleAlternation()
			// Rebuild messages from repaired entries so the fix takes effect
			rebuilt := a.context.BuildMessages()
			rebuilt = NormalizeAPIMessages(rebuilt)
			params.Messages = rebuilt
			continue
		}

		// Special errors: pass through to Run loop for handling
		if strings.Contains(errMsg, "model confused") ||
			strings.Contains(errMsg, "stream stalled") ||
			isContextLengthError(errMsg) {
			return nil, nil, err
		}

		// Transient error: retry
		if isTransientError(errMsg) {
			fmt.Fprintf(os.Stderr, "\n[WARN] Transient error during non-streaming: %v\n", err)
			continue
		}

		// Non-transient error: give up
		return nil, nil, fmt.Errorf("stream fallback error: %w", err)
	}

	return nil, nil, fmt.Errorf("stream fallback error after %d retries", maxRetries)
}

func (a *AgentLoop) buildToolParams() []anthropic.ToolUnionParam {
	toolParams := make([]anthropic.ToolUnionParam, 0, len(a.registry.AllTools()))
	for _, t := range a.registry.AllTools() {
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
	return toolParams
}

func (a *AgentLoop) parseResponse(response *anthropic.Message) ([]map[string]any, []string) {
	var toolCalls []map[string]any
	var textParts []string
	var thinking string

	for _, block := range response.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			textParts = append(textParts, v.Text)
		case anthropic.ToolUseBlock:
			var input map[string]any
			if len(v.Input) > 0 {
				if err := json.Unmarshal(v.Input, &input); err != nil {
					fmt.Fprintf(os.Stderr, "[DEBUG] parseResponse: unmarshal input failed: %v\n", err)
				}
			}
			if input == nil {
				input = make(map[string]any)
			}

			call := map[string]any{
				"id":    v.ID,
				"name":  v.Name,
				"input": input,
			}
			toolCalls = append(toolCalls, call)
		case anthropic.ThinkingBlock:
			thinking = v.Thinking
		}
	}

	// Display thinking if present (matches Rust behavior)
	if thinking != "" {
		lines := strings.Split(thinking, "\n")
		preview := lines[0]
		if len(preview) > 120 {
			preview = preview[:120] + "..."
		}
		fmt.Fprintf(os.Stderr, "\n[THINK] %s\n", preview)
	}

	return toolCalls, textParts
}

func (a *AgentLoop) executeToolCallsConcurrent(toolCalls []map[string]any) {
	var toolResults []anthropic.ToolResultBlockParam

	// Print all tool calls upfront
	for _, call := range toolCalls {
		toolName, _ := call["name"].(string)
		input, _ := call["input"].(map[string]any)
		inputPreview := formatToolArgs(toolName, input)

		if toolName == "exec" {
			fmt.Fprintf(os.Stderr, "  [%s]: %s\n", toolName, inputPreview)
		} else {
			fmt.Fprintf(os.Stderr, "  [%s] %s\n", toolName, inputPreview)
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
		tool := a.registry.Get(toolName)
		if tool != nil {
			denial := a.gate.Check(tool, input)
			if denial != nil {
				entries[i].denied = true
				entries[i].errText = denial.Output
			}
		}
	}

	// Execute approved tool calls concurrently
	type jobResult struct {
		index int
		param anthropic.ToolResultBlockParam
		output string
	}
	ch := make(chan jobResult, len(entries))

	for _, e := range entries {
		go func(ent toolCallEntry) {
			// Check for interrupt before starting each tool
			if a.IsInterrupted() {
				ch <- jobResult{
					index: ent.index,
					param: anthropic.ToolResultBlockParam{
						ToolUseID: ent.call["id"].(string),
						Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: "Interrupted by user"}}},
						IsError:   param.NewOpt(true),
					},
					output: "Interrupted by user",
				}
				return
			}
			if ent.denied {
				ch <- jobResult{
					index: ent.index,
					param: anthropic.ToolResultBlockParam{
						ToolUseID: ent.call["id"].(string),
						Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: ent.errText}}},
						IsError:   param.NewOpt(true),
					},
					output: ent.errText,
				}
				return
			}
			// Skip already-validated gate check in executeSingleTool
			param, output := a.executeSingleToolApproved(ent.call)
			ch <- jobResult{index: ent.index, param: param, output: output}
		}(e)
	}

	// Collect results and sort by original index
	collected := make([]jobResult, 0, len(entries))
	for i := 0; i < len(entries); i++ {
		collected = append(collected, <-ch)
	}
	// Sort by index to preserve original order
	sort.Slice(collected, func(i, j int) bool {
		return collected[i].index < collected[j].index
	})

	// Append results in order
	for _, jr := range collected {
		toolResults = append(toolResults, jr.param)
	}

	a.context.AddToolResults(toolResults)
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
	// Keep first 80% and last 20%, with safe UTF-8 boundary truncation
	first := limit * 4 / 5
	last := limit - first
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

// executeSingleTool runs one tool call with timing, truncation, and timeout.
// Returns the ToolResultBlockParam and the output string.
func (a *AgentLoop) executeSingleTool(call map[string]any) (anthropic.ToolResultBlockParam, string) {
	return a.executeTool(call, true)
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
	input, _ := call["input"].(map[string]any)
	if input == nil {
		input = make(map[string]any)
	}

	// Coerce argument types to match schema
	if tool := a.registry.Get(toolName); tool != nil {
		tools.CoerceArguments(tool.InputSchema(), input)
	}

	// Record tool use to transcript
	if a.transcript != nil {
		_ = a.transcript.WriteToolUse(toolUseID, toolName, input)
	}

	// Agent-controlled timeout -- default 600s, clamped to [1, 600] seconds
	timeout := a.toolTimeout
	if t, ok := input["timeout"].(float64); ok && t > 0 {
		secs := int(t)
		if secs < 1 {
			secs = 1
		}
		if secs > 600 {
			secs = 600
		}
		timeout = time.Duration(secs) * time.Second
	}
	// Remove timeout from tool input -- it's a meta-parameter, not a tool param
	delete(input, "timeout")

	// Auto-snapshot before write/edit tools
	if toolName == "write_file" || toolName == "edit_file" || toolName == "multi_edit" {
		if path, ok := input["path"].(string); ok && path != "" {
			_ = a.snapshots.TakeSnapshotWithDesc(path, "before " + toolName)
		}
	}

	tool := a.registry.Get(toolName)
	if tool == nil {
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

	// Read-before-edit enforcement: file must be read and unmodified
	if (toolName == "write_file" || toolName == "edit_file" || toolName == "multi_edit") && !checkPermissions {
		if path, ok := input["path"].(string); ok && path != "" {
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

	// Execute with interrupt-aware context (agent-controlled timeout, default 30s)
	if timeout <= 0 {
		timeout = 600 * time.Second
	}
	ctx, cancel := a.interruptCtx(context.Background(), timeout)
	defer cancel()

	resultCh := make(chan tools.ToolResult, 1)
	start := time.Now()
	go func() {
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
				Output:  fmt.Sprintf("Error: %s timed out after %v", toolName, timeout),
				IsError: true,
			}
		}
	case <-time.After(timeout):
		cancelled = true
		result = tools.ToolResult{
			Output:  fmt.Sprintf("Error: %s timed out after %v", toolName, timeout),
			IsError: true,
		}
	}
	elapsed := time.Since(start)

	// Mark file as read after successful read_file
	if !result.IsError && toolName == "read_file" {
		if path, ok := input["path"].(string); ok && path != "" {
			a.registry.MarkFileRead(path)
		}
	}

	// Post-snapshot for write tools: capture the new state with a meaningful description
	if !result.IsError && (toolName == "write_file" || toolName == "edit_file" || toolName == "multi_edit") {
		if path, ok := input["path"].(string); ok && path != "" {
			desc := toolName
			if toolName == "edit_file" {
				if oldStr, ok2 := input["old_string"].(string); ok2 {
					if newStr, ok3 := input["new_string"].(string); ok3 {
						oldPreview := limitStr(oldStr, 50)
						newPreview := limitStr(newStr, 50)
						desc = fmt.Sprintf("edit: '%s' -> '%s'", oldPreview, newPreview)
					}
				}
			}
			_ = a.snapshots.TakeSnapshotWithDesc(path, desc)
		}
	}

	// rm/rmrf cleanup: clear snapshot history for deleted files
	if !result.IsError && toolName == "fileops" {
		if op, ok := input["operation"].(string); ok && (op == "rm" || op == "rmrf") {
			if path, ok2 := input["path"].(string); ok2 && path != "" {
				if op == "rm" {
					a.snapshots.ClearPath(path)
				} else {
					a.snapshots.ClearUnderDir(path)
				}
			}
		}
	}

	// Truncate long outputs
	output := a.truncateOutput(result.Output)

	// Display timing to stderr
	if cancelled {
		fmt.Fprintf(os.Stderr, "  [TIMEOUT] timed out after %v\n", timeout)
	} else if result.IsError {
		preview := limitStr(output, 150)
		fmt.Fprintf(os.Stderr, "  [ERR] %s (%v): %s\n", toolName, elapsed.Round(10*time.Millisecond), preview)
	} else {
		preview := toolResultPreview(toolName, output)
		if toolName == "exec" {
			// For exec, show result with tool name prefix
			fmt.Fprintf(os.Stderr, "  [+] %s: %s\n", toolName, preview)
		} else if preview == "" {
			fmt.Fprintf(os.Stderr, "  [+] %s\n", toolName)
		} else {
			fmt.Fprintf(os.Stderr, "  [+] %s: %s\n", toolName, preview)
		}
	}

	// Record result to transcript
	if a.transcript != nil {
		_ = a.transcript.WriteToolResult(toolUseID, toolName, output)
	}

	return anthropic.ToolResultBlockParam{
		ToolUseID: toolUseID,
		Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: output}}},
		IsError:   param.NewOpt(result.IsError),
	}, output
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

// formatToolArgs formats tool input as a compact string, showing file paths prominently.
func formatToolArgs(toolName string, input map[string]any) string {
	// Show the most relevant arg for each tool type
	switch toolName {
	case "read_file", "write_file", "edit_file", "list_dir":
		if path, ok := input["path"].(string); ok {
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
			if path, ok2 := input["path"].(string); ok2 {
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

// PostCompactRecovery re-injects critical context after compaction.
// This prevents the model from losing awareness of files it was working on
// and skills it was using, reducing wasted turns re-reading them.
// Returns the list of recovered file paths (for deduplication in AddHistorySnip).
func (a *AgentLoop) PostCompactRecovery() []string {
	if !a.config.PostCompactRecoverFiles {
		return nil
	}

	var recoveredPaths []string

	// --- File content recovery ---
	if a.registry != nil {
		maxFiles := a.config.PostCompactMaxFiles
		if maxFiles <= 0 {
			maxFiles = 5
		}
		maxFileChars := a.config.PostCompactMaxFileChars
		if maxFileChars <= 0 {
			maxFileChars = 50000
		}

		paths := a.registry.GetRecentlyReadFiles(maxFiles)
		totalChars := 0
		filesRecovered := 0

		for _, path := range paths {
			// Expand the normalized path back to a real path
			realPath := path
			if !filepath.IsAbs(realPath) {
				realPath = filepath.Join(a.config.ProjectDir, realPath)
			}

			data, err := os.ReadFile(realPath)
			if err != nil {
				continue // file may have been deleted
			}

			content := string(data)
			if totalChars+len(content) > maxFileChars {
				// Truncate to fit budget
				remaining := maxFileChars - totalChars
				if remaining < 200 {
					break
				}
				content = content[:remaining] + "\n... [truncated]"
			}

			attachment := fmt.Sprintf("[Post-compact file recovery: %s]\n```\n%s\n```", path, content)
			a.context.AddAttachment(attachment)
			totalChars += len(content)
			filesRecovered++
			recoveredPaths = append(recoveredPaths, path)

			// Re-mark file as read so edit checks still work
			a.registry.MarkFileRead(path)
		}

		if filesRecovered > 0 {
			fmt.Fprintf(os.Stderr, "[post-compact] Recovered %d files (%d chars)\n", filesRecovered, totalChars)
		}
	}

	// --- Skill content recovery ---
	if a.skillTracker != nil && a.config.SkillLoader != nil {
		maxSkillChars := a.config.PostCompactMaxSkillChars
		if maxSkillChars <= 0 {
			maxSkillChars = 5000
		}
		maxTotalSkillChars := a.config.PostCompactMaxTotalSkillChars
		if maxTotalSkillChars <= 0 {
			maxTotalSkillChars = 25000
		}

		readSkills := a.skillTracker.GetReadSkillNames()
		totalChars := 0
		skillsRecovered := 0

		for _, name := range readSkills {
			content := a.config.SkillLoader.LoadSkill(name)
			if content == "" {
				continue
			}

			if len(content) > maxSkillChars {
				content = content[:maxSkillChars] + "\n... [truncated]"
			}

			if totalChars+len(content) > maxTotalSkillChars {
				break
			}

			attachment := fmt.Sprintf("[Post-compact skill recovery: %s]\n%s", name, content)
			a.context.AddAttachment(attachment)
			totalChars += len(content)
			skillsRecovered++
		}

		if skillsRecovered > 0 {
			fmt.Fprintf(os.Stderr, "[post-compact] Recovered %d skills (%d chars)\n", skillsRecovered, totalChars)
		}
	}

	return recoveredPaths
}

// tryCompaction attempts LLM-driven compaction, falling back to truncation.
// When session memory exists and has content, uses SM-compact (免 API 压缩)
// to skip the LLM call and use session memory as the summary directly.
func (a *AgentLoop) tryCompaction() {
	// Phase 0: Micro-compact — clear old tool results every turn (cheap, no LLM call)
	if a.config.MicroCompactEnabled {
		keepRecent := a.config.MicroCompactKeepRecent
		if keepRecent <= 0 {
			keepRecent = 5
		}
		cleared := a.context.MicroCompactEntries(keepRecent, a.config.MicroCompactPlaceholder)
		if cleared > 0 {
			fmt.Fprintf(os.Stderr, "\n[micro-compact] Cleared %d old tool results\n", cleared)
		}
	}

	if a.compactor == nil {
		a.context.CompactContext()
		return
	}

	// Phase 1: SM-compact — use session memory as summary instead of calling LLM API.
	// This is the preferred path when memory is available: saves an LLM API call
	// and leverages incrementally collected session memory as the context summary.
	if a.config.SessionMemory != nil {
		if memContent := a.config.SessionMemory.FormatForPrompt(); memContent != "" {
			a.trySMCompact(memContent)
			// Mark system prompt dirty after compaction
			if a.config.cachedPrompt != nil {
				a.config.cachedPrompt.MarkDirty()
			}
			return
		}
	}

	// Phase 2: LLM-driven compaction (existing path)
	a.tryLLMCompaction()

	// Mark system prompt dirty after compaction
	if a.config.cachedPrompt != nil {
		a.config.cachedPrompt.MarkDirty()
	}
}

// trySMCompact performs compaction using session memory as the summary,
// skipping the LLM API call entirely. Inspired by the official Claude Code
// SM-compact mechanism (sessionMemoryCompact.ts).
func (a *AgentLoop) trySMCompact(sessionMemoryContent string) {
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

	// Format the session memory as a compact summary
	boundaryText := fmt.Sprintf("[SM-compact: %d tokens compressed, session memory used as summary]", preTokens)
	summaryContent := fmt.Sprintf("%s\n\n%s", boundaryText, sessionMemoryContent)

	fmt.Fprintf(os.Stderr, "\n[sm-compact] Using session memory as summary (%d tokens -> ~%d tokens)\n",
		preTokens, EstimateTokens(summaryContent)+6)

	// Inject boundary + summary into context
	a.context.AddCompactBoundary(CompactTriggerSMCompact, preTokens)
	a.context.AddSummary(summaryContent)

	// Update session memory with compaction state
	if a.config.SessionMemory != nil {
		a.config.SessionMemory.AddNote("state", fmt.Sprintf("Compaction (sm-compact): %d tokens compressed", preTokens), "auto")
	}

	// Phase 2: Post-compact recovery — re-inject critical context
	recoveredPaths := a.PostCompactRecovery()

	// Phase 3: History snip — preserve recent messages verbatim
	snipCount := a.config.PostCompactHistorySnipCount
	if snipCount <= 0 {
		snipCount = 3
	}
	a.context.AddHistorySnip(snipCount, recoveredPaths)

	// Calculate real post-compact token count for cooldown
	actualMessages := a.context.BuildMessages()
	actualPostTokens := estimateMessageParamsTokens(actualMessages)
	a.compactor.SetPostCompactTokens(actualPostTokens)
}

// tryLLMCompaction performs LLM-driven compaction (the existing path).
// Returns true if compaction was performed.
func (a *AgentLoop) tryLLMCompaction() {
	messages := a.context.BuildMessages()
	summary, performed := a.compactor.Compact(messages, a.config.Model, a.config.APIKey, a.config.BaseURL)
	if performed && summary != "" {
		// Inject boundary marker and summary into context
		preTokens := a.context.EstimatedTokens()
		a.context.AddCompactBoundary(CompactTriggerAuto, preTokens)
		a.context.AddSummary(summary)

		// Save compaction summary to session memory
		if a.config.SessionMemory != nil {
			a.config.SessionMemory.AddNote("state", fmt.Sprintf("Compaction: %s", summary), "auto")
		}

		// Phase 2: Post-compact recovery — re-inject critical context
		recoveredPaths := a.PostCompactRecovery()

		// Phase 3: History snip — preserve recent messages verbatim
		snipCount := a.config.PostCompactHistorySnipCount
		if snipCount <= 0 {
			snipCount = 3
		}
		a.context.AddHistorySnip(snipCount, recoveredPaths)

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
}

// isLocalEndpoint detects if the base URL points to a local provider.
func isLocalEndpoint(baseURL string) bool {
	lower := strings.ToLower(baseURL)
	return strings.Contains(lower, "localhost") ||
		strings.Contains(lower, "127.0.0.1") ||
		strings.Contains(lower, "0.0.0.0") ||
		strings.Contains(lower, "::1") ||
		strings.Contains(lower, "local")
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

// CollectHandler.AsMessageContent reconstructs ContentBlockUnion slices from collected data.
func (h *CollectHandler) AsMessageContent() []anthropic.ContentBlockUnion {
	h.mu.Lock()
	defer h.mu.Unlock()

	var content []anthropic.ContentBlockUnion

	// Use text if available, otherwise fall back to thinking (some models
	// return only thinking blocks when no tools are needed).
	textContent := h.Text
	if textContent == "" {
		textContent = h.Thinking
	}
	if textContent != "" {
		content = append(content, anthropic.ContentBlockUnion{
			Type: "text",
			Text: textContent,
		})
	}
	for _, tc := range h.ToolCalls {
		var input json.RawMessage
		if tc.Arguments != "" {
			input = json.RawMessage(tc.Arguments)
		} else {
			input = json.RawMessage("{}")
		}
		content = append(content, anthropic.ContentBlockUnion{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		})
	}
	return content
}

// StreamAdapter.Process takes a *ssestream.Stream directly
func (sa *StreamAdapter) ProcessStream(stream *ssestream.Stream[anthropic.MessageStreamEventUnion]) error {
	return sa.Process(stream, nil)
}
