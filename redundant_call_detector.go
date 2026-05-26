package main

import (
	"strings"
)

// RedundantCallDetector tracks recent tool calls to detect redundant
// operations. This matches openclacky's proactive behavior: when the
// LLM makes multiple similar calls in sequence (e.g., reading 5 files
// one by one), or makes a call that was already made this turn, inject
// a hint to avoid waste.
type RedundantCallDetector struct {
	recentCalls []toolCallRecord // last N tool calls within this turn
}

type toolCallRecord struct {
	toolName string
	input    map[string]any
	result   string // first 200 chars of result
}

// NewRedundantCallDetector creates a fresh detector.
func NewRedundantCallDetector() *RedundantCallDetector {
	return &RedundantCallDetector{}
}

// Record adds a tool call to the recent history.
func (d *RedundantCallDetector) Record(toolName string, input map[string]any, resultPreview string) {
	if len(resultPreview) > 200 {
		resultPreview = resultPreview[:200]
	}
	d.recentCalls = append(d.recentCalls, toolCallRecord{
		toolName: toolName,
		input:    input,
		result:   resultPreview,
	})
	// Keep only last 20 calls per turn
	if len(d.recentCalls) > 20 {
		d.recentCalls = d.recentCalls[len(d.recentCalls)-20:]
	}
}

// Clear resets the detector at turn boundaries.
func (d *RedundantCallDetector) Clear() {
	d.recentCalls = nil
}

// DetectRedundancy checks if a proposed tool call is redundant.
// Returns a hint string if redundancy detected, empty string if not.
//
// Detection patterns:
//   1. Same tool + same file: "You already read this file this turn"
//   2. Same tool + similar pattern: "You already searched this pattern"
//   3. Sequential reads of similar files: "Consider using multi_edit or grep to batch"
func (d *RedundantCallDetector) DetectRedundancy(toolName string, input map[string]any) string {
	// Pattern 1: Read/Edit the same file twice in one turn
	if toolName == "read_file" || toolName == "edit_file" || toolName == "multi_edit" {
		path, _ := input["file_path"].(string)
		if path != "" {
			for _, call := range d.recentCalls {
				if call.toolName == toolName {
					prevPath, _ := call.input["file_path"].(string)
					if prevPath == path {
						return "Note: You already called " + toolName + " on this file this turn. The result is: " + call.result + ". Avoid repeating the same call."
					}
				}
			}
		}
	}

	// Pattern 2: Grep with same/similar pattern twice
	if toolName == "grep" || toolName == "search_files" {
		pattern, _ := input["pattern"].(string)
		if pattern != "" {
			for _, call := range d.recentCalls {
				if call.toolName == toolName || call.toolName == "grep" || call.toolName == "search_files" {
					prevPattern, _ := call.input["pattern"].(string)
					if prevPattern == pattern || strings.Contains(pattern, prevPattern) || strings.Contains(prevPattern, pattern) {
						return "Note: You already searched for a similar pattern this turn. Consider using the previous result instead of repeating the search."
					}
				}
			}
		}
	}

	// Pattern 3: Multiple sequential read_file calls
	if toolName == "read_file" && len(d.recentCalls) >= 3 {
		readCount := 0
		for _, call := range d.recentCalls {
			if call.toolName == "read_file" {
				readCount++
			}
		}
		if readCount >= 3 {
			return "Note: You've already made multiple read_file calls this turn. If you need to read several files, consider using grep to search across them first, or read the most relevant file and use that context."
		}
	}

	return ""
}