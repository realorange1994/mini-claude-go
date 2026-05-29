package compaction

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode"
)

const (
	// Default model token limits
	DefaultMaxTokens     = 200000
	DefaultContextWindow = 1000000
)

// SummarizationPrompt is the structured prompt for initial compaction summarization.
// Aligned to TS SUMMARIZATION_PROMPT.
const SummarizationPrompt = `You are summarizing a conversation with a coding assistant. Produce a structured summary in the following format:

<summary>
Goal: [What the user is trying to achieve]
Constraints: [Any constraints, preferences, or rules mentioned]
Progress:
- Done: [Completed tasks and decisions]
- In Progress: [Ongoing work]
- Blocked: [Any blockers, if applicable]
Key Decisions: [Important architectural or implementation decisions made]
Next Steps: [What should happen next]
Critical Context: [File paths, code snippets, or other details essential for continuing]
</summary>

Here is the conversation to summarize:
<conversation>
{{conversation}}
</conversation>

Be concise but thorough. Focus on actionable information. If something is not applicable, omit that section.`

// UpdateSummarizationPrompt is the prompt for incremental updates to an existing summary.
// Aligned to TS UPDATE_SUMMARIZATION_PROMPT.
const UpdateSummarizationPrompt = `You are updating an existing conversation summary based on new messages. Merge the new information into the existing summary while preserving all important details.

Here is the current summary:
<previous-summary>
{{previous_summary}}
</previous-summary>

Here are the new messages to incorporate:
<conversation>
{{conversation}}
</conversation>

Produce an updated summary in the same format:

<summary>
Goal: [Updated goal]
Constraints: [Updated constraints]
Progress:
- Done: [Updated]
- In Progress: [Updated]
- Blocked: [Updated]
Key Decisions: [Updated]
Next Steps: [Updated]
Critical Context: [Updated]
</summary>`

// TokenCounter estimates token counts for text.
type TokenCounter struct {
	model         string
	maxTokens     int
	contextWindow int
}

// NewTokenCounter creates a token counter.
func NewTokenCounter(model string) *TokenCounter {
	return &TokenCounter{
		model:         model,
		maxTokens:     DefaultMaxTokens,
		contextWindow: DefaultContextWindow,
	}
}

// Count estimates the number of tokens in text.
// Uses character-level heuristics: ~4 chars per token for English,
// ~2 chars per token for CJK languages, and punctuation-aware counting.
func (tc *TokenCounter) Count(text string) int {
	if text == "" {
		return 0
	}

	// Count words and CJK characters separately
	wordTokens := 0
	cjkTokens := 0
	inWord := false

	for _, r := range text {
		if isCJK(r) {
			// Each CJK char is roughly 1-2 tokens depending on encoding
			cjkTokens += 2
		} else if unicode.IsSpace(r) || unicode.IsPunct(r) {
			// Punctuation and whitespace are part of token boundaries
			if inWord {
				wordTokens++
				inWord = false
			}
		} else {
			// Regular character - start or continue a word
			if !inWord {
				inWord = true
			}
		}
	}
	if inWord {
		wordTokens++
	}

	// English words: roughly 1 word = 1-1.5 tokens
	// CJK: ~1.5 chars = 1 token, so cjkTokens/2 is a reasonable estimate
	total := wordTokens + cjkTokens/2

	// Add overhead for tokenization artifacts
	// Special chars, numbers, etc. add ~5% overhead
	return total + int(math.Ceil(float64(total)*0.05))
}

// CountMessages estimates tokens for a list of messages.
func (tc *TokenCounter) CountMessages(messages []string) int {
	total := 0
	for _, msg := range messages {
		total += tc.Count(msg)
	}
	return total
}

// AvailableTokens returns remaining tokens before limit.
func (tc *TokenCounter) AvailableTokens(usedTokens int) int {
	return tc.maxTokens - usedTokens
}

// EstimateCompressionRatio estimates how much we need to compress.
func (tc *TokenCounter) EstimateCompressionRatio(currentTokens, targetTokens int) float64 {
	if currentTokens == 0 {
		return 1.0
	}
	ratio := float64(targetTokens) / float64(currentTokens)
	if ratio > 1.0 {
		return 1.0
	}
	return ratio
}

// CompactionStrategy defines how to compact context.
type CompactionStrategy struct {
	// Target token count after compaction
	TargetTokens int
	// Preserve recent messages count
	PreserveRecent int
	// Whether to include branch summaries
	IncludeBranchSummaries bool
}

