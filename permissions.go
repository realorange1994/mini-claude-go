package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"miniclaudecode-go/permissions"
	"miniclaudecode-go/tools"
)

// PermissionDenied is returned when a tool call is blocked.
type PermissionDenied struct {
	Reason string
}

func (e PermissionDenied) Error() string { return e.Reason }

// PermissionGate implements the two-layer permission check.
type PermissionGate struct {
	config        *Config
	classifier    *AutoModeClassifier
	transcriptSrc TranscriptSource
	denialCount   int // consecutive denial count for auto mode
	// recentlyApproved tracks recent user approvals from AskUserQuestion.
	// When the classifier would deny a tool, we check if the user already
	// explicitly approved it — if so, bypass the classifier.
	recentlyApproved []approvedAction
	// ruleStore holds permission rules loaded from settings.json files.
	ruleStore *permissions.RuleStore
	// projectDir is the working directory for path validation.
	projectDir string
	// strippedRules holds dangerous rules stripped in auto mode for restoration.
	strippedRules map[string][]*permissions.ParsedRule
}

// approvedAction records a user-approved dangerous tool call via AskUserQuestion.
type approvedAction struct {
	toolName string
	params   string // compact serialization for matching
	expires  time.Time
}

// TranscriptSource provides compact transcript data for the classifier.
type TranscriptSource interface {
	BuildCompactTranscript(maxMessages int) string
}

func NewPermissionGate(cfg *Config) *PermissionGate {
	return &PermissionGate{config: cfg}
}

// WithRuleStore sets the rule store for permission rule checks.
func (g *PermissionGate) WithRuleStore(rs *permissions.RuleStore, projectDir string) *PermissionGate {
	g.ruleStore = rs
	g.projectDir = projectDir
	return g
}

// WithClassifier sets the auto mode classifier.
func (g *PermissionGate) WithClassifier(c *AutoModeClassifier) *PermissionGate {
	g.classifier = c
	return g
}

// WithTranscriptSource sets the transcript source for the classifier.
func (g *PermissionGate) WithTranscriptSource(src TranscriptSource) *PermissionGate {
	g.transcriptSrc = src
	return g
}

// ResetPostCompact clears classifier cache and approval state after context compaction.
func (g *PermissionGate) ResetPostCompact() {
	if g.classifier != nil {
		g.classifier.ClearCache()
	}
	g.recentlyApproved = nil
	g.denialCount = 0
}

