package main

import (
	"crypto/sha256"
	"encoding/json"
	"sort"
	"sync"
)

// ─── Repeated Step Detection (MiMo-Code 6) ─────────────────────────────────
//
// Detects when consecutive assistant steps produce identical action signatures.
// Complementary to doom-loop detector (which tracks tool failures).
//
// MiMo-Code source: session/prompt.ts (117-157 lines)

const (
	RepeatedStepThreshold = 3
	RepeatedStepBuffer    = 5
)

// StepSignature represents a deterministic signature of an assistant step.
type StepSignature struct {
	Hash string
	ToolCalls []string
}

// RepeatedStepDetector detects repeated step patterns.
type RepeatedStepDetector struct {
	mu     sync.Mutex
	buffer []StepSignature
}

// NewRepeatedStepDetector creates a new repeated step detector.
func NewRepeatedStepDetector() *RepeatedStepDetector {
	return &RepeatedStepDetector{
		buffer: make([]StepSignature, 0, RepeatedStepBuffer),
	}
}

// CheckRecord records a step's tool calls and checks for repetition.
// Returns true if RepeatedStepThreshold consecutive steps have the same signature.
func (d *RepeatedStepDetector) CheckRecord(toolCalls []map[string]any) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	signature := computeStepSignature(toolCalls)

	d.buffer = append(d.buffer, signature)
	if len(d.buffer) > RepeatedStepBuffer {
		d.buffer = d.buffer[1:]
	}

	if len(d.buffer) < RepeatedStepThreshold {
		return false
	}

	last := d.buffer[len(d.buffer)-1]
	count := 0
	for i := len(d.buffer) - 1; i >= 0; i-- {
		if d.buffer[i].Hash == last.Hash {
			count++
		} else {
			break
		}
	}

	return count >= RepeatedStepThreshold
}

// Reset resets the detector state.
func (d *RepeatedStepDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.buffer = make([]StepSignature, 0, RepeatedStepBuffer)
}

// computeStepSignature computes a deterministic signature of tool calls.
func computeStepSignature(toolCalls []map[string]any) StepSignature {
	if len(toolCalls) == 0 {
		return StepSignature{Hash: "empty"}
	}

	var tools []string
	for _, call := range toolCalls {
		name, _ := call["name"].(string)
		input, _ := call["input"].(map[string]any)

		// Stable JSON serialization (sorted keys)
		stableInput := stableStringify(input)
		tools = append(tools, name+":"+stableInput)
	}

	sort.Strings(tools)
	combined := ""
	for _, t := range tools {
		combined += t + ";"
	}

	hash := sha256.Sum256([]byte(combined))
	return StepSignature{
		Hash:      string(hash[:8]),
		ToolCalls: tools,
	}
}

// stableStringify produces deterministic JSON with sorted keys.
func stableStringify(v any) string {
	if v == nil {
		return "null"
	}

	switch val := v.(type) {
	case map[string]any:
		if val == nil {
			return "null"
		}
		if len(val) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		result := "{"
		for i, k := range keys {
			if i > 0 {
				result += ","
			}
			result += `"` + k + `":` + stableStringify(val[k])
		}
		result += "}"
		return result
	case []any:
		result := "["
		for i, item := range val {
			if i > 0 {
				result += ","
			}
			result += stableStringify(item)
		}
		result += "]"
		return result
	default:
		data, _ := json.Marshal(v)
		return string(data)
	}
}
