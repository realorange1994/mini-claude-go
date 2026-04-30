package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

// ---------------------------------------------------------------------------
// Chunk types
// ---------------------------------------------------------------------------

// ChunkType identifies the kind of streaming event.
type ChunkType string

const (
	ChunkTypeText         ChunkType = "text"         // Incremental text token
	ChunkTypeToolCall     ChunkType = "tool_call"    // Tool call started (id + name)
	ChunkTypeToolArgument ChunkType = "tool_argument" // Tool argument JSON delta
	ChunkTypeThinking     ChunkType = "thinking"     // Extended thinking block
	ChunkTypeUsage    ChunkType = "usage"        // Token usage info
	ChunkTypeError    ChunkType = "error"        // Error occurred
	ChunkTypeDone     ChunkType = "done"         // Stream complete
	ChunkTypeBlockStop ChunkType = "block_stop"  // Content block finished
)

// StreamChunk is a single event emitted during a streaming response.
type StreamChunk struct {
	Type    ChunkType
	Content string // text token, argument delta, error message
	ID      string // tool call id
	Name    string // tool name
	Usage   *Usage
}

// Usage holds token counts.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// streamHandler is the callback signature for consuming chunks.
type streamHandler func(chunk StreamChunk) error

// ---------------------------------------------------------------------------
// CollectHandler -- assembles streamed tokens into a complete response
// ---------------------------------------------------------------------------

// CollectHandler implements streamHandler and collects all chunks so the
// final assembled response (text, tool calls, thinking) can be retrieved
// after the stream finishes.
type CollectHandler struct {
	mu            sync.Mutex
	Text          string
	ToolCalls     []ToolCallInfo
	Thinking      string
	Err           error
	Usage         *Usage
	ChunksCollect int
	toolUseAsText bool // detects model echoing tool syntax as text
	finishReason  string // captured from MessageDeltaEvent.stop_reason
}

// ToolCallInfo records a tool call assembled from streaming deltas.
type ToolCallInfo struct {
	ID        string
	Name      string
	Arguments string
}

// StreamResult is the result of a streaming API call, including partial
// delivery on failure. Matching Hermes-agent StreamResult.
type StreamResult struct {
	ToolCalls []ToolCallInfo
	Text      string
	Thinking  string
	Completed bool // true = stream ended normally, false = partial after failure
	// Why the stream ended. Matches Anthropic stop_reason values:
	// - "end_turn": normal completion
	// - "stop_sequence": stop sequence hit
	// - "max_tokens": output token limit reached
	// - "tool_use": model yielded to tool use
	// - "": stream ended abnormally (error, stall, interrupt)
	FinishReason string
}

// StreamResult returns the complete streaming result from a CollectHandler.
// When completed=false, partial results are returned after a failure.
func StreamResultFrom(h *CollectHandler, completed bool) StreamResult {
	h.mu.Lock()
	text := h.Text
	thinking := h.Thinking
	finishReason := h.finishReason
	toolCalls := make([]ToolCallInfo, len(h.ToolCalls))
	copy(toolCalls, h.ToolCalls)
	h.mu.Unlock()

	if text == "" {
		text = thinking
	}
	return StreamResult{
		ToolCalls:    toolCalls,
		Text:         text,
		Thinking:     h.Thinking,
		Completed:    completed,
		FinishReason: finishReason,
	}
}

// NewCollectHandler creates a ready-to-use handler.
func NewCollectHandler() *CollectHandler {
	return &CollectHandler{
		ToolCalls: make([]ToolCallInfo, 0),
	}
}