// Check runs the permission gauntlet. Returns a ToolResult if denied, nil if allowed.
// Implements upstream's hasPermissionsToUseToolInner flow:
//   0:  auto mode: strip dangerous rules on entry, restore on exit
//   1a: tool-level deny rule → deny (bypass-immune)
//   1b: content-specific deny rule → deny
//   1c: file path validation → deny/ask/safetyCheck
//   1d: tool-level ask rule → ask (bypass-immune)
//   1e: content-specific ask rule → ask (bypass-immune)
//   2:  tool.CheckPermissions() → get PermissionResult
//   2d: behavior === 'deny' → hard deny (bypass-immune)
//   2e: requiresUserInteraction + ask → ask (bypass-immune)
//   2f: content-specific ask rule → ask (bypass-immune)
//   2g: safetyCheck → ask (bypass-immune)
//   3a: bypassPermissions → behavior: 'allow' (only reached if 1-2 didn't return)
//   3b: allow rule → allow
//   4: passthrough → ask (mode-based)
func (g *PermissionGate) Check(tool tools.Tool, params map[string]any) *tools.ToolResult {
	// STEP 0: Auto mode — strip dangerous allow rules on entry
	autoModeStripped := false
	if g.config.PermissionMode == ModeAuto && g.ruleStore != nil && !g.shouldAvoidPrompts() {
		if g.strippedRules == nil {
			g.strippedRules = g.ruleStore.StripDangerousAllowRules()
			autoModeStripped = true
		}
	}

	// Helper: restore stripped rules on early exit
	restoreStripped := func() {
		if autoModeStripped && g.strippedRules != nil && g.ruleStore != nil {
			g.ruleStore.RestoreStrippedRules(g.strippedRules)
			g.strippedRules = nil
		}
	}

	// Get tool name and content for rule matching
	toolName := tool.Name()
	upstreamName := tools.InternalToUpstreamName(toolName)
	content := g.extractRuleContent(toolName, params)
	pathParam := g.extractPathParam(toolName, params)

	// STEP 1a: Tool-level deny rule (bypass-immune)
	if rule := g.findToolLevelDeny(upstreamName); rule != nil {
		restoreStripped()
		return &tools.ToolResult{
			Output:  fmt.Sprintf("Permission denied by rule: %s", rule.Content),
			IsError: true,
		}
	}

	// STEP 1b: Content-specific deny rule (bypass-immune)
	if rule := g.findContentDeny(upstreamName, content); rule != nil {
		restoreStripped()
		return &tools.ToolResult{
			Output:  fmt.Sprintf("Permission denied by rule: %s", rule.Content),
			IsError: true,
		}
	}

	// STEP 1c: File path validation for write/read/fileops tools
	if pathParam != "" {
		opType := permissions.OpRead
		if g.isWriteTool(toolName) {
			opType = permissions.OpWrite
		}
		var vResult permissions.PathValidationResult
		if opType == permissions.OpRead {
			vResult = permissions.ValidateReadPath(pathParam, g.ruleStore)
		} else {
			vResult = permissions.ValidatePath(pathParam, opType, g.ruleStore, g.projectDir)
		}
		if !vResult.Allowed {
			restoreStripped()
			// Check if it requires user interaction (ask)
			if vResult.Reason == "safetyCheck" || vResult.Reason == "rule" {
				// Return as a safetyCheck ask — caller will prompt user
				askResult := tools.PermissionResultAsk(vResult.Message, vResult.Reason)
				askResult.MatchedRule = vResult.Reason
				if g.shouldAvoidPrompts() {
					return &tools.ToolResult{
						Output:  fmt.Sprintf("Permission denied: %s (interactive prompts disabled for sub-agent)", askResult.Message),
						IsError: true,
					}
				}
				if !g.askUserWithWarning(tool.Name(), params, askResult.Message) {
					return &tools.ToolResult{Output: "Permission denied: user rejected.", IsError: true}
				}
				// User approved — allow to continue
			} else {
				return &tools.ToolResult{
					Output:  fmt.Sprintf("Permission denied: %s", vResult.Message),
					IsError: true,
				}
			}
		}
	}

	// STEP 1d: Tool-level ask rule (bypass-immune)
	if rule := g.findToolLevelAsk(upstreamName); rule != nil {
		restoreStripped()
		if g.shouldAvoidPrompts() {
			return &tools.ToolResult{
				Output:  fmt.Sprintf("Permission denied: %s requires confirmation (interactive prompts disabled for sub-agent)", rule.Content),
				IsError: true,
			}
		}
		msg := fmt.Sprintf("Tool requires confirmation by rule: %s", rule.Content)
		if !g.askUserWithWarning(tool.Name(), params, msg) {
			return &tools.ToolResult{Output: "Permission denied: user rejected.", IsError: true}
		}
		return nil // user approved
	}

	// STEP 1e: Content-specific ask rule (bypass-immune)
	if rule := g.findContentAsk(upstreamName, content); rule != nil {
		restoreStripped()
		if g.shouldAvoidPrompts() {
			return &tools.ToolResult{
				Output:  fmt.Sprintf("Permission denied: %s requires confirmation (interactive prompts disabled for sub-agent)", rule.Content),
				IsError: true,
			}
		}
		msg := fmt.Sprintf("Tool requires confirmation by rule: %s", rule.Content)
		if !g.askUserWithWarning(tool.Name(), params, msg) {
			return &tools.ToolResult{Output: "Permission denied: user rejected.", IsError: true}
		}
		return nil // user approved
	}

	// STEP 2: tool-level self-check
	result := tool.CheckPermissions(params)

	// Step 2d: deny is always bypass-immune
	if result.Behavior == tools.PermissionDeny {
		restoreStripped()
		return &tools.ToolResult{
			Output:  fmt.Sprintf("Permission denied: %s", result.Message),
			IsError: true,
		}
	}

	// Step 2e: ask from safetyCheck is bypass-immune
	if result.Behavior == tools.PermissionAsk && result.DecisionReason == "safetyCheck" {
		restoreStripped()
		if g.shouldAvoidPrompts() {
			return &tools.ToolResult{
				Output:  fmt.Sprintf("Permission denied: %s (interactive prompts disabled for sub-agent)", result.Message),
				IsError: true,
			}
		}
		if !g.askUserWithWarning(tool.Name(), params, result.Message) {
			return &tools.ToolResult{Output: "Permission denied: user rejected.", IsError: true}
		}
		return nil // user approved
	}

	// Step 2f: ask from tool rules (non-safetyCheck) — also bypass-immune per upstream
	if result.Behavior == tools.PermissionAsk {
		restoreStripped()
		if g.shouldAvoidPrompts() {
			return &tools.ToolResult{
				Output:  fmt.Sprintf("Permission denied: %s (interactive prompts disabled for sub-agent)", result.Message),
				IsError: true,
			}
		}
		if !g.askUserWithWarning(tool.Name(), params, result.Message) {
			return &tools.ToolResult{Output: "Permission denied: user rejected.", IsError: true}
		}
		return nil // user approved
	}

	// Layer 1.5: denied patterns check (hard denial)
	if len(g.config.DeniedPatterns) > 0 {
		var target string
		switch tool.Name() {
		case "exec":
			target, _ = params["command"].(string)
		case "write_file", "edit_file", "multi_edit":
			target, _ = params["file_path"].(string)
		case "fileops":
			target, _ = params["path"].(string)
		}
		if target != "" {
			lower := strings.ToLower(target)
			for _, pattern := range g.config.DeniedPatterns {
				if strings.Contains(lower, strings.ToLower(pattern)) {
					restoreStripped()
					return &tools.ToolResult{
						Output:  fmt.Sprintf("Permission denied: matches denied pattern %q", pattern),
						IsError: true,
					}
				}
			}
		}
	}

	// Step 3a: bypass mode — allow all (only reached if 1-2 didn't return)
	switch g.config.PermissionMode {
	case ModeBypass:
		restoreStripped()
		return nil
	case ModePlan:
		writeTools := map[string]bool{"exec": true, "write_file": true, "edit_file": true, "multi_edit": true, "fileops": true}
		if writeTools[tool.Name()] {
			restoreStripped()
			return &tools.ToolResult{
				Output:  fmt.Sprintf("Permission denied: '%s' is blocked in plan (read-only) mode.", tool.Name()),
				IsError: true,
			}
		}

	case ModeAsk:
		dangerousTools := map[string]bool{
			"exec": true, "write_file": true, "edit_file": true,
			"multi_edit": true, "fileops": true,
		}
		isDangerous := dangerousTools[tool.Name()]

		if g.shouldAvoidPrompts() {
			if isDangerous {
				restoreStripped()
				return &tools.ToolResult{Output: fmt.Sprintf("Permission denied: '%s' requires user approval (interactive prompts disabled for sub-agent).", tool.Name()), IsError: true}
			}
			restoreStripped()
			return nil // non-dangerous tool, allow
		}

		if isDangerous {
			if tool.Name() == "exec" {
				cmd, _ := params["command"].(string)
				if g.isSafeCommand(cmd) {
					restoreStripped()
					return nil // Safe command, allow without asking
				}
			}
			if !g.askUser(tool.Name(), params) {
				restoreStripped()
				return &tools.ToolResult{Output: "Permission denied: user rejected.", IsError: true}
			}
		}

	case ModeAuto:
		ret := g.checkAutoMode(tool, params, result)
		if ret != nil {
			restoreStripped()
		}
		return ret
	}

	// Step 3b: allow — tool-level allow is always allowed
	if result.Behavior == tools.PermissionAllow {
		restoreStripped()
		return nil
	}

	// Step 4: passthrough — defer to mode-based logic.
	// For Ask mode: prompt for dangerous tools (already handled in switch above).
	// For Auto mode: go through classifier.
	// For Bypass mode: allow all (already handled in switch above).
	if result.Behavior == tools.PermissionPassthrough {
		switch g.config.PermissionMode {
		case ModeAuto:
			ret := g.checkAutoMode(tool, params, result)
			if ret != nil {
				restoreStripped()
			}
			return ret
		default:
			// Already handled in switch above (Ask/Bypass/Plan).
			restoreStripped()
			return nil
		}
	}
	return nil
}