// DefaultStrategy returns the default compaction strategy.
func DefaultStrategy() *CompactionStrategy {
	return &CompactionStrategy{
		TargetTokens:           80000,
		PreserveRecent:         10,
		IncludeBranchSummaries: true,
	}
}

// Compactor handles context compaction.
type Compactor struct {
	counter  *TokenCounter
	strategy *CompactionStrategy
}

// NewCompactor creates a new compactor.
func NewCompactor(model string, strategy *CompactionStrategy) *Compactor {
	if strategy == nil {
		strategy = DefaultStrategy()
	}
	return &Compactor{
		counter:  NewTokenCounter(model),
		strategy: strategy,
	}
}

// ShouldCompact returns true if compaction is needed.
func (c *Compactor) ShouldCompact(messageTokens int) bool {
	return messageTokens > c.strategy.TargetTokens
}

// CompactMessages reduces messages to fit within token limit.
// Preserves the most recent messages and tries to keep as many older ones as possible.
func (c *Compactor) CompactMessages(messages []string, preserveRecent int) ([]string, int, error) {
	if preserveRecent <= 0 {
		preserveRecent = c.strategy.PreserveRecent
	}

	if len(messages) <= preserveRecent {
		return messages, c.counter.CountMessages(messages), nil
	}

	// Keep recent messages
	recent := messages[len(messages)-preserveRecent:]
	older := messages[:len(messages)-preserveRecent]

	// Count tokens in recent messages
	recentTokens := c.counter.CountMessages(recent)
	available := c.strategy.TargetTokens - recentTokens

	if available <= 0 {
		// Recent messages already exceed the target, keep all of them
		return recent, recentTokens, nil
	}

	// Try to keep older messages until we hit the limit
	// Work backwards from the end of older messages
	var kept []string
	var keptTokens int

	for i := len(older) - 1; i >= 0; i-- {
		msgTokens := c.counter.Count(older[i])
		if keptTokens+msgTokens > available && len(kept) > 0 {
			break
		}
		kept = append([]string{older[i]}, kept...)
		keptTokens += msgTokens
	}

	// If we kept nothing from older messages, force keep at least one
	if len(kept) == 0 && len(older) > 0 {
		// Keep the first message (usually the system prompt or initial instruction)
		kept = append(kept, older[0])
		keptTokens = c.counter.Count(older[0])
	}

	result := append(kept, recent...)
	return result, keptTokens + recentTokens, nil
}

// SummarizeMessages creates a text summary of the messages.
// This is a non-LLM summarization method that extracts key information.
func (c *Compactor) SummarizeMessages(messages []string) string {
	var summary strings.Builder

	// Extract key actions, decisions, and file references
	actions := extractKeyActions(messages)
	files := extractFileReferences(messages)
	decisions := extractDecisions(messages)

	if len(actions) > 0 {
		summary.WriteString("Key Actions:\n")
		for _, a := range actions {
			summary.WriteString("- " + a + "\n")
		}
	}

	if len(files) > 0 {
		summary.WriteString("Files Modified:\n")
		for _, f := range files {
			summary.WriteString("- " + f + "\n")
		}
	}

	if len(decisions) > 0 {
		summary.WriteString("Key Decisions:\n")
		for _, d := range decisions {
			summary.WriteString("- " + d + "\n")
		}
	}

	tokenCount := c.counter.CountMessages(messages)
	summary.WriteString(fmt.Sprintf("Total: %d messages, ~%d tokens", len(messages), tokenCount))

	return summary.String()
}

// BranchSummarizer creates summaries of branches.
type BranchSummarizer struct {
	counter *TokenCounter
}

// NewBranchSummarizer creates a branch summarizer.
func NewBranchSummarizer(model string) *BranchSummarizer {
	return &BranchSummarizer{counter: NewTokenCounter(model)}
}

