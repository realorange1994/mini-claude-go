package main

import (
	"strings"
)

// ─── Bash Command Arity (MiMo-Code 1) ──────────────────────────────────────
//
// Maps shell command prefixes to their "arity" for permission decisions.
//
// MiMo-Code source: permission/arity.ts (163 lines)

// ArityEntry represents a command arity mapping.
type ArityEntry struct {
	Prefix []string
	Level  int // 0=safe, 1=moderate, 2=dangerous
}

// BashArityClassifier classifies bash commands by arity.
type BashArityClassifier struct {
	entries []ArityEntry
}

// NewBashArityClassifier creates a new arity classifier.
func NewBashArityClassifier() *BashArityClassifier {
	c := &BashArityClassifier{}
	c.registerDefaults()
	return c
}

// registerDefaults registers default arity mappings.
func (c *BashArityClassifier) registerDefaults() {
	// Safe commands (level 0)
	safe := []string{
		"ls", "pwd", "echo", "cat", "head", "tail", "wc", "grep",
		"find", "which", "whoami", "date", "env", "printenv",
	}
	for _, cmd := range safe {
		c.entries = append(c.entries, ArityEntry{Prefix: []string{cmd}, Level: 0})
	}

	// Moderate commands (level 1)
	moderate := []string{
		"git status", "git log", "git diff", "git branch",
		"npm list", "npm info", "npm view",
		"docker ps", "docker images",
	}
	for _, cmd := range moderate {
		parts := strings.Split(cmd, " ")
		c.entries = append(c.entries, ArityEntry{Prefix: parts, Level: 1})
	}

	// Dangerous commands (level 2)
	dangerous := []string{
		"git push", "git reset --hard", "git clean -f",
		"rm -rf", "sudo", "chmod 777",
		"npm publish", "docker rm", "docker rmi",
	}
	for _, cmd := range dangerous {
		parts := strings.Split(cmd, " ")
		c.entries = append(c.entries, ArityEntry{Prefix: parts, Level: 2})
	}
}

// Classify classifies a command by its arity.
func (c *BashArityClassifier) Classify(command string) int {
	tokens := tokenizeCommandBash(command)
	if len(tokens) == 0 {
		return 0
	}

	bestMatch := 0
	bestLen := 0

	for _, entry := range c.entries {
		if matchesPrefix(tokens, entry.Prefix) && len(entry.Prefix) > bestLen {
			bestMatch = entry.Level
			bestLen = len(entry.Prefix)
		}
	}

	return bestMatch
}

// tokenizeCommandBash tokenizes a command string.
func tokenizeCommandBash(command string) []string {
	return strings.Fields(strings.TrimSpace(command))
}

// matchesPrefix checks if tokens start with the given prefix.
func matchesPrefix(tokens, prefix []string) bool {
	if len(tokens) < len(prefix) {
		return false
	}
	for i, p := range prefix {
		if tokens[i] != p {
			return false
		}
	}
	return true
}

// FormatArity formats an arity level for display.
func FormatArity(level int) string {
	switch level {
	case 0:
		return "safe"
	case 1:
		return "moderate"
	case 2:
		return "dangerous"
	default:
		return "unknown"
	}
}