func (g *PermissionGate) isSafeCommand(command string) bool {
	cmd := strings.TrimSpace(strings.ToLower(command))
	for _, safe := range g.config.AllowedCommands {
		if strings.HasPrefix(cmd, safe) {
			remainder := cmd[len(safe):]
			if remainder == "" {
				return true
			}
			// Only allow if remainder is a simple argument (no shell metacharacters)
			if remainder[0] == ' ' && !containsShellMetacharacters(remainder[1:]) {
				return true
			}
		}
	}
	return false
}

// containsShellMetacharacters checks if a string contains characters that
// could be used for command injection (shell operators, redirections, etc.).
func containsShellMetacharacters(s string) bool {
	return strings.ContainsAny(s, "&|;`$(){}[]<>!#~\n\r")
}

// shouldAvoidPrompts reports whether interactive permission prompts should be
// skipped (e.g. sub-agents that have no terminal user to ask).
func (g *PermissionGate) shouldAvoidPrompts() bool {
	return g.config.ShouldAvoidPermissionPrompts
}

func (g *PermissionGate) askUser(toolName string, params map[string]any) bool {
	return g.askUserWithWarning(toolName, params, "")
}

// checkAutoMode implements the auto mode permission check using the classifier.
// Safe tools are auto-allowed. Other tools are evaluated by the LLM classifier.
// After 3 consecutive denials, falls back to interactive prompt.
// IMPORTANT: If the user explicitly approved this tool via AskUserQuestion, we
// bypass the classifier entirely (the user's explicit consent is binding).
func (g *PermissionGate) checkAutoMode(tool tools.Tool, params map[string]any, toolResult tools.PermissionResult) *tools.ToolResult {
	// If tool returned ask with ClassifierApprovable=false, always prompt user
	// (classifier cannot approve suspicious Windows patterns, etc.)
	if toolResult.Behavior == tools.PermissionAsk && !toolResult.ClassifierApprovable {
		if g.shouldAvoidPrompts() {
			return &tools.ToolResult{
				Output:  fmt.Sprintf("Permission denied: %s (interactive prompts disabled for sub-agent)", toolResult.Message),
				IsError: true,
			}
		}
		if !g.askUserWithWarning(tool.Name(), params, toolResult.Message) {
			return &tools.ToolResult{Output: "Permission denied: user rejected.", IsError: true}
		}
		return nil // user approved
	}

	// Step 3c: toolAlwaysAllowedRule — if rule store has a tool-level allow rule,
	// allow without classifier evaluation.
	if g.ruleStore != nil && g.ruleStore.HasAllowRule(tool.Name()) {
		g.denialCount = 0
		return nil
	}

	// Fast path: whitelisted tools are always allowed
	if IsAutoAllowlisted(tool.Name(), params) {
		g.denialCount = 0
		return nil
	}

	// If classifier is not available, fall back to legacy behavior: allow all
	if g.classifier == nil || !g.classifier.IsEnabled() {
		// No classifier configured: auto mode allows all tools (old behavior)
		return nil
	}

	// Check if this tool was explicitly approved by the user via AskUserQuestion.
	// If the user said "Yes, continue" to a question about this exact tool/action,
	// their explicit consent is binding — skip the classifier.
	if g.toolMatchesRecentApproval(tool.Name(), params) {
		g.denialCount = 0
		return nil
	}

	// Build transcript for classifier context
	transcript := ""
	if g.transcriptSrc != nil {
		transcript = g.transcriptSrc.BuildCompactTranscript(20)
	}

	// Call classifier
	result := g.classifier.Classify(tool.Name(), params, transcript)

	if !result.Allow {
		g.denialCount++
		// After 3 consecutive denials, fall back to interactive prompt
		// (unless interactive prompts are disabled for sub-agents)
		if g.denialCount >= 3 && !g.shouldAvoidPrompts() {
			fmt.Fprintf(os.Stderr, "  [auto-classifier] %d consecutive denials, falling back to manual approval\n", g.denialCount)
			if g.askUser(tool.Name(), params) {
				g.denialCount = 0
				return nil
			}
			return &tools.ToolResult{Output: "Permission denied: user rejected.", IsError: true}
		}
		return &tools.ToolResult{
			Output:  fmt.Sprintf("Permission denied: %s", result.Reason),
			IsError: true,
		}
	}

	// Allowed: reset denial count
	g.denialCount = 0
	return nil
}