// Handle consumes one chunk. Safe for concurrent use.
func (h *CollectHandler) Handle(chunk StreamChunk) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ChunksCollect++

	switch chunk.Type {
	case ChunkTypeText:
		// Detect model echoing tool syntax as text (stuck pattern)
		// Only flag when multiple strong indicators appear together -- the model
		// is outputting raw tool_use JSON as text instead of actual tool calls.
		// Must have structural markers of a real tool call (type + id + name),
		// not just passing references to these keywords.
		lower := strings.ToLower(chunk.Content)
		hasType := strings.Contains(lower, `"type":"tool_use"`) || strings.Contains(lower, `"type": "tool_use"`)
		hasID := strings.Contains(lower, `"id":"`) || strings.Contains(lower, `"id": "`)
		hasName := strings.Contains(lower, `"name":"`) || strings.Contains(lower, `"name": "`)
		// Only trigger when at least 2 of 3 structural markers present
		if (hasType && hasID) || (hasType && hasName) || (hasID && hasName) {
			h.toolUseAsText = true
		} else {
			h.Text += chunk.Content
		}

	case ChunkTypeToolCall:
		h.ToolCalls = append(h.ToolCalls, ToolCallInfo{
			ID:   chunk.ID,
			Name: chunk.Name,
		})

	case ChunkTypeToolArgument:
		if n := len(h.ToolCalls); n > 0 {
			h.ToolCalls[n-1].Arguments += chunk.Content
		}

	case ChunkTypeThinking:
		h.Thinking += chunk.Content

	case ChunkTypeUsage:
		h.Usage = chunk.Usage

	case ChunkTypeError:
		h.Err = fmt.Errorf("stream error: %s", chunk.Content)

	case ChunkTypeDone:
		// stream finished

	case ChunkTypeBlockStop:
		// content block finished -- no-op for collector
	}

	return nil
}

// FullResponse returns the assembled text.
// If no text blocks were received but thinking was, returns thinking as fallback
// (some models return only thinking when no tools are needed).
func (h *CollectHandler) FullResponse() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Text != "" {
		return h.Text
	}
	return h.Thinking
}

// IsToolUseAsText reports whether the model echoed tool syntax as plain text.
// Thread-safe.
func (h *CollectHandler) IsToolUseAsText() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.toolUseAsText
}

// SetFinishReason records the stop_reason from the stream.
func (h *CollectHandler) SetFinishReason(reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finishReason = reason
}

// FinishReason returns why the stream ended (end_turn, max_tokens, tool_use, etc.)
func (h *CollectHandler) FinishReason() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.finishReason
}

// HasPartialToolCall checks if the last tool call has no arguments yet
// (stream cut off mid-tool-call).
func (h *CollectHandler) HasPartialToolCall() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := len(h.ToolCalls)
	if n == 0 {
		return false
	}
	return h.ToolCalls[n-1].Arguments == ""
}

// ClearPartialToolCall removes the last incomplete tool call before retry
// to avoid duplicating tool_call entries on reconnect.
func (h *CollectHandler) ClearPartialToolCall() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n := len(h.ToolCalls); n > 0 {
		h.ToolCalls = h.ToolCalls[:n-1]
	}
}

// ClearAll removes all accumulated state (text, tool calls, thinking).
// Used before stream retries where the API will send a completely
// new response -- old collected data would have mismatched IDs.
func (h *CollectHandler) ClearAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Text = ""
	h.ToolCalls = nil
	h.Thinking = ""
}

// ClearText removes all pending text that was already streamed to the user.
// Used when retry cannot recover text deltas (text-only case).
func (h *CollectHandler) ClearText() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Text = ""
}

// HasTruncatedToolArgs checks if any tool call has invalid JSON arguments,
// indicating the stream was cut off mid-tool-call.
func (h *CollectHandler) HasTruncatedToolArgs() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.ToolCalls {
		if h.ToolCalls[i].Arguments != "" {
			var js json.RawMessage
			if json.Unmarshal([]byte(h.ToolCalls[i].Arguments), &js) != nil {
				return true
			}
		}
	}
	return false
}

