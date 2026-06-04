package tools

import (
	"context"
	"fmt"
	"strings"
)

// AskUserQuestionTool allows the model to ask the user questions with multiple-choice options.
type AskUserQuestionTool struct{}

func (*AskUserQuestionTool) Name() string { return "AskUserQuestion" }

func (*AskUserQuestionTool) Description() string {
	return "Prompts the user with a question. Use this when you need to clarify something before " +
		"proceeding. Multiple-choice: presents 2-4 options plus an 'Other' choice. Sensitive: when " +
		"sensitive is true, reads masked password input with no options."
}

func (*AskUserQuestionTool) InputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"questions"},
		"properties": map[string]any{
			"questions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"question", "header"},
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
								"type":     "object",
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
						"sensitive": map[string]any{
							"type":        "boolean",
							"description": "When true, this is a secure input question (e.g. password entry). Input is masked and the answer is redacted in the response.",
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

func (t *AskUserQuestionTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	// Check context early
	select {
	case <-ctx.Done():
		return ToolResult{Output: fmt.Sprintf("Error: AskUserQuestion timed out: %v", ctx.Err()), IsError: true}
	default:
	}

	questionsRaw, ok := params["questions"].([]any)
	if !ok {
		return ToolResultError("questions must be an array")
	}

	type option struct {
		Label       string
		Description string
	}
	type question struct {
		Question  string
		Header    string
		Options   []option
		Sensitive bool
	}

	var questions []question
	for _, raw := range questionsRaw {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		q := question{
			Question:  strVal(m, "question"),
			Header:    strVal(m, "header"),
			Sensitive: boolVal(m, "sensitive"),
		}
		optsRaw, ok := m["options"].([]any)
		if !ok && !q.Sensitive {
			return ToolResultError("each non-sensitive question must have an options array")
		}
		if !q.Sensitive {
			if len(optsRaw) < 2 {
				return ToolResultError("each non-sensitive question must have at least 2 options")
			}
			if len(optsRaw) > 4 {
				return ToolResultError("each non-sensitive question must have at most 4 options")
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
		}
		questions = append(questions, q)
	}

	if len(questions) == 0 {
		return ToolResultError("at least one question is required")
	}
	if len(questions) > 4 {
		return ToolResultError("at most 4 questions allowed")
	}

	answers := make(map[string]string)
	// sensitiveRawValues stores unredacted answers for sensitive questions.
	// These are included in the tool result (so the LLM can use them, e.g.
	// as sudo_password for exec), but masked in terminal display.
	sensitiveRawValues := make(map[string]string)

	for qIdx, q := range questions {
		// Sensitive question: password/credential input with masked display
		if q.Sensitive {
			fmt.Printf("\n┌─ %s ──────────────────────────────────────\n", q.Header)
			fmt.Printf("│  %s\n", q.Question)
			fmt.Printf("│\n")
			fmt.Printf("│  (input hidden)\n")
			fmt.Printf("└─────────────────────────────────────────────\n")
			fmt.Printf("  Password: ")

			input, err := readLineHidden(ctx)
			if err != nil {
				if ctx.Err() != nil {
					if len(answers) > 0 {
						var sb strings.Builder
						for i, prevQ := range questions[:qIdx] {
							if i > 0 {
								sb.WriteString("\n")
							}
							sb.WriteString(fmt.Sprintf("Q: %s\nA: [REDACTED]\n", prevQ.Question))
						}
						sb.WriteString(fmt.Sprintf("\n[Question %d/%d was cancelled by user — proceeding without answer]\n", qIdx+1, len(questions)))
						return ToolResultOK(sb.String())
					}
					return ToolResultOK("[Sensitive question cancelled by user]")
				}
				return ToolResultError(fmt.Sprintf("failed to read sensitive input: %v", err))
			}

			input = strings.TrimSpace(input)
			if input == "" {
				fmt.Printf("  Empty password. Retrying.\n")
				// Allow retry for empty password
				continue
			}
			sensitiveRawValues[q.Question] = input
			answers[q.Question] = "[SENSITIVE]"
			fmt.Printf("  Password received.\n")
			continue
		}

		// Non-sensitive question: multiple-choice with "Other" option
		allOptions := make([]option, len(q.Options)+1)
		copy(allOptions, q.Options)
		customIdx := len(q.Options) + 1
		allOptions[customIdx-1] = option{
			Label:       "Other",
			Description: "Type your own answer instead of choosing one of the above",
		}

		fmt.Printf("\n┌─ %s ──────────────────────────────────────\n", q.Header)
		fmt.Printf("│  %s\n", q.Question)
		fmt.Printf("│\n")
		for i, opt := range allOptions {
			fmt.Printf("│  %d. %s — %s\n", i+1, opt.Label, opt.Description)
		}
		fmt.Printf("│\n")
		fmt.Printf("│  Enter a number (1-%d): ", len(allOptions))
		fmt.Printf("│\n")
		fmt.Printf("└─────────────────────────────────────────────\n")

		for {
			input, err := readLineWithContext(ctx)
			if err != nil {
				if ctx.Err() != nil {
					if len(answers) > 0 {
						var sb strings.Builder
						for i, prevQ := range questions[:qIdx] {
							if i > 0 {
								sb.WriteString("\n")
							}
							a := answers[prevQ.Question]
							if sensitiveRawValues[prevQ.Question] != "" {
								a = "[REDACTED]"
							}
							sb.WriteString(fmt.Sprintf("Q: %s\nA: %s (already answered before cancellation)\n", prevQ.Question, a))
						}
						sb.WriteString(fmt.Sprintf("\n[Question %d/%d was cancelled by user — proceeding without answer]\n", qIdx+1, len(questions)))
						return ToolResultOK(sb.String())
					}
					return ToolResultOK("[Question cancelled by user — proceeding without answer]")
				}
				return ToolResultError(fmt.Sprintf("failed to read input: %v", err))
			}

			input = strings.TrimSpace(input)
			if len(input) > 3 {
				input = input[:3]
			}
			num, err := parseNumber(input)
			if err != nil || num < 1 || num > len(allOptions) {
				fmt.Printf("  Please enter a number between 1 and %d: ", len(allOptions))
				continue
			}

			if num == customIdx {
				fmt.Printf("  Your answer: ")
				customInput, err := readLineWithContext(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return ToolResultOK("[Custom input cancelled by user]")
					}
					return ToolResultError(fmt.Sprintf("failed to read custom input: %v", err))
				}
				customInput = strings.TrimSpace(customInput)
				if customInput == "" {
					fmt.Printf("  Empty input. Please enter your answer or select a numbered option.\n")
					continue
				}
				answers[q.Question] = customInput
				fmt.Printf("  Selected: (custom) %s\n", customInput)
				break
			}

			answers[q.Question] = allOptions[num-1].Label
			fmt.Printf("  Selected: %s\n", allOptions[num-1].Label)
			break
		}
	}

	// Build output summary.
	// For sensitive questions, include the raw value so the LLM can use it
	// (e.g., as sudo_password for exec), but mark it with [SENSITIVE] prefix.
	var sb strings.Builder
	for i, q := range questions {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("Q: %s\n", q.Question))
		if raw, ok := sensitiveRawValues[q.Question]; ok {
			sb.WriteString(fmt.Sprintf("A: [SENSITIVE] %s\n", raw))
		} else {
			sb.WriteString(fmt.Sprintf("A: %s\n", answers[q.Question]))
		}
	}

	return ToolResultOK(sb.String())
}

func (t *AskUserQuestionTool) Execute(params map[string]any) ToolResult {
	return t.ExecuteContext(context.Background(), params)
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

// boolVal extracts a bool from a map[string]any, defaulting to false.
func boolVal(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}
