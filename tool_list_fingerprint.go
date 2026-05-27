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

// PrefixFingerprint tracks the hash of the complete prompt prefix (system + tools + fewshots).
// When the prefix changes (system prompt, tool schemas, or few-shot examples), the entire
// cached prefix in the API is invalidated. By tracking the fingerprint, we can detect
// prefix drift and log cache invalidation causes.
//
// DeepSeek-Reasonix pattern (memory/runtime.ts: ImmutablePrefix.fingerprint): the prefix
// is stable across turns; only /new or tool mutations change it. Each change costs a
// full cache miss on the next API call.
type PrefixFingerprint struct {
	lastHash string
	driftDetected bool
}

// NewPrefixFingerprint creates a new prefix fingerprint tracker.
func NewPrefixFingerprint() *PrefixFingerprint {
	return &PrefixFingerprint{}
}

// CheckAndRecord checks if the prompt prefix has drifted since the last call.
// Returns true if drift was detected. Records the new fingerprint.
func (f *PrefixFingerprint) CheckAndRecord(system string, toolSchemas map[string]string, fewshots []map[string]any) bool {
	newHash := computePrefixFingerprint(system, toolSchemas, fewshots)
	f.driftDetected = f.lastHash != "" && f.lastHash != newHash
	f.lastHash = newHash
	return f.driftDetected
}

// DriftDetected returns whether drift was detected in the last CheckAndRecord call.
func (f *PrefixFingerprint) DriftDetected() bool {
	return f.driftDetected
}

// LastHash returns the current prefix fingerprint hash.
func (f *PrefixFingerprint) LastHash() string {
	return f.lastHash
}

// DriftKind classifies the type of tool-list drift for cache impact assessment.
// Ordered by "cache cost" — identity and append are nearly free; reorder is catastrophic.
type DriftKind string

const (
	DriftIdentity DriftKind = "identity"  // No change - same tools, same order, same content
	DriftAppend   DriftKind = "append"    // New tools added at end, existing unchanged
	DriftEdit     DriftKind = "edit"      // Same tool set but schemas changed
	DriftReorder  DriftKind = "reorder"   // Same tool set but order changed - cache as bad as structural
	DriftRemove   DriftKind = "remove"    // Tools removed - catastrophic regardless of other changes
)

// DriftReport contains the classification of tool list drift
type DriftReport struct {
	Kind    DriftKind
	Added   []string
	Removed []string
	Edited  []string
}

// classifyToolListDrift analyzes before/after tool lists and classifies the drift.
// This helps understand cache impact: identity and append are cheap, others cause cache misses.
func classifyToolListDrift(before []string, after []string, beforeSchemas, afterSchemas map[string]string) DriftReport {
	beforeSet := make(map[string]bool)
	for _, n := range before {
		beforeSet[n] = true
	}
	afterSet := make(map[string]bool)
	for _, n := range after {
		afterSet[n] = true
	}

	var added, removed []string
	for _, n := range after {
		if !beforeSet[n] {
			added = append(added, n)
		}
	}
	for _, n := range before {
		if !afterSet[n] {
			removed = append(removed, n)
		}
	}

	var edited []string
	sharedLen := min(len(before), len(after))
	for i := 0; i < sharedLen; i++ {
		if before[i] == after[i] && beforeSchemas[before[i]] != afterSchemas[after[i]] {
			edited = append(edited, before[i])
		}
	}

	// Identity: same length, same names in order, same content
	if len(before) == len(after) && len(edited) == 0 {
		sameOrder := true
		for i := range before {
			if before[i] != after[i] {
				sameOrder = false
				break
			}
		}
		if sameOrder {
			return DriftReport{Kind: DriftIdentity, Added: nil, Removed: nil, Edited: nil}
		}
	}

	// Remove anywhere: catastrophic
	if len(removed) > 0 {
		return DriftReport{Kind: DriftRemove, Added: added, Removed: removed, Edited: nil}
	}

	// Append: every before-tool stays, new ones at end
	if len(after) > len(before) {
		allMatch := true
		for i := range before {
			if before[i] != after[i] || beforeSchemas[before[i]] != afterSchemas[after[i]] {
				allMatch = false
				break
			}
		}
		if allMatch {
			return DriftReport{Kind: DriftAppend, Added: added, Removed: nil, Edited: nil}
		}
	}

	// Same name set? Then positions or content changed
	sameNameSet := len(beforeSet) == len(afterSet)
	if sameNameSet {
		for n := range beforeSet {
			if !afterSet[n] {
				sameNameSet = false
				break
			}
		}
		if sameNameSet {
			positionsMatch := true
			for i := range before {
				if before[i] != after[i] {
					positionsMatch = false
					break
				}
			}
			if positionsMatch {
				return DriftReport{Kind: DriftEdit, Added: nil, Removed: nil, Edited: edited}
			}
			// Same set, different order - cache-wise as bad as structural change
			return DriftReport{Kind: DriftReorder, Added: nil, Removed: nil, Edited: nil}
		}
	}

	// Additions present but NOT clean appends - treat as reorder
	return DriftReport{Kind: DriftReorder, Added: added, Removed: nil, Edited: nil}
}