// AsParsedResponse returns tool calls and text parts directly, bypassing
// SDK ContentBlockUnion (which loses text on AsAny() cast with non-Claude models).
func (h *CollectHandler) AsParsedResponse() ([]map[string]any, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var textParts []string
	text := h.Text
	if text == "" {
		text = h.Thinking
	}
	if text != "" {
		textParts = append(textParts, text)
	}

	toolCalls := make([]map[string]any, 0, len(h.ToolCalls))
	for _, tc := range h.ToolCalls {
		input := make(map[string]any)
		if tc.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Arguments), &input)
		}
		toolCalls = append(toolCalls, map[string]any{
			"id":    tc.ID,
			"name":  tc.Name,
			"input": input,
		})
	}

	return toolCalls, textParts
}

// ThinkFilterState tracks the state machine for filtering <thinking>...</thinking>
// blocks from terminal display.
type ThinkFilterState int

const (
	ThinkNormal   ThinkFilterState = iota // normal text output
	ThinkInTag                             // detected <think
	ThinkInBlock                           // inside thinking block
	ThinkClosing                           // detected </think
)

// ---------------------------------------------------------------------------
// TerminalHandler -- prints [Tool: name] ... and [THINK] thinking on completion
// ---------------------------------------------------------------------------

// TerminalHandler shows a clean progress display during streaming.
// Shows thinking tokens in dim text, tool calls, and results.
type TerminalHandler struct {
	seenToolCall   bool
	thinkingBuf    strings.Builder
	curToolName    string
	curToolArgs    strings.Builder
	thinkState     ThinkFilterState
	thinkBuf       string
}

// filterThinking runs text through the think filter state machine.
// Text inside <thinking>...</thinking> or <think>...</think> blocks is
// wrapped with ANSI dim/gray escape codes; tag markers are stripped.
func (h *TerminalHandler) filterThinking(text string) string {
	var out strings.Builder
	i := 0
	for i < len(text) {
		switch h.thinkState {
		case ThinkNormal:
			// Check for opening tag <think or <thinking
			if text[i] == '<' {
				remaining := text[i:]
				if strings.HasPrefix(remaining, "<thinking") {
					h.thinkState = ThinkInTag
					h.thinkBuf = "<thinking"
					i += len("<thinking")
					continue
				}
				if strings.HasPrefix(remaining, "<think") {
					h.thinkState = ThinkInTag
					h.thinkBuf = "<think"
					i += len("<think")
					continue
				}
			}
			out.WriteByte(text[i])
			i++

		case ThinkInTag:
			h.thinkBuf += string(text[i])
			if text[i] == '>' {
				h.thinkState = ThinkInBlock
				h.thinkBuf = ""
				// Start dim output
				out.WriteString("\033[2m")
			}
			i++

		case ThinkInBlock:
			// Check for closing tag </think or </thinking
			if text[i] == '<' {
				remaining := text[i:]
				if strings.HasPrefix(remaining, "</thinking") {
					h.thinkState = ThinkClosing
					h.thinkBuf = "</thinking"
					i += len("</thinking")
					continue
				}
				if strings.HasPrefix(remaining, "</think") {
					h.thinkState = ThinkClosing
					h.thinkBuf = "</think"
					i += len("</think")
					continue
				}
			}
			out.WriteByte(text[i])
			i++

		case ThinkClosing:
			h.thinkBuf += string(text[i])
			if text[i] == '>' {
				h.thinkState = ThinkNormal
				h.thinkBuf = ""
				// End dim output
				out.WriteString("\033[0m")
			}
			i++
		}
	}
	return out.String()
}

