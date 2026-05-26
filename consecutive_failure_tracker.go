package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// ConsecutiveCallTracker tracks consecutive identical tool call failures
// to prevent wasted turns on the same broken call.
//
// DeepSeek-Reasonix pattern: when the same tool call (same tool + same args)
// fails validation two times in a row, return a sharper error telling the model
// NOT to retry with identical args. This saves an entire wasted API round-trip.
type ConsecutiveCallTracker struct {
	// _lastMalformed tracks per-tool fingerprint of the last validation failure
	lastMalformed map[string]string // toolName -> argsFingerprint
	// _lastGateRejection tracks per-tool rejection for gate failures
	lastGateRejection map[string]string // toolName -> "reason:fingerprint"
}

// NewConsecutiveCallTracker creates a fresh tracker.
func NewConsecutiveCallTracker() *ConsecutiveCallTracker {
	return &ConsecutiveCallTracker{
		lastMalformed:     make(map[string]string),
		lastGateRejection: make(map[string]string),
	}
}

// fingerprintArgs computes a short hash of tool arguments for fingerprinting.
func fingerprintArgs(args map[string]any) string {
	data, _ := json.Marshal(args)
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8])
}

// CheckMalformedCall checks if a tool call's validation just failed.
// Returns an error hint if this is the second consecutive identical failure.
func (t *ConsecutiveCallTracker) CheckMalformedCall(toolName string, args map[string]any, detail string) (errorHint string) {
	fp := fingerprintArgs(args)
	prev := t.lastMalformed[toolName]
	t.lastMalformed[toolName] = fp

	if prev == fp && prev != "" {
		// Second consecutive identical malformed call
		return fmt.Sprintf("%s: same call just failed validation (%s) — DO NOT retry with identical args. Either fix the call (read the schema in the tool spec) or pick a different tool.", toolName, detail)
	}
	return ""
}

// CheckGateRejection checks if a tool call was just rejected by a gate (edit-gate, shell-gate, etc.).
// Returns an error hint if this is the second consecutive identical rejection.
func (t *ConsecutiveCallTracker) CheckGateRejection(toolName string, args map[string]any, result string) (errorHint string) {
	reason := rejectionReason(toolName, result)
	if reason == "" {
		// Not a rejection, clear tracking
		delete(t.lastGateRejection, toolName)
		return ""
	}

	fp := fingerprintArgs(args)
	key := fmt.Sprintf("%s:%s", reason, fp)
	prev := t.lastGateRejection[toolName]
	t.lastGateRejection[toolName] = key

	if prev == key && prev != "" {
		// Second consecutive identical gate rejection
		return fmt.Sprintf("%s: same call was just rejected by %s — do not retry identical args. %s", toolName, reason, rejectionRecoveryHint(reason))
	}
	return ""
}

// rejectionReason extracts the rejection reason from a tool result.
func rejectionReason(toolName, result string) string {
	// Check for edit-gate rejection
	if (toolName == "edit_file" || toolName == "write_file") && containsRegex(result, `rejected this edit`) {
		return "edit-gate"
	}
	// Check for shell-gate rejection
	if toolName == "exec" && containsRegex(result, `rejected|not allowed|forbidden`) {
		return "shell-gate"
	}
	// Check for read-before-edit rejection
	if (toolName == "edit_file" || toolName == "multi_edit") && containsRegex(result, `read.*first|read.*before`) {
		return "read-before-edit"
	}
	// Check for engineering-lifecycle rejection
	if containsRegex(result, `engineering.?lifecycle|checkpoint|evidence`) {
		return "engineering-lifecycle"
	}
	return ""
}

// rejectionRecoveryHint returns tool-specific recovery guidance.
func rejectionRecoveryHint(reason string) string {
	switch reason {
	case "edit-gate":
		return "Do not re-emit the same edit. Try a genuinely different edit or ask the user how to proceed."
	case "read-before-edit":
		return "Call read_file on the target path first, then re-issue the edit."
	case "shell-gate":
		return "Do not retry the same command. Use an allowlisted/read-only command, wait for approval, or ask the user how to proceed."
	case "engineering-lifecycle":
		return "Switch to read-only exploration, submit or revise the plan, or choose a different tool call."
	default:
		return "Choose a different tool call or ask the user how to proceed."
	}
}

// containsRegex is a simple contains check (simplified from regex)
func containsRegex(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Clear resets the tracker state.
func (t *ConsecutiveCallTracker) Clear() {
	t.lastMalformed = make(map[string]string)
	t.lastGateRejection = make(map[string]string)
}