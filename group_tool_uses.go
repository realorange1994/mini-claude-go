package main

import "strconv"

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