func (h *TerminalHandler) Handle(chunk StreamChunk) error {
	switch chunk.Type {
	case ChunkTypeThinking:
		// Buffer thinking, show after tool call starts or text
		h.thinkingBuf.WriteString(chunk.Content)
	case ChunkTypeToolCall:
		h.seenToolCall = true
		// Show buffered thinking before tool call
		if th := h.thinkingBuf.String(); th != "" {
			lines := strings.Split(th, "\n")
			preview := lines[0]
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			fmt.Fprintf(os.Stderr, "\n[THINK] %s\n", preview)
		}
		h.thinkingBuf.Reset()
		h.curToolName = chunk.Name
		h.curToolArgs.Reset()
		fmt.Fprintf(os.Stderr, "[Tool: %s] ", chunk.Name)
	case ChunkTypeToolArgument:
		h.curToolArgs.WriteString(chunk.Content)
	case ChunkTypeBlockStop:
		// Content block finished -- flush pending tool call with parsed args
		if h.curToolName != "" {
			h.flushToolCall()
		}
	case ChunkTypeDone:
		// Flush any pending tool call with its parsed args
		if h.curToolName != "" {
			h.flushToolCall()
		}
		// Flush buffered thinking if no tool call was seen
		if !h.seenToolCall && h.thinkingBuf.Len() > 0 {
			th := h.thinkingBuf.String()
			lines := strings.Split(th, "\n")
			preview := lines[0]
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			fmt.Fprintf(os.Stderr, "\n[THINK] %s\n", preview)
			h.thinkingBuf.Reset()
		}
	case ChunkTypeText:
		// Flush any pending tool call before printing text
		if h.curToolName != "" {
			h.flushToolCall()
		}
		// Run through think filter state machine before output
		filtered := h.filterThinking(chunk.Content)
		if filtered != "" {
			fmt.Fprint(os.Stderr, filtered)
		}
	}
	return nil
}

// flushToolCall parses the accumulated tool arguments and prints a compact summary.
func (h *TerminalHandler) flushToolCall() {
	if h.curToolName == "" {
		return
	}
	args := h.curToolArgs.String()
	summary := toolArgSummary(h.curToolName, args)
	fmt.Fprintf(os.Stderr, "%s\n", summary)
	h.curToolName = ""
	h.curToolArgs.Reset()
}

// toolArgSummary extracts the most relevant field from tool JSON arguments.
func toolArgSummary(toolName, argsJSON string) string {
	var input map[string]any
	if argsJSON != "" {
		_ = json.Unmarshal([]byte(argsJSON), &input)
	}
	if input == nil {
		input = make(map[string]any)
	}

	switch toolName {
	case "read_file", "write_file", "edit_file", "multi_edit", "fileops":
		if path, ok := input["path"].(string); ok {
			return path
		}
	case "list_dir":
		if path, ok := input["path"].(string); ok {
			return path
		}
		return "." // Default to current directory
	case "exec", "terminal":
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
	case "system":
		if op, ok := input["operation"].(string); ok {
			return op
		}
	case "git":
		if args, ok := input["args"].(string); ok {
			return fmt.Sprintf("git %s", args)
		}
	case "web_search", "exa_search":
		if query, ok := input["query"].(string); ok {
			return query
		}
	case "web_fetch":
		if url, ok := input["url"].(string); ok {
			return url
		}
	case "process":
		if name, ok := input["process_name"].(string); ok {
			return name
		}
		if pid, ok := input["pid"].(float64); ok {
			return fmt.Sprintf("PID %.0f", pid)
		}
	case "runtime_info":
		if show, ok := input["show"].(string); ok {
			return show
		}
	}

	// Fallback: show all args compactly
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

// ---------------------------------------------------------------------------
// StreamBus -- pub/sub for StreamChunk events
// ---------------------------------------------------------------------------

// StreamBus publishes streaming chunks to named subscribers.
type StreamBus struct {
	mu          sync.RWMutex
	subscribers map[string]chan StreamChunk
}

// NewStreamBus creates an empty bus.
func NewStreamBus() *StreamBus {
	return &StreamBus{
		subscribers: make(map[string]chan StreamChunk),
	}
}

// Subscribe registers a subscriber and returns a receive-only channel.
func (b *StreamBus) Subscribe(id string) <-chan StreamChunk {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan StreamChunk, 100) // buffered so publisher never blocks
	b.subscribers[id] = ch
	return ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *StreamBus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.subscribers[id]; ok {
		close(ch)
		delete(b.subscribers, id)
	}
}