func (g *PermissionGate) askUserWithWarning(toolName string, params map[string]any, warning string) bool {
	var detail string
	switch toolName {
	case "exec":
		detail, _ = params["command"].(string)
	case "write_file", "edit_file":
		detail, _ = params["file_path"].(string)
	}

	prompt := fmt.Sprintf("\n[Permission] Allow '%s'", toolName)
	if detail != "" {
		prompt += ": " + detail
	}
	if warning != "" {
		prompt += "\n  [WARN] " + warning
	}
	prompt += "? [y/N] "

	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}

// RecordUserApproval records that the user explicitly approved a tool action
// (typically via AskUserQuestion). This approval is valid for 2 minutes and
// allows the matching tool call to bypass the auto classifier.
func (g *PermissionGate) RecordUserApproval(toolName string, params map[string]any) {
	compact := compactParams(toolName, params)
	g.recentlyApproved = append(g.recentlyApproved, approvedAction{
		toolName: toolName,
		params:   compact,
		expires:  time.Now().Add(2 * time.Minute),
	})
	// Trim old entries
	now := time.Now()
	valid := g.recentlyApproved[:0]
	for _, a := range g.recentlyApproved {
		if a.expires.After(now) {
			valid = append(valid, a)
		}
	}
	g.recentlyApproved = valid
}

