package permissions

import (
	"path/filepath"
	"regexp"
	"strings"

	"miniclaudecode-go/tools"
)

// OperationType represents the type of file operation.
type OperationType int

const (
	OpRead OperationType = iota
	OpWrite
	OpCreate
)

// PathValidationResult holds the result of path validation.
type PathValidationResult struct {
	Allowed bool
	Reason  string // "rule", "safetyCheck", "other", "workingDir"
	Message string
}

// path validation regexes
var (
	reTilde       = regexp.MustCompile(`^~[^/]`)  // ~root, ~+, ~-
	reShellExpand = regexp.MustCompile(`^[$%=]`)  // $VAR, %VAR%, =prefix
	reGlobWrite   = regexp.MustCompile(`[*?[\]{}]`) // glob chars in write
)

// ValidatePath performs comprehensive path validation for file operations.
// Matches upstream's validatePath + isPathAllowed flow.
func ValidatePath(path string, opType OperationType, ruleStore *RuleStore, cwd string) PathValidationResult {
	result := PathValidationResult{Allowed: false}

	// 1. Expand ~ to homedir
	expanded := expandTilde(path)

	// 2. Block UNC paths
	if isUncPath(expanded) {
		result.Message = "UNC network paths require manual approval"
		result.Reason = "other"
		return result
	}

	// 3. Block tilde variants after expansion (~root, ~+, ~-)
	if reTilde.MatchString(expanded) {
		result.Message = "Tilde expansion variants require manual approval"
		result.Reason = "other"
		return result
	}

	// 4. Block shell expansion syntax: $VAR, %VAR%, =prefix
	if reShellExpand.MatchString(expanded) {
		result.Message = "Shell expansion syntax in paths requires manual approval"
		result.Reason = "other"
		return result
	}

	// 5. Block glob patterns in write/create operations
	if (opType == OpWrite || opType == OpCreate) && reGlobWrite.MatchString(expanded) {
		result.Message = "Glob patterns are not allowed in write operations"
		result.Reason = "other"
		return result
	}

	// Resolve symlinks for safety checks
	resolved := resolveSymlinks(expanded)

	// 6a. Check deny rules in rule store
	if ruleStore != nil {
		// Check content-specific deny rules for Edit tool
		if denyRule := ruleStore.FindContentRule(tools.FileEditToolName, path, "deny"); denyRule != nil {
			result.Message = "Path denied by rule: " + FormatRule(denyRule)
			result.Reason = "rule"
			return result
		}
		if denyRule := ruleStore.FindContentRule(tools.FileWriteToolName, path, "deny"); denyRule != nil {
			result.Message = "Path denied by rule: " + FormatRule(denyRule)
			result.Reason = "rule"
			return result
		}
	}

	// 6b. Internal editable paths bypass dangerous-dir checks
	if opType == OpWrite || opType == OpCreate {
		if IsInternalEditablePath(expanded, cwd) {
			result.Allowed = true
			return result
		}
	}

	// 6c. Safety checks (dangerous files, directories, Windows patterns)
	if opType == OpWrite || opType == OpCreate {
		safetyResult := tools.CheckPathSafetyForAutoEdit(resolved)
		if safetyResult.Behavior == tools.PermissionAsk {
			result.Message = safetyResult.Message
			result.Reason = "safetyCheck"
			return result
		}
	}

	// 6d. Check ask rules in rule store
	if ruleStore != nil {
		if askRule := ruleStore.FindContentRule(tools.FileEditToolName, path, "ask"); askRule != nil {
			result.Message = "Path requires confirmation by rule: " + FormatRule(askRule)
			result.Reason = "rule"
			return result
		}
		if askRule := ruleStore.FindContentRule(tools.FileWriteToolName, path, "ask"); askRule != nil {
			result.Message = "Path requires confirmation by rule: " + FormatRule(askRule)
			result.Reason = "rule"
			return result
		}
	}

	// 6e. Check allow rules
	if ruleStore != nil {
		if allowRule := ruleStore.FindContentRule(tools.FileEditToolName, path, "allow"); allowRule != nil {
			result.Allowed = true
			return result
		}
		if allowRule := ruleStore.FindContentRule(tools.FileWriteToolName, path, "allow"); allowRule != nil {
			result.Allowed = true
			return result
		}
	}

	// Default: for write operations, deny; for read, also deny (conservative)
	result.Message = "Path access not allowed by default"
	result.Reason = "other"
	return result
}