// Publish delivers a chunk to every subscriber (non-blocking, drops on full).
func (b *StreamBus) Publish(chunk StreamChunk) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for id, ch := range b.subscribers {
		select {
		case ch <- chunk:
		default:
			fmt.Fprintf(os.Stderr, "[WARN] StreamBus: subscriber %q channel full, dropping chunk\n", id)
		}
	}
}

// Close shuts down every subscriber channel.
func (b *StreamBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subscribers {
		close(ch)
	}
	b.subscribers = make(map[string]chan StreamChunk)
}

// ---------------------------------------------------------------------------
// StreamProgress -- tracks streaming metrics (TTFB, throughput)
// ---------------------------------------------------------------------------

// StreamProgress tracks timing and token metrics during a streaming response.
type StreamProgress struct {
	StartTime   time.Time
	FirstByteAt time.Time // TTFB
	TokensRecv  int
	mu          sync.Mutex
}

// RecordFirstByte sets FirstByteAt if this is the first content chunk.
func (p *StreamProgress) RecordFirstByte() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.FirstByteAt.IsZero() {
		p.FirstByteAt = time.Now()
	}
}

// RecordTokens increments the received token count.
func (p *StreamProgress) RecordTokens(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.TokensRecv += n
}

// TTFB returns the time to first byte duration. Returns 0 if first byte
// has not been recorded yet.
func (p *StreamProgress) TTFB() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.FirstByteAt.IsZero() {
		return 0
	}
	return p.FirstByteAt.Sub(p.StartTime)
}

// Throughput returns tokens per second. Returns 0 if not enough data.
func (p *StreamProgress) Throughput() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.TokensRecv == 0 || p.FirstByteAt.IsZero() {
		return 0
	}
	elapsed := time.Since(p.FirstByteAt).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(p.TokensRecv) / elapsed
}

// ---------------------------------------------------------------------------
// DeltasState -- tracks what content was already streamed (for retry safety)
// ---------------------------------------------------------------------------

// DeltasState tracks what content was already streamed to the user, used to
// decide whether a retry is safe or would cause text duplication.
//
// - None: no deltas sent yet -- clean retry is safe
// - TextOnly: text was already streamed -- retry would duplicate text
// - ToolInFlight: a tool call started with this ID but may be incomplete
type DeltasState string

const (
	DeltasStateNone        DeltasState = "none"
	DeltasStateTextOnly    DeltasState = "text_only"
	DeltasStateToolInFlight DeltasState = "tool_in_flight"
)

// ---------------------------------------------------------------------------
// StreamAdapter -- bridges anthropic SDK streaming events → StreamChunk
// ---------------------------------------------------------------------------

// StreamAdapter wraps the anthropic streaming response iterator and feeds
// chunks into a handler (and optionally a bus).
type StreamAdapter struct {
	handler        streamHandler
	bus            *StreamBus
	stallTimeoutMs int       // stall timeout in ms (0 = defaults)
	startupMs      int       // startup timeout in ms (0 = defaults)
	finishReason   string    // captured from MessageDeltaEvent.stop_reason
	deltasState    DeltasState // tracks what was already streamed
	progress       StreamProgress
}

// NewStreamAdapter creates an adapter that dispatches every chunk to the
// given handler and publishes on the bus (nil bus = no publishing).
func NewStreamAdapter(handler streamHandler, bus *StreamBus) *StreamAdapter {
	return &StreamAdapter{
		handler:      handler,
		bus:          bus,
		deltasState:  DeltasStateNone,
		progress:     StreamProgress{StartTime: time.Now()},
	}
}

// WithStallTimeout sets dynamic stall timeouts.
// isLocal=true → reduced timeouts to prevent long hangs.
// estTokens estimates context size for scaling.
func (sa *StreamAdapter) WithStallTimeout(isLocal bool, estTokens int) *StreamAdapter {
	if isLocal {
		sa.stallTimeoutMs = 60_000
		sa.startupMs = 120_000
	} else if estTokens > 100_000 {
		sa.stallTimeoutMs = 300_000
		sa.startupMs = 360_000
	} else if estTokens > 50_000 {
		sa.stallTimeoutMs = 240_000
		sa.startupMs = 300_000
	}
	// else: keep defaults in Process()
	return sa
}

