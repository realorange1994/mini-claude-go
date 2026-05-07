package tools

import (
	"regexp"
	"strings"
)

// DANGEROUS_FILES lists files that should not be auto-edited without explicit
// permission. These can be used for code execution or data exfiltration.
// Matches upstream's DANGEROUS_FILES.
var DANGEROUS_FILES = []string{
	".gitconfig",
	".gitmodules",
	".bashrc",
	".bash_profile",
	".zshrc",
	".zprofile",
	".profile",
	".ripgreprc",
	".mcp.json",
	".claude.json",
}

// DANGEROUS_DIRECTORIES lists directories that should be protected from
// auto-editing. Matches upstream's DANGEROUS_DIRECTORIES.
var DANGEROUS_DIRECTORIES = []string{
	".git",
	".vscode",
	".idea",
	".claude",
}

// isDangerousFilePath checks if a file path points to a dangerous file or
// directory. Returns (true, message) if dangerous, (false, "") if safe.
// Matches upstream's isDangerousFilePathToAutoEdit.
func isDangerousFilePath(path string) (bool, string) {
	fp := expandPath(path)

	// UNC path check
	if isUncPath(fp) {
		return true, "path appears to be a UNC path that could access network resources"
	}

	// Check if any path segment is a dangerous directory
	segments := strings.Split(strings.ReplaceAll(fp, "\\", "/"), "/")
	for _, seg := range segments {
		segLower := strings.ToLower(seg)
		for _, dir := range DANGEROUS_DIRECTORIES {
			// Skip .claude/worktrees/ (structural path, not user-created)
			if dir == ".claude" {
				idx := indexOf(segments, seg)
				if idx >= 0 && idx+1 < len(segments) && strings.ToLower(segments[idx+1]) == "worktrees" {
					continue
				}
			}
			if segLower == dir {
				return true, "file is inside a sensitive directory: " + dir
			}
		}
	}

	// Check filename against dangerous files
	if len(segments) > 0 {
		fileName := strings.ToLower(segments[len(segments)-1])
		for _, dangerousFile := range DANGEROUS_FILES {
			if fileName == strings.ToLower(dangerousFile) {
				return true, "file is a sensitive configuration file: " + dangerousFile
			}
		}
	}

	return false, ""
}

// hasSuspiciousWindowsPathPattern detects suspicious Windows path patterns.
// Returns (true, message) if suspicious, (false, "") if safe.
// Matches upstream's hasSuspiciousWindowsPathPattern.
func hasSuspiciousWindowsPathPattern(path string) (bool, string) {
	// NTFS Alternate Data Streams: colon after position 2 (e.g., file.txt:Zone.Identifier)
	// Matches upstream: if (isWsl || isWindows) && path.slice(2).includes(':')
	if len(path) > 2 {
		rest := path[2:]
		if strings.Contains(rest, ":") {
			return true, "path contains NTFS Alternate Data Stream syntax"
		}
	}
	// 8.3 short names (GIT~1, CLAUDE~1)
	if reShortName.MatchString(path) {
		return true, "path contains 8.3 short name pattern"
	}
	// Long path prefixes (\\?\, \\.\, //?/, //./)
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) ||
		strings.HasPrefix(path, "//?/") || strings.HasPrefix(path, "//./") {
		return true, "path uses a long path prefix"
	}
	// Trailing dots or spaces (.git., .claude , .bashrc...)
	if reTrailingDotsSpace.MatchString(path) {
		return true, "path has trailing dots or spaces"
	}
	// DOS device names (.git.CON, settings.json.PRN)
	if reDosDevices.MatchString(path) {
		return true, "path contains a DOS device name"
	}
	// Three or more consecutive dots as path component (.../file.txt)
	if reTripleDots.MatchString(path) {
		return true, "path contains three or more consecutive dots as a path component"
	}
	// UNC paths
	if isUncPath(path) {
		return true, "path is a UNC path that could leak credentials"
	}
	return false, ""
}

var reShortName = regexp.MustCompile(`~\d`)
var reTrailingDotsSpace = regexp.MustCompile(`[.\s]+$`)
var reDosDevices = regexp.MustCompile(`\.(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?i)$`)
var reTripleDots = regexp.MustCompile(`(^|[/\\])\.{3,}([/\\]|$)`)

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}

// CheckPathSafetyForAutoEdit checks if a file path is safe for auto-editing.
// Returns PermissionResult: allow if safe, ask with safetyCheck reason if unsafe.
// Matches upstream's checkPathSafetyForAutoEdit.
func CheckPathSafetyForAutoEdit(path string) PermissionResult {
	// Check suspicious Windows path patterns (NOT classifier-approvable)
	if suspicious, _ := hasSuspiciousWindowsPathPattern(path); suspicious {
		return PermissionResultAskNotClassifiable(
			"Claude requested permissions to write to "+path+", which contains a suspicious Windows path pattern that requires manual approval.",
			"safetyCheck",
		)
	}

	// Check Claude config files (classifier-approvable)
	if isClaudeConfigPath(path) {
		return PermissionResultAsk(
			"Claude requested permissions to edit "+path+", which is a Claude configuration file that requires manual approval.",
			"safetyCheck",
		)
	}

	// Check dangerous files/directories (classifier-approvable)
	if dangerous, msg := isDangerousFilePath(path); dangerous {
		return PermissionResultAsk(
			"Claude requested permissions to edit "+path+" which is a sensitive file: "+msg,
			"safetyCheck",
		)
	}

	return PermissionResultPassthrough()
}

// isClaudeConfigPath checks if a path is a Claude Code configuration file or
// directory that should be auto-edit blocked but classifier-approvable.
// Matches upstream's Claude config file check in checkPathSafetyForAutoEdit.
func isClaudeConfigPath(path string) bool {
	p := strings.ReplaceAll(strings.ToLower(path), "\\", "/")
	configPaths := []string{
		".claude/settings.json",
		".claude/settings.local.json",
		".claude/commands/",
		".claude/agents/",
		".claude/skills/",
	}
	for _, configPath := range configPaths {
		if strings.HasSuffix(p, configPath) || strings.Contains(p, "/"+configPath) {
			return true
		}
	}
	return false
}
