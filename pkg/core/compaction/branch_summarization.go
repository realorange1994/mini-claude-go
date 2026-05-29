// Package compaction provides branch summarization logic.
// Aligned to pi's branch-summarization.ts.
package compaction

import (
	"context"
	"fmt"
	"strings"
)

// BranchSummaryResult is returned by GenerateBranchSummary.
type BranchSummaryResult struct {
	Summary      string
	ReadFiles    []string
	ModifiedFiles []string
	Aborted      bool
	Error        string
}

// BranchSummaryDetails stores cumulative file tracking across nested branch summaries.
type BranchSummaryDetails struct {
	ReadFiles    []string `json:"readFiles"`
	ModifiedFiles []string `json:"modifiedFiles"`
}

// BranchPreparation is the output of PrepareBranchEntries.
type BranchPreparation struct {
	Messages    []string
	FileOps     FileOperations
	TotalTokens int
}

// CollectEntriesResult is the output of CollectEntriesForBranchSummary.
type CollectEntriesResult struct {
	Entries         []SessionEntry
	CommonAncestorID string
}

// GenerateBranchSummaryOptions configures the LLM summarization call.
type GenerateBranchSummaryOptions struct {
	ReserveTokens int // default 16384
	CustomInstructions string
	ReplaceInstructions bool // If true, replaces default prompt instead of appending
}

// Branch summarization constants, aligned to TS.
const (
	// BRANCH_SUMMARY_PREAMBLE is prepended to every generated branch summary.
	BRANCH_SUMMARY_PREAMBLE = "The user explored a different conversation branch before returning here.\nSummary of that exploration:\n\n"

	// BRANCH_SUMMARY_PROMPT is the structured prompt for branch summarization.
	BRANCH_SUMMARY_PROMPT = `Summarize this conversation branch. The user explored this branch before returning to the main conversation.

Produce a structured summary in the following format:

## Goal
[What the user was trying to achieve in this branch]

## Constraints & Preferences
[Any constraints or preferences mentioned]

## Progress
- Done: [Completed tasks and decisions]
- In Progress: [Ongoing work when they left]
- Blocked: [Any blockers encountered]

## Key Decisions
[Important decisions made in this branch]

## Next Steps
[What would need to happen next if they return to this branch]

Keep each section concise. Preserve exact file paths, function names, and error messages.`
)

// PrepareBranchEntries converts entries into messages with a token budget.
// Two-pass approach: first pass collects file operations, second pass collects messages.
func PrepareBranchEntries(entries []SessionEntry, tokenBudget int) BranchPreparation {
	fileOps := FileOperations{}
	readSet := make(map[string]bool)
	modSet := make(map[string]bool)

	// First pass: collect file operations from all entries
	for _, e := range entries {
		if e.Details != nil {
			// From branch_summary entries (cumulative file tracking)
			if rf, ok := e.Details["readFiles"].([]string); ok {
				for _, f := range rf {
					readSet[f] = true
				}
			}
			if mf, ok := e.Details["modifiedFiles"].([]string); ok {
				for _, f := range mf {
					modSet[f] = true
				}
			}
		}
		// Extract from tool calls in messages
		for _, msg := range e.Messages {
			files := extractFileReferences([]string{msg})
			for _, f := range files {
				if e.Type == EntryTypeToolResult {
					readSet[f] = true
				} else {
					modSet[f] = true
				}
			}
		}
	}

	for f := range readSet {
		fileOps.ReadFiles = append(fileOps.ReadFiles, f)
	}
	for f := range modSet {
		fileOps.ModifiedFiles = append(fileOps.ModifiedFiles, f)
	}

	// Second pass: collect messages backward until budget exceeded
	var messages []string
	totalTokens := 0
	ninetyPercent := int(float64(tokenBudget) * 0.9)

	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		entryTokens := estimateTokens(e.Content)

		// Force-include compaction and branch_summary entries if under 90% budget
		forceInclude := (e.Type == EntryTypeCompaction || e.Type == EntryTypeBranchSummary) && totalTokens < ninetyPercent

		if totalTokens+entryTokens > tokenBudget && !forceInclude {
			break
		}

		messages = append(messages, e.Messages...)
		totalTokens += entryTokens
	}

	// Reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return BranchPreparation{
		Messages:    messages,
		FileOps:     fileOps,
		TotalTokens: totalTokens,
	}
}

// GenerateBranchSummary generates a summary for a conversation branch.
// Aligned to TS generateBranchSummary().
func GenerateBranchSummary(ctx context.Context, entries []SessionEntry, llmClient LLMClient, model string, opts GenerateBranchSummaryOptions) BranchSummaryResult {
	reserveTokens := opts.ReserveTokens
	if reserveTokens <= 0 {
		reserveTokens = 16384
	}

	// Use a default context window estimate
	contextWindow := 128000
	tokenBudget := contextWindow - reserveTokens

	prep := PrepareBranchEntries(entries, tokenBudget)
	if len(prep.Messages) == 0 {
		return BranchSummaryResult{Summary: "No content to summarize"}
	}

	// Serialize messages
	conversation := strings.Join(prep.Messages, "\n---\n")

	// Build prompt
	prompt := ""
	if opts.ReplaceInstructions && opts.CustomInstructions != "" {
		prompt = "<conversation>\n" + conversation + "\n</conversation>\n\n" + opts.CustomInstructions
	} else {
		prompt = "<conversation>\n" + conversation + "\n</conversation>\n\n" + BRANCH_SUMMARY_PROMPT
		if opts.CustomInstructions != "" {
			prompt += "\n\n" + opts.CustomInstructions
		}
	}

	maxTokens := 2048
	result, err := llmClient.Generate(ctx, model, "You are a coding assistant summarizer.", prompt, maxTokens)
	if err != nil {
		return BranchSummaryResult{Error: fmt.Sprintf("branch summary generation failed: %v", err)}
	}

	// Prepend preamble
	summary := BRANCH_SUMMARY_PREAMBLE + result

	// Append file operations
	if len(prep.FileOps.ReadFiles) > 0 || len(prep.FileOps.ModifiedFiles) > 0 {
		summary += "\n\nFiles read: " + formatFileList(prep.FileOps.ReadFiles)
		summary += "\nFiles modified: " + formatFileList(prep.FileOps.ModifiedFiles)
	}

	return BranchSummaryResult{
		Summary:      summary,
		ReadFiles:    prep.FileOps.ReadFiles,
		ModifiedFiles: prep.FileOps.ModifiedFiles,
	}
}