// Summarize creates a branch summary from messages.
// Uses heuristic extraction to identify key actions, files, and decisions.
func (bs *BranchSummarizer) Summarize(sessionId, branchName string, messages []string, model string) (string, error) {
	tokenCount := bs.counter.CountMessages(messages)
	summary := strings.Builder{}

	summary.WriteString(fmt.Sprintf("Branch: %s\n", branchName))
	summary.WriteString(fmt.Sprintf("Session: %s\n", sessionId))
	summary.WriteString(fmt.Sprintf("Messages: %d, Tokens: ~%d\n", len(messages), tokenCount))

	// Extract key information from messages
	actions := extractKeyActions(messages)
	files := extractFileReferences(messages)
	decisions := extractDecisions(messages)

	if len(actions) > 0 {
		summary.WriteString("\nKey Actions:\n")
		for _, a := range actions {
			summary.WriteString("- " + a + "\n")
		}
	}

	if len(files) > 0 {
		summary.WriteString("\nFiles Modified/Accessed:\n")
		for _, f := range files {
			summary.WriteString("- " + f + "\n")
		}
	}

	if len(decisions) > 0 {
		summary.WriteString("\nKey Decisions:\n")
		for _, d := range decisions {
			summary.WriteString("- " + d + "\n")
		}
	}

	return summary.String(), nil
}

// extractKeyActions extracts key actions from messages.
// Looks for tool invocations, bash commands, and file operations.
func extractKeyActions(messages []string) []string {
	var actions []string
	actionSet := make(map[string]bool)

	for _, msg := range messages {
		lower := strings.ToLower(msg)

		// Look for tool invocations
		if strings.Contains(lower, "tool:") || strings.Contains(lower, "tool_use") {
			// Extract first line as a summary
			lines := strings.Split(msg, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if len(line) > 10 && len(line) < 200 && !actionSet[line] {
					actions = append(actions, line)
					actionSet[line] = true
					break
				}
			}
		}

		// Look for bash commands
		if strings.Contains(lower, "bash") || strings.Contains(lower, "$ ") {
			lines := strings.Split(msg, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "$ ") || strings.HasPrefix(line, "bash") {
					if len(line) < 200 && !actionSet[line] {
						actions = append(actions, line)
						actionSet[line] = true
					}
					break
				}
			}
		}
	}

	return actions
}

// extractFileReferences finds file paths referenced in messages.
func extractFileReferences(messages []string) []string {
	var files []string
	fileSet := make(map[string]bool)

	for _, msg := range messages {
		// Look for file paths (simple pattern: words with slashes/dots)
		words := strings.Fields(msg)
		for _, word := range words {
			// Clean up the word
			word = strings.Trim(word, "(),;\"'")
			if len(word) > 5 && len(word) < 200 {
				hasExt := false
				for i, c := range word {
					if c == '.' && i > 0 {
						hasExt = true
					}
				}
				if hasExt && !fileSet[word] {
					files = append(files, word)
					fileSet[word] = true
				}
			}
		}
	}

	return files
}

// extractDecisions finds decision-related content in messages.
func extractDecisions(messages []string) []string {
	var decisions []string
	decisionSet := make(map[string]bool)

	for _, msg := range messages {
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "decided") || strings.Contains(lower, "i'll") ||
			strings.Contains(lower, "i will") || strings.Contains(lower, "let's") {
			// Extract relevant sentences
			sentences := strings.Split(msg, ".")
			for _, sentence := range sentences {
				sentence = strings.TrimSpace(sentence)
				sLower := strings.ToLower(sentence)
				if (strings.Contains(sLower, "decided") ||
					strings.Contains(sLower, "i'll") ||
					strings.Contains(sLower, "i will") ||
					strings.Contains(sLower, "let's")) &&
					len(sentence) > 10 && len(sentence) < 150 &&
					!decisionSet[sentence] {
					decisions = append(decisions, sentence)
					decisionSet[sentence] = true
					if len(decisions) >= 5 {
						break
					}
				}
			}
		}
	}

	return decisions
}

func countCJKChars(s string) int {
	count := 0
	for _, r := range s {
		if isCJK(r) {
			count++
		}
	}
	return count
}

func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3000 && r <= 0x303F) ||
		(r >= 0xFF00 && r <= 0xFFEF) ||
		(r >= 0x2E80 && r <= 0x2EFF) || // CJK radicals
		(r >= 0xAC00 && r <= 0xD7AF) || // Korean hangul
		(r >= 0x3040 && r <= 0x309F) || // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) // Katakana
}

// estimateTokens estimates tokens for a text message using chars/4 heuristic.
// Aligned to TS estimateTokens().
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(text) / 4
}

// estimateContextTokens estimates total tokens for a slice of messages.
// Uses the chars/4 heuristic for each message.
func estimateContextTokens(messages []string) int {
	total := 0
	for _, msg := range messages {
		total += estimateTokens(msg)
	}
	return total
}