// ValidateReadPath performs path validation for read operations.
func ValidateReadPath(path string, ruleStore *RuleStore) PathValidationResult {
	result := PathValidationResult{Allowed: false}

	// 1. UNC paths
	expanded := expandTilde(path)
	if isUncPath(expanded) {
		result.Message = "UNC network paths require manual approval"
		result.Reason = "other"
		return result
	}

	// 2. Suspicious Windows patterns (prompt user, not hard denial)
	if suspicious, msg := hasSuspiciousWindowsPathPattern(expanded); suspicious {
		result.Message = msg
		result.Reason = "safetyCheck"
		return result
	}

	// 3. Check deny rules for Read tool
	if ruleStore != nil {
		if denyRule := ruleStore.FindContentRule(tools.FileReadToolName, path, "deny"); denyRule != nil {
			result.Message = "Path denied by rule: " + FormatRule(denyRule)
			result.Reason = "rule"
			return result
		}
	}

	// 4. Check ask rules for Read tool
	if ruleStore != nil {
		if askRule := ruleStore.FindContentRule(tools.FileReadToolName, path, "ask"); askRule != nil {
			result.Message = "Path requires confirmation by rule: " + FormatRule(askRule)
			result.Reason = "rule"
			return result
		}
	}

	// 5. Internal readable paths
	if IsInternalReadablePath(expanded) {
		result.Allowed = true
		return result
	}

	// 6. Check allow rules for Read tool
	if ruleStore != nil {
		if allowRule := ruleStore.FindContentRule(tools.FileReadToolName, path, "allow"); allowRule != nil {
			result.Allowed = true
			return result
		}
	}

	// Default: deny
	result.Message = "Path read not allowed by default"
	result.Reason = "other"
	return result
}

// expandTilde expands ~ or ~/ to the user's home directory.
func expandTilde(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home := homeDir(); home != "" {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

// resolveSymlinks resolves symlinks for safety checks.
func resolveSymlinks(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// isUncPath checks for vulnerable UNC paths.
func isUncPath(path string) bool {
	return strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//")
}

// hasSuspiciousWindowsPathPattern checks for suspicious Windows path patterns.
// This duplicates tools.hasSuspiciousWindowsPathPattern since it's not exported.
func hasSuspiciousWindowsPathPattern(path string) (bool, string) {
	// 8.3 short names
	if matched, _ := regexp.MatchString(`~\d`, path); matched {
		return true, "path contains 8.3 short name pattern"
	}
	// Long path prefixes
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) ||
		strings.HasPrefix(path, "//?/") || strings.HasPrefix(path, "//./") {
		return true, "path uses a long path prefix"
	}
	// Trailing dots or spaces
	if matched, _ := regexp.MatchString(`[.\s]+$`, path); matched {
		return true, "path has trailing dots or spaces"
	}
	// DOS device names
	if matched, _ := regexp.MatchString(`\.(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?i)$`, path); matched {
		return true, "path contains a DOS device name"
	}
	// Three or more consecutive dots
	if matched, _ := regexp.MatchString(`(^|[/\\])\.{3,}([/\\]|$)`, path); matched {
		return true, "path contains three or more consecutive dots as a path component"
	}
	// UNC paths
	if isUncPath(path) {
		return true, "path is a UNC path that could leak credentials"
	}
	return false, ""
}
