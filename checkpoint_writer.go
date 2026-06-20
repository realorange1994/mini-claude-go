package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Checkpoint Writer (MiMo-Code P8) ──────────────────────────────────────
//
// Spawns a background goroutine that writes structured checkpoint.md files.
// Uses token-budgeted boundary computation to select conversation tail.
//
// MiMo-Code source: session/checkpoint.ts (1478 lines, simplified to ~200)

const (
	// CheckpointWriterBudget tokens allocated for conversation tail
	CheckpointWriterBudget = 15000
	// CheckpointWriterMaxFailures max consecutive failures before giving up
	CheckpointWriterMaxFailures = 3
)

// CheckpointWriter manages background checkpoint writing.
type CheckpointWriter struct {
	mu              sync.Mutex
	sessionDir      string
	checkpointDir   string
	lastMessageID   string
	pending         *CheckpointRequest
	running         bool
	consecutiveFails int
	stopCh          chan struct{}
}

// CheckpointRequest represents a pending checkpoint write request.
type CheckpointRequest struct {
	SessionID   string
	Messages    []CheckpointMessage
	ProjectDir  string
	Timestamp   time.Time
}

// CheckpointMessage represents a message for checkpoint writing.
type CheckpointMessage struct {
	Role    string
	Content string
	Tokens  int
}

// CheckpointWriterResult holds the result of a checkpoint write.
type CheckpointWriterResult struct {
	CheckpointID string
	Tokens       int
	Written      bool
	Error        error
}

// NewCheckpointWriter creates a new checkpoint writer.
func NewCheckpointWriter(sessionDir string) *CheckpointWriter {
	return &CheckpointWriter{
		sessionDir:    sessionDir,
		checkpointDir: filepath.Join(sessionDir, "checkpoints"),
		stopCh:        make(chan struct{}),
	}
}

// Start starts the checkpoint writer background goroutine.
func (w *CheckpointWriter) Start() {
	go w.run()
}

// Stop stops the checkpoint writer.
func (w *CheckpointWriter) Stop() {
	close(w.stopCh)
}

// Submit submits a checkpoint write request.
// If a writer is already running, the request is queued (newest wins).
func (w *CheckpointWriter) Submit(req CheckpointRequest) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		// Queue request (newest wins — superset of older range)
		w.pending = &req
		return
	}

	// Start writer immediately
	w.running = true
	go w.writeCheckpoint(req)
}

// run is the background goroutine that processes pending requests.
func (w *CheckpointWriter) run() {
	for {
		select {
		case <-w.stopCh:
			return
		default:
			w.mu.Lock()
			if w.pending != nil {
				req := w.pending
				w.pending = nil
				w.mu.Unlock()
				w.writeCheckpoint(*req)
			} else {
				w.running = false
				w.mu.Unlock()
				time.Sleep(1 * time.Second)
			}
		}
	}
}

// writeCheckpoint writes a checkpoint file.
func (w *CheckpointWriter) writeCheckpoint(req CheckpointRequest) CheckpointWriterResult {
	w.mu.Lock()
	w.running = true
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
	}()

	// Ensure checkpoint directory exists
	if err := os.MkdirAll(w.checkpointDir, 0755); err != nil {
		w.consecutiveFails++
		return CheckpointWriterResult{Error: fmt.Errorf("create checkpoint dir: %w", err)}
	}

	// Compute token-budgeted boundary
	tail := computeBoundary(req.Messages, CheckpointWriterBudget)

	// Generate checkpoint ID
	checkpointID := fmt.Sprintf("cp-%s", time.Now().Format("20060102-150405"))

	// Build checkpoint content
	content := buildCheckpointContent(checkpointID, tail, req.ProjectDir)

	// Write checkpoint file
	checkpointPath := filepath.Join(w.checkpointDir, checkpointID+".md")
	if err := os.WriteFile(checkpointPath, []byte(content), 0644); err != nil {
		w.consecutiveFails++
		return CheckpointWriterResult{Error: fmt.Errorf("write checkpoint: %w", err)}
	}

	// Write metadata file
	metadata := map[string]any{
		"id":         checkpointID,
		"timestamp":  time.Now().Format(time.RFC3339),
		"messages":   len(req.Messages),
		"tailTokens": countTokens(tail),
	}
	metadataPath := filepath.Join(w.checkpointDir, checkpointID+".json")
	metadataBytes, _ := json.MarshalIndent(metadata, "", "  ")
	os.WriteFile(metadataPath, metadataBytes, 0644)

	// Update state
	w.mu.Lock()
	w.lastMessageID = checkpointID
	w.consecutiveFails = 0
	w.mu.Unlock()

	return CheckpointWriterResult{
		CheckpointID: checkpointID,
		Tokens:       countTokens(tail),
		Written:      true,
	}
}

// computeBoundary selects the conversation tail within the token budget.
func computeBoundary(messages []CheckpointMessage, budgetTokens int) []CheckpointMessage {
	totalTokens := 0
	startIdx := len(messages)

	// Walk backward from the end
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := messages[i].Tokens
		if msgTokens == 0 {
			msgTokens = estimateTokensCW(messages[i].Content)
		}
		if totalTokens+msgTokens > budgetTokens {
			break
		}
		totalTokens += msgTokens
		startIdx = i
	}

	return messages[startIdx:]
}

// buildCheckpointContent builds the checkpoint markdown content.
func buildCheckpointContent(checkpointID string, tail []CheckpointMessage, projectDir string) string {
	var sb string

	sb += "# Session Checkpoint\n\n"
	sb += fmt.Sprintf("_Generated at %s by checkpoint writer._\n\n", time.Now().Format(time.RFC3339))

	// Goal section
	sb += "## §1 Goal\n\n"
	sb += "Extract the current goal from conversation context.\n\n"

	// Instructions section
	sb += "## §2 Instructions\n\n"
	sb += "User instructions and constraints from the conversation.\n\n"

	// Discoveries section
	sb += "## §3 Discoveries\n\n"
	sb += "Key facts learned during this session.\n\n"

	// Accomplished section
	sb += "## §4 Accomplished\n\n"
	sb += "What has been completed so far.\n\n"

	// Relevant files section
	sb += "## §5 Relevant files\n\n"
	sb += "Files actively being read or modified.\n\n"

	// Conversation tail
	sb += "## §6 Conversation tail\n\n"
	for _, msg := range tail {
		sb += fmt.Sprintf("**%s**: %s\n\n", msg.Role, truncateStringCW(msg.Content, 200))
	}

	return sb
}

// estimateTokensCW estimates tokens from character count.
func estimateTokensCW(text string) int {
	return (len(text) + 3) / 4
}

// countTokens counts tokens in a message slice.
func countTokens(messages []CheckpointMessage) int {
	total := 0
	for _, msg := range messages {
		if msg.Tokens > 0 {
			total += msg.Tokens
		} else {
			total += estimateTokensCW(msg.Content)
		}
	}
	return total
}

// truncateStringCW truncates a string to maxLen.
func truncateStringCW(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// GetLastCheckpointID returns the last checkpoint ID.
func (w *CheckpointWriter) GetLastCheckpointID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastMessageID
}

// IsRunning returns whether a checkpoint write is in progress.
func (w *CheckpointWriter) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}