// LLMClient is the interface for LLM-based compaction summary generation.
// This is a minimal interface to avoid importing agent package (circular dependency).
type LLMClient interface {
	Generate(ctx context.Context, model string, systemPrompt string, userPrompt string, maxTokens int) (string, error)
}

// LLMCompactor generates compaction summaries using an LLM.
// Aligned to TS compaction.generateSummary().
type LLMCompactor struct {
	llmClient LLMClient
	model     string
}

// NewLLMCompactor creates a new LLM-based compactor.
func NewLLMCompactor(model string, llmClient LLMClient) *LLMCompactor {
	return &LLMCompactor{
		llmClient: llmClient,
		model:     model,
	}
}

// GenerateSummary generates a structured summary of messages using the LLM.
// If previousSummary is non-empty, it generates an incremental update.
func (lc *LLMCompactor) GenerateSummary(ctx context.Context, messages []string, previousSummary string) (string, error) {
	if lc.llmClient == nil {
		return "", fmt.Errorf("LLM client not set")
	}

	conversation := strings.Join(messages, "\n---\n")

	prompt := SummarizationPrompt
	if previousSummary != "" {
		prompt = UpdateSummarizationPrompt
	}

	prompt = strings.Replace(prompt, "{{conversation}}", conversation, 1)
	if previousSummary != "" {
		prompt = strings.Replace(prompt, "{{previous_summary}}", previousSummary, 1)
	}

	// Generate with limited tokens to avoid consuming too much of the budget
	maxTokens := 4096
	result, err := lc.llmClient.Generate(ctx, lc.model, "You are a coding assistant summarizer.", prompt, maxTokens)
	if err != nil {
		return "", fmt.Errorf("LLM compaction summary failed: %w", err)
	}

	return result, nil
}

// ShouldUseLLMCompaction determines if LLM-based compaction should be used.
// Uses LLM compaction if the LLM client is available and messages are substantial.
func (lc *LLMCompactor) ShouldUseLLMCompaction(messages []string) bool {
	return lc.llmClient != nil && len(messages) > 5
}

// ---------------------------------------------------------------------------
// TS-aligned compaction: cut points, preparation, and compact()
// ---------------------------------------------------------------------------

const (
	// ESTIMATED_IMAGE_CHARS is the estimated token cost per image block.
	ESTIMATED_IMAGE_CHARS = 4800

	// TURN_PREFIX_SUMMARIZATION_PROMPT is used when splitting a mid-turn.
	TURN_PREFIX_SUMMARIZATION_PROMPT = `Summarize the early part of this conversation turn (before it was interrupted by context limits). Focus on:

- Original Request: What the user asked for
- Early Progress: What was accomplished before the split
- Context for Suffix: What the next part of the turn needs to know

Keep it concise. Preserve exact file paths and error messages.`
)

// CompactionSettings holds runtime configuration for compaction.
// Aligned to TS CompactionSettings.
type CompactionSettings struct {
	Enabled        bool
	ReserveTokens  int
	KeepRecentTokens int
}

// DefaultCompactionSettings is the default configuration.
var DefaultCompactionSettings = CompactionSettings{
	Enabled:        true,
	ReserveTokens:  16384,
	KeepRecentTokens: 20000,
}

// CompactionResult is returned by Compact().
type CompactionResult struct {
	Summary          string
	FirstKeptEntryID string
	TokensBefore     int
	ReadFiles        []string
	ModifiedFiles    []string
}

// CutPointResult is returned by FindCutPoint().
type CutPointResult struct {
	FirstKeptEntryIndex int
	TurnStartIndex      int
	IsSplitTurn         bool
}

// FileOperations tracks read/modified files across compaction.
type FileOperations struct {
	ReadFiles    []string
	ModifiedFiles []string
}

// CompactionPreparation holds prepared data for the main compaction.
type CompactionPreparation struct {
	FirstKeptEntryID  string
	MessagesToSummarize []string
	TurnPrefixMessages  []string
	IsSplitTurn         bool
	TokensBefore        int
	PreviousSummary     string
	FileOps             FileOperations
	Settings            CompactionSettings
}

// SessionEntryType represents the type of a session entry.
type SessionEntryType string

