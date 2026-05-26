package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

// ToolListFingerprint tracks the hash of the complete tool list to detect
// schema drift that would invalidate the prompt cache.
//
// DeepSeek-Reasonix pattern (mcp/drift.ts): when the tool list changes
// (tools added, removed, or schemas modified), the entire prompt prefix
// shifts and all cached tokens are lost. By tracking the fingerprint,
// we can:
//   - Skip unnecessary tool re-registration when nothing changed
//   - Log when cache invalidation is caused by tool list drift
//   - Pin the tool list between turns to preserve cache hits
type ToolListFingerprint struct {
	lastHash string
	// toolCount tracks how many tools were registered
	toolCount int
	// driftDetected is true if the tool list changed since last check
	driftDetected bool
}

// NewToolListFingerprint creates a new fingerprint tracker.
func NewToolListFingerprint() *ToolListFingerprint {
	return &ToolListFingerprint{}
}

// ComputeHash computes a deterministic hash of the tool list.
// Tool names are sorted to ensure stable hashing.
func ComputeToolListHash(toolNames []string, toolSchemas map[string]string) string {
	h := sha256.New()

	// Sort tool names for deterministic ordering
	sorted := make([]string, len(toolNames))
	copy(sorted, toolNames)
	// Simple insertion sort (tool lists are small, <100 items)
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}

	for _, name := range sorted {
		h.Write([]byte(name))
		h.Write([]byte{0}) // null separator
		if schema, ok := toolSchemas[name]; ok {
			h.Write([]byte(schema))
		}
		h.Write([]byte{0})
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// CheckAndRecord checks if the tool list has drifted since the last call.
// Returns true if drift was detected. Records the new fingerprint.
func (f *ToolListFingerprint) CheckAndRecord(toolNames []string, toolSchemas map[string]string) bool {
	newHash := ComputeToolListHash(toolNames, toolSchemas)
	newCount := len(toolNames)

	f.driftDetected = f.lastHash != "" && (f.lastHash != newHash || f.toolCount != newCount)

	f.lastHash = newHash
	f.toolCount = newCount

	return f.driftDetected
}

// DriftDetected returns whether drift was detected in the last CheckAndRecord call.
func (f *ToolListFingerprint) DriftDetected() bool {
	return f.driftDetected
}

// LastHash returns the current fingerprint hash.
func (f *ToolListFingerprint) LastHash() string {
	return f.lastHash
}

// FoldSummaryPin tracks important content that must survive compaction.
//
// DeepSeek-Reasonix pattern: when folding/compacting the context, certain
// content must be preserved (active skill, constraints, tool results in
// progress). This struct tracks what needs to be pinned.
type FoldSummaryPin struct {
	// ActiveSkills tracks currently active skill names
	ActiveSkills []string
	// Constraints tracks user-imposed constraints that must survive folding
	Constraints []string
	// InProgressToolCall tracks tool calls that are awaiting results
	InProgressToolCall string
	SystemPrompt       string // caches system prompt for extracting pinned constraints
}

// NewFoldSummaryPin creates a new pin tracker.
func NewFoldSummaryPin() *FoldSummaryPin {
	return &FoldSummaryPin{}
}

// SetActiveSkill records the currently active skill.
func (p *FoldSummaryPin) SetActiveSkill(skillName string) {
	// Replace, don't append — only one active skill at a time
	p.ActiveSkills = []string{skillName}
}

// AddConstraint adds a constraint that must survive folding.
func (p *FoldSummaryPin) AddConstraint(constraint string) {
	// Deduplicate
	for _, c := range p.Constraints {
		if c == constraint {
			return
		}
	}
	p.Constraints = append(p.Constraints, constraint)
}

// SetInProgressToolCall records a tool call awaiting results.
func (p *FoldSummaryPin) SetInProgressToolCall(toolUseID string) {
	p.InProgressToolCall = toolUseID
}

// SetSystemPrompt caches the system prompt for extracting pinned constraints during compaction.
func (p *FoldSummaryPin) SetSystemPrompt(systemPrompt string) {
	p.SystemPrompt = systemPrompt
}

// BuildPinPrompt generates a prompt fragment that ensures pinned content
// survives compaction. This is prepended to the compaction summary.
func (p *FoldSummaryPin) BuildPinPrompt() string {
	var parts []string

	// Extract pinned constraints from system prompt (DeepSeek-Reasonix pattern)
	if p.SystemPrompt != "" {
		if constraints := extractPinnedConstraints(p.SystemPrompt); constraints != "" {
			parts = append(parts, "[PINNED CONSTRAINTS]\n"+constraints)
		}
	}

	if len(p.ActiveSkills) > 0 {
		skillsJSON, _ := json.Marshal(p.ActiveSkills)
		parts = append(parts, "active_skills="+string(skillsJSON))
	}
	if len(p.Constraints) > 0 {
		constraintsJSON, _ := json.Marshal(p.Constraints)
		parts = append(parts, "constraints="+string(constraintsJSON))
	}
	if p.InProgressToolCall != "" {
		parts = append(parts, "in_progress_tool="+p.InProgressToolCall)
	}

	if len(parts) == 0 {
		return ""
	}
	return "[PERSIST] " + strings.Join(parts, " ")
}

// Clear resets all pin state.
func (p *FoldSummaryPin) Clear() {
	p.ActiveSkills = nil
	p.Constraints = nil
	p.InProgressToolCall = ""
}