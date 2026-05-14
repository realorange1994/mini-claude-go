package main

import "strconv"

// ─── Hook Summary Types ──────────────────────────────────────────────────────

// collapseHookSummaries ported from upstream: src/utils/collapseHookSummaries.ts

// HookSummary represents a system stop hook summary message.
type HookSummary struct {
	Type                 string
	Subtype              string
	HookLabel            string
	HookCount            int
	HookInfos            []any
	HookErrors           []any
	PreventedContinuation bool
	HasOutput            bool
	TotalDurationMs      int
}

// IsLabeledHookSummary returns true if the message is a labeled hook summary.
func IsLabeledHookSummary(msg any) bool {
	s, ok := msg.(*HookSummary)
	if !ok {
		return false
	}
	return s.Type == "system" && s.Subtype == "stop_hook_summary" && s.HookLabel != ""
}

// CollapseHookSummaries collapses consecutive hook summary messages with the same hookLabel
// (e.g. PostToolUse) into a single summary.
func CollapseHookSummaries(messages []any) []any {
	result := make([]any, 0, len(messages))
	i := 0

	for i < len(messages) {
		msg := messages[i]
		if IsLabeledHookSummary(msg) {
			s := msg.(*HookSummary)
			label := s.HookLabel
			var group []*HookSummary

			for i < len(messages) {
				next := messages[i]
				if !IsLabeledHookSummary(next) || next.(*HookSummary).HookLabel != label {
					break
				}
				group = append(group, next.(*HookSummary))
				i++
			}

			if len(group) == 1 {
				result = append(result, msg)
			} else {
				// Collapse the group into a single summary
				merged := &HookSummary{
					Type:      group[0].Type,
					Subtype:   group[0].Subtype,
					HookLabel: label,
				}
				for _, m := range group {
					merged.HookCount += m.HookCount
					merged.HookInfos = append(merged.HookInfos, m.HookInfos...)
					merged.HookErrors = append(merged.HookErrors, m.HookErrors...)
					if m.PreventedContinuation {
						merged.PreventedContinuation = true
					}
					if m.HasOutput {
						merged.HasOutput = true
					}
					if m.TotalDurationMs > merged.TotalDurationMs {
						merged.TotalDurationMs = m.TotalDurationMs
					}
				}
				result = append(result, merged)
			}
		} else {
			result = append(result, msg)
			i++
		}
	}

	return result
}

// ─── Tool Use Grouping Types ─────────────────────────────────────────────────

// groupToolUses ported from upstream: src/utils/groupToolUses.ts

// ToolUseGroup represents a grouping of consecutive tool uses.
type ToolUseGroup struct {
	ID           string
	Name         string
	Input        string
	Output       string
	Status       string
	IsGrouped    bool
	ToolUses     []ToolUseEntry
}

// ToolUseEntry represents a single tool use invocation.
type ToolUseEntry struct {
	ID     string
	Name   string
	Input  string
	Output string
	Status string
}

// ApplyGrouping groups consecutive identical tool uses.
// This is used to collapse repeated tool invocations (like multiple file reads)
// into a single expandable group in the UI.
func ApplyGrouping(toolUses []ToolUseEntry) []ToolUseGroup {
	var result []ToolUseGroup

	for i := 0; i < len(toolUses); i++ {
		entry := toolUses[i]

		// Look ahead for consecutive same-name tool uses
		var group []ToolUseEntry
		group = append(group, entry)

		for j := i + 1; j < len(toolUses); j++ {
			if toolUses[j].Name == entry.Name {
				group = append(group, toolUses[j])
				i = j
			} else {
				break
			}
		}

		if len(group) == 1 {
			result = append(result, ToolUseGroup{
				ID:        entry.ID,
				Name:      entry.Name,
				Input:     entry.Input,
				Output:    entry.Output,
				Status:    entry.Status,
				IsGrouped: false,
				ToolUses:  group,
			})
		} else {
			result = append(result, ToolUseGroup{
				ID:        entry.ID,
				Name:      entry.Name,
				Input:     entry.Input,
				Output:    entry.Output,
				Status:    entry.Status,
				IsGrouped: true,
				ToolUses:  group,
			})
		}
	}

	return result
}

// RenderGroupedToolUse returns a display string for a (possibly grouped) tool use.
func RenderGroupedToolUse(group ToolUseGroup) string {
	if !group.IsGrouped {
		return group.Name + "(" + group.ID + ")"
	}
	count := len(group.ToolUses)
	return group.Name + "(" + group.ID + ") [x" + strconv.Itoa(count) + "]"
}
