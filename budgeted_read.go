package main

import (
	"fmt"
	"os"
	"strings"
)

// ─── Budgeted Read (MiMo-Code P6) ──────────────────────────────────────────
//
// Reads files with a token budget to prevent context overflow during rebuild.
// Two modes:
//   - ReadBudgeted: proportional truncation with continuation hint
//   - ReadBudgetedSectionAware: section-aware truncation preserving structure
//
// MiMo-Code source: session/budgeted-read.ts (118 lines)

// BudgetedReadResult holds the result of a budgeted read.
type BudgetedReadResult struct {
	Text        string
	Truncated   bool
	TotalTokens int
	UsedTokens  int
}

// estimateTokensBudgeted estimates token count from character count.
func estimateTokensBudgeted(text string) int {
	return (len(text) + 3) / 4
}

// ReadBudgeted reads a file with a token budget.
// If the file exceeds the budget, it truncates proportionally with a hint.
func ReadBudgeted(filePath string, budgetTokens int) (*BudgetedReadResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	fullText := string(data)
	totalTokens := estimateTokensBudgeted(fullText)

	if totalTokens <= budgetTokens {
		return &BudgetedReadResult{
			Text:        fullText,
			Truncated:   false,
			TotalTokens: totalTokens,
			UsedTokens:  totalTokens,
		}, nil
	}

	ratio := float64(budgetTokens) / float64(totalTokens)
	truncateLen := int(float64(len(fullText)) * ratio * 0.95)

	cutPoint := truncateLen
	if idx := strings.LastIndex(fullText[:truncateLen], "\n"); idx > 0 {
		cutPoint = idx
	}

	clean := fullText[:cutPoint]
	hint := fmt.Sprintf("\n\n⚠️ Truncated at ~%d tokens. %s is ~%d tokens total. Read(\"%s\", offset=%d) for the rest.",
		budgetTokens, filePath, totalTokens, filePath, len(clean))

	return &BudgetedReadResult{
		Text:        clean + hint,
		Truncated:   true,
		TotalTokens: totalTokens,
		UsedTokens:  estimateTokensBudgeted(clean),
	}, nil
}

// Section represents a section in a structured file.
type Section struct {
	Header     string
	Italic     string
	Body       []string
	IndexLines []string
}

// ParseSections parses a markdown file into sections (## headers).
func ParseSections(text string) (preamble []string, sections []Section) {
	lines := strings.Split(text, "\n")
	var current *Section
	italicSeen := false

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if current != nil {
				sections = append(sections, *current)
			}
			current = &Section{Header: line}
			italicSeen = false
			continue
		}

		if current != nil {
			if !italicSeen && strings.HasPrefix(line, "_") && strings.HasSuffix(line, "_") {
				current.Italic = line
				italicSeen = true
				continue
			}
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- See ") && strings.Contains(trimmed, ".md") {
				current.IndexLines = append(current.IndexLines, line)
			}
			current.Body = append(current.Body, line)
		} else {
			preamble = append(preamble, line)
		}
	}

	if current != nil {
		sections = append(sections, *current)
	}

	return preamble, sections
}

// ReadBudgetedSectionAware reads a file with section-aware truncation.
func ReadBudgetedSectionAware(filePath string, budgetTokens int) (*BudgetedReadResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	fullText := string(data)
	totalTokens := estimateTokensBudgeted(fullText)

	if totalTokens <= budgetTokens {
		return &BudgetedReadResult{
			Text:        fullText,
			Truncated:   false,
			TotalTokens: totalTokens,
			UsedTokens:  totalTokens,
		}, nil
	}

	preamble, sections := ParseSections(fullText)

	var headerParts []string
	headerParts = append(headerParts, preamble...)
	for _, sec := range sections {
		headerParts = append(headerParts, sec.Header)
		if sec.Italic != "" {
			headerParts = append(headerParts, sec.Italic)
		}
		headerParts = append(headerParts, sec.IndexLines...)
	}
	headerOnlyTokens := estimateTokensBudgeted(strings.Join(headerParts, "\n"))

	if headerOnlyTokens >= budgetTokens {
		var skeleton []string
		skeleton = append(skeleton, preamble...)
		for _, sec := range sections {
			skeleton = append(skeleton, sec.Header)
			if sec.Italic != "" {
				skeleton = append(skeleton, sec.Italic)
			}
			skeleton = append(skeleton, sec.IndexLines...)
			skeleton = append(skeleton, "")
		}
		hint := fmt.Sprintf("\n\n⚠️ File extremely large (%d tokens vs budget %d). Only structure shown.\n   Read(\"%s\") for full content.",
			totalTokens, budgetTokens, filePath)
		return &BudgetedReadResult{
			Text:        strings.Join(skeleton, "\n") + hint,
			Truncated:   true,
			TotalTokens: totalTokens,
			UsedTokens:  headerOnlyTokens,
		}, nil
	}

	var out []string
	out = append(out, preamble...)
	used := estimateTokensBudgeted(strings.Join(out, "\n"))

	for _, sec := range sections {
		secHeader := []string{sec.Header}
		if sec.Italic != "" {
			secHeader = append(secHeader, sec.Italic)
		}
		secHeader = append(secHeader, sec.IndexLines...)
		used += estimateTokensBudgeted(strings.Join(secHeader, "\n"))
		out = append(out, secHeader...)

		var bodyLines []string
		for _, line := range sec.Body {
			isIndex := false
			for _, idx := range sec.IndexLines {
				if line == idx {
					isIndex = true
					break
				}
			}
			if !isIndex {
				bodyLines = append(bodyLines, line)
			}
		}

		bodyText := strings.Join(bodyLines, "\n")
		bodyTokens := estimateTokensBudgeted(bodyText)

		if used+bodyTokens <= budgetTokens {
			out = append(out, bodyText)
			used += bodyTokens
		} else {
			remaining := budgetTokens - used
			if remaining > 50 {
				cutLen := int(float64(len(bodyText)) * (float64(remaining) / float64(bodyTokens)) * 0.95)
				if cutLen > len(bodyText) {
					cutLen = len(bodyText)
				}
				out = append(out, bodyText[:cutLen])
				used += remaining
			}
		}
		out = append(out, "")
	}

	hint := fmt.Sprintf("\n\n⚠️ Truncated at ~%d tokens. %s is ~%d tokens total. Read(\"%s\") for full content.",
		budgetTokens, filePath, totalTokens, filePath)
	return &BudgetedReadResult{
		Text:        strings.Join(out, "\n") + hint,
		Truncated:   true,
		TotalTokens: totalTokens,
		UsedTokens:  used,
	}, nil
}