const (
	EntryTypeMessage       SessionEntryType = "message"
	EntryTypeCustomMessage SessionEntryType = "custom_message"
	EntryTypeBranchSummary SessionEntryType = "branch_summary"
	EntryTypeCompaction    SessionEntryType = "compaction"
	EntryTypeBashExecution SessionEntryType = "bash_execution"
	EntryTypeToolResult    SessionEntryType = "tool_result"
	EntryTypeSettings      SessionEntryType = "settings_change"
)

// SessionEntry represents a session entry for compaction purposes.
type SessionEntry struct {
	ID       string
	Type     SessionEntryType
	Content  string   // extracted text content
	Messages []string // for message entries, the raw messages
	Details  map[string]any
}

// extractFileOperations collects read/modified files from messages and previous compaction details.
func extractFileOps(entries []SessionEntry, prevCompactionIdx int) FileOperations {
	readSet := make(map[string]bool)
	modSet := make(map[string]bool)

	// Collect from previous compaction details
	for i := prevCompactionIdx; i < len(entries); i++ {
		if entries[i].Details != nil {
			if rf, ok := entries[i].Details["readFiles"].([]string); ok {
				for _, f := range rf {
					readSet[f] = true
				}
			}
			if mf, ok := entries[i].Details["modifiedFiles"].([]string); ok {
				for _, f := range mf {
					modSet[f] = true
				}
			}
		}
	}

	// Collect from tool calls in messages
	for _, e := range entries {
		for _, msg := range e.Messages {
			files := extractFileReferences([]string{msg})
			for _, f := range files {
				// Heuristic: files in tool results are "read", files in tool calls are "modified"
				if e.Type == EntryTypeToolResult {
					readSet[f] = true
				} else if e.Type == EntryTypeMessage {
					modSet[f] = true
				}
			}
		}
	}

	readFiles := make([]string, 0, len(readSet))
	for f := range readSet {
		readFiles = append(readFiles, f)
	}
	modFiles := make([]string, 0, len(modSet))
	for f := range modSet {
		modFiles = append(modFiles, f)
	}

	return FileOperations{ReadFiles: readFiles, ModifiedFiles: modFiles}
}

// findValidCutPoints returns indices of entries that are valid cut points.
// Never cuts at toolResult entries.
func findValidCutPoints(entries []SessionEntry, startIndex, endIndex int) []int {
	var cuts []int
	for i := startIndex; i < endIndex; i++ {
		switch entries[i].Type {
		case EntryTypeMessage, EntryTypeCustomMessage, EntryTypeBranchSummary,
			EntryTypeCompaction, EntryTypeBashExecution:
			cuts = append(cuts, i)
		// tool_result is NOT a valid cut point
		}
	}
	return cuts
}

// findTurnStartIndex walks backwards from entryIndex to find the user/bash/branch/custom
// message that started the current turn.
func findTurnStartIndex(entries []SessionEntry, entryIndex, startIndex int) int {
	for i := entryIndex; i >= startIndex; i-- {
		switch entries[i].Type {
		case EntryTypeMessage:
			return i
		case EntryTypeBashExecution, EntryTypeBranchSummary, EntryTypeCustomMessage:
			return i
		}
	}
	return startIndex
}

// FindCutPoint finds where to cut the conversation for compaction.
// Aligned to TS findCutPoint().
func FindCutPoint(entries []SessionEntry, startIndex, endIndex int, keepRecentTokens int) CutPointResult {
	validCuts := findValidCutPoints(entries, startIndex, endIndex)
	cutSet := make(map[int]bool)
	for _, c := range validCuts {
		cutSet[c] = true
	}

	// Walk backwards accumulating tokens
	accumulated := 0
	cutIdx := endIndex - 1

	for i := endIndex - 1; i >= startIndex; i-- {
		accumulated += estimateTokens(entries[i].Content)
		if accumulated > keepRecentTokens {
			// Find closest valid cut point at or after this position
			for j := i; j < endIndex; j++ {
				if cutSet[j] {
					cutIdx = j
					break
				}
			}
			break
		}
	}

	// Walk backwards from cutIdx to include non-message entries
	firstKept := cutIdx
	for i := cutIdx - 1; i >= startIndex; i-- {
		if entries[i].Type == EntryTypeCompaction {
			break
		}
		// Include settings, bash, etc. until we hit a message
		if entries[i].Type == EntryTypeMessage || entries[i].Type == EntryTypeBashExecution ||
			entries[i].Type == EntryTypeBranchSummary || entries[i].Type == EntryTypeCustomMessage {
			break
		}
		firstKept = i
	}

	// Determine if this is a split turn
	isSplitTurn := false
	turnStartIdx := firstKept

	if firstKept < endIndex {
		// If the cut entry is not a user message, we're splitting a turn
		if entries[firstKept].Type != EntryTypeMessage {
			isSplitTurn = true
			turnStartIdx = findTurnStartIndex(entries, firstKept, startIndex)
		}
	}

	return CutPointResult{
		FirstKeptEntryIndex: firstKept,
		TurnStartIndex:      turnStartIdx,
		IsSplitTurn:         isSplitTurn,
	}
}