// Process consumes the full streaming response, feeding each event through
// the handler. It returns any stream-level error.
// The cancel function is used by the safety timer to forcefully close the connection.
func (sa *StreamAdapter) Process(stream *ssestream.Stream[anthropic.MessageStreamEventUnion], cancel context.CancelFunc) error {
	var streamErr error // set when handler returns error

	// Wrap handler: on error, close stream and store error for post-loop return
	origHandler := sa.handler
	wrapped := func(chunk StreamChunk) error {
		if streamErr != nil {
			return streamErr // already aborted, skip remaining events
		}
		err := origHandler(chunk)
		if err != nil {
			streamErr = err
			stream.Close() // close immediately to unblock Next()
			return err
		}
		return nil
	}

	// Stall detection: reset timer on each successfully processed event.
	// Dynamic timeouts (matching hermes-agent patterns):
	//   - Local providers: 300s stall / 600s startup (cold start can be slow)
	//   - >100K tokens: 300s stall / 360s startup
	//   - >50K tokens: 240s stall / 300s startup
	//   - Default: 90s stall / 120s startup
	stallTO := sa.stallTimeoutMs
	startupTO := sa.startupMs
	if stallTO == 0 {
		stallTO = 90_000
	}
	if startupTO == 0 {
		startupTO = 120_000
	}
	stallReset := make(chan struct{}, 16)
	done := make(chan struct{})
	defer close(done)
	go func() {
		timer := time.NewTimer(time.Duration(startupTO) * time.Millisecond)
		defer timer.Stop()
		hasFirstEvent := false
		stallCount := 0
		for {
			select {
			case <-stallReset:
				stallCount = 0 // reset stall count on each successful event
				if !hasFirstEvent {
					hasFirstEvent = true
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(time.Duration(stallTO) * time.Millisecond)
			case <-timer.C:
				stallCount++
				timeoutVal := stallTO
				if !hasFirstEvent {
					timeoutVal = startupTO
				}
				fmt.Fprintf(os.Stderr, "\n[WARN] Stream stalled (no data for %dms), forcing close...\n", timeoutVal)
				stream.Close()
				if cancel != nil {
					cancel()
				}
				return
			case <-done:
				return
			}
		}
	}()

		sa.deltasState = DeltasStateNone // tracks what was already streamed

	for stream.Next() {
		// Signal stall detector that we received an event
		select {
		case stallReset <- struct{}{}:
		default:
		}

		event := stream.Current()

		switch e := event.AsAny().(type) {
		case anthropic.MessageStartEvent:
			// Message started -- carry on
		case anthropic.ContentBlockStartEvent:
			// A new content block started
			block := e.ContentBlock
			switch block.Type {
			case "tool_use":
				chunk := StreamChunk{
					Type: ChunkTypeToolCall,
					ID:   block.ID,
					Name: block.Name,
				}
				sa.trackDeltaState(ChunkTypeToolCall, chunk.ID)
				if err := wrapped(chunk); err != nil {
					return err
				}
			}

		case anthropic.ContentBlockDeltaEvent:
			// Incremental content -- dispatch based on delta type
			if chunk := sa.handleDeltaRaw(e.Delta); chunk.Type != "" {
				// Track stream progress metrics
				if chunk.Type == ChunkTypeText || chunk.Type == ChunkTypeThinking || chunk.Type == ChunkTypeToolArgument {
					sa.progress.RecordFirstByte()
					sa.progress.RecordTokens(1)
				}
				sa.trackDeltaState(chunk.Type, chunk.ID)
				if err := wrapped(chunk); err != nil {
					return err
				}
			}

		case anthropic.ContentBlockStopEvent:
			// Content block finished -- flush pending tool call display
			if err := wrapped(StreamChunk{Type: ChunkTypeBlockStop}); err != nil {
				return err
			}

		case anthropic.MessageDeltaEvent:
			// Carries stop_reason and cumulative usage info (matching Hermes finish_reason tracking)
			if e.Delta.StopReason != "" {
				sa.finishReason = string(e.Delta.StopReason)
			}
			usage := e.Usage
			if usage.InputTokens > 0 || usage.OutputTokens > 0 {
				chunk := StreamChunk{
					Type: ChunkTypeUsage,
					Usage: &Usage{
						InputTokens:  int(usage.InputTokens),
						OutputTokens: int(usage.OutputTokens),
					},
				}
				_ = wrapped(chunk)
			}

		case anthropic.MessageStopEvent:
			// Message completed normally
		}
	}

	// Emit done / error chunk
	if streamErr != nil {
		// Handler detected an issue (e.g. model confusion); return it
		return streamErr
	}
	if err := stream.Err(); err != nil {
		// Distinguish context cancellation (stall timer or caller) from real errors
		if contextErr(err) {
			return fmt.Errorf("stream stalled: %w", err)
		}
		sa.dispatch(StreamChunk{
			Type:    ChunkTypeError,
			Content: err.Error(),
		})
		// Still send Done so subscribers don't hang
		sa.dispatch(StreamChunk{Type: ChunkTypeDone})
		return err
	}
	sa.dispatch(StreamChunk{Type: ChunkTypeDone})
	return nil
}

// contextErr returns true if the error is caused by context cancellation or deadline.
func contextErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "deadline exceeded")
}

