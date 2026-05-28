package permissions

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

// PermissionType represents different permission types
type PermissionType string

const (
	PermBash    PermissionType = "bash"
	PermEdit    PermissionType = "edit"
	PermWrite   PermissionType = "write"
	PermDelete  PermissionType = "delete"
	PermRead    PermissionType = "read"
	PermMcp     PermissionType = "mcp"
	PermMcpAuth PermissionType = "mcp_auth"
	PermClient  PermissionType = "client_request"
)

// PermissionDecision represents the user's decision
type PermissionDecision int

const (
	DecisionAllow     PermissionDecision = iota
	DecisionDeny
	DecisionAllowOnce
	DecisionDenyOnce
)

// Rule represents a permission rule
type Rule struct {
	Type     PermissionType
	Pattern  string // glob pattern for matching
	Decision PermissionDecision
}

// Request represents a permission request
type Request struct {
	Type    PermissionType
	Target  string
	Message string
	ToolId  string
}

// Manager manages permissions
type Manager struct {
	rules       []Rule
	defaultMode string      // "allow", "deny", "ask"
	mu          sync.RWMutex
	input       *bufio.Reader // for reading user input
}

// NewManager creates a new permission manager
func NewManager(defaultMode string) *Manager {
	return &Manager{
		rules:       []Rule{},
		defaultMode: defaultMode,
		input:       bufio.NewReader(os.Stdin),
	}
}

// Check checks if an action is permitted
func (m *Manager) Check(req Request) (PermissionDecision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check rules first (most specific first)
	for _, rule := range m.rules {
		if rule.Type == req.Type && matchPattern(rule.Pattern, req.Target) {
			return rule.Decision, nil
		}
	}

	// Use default mode
	switch m.defaultMode {
	case "allow":
		return DecisionAllow, nil
	case "deny":
		return DecisionDeny, nil
	default:
		return m.askUser(req)
	}
}

// AddRule adds a permission rule
func (m *Manager) AddRule(rule Rule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules = append(m.rules, rule)
}

// RemoveRule removes a permission rule
func (m *Manager) RemoveRule(typePerm PermissionType, pattern string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, rule := range m.rules {
		if rule.Type == typePerm && rule.Pattern == pattern {
			m.rules = append(m.rules[:i], m.rules[i+1:]...)
			return
		}
	}
}

// SetDefaultMode sets the default permission mode
func (m *Manager) SetDefaultMode(mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultMode = mode
}

func (m *Manager) askUser(req Request) (PermissionDecision, error) {
	fmt.Printf("\nPermission required: %s\n  Target: %s\n  Message: %s\n", req.Type, req.Target, req.Message)
	fmt.Print("Allow? (y/n/always/never): ")

	input, _ := m.input.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	switch input {
	case "y", "yes":
		return DecisionAllowOnce, nil
	case "n", "no":
		return DecisionDenyOnce, nil
	case "always":
		m.AddRule(Rule{Type: req.Type, Pattern: req.Target, Decision: DecisionAllow})
		return DecisionAllow, nil
	case "never":
		m.AddRule(Rule{Type: req.Type, Pattern: req.Target, Decision: DecisionDeny})
		return DecisionDeny, nil
	default:
		return DecisionDenyOnce, nil
	}
}

func matchPattern(pattern, target string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == target {
		return true
	}
	// Simple glob matching
	if strings.HasPrefix(pattern, "*.") {
		ext := pattern[1:] // e.g., ".go"
		return strings.HasSuffix(target, ext)
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(target, prefix)
	}
	return false
}
