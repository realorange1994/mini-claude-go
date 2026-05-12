package permissions

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ─── Path safety utilities ────────────────────────────────────────────────────

// hasPathPrefix checks whether abs starts with dir followed by a path separator.
// This is the secure replacement for strings.Contains(abs, dir) which can be
// fooled by paths like /home/user/my-.claude/projects/evil.
//
// It handles the edge case where abs == dir exactly (returns true),
// and the case where abs starts with dir + separator (returns true).
func hasPathPrefix(abs, dir string) bool {
	if abs == dir {
		return true
	}
	if !strings.HasPrefix(abs, dir) {
		return false
	}
	// Check that the character after dir is a path separator
	remainder := abs[len(dir):]
	if len(remainder) == 0 {
		return true
	}
	return remainder[0] == filepath.Separator || remainder[0] == '/' || remainder[0] == '\\'
}

// hasPathComponent checks whether abs contains a full path component matching comp.
// This is the secure replacement for strings.Contains(abs, comp) which can match
// substrings (e.g., "session-memory" matches "my-session-memory-evil").
//
// It checks that comp appears between path separators (or at start/end).
// Handles both / and \ separators for cross-platform compatibility.
func hasPathComponent(abs, comp string) bool {
	fwdSep := "/" + comp + "/"
	bwdSep := string(filepath.Separator) + comp + string(filepath.Separator)
	// Check for /comp/ or \comp\
	if strings.Contains(abs, fwdSep) || strings.Contains(abs, bwdSep) {
		return true
	}
	// Check starts with comp/
	if strings.HasPrefix(abs, comp+"/") || strings.HasPrefix(abs, comp+string(filepath.Separator)) {
		return true
	}
	// Check ends with /comp
	if strings.HasSuffix(abs, "/"+comp) || strings.HasSuffix(abs, string(filepath.Separator)+comp) {
		return true
	}
	// Exact match
	if abs == comp {
		return true
	}
	return false
}

// hasSuspiciousColon checks for Windows Alternate Data Streams (ADS) colon
// patterns that could be used for path traversal attacks.
// On Windows, "file.txt:Zone.Identifier" is an ADS.
// Drive letters like "C:" at position 1 are allowed.
func hasSuspiciousColon(path string) bool {
	// Allow drive letter colon at position 1 (e.g., "C:\")
	if len(path) >= 2 && path[1] == ':' {
		path = path[2:] // skip drive letter
	}
	return strings.Contains(path, ":")
}

// isSymlinkEscape checks whether a path resolves to a location outside the
// expected parent directory via symlinks. Returns true if escape is detected
// or if the path cannot be resolved.
func isSymlinkEscape(path, parentDir string) bool {
	// Resolve symlinks in the path
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		// If the path doesn't exist yet, try resolving the parent
		parent := filepath.Dir(path)
		resolvedParent, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return true // can't resolve — treat as potential escape
		}
		resolved = filepath.Join(resolvedParent, filepath.Base(path))
	}

	// Resolve the parent directory
	resolvedParent, err := filepath.EvalSymlinks(parentDir)
	if err != nil {
		return true // can't resolve parent — treat as potential escape
	}

	// Check that the resolved path is within the parent
	if !hasPathPrefix(resolved, resolvedParent) {
		return true
	}
	return false
}

// ─── Internal path checks ─────────────────────────────────────────────────────

// IsInternalEditablePath checks if a path is Claude Code's internal editable path
// that should bypass dangerous-directory checks. Matches upstream's
// checkEditableInternalPath.
func IsInternalEditablePath(path, cwd string) bool {
	abs := resolvePath(path, cwd)

	// Reject paths with suspicious colon (ADS attack on Windows/WSL)
	if hasSuspiciousColon(abs) {
		return false
	}

	// Plan files: ~/.claude/plans/*.md or {plansDir}/{planSlug}.md
	if isPlanFile(abs) {
		return true
	}

	// Scratchpad directory: /tmp/claude-{uid}/*/scratchpad/
	if isInScratchpadDir(abs) {
		return true
	}

	// Template job directory: under ~/.claude/jobs/
	if isInJobsDir(abs) {
		return true
	}

	// Agent memory directory: ~/.claude/projects/*/agent-memory/
	if isAgentMemoryPath(abs) {
		return true
	}

	// Auto memory directory: under ~/.claude/ (default path)
	if isInAutoMemoryDir(abs) {
		return true
	}

	// Launch config: .claude/launch.json in project dir only
	if isLaunchConfig(abs, cwd) {
		return true
	}

	return false
}

