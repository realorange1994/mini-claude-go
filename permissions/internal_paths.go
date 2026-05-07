package permissions

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// IsInternalEditablePath checks if a path is Claude Code's internal editable path
// that should bypass dangerous-directory checks. Matches upstream's
// checkEditableInternalPath.
func IsInternalEditablePath(path, cwd string) bool {
	abs := resolvePath(path, cwd)

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
		if strings.HasPrefix(abs, plansDir) && filepath.Ext(abs) == ".md" {
			return true
		}
	}
	return false
}

// isInScratchpadDir checks for scratchpad directory.
func isInScratchpadDir(abs string) bool {
	if runtime.GOOS == "windows" {
		tmp := os.TempDir()
		if strings.HasPrefix(abs, tmp) && strings.Contains(abs, "scratchpad") {
			return true
		}
	} else {
		if strings.HasPrefix(abs, "/tmp/claude-") && strings.Contains(abs, "scratchpad") {
			return true
		}
	}
	return false
}

// isInJobsDir checks for template job directory under ~/.claude/jobs/.
func isInJobsDir(abs string) bool {
	if home := homeDir(); home != "" {
		jobsDir := filepath.Join(home, ".claude", "jobs")
		return strings.HasPrefix(abs, jobsDir)
	}
	return false
}

// isAgentMemoryPath checks for agent memory directory.
func isAgentMemoryPath(abs string) bool {
	if home := homeDir(); home != "" {
		if strings.Contains(abs, filepath.Join(".claude", "projects")) &&
			strings.Contains(abs, "agent-memory") {
			return true
		}
	}
	return false
}

// isInAutoMemoryDir checks for auto memory directory under ~/.claude/.
func isInAutoMemoryDir(abs string) bool {
	if home := homeDir(); home != "" {
		memoryDir := filepath.Join(home, ".claude", "auto-memory")
		return strings.HasPrefix(abs, memoryDir)
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
func isInSessionMemoryDir(abs string) bool {
	return strings.Contains(abs, "session-memory")
}

// isInProjectsDir checks for ~/.claude/projects/.
func isInProjectsDir(abs string) bool {
	if home := homeDir(); home != "" {
		projDir := filepath.Join(home, ".claude", "projects")
		return strings.HasPrefix(abs, projDir)
	}
	return false
}

// isInToolResultsDir checks for tool results directory.
func isInToolResultsDir(abs string) bool {
	return strings.Contains(abs, "tool-results")
}

// isInProjectTempDir checks for project temp directory.
func isInProjectTempDir(abs string) bool {
	if runtime.GOOS == "windows" {
		tmp := os.TempDir()
		return strings.HasPrefix(abs, tmp) && strings.Contains(abs, "claude-")
	}
	return strings.HasPrefix(abs, "/tmp/claude-")
}

// isInTasksDir checks for ~/.claude/tasks/.
func isInTasksDir(abs string) bool {
	if home := homeDir(); home != "" {
		tasksDir := filepath.Join(home, ".claude", "tasks")
		return strings.HasPrefix(abs, tasksDir)
	}
	return false
}

// isInTeamsDir checks for ~/.claude/teams/.
func isInTeamsDir(abs string) bool {
	if home := homeDir(); home != "" {
		teamsDir := filepath.Join(home, ".claude", "teams")
		return strings.HasPrefix(abs, teamsDir)
	}
	return false
}

// isInBundledSkillsDir checks for bundled skills directory.
func isInBundledSkillsDir(abs string) bool {
	return strings.Contains(abs, "bundled-skills")
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
