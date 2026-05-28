// Package messages provides message types and transformers for the coding agent.
package messages

import (
	"fmt"
	"strings"
)

const (
	// CompactionSummaryPrefix/Suffix wrap compaction summaries so the model treats them as metadata.
	CompactionSummaryPrefix = `The conversation history before this point was compacted into the following summary:

<summary>
`
	CompactionSummarySuffix = `
</summary>`

	// BranchSummaryPrefix/Suffix wrap branch return summaries.
	BranchSummaryPrefix = `The following is a summary of a branch that this conversation came back from:

<summary>
`
	BranchSummarySuffix = `</summary>`
)

// BashExecutionText formats a bash execution result as user-facing text for LLM context.
func BashExecutionText(command, output string, exitCode int, cancelled bool, truncated bool, fullPath string) string {
	var b strings.Builder
	b.WriteString("Ran `" + command + "`\n")
	if output != "" {
		b.WriteString("```\n" + output + "\n```")
	} else {
		b.WriteString("(no output)")
	}
	if cancelled {
		b.WriteString("\n\n(command cancelled)")
	} else if exitCode != 0 {
		b.WriteString(fmt.Sprintf("\n\nCommand exited with code %d", exitCode))
	}
	if truncated && fullPath != "" {
		b.WriteString(fmt.Sprintf("\n\n[Output truncated. Full output: %s]", fullPath))
	}
	return b.String()
}