// toolMatchesRecentApproval checks if the given tool call matches a recent
// explicit user approval (from AskUserQuestion). If so, the classifier is bypassed.
func (g *PermissionGate) toolMatchesRecentApproval(toolName string, params map[string]any) bool {
	now := time.Now()
	compact := compactParams(toolName, params)
	for _, a := range g.recentlyApproved {
		if a.expires.After(now) && a.toolName == toolName && a.params == compact {
			return true
		}
	}
	return false
}

// compactParams produces a compact string representation of tool params for
// matching user approvals. Only includes the key identifying parameter.
func compactParams(toolName string, params map[string]any) string {
	switch toolName {
	case "exec":
		if cmd, ok := params["command"].(string); ok {
			return cmd
		}
	case "write_file", "edit_file", "multi_edit":
		if p, ok := params["file_path"].(string); ok {
			return p
		}
	case "fileops":
		if p, ok := params["path"].(string); ok {
			return p
		}
	case "git":
		if args, ok := params["args"].(string); ok {
			return args
		}
	default:
		data, err := json.Marshal(params)
		if err == nil {
			return string(data)
		}
	}
	return ""
}

// extractRuleContent extracts the content parameter for rule matching.
// For exec/bash tools, uses the command; for file tools, uses the path.
func (g *PermissionGate) extractRuleContent(toolName string, params map[string]any) string {
	switch toolName {
	case "exec":
		if cmd, ok := params["command"].(string); ok {
			return cmd
		}
	case "write_file", "edit_file", "multi_edit":
		if p, ok := params["file_path"].(string); ok {
			return p
		}
	case "read_file":
		if p, ok := params["file_path"].(string); ok {
			return p
		}
	case "fileops":
		if p, ok := params["path"].(string); ok {
			return p
		}
	case "git":
		if args, ok := params["args"].(string); ok {
			return args
		}
	}
	return ""
}

// extractPathParam extracts the file path parameter for path validation.
func (g *PermissionGate) extractPathParam(toolName string, params map[string]any) string {
	switch toolName {
	case "write_file", "edit_file", "multi_edit", "read_file":
		if p, ok := params["file_path"].(string); ok {
			return p
		}
	case "fileops":
		if p, ok := params["path"].(string); ok {
			return p
		}
	}
	return ""
}

// isWriteTool returns true if the tool performs write operations.
func (g *PermissionGate) isWriteTool(toolName string) bool {
	switch toolName {
	case "write_file", "edit_file", "multi_edit", "fileops":
		return true
	}
	return false
}

// findToolLevelDeny checks if there's a tool-level deny rule for the given tool.
func (g *PermissionGate) findToolLevelDeny(upstreamName string) *permissions.ParsedRule {
	if g.ruleStore == nil {
		return nil
	}
	if g.ruleStore.HasDenyRule(upstreamName) {
		if rules := g.ruleStore.GetRulesForTool(upstreamName); len(rules) > 0 {
			for _, r := range rules {
				if r.Behavior == "deny" && r.IsToolLevel() {
					return r
				}
			}
		}
		return &permissions.ParsedRule{ToolName: upstreamName, Content: upstreamName, Behavior: "deny"}
	}
	return nil
}

// findContentDeny checks if there's a content-specific deny rule for the given tool and content.
func (g *PermissionGate) findContentDeny(upstreamName, content string) *permissions.ParsedRule {
	if g.ruleStore == nil || content == "" {
		return nil
	}
	return g.ruleStore.FindContentRule(upstreamName, content, "deny")
}

// findToolLevelAsk checks if there's a tool-level ask rule for the given tool.
func (g *PermissionGate) findToolLevelAsk(upstreamName string) *permissions.ParsedRule {
	if g.ruleStore == nil {
		return nil
	}
	if g.ruleStore.HasAskRule(upstreamName) {
		if rules := g.ruleStore.GetRulesForTool(upstreamName); len(rules) > 0 {
			for _, r := range rules {
				if r.Behavior == "ask" && r.IsToolLevel() {
					return r
				}
			}
		}
		return &permissions.ParsedRule{ToolName: upstreamName, Content: upstreamName, Behavior: "ask"}
	}
	return nil
}

// findContentAsk checks if there's a content-specific ask rule for the given tool and content.
func (g *PermissionGate) findContentAsk(upstreamName, content string) *permissions.ParsedRule {
	if g.ruleStore == nil || content == "" {
		return nil
	}
	return g.ruleStore.FindContentRule(upstreamName, content, "ask")
}
