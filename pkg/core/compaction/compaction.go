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
