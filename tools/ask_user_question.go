package tools

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// AskUserQuestionTool allows the model to ask the user questions with multiple-choice options.
type AskUserQuestionTool struct{}

func (*AskUserQuestionTool) Name() string { return "AskUserQuestion" }

func (*AskUserQuestionTool) Description() string {
	return "Prompts the user with a multiple-choice question. Use this when you need to " +
		"clarify something before proceeding. The question is presented with 2-4 options, " +
		"and the user selects one by entering a number."
}

func (*AskUserQuestionTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"questions"},
		"properties": map[string]any{
			"questions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"required": []string{"question", "header", "options"},
					"properties": map[string]any{
						"question": map[string]any{
							"type":        "string",
							"description": "The complete question to ask. Should be clear, specific, and end with a question mark.",
						},
						"header": map[string]any{
							"type":        "string",
							"description": "Very short label displayed as a chip/tag (max 12 chars). Examples: 'Auth method', 'Library', 'Approach'.",
						},
						"options": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"required": []string{"label", "description"},
								"properties": map[string]any{
									"label": map[string]any{
										"type":        "string",
										"description": "The display text for this option (1-5 words).",
									},
									"description": map[string]any{
										"type":        "string",
										"description": "Explanation of what this option means or what will happen if chosen.",
									},
								},
							},
							"description": "2-4 choices. Each option should be a distinct, mutually exclusive choice.",
						},
					},
				},
				"description": "Questions to ask the user (1-4 questions)",
			},
		},
	}
}

func (*AskUserQuestionTool) CheckPermissions(params map[string]any) PermissionResult {
	return PermissionResultPassthrough() // Always allowed - user must interact to proceed
}

func (*AskUserQuestionTool) Execute(params map[string]any) ToolResult {
	questionsRaw, ok := params["questions"].([]any)
	if !ok {
		return ToolResultError("questions must be an array")
	}

	type option struct {
		Label       string
		Description string
	}
	type question struct {
		Question string
		Header   string
		Options  []option
	}

	var questions []question
	for _, raw := range questionsRaw {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		q := question{
			Question: strVal(m, "question"),
			Header:   strVal(m, "header"),
		}
		optsRaw, ok := m["options"].([]any)
		if !ok {
			return ToolResultError("each question must have an options array")
		}
		if len(optsRaw) < 2 {
			return ToolResultError("each question must have at least 2 options")
		}
		if len(optsRaw) > 4 {
			return ToolResultError("each question must have at most 4 options")
		}
		for _, oRaw := range optsRaw {
			om, ok := oRaw.(map[string]any)
			if !ok {
				continue
			}
			q.Options = append(q.Options, option{
				Label:       strVal(om, "label"),
				Description: strVal(om, "description"),
			})
		}
		questions = append(questions, q)
	}

	if len(questions) == 0 {
		return ToolResultError("at least one question is required")
	}
	if len(questions) > 4 {
		return ToolResultError("at most 4 questions allowed")
	}

	reader := bufio.NewReader(os.Stdin)
	answers := make(map[string]string)

	for _, q := range questions {
		fmt.Printf("\n┌─ %s ──────────────────────────────────────\n", q.Header)
		fmt.Printf("│  %s\n", q.Question)
		fmt.Printf("│\n")
		for i, opt := range q.Options {
			fmt.Printf("│  %d. %s — %s\n", i+1, opt.Label, opt.Description)
		}
		fmt.Printf("│\n")
		fmt.Printf("│  Enter a number (1-%d): ", len(q.Options))
		fmt.Printf("│\n")
		fmt.Printf("└─────────────────────────────────────────────\n")

		for {
			input, err := reader.ReadString('\n')
			if err != nil {
				return ToolResultError(fmt.Sprintf("failed to read input: %v", err))
			}
			input = strings.TrimSpace(input)
			if len(input) > 3 {
				input = input[:3]
			}
			num, err := parseNumber(input)
			if err != nil || num < 1 || num > len(q.Options) {
				fmt.Printf("  Please enter a number between 1 and %d: ", len(q.Options))
				continue
			}
			answers[q.Question] = q.Options[num-1].Label
			fmt.Printf("  Selected: %s\n", q.Options[num-1].Label)
			break
		}
	}

	// Build output summary
	var sb strings.Builder
	for i, q := range questions {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("Q: %s\n", q.Question))
		sb.WriteString(fmt.Sprintf("A: %s\n", answers[q.Question]))
	}

	return ToolResultOK(sb.String())
}

func parseNumber(s string) (int, error) {
	num := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid number: %s", s)
		}
		num = num*10 + int(c-'0')
	}
	return num, nil
}