// handleDeltaRaw dispatches a RawContentBlockDeltaUnion and returns the chunk type.
func (sa *StreamAdapter) handleDeltaRaw(delta anthropic.RawContentBlockDeltaUnion) StreamChunk {
	switch d := delta.AsAny().(type) {
	case anthropic.TextDelta:
		if d.Text != "" {
			return StreamChunk{
				Type:    ChunkTypeText,
				Content: d.Text,
			}
		}
	case anthropic.InputJSONDelta:
		return StreamChunk{
			Type:    ChunkTypeToolArgument,
			Content: d.PartialJSON,
		}
	case anthropic.ThinkingDelta:
		if d.Thinking != "" {
			return StreamChunk{
				Type:    ChunkTypeThinking,
				Content: d.Thinking,
			}
		}
	}
	return StreamChunk{}
}

func (sa *StreamAdapter) dispatch(chunk StreamChunk) {
	if sa.handler != nil {
		_ = sa.handler(chunk) // handler errors are non-fatal for the stream
	}
	if sa.bus != nil {
		sa.bus.Publish(chunk)
	}
}

// FinishReason returns the captured stop_reason after Process completes.
func (sa *StreamAdapter) FinishReason() string {
	return sa.finishReason
}

// DeltasState returns what content was already streamed, used to decide
// whether a retry is safe or would cause text duplication.
func (sa *StreamAdapter) DeltasState() DeltasState {
	return sa.deltasState
}

// Progress returns a copy of the current stream progress metrics.
func (sa *StreamAdapter) Progress() StreamProgress {
	sa.progress.mu.Lock()
	defer sa.progress.mu.Unlock()
	// Return a copy
	return sa.progress
}

// trackDeltaState updates the deltas state based on each chunk type received.
// - Text (first delta, no prior tool call) → TextOnly
// - ToolCall → ToolInFlight (tool started but args may be incomplete)
// - ToolArgument → stays ToolInFlight
func (sa *StreamAdapter) trackDeltaState(chunkType ChunkType, id string) {
	switch sa.deltasState {
	case DeltasStateNone:
		switch chunkType {
		case ChunkTypeText:
			sa.deltasState = DeltasStateTextOnly
		case ChunkTypeToolCall:
			sa.deltasState = DeltasStateToolInFlight
		}
	case DeltasStateTextOnly:
		// text already streamed; tool call after text = still can't safely retry
	case DeltasStateToolInFlight:
		// tool in flight stays in flight until cleared
	}
}