// IsInternalReadablePath checks if a path is Claude Code's internal readable path.
// Matches upstream's checkReadableInternalPath.
func IsInternalReadablePath(path string) bool {
	abs := resolvePath(path, "")

	// Reject paths with suspicious colon (ADS attack on Windows/WSL)
	if hasSuspiciousColon(abs) {
		return false
	}

	// Session memory directory
	if isInSessionMemoryDir(abs) {
		return true
	}

	// Project directory: ~/.claude/projects/*/
	if isInProjectsDir(abs) {
		return true
	}

	// Plan files
	if isPlanFile(abs) {
		return true
	}

	// Tool results directory
	if isInToolResultsDir(abs) {
		return true
	}

	// Scratchpad directory
	if isInScratchpadDir(abs) {
		return true
	}

	// Project temp directory: /tmp/claude-{uid}/*/
	if isInProjectTempDir(abs) {
		return true
	}

	// Agent memory directory
	if isAgentMemoryPath(abs) {
		return true
	}

	// Auto memory directory
	if isInAutoMemoryDir(abs) {
		return true
	}

	// Tasks directory: ~/.claude/tasks/
	if isInTasksDir(abs) {
		return true
	}

	// Teams directory: ~/.claude/teams/
	if isInTeamsDir(abs) {
		return true
	}

	// Bundled skills root: /tmp/claude-{uid}/bundled-skills/
	if isInBundledSkillsDir(abs) {
		return true
	}

	return false
}

// isPlanFile checks if path is a session plan file.
func isPlanFile(abs string) bool {
	if home := homeDir(); home != "" {
		plansDir := filepath.Join(home, ".claude", "plans")
		if hasPathPrefix(abs, plansDir) && filepath.Ext(abs) == ".md" {
			return true
		}
	}
	return false
}

// isInScratchpadDir checks for scratchpad directory.
// Pattern: {os.TempDir}/claude-{uid}/.../scratchpad/... (or /tmp/claude-{uid}/.../scratchpad/... on Unix).
func isInScratchpadDir(abs string) bool {
	tmp := os.TempDir()
	if !hasPathPrefix(abs, tmp) {
		// On Unix, also accept /tmp/ prefix
		if runtime.GOOS != "windows" && !strings.HasPrefix(abs, "/tmp/") {
			return false
		}
	}
	return hasPathComponent(abs, "scratchpad") && isInProjectTempDir(abs)
}

// isInJobsDir checks for template job directory under ~/.claude/jobs/.
func isInJobsDir(abs string) bool {
	if home := homeDir(); home != "" {
		jobsDir := filepath.Join(home, ".claude", "jobs")
		return hasPathPrefix(abs, jobsDir)
	}
	return false
}

// isAgentMemoryPath checks for agent memory directory.
// Must be under ~/.claude/projects/*/agent-memory/
func isAgentMemoryPath(abs string) bool {
	if home := homeDir(); home != "" {
		projectsDir := filepath.Join(home, ".claude", "projects")
		// Must start with ~/.claude/projects/ and contain agent-memory as a path component
		if hasPathPrefix(abs, projectsDir) && hasPathComponent(abs, "agent-memory") {
			return true
		}
	}
	return false
}

// isInAutoMemoryDir checks for auto memory directory under ~/.claude/.
func isInAutoMemoryDir(abs string) bool {
	if home := homeDir(); home != "" {
		memoryDir := filepath.Join(home, ".claude", "auto-memory")
		return hasPathPrefix(abs, memoryDir)
	}
	return false
}

