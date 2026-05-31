// Package systemprompt builds system prompts aligned to pi's system-prompt.ts.
package systemprompt

import (
	"fmt"
	"miniclaudecode-go/pkg/core/skills"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ContextFile is a pre-loaded context file with its path and content.
type ContextFile struct {
	Path    string
	Content string
}

// BuildSystemPromptOptions configures how the system prompt is built.
// Aligned to TS BuildSystemPromptOptions.
type BuildSystemPromptOptions struct {
	// CustomPrompt replaces the default system prompt.
	CustomPrompt string
	// SelectedTools lists tools to include. Default: ["read","bash","edit","write"].
	SelectedTools []string
	// ToolSnippets provides one-line descriptions keyed by tool name.
	// A tool only appears in the "Available tools" list when it has a snippet.
	ToolSnippets map[string]string
	// PromptGuidelines are additional guideline bullets.
	PromptGuidelines []string
	// AppendSystemPrompt is text appended to the end of the system prompt.
	AppendSystemPrompt string
	// Cwd is the working directory.
	Cwd string
	// ContextFiles are pre-loaded context files.
	ContextFiles []ContextFile
	// Skills are discovered skills to include as <available_skills>.
	Skills []skills.Skill
}

// BuildSystemPrompt constructs the full system prompt.
// Aligned to TS buildSystemPrompt().
func BuildSystemPrompt(opts BuildSystemPromptOptions) string {
	cwd := opts.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	// Normalize path separators for display
	cwd = filepath.ToSlash(cwd)

	now := time.Now()
	date := now.Format("2006-01-02")

	appendSection := ""
	if opts.AppendSystemPrompt != "" {
		appendSection = "\n\n" + opts.AppendSystemPrompt
	}

	// Custom prompt path
	if opts.CustomPrompt != "" {
		prompt := opts.CustomPrompt
		if appendSection != "" {
			prompt += appendSection
		}
		// Append context files
		if len(opts.ContextFiles) > 0 {
			prompt += "\n\n<project_context>\n\n"
			prompt += "Project-specific instructions and guidelines:\n\n"
			for _, cf := range opts.ContextFiles {
				prompt += fmt.Sprintf("<project_instructions path=\"%s\">\n%s\n</project_instructions>\n\n", cf.Path, cf.Content)
			}
			prompt += "</project_context>\n"
		}
		// Append skills if read tool is available
		hasRead := len(opts.SelectedTools) == 0 || contains(opts.SelectedTools, "read")
		if hasRead && len(opts.Skills) > 0 {
			prompt += skills.FormatSkillsForPrompt(opts.Skills)
		}
		prompt += fmt.Sprintf("\n\nCurrent date: %s", date)
		prompt += fmt.Sprintf("\nCurrent working directory: %s", cwd)
		return prompt
	}

	// Default prompt
	selectedTools := opts.SelectedTools
	if len(selectedTools) == 0 {
		selectedTools = []string{"read", "bash", "edit", "write"}
	}

	// Visible tools = tools that have snippets
	var visibleTools []string
	for _, name := range selectedTools {
		if opts.ToolSnippets[name] != "" {
			visibleTools = append(visibleTools, name)
		}
	}

	var toolsList string
	if len(visibleTools) > 0 {
		var lines []string
		for _, name := range visibleTools {
			lines = append(lines, fmt.Sprintf("- %s: %s", name, opts.ToolSnippets[name]))
		}
		toolsList = strings.Join(lines, "\n")
	} else {
		toolsList = "(none)"
	}

	// Guidelines
	hasBash := contains(selectedTools, "bash")
	hasGrep := contains(selectedTools, "grep")
	hasFind := contains(selectedTools, "find")
	hasLs := contains(selectedTools, "ls")
	hasRead := contains(selectedTools, "read")

	var guidelineSet []string
	var guidelinesList []string
	addGuideline := func(g string) {
		for _, existing := range guidelineSet {
			if existing == g {
				return
			}
		}
		guidelineSet = append(guidelineSet, g)
		guidelinesList = append(guidelinesList, g)
	}

	if hasBash && !hasGrep && !hasFind && !hasLs {
		addGuideline("Use bash for file operations like ls, rg, find")
	} else if hasBash && (hasGrep || hasFind || hasLs) {
		addGuideline("Prefer grep/find/ls tools over bash for file exploration (faster, respects .gitignore)")
	}

	for _, g := range opts.PromptGuidelines {
		g = strings.TrimSpace(g)
		if g != "" {
			addGuideline(g)
		}
	}

	addGuideline("Be concise in your responses")
	addGuideline("Show file paths clearly when working with files")

	var guidelines string
	for _, g := range guidelinesList {
		guidelines += "- " + g + "\n"
	}

	prompt := fmt.Sprintf(`You are an expert coding assistant operating inside a coding agent harness. You help users by reading files, executing commands, editing code, and writing new files.

Available tools:
%s

In addition to the tools above, you may have access to other custom tools depending on the project.

Guidelines:
%s`, toolsList, guidelines)

	if appendSection != "" {
		prompt += appendSection
	}

	// Append context files
	if len(opts.ContextFiles) > 0 {
		prompt += "\n\n<project_context>\n\n"
		prompt += "Project-specific instructions and guidelines:\n\n"
		for _, cf := range opts.ContextFiles {
			prompt += fmt.Sprintf("<project_instructions path=\"%s\">\n%s\n</project_instructions>\n\n", cf.Path, cf.Content)
		}
		prompt += "</project_context>\n"
	}

	// Append skills
	if hasRead && len(opts.Skills) > 0 {
		prompt += skills.FormatSkillsForPrompt(opts.Skills)
	}

	prompt += fmt.Sprintf("\n\nCurrent date: %s", date)
	prompt += fmt.Sprintf("\nCurrent working directory: %s", cwd)

	return prompt
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