// estimateContextTokensWithUsage finds the last assistant usage and estimates
// base tokens + trailing message tokens.
func estimateContextTokensWithUsage(entries []SessionEntry) (tokens, usageTokens, trailingTokens, lastUsageIdx int) {
	lastUsageIdx = -1
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Type == EntryTypeMessage && entries[i].Details != nil {
			if usage, ok := entries[i].Details["usage"].(map[string]any); ok {
				if totalTokens, ok := usage["totalTokens"].(float64); ok {
					tokens = int(totalTokens)
					lastUsageIdx = i
					break
				}
			}
		}
	}
	usageTokens = tokens

	// Estimate trailing messages after last usage
	if lastUsageIdx >= 0 {
		for i := lastUsageIdx + 1; i < len(entries); i++ {
			trailingTokens += estimateTokens(entries[i].Content)
		}
		tokens += trailingTokens
	}
	return
}

// ShouldCompact checks if context tokens exceed the compaction threshold.
func ShouldCompact(contextTokens int, contextWindow int, settings CompactionSettings) bool {
	return contextTokens > contextWindow-settings.ReserveTokens
}

// PrepareCompaction prepares data for the main compaction.
// Returns nil if the last entry is already a compaction or migration is needed.
func PrepareCompaction(entries []SessionEntry, settings CompactionSettings) *CompactionPreparation {
	if len(entries) == 0 {
		return nil
	}

	// Find previous compaction entry
	prevCompactionIdx := -1
	var previousSummary string
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Type == EntryTypeCompaction {
			prevCompactionIdx = i
			if s, ok := entries[i].Details["summary"].(string); ok {
				previousSummary = s
			}
			break
		}
	}

	// If the last entry is already a compaction, no need to compact
	if len(entries) > 0 && entries[len(entries)-1].Type == EntryTypeCompaction {
		return nil
	}

	boundaryStart := 0
	if prevCompactionIdx >= 0 {
		boundaryStart = prevCompactionIdx + 1
	}
	boundaryEnd := len(entries)

	if boundaryStart >= boundaryEnd {
		return nil
	}

	// Compute tokens before compaction
	tokensBefore, _, _, _ := estimateContextTokensWithUsage(entries)

	// Find cut point
	cutResult := FindCutPoint(entries, boundaryStart, boundaryEnd, settings.KeepRecentTokens)

	// Collect messages to summarize
	var messagesToSummarize []string
	for i := boundaryStart; i < cutResult.FirstKeptEntryIndex; i++ {
		messagesToSummarize = append(messagesToSummarize, entries[i].Messages...)
	}

	// If splitting a turn, collect turn prefix messages
	var turnPrefixMessages []string
	if cutResult.IsSplitTurn {
		for i := cutResult.TurnStartIndex; i < cutResult.FirstKeptEntryIndex; i++ {
			turnPrefixMessages = append(turnPrefixMessages, entries[i].Messages...)
		}
	}

	// Extract file operations
	fileOps := extractFileOps(entries, prevCompactionIdx)

	return &CompactionPreparation{
		FirstKeptEntryID:    entries[cutResult.FirstKeptEntryIndex].ID,
		MessagesToSummarize: messagesToSummarize,
		TurnPrefixMessages:  turnPrefixMessages,
		IsSplitTurn:         cutResult.IsSplitTurn,
		TokensBefore:        tokensBefore,
		PreviousSummary:     previousSummary,
		FileOps:             fileOps,
		Settings:            settings,
	}
}

// formatFileList formats a list of files into a compact string.
func formatFileList(files []string) string {
	if len(files) == 0 {
		return "(none)"
	}
	return strings.Join(files, ", ")
}