// isLaunchConfig checks for .claude/launch.json in project dir only.
func isLaunchConfig(abs, cwd string) bool {
	if cwd != "" {
		launchConfig := filepath.Join(cwd, ".claude", "launch.json")
		abs = resolvePath(abs, cwd)
		return abs == launchConfig
	}
	return false
}

// isInSessionMemoryDir checks for session memory directory.
// Must be under ~/.claude/projects/*/session-memory/
func isInSessionMemoryDir(abs string) bool {
	if home := homeDir(); home != "" {
		projectsDir := filepath.Join(home, ".claude", "projects")
		// Must start with ~/.claude/projects/ and contain session-memory as a path component
		if hasPathPrefix(abs, projectsDir) && hasPathComponent(abs, "session-memory") {
			return true
		}
	}
	return false
}

// isInProjectsDir checks for ~/.claude/projects/.
func isInProjectsDir(abs string) bool {
	if home := homeDir(); home != "" {
		projDir := filepath.Join(home, ".claude", "projects")
		return hasPathPrefix(abs, projDir)
	}
	return false
}

// isInToolResultsDir checks for tool results directory.
// Must be under ~/.claude/projects/*/tool-results/
func isInToolResultsDir(abs string) bool {
	if home := homeDir(); home != "" {
		projectsDir := filepath.Join(home, ".claude", "projects")
		// Must start with ~/.claude/projects/ and contain tool-results as a path component
		if hasPathPrefix(abs, projectsDir) && hasPathComponent(abs, "tool-results") {
			return true
		}
	}
	return false
}

// isInProjectTempDir checks for project temp directory.
// Pattern: {os.TempDir}/claude-{uid}/...
func isInProjectTempDir(abs string) bool {
	tmp := os.TempDir()
	if !hasPathPrefix(abs, tmp) {
		return false
	}
	// Check that the first component after temp dir starts with "claude-"
	rest := abs[len(tmp):]
	if len(rest) == 0 {
		return false // just the temp dir itself
	}
	// Skip the leading separator
	if rest[0] == filepath.Separator {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return false
	}
	// Find the end of the first component (next separator or end of string)
	i := strings.IndexFunc(rest, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	if i > 0 {
		return strings.HasPrefix(rest[:i], "claude-")
	}
	// No separator — the whole rest is one component
	return strings.HasPrefix(rest, "claude-")
}

// isInTasksDir checks for ~/.claude/tasks/.
func isInTasksDir(abs string) bool {
	if home := homeDir(); home != "" {
		tasksDir := filepath.Join(home, ".claude", "tasks")
		return hasPathPrefix(abs, tasksDir)
	}
	return false
}

// isInTeamsDir checks for ~/.claude/teams/.
func isInTeamsDir(abs string) bool {
	if home := homeDir(); home != "" {
		teamsDir := filepath.Join(home, ".claude", "teams")
		return hasPathPrefix(abs, teamsDir)
	}
	return false
}

// isInBundledSkillsDir checks for bundled skills directory.
// Must be under {os.TempDir}/claude-{uid}/bundled-skills/ (or /tmp/claude-{uid}/bundled-skills/ on Unix).
func isInBundledSkillsDir(abs string) bool {
	tmp := os.TempDir()
	if !hasPathPrefix(abs, tmp) {
		// On Unix, also accept /tmp/ prefix for non-standard tmp locations
		if runtime.GOOS != "windows" && !strings.HasPrefix(abs, "/tmp/") {
			return false
		}
	}
	// Check that bundled-skills appears as a path component
	// AND the path is under a claude-* directory
	return hasPathComponent(abs, "bundled-skills") && isInProjectTempDir(abs)
}

// homeDir returns the user's home directory.
func homeDir() string {
	if p := os.Getenv("USERPROFILE"); p != "" {
		return p
	}
	if p := os.Getenv("HOME"); p != "" {
		return p
	}
	return ""
}

// resolvePath resolves a relative path to an absolute path.
func resolvePath(path, cwd string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return filepath.Clean(filepath.Join(cwd, path))
}